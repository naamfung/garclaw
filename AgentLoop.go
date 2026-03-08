package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/toon-format/toon-go"
)

// 全局记忆存储实例
var globalMemoryStore *MemoryStore

// 初始化全局记忆存储
func init() {
	globalMemoryStore = NewMemoryStore(workspaceDir)
}

// evaluateExpression 解析与计算数学表达式
func evaluateExpression(expr string) (float64, error) {
	// 移除表达式中的空格
	expr = strings.ReplaceAll(expr, " ", "")

	// 定义解析器状态
	type parser struct {
		expr string
		pos  int
	}

	// 声明内部函数
	var parseExpression func(*parser) (float64, error)
	var parseTerm func(*parser) (float64, error)
	var parseFactor func(*parser) (float64, error)

	// 解析表达式（处理加减运算）
	parseExpression = func(p *parser) (float64, error) {
		result, err := parseTerm(p)
		if err != nil {
			return 0, err
		}

		for p.pos < len(p.expr) {
			op := p.expr[p.pos]
			if op != '+' && op != '-' {
				break
			}
			p.pos++

			right, err := parseTerm(p)
			if err != nil {
				return 0, err
			}

			if op == '+' {
				result += right
			} else {
				result -= right
			}
		}

		return result, nil
	}

	// 解析项（处理乘除运算）
	parseTerm = func(p *parser) (float64, error) {
		result, err := parseFactor(p)
		if err != nil {
			return 0, err
		}

		for p.pos < len(p.expr) {
			op := p.expr[p.pos]
			if op != '*' && op != '/' {
				break
			}
			p.pos++

			right, err := parseFactor(p)
			if err != nil {
				return 0, err
			}

			if op == '*' {
				result *= right
			} else {
				if right == 0 {
					return 0, errors.New("division by zero")
				}
				result /= right
			}
		}

		return result, nil
	}

	// 解析因子（处理数字与括号）
	parseFactor = func(p *parser) (float64, error) {
		if p.pos >= len(p.expr) {
			return 0, errors.New("invalid expression: unexpected end")
		}

		// 处理括号
		if p.expr[p.pos] == '(' {
			p.pos++
			result, err := parseExpression(p)
			if err != nil {
				return 0, err
			}
			if p.pos >= len(p.expr) || p.expr[p.pos] != ')' {
				return 0, errors.New("invalid expression: missing closing parenthesis")
			}
			p.pos++
			return result, nil
		}

		// 处理数字
		start := p.pos
		if p.expr[p.pos] == '+' || p.expr[p.pos] == '-' {
			p.pos++
		}

		for p.pos < len(p.expr) {
			c := p.expr[p.pos]
			if (c < '0' || c > '9') && c != '.' {
				break
			}
			p.pos++
		}

		if p.pos == start {
			return 0, errors.New("invalid expression: expected number or parenthesis")
		}

		numStr := p.expr[start:p.pos]
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, errors.New("invalid expression: invalid number")
		}

		return num, nil
	}

	p := &parser{expr: expr}

	// 解析表达式
	result, err := parseExpression(p)
	if err != nil {
		return 0, err
	}

	// 确保解析完整个表达式
	if p.pos < len(p.expr) {
		return 0, errors.New("invalid expression: unexpected characters at end")
	}

	return result, nil
}

