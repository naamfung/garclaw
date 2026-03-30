package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "strconv"
    "strings"
    "time"

    "github.com/toon-format/toon-go"
)

// AGENTIC_TAGS 用于前端解析工具调用的标记
const (
    AgenticToolCallStart   = "<<<AGENTIC_TOOL_CALL_START>>>"
    AgenticToolCallEnd     = "<<<AGENTIC_TOOL_CALL_END>>>"
    AgenticToolNamePrefix  = "<<<TOOL_NAME:"
    AgenticToolArgsStart   = "<<<TOOL_ARGS_START>>>"
    AgenticToolArgsEnd     = "<<<TOOL_ARGS_END>>>"
    AgenticTagSuffix       = ">>>"
)

// sanitizeContent 清理内容中的非法控制字符
func sanitizeContent(content string) string {
    var builder strings.Builder
    builder.Grow(len(content))

    for _, r := range content {
        switch r {
        case '\n', '\t':
            builder.WriteRune(r)
        case '\r':
            continue
        default:
            if r < 0x20 || r == 0x7F {
                continue
            }
            builder.WriteRune(r)
        }
    }
    return builder.String()
}

// sendToolCallStart 发送工具调用开始标记
func sendToolCallStart(ch Channel, toolName string, argsJSON string) {
    var sb strings.Builder
    sb.WriteString(AgenticToolCallStart)
    sb.WriteString("\n")
    sb.WriteString(AgenticToolNamePrefix)
    sb.WriteString(toolName)
    sb.WriteString(AgenticTagSuffix)
    sb.WriteString("\n")
    sb.WriteString(AgenticToolArgsStart)
    sb.WriteString(argsJSON)
    sb.WriteString(AgenticToolArgsEnd)
    sb.WriteString("\n")
    ch.WriteChunk(StreamChunk{Content: sb.String()})
}

// sendToolCallEnd 发送工具调用结束标记
func sendToolCallEnd(ch Channel) {
    ch.WriteChunk(StreamChunk{Content: AgenticToolCallEnd + "\n"})
}

// getCurrentTaskDescriptionFromMessages 从消息历史中提取最后一条用户消息作为任务描述
func getCurrentTaskDescriptionFromMessages(messages []Message) string {
    for i := len(messages) - 1; i >= 0; i-- {
        if messages[i].Role == "user" {
            if content, ok := messages[i].Content.(string); ok && content != "" {
                return content
            }
        }
    }
    return ""
}

