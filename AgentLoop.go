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
        AgenticToolCallStart = "<<<AGENTIC_TOOL_CALL_START>>>"
        AgenticToolCallEnd   = "<<<AGENTIC_TOOL_CALL_END>>>"
        AgenticToolNamePrefix = "<<<TOOL_NAME:"
        AgenticToolArgsStart  = "<<<TOOL_ARGS_START>>>"
        AgenticToolArgsEnd    = "<<<TOOL_ARGS_END>>>"
        AgenticTagSuffix      = ">>>"
)

// sanitizeContent 清理内容中的非法控制字符，避免 API 报错 "messages in request are illegal"
func sanitizeContent(content string) string {
        // 创建一个 strings.Builder 来构建清理后的字符串
        var builder strings.Builder
        builder.Grow(len(content))

        for _, r := range content {
                // 允许的字符：
                // - 换行符 \n (0x0A)
                // - 制表符 \t (0x09)
                // - 回车符 \r (0x0D) - 但通常应该转换为 \n
                // - 其他可打印字符 (0x20-0x7E, 0x80+)
                switch r {
                case '\n', '\t':
                        builder.WriteRune(r)
                case '\r':
                        // 跳过回车符，由 \n 处理换行
                        continue
                default:
                        // 跳过其他控制字符 (0x00-0x1F, 除了 \t \n \r)
                        if r < 0x20 {
                                continue
                        }
                        // 跳过 Unicode 控制字符
                        if r == 0x7F {
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

// executeTool 执行单个工具调用，返回增强消息
// role 参数用于执行时权限验证，为 nil 时允许所有工具
func executeTool(ctx context.Context, toolID, toolName string, argsMap map[string]interface{}, ch Channel, role *Role) EnrichedMessage {
        var content string
        status := TaskStatusSuccess // 默认为成功

        // 检查上下文是否已被取消
        if ctx.Err() == context.Canceled {
                return CancelToolResult(toolID, CancelByUser, "User cancelled before execution", toolName)
        }

        // 执行时权限验证：检查角色是否有权使用此工具
        if role != nil && !role.IsToolAllowed(toolName) {
                errMsg := fmt.Sprintf("❌ 权限拒绝：当前角色「%s」无权使用工具「%s」。\n\n可用工具：%v", 
                        role.DisplayName, toolName, getAllowedToolsList(role))
                // 发送工具调用格式
                argsJSON, _ := json.Marshal(map[string]interface{}{"error": "permission denied"})
                sendToolCallStart(ch, toolName, string(argsJSON))
                ch.WriteChunk(StreamChunk{Content: errMsg + "\n"})
                sendToolCallEnd(ch)
                return NewToolResultMessage(toolID, errMsg, TaskStatusFailed, toolName)
        }

        // 将参数转换为 JSON
        argsJSON, _ := json.Marshal(argsMap)

        // 发送工具调用开始标记
        sendToolCallStart(ch, toolName, string(argsJSON))

        // 确保在函数结束时发送结束标记
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
                        // 获取 force 参数
                        force := false
                        if forceVal, ok := argsMap["force"].(bool); ok {
                                force = forceVal
                        }

                        // 获取 is_blocking_confirmed 参数（用于确认后执行）
                        isBlockingConfirmed := false
                        if confirmedVal, ok := argsMap["is_blocking_confirmed"].(bool); ok {
                                isBlockingConfirmed = confirmedVal
                        }

                        result := runShellWithTimeout(ctx, command, force, isBlockingConfirmed)

                        // 处理确认请求
                        if result.ConfirmRequired {
                                // 返回确认请求给模型
                                var confirmResult strings.Builder
                                confirmResult.WriteString("⚠️ **确认请求**\n\n")
                                confirmResult.WriteString(result.ConfirmMessage)
                                confirmResult.WriteString("\n\n---\n")
                                confirmResult.WriteString("要强制执行此命令，请使用: `shell(command=\"...\", force=true)`\n")
                                confirmResult.WriteString("或使用建议的替代命令。")

                                content = confirmResult.String()
                                status = TaskStatusSuccess // 不是失败，是等待确认
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
                                // 可选超时参数
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
                                        Visible: true, // 默认等待可见
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
                                // 使用 TOON 格式输出结果
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

        // ========== 文本处理工具 ==========
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

        // ========== 后台任务管理工具 ==========
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
        // ========== 子代理工具 ==========
        case "spawn":
                content, _ = handleSpawn(ctx, argsMap, ch)
        case "spawn_check":
                content, _ = handleSpawnCheck(ctx, argsMap, ch)
        case "spawn_list":
                content, _ = handleSpawnList(ctx, argsMap, ch)
        case "spawn_cancel":
                content, _ = handleSpawnCancel(ctx, argsMap, ch)
        // ========== 记忆整合工具 ==========
        case "save_memory":
                content, _ = HandleSaveMemoryTool(argsMap)
        default:
                // 检查是否是 MCP 工具 (mcp_{server}_{tool} 格式)
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

        // 如果状态仍为 success 但内容包含错误，则更新状态
        if status == TaskStatusSuccess && (strings.HasPrefix(content, "Error:") || strings.HasPrefix(content, "error:")) {
                status = TaskStatusFailed
        }

        // 清理内容中的非法控制字符，避免 API 报错
        content = sanitizeContent(content)

        // 发送工具结果内容（在结束标记之前）
        if content != "" {
                ch.WriteChunk(StreamChunk{Content: content + "\n"})
        }

        return NewToolResultMessage(toolID, content, status, toolName)
}

// getAllowedToolsList 获取角色允许使用的工具列表（用于错误提示）
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

// AgentLoop 核心循环，检查写入错误并退出
func AgentLoop(ctx context.Context, ch Channel, messages []Message, apiType, baseURL, apiKey, modelID string,
        temperature float64, maxTokens int, stream bool, thinking bool) ([]Message, error) {

        // 获取当前角色的 Role（用于工具权限过滤）和模型配置
        var currentRole *Role
        var effectiveAPIType, effectiveBaseURL, effectiveAPIKey, effectiveModelID string
        var effectiveTemperature float64
        var effectiveMaxTokens int

        // 默认使用全局配置
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

                        // 获取演员关联的模型配置
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

                // 检查是否需要更新系统提示词（角色切换后）
                needUpdate := false
                if globalStage != nil {
                        needUpdate = globalStage.NeedUpdateSystemPrompt()
                }

                if !hasSystemPrompt || needUpdate {
                        var systemPrompt string

                        // 根据当前演员构建系统提示
                        if globalRoleManager != nil && globalActorManager != nil && globalStage != nil {
                                currentActor := globalStage.GetCurrentActor()
                                systemPrompt = BuildSystemPromptForActor(currentActor, globalActorManager, globalRoleManager, globalStage)

                                // 添加记忆上下文
                                if globalMemoryManager != nil {
                                        memoryContext := globalMemoryManager.GetContextForPrompt()
                                        if memoryContext != "" {
                                                systemPrompt += "\n\n" + memoryContext
                                        }
                                }
                        } else {
                                systemPrompt = SYSTEM_PROMPT
                        }

                        if systemPrompt != "" {
                                if needUpdate && systemPromptIndex >= 0 {
                                        // 更新现有的 system 消息
                                        messages[systemPromptIndex] = Message{Role: "system", Content: systemPrompt}
                                        // 清除更新标记
                                        globalStage.ClearUpdateSystemPrompt()
                                } else {
                                        // 添加新的 system 消息
                                        messages = append([]Message{{Role: "system", Content: systemPrompt}}, messages...)
                                }
                        }
                }
        }

        // 初始化 Hook 管理器
        hookManager := GetHookManager()
        
        iteration := 0

        // 记录用户消息到记忆整合器（如果有的话）
        if globalMemoryConsolidator != nil && len(messages) > 0 {
                // 找到最后一条用户消息
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

                // 执行 BeforeModelCall Hook
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
                // 用于合并增量 tool_calls
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

                        // 发送给频道，如果失败则退出
                        // 注意：字符串替换已在 Channel 层统一处理（流式最长匹配）
                        // 重要：不在此处发送 Done: true，因为如果是工具调用，还需要执行工具
                        // Done 标志会在 AgentLoop 结束时统一发送
                        chunkToSend := chunk
                        chunkToSend.Done = false
                        if writeErr := ch.WriteChunk(chunkToSend); writeErr != nil {
                                log.Printf("WebSocket write failed: %v, stopping AgentLoop", writeErr)
                                return messages, writeErr
                        }

                        // 收集完整内容
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
                        // 合并增量 tool_calls（按 index 合并）
                        if len(chunk.ToolCalls) > 0 {
                                for _, tc := range chunk.ToolCalls {
                                        // 获取 index
                                        idx := 0
                                        if idxFloat, ok := tc["index"].(float64); ok {
                                                idx = int(idxFloat)
                                        } else if idxInt, ok := tc["index"].(int); ok {
                                                idx = idxInt
                                        }

                                        // 获取或创建该 index 的 tool call
                                        existing, exists := toolCallsMap[idx]
                                        if !exists {
                                                existing = make(map[string]interface{})
                                                toolCallsMap[idx] = existing
                                        }

                                        // 合并字段
                                        for k, v := range tc {
                                                if k == "function" {
                                                        // 特殊处理 function 字段
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
                                                        // 合并 function 字段
                                                        for fk, fv := range funcMap {
                                                                if fk == "arguments" {
                                                                        // arguments 需要拼接
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
                                                        // 对于非 function 字段，只在新值非空且非空字符串时才更新，避免用空值覆盖已有值
                                                        if v != nil {
                                                                if str, ok := v.(string); ok && str == "" {
                                                                        continue // 跳过空字符串
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

                // 将合并后的 tool_calls 转换为数组（按 index 排序），并移除 index 字段（仅用于流式合并）
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
                                        // 移除 index 字段，因为它只用于流式传输，不应出现在最终消息中
                                        delete(tc, "index")
                                        toolCalls = append(toolCalls, tc)
                                }
                        }
                }

                // 构建 assistant 消息
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

                // 记录助手消息到记忆整合器
                if globalMemoryConsolidator != nil {
                        contentStr, _ := respContent.(string)
                        globalMemoryConsolidator.AddMessage("default", ConsolidationMessage{
                                Role:      "assistant",
                                Content:   contentStr,
                                Timestamp: time.Now(),
                        })
                }

                // 检查是否需要结束
                if stopReason != "tool_use" && stopReason != "function_call" && stopReason != "tool_calls" {
                        // 检查是否有自动切换标记
                        if globalStage != nil && globalStage.AutoSwitchEnabled() {
                                contentStr, _ := respContent.(string)
                                hasMarker, targetActor, isEnd := ParseSwitchMarker(contentStr)

                                if hasMarker && !isEnd && targetActor != "" && globalStage.CanAutoSwitch() {
                                        // 验证目标演员存在
                                        if _, ok := globalActorManager.GetActor(targetActor); ok {
                                                // 移除切换标记
                                                cleanedContent := StripSwitchMarker(contentStr)

                                                // 更新最后一条消息的内容
                                                messages[len(messages)-1] = Message{
                                                        Role:             "assistant",
                                                        Content:          cleanedContent,
                                                        ReasoningContent: reasoningContent,
                                                }

                                                // 切换演员
                                                globalStage.SetCurrentActor(targetActor)
                                                turns := globalStage.IncrementAutoTurns()

                                                // 输出切换提示
                                                switchMsg := fmt.Sprintf("\n═══════════════════════════════════════════════════════════════\n[Auto Switch → %s | Turns: %d/%d]\n═══════════════════════════════════════════════════════════════\n", targetActor, turns, 20)
                                                ch.WriteChunk(StreamChunk{Content: switchMsg})

                                                // 重新构建系统提示并继续循环
                                                // 移除旧的系统消息
                                                newMessages := make([]Message, 0)
                                                for _, msg := range messages {
                                                        if msg.Role != "system" {
                                                                newMessages = append(newMessages, msg)
                                                        }
                                                }

                                                // 构建新的系统提示
                                                newSystemPrompt := BuildSystemPromptForActor(targetActor, globalActorManager, globalRoleManager, globalStage)
                                                if globalMemoryManager != nil {
                                                        memoryContext := globalMemoryManager.GetContextForPrompt()
                                                        if memoryContext != "" {
                                                                newSystemPrompt += "\n\n" + memoryContext
                                                        }
                                                }
                                                newMessages = append([]Message{{Role: "system", Content: newSystemPrompt}}, newMessages...)
                                                messages = newMessages

                                                continue // 继续循环，使用新演员
                                        }
                                } else if isEnd {
                                        // 场景结束，输出提示
                                        ch.WriteChunk(StreamChunk{Content: "\n═══════════════════════════════════════════════════════════════\n[Auto Stopped: END marker]\n═══════════════════════════════════════════════════════════════\n"})
                                        // 移除结束标记
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

                // 执行工具调用
                var results []EnrichedMessage

                if IsDebug {
                        fmt.Println("===================== Executing tool calls =====================")
                        fmt.Printf("API type: %s\n", apiType)
                        fmt.Printf("Response content type: %T\n", respContent)
                        fmt.Printf("Response content: %v\n", respContent)
                }

                if apiType == "openai" {
                        // 使用合并后的 toolCalls 而不是 respContent
                        var toolCallsSlice []interface{}
                        // toolCalls 是 []map[string]interface{} 类型
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

                        // 替换最后一条 assistant 消息为包含有效 tool_calls 的消息
                        messages = messages[:len(messages)-1]
                        messages = append(messages, Message{
                                Role:      "assistant",
                                ToolCalls: validToolCalls,
                        })

                        for _, call := range callsToProcess {
                                // 检查 context 是否已被取消
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

                                // 执行 BeforeToolCall Hook
                                if hookManager != nil && hookManager.IsEnabled() {
                                        hookResult := hookManager.RunBeforeTool(ctx, 0, "", iteration, call.Name, argsMap)
                                        if err != nil {
                                        } else if hookResult.Action == HookOutcomeBlock {
                                                results = append(results, NewToolResultMessage(call.ID, hookResult.Reason, TaskStatusFailed, call.Name))
                                                continue
                                        } else if hookResult.Action == HookOutcomeModify && hookResult.ModifiedInput != nil {
                                                // 使用修改后的输入
                                                argsMap = hookResult.ModifiedInput
                                        }
                                }

                                result := executeTool(ctx, call.ID, call.Name, argsMap, ch, currentRole)

                                // 循环检测
                                contentStr, _ := result.Content.(string)
                                isErr := result.Meta.Status == TaskStatusFailed
                                if loopResult := CheckLoop(call.Name, argsMap, contentStr, isErr); loopResult != nil {
                                        // 检测到循环，附加警告信息
                                        contentStr = contentStr + "\n\n" + loopResult.WarningMessage
                                        if loopResult.Suggestion != "" {
                                                contentStr = contentStr + "\n\n💡 建议：" + loopResult.Suggestion
                                        }
                                        result.Content = contentStr
                                        log.Printf("[AgentLoop] Loop detected: %s (count: %d)", call.Name, loopResult.LoopCount)
                                }

                                // 执行 AfterToolCall Hook
                                if hookManager != nil && hookManager.IsEnabled() {
                                        contentStr, _ := result.Content.(string)
                                        toolResultInfo := &ToolResultInfo{
                                                Content:    contentStr,
                                                IsError:    result.Meta.Status == TaskStatusFailed,
                                        }
                                        hookResult := hookManager.RunAfterTool(ctx, 0, "", iteration, call.Name, argsMap, toolResultInfo)
                                        if hookResult.Action == HookOutcomeBlock {
                                                result = NewToolResultMessage(call.ID, hookResult.Reason, TaskStatusFailed, call.Name)
                                        } else if hookResult.Action == HookOutcomeModify {
                                                // 附加警告信息到结果中
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
                                        // 检查 context 是否已被取消
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

                                                // 执行 BeforeToolCall Hook
                                                if hookManager != nil && hookManager.IsEnabled() {
                                                        hookResult := hookManager.RunBeforeTool(ctx, 0, "", iteration, toolName, input)
                                                        if err != nil {
                                                        } else if hookResult.Action == HookOutcomeBlock {
                                                                results = append(results, NewToolResultMessage(toolID, hookResult.Reason, TaskStatusFailed, toolName))
                                                                continue
                                                        } else if hookResult.Action == HookOutcomeModify && hookResult.ModifiedInput != nil {
                                                                // 使用修改后的输入
                                                                input = hookResult.ModifiedInput
                                                        }
                                                }

                                                result := executeTool(ctx, toolID, toolName, input, ch, currentRole)

                                                // 循环检测
                                                contentStrLoop, _ := result.Content.(string)
                                                isErrLoop := result.Meta.Status == TaskStatusFailed
                                                if loopResult := CheckLoop(toolName, input, contentStrLoop, isErrLoop); loopResult != nil {
                                                        // 检测到循环，附加警告信息
                                                        contentStrLoop = contentStrLoop + "\n\n" + loopResult.WarningMessage
                                                        if loopResult.Suggestion != "" {
                                                                contentStrLoop = contentStrLoop + "\n\n💡 建议：" + loopResult.Suggestion
                                                        }
                                                        result.Content = contentStrLoop
                                                        log.Printf("[AgentLoop] Loop detected: %s (count: %d)", toolName, loopResult.LoopCount)
                                                }

                                                // 执行 AfterToolCall Hook
                                                if hookManager != nil && hookManager.IsEnabled() {
                                                        contentStr, _ := result.Content.(string)
                                                        toolResultInfo := &ToolResultInfo{
                                                                Content:    contentStr,
                                                                IsError:    result.Meta.Status == TaskStatusFailed,
                                                        }
                                                        hookResult := hookManager.RunAfterTool(ctx, 0, "", iteration, toolName, input, toolResultInfo)
                                                        if hookResult.Action == HookOutcomeBlock {
                                                                result = NewToolResultMessage(toolID, hookResult.Reason, TaskStatusFailed, toolName)
                                                        } else if hookResult.Action == HookOutcomeModify {
                                                                // 附加警告信息到结果中
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
                        // 使用 ToAPIMessage 转换，自动添加状态标记
                        messages = append(messages, result.ToAPIMessage())

                        // 记录工具调用到任务追踪器
                        if globalTaskTracker != nil {
                                contentStr, _ := result.Content.(string)
                                globalTaskTracker.RecordToolCall(
                                        result.Meta.ToolName,
                                        result.Meta.Status,
                                        "", // input summary
                                        truncateString(contentStr, 100),
                                )
                        }
                }

                // 智能进度检查（替代旧的 roundsSinceTodo 计数器）
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

        // 发送 Done 标志，表示整个响应结束
        ch.WriteChunk(StreamChunk{Done: true})

        // 尝试记忆整合（异步执行，不阻塞主流程）
        if globalMemoryConsolidator != nil {
                go func() {
                        // 使用固定的 sessionKey（简化实现）
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