// executeTool 执行单个工具调用，返回 ToolResult 与是否使用了 todo
func executeTool(toolID, toolName string, argsMap map[string]interface{}) (ToolResult, bool) {
	usedTodo := false
	var content string

	switch toolName {
	case "shell":
		command, _ := argsMap["command"].(string)
		if command == "" {
			content = "Error: Empty command"
		} else {
			fmt.Printf("$ %s\n", command)
			result := runShell(command)
			if result.Err != nil {
				content = fmt.Sprintf("Error: %v", result.Err)
			} else {
				content = result.Stdout
				if result.ExitCode != 0 && result.Stderr != "" {
					content += "\n" + result.Stderr
				}
			}
			if len(content) > 512 && isDebug {
				fmt.Println(TruncateString(content, 512))
			} else {
				fmt.Println(content)
			}
		}

	case "read_file_line":
		filename, _ := argsMap["filename"].(string)
		lineNumFloat, _ := argsMap["line_num"].(float64)
		lineNum := int(lineNumFloat)
		if filename == "" || lineNum < 1 {
			content = "Error: Invalid arguments for read_file_line"
		} else {
			fmt.Printf("Reading line %d from %s\n", lineNum, filename)
			c, err := ReadFileLine(filename, lineNum)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = c
			}
			fmt.Println(TruncateString(content, 200))
		}

	case "write_file_line":
		filename, _ := argsMap["filename"].(string)
		lineNumFloat, _ := argsMap["line_num"].(float64)
		lineNum := int(lineNumFloat)
		text, _ := argsMap["content"].(string)
		if filename == "" || lineNum < 1 {
			content = "Error: Invalid arguments for write_file_line"
		} else {
			fmt.Printf("Writing to line %d in %s\n", lineNum, filename)
			err := WriteFileLine(filename, lineNum, text)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = "Successfully wrote to line " + strconv.Itoa(lineNum)
			}
			fmt.Println(content)
		}

	case "read_all_lines":
		filename, _ := argsMap["filename"].(string)
		if filename == "" {
			content = "Error: Invalid arguments for read_all_lines"
		} else {
			fmt.Printf("Reading all lines from %s\n", filename)
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
		filename, _ := argsMap["filename"].(string)
		linesInterface, _ := argsMap["lines"].([]interface{})
		if filename == "" || linesInterface == nil {
			content = "Error: Invalid arguments for write_all_lines"
		} else {
			lines := make([]string, len(linesInterface))
			for i, line := range linesInterface {
				if lineStr, ok := line.(string); ok {
					lines[i] = lineStr
				}
			}
			fmt.Printf("Writing all lines to %s\n", filename)
			err := WriteAllLines(filename, lines)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = "Successfully wrote " + strconv.Itoa(len(lines)) + " lines to " + filename
			}
			fmt.Println(content)
		}

	case "search":
		keyword, _ := argsMap["keyword"].(string)
		if keyword == "" {
			content = "Error: Empty keyword in search tool call"
		} else {
			fmt.Printf("Searching for: %s\n", keyword)
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
			fmt.Println("Search completed")
		}

	case "visit":
		url, _ := argsMap["url"].(string)
		if url == "" {
			content = "Error: Empty url in visit tool call"
		} else {
			fmt.Printf("Visiting: %s\n", url)
			pageText, err := Visit(url)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = "Visit completed. Page content: " + pageText
			}
			fmt.Println("Visit completed")
		}

	case "download":
		url, _ := argsMap["url"].(string)
		if url == "" {
			content = "Error: Empty url in download tool call"
		} else {
			fmt.Printf("Downloading from: %s\n", url)
			fileName, err := Download(url)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = "Download completed, saved to: " + fileName
			}
			fmt.Println(content)
		}

	case "todo":
		itemsInterface, _ := argsMap["items"].([]interface{})
		if itemsInterface == nil {
			content = "Error: Invalid items in todo tool call"
		} else {
			var items []TodoItem
			for _, itemInterface := range itemsInterface {
				if itemMap, ok := itemInterface.(map[string]interface{}); ok {
					item := TodoItem{}
					if id, ok := itemMap["id"].(string); ok {
						item.ID = id
					}
					if text, ok := itemMap["text"].(string); ok {
						item.Text = text
					}
					if status, ok := itemMap["status"].(string); ok {
						item.Status = status
					}
					items = append(items, item)
				}
			}
			fmt.Println("Updating todo list...")
			output, err := TODO.Update(items)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = output
			}
			fmt.Println(content)
			usedTodo = true
		}

	case "memory_write":
		contentStr, _ := argsMap["content"].(string)
		if contentStr == "" {
			content = "Error: Empty content in memory_write tool call"
		} else {
			// 使用全局记忆存储
			category := "general"
			if cat, ok := argsMap["category"].(string); ok && cat != "" {
				category = cat
			}
			content = globalMemoryStore.WriteDailyMemory(contentStr, category)
			fmt.Println(content)
		}

	case "memory_search":
		query, _ := argsMap["query"].(string)
		if query == "" {
			content = "Error: Empty query in memory_search tool call"
		} else {
			// 使用全局记忆存储
			content = globalMemoryStore.SearchMemory(query)
			fmt.Println("Memory search completed")
		}

	case "calculate":
		expression, _ := argsMap["expression"].(string)

		if expression == "" {
			content = "Error: Empty expression"
		} else {
			result, err := evaluateExpression(expression)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = fmt.Sprintf("%.6f", result)
			}

			fmt.Printf("Calculated: %s = %s\n", expression, content)
		}

	case "mail":
		to, _ := argsMap["to"].(string)
		subject, _ := argsMap["subject"].(string)
		message, _ := argsMap["message"].(string)

		if to == "" || subject == "" || message == "" {
			content = "Error: Missing required parameters for mail tool"
		} else {
			// 调用 MailChannel.Send() 发送邮件
			if globalChannelManager != nil {
				mailChannel := globalChannelManager.Get("mail")
				if mailChannel != nil {
					success := mailChannel.Send(to, message, map[string]interface{}{"subject": subject})
					if success {
						content = "Mail sent to " + to + " successfully"
						fmt.Println(content)
					} else {
						content = "Error: Failed to send mail"
					}
				} else {
					content = "Error: Mail channel not found"
				}
			} else {
				content = "Error: Channel manager not initialized"
			}
		}

	default:
		content = "Error: Unknown tool name"
	}

	return ToolResult{
		Type:      "tool_result",
		ToolUseID: toolID,
		Content:   content,
	}, usedTodo
}