// executeTool 执行单个工具调用，返回增强消息
func executeTool(ctx context.Context, toolID, toolName string, argsMap map[string]interface{}, ch Channel, role *Role) EnrichedMessage {
    var content string
    status := TaskStatusSuccess

    if ctx.Err() == context.Canceled {
        return CancelToolResult(toolID, CancelByUser, "User cancelled before execution", toolName)
    }

    if role != nil && !role.IsToolAllowed(toolName) {
        errMsg := fmt.Sprintf("❌ 权限拒绝：当前角色「%s」无权使用工具「%s」。\n\n可用工具：%v",
            role.DisplayName, toolName, getAllowedToolsList(role))
        argsJSON, _ := json.Marshal(map[string]interface{}{"error": "permission denied"})
        sendToolCallStart(ch, toolName, string(argsJSON))
        ch.WriteChunk(StreamChunk{Content: errMsg + "\n"})
        sendToolCallEnd(ch)
        return NewToolResultMessage(toolID, errMsg, TaskStatusFailed, toolName)
    }

    argsJSON, _ := json.Marshal(argsMap)
    sendToolCallStart(ch, toolName, string(argsJSON))
    defer sendToolCallEnd(ch)

    switch toolName {
    case "smart_shell":
        content, _ = handleSmartShell(ctx, argsMap, ch)

    case "shell":
        command, ok := argsMap["command"].(string)
        if !ok || command == "" {
            content = "Error: Invalid or empty command"
            status = TaskStatusFailed
        } else {
            force := false
            if forceVal, ok := argsMap["force"].(bool); ok {
                force = forceVal
            }
            isBlockingConfirmed := false
            if confirmedVal, ok := argsMap["is_blocking_confirmed"].(bool); ok {
                isBlockingConfirmed = confirmedVal
            }

            result := runShellWithTimeout(ctx, command, force, isBlockingConfirmed)

            if result.ConfirmRequired {
                var confirmResult strings.Builder
                confirmResult.WriteString("⚠️ **确认请求**\n\n")
                confirmResult.WriteString(result.ConfirmMessage)
                confirmResult.WriteString("\n\n---\n")
                confirmResult.WriteString("要强制执行此命令，请使用: `shell(command=\"...\", force=true)`\n")
                confirmResult.WriteString("或使用建议的替代命令。")

                content = confirmResult.String()
                status = TaskStatusSuccess
            } else if result.Err != nil {
                if ctx.Err() == context.Canceled {
                    return CancelToolResult(toolID, CancelByUser, "Command cancelled by user", toolName)
                } else {
                    content = fmt.Sprintf("Error: %v", result.Err)
                    status = TaskStatusFailed
                }
            } else {
                content = result.Stdout
                if result.ExitCode != 0 && result.Stderr != "" {
                    content += "\n" + result.Stderr
                    status = TaskStatusFailed
                }
            }
            fmt.Println(content)
        }

    case "read_file_line":
        filename, ok1 := argsMap["filename"].(string)
        lineNumFloat, ok2 := argsMap["line_num"].(float64)
        if !ok1 || !ok2 || filename == "" || lineNumFloat < 1 {
            content = "Error: Invalid arguments for read_file_line"
        } else {
            lineNum := int(lineNumFloat)
            c, err := ReadFileLine(filename, lineNum)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                content = c
            }
            fmt.Println(TruncateString(content, 200))
        }

    case "write_file_line":
        filename, ok1 := argsMap["filename"].(string)
        lineNumFloat, ok2 := argsMap["line_num"].(float64)
        text, ok3 := argsMap["content"].(string)
        if !ok1 || !ok2 || !ok3 || filename == "" || lineNumFloat < 1 {
            content = "Error: Invalid arguments for write_file_line"
        } else {
            lineNum := int(lineNumFloat)
            err := WriteFileLine(filename, lineNum, text)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                content = "Successfully wrote to line " + strconv.Itoa(lineNum)
            }
            fmt.Println(content)
        }

    case "read_all_lines":
        filename, ok := argsMap["filename"].(string)
        if !ok || filename == "" {
            content = "Error: Invalid arguments for read_all_lines"
        } else {
            lines, err := ReadAllLines(filename)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                linesJSON, err := json.Marshal(lines)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    content = string(linesJSON)
                }
            }
            fmt.Println(TruncateString(content, 200))
        }

    case "write_all_lines":
        filename, ok1 := argsMap["filename"].(string)
        linesInterface, ok2 := argsMap["lines"].([]interface{})
        if !ok1 || !ok2 || filename == "" {
            content = "Error: Invalid arguments for write_all_lines"
        } else {
            lines := make([]string, len(linesInterface))
            valid := true
            for i, line := range linesInterface {
                if lineStr, ok := line.(string); ok {
                    lines[i] = lineStr
                } else {
                    content = fmt.Sprintf("Error: line %d is not a string", i)
                    valid = false
                    break
                }
            }
            if valid {
                err := WriteAllLines(filename, lines)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    content = "Successfully wrote " + strconv.Itoa(len(lines)) + " lines to " + filename
                }
                fmt.Println(content)
            }
        }

    case "browser_search":
        keyword, ok := argsMap["keyword"].(string)
        if !ok || keyword == "" {
            content = "Error: Empty keyword in browser_search tool call"
        } else {
            resultsList, err := Search(keyword)
            if err != nil {
                content = "Error: " + err.Error()
            } else if resultsList != nil {
                resultsTOON, err := toon.Marshal(resultsList)
                if err != nil {
                    content = "Error: Failed to marshal search results"
                    log.Printf("Failed to marshal search results: %v", err)
                } else {
                    content = string(resultsTOON)
                }
            } else {
                content = "No search results found"
            }
            fmt.Println("Browser search completed")
        }

    case "browser_visit":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_visit tool call"
        } else {
            result, err := Visit(url)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, err := toon.Marshal(result)
                if err != nil {
                    content = "Error: Failed to marshal visit result"
                } else {
                    content = string(resultTOON)
                }
            }
            fmt.Println("Browser visit completed")
        }

    case "browser_download":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_download tool call"
        } else {
            fileName, err := Download(url)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                content = "Browser download completed, saved to: " + fileName
            }
            fmt.Println(content)
        }

    // ========== 浏览器增强工具 ==========
    case "browser_click":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_click tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_click tool call"
            } else {
                timeout := 0
                if t, ok := argsMap["timeout"].(float64); ok {
                    timeout = int(t)
                }
                result, err := BrowserClick(url, selector, timeout)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser click completed")
            }
        }

    case "browser_type":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_type tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_type tool call"
            } else {
                text, ok := argsMap["text"].(string)
                if !ok {
                    content = "Error: Empty text in browser_type tool call"
                } else {
                    submit, _ := argsMap["submit"].(bool)
                    timeout := 0
                    if t, ok := argsMap["timeout"].(float64); ok {
                        timeout = int(t)
                    }
                    result, err := BrowserType(url, selector, text, submit, timeout)
                    if err != nil {
                        content = "Error: " + err.Error()
                    } else {
                        resultTOON, _ := toon.Marshal(result)
                        content = string(resultTOON)
                    }
                    fmt.Println("Browser type completed")
                }
            }
        }

    case "browser_scroll":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_scroll tool call"
        } else {
            direction, ok := argsMap["direction"].(string)
            if !ok || direction == "" {
                content = "Error: Empty direction in browser_scroll tool call"
            } else {
                amount := 500
                if a, ok := argsMap["amount"].(float64); ok {
                    amount = int(a)
                }
                timeout := 0
                if t, ok := argsMap["timeout"].(float64); ok {
                    timeout = int(t)
                }
                result, err := BrowserScroll(url, direction, amount, timeout)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser scroll completed")
            }
        }

    case "browser_wait_element":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_wait_element tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_wait_element tool call"
            } else {
                timeout := 10
                if t, ok := argsMap["timeout"].(float64); ok {
                    timeout = int(t)
                }
                result, err := BrowserWaitElement(url, selector, timeout)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser wait element completed")
            }
        }

    case "browser_extract_links":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_extract_links tool call"
        } else {
            timeout := 0
            if t, ok := argsMap["timeout"].(float64); ok {
                timeout = int(t)
            }
            result, err := BrowserExtractLinks(url, timeout)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser extract links completed")
        }

    case "browser_extract_images":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_extract_images tool call"
        } else {
            timeout := 0
            if t, ok := argsMap["timeout"].(float64); ok {
                timeout = int(t)
            }
            result, err := BrowserExtractImages(url, timeout)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser extract images completed")
        }

    case "browser_extract_elements":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_extract_elements tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_extract_elements tool call"
            } else {
                includeHTML, _ := argsMap["include_html"].(bool)
                timeout := 0
                if t, ok := argsMap["timeout"].(float64); ok {
                    timeout = int(t)
                }
                result, err := BrowserExtractElements(url, selector, includeHTML, timeout)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser extract elements completed")
            }
        }

    case "browser_screenshot":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_screenshot tool call"
        } else {
            fullPage, _ := argsMap["full_page"].(bool)
            timeout := 0
            if t, ok := argsMap["timeout"].(float64); ok {
                timeout = int(t)
            }
            result, err := BrowserScreenshot(url, fullPage, timeout)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser screenshot completed")
        }

    case "browser_execute_js":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_execute_js tool call"
        } else {
            script, ok := argsMap["script"].(string)
            if !ok || script == "" {
                content = "Error: Empty script in browser_execute_js tool call"
            } else {
                timeout := 0
                if t, ok := argsMap["timeout"].(float64); ok {
                    timeout = int(t)
                }
                result, err := BrowserExecuteJS(url, script, timeout)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser execute JS completed")
            }
        }

    case "browser_fill_form":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_fill_form tool call"
        } else {
            formDataRaw, ok := argsMap["form_data"].(map[string]interface{})
            if !ok {
                content = "Error: Invalid form_data in browser_fill_form tool call"
            } else {
                formData := make(FormData)
                for k, v := range formDataRaw {
                    if strVal, ok := v.(string); ok {
                        formData[k] = strVal
                    }
                }
                submitSelector, _ := argsMap["submit_selector"].(string)
                timeout := 0
                if t, ok := argsMap["timeout"].(float64); ok {
                    timeout = int(t)
                }
                result, err := BrowserFillForm(url, formData, submitSelector, timeout)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser fill form completed")
            }
        }

    // ========== 浏览器高级工具 ==========
    case "browser_hover":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_hover tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_hover tool call"
            } else {
                result, err := BrowserHover(url, selector)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser hover completed")
            }
        }

    case "browser_double_click":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_double_click tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_double_click tool call"
            } else {
                result, err := BrowserDoubleClick(url, selector)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser double click completed")
            }
        }

    case "browser_right_click":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_right_click tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_right_click tool call"
            } else {
                result, err := BrowserRightClick(url, selector)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser right click completed")
            }
        }

    case "browser_drag":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_drag tool call"
        } else {
            sourceSelector, ok := argsMap["source_selector"].(string)
            if !ok || sourceSelector == "" {
                content = "Error: Empty source_selector in browser_drag tool call"
            } else {
                targetSelector, ok := argsMap["target_selector"].(string)
                if !ok || targetSelector == "" {
                    content = "Error: Empty target_selector in browser_drag tool call"
                } else {
                    result, err := BrowserDrag(url, sourceSelector, targetSelector)
                    if err != nil {
                        content = "Error: " + err.Error()
                    } else {
                        resultTOON, _ := toon.Marshal(result)
                        content = string(resultTOON)
                    }
                    fmt.Println("Browser drag completed")
                }
            }
        }

    case "browser_wait_smart":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_wait_smart tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_wait_smart tool call"
            } else {
                opts := BrowserWaitForOptions{
                    Visible: true,
                }
                if v, ok := argsMap["visible"].(bool); ok {
                    opts.Visible = v
                }
                if v, ok := argsMap["interactable"].(bool); ok {
                    opts.Interactable = v
                }
                if v, ok := argsMap["stable"].(bool); ok {
                    opts.Stable = v
                }
                if t, ok := argsMap["timeout"].(float64); ok {
                    opts.Timeout = int(t)
                }
                result, err := BrowserWaitForSmart(url, selector, opts)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser smart wait completed")
            }
        }

    case "browser_navigate":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_navigate tool call"
        } else {
            action, ok := argsMap["action"].(string)
            if !ok || action == "" {
                content = "Error: Empty action in browser_navigate tool call"
            } else {
                var result *BrowserNavigateResult
                var err error
                switch action {
                case "back":
                    result, err = BrowserNavigateBack(url)
                case "forward":
                    result, err = BrowserNavigateForward(url)
                case "refresh":
                    result, err = BrowserRefresh(url)
                default:
                    content = "Error: Invalid action: " + action
                }
                if err != nil {
                    content = "Error: " + err.Error()
                } else if result != nil {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser navigate completed:", action)
            }
        }

    case "browser_get_cookies":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_get_cookies tool call"
        } else {
            result, err := BrowserGetCookies(url)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser get cookies completed")
        }

    case "browser_cookie_save":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_cookie_save tool call"
        } else {
            filePath, _ := argsMap["file_path"].(string)
            result, err := BrowserCookieSave(url, filePath)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser cookie save completed")
        }

    case "browser_cookie_load":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_cookie_load tool call"
        } else {
            filePath, ok := argsMap["file_path"].(string)
            if !ok || filePath == "" {
                content = "Error: Empty file_path in browser_cookie_load tool call"
            } else {
                result, err := BrowserCookieLoad(url, filePath)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser cookie load completed")
            }
        }

    case "browser_snapshot":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_snapshot tool call"
        } else {
            maxDepth := 5
            if d, ok := argsMap["max_depth"].(float64); ok {
                maxDepth = int(d)
            }
            result, err := BrowserSnapshot(url, maxDepth)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser snapshot completed")
        }

    case "browser_upload_file":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_upload_file tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_upload_file tool call"
            } else {
                filePathsRaw, ok := argsMap["file_paths"].([]interface{})
                if !ok {
                    content = "Error: Invalid file_paths in browser_upload_file tool call"
                } else {
                    var filePaths []string
                    for _, p := range filePathsRaw {
                        if s, ok := p.(string); ok {
                            filePaths = append(filePaths, s)
                        }
                    }
                    result, err := BrowserUploadFile(url, selector, filePaths)
                    if err != nil {
                        content = "Error: " + err.Error()
                    } else {
                        resultTOON, _ := toon.Marshal(result)
                        content = string(resultTOON)
                    }
                    fmt.Println("Browser upload file completed")
                }
            }
        }

    case "browser_select_option":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_select_option tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_select_option tool call"
            } else {
                valuesRaw, ok := argsMap["values"].([]interface{})
                if !ok {
                    content = "Error: Invalid values in browser_select_option tool call"
                } else {
                    var values []string
                    for _, v := range valuesRaw {
                        if s, ok := v.(string); ok {
                            values = append(values, s)
                        }
                    }
                    result, err := BrowserSelectOption(url, selector, values)
                    if err != nil {
                        content = "Error: " + err.Error()
                    } else {
                        resultTOON, _ := toon.Marshal(result)
                        content = string(resultTOON)
                    }
                    fmt.Println("Browser select option completed")
                }
            }
        }

    case "browser_key_press":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_key_press tool call"
        } else {
            keysRaw, ok := argsMap["keys"].([]interface{})
            if !ok {
                content = "Error: Invalid keys in browser_key_press tool call"
            } else {
                var keys []string
                for _, k := range keysRaw {
                    if s, ok := k.(string); ok {
                        keys = append(keys, s)
                    }
                }
                result, err := BrowserKeyPress(url, keys)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser key press completed")
            }
        }

    case "browser_element_screenshot":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_element_screenshot tool call"
        } else {
            selector, ok := argsMap["selector"].(string)
            if !ok || selector == "" {
                content = "Error: Empty selector in browser_element_screenshot tool call"
            } else {
                result, err := BrowserElementScreenshot(url, selector)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser element screenshot completed")
            }
        }

    case "browser_pdf":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_pdf tool call"
        } else {
            timeout := 0
            if t, ok := argsMap["timeout"].(float64); ok {
                timeout = int(t)
            }
            result, err := BrowserPDF(url, timeout)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser PDF export completed")
        }

    case "browser_pdf_from_file":
        filePath, ok := argsMap["file_path"].(string)
        if !ok || filePath == "" {
            content = "Error: Empty file_path in browser_pdf_from_file tool call"
        } else {
            result, err := BrowserPDFFromFile(filePath)
            if err != nil {
                content = "Error: " + err.Error()
            } else {
                resultTOON, _ := toon.Marshal(result)
                content = string(resultTOON)
            }
            fmt.Println("Browser PDF from file completed")
        }

    case "browser_set_headers":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_set_headers tool call"
        } else {
            headersInterface, ok := argsMap["headers"].([]interface{})
            if !ok {
                content = "Error: Invalid headers in browser_set_headers tool call"
            } else {
                var headers []string
                for _, h := range headersInterface {
                    if hStr, ok := h.(string); ok {
                        headers = append(headers, hStr)
                    }
                }
                result, err := BrowserSetHeaders(url, headers)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser set headers completed")
            }
        }

    case "browser_set_user_agent":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_set_user_agent tool call"
        } else {
            userAgent, ok := argsMap["user_agent"].(string)
            if !ok || userAgent == "" {
                content = "Error: Empty user_agent in browser_set_user_agent tool call"
            } else {
                result, err := BrowserSetUserAgent(url, userAgent)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser set user agent completed")
            }
        }

    case "browser_emulate_device":
        url, ok := argsMap["url"].(string)
        if !ok || url == "" {
            content = "Error: Empty url in browser_emulate_device tool call"
        } else {
            device, ok := argsMap["device"].(string)
            if !ok || device == "" {
                content = "Error: Empty device in browser_emulate_device tool call"
            } else {
                result, err := BrowserEmulateDevice(url, device)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    resultTOON, _ := toon.Marshal(result)
                    content = string(resultTOON)
                }
                fmt.Println("Browser emulate device completed")
            }
        }

    case "todo":
        itemsInterface, ok := argsMap["items"].([]interface{})
        if !ok {
            content = "Error: Invalid items in todo tool call"
        } else {
            var items []TodoItem
            valid := true
            for _, itemInterface := range itemsInterface {
                itemMap, ok := itemInterface.(map[string]interface{})
                if !ok {
                    content = "Error: Invalid item format"
                    valid = false
                    break
                }
                item := TodoItem{}
                if id, ok := itemMap["id"].(string); ok {
                    item.ID = id
                }
                if text, ok := itemMap["text"].(string); ok {
                    item.Text = text
                } else {
                    content = "Error: Item missing text"
                    valid = false
                    break
                }
                if status, ok := itemMap["status"].(string); ok {
                    item.Status = status
                } else {
                    content = "Error: Item missing status"
                    valid = false
                    break
                }
                items = append(items, item)
            }
            if valid {
                fmt.Println("Updating todo list...")
                output, err := TODO.Update(items)
                if err != nil {
                    content = "Error: " + err.Error()
                } else {
                    content = output
                }
                fmt.Println(content)
            }
        }

    case "cron_add":
        content, _ = handleCronAdd(ctx, argsMap, ch)
    case "cron_remove":
        content, _ = handleCronRemove(ctx, argsMap, ch)
    case "cron_list":
        content, _ = handleCronList(ctx, argsMap, ch)
    case "cron_status":
        content, _ = handleCronStatus(ctx, argsMap, ch)

    case "memory_save":
        content, _ = handleMemorySave(ctx, argsMap, ch)
    case "memory_recall":
        content, _ = handleMemoryRecall(ctx, argsMap, ch)
    case "memory_forget":
        content, _ = handleMemoryForget(ctx, argsMap, ch)
    case "memory_list":
        content, _ = handleMemoryList(ctx, argsMap, ch)

    case "text_search":
        keyword, ok := argsMap["keyword"].(string)
        if !ok || keyword == "" {
            content = "Error: Empty keyword in text_search tool call"
            status = TaskStatusFailed
        } else {
            opts := TextSearchOptions{}
            if rootDir, ok := argsMap["root_dir"].(string); ok && rootDir != "" {
                opts.RootDir = rootDir
            }
            if filePattern, ok := argsMap["file_pattern"].(string); ok {
                opts.FilePattern = filePattern
            }
            if ignoreCase, ok := argsMap["ignore_case"].(bool); ok {
                opts.IgnoreCase = ignoreCase
            }
            if useRegex, ok := argsMap["use_regex"].(bool); ok {
                opts.UseRegex = useRegex
            }
            if maxDepth, ok := argsMap["max_depth"].(float64); ok {
                opts.MaxDepth = int(maxDepth)
            }
            if maxResults, ok := argsMap["max_results"].(float64); ok {
                opts.MaxResults = int(maxResults)
            }

            results, err := TextSearch(keyword, opts)
            if err != nil {
                content = "Error: " + err.Error()
                status = TaskStatusFailed
            } else if len(results) == 0 {
                content = "No matches found"
            } else {
                resultsTOON, err := toon.Marshal(results)
                if err != nil {
                    content = "Error: Failed to marshal search results"
                    status = TaskStatusFailed
                } else {
                    content = string(resultsTOON)
                }
            }
            fmt.Printf("Text search completed: %d results\n", len(results))
        }

    case "text_replace":
        content, _ = handleTextReplace(ctx, argsMap, ch)
    case "text_grep":
        content, _ = handleTextSearch(ctx, argsMap, ch)
    case "text_transform":
        content, _ = handleTextTransform(ctx, argsMap, ch)

    case "plugin_list":
        content, _ = handlePluginList(ctx, argsMap, ch)
    case "plugin_load":
        content, _ = handlePluginLoad(ctx, argsMap, ch)
    case "plugin_unload":
        content, _ = handlePluginUnload(ctx, argsMap, ch)
    case "plugin_reload":
        content, _ = handlePluginReload(ctx, argsMap, ch)
    case "plugin_call":
        content, _ = handlePluginCall(ctx, argsMap, ch)
    case "plugin_compile":
        content, _ = handlePluginCompile(ctx, argsMap, ch)
    case "plugin_delete":
        content, _ = handlePluginDelete(ctx, argsMap, ch)

    case "shell_delayed":
        content, _ = handleDelayedExec(ctx, argsMap, ch)
    case "shell_delayed_check":
        content, _ = handleTaskCheck(ctx, argsMap, ch)
    case "shell_delayed_terminate":
        content, _ = handleTaskTerminate(ctx, argsMap, ch)
    case "shell_delayed_list":
        content, _ = handleTaskList(ctx, argsMap, ch)
    case "shell_delayed_wait":
        content, _ = handleTaskWait(ctx, argsMap, ch)
    case "shell_delayed_remove":
        content, _ = handleTaskRemove(ctx, argsMap, ch)

    case "spawn":
        content, _ = handleSpawn(ctx, argsMap, ch)
    case "spawn_check":
        content, _ = handleSpawnCheck(ctx, argsMap, ch)
    case "spawn_list":
        content, _ = handleSpawnList(ctx, argsMap, ch)
    case "spawn_cancel":
        content, _ = handleSpawnCancel(ctx, argsMap, ch)

    case "consolidate_memory":
        content, _ = HandleConsolidateMemory(argsMap)

    default:
        if strings.HasPrefix(toolName, "mcp_") && globalMCPClientManager != nil {
            result, err := globalMCPClientManager.CallTool(ctx, toolName, argsMap)
            if err != nil {
                content = fmt.Sprintf("Error: %v", err)
                status = TaskStatusFailed
            } else {
                content = result
            }
        } else {
            content = "Error: Unknown tool name"
            status = TaskStatusFailed
        }
    }

    if status == TaskStatusSuccess && (strings.HasPrefix(content, "Error:") || strings.HasPrefix(content, "error:")) {
        status = TaskStatusFailed
    }

    content = sanitizeContent(content)
    if content != "" {
        ch.WriteChunk(StreamChunk{Content: content + "\n"})
    }

    return NewToolResultMessage(toolID, content, status, toolName)
}

