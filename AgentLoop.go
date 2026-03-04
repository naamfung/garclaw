package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/toon-format/toon-go"
)

// 核心agent循环
func AgentLoop(messages []Message, apiType, baseURL, apiKey, modelID string, temperature float64, maxTokens int, stream bool, thinking bool) {
	roundsSinceTodo := 0
	for {
		resp, err := CallModel(messages, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
		if err != nil {
			fmt.Printf("Error calling model: %v\n", err)
			return
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
			// 对于工具调用，使用ToolCalls字段
			messages = append(messages, Message{
				Role:      "assistant",
				ToolCalls: resp.Content,
			})
		} else {
			// 对于普通回复，使用Content字段
			messages = append(messages, Message{
				Role:             "assistant",
				Content:          resp.Content,
				ReasoningContent: resp.ReasoningContent,
			})
		}

		// 如果模型未有调用工具，结束
		// 注意：需要包含 "tool_calls" 原因
		if resp.StopReason != "tool_use" && resp.StopReason != "function_call" && resp.StopReason != "tool_calls" {
			return
		}

		// 执行工具调用
		var results []ToolResult
		usedTodo := false

		// 打印调试信息
		if isDebug {
			fmt.Println("===================== Executing tool calls =====================")
			fmt.Printf("API type: %s\n", apiType)
			fmt.Printf("Response content type: %T\n", resp.Content)
			fmt.Printf("Response content: %v\n", resp.Content)
		}

		// 处理不同API的工具调用格式
		if apiType == "openai" {
			// DeepSeek与OpenAI的工具调用格式
			// 尝试将 resp.Content 转换为 []map[string]interface{}
			var toolCalls []map[string]interface{}

			// 兼容两种可能的类型：[]interface{} 与 []map[string]interface{}
			if contentArray, ok := resp.Content.([]interface{}); ok {
				for _, item := range contentArray {
					if toolUse, ok := item.(map[string]interface{}); ok {
						toolCalls = append(toolCalls, toolUse)
					}
				}
			} else if contentMapSlice, ok := resp.Content.([]map[string]interface{}); ok {
				toolCalls = contentMapSlice
			} else {
				if isDebug {
					fmt.Printf("Warning: resp.Content is not a slice of maps: %T\n", resp.Content)
				}
			}

			for _, toolUse := range toolCalls {
				// 标准OpenAI格式：type="function", id, function
				if toolUse["type"] != "function" {
					// 即使类型不正确，也要添加一个错误结果
					toolID, _ := toolUse["id"].(string)
					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   "Error: Invalid tool type",
					})
					continue
				}
				toolID, _ := toolUse["id"].(string)
				function, ok := toolUse["function"].(map[string]interface{})
				if !ok {
					// 即使function字段不存在，也要添加一个错误结果
					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   "Error: Invalid function field",
					})
					continue
				}
				toolName, _ := function["name"].(string)
				argsStr, _ := function["arguments"].(string)

				// 解析arguments JSON
				var argsMap map[string]interface{}
				if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
					fmt.Printf("Failed to parse arguments: %v\n", err)
					// 即使解析失败，也要添加一个错误结果
					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   "Error: Failed to parse arguments",
					})
					continue
				}

				switch toolName {
				case "shell":
					command, _ := argsMap["command"].(string)
					if command == "" {
						fmt.Printf("Warning: empty command in tool call\n")
						// 即使命令为空，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Empty command",
						})
						continue
					}

					fmt.Printf("$ %s\n", command)
					result := runShell(command)
					var output string
					if result.Err != nil {
						// 真正无法执行的错误（如命令不存在、危险拦截）
						output = fmt.Sprintf("Error: %v", result.Err)
					} else {
						// 命令已执行，根据退出码判断
						output = result.Stdout
						if result.ExitCode != 0 {
							// 可附加 stderr 信息
							if result.Stderr != "" {
								output += "\n" + result.Stderr
							}
						}
					}

					// 打印命令输出（截断）
					if len(output) > 512 && isDebug {
						fmt.Println(TruncateString(output, 512))
					} else {
						fmt.Println(output)
					}

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "read_file_line":
					filename, _ := argsMap["filename"].(string)
					lineNumFloat, _ := argsMap["line_num"].(float64)
					lineNum := int(lineNumFloat)

					if filename == "" || lineNum < 1 {
						fmt.Printf("Warning: invalid arguments for read_file_line\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Invalid arguments for read_file_line",
						})
						continue
					}

					fmt.Printf("Reading line %d from %s\n", lineNum, filename)
					content, err := ReadFileLine(filename, lineNum)
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else {
						output = content
					}

					// 打印输出（截断）
					fmt.Println(TruncateString(output, 200))

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "write_file_line":
					filename, _ := argsMap["filename"].(string)
					lineNumFloat, _ := argsMap["line_num"].(float64)
					lineNum := int(lineNumFloat)
					content, _ := argsMap["content"].(string)

					if filename == "" || lineNum < 1 {
						fmt.Printf("Warning: invalid arguments for write_file_line\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Invalid arguments for write_file_line",
						})
						continue
					}

					fmt.Printf("Writing to line %d in %s\n", lineNum, filename)
					err := WriteFileLine(filename, lineNum, content)
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else {
						output = "Successfully wrote to line " + strconv.Itoa(lineNum)
					}

					// 打印输出
					fmt.Println(output)

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "read_all_lines":
					filename, _ := argsMap["filename"].(string)

					if filename == "" {
						fmt.Printf("Warning: invalid arguments for read_all_lines\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Invalid arguments for read_all_lines",
						})
						continue
					}

					fmt.Printf("Reading all lines from %s\n", filename)
					lines, err := ReadAllLines(filename)
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else {
						// 将字符串切片转换为JSON字符串
						linesJSON, err := json.Marshal(lines)
						if err != nil {
							output = "Error: " + err.Error()
						} else {
							output = string(linesJSON)
						}
					}

					// 打印输出（截断）
					fmt.Println(TruncateString(output, 200))

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "write_all_lines":
					filename, _ := argsMap["filename"].(string)
					linesInterface, _ := argsMap["lines"].([]interface{})

					if filename == "" || linesInterface == nil {
						fmt.Printf("Warning: invalid arguments for write_all_lines\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Invalid arguments for write_all_lines",
						})
						continue
					}

					// 将 []interface{} 转换为 []string
					lines := make([]string, len(linesInterface))
					for i, line := range linesInterface {
						if lineStr, ok := line.(string); ok {
							lines[i] = lineStr
						}
					}

					fmt.Printf("Writing all lines to %s\n", filename)
					err := WriteAllLines(filename, lines)
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else {
						output = "Successfully wrote " + strconv.Itoa(len(lines)) + " lines to " + filename
					}

					// 打印输出
					fmt.Println(output)

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "search":
					keyword, _ := argsMap["keyword"].(string)
					if keyword == "" {
						fmt.Printf("Warning: empty keyword in search tool call\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Empty keyword in search tool call",
						})
						continue
					}

					fmt.Printf("Searching for: %s\n", keyword)
					resultsList, err := Search(keyword)

					// 将搜索结果转换为 TOON 字符串
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else if resultsList != nil {
						resultsTOON, err := toon.Marshal(resultsList)
						if err != nil {
							output = "Error: Failed to marshal search results"
							log.Printf("Failed to marshal search results: %v", err)
						} else {
							output = string(resultsTOON)
						}
					} else {
						output = "No search results found"
					}

					// 打印输出
					fmt.Println("Search completed")

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "visit":
					url, _ := argsMap["url"].(string)
					if url == "" {
						fmt.Printf("Warning: empty url in visit tool call\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Empty url in visit tool call",
						})
						continue
					}

					fmt.Printf("Visiting: %s\n", url)
					pageText, err := Visit(url)
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else {
						output = "Visit completed. Page content: " + pageText
					}

					// 打印输出
					fmt.Println("Visit completed")

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "download":
					url, _ := argsMap["url"].(string)
					if url == "" {
						fmt.Printf("Warning: empty url in download tool call\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Empty url in download tool call",
						})
						continue
					}

					fmt.Printf("Downloading from: %s\n", url)
					fileName, err := Download(url)
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else {
						output = "Download completed, saved to: " + fileName
					}

					// 打印输出
					fmt.Println(output)

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
				case "todo":
					itemsInterface, _ := argsMap["items"].([]interface{})
					if itemsInterface == nil {
						fmt.Printf("Warning: invalid items in todo tool call\n")
						// 即使参数无效，也要添加一个错误结果
						results = append(results, ToolResult{
							Type:      "tool_result",
							ToolUseID: toolID,
							Content:   "Error: Invalid items in todo tool call",
						})
						continue
					}

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
						output = "Error: " + err.Error()
					}

					// 打印输出
					fmt.Println(output)

					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   output,
					})
					usedTodo = true
				default:
					// 即使工具名称不匹配，也要添加一个错误结果
					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: toolID,
						Content:   "Error: Unknown tool name",
					})
					continue
				}

				if isDebug {
					fmt.Printf("Added tool result, now %d results\n", len(results))
				}
			}
		} else {
			// Anthropic与Ollama的工具调用格式
			if contentArray, ok := resp.Content.([]interface{}); ok {
				for _, item := range contentArray {
					if toolUse, ok := item.(map[string]interface{}); ok {
						if toolUse["type"] == "tool_use" {
							toolName, nameOk := toolUse["name"].(string)
							input, inputOk := toolUse["input"].(map[string]interface{})
							toolID, _ := toolUse["id"].(string)

							// 检查必要字段
							if !nameOk || !inputOk {
								// 即使字段无效，也要添加一个错误结果
								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolID,
									Content:   "Error: Invalid tool use fields",
								})
								continue
							}
							switch toolName {
							case "shell":
								command, _ := input["command"].(string)
								if command == "" {
									// 即使命令为空，也要添加一个错误结果
									results = append(results, ToolResult{
										Type:      "tool_result",
										ToolUseID: toolID,
										Content:   "Error: Empty command",
									})
									continue
								}
								fmt.Printf("$ %s\n", command)
								result := runShell(command)
								var output string
								if result.Err != nil {
									// 真正无法执行的错误（如命令不存在、危险拦截）
									output = fmt.Sprintf("Error: %v", result.Err)
								} else {
									// 命令已执行，根据退出码判断
									output = result.Stdout
									if result.ExitCode != 0 {
										// 可附加 stderr 信息
										if result.Stderr != "" {
											output += "\n" + result.Stderr
										}
									}
								}
								if len(output) > 200 {
									fmt.Println(TruncateString(output, 200))
								} else {
									fmt.Println(output)
								}
								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolID,
									Content:   output,
								})
							case "read_file_line":
								filename, _ := input["filename"].(string)
								lineNumFloat, _ := input["line_num"].(float64)
								lineNum := int(lineNumFloat)

								if filename == "" || lineNum < 1 {
									fmt.Printf("Warning: invalid arguments for read_file_line\n")
									// 即使参数无效，也要添加一个错误结果
									results = append(results, ToolResult{
										Type:      "tool_result",
										ToolUseID: toolID,
										Content:   "Error: Invalid arguments for read_file_line",
									})
									continue
								}

								fmt.Printf("Reading line %d from %s\n", lineNum, filename)
								content, err := ReadFileLine(filename, lineNum)
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else {
									output = content
								}

								// 打印输出（截断）
								if len(output) > 200 {
									fmt.Println(TruncateString(output, 200))
								} else {
									fmt.Println(output)
								}

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "write_file_line":
								filename := input["filename"].(string)
								lineNum := int(input["line_num"].(float64))
								content := input["content"].(string)

								if filename == "" || lineNum < 1 {
									fmt.Printf("Warning: invalid arguments for write_file_line\n")
									continue
								}

								fmt.Printf("Writing to line %d in %s\n", lineNum, filename)
								err := WriteFileLine(filename, lineNum, content)
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else {
									output = "Successfully wrote to line " + strconv.Itoa(lineNum)
								}

								// 打印输出
								fmt.Println(output)

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "read_all_lines":
								filename := input["filename"].(string)

								if filename == "" {
									fmt.Printf("Warning: invalid arguments for read_all_lines\n")
									continue
								}

								fmt.Printf("Reading all lines from %s\n", filename)
								lines, err := ReadAllLines(filename)
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else {
									// 将字符串切片转换为JSON字符串
									linesJSON, err := json.Marshal(lines)
									if err != nil {
										output = "Error: " + err.Error()
									} else {
										output = string(linesJSON)
									}
								}

								// 打印输出（截断）
								if len(output) > 200 {
									fmt.Println(TruncateString(output, 200))
								} else {
									fmt.Println(output)
								}

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "write_all_lines":
								filename := input["filename"].(string)
								linesInterface := input["lines"].([]interface{})

								if filename == "" || linesInterface == nil {
									fmt.Printf("Warning: invalid arguments for write_all_lines\n")
									continue
								}

								// 将 []interface{} 转换为 []string
								lines := make([]string, len(linesInterface))
								for i, line := range linesInterface {
									if lineStr, ok := line.(string); ok {
										lines[i] = lineStr
									}
								}

								fmt.Printf("Writing all lines to %s\n", filename)
								err := WriteAllLines(filename, lines)
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else {
									output = "Successfully wrote " + strconv.Itoa(len(lines)) + " lines to " + filename
								}

								// 打印输出
								fmt.Println(output)

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "search":
								keyword := input["keyword"].(string)
								if keyword == "" {
									fmt.Printf("Warning: empty keyword in search tool call\n")
									continue
								}

								fmt.Printf("Searching for: %s\n", keyword)
								resultsList, err := Search(keyword)

								// 将搜索结果转换为 TOON 字符串
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else if resultsList != nil {
									resultsTOON, err := toon.Marshal(resultsList)
									if err != nil {
										output = "Error: Failed to marshal search results"
										log.Printf("Failed to marshal search results: %v", err)
									} else {
										output = string(resultsTOON)
									}
								} else {
									output = "No search results found"
								}

								// 打印输出
								fmt.Println("Search completed")

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "visit":
								url := input["url"].(string)
								if url == "" {
									fmt.Printf("Warning: empty url in visit tool call\n")
									continue
								}

								fmt.Printf("Visiting: %s\n", url)
								pageText, err := Visit(url)
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else {
									output = "Visit completed. Page content: " + pageText
								}

								// 打印输出
								fmt.Println("Visit completed")

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "download":
								url := input["url"].(string)
								if url == "" {
									fmt.Printf("Warning: empty url in download tool call\n")
									continue
								}

								fmt.Printf("Downloading from: %s\n", url)
								fileName, err := Download(url)
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else {
									output = "Download completed, saved to: " + fileName
								}

								// 打印输出
								fmt.Println(output)

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "todo":
								itemsInterface := input["items"].([]interface{})
								if itemsInterface == nil {
									fmt.Printf("Warning: invalid items in todo tool call\n")
									continue
								}

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
									output = "Error: " + err.Error()
								}

								// 打印输出
								fmt.Println(output)

								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
								usedTodo = true
							default:
								continue
							}
						}
					}
				}
			}
		}

		// 更新todo使用计数并添加提醒
		if usedTodo {
			roundsSinceTodo = 0
		} else {
			roundsSinceTodo++
			if roundsSinceTodo >= 3 {
				// 注入todo提醒
				messages = append(messages, Message{
					Role:    "user",
					Content: "<reminder>Update your todos.</reminder>",
				})
				roundsSinceTodo = 0
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

		// 打印调试信息，查看messages数组
		if isDebug {
			fmt.Printf("Number of messages before second call: %d\n", len(messages))
			for i, msg := range messages {
				fmt.Printf("Message %d: Role=%s, Content=%v, ToolCallID=%s\n", i, msg.Role, msg.Content, msg.ToolCallID)
			}
		}

		// 再次调用LLM，获取模型对工具执行结果的响应
		// 注意：下一次循环会再次调用CallModel，其中会打印文本内容
	}
}