// 核心agent循环
func AgentLoop(messages []Message, apiType, baseURL, apiKey, modelID string, temperature float64, maxTokens int, stream bool, thinking bool) []Message {
	roundsSinceTodo := 0
	for {
		resp, err := CallModel(messages, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
		if err != nil {
			fmt.Printf("Error calling model: %v\n", err)
			return messages
		}

		// 打印响应信息，用于调试
		if isDebug {
			fmt.Println("==================================================================")
			fmt.Printf("Response stop reason: %s\n", resp.StopReason)
			fmt.Printf("Response content type: %T\n", resp.Content)
			fmt.Printf("Response content: %v\n", resp.Content)
			fmt.Println("==================================================================")
		}

		// 添加assistant的回复
		if resp.StopReason == "tool_use" || resp.StopReason == "function_call" || resp.StopReason == "tool_calls" {
			messages = append(messages, Message{
				Role:      "assistant",
				ToolCalls: resp.Content,
			})
		} else {
			messages = append(messages, Message{
				Role:             "assistant",
				Content:          resp.Content,
				ReasoningContent: resp.ReasoningContent,
			})
		}

		// 如果模型未有调用工具，结束
		if resp.StopReason != "tool_use" && resp.StopReason != "function_call" && resp.StopReason != "tool_calls" {
			// [!code ++] 处理空响应情况，避免静默退出
			if resp.Content == nil || fmt.Sprint(resp.Content) == "" {
				fmt.Println("模型终止响应..")
			}
			return messages
		}

		// 执行工具调用
		var results []ToolResult
		usedTodo := false

		if isDebug {
			fmt.Println("===================== Executing tool calls =====================")
			fmt.Printf("API type: %s\n", apiType)
			fmt.Printf("Response content type: %T\n", resp.Content)
			fmt.Printf("Response content: %v\n", resp.Content)
		}

		// 处理不同API的工具调用格式
		if apiType == "openai" {
			// 确保 resp.Content 是切片
			var toolCallsSlice []interface{}
			switch v := resp.Content.(type) {
			case []interface{}:
				toolCallsSlice = v
			case []map[string]interface{}:
				toolCallsSlice = make([]interface{}, len(v))
				for i, m := range v {
					toolCallsSlice[i] = m
				}
			default:
				if isDebug {
					fmt.Printf("Warning: resp.Content is not a slice of tool calls: %T\n", resp.Content)
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
					if isDebug {
						fmt.Printf("Warning: invalid tool call item: %v\n", item)
					}
					continue
				}

				// 提取 toolID
				toolID, ok := toolUse["id"].(string)
				if !ok {
					if idVal, exists := toolUse["id"]; exists {
						toolID = fmt.Sprint(idVal)
					} else {
						if isDebug {
							fmt.Printf("Warning: tool call missing id: %v\n", toolUse)
						}
						continue
					}
				}
				if toolID == "" {
					if isDebug {
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

			// 重新构建助手消息，使用有效的工具调用列表
			messages = messages[:len(messages)-1]
			messages = append(messages, Message{
				Role:      "assistant",
				ToolCalls: validToolCalls,
			})

			// 处理有效的工具调用
			for _, call := range callsToProcess {
				if call.Name == "" {
					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: call.ID,
						Content:   "Error: Invalid tool type or function field",
					})
					continue
				}

				var argsMap map[string]interface{}
				if err := json.Unmarshal([]byte(call.ArgsJSON), &argsMap); err != nil {
					if isDebug {
						fmt.Printf("Failed to parse arguments: %v\n", err)
					}
					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: call.ID,
						Content:   "Error: Failed to parse arguments",
					})
					continue
				}

				result, todoUsed := executeTool(call.ID, call.Name, argsMap)
				results = append(results, result)
				if todoUsed {
					usedTodo = true
				}
			}
		} else {
			// Anthropic与Ollama的工具调用格式
			if contentArray, ok := resp.Content.([]interface{}); ok {
				for _, item := range contentArray {
					if toolUse, ok := item.(map[string]interface{}); ok && toolUse["type"] == "tool_use" {
						toolName, nameOk := toolUse["name"].(string)
						input, inputOk := toolUse["input"].(map[string]interface{})

						toolID, ok := toolUse["id"].(string)
						if !ok {
							if idVal, exists := toolUse["id"]; exists {
								toolID = fmt.Sprint(idVal)
							} else {
								if isDebug {
									fmt.Printf("Warning: tool call missing id: %v\n", toolUse)
								}
								continue
							}
						}
						if toolID == "" {
							if isDebug {
								fmt.Printf("Warning: tool call has empty id: %v\n", toolUse)
							}
							continue
						}

						if !nameOk || !inputOk {
							results = append(results, ToolResult{
								Type:      "tool_result",
								ToolUseID: toolID,
								Content:   "Error: Invalid tool use fields",
							})
							continue
						}

						result, todoUsed := executeTool(toolID, toolName, input)
						results = append(results, result)
						if todoUsed {
							usedTodo = true
						}
					}
				}
			}
		}

		// 添加工具执行结果到消息历史
		for _, result := range results {
			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: result.ToolUseID,
				Content:    result.Content,
			})
		}

		// 更新todo使用计数并添加提醒（放在tool消息之后）
		if usedTodo {
			roundsSinceTodo = 0
		} else {
			roundsSinceTodo++
			if roundsSinceTodo >= 3 {
				// [!code ++] 将提醒内容改为明确的请求，引导模型继续生成
				messages = append(messages, Message{
					Role:    "user",
					Content: "<reminder>Please update your todo list and then proceed with the task.</reminder>", // 原为 "<reminder>Update your todos.</reminder>"
				})
				roundsSinceTodo = 0
			}
		}

		if isDebug {
			fmt.Printf("Number of messages before second call: %d\n", len(messages))
			for i, msg := range messages {
				fmt.Printf("Message %d: Role=%s, Content=%v, ToolCallID=%s\n", i, msg.Role, msg.Content, msg.ToolCallID)
			}
		}
	}
}