func getAllowedToolsList(role *Role) string {
    if role == nil {
        return "所有工具"
    }
    switch role.ToolPermission.Mode {
    case ToolPermissionAll:
        return "所有工具"
    case ToolPermissionAllowlist:
        if len(role.ToolPermission.AllowedTools) == 0 {
            return "无"
        }
        return strings.Join(role.ToolPermission.AllowedTools, ", ")
    case ToolPermissionDenylist:
        return "除 " + strings.Join(role.ToolPermission.DeniedTools, ", ") + " 以外的工具"
    default:
        return "所有工具"
    }
}

// AgentLoop 核心循环
func AgentLoop(ctx context.Context, ch Channel, messages []Message, apiType, baseURL, apiKey, modelID string,
    temperature float64, maxTokens int, stream bool, thinking bool) ([]Message, error) {

    // 注入记忆上下文
    if globalUnifiedMemory != nil {
        taskDesc := getCurrentTaskDescriptionFromMessages(messages)
        memoryContext := globalUnifiedMemory.GetContextForPrompt(taskDesc)
        if memoryContext != "" {
            if len(messages) > 0 && messages[0].Role == "system" {
                if content, ok := messages[0].Content.(string); ok {
                    messages[0].Content = content + "\n\n" + memoryContext
                }
            } else {
                messages = append([]Message{{Role: "system", Content: memoryContext}}, messages...)
            }
        }
    }

    // 获取当前角色的 Role（用于工具权限过滤）和模型配置
    var currentRole *Role
    var effectiveAPIType, effectiveBaseURL, effectiveAPIKey, effectiveModelID string
    var effectiveTemperature float64
    var effectiveMaxTokens int

    effectiveAPIType = apiType
    effectiveBaseURL = baseURL
    effectiveAPIKey = apiKey
    effectiveModelID = modelID
    effectiveTemperature = temperature
    effectiveMaxTokens = maxTokens

    if globalRoleManager != nil && globalActorManager != nil && globalStage != nil {
        currentActor := globalStage.GetCurrentActor()
        if actor, ok := globalActorManager.GetActor(currentActor); ok {
            currentRole, _ = globalRoleManager.GetRole(actor.Role)
            if modelConfig := globalActorManager.GetActorModel(currentActor); modelConfig != nil {
                if modelConfig.APIType != "" {
                    effectiveAPIType = modelConfig.APIType
                }
                if modelConfig.BaseURL != "" {
                    effectiveBaseURL = modelConfig.BaseURL
                }
                if modelConfig.APIKey != "" {
                    effectiveAPIKey = modelConfig.ResolveAPIKey()
                }
                if modelConfig.Model != "" {
                    effectiveModelID = modelConfig.Model
                }
                if modelConfig.Temperature > 0 {
                    effectiveTemperature = modelConfig.Temperature
                }
                if modelConfig.MaxTokens > 0 {
                    effectiveMaxTokens = modelConfig.MaxTokens
                }
            }
        }
    }

    // 注入或更新系统提示
    if len(messages) > 0 {
        hasSystemPrompt := false
        systemPromptIndex := -1
        for i, msg := range messages {
            if msg.Role == "system" {
                hasSystemPrompt = true
                systemPromptIndex = i
                break
            }
        }

        needUpdate := false
        if globalStage != nil {
            needUpdate = globalStage.NeedUpdateSystemPrompt()
        }

        if !hasSystemPrompt || needUpdate {
            var systemPrompt string

            if globalRoleManager != nil && globalActorManager != nil && globalStage != nil {
                currentActor := globalStage.GetCurrentActor()
                systemPrompt = BuildSystemPromptForActor(currentActor, globalActorManager, globalRoleManager, globalStage)
            } else {
                systemPrompt = SYSTEM_PROMPT
            }

            if systemPrompt != "" {
                if needUpdate && systemPromptIndex >= 0 {
                    messages[systemPromptIndex] = Message{Role: "system", Content: systemPrompt}
                    globalStage.ClearUpdateSystemPrompt()
                } else {
                    messages = append([]Message{{Role: "system", Content: systemPrompt}}, messages...)
                }
            }
        }
    }

    hookManager := GetHookManager()
    iteration := 0

    // 记录用户消息到记忆整合器（使用原始内容，无隐式总结）
    if globalMemoryConsolidator != nil && len(messages) > 0 {
        for i := len(messages) - 1; i >= 0; i-- {
            if messages[i].Role == "user" {
                if content, ok := messages[i].Content.(string); ok && content != "" {
                    globalMemoryConsolidator.AddMessage("default", ConsolidationMessage{
                        Role:      "user",
                        Content:   content,
                        Timestamp: time.Now(),
                    })
                    break
                }
            }
        }
    }

    for {
        iteration++
        select {
        case <-ctx.Done():
            return messages, ctx.Err()
        default:
        }

        if hookManager != nil && hookManager.IsEnabled() {
            hookResult := hookManager.RunBeforeModel(ctx, 0, "", iteration, "", len(messages), 0)
            if hookResult.Action == HookOutcomeBlock {
                ch.WriteChunk(StreamChunk{Content: hookResult.Reason, Done: true})
                return messages, fmt.Errorf("blocked by hook: %s", hookResult.Reason)
            }
        }

        chunkChan, err := CallModel(ctx, messages, effectiveAPIType, effectiveBaseURL, effectiveAPIKey, effectiveModelID, effectiveTemperature, effectiveMaxTokens, stream, thinking, currentRole)
        if err != nil {
            if writeErr := ch.WriteChunk(StreamChunk{Error: err.Error()}); writeErr != nil {
                log.Printf("Failed to write error chunk: %v", writeErr)
            }
            return messages, err
        }

        var respContent interface{}
        var reasoningContent string
        var toolCalls []map[string]interface{}
        var stopReason string
        toolCallsMap := make(map[int]map[string]interface{})

        for chunk := range chunkChan {
            select {
            case <-ctx.Done():
                ch.WriteChunk(StreamChunk{Error: ctx.Err().Error()})
                return messages, ctx.Err()
            default:
            }

            if chunk.Error != "" {
                if writeErr := ch.WriteChunk(chunk); writeErr != nil {
                    log.Printf("Failed to write error chunk: %v", writeErr)
                    return messages, fmt.Errorf("%s", chunk.Error)
                }
                return messages, fmt.Errorf("%s", chunk.Error)
            }

            chunkToSend := chunk
            chunkToSend.Done = false
            if writeErr := ch.WriteChunk(chunkToSend); writeErr != nil {
                log.Printf("WebSocket write failed: %v, stopping AgentLoop", writeErr)
                return messages, writeErr
            }

            if chunk.Content != "" {
                if str, ok := respContent.(string); ok {
                    respContent = str + chunk.Content
                } else {
                    respContent = chunk.Content
                }
            }
            if chunk.ReasoningContent != "" {
                reasoningContent += chunk.ReasoningContent
            }
            if len(chunk.ToolCalls) > 0 {
                for _, tc := range chunk.ToolCalls {
                    idx := 0
                    if idxFloat, ok := tc["index"].(float64); ok {
                        idx = int(idxFloat)
                    } else if idxInt, ok := tc["index"].(int); ok {
                        idx = idxInt
                    }

                    existing, exists := toolCallsMap[idx]
                    if !exists {
                        existing = make(map[string]interface{})
                        toolCallsMap[idx] = existing
                    }

                    for k, v := range tc {
                        if k == "function" {
                            funcMap, ok := v.(map[string]interface{})
                            if !ok {
                                existing[k] = v
                                continue
                            }
                            existingFunc, funcOk := existing["function"].(map[string]interface{})
                            if !funcOk {
                                existingFunc = make(map[string]interface{})
                                existing["function"] = existingFunc
                            }
                            for fk, fv := range funcMap {
                                if fk == "arguments" {
                                    if argStr, ok := fv.(string); ok {
                                        if existingArgs, argsOk := existingFunc["arguments"].(string); argsOk {
                                            existingFunc["arguments"] = existingArgs + argStr
                                        } else {
                                            existingFunc["arguments"] = argStr
                                        }
                                    }
                                } else {
                                    existingFunc[fk] = fv
                                }
                            }
                        } else {
                            if v != nil {
                                if str, ok := v.(string); ok && str == "" {
                                    continue
                                }
                                existing[k] = v
                            }
                        }
                    }
                }
            }
            if chunk.Done {
                stopReason = chunk.FinishReason
                break
            }
        }

        if len(toolCallsMap) > 0 {
            maxIdx := 0
            for idx := range toolCallsMap {
                if idx > maxIdx {
                    maxIdx = idx
                }
            }
            toolCalls = make([]map[string]interface{}, 0, maxIdx+1)
            for i := 0; i <= maxIdx; i++ {
                if tc, exists := toolCallsMap[i]; exists {
                    delete(tc, "index")
                    toolCalls = append(toolCalls, tc)
                }
            }
        }

        if stopReason == "tool_use" || stopReason == "function_call" || stopReason == "tool_calls" {
            messages = append(messages, Message{
                Role:      "assistant",
                ToolCalls: toolCalls,
            })
        } else {
            messages = append(messages, Message{
                Role:             "assistant",
                Content:          respContent,
                ReasoningContent: reasoningContent,
            })
        }

        // 记录助手消息到记忆整合器（使用原始内容，无隐式总结）
        if globalMemoryConsolidator != nil {
            contentStr, _ := respContent.(string)
            globalMemoryConsolidator.AddMessage("default", ConsolidationMessage{
                Role:      "assistant",
                Content:   contentStr,
                Timestamp: time.Now(),
            })
        }

        if stopReason != "tool_use" && stopReason != "function_call" && stopReason != "tool_calls" {
            if globalStage != nil && globalStage.AutoSwitchEnabled() {
                contentStr, _ := respContent.(string)
                hasMarker, targetActor, isEnd := ParseSwitchMarker(contentStr)

                if hasMarker && !isEnd && targetActor != "" && globalStage.CanAutoSwitch() {
                    if _, ok := globalActorManager.GetActor(targetActor); ok {
                        cleanedContent := StripSwitchMarker(contentStr)

                        messages[len(messages)-1] = Message{
                            Role:             "assistant",
                            Content:          cleanedContent,
                            ReasoningContent: reasoningContent,
                        }

                        globalStage.SetCurrentActor(targetActor)
                        turns := globalStage.IncrementAutoTurns()

                        switchMsg := fmt.Sprintf("\n═══════════════════════════════════════════════════════════════\n[Auto Switch → %s | Turns: %d/%d]\n═══════════════════════════════════════════════════════════════\n", targetActor, turns, 20)
                        ch.WriteChunk(StreamChunk{Content: switchMsg})

                        newMessages := make([]Message, 0)
                        for _, msg := range messages {
                            if msg.Role != "system" {
                                newMessages = append(newMessages, msg)
                            }
                        }

                        newSystemPrompt := BuildSystemPromptForActor(targetActor, globalActorManager, globalRoleManager, globalStage)
                        if globalUnifiedMemory != nil {
                            memoryContext := globalUnifiedMemory.GetContextForPrompt("")
                            if memoryContext != "" {
                                newSystemPrompt += "\n\n" + memoryContext
                            }
                        }
                        newMessages = append([]Message{{Role: "system", Content: newSystemPrompt}}, newMessages...)
                        messages = newMessages

                        continue
                    }
                } else if isEnd {
                    ch.WriteChunk(StreamChunk{Content: "\n═══════════════════════════════════════════════════════════════\n[Auto Stopped: END marker]\n═══════════════════════════════════════════════════════════════\n"})
                    cleanedContent := StripSwitchMarker(contentStr)
                    messages[len(messages)-1] = Message{
                        Role:             "assistant",
                        Content:          cleanedContent,
                        ReasoningContent: reasoningContent,
                    }
                }
            }
            break
        }

        var results []EnrichedMessage

        if IsDebug {
            fmt.Println("===================== Executing tool calls =====================")
            fmt.Printf("API type: %s\n", apiType)
            fmt.Printf("Response content type: %T\n", respContent)
            fmt.Printf("Response content: %v\n", respContent)
        }

        if apiType == "openai" {
            var toolCallsSlice []interface{}
            for _, m := range toolCalls {
                toolCallsSlice = append(toolCallsSlice, m)
            }

            if len(toolCallsSlice) == 0 {
                if IsDebug {
                    fmt.Printf("Warning: no tool calls to process\n")
                }
                continue
            }

            validToolCalls := []interface{}{}
            type callInfo struct {
                ID       string
                Name     string
                ArgsJSON string
            }
            var callsToProcess []callInfo

            for _, item := range toolCallsSlice {
                toolUse, ok := item.(map[string]interface{})
                if !ok {
                    if IsDebug {
                        fmt.Printf("Warning: invalid tool call item: %v\n", item)
                    }
                    continue
                }

                toolID, ok := toolUse["id"].(string)
                if !ok {
                    if idVal, exists := toolUse["id"]; exists {
                        toolID = fmt.Sprint(idVal)
                    } else {
                        if IsDebug {
                            fmt.Printf("Warning: tool call missing id: %v\n", toolUse)
                        }
                        continue
                    }
                }
                if toolID == "" {
                    if IsDebug {
                        fmt.Printf("Warning: tool call has empty id: %v\n", toolUse)
                    }
                    continue
                }

                if toolUse["type"] != "function" {
                    validToolCalls = append(validToolCalls, toolUse)
                    callsToProcess = append(callsToProcess, callInfo{
                        ID:       toolID,
                        Name:     "",
                        ArgsJSON: "",
                    })
                    continue
                }
                function, ok := toolUse["function"].(map[string]interface{})
                if !ok {
                    validToolCalls = append(validToolCalls, toolUse)
                    callsToProcess = append(callsToProcess, callInfo{
                        ID:       toolID,
                        Name:     "",
                        ArgsJSON: "",
                    })
                    continue
                }
                toolName, _ := function["name"].(string)
                argsStr, _ := function["arguments"].(string)

                validToolCalls = append(validToolCalls, toolUse)
                callsToProcess = append(callsToProcess, callInfo{
                    ID:       toolID,
                    Name:     toolName,
                    ArgsJSON: argsStr,
                })
            }

            messages = messages[:len(messages)-1]
            messages = append(messages, Message{
                Role:      "assistant",
                ToolCalls: validToolCalls,
            })

            for _, call := range callsToProcess {
                select {
                case <-ctx.Done():
                    log.Printf("[AgentLoop] Context cancelled, stopping tool execution")
                    return messages, ctx.Err()
                default:
                }

                if call.Name == "" {
                    results = append(results, NewToolResultMessage(call.ID, "Error: Invalid tool type or function field", TaskStatusFailed, ""))
                    continue
                }

                var argsMap map[string]interface{}
                if err := json.Unmarshal([]byte(call.ArgsJSON), &argsMap); err != nil {
                    if IsDebug {
                        fmt.Printf("Failed to parse arguments: %v\n", err)
                    }
                    results = append(results, NewToolResultMessage(call.ID, "Error: Failed to parse arguments", TaskStatusFailed, call.Name))
                    continue
                }

                if hookManager != nil && hookManager.IsEnabled() {
                    hookResult := hookManager.RunBeforeTool(ctx, 0, "", iteration, call.Name, argsMap)
                    if hookResult.Action == HookOutcomeBlock {
                        results = append(results, NewToolResultMessage(call.ID, hookResult.Reason, TaskStatusFailed, call.Name))
                        continue
                    } else if hookResult.Action == HookOutcomeModify && hookResult.ModifiedInput != nil {
                        argsMap = hookResult.ModifiedInput
                    }
                }

                result := executeTool(ctx, call.ID, call.Name, argsMap, ch, currentRole)

                contentStr, _ := result.Content.(string)
                isErr := result.Meta.Status == TaskStatusFailed
                if loopResult := CheckLoop(call.Name, argsMap, contentStr, isErr); loopResult != nil {
                    // 主动学习：注入历史经验
                    if globalUnifiedMemory != nil {
                        exps := globalUnifiedMemory.RetrieveExperiences(call.Name, 2)
                        if len(exps) > 0 {
                            var expMsg strings.Builder
                            expMsg.WriteString("\n\n## 📚 历史经验参考\n")
                            for _, exp := range exps {
                                expMsg.WriteString(fmt.Sprintf("- %s (评分: %.2f)\n", exp.Summary, exp.Score))
                            }
                            expMsg.WriteString("建议参考上述成功经验，避免重复错误。")
                            loopResult.WarningMessage += expMsg.String()
                        }
                    }
                    contentStr = contentStr + "\n\n" + loopResult.WarningMessage
                    if loopResult.Suggestion != "" {
                        contentStr = contentStr + "\n\n💡 建议：" + loopResult.Suggestion
                    }
                    result.Content = contentStr
                    log.Printf("[AgentLoop] Loop detected: %s (count: %d)", call.Name, loopResult.LoopCount)
                }

                if hookManager != nil && hookManager.IsEnabled() {
                    contentStr, _ := result.Content.(string)
                    toolResultInfo := &ToolResultInfo{
                        Content: contentStr,
                        IsError: result.Meta.Status == TaskStatusFailed,
                    }
                    hookResult := hookManager.RunAfterTool(ctx, 0, "", iteration, call.Name, argsMap, toolResultInfo)
                    if hookResult.Action == HookOutcomeBlock {
                        result = NewToolResultMessage(call.ID, hookResult.Reason, TaskStatusFailed, call.Name)
                    } else if hookResult.Action == HookOutcomeModify {
                        if warning, ok := hookResult.Patch["warning"].(string); ok {
                            contentStr = contentStr + "\n\n" + warning
                            result.Content = contentStr
                        }
                    }
                }

                results = append(results, result)
            }
        } else {
            if contentArray, ok := respContent.([]interface{}); ok {
                for _, item := range contentArray {
                    select {
                    case <-ctx.Done():
                        log.Printf("[AgentLoop] Context cancelled, stopping tool execution (anthropic format)")
                        return messages, ctx.Err()
                    default:
                    }

                    if toolUse, ok := item.(map[string]interface{}); ok && toolUse["type"] == "tool_use" {
                        toolName, nameOk := toolUse["name"].(string)
                        input, inputOk := toolUse["input"].(map[string]interface{})

                        toolID, ok := toolUse["id"].(string)
                        if !ok {
                            if idVal, exists := toolUse["id"]; exists {
                                toolID = fmt.Sprint(idVal)
                            } else {
                                if IsDebug {
                                    fmt.Printf("Warning: tool call missing id: %v\n", toolUse)
                                }
                                continue
                            }
                        }
                        if toolID == "" {
                            if IsDebug {
                                fmt.Printf("Warning: tool call has empty id: %v\n", toolUse)
                            }
                            continue
                        }

                        if !nameOk || !inputOk {
                            results = append(results, NewToolResultMessage(toolID, "Error: Invalid tool use fields", TaskStatusFailed, toolName))
                            continue
                        }

                        if hookManager != nil && hookManager.IsEnabled() {
                            hookResult := hookManager.RunBeforeTool(ctx, 0, "", iteration, toolName, input)
                            if hookResult.Action == HookOutcomeBlock {
                                results = append(results, NewToolResultMessage(toolID, hookResult.Reason, TaskStatusFailed, toolName))
                                continue
                            } else if hookResult.Action == HookOutcomeModify && hookResult.ModifiedInput != nil {
                                input = hookResult.ModifiedInput
                            }
                        }

                        result := executeTool(ctx, toolID, toolName, input, ch, currentRole)

                        contentStr, _ := result.Content.(string)
                        isErr := result.Meta.Status == TaskStatusFailed
                        if loopResult := CheckLoop(toolName, input, contentStr, isErr); loopResult != nil {
                            if globalUnifiedMemory != nil {
                                exps := globalUnifiedMemory.RetrieveExperiences(toolName, 2)
                                if len(exps) > 0 {
                                    var expMsg strings.Builder
                                    expMsg.WriteString("\n\n## 📚 历史经验参考\n")
                                    for _, exp := range exps {
                                        expMsg.WriteString(fmt.Sprintf("- %s (评分: %.2f)\n", exp.Summary, exp.Score))
                                    }
                                    expMsg.WriteString("建议参考上述成功经验，避免重复错误。")
                                    loopResult.WarningMessage += expMsg.String()
                                }
                            }
                            contentStr = contentStr + "\n\n" + loopResult.WarningMessage
                            if loopResult.Suggestion != "" {
                                contentStr = contentStr + "\n\n💡 建议：" + loopResult.Suggestion
                            }
                            result.Content = contentStr
                            log.Printf("[AgentLoop] Loop detected: %s (count: %d)", toolName, loopResult.LoopCount)
                        }

                        if hookManager != nil && hookManager.IsEnabled() {
                            contentStr, _ := result.Content.(string)
                            toolResultInfo := &ToolResultInfo{
                                Content: contentStr,
                                IsError: result.Meta.Status == TaskStatusFailed,
                            }
                            hookResult := hookManager.RunAfterTool(ctx, 0, "", iteration, toolName, input, toolResultInfo)
                            if hookResult.Action == HookOutcomeBlock {
                                result = NewToolResultMessage(toolID, hookResult.Reason, TaskStatusFailed, toolName)
                            } else if hookResult.Action == HookOutcomeModify {
                                if warning, ok := hookResult.Patch["warning"].(string); ok {
                                    contentStr = contentStr + "\n\n" + warning
                                    result.Content = contentStr
                                }
                            }
                        }

                        results = append(results, result)
                    }
                }
            }
        }

        for _, result := range results {
            messages = append(messages, result.ToAPIMessage())

            if globalTaskTracker != nil {
                contentStr, _ := result.Content.(string)
                globalTaskTracker.RecordToolCall(
                    result.Meta.ToolName,
                    result.Meta.Status,
                    "",
                    truncateString(contentStr, 100),
                )
            }
        }

        if globalTaskTracker != nil {
            shouldPrompt, promptMsg := globalTaskTracker.ShouldPromptTodo()
            if shouldPrompt && promptMsg != "" {
                messages = append(messages, Message{
                    Role:    "user",
                    Content: promptMsg,
                })
            }
        }

        if IsDebug {
            fmt.Printf("Number of messages before second call: %d\n", len(messages))
            for i, msg := range messages {
                fmt.Printf("Message %d: Role=%s, Content=%v, ToolCallID=%s\n", i, msg.Role, msg.Content, msg.ToolCallID)
            }
        }
    }

    ch.WriteChunk(StreamChunk{Done: true})

    if globalMemoryConsolidator != nil {
        go func() {
            sessionKey := "default"
            if should, _ := globalMemoryConsolidator.ShouldConsolidate(sessionKey); should {
                log.Println("[MemoryConsolidator] Triggering automatic consolidation...")
                if err := globalMemoryConsolidator.MaybeConsolidate(ctx, sessionKey); err != nil {
                    log.Printf("[MemoryConsolidator] Consolidation failed: %v", err)
                }
            }
        }()
    }

    return messages, nil
}
