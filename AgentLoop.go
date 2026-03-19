package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/toon-format/toon-go"
)

// executeTool 执行单个工具调用，返回 ToolResult 和是否使用了 todo
func executeTool(ctx context.Context, toolID, toolName string, argsMap map[string]interface{}, ch Channel) (ToolResult, bool) {
	usedTodo := false
	var content string

	switch toolName {
	case "shell":
		command, ok := argsMap["command"].(string)
		if !ok || command == "" {
			content = "Error: Invalid or empty command"
		} else {
			ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("$ %s\n", command)})
			result := runShell(ctx, command)
			if result.Err != nil {
				if ctx.Err() == context.Canceled {
					content = "Command cancelled by user."
				} else {
					content = fmt.Sprintf("Error: %v", result.Err)
				}
			} else {
				content = result.Stdout
				if result.ExitCode != 0 && result.Stderr != "" {
					content += "\n" + result.Stderr
				}
			}
			if len(content) > 512 && IsDebug {
				ch.WriteChunk(StreamChunk{Content: TruncateString(content, 512)})
			} else {
				ch.WriteChunk(StreamChunk{Content: content})
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
			ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("Reading line %d from %s\n", lineNum, filename)})
			c, err := ReadFileLine(filename, lineNum)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = c
			}
			ch.WriteChunk(StreamChunk{Content: TruncateString(content, 200)})
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
			ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("Writing to line %d in %s\n", lineNum, filename)})
			err := WriteFileLine(filename, lineNum, text)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = "Successfully wrote to line " + strconv.Itoa(lineNum)
			}
			ch.WriteChunk(StreamChunk{Content: content})
			fmt.Println(content)
		}

	case "read_all_lines":
		filename, ok := argsMap["filename"].(string)
		if !ok || filename == "" {
			content = "Error: Invalid arguments for read_all_lines"
		} else {
			ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("Reading all lines from %s\n", filename)})
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
			ch.WriteChunk(StreamChunk{Content: TruncateString(content, 200)})
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
				ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("Writing all lines to %s\n", filename)})
				err := WriteAllLines(filename, lines)
				if err != nil {
					content = "Error: " + err.Error()
				} else {
					content = "Successfully wrote " + strconv.Itoa(len(lines)) + " lines to " + filename
				}
				ch.WriteChunk(StreamChunk{Content: content})
				fmt.Println(content)
			}
		}

	case "search":
		keyword, ok := argsMap["keyword"].(string)
		if !ok || keyword == "" {
			content = "Error: Empty keyword in search tool call"
		} else {
			ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("Searching for: %s\n", keyword)})
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
			ch.WriteChunk(StreamChunk{Content: "Search completed\n"})
			fmt.Println("Search completed")
		}

	case "visit":
		url, ok := argsMap["url"].(string)
		if !ok || url == "" {
			content = "Error: Empty url in visit tool call"
		} else {
			ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("Visiting: %s\n", url)})
			pageText, err := Visit(url)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = "Visit completed. Page content: " + pageText
			}
			ch.WriteChunk(StreamChunk{Content: "Visit completed\n"})
			fmt.Println("Visit completed")
		}

	case "download":
		url, ok := argsMap["url"].(string)
		if !ok || url == "" {
			content = "Error: Empty url in download tool call"
		} else {
			ch.WriteChunk(StreamChunk{Content: fmt.Sprintf("Downloading from: %s\n", url)})
			fileName, err := Download(url)
			if err != nil {
				content = "Error: " + err.Error()
			} else {
				content = "Download completed, saved to: " + fileName
			}
			ch.WriteChunk(StreamChunk{Content: content})
			fmt.Println(content)
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
				ch.WriteChunk(StreamChunk{Content: content})
				fmt.Println(content)
				usedTodo = true
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

// AgentLoop 核心循环，检查写入错误并退出
func AgentLoop(ctx context.Context, ch Channel, messages []Message, apiType, baseURL, apiKey, modelID string,
	temperature float64, maxTokens int, stream bool, thinking bool) ([]Message, error) {

	roundsSinceTodo := 0

	for {
		select {
		case <-ctx.Done():
			return messages, ctx.Err()
		default:
		}

		chunkChan, err := CallModel(ctx, messages, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
		if err != nil {
			if writeErr := ch.WriteChunk(StreamChunk{Error: err}); writeErr != nil {
				log.Printf("Failed to write error chunk: %v", writeErr)
			}
			return messages, err
		}

		var respContent interface{}
		var reasoningContent string
		var toolCalls []map[string]interface{}
		var stopReason string

		for chunk := range chunkChan {
			select {
			case <-ctx.Done():
				ch.WriteChunk(StreamChunk{Error: ctx.Err()})
				return messages, ctx.Err()
			default:
			}

			if chunk.Error != nil {
				if writeErr := ch.WriteChunk(chunk); writeErr != nil {
					log.Printf("Failed to write error chunk: %v", writeErr)
					return messages, chunk.Error
				}
				return messages, chunk.Error
			}

			// 发送给频道，如果失败则退出
			if writeErr := ch.WriteChunk(chunk); writeErr != nil {
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
			if len(chunk.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.ToolCalls...)
			}
			if chunk.Done {
				stopReason = chunk.FinishReason
				break
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

		// 检查是否需要结束
		if stopReason != "tool_use" && stopReason != "function_call" && stopReason != "tool_calls" {
			break
		}

		// 执行工具调用
		var results []ToolResult
		usedTodo := false

		if IsDebug {
			fmt.Println("===================== Executing tool calls =====================")
			fmt.Printf("API type: %s\n", apiType)
			fmt.Printf("Response content type: %T\n", respContent)
			fmt.Printf("Response content: %v\n", respContent)
		}

		if apiType == "openai" {
			var toolCallsSlice []interface{}
			switch v := respContent.(type) {
			case []interface{}:
				toolCallsSlice = v
			case []map[string]interface{}:
				toolCallsSlice = make([]interface{}, len(v))
				for i, m := range v {
					toolCallsSlice[i] = m
				}
			default:
				if IsDebug {
					fmt.Printf("Warning: resp.Content is not a slice of tool calls: %T\n", respContent)
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
					if IsDebug {
						fmt.Printf("Failed to parse arguments: %v\n", err)
					}
					results = append(results, ToolResult{
						Type:      "tool_result",
						ToolUseID: call.ID,
						Content:   "Error: Failed to parse arguments",
					})
					continue
				}

				result, todoUsed := executeTool(ctx, call.ID, call.Name, argsMap, ch)
				results = append(results, result)
				if todoUsed {
					usedTodo = true
				}
			}
		} else {
			if contentArray, ok := respContent.([]interface{}); ok {
				for _, item := range contentArray {
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
							results = append(results, ToolResult{
								Type:      "tool_result",
								ToolUseID: toolID,
								Content:   "Error: Invalid tool use fields",
							})
							continue
						}

						result, todoUsed := executeTool(ctx, toolID, toolName, input, ch)
						results = append(results, result)
						if todoUsed {
							usedTodo = true
						}
					}
				}
			}
		}

		for _, result := range results {
			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: result.ToolUseID,
				Content:    result.Content,
			})
		}

		if usedTodo {
			roundsSinceTodo = 0
		} else {
			roundsSinceTodo++
			if roundsSinceTodo >= 3 {
				messages = append(messages, Message{
					Role:    "user",
					Content: "<reminder>Please update your todo list and then proceed with the task.</reminder>",
				})
				roundsSinceTodo = 0
			}
		}

		if IsDebug {
			fmt.Printf("Number of messages before second call: %d\n", len(messages))
			for i, msg := range messages {
				fmt.Printf("Message %d: Role=%s, Content=%v, ToolCallID=%s\n", i, msg.Role, msg.Content, msg.ToolCallID)
			}
		}
	}

	return messages, nil
}