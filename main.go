package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/toon-format/toon-go"
)

// 配置
const (
	DEFAULT_API_TYPE       = "openai" // 可选值: anthropic, ollama, openai
	ANTHROPIC_BASE_URL     = "https://api.anthropic.com/v1"
	OLLAMA_BASE_URL        = "http://localhost:11434/api"
	OPENAI_BASE_URL        = "https://api.openai.com/v1"
	DEFAULT_MODEL_ID       = "claude-3-opus-20240229"
	CONFIG_FILE            = "config.toon"
	isDebug                = false // 控制调试信息的显示
	SYSTEM_PROMPT_TEMPLATE = "You are a coding agent. When the user asks to list files, run commands, or interact with the system, you MUST use the shell {{tool_or_function}}. When you need to read a specific line from a file, use the read_file_line {{tool_or_function}}. When you need to write content to a specific line in a file, use the write_file_line {{tool_or_function}}. When you need to read all lines from a file, use the read_all_lines {{tool_or_function}}. When you need to write all lines to a file, use the write_all_lines {{tool_or_function}}. When you need to manage tasks, use the todo {{tool_or_function}}. Do NOT explain how to run the command, do NOT provide alternative methods, just use the {{tool_or_function}} directly. For example, when asked to list files, use the shell {{tool_or_function}} with command 'ls' or 'ls -la' (Unix/Linux). Your response MUST be a {{tool_or_function}} call, not a regular message. Under no circumstances should you provide explanations or instructions to the user - only use the {{tool_or_function}}."
)

// 消息结构
type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"`
	ToolCalls  interface{} `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

// 工具调用结构
type ToolUse struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// 工具结果结构
type ToolResult struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

// 响应结构
type Response struct {
	Content    interface{} `json:"content"`
	StopReason string      `json:"stop_reason"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Content      string
	ToolCalls    []map[string]interface{} // 用于存放工具调用（可能是多个，每个可能不完整）
	Done         bool
	Error        error
	FinishReason string
}

// 调用LLM API
func CallModel(messages []Message, apiType, baseURL, apiKey, modelID string, temperature float64, maxTokens int) (Response, error) {
	// 确保有默认值
	if apiType == "" {
		apiType = DEFAULT_API_TYPE
	}
	if modelID == "" {
		modelID = DEFAULT_MODEL_ID
	}

	var data map[string]interface{}
	var endpoint string

	switch apiType {
	case "anthropic":
		// 如果baseURL为空，使用默认值
		if baseURL == "" {
			baseURL = ANTHROPIC_BASE_URL
		}
		// 生成系统提示，Anthropic 使用 "tool"
		systemPrompt := strings.ReplaceAll(SYSTEM_PROMPT_TEMPLATE, "{{tool_or_function}}", "tool")
		data = map[string]interface{}{
			"model":       modelID,
			"system":      systemPrompt,
			"messages":    messages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      true,
		}
		endpoint = "/messages"

	case "ollama":
		// Ollama使用固定的baseURL
		baseURL = OLLAMA_BASE_URL
		// Ollama不需要API key
		// 转换messages格式为Ollama格式
		ollamaMessages := []map[string]interface{}{}
		for _, msg := range messages {
			ollamaMsg := map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			}
			ollamaMessages = append(ollamaMessages, ollamaMsg)
		}
		// 生成系统提示，Ollama 使用 "tool"
		systemPrompt := strings.ReplaceAll(SYSTEM_PROMPT_TEMPLATE, "{{tool_or_function}}", "tool")
		data = map[string]interface{}{
			"model":       modelID,
			"messages":    ollamaMessages,
			"tools":       getTools(apiType),
			"stream":      true,
			"system":      systemPrompt,
			"temperature": temperature,
		}
		endpoint = "/chat"

	case "openai":
		// 如果baseURL为空，使用默认值
		if baseURL == "" {
			baseURL = OPENAI_BASE_URL
		}
		if isDebug {
			fmt.Printf("Using OpenAI base URL: %s\n", baseURL)
		}
		// 转换messages格式为OpenAI格式
		openaiMessages := []map[string]interface{}{}
		for _, msg := range messages {
			openaiMsg := map[string]interface{}{}

			// 设置角色
			openaiMsg["role"] = msg.Role

			// 处理不同类型的消息
			if msg.Role == "tool" {
				// 工具消息的格式
				openaiMsg["tool_call_id"] = msg.ToolCallID
				openaiMsg["content"] = msg.Content
			} else if msg.Role == "assistant" && msg.ToolCalls != nil {
				// 带有工具调用的assistant消息
				openaiMsg["tool_calls"] = msg.ToolCalls
				// 有些模型需要content字段，即使为空
				if msg.Content != nil {
					openaiMsg["content"] = msg.Content
				} else {
					openaiMsg["content"] = nil
				}
			} else {
				// 普通消息
				openaiMsg["content"] = msg.Content
			}

			openaiMessages = append(openaiMessages, openaiMsg)
		}
		// 生成系统提示，OpenAI 使用 "function"
		systemPrompt := strings.ReplaceAll(SYSTEM_PROMPT_TEMPLATE, "{{tool_or_function}}", "function")
		data = map[string]interface{}{
			"model":       modelID,
			"messages":    openaiMessages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      true, // 启用流式
			"system":      systemPrompt,
		}
		endpoint = "/chat/completions"

	default:
		return Response{}, fmt.Errorf("unsupported API type: %s", apiType)
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return Response{}, err
	}

	req, err := http.NewRequest("POST", baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return Response{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		if apiType == "openai" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else if apiType == "anthropic" {
			req.Header.Set("x-api-key", apiKey)
		}
	}

	client := &http.Client{}
	if isDebug {
		fmt.Printf("Sending request to: %s\n", baseURL+endpoint)
		fmt.Printf("Request data: %v\n", data)
	}

	resp, err := client.Do(req)
	if err != nil {
		if isDebug {
			fmt.Printf("Error sending request: %v\n", err)
		}
		return Response{}, err
	}
	defer resp.Body.Close()

	// 打印响应状态码
	if isDebug {
		fmt.Printf("Response status code: %d\n", resp.StatusCode)
	}

	// 处理流式输出
	if streamEnabled, ok := data["stream"].(bool); ok && streamEnabled {
		// 获取流式响应通道
		chunkChan, err := getStreamChunks(resp.Body, apiType)
		if err != nil {
			return Response{}, err
		}

		// 处理流式响应
		var fullContent strings.Builder
		var finishReason string = "stop"

		// 用于收集工具调用的结构（按索引）
		type pendingToolCall struct {
			ID       string
			Type     string
			Function struct {
				Name      string
				Arguments strings.Builder
			}
		}
		pendingTools := make(map[int]*pendingToolCall)

		for chunk := range chunkChan {
			if chunk.Error != nil {
				return Response{}, chunk.Error
			}

			// 实时打印文本内容
			if chunk.Content != "" {
				fmt.Print(chunk.Content)
				stdout := os.Stdout
				stdout.Sync()
				fullContent.WriteString(chunk.Content)
			}

			// 处理工具调用块
			if len(chunk.ToolCalls) > 0 {
				for _, tc := range chunk.ToolCalls {
					// 获取索引
					idxVal, hasIdx := tc["index"]
					if !hasIdx {
						// 无有 index，直接当作完整工具调用处理（可能某些API不使用index）
						// 这种情况简单处理，直接添加到结果中
						// 但为了保险，我们仍尝试处理
						continue
					}
					idx, ok := idxVal.(float64)
					if !ok {
						continue
					}
					intIdx := int(idx)

					// 获取或创建 pending tool
					pt, exists := pendingTools[intIdx]
					if !exists {
						pt = &pendingToolCall{}
						pendingTools[intIdx] = pt
					}

					// 填充字段
					if id, ok := tc["id"].(string); ok && id != "" {
						pt.ID = id
					}
					if typ, ok := tc["type"].(string); ok && typ != "" {
						pt.Type = typ
					}
					if function, ok := tc["function"].(map[string]interface{}); ok {
						if name, ok := function["name"].(string); ok && name != "" {
							pt.Function.Name = name
						}
						if args, ok := function["arguments"].(string); ok && args != "" {
							// 拼接 arguments
							pt.Function.Arguments.WriteString(args)
						}
					}
				}
			}

			if chunk.Done {
				if chunk.FinishReason != "" {
					finishReason = chunk.FinishReason
				}
				fmt.Println() // 换行
				stdout := os.Stdout
				stdout.Sync()
				break
			}
		}

		// 流结束，将收集的工具调用转换为最终格式
		if len(pendingTools) > 0 {
			var toolCalls []map[string]interface{}
			// 按索引排序
			for i := 0; i < len(pendingTools); i++ {
				pt := pendingTools[i]
				if pt == nil {
					continue
				}
				tc := map[string]interface{}{
					"id":   pt.ID,
					"type": pt.Type,
					"function": map[string]interface{}{
						"name":      pt.Function.Name,
						"arguments": pt.Function.Arguments.String(),
					},
				}
				toolCalls = append(toolCalls, tc)
			}
			return Response{
				Content:    toolCalls,
				StopReason: finishReason,
			}, nil
		}

		// 没有工具调用，返回普通文本
		return Response{
			Content:    fullContent.String(),
			StopReason: finishReason,
		}, nil
	} else {
		// 处理非流式输出
		// 读取响应体
		var responseBody []byte
		responseBody, err = io.ReadAll(resp.Body)
		if err != nil {
			if isDebug {
				fmt.Printf("Error reading response body: %v\n", err)
			}
			return Response{}, err
		}

		// 打印响应体
		if isDebug {
			fmt.Printf("Response body: %s\n", string(responseBody))
		}

		// 重置响应体，以便后续解码
		r := bytes.NewReader(responseBody)
		resp.Body = io.NopCloser(r)

		// 处理不同API的响应格式
		var result Response
		if apiType == "openai" {
			// OpenAI的响应格式不同
			var openaiResp struct {
				Choices []struct {
					Message struct {
						Role      string      `json:"role"`
						Content   interface{} `json:"content"`
						ToolCalls []struct {
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
						FunctionCall struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function_call"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}

			err = json.NewDecoder(resp.Body).Decode(&openaiResp)
			if err != nil {
				return Response{}, err
			}

			if len(openaiResp.Choices) > 0 {
				choice := openaiResp.Choices[0]
				result.StopReason = choice.FinishReason

				// 打印message的完整结构，用于调试
				if isDebug {
					messageJson, _ := json.Marshal(choice.Message)
					fmt.Printf("Message structure: %s\n", string(messageJson))
				}

				// 打印文本内容（如果有） - 这是为了让用户看到模型的回复
				if content, ok := choice.Message.Content.(string); ok && content != "" {
					fmt.Println(content)
				}

				// 检查是否有tool_calls字段（标准OpenAI格式）
				if len(choice.Message.ToolCalls) > 0 {
					var content []map[string]interface{}
					for _, toolCall := range choice.Message.ToolCalls {
						// 解析arguments
						var args map[string]interface{}
						json.Unmarshal([]byte(toolCall.Function.Arguments), &args)

						toolUse := map[string]interface{}{
							"id":   toolCall.ID,
							"type": "function",
							"function": map[string]interface{}{
								"name":      toolCall.Function.Name,
								"arguments": toolCall.Function.Arguments,
							},
						}
						content = append(content, toolUse)
					}
					result.Content = content
					// 强制设置stop_reason为function_call
					result.StopReason = "function_call"
				} else {
					// 检查是否有function_call字段（某些模型的格式）
					if choice.Message.FunctionCall.Name != "" {
						// 解析arguments
						var args map[string]interface{}
						json.Unmarshal([]byte(choice.Message.FunctionCall.Arguments), &args)

						toolUse := map[string]interface{}{
							"type":  "function",
							"id":    "1", // 某些模型可能没有id
							"name":  choice.Message.FunctionCall.Name,
							"input": args,
						}
						result.Content = []map[string]interface{}{toolUse}
						// 强制设置stop_reason为function_call
						result.StopReason = "function_call"
					} else {
						// 纯文本回复，直接作为结果内容（不需要额外打印，因为上面已经打印了）
						result.Content = choice.Message.Content
					}
				}
			}
		} else if apiType == "ollama" {
			// Ollama的响应格式
			var ollamaResp struct {
				Message struct {
					Role    string      `json:"role"`
					Content interface{} `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}

			err = json.NewDecoder(resp.Body).Decode(&ollamaResp)
			if err != nil {
				return Response{}, err
			}

			// 打印文本内容
			if content, ok := ollamaResp.Message.Content.(string); ok && content != "" {
				fmt.Println(content)
			}

			result.Content = ollamaResp.Message.Content
			if ollamaResp.Done {
				result.StopReason = "stop"
			} else {
				result.StopReason = "tool_use"
			}
		} else {
			// Anthropic的响应格式
			err = json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				return Response{}, err
			}
			// 打印文本内容（假设Content是字符串）
			if content, ok := result.Content.(string); ok && content != "" {
				fmt.Println(content)
			}
		}

		return result, nil
	}
}

// getStreamChunks 从响应体中获取流式响应块
func getStreamChunks(body io.ReadCloser, apiType string) (<-chan StreamChunk, error) {
	chunkChan := make(chan StreamChunk, 100)

	go func() {
		defer close(chunkChan)
		defer body.Close()

		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 64*1024), 10*1024*1024) // 10MB max

		for scanner.Scan() {
			line := scanner.Text()

			// 只处理以 data: 开头的行（SSE格式）
			if strings.HasPrefix(line, "data:") {
				// 移除 data: 前缀，包括可能的空格
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)

				if data == "[DONE]" {
					chunkChan <- StreamChunk{Done: true}
					return
				}

				// 解析 JSON 响应
				var response map[string]interface{}
				if err := json.Unmarshal([]byte(data), &response); err != nil {
					continue
				}

				// 处理 OpenAI 格式
				if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
					choice := choices[0]
					if choiceMap, ok := choice.(map[string]interface{}); ok {
						chunk := StreamChunk{}

						// 提取 delta 内容
						if delta, ok := choiceMap["delta"].(map[string]interface{}); ok {
							// 文本内容
							if content, ok := delta["content"].(string); ok {
								chunk.Content = content
							}
							// 工具调用
							if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
								var tcs []map[string]interface{}
								for _, tc := range toolCalls {
									if tcMap, ok := tc.(map[string]interface{}); ok {
										tcs = append(tcs, tcMap)
									}
								}
								chunk.ToolCalls = tcs
							}
						}

						// 检查结束标记
						if finishReason, ok := choiceMap["finish_reason"].(string); ok && finishReason != "" {
							chunk.Done = true
							chunk.FinishReason = finishReason
						}

						// 发送块
						chunkChan <- chunk
						if chunk.Done {
							return
						}
					}
				}
			} else {
				// 尝试解析为非SSE格式（如Ollama或Anthropic）
				var ollamaChunk struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
					Done bool `json:"done"`
				}

				if err := json.Unmarshal([]byte(line), &ollamaChunk); err == nil {
					// Ollama格式
					chunkChan <- StreamChunk{
						Content: ollamaChunk.Message.Content,
						Done:    ollamaChunk.Done,
					}

					if ollamaChunk.Done {
						return
					}
					continue
				}

				// 尝试解析为Anthropic格式
				var anthropicChunk struct {
					Type  string `json:"type"`
					Delta struct {
						Text string `json:"text"`
					} `json:"delta"`
				}

				if err := json.Unmarshal([]byte(line), &anthropicChunk); err == nil {
					// Anthropic格式
					if anthropicChunk.Type == "content_block_delta" {
						chunkChan <- StreamChunk{Content: anthropicChunk.Delta.Text}
					} else if anthropicChunk.Type == "message_stop" {
						chunkChan <- StreamChunk{Done: true, FinishReason: "stop"}
						return
					}
					continue
				}
			}
		}

		if err := scanner.Err(); err != nil {
			chunkChan <- StreamChunk{Error: fmt.Errorf("scanner: %w", err)}
		}
	}()

	return chunkChan, nil
}

// 核心agent循环
func agentLoop(messages []Message, apiType, baseURL, apiKey, modelID string, temperature float64, maxTokens int) {
	roundsSinceTodo := 0
	for {
		resp, err := CallModel(messages, apiType, baseURL, apiKey, modelID, temperature, maxTokens)
		if err != nil {
			fmt.Printf("Error calling LLM: %v\n", err)
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
				Role:    "assistant",
				Content: resp.Content,
			})
		}

		// 如果模型没有调用工具，结束
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
					continue
				}
				toolID, _ := toolUse["id"].(string)
				function, ok := toolUse["function"].(map[string]interface{})
				if !ok {
					continue
				}
				toolName, _ := function["name"].(string)
				argsStr, _ := function["arguments"].(string)

				// 解析arguments JSON
				var argsMap map[string]interface{}
				if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
					fmt.Printf("Failed to parse arguments: %v\n", err)
					continue
				}

				switch toolName {
				case "shell":
					command, _ := argsMap["command"].(string)
					if command == "" {
						fmt.Printf("Warning: empty command in tool call\n")
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
						fmt.Println(output[:512] + "...")
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
						fmt.Println(output[:200] + "...")
					} else {
						fmt.Println(output)
					}

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
						fmt.Println(output[:200] + "...")
					} else {
						fmt.Println(output)
					}

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
				case "download_novel":
					novelURL, _ := argsMap["novel_url"].(string)
					if novelURL == "" {
						fmt.Printf("Warning: empty novel_url in download_novel tool call\n")
						continue
					}

					fmt.Printf("Downloading novel from: %s\n", novelURL)
					err := DownloadNovel(novelURL)
					var output string
					if err != nil {
						output = "Error: " + err.Error()
					} else {
						output = "Novel download completed"
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
							toolName := toolUse["name"].(string)
							input := toolUse["input"].(map[string]interface{})
							switch toolName {
							case "shell":
								command := input["command"].(string)
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
									fmt.Println(output[:200])
								} else {
									fmt.Println(output)
								}
								results = append(results, ToolResult{
									Type:      "tool_result",
									ToolUseID: toolUse["id"].(string),
									Content:   output,
								})
							case "read_file_line":
								filename := input["filename"].(string)
								lineNum := int(input["line_num"].(float64))

								if filename == "" || lineNum < 1 {
									fmt.Printf("Warning: invalid arguments for read_file_line\n")
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
									fmt.Println(output[:200] + "...")
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
									fmt.Println(output[:200] + "...")
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
							case "download_novel":
								novelURL := input["novel_url"].(string)
								if novelURL == "" {
									fmt.Printf("Warning: empty novel_url in download_novel tool call\n")
									continue
								}

								fmt.Printf("Downloading novel from: %s\n", novelURL)
								err := DownloadNovel(novelURL)
								var output string
								if err != nil {
									output = "Error: " + err.Error()
								} else {
									output = "Novel download completed"
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

func main() {
	// 读取配置文件
	config, err := loadConfig()

	// 优先使用配置文件中的值，当配置文件中没有值时，使用环境变量中的值
	apiType := config.APIConfig.APIType
	if apiType == "" {
		apiType = os.Getenv("API_TYPE")
		if apiType == "" {
			apiType = "openai" // 默认值
		}
	}

	baseURL := config.APIConfig.BaseURL
	if baseURL == "" {
		if apiType == "openai" {
			baseURL = os.Getenv("OPENAI_BASE_URL")
		} else if apiType == "anthropic" {
			baseURL = os.Getenv("ANTHROPIC_BASE_URL")
		}
	}

	apiKey := config.APIConfig.APIKey
	if apiKey == "" {
		if apiType == "openai" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		} else if apiType == "anthropic" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}

	modelID := config.APIConfig.Model
	if modelID == "" {
		modelID = os.Getenv("MODEL_ID")
		if modelID == "" {
			modelID = DEFAULT_MODEL_ID
		}
	}

	temperature := config.APIConfig.Temperature
	if temperature == 0 {
		tempStr := os.Getenv("TEMPERATURE")
		if tempStr != "" {
			if temp, err := strconv.ParseFloat(tempStr, 64); err == nil {
				temperature = temp
			}
		}
		if temperature == 0 {
			temperature = 0.7 // 默认值
		}
	}

	maxTokens := config.APIConfig.MaxTokens
	if maxTokens == 0 {
		tokensStr := os.Getenv("MAX_TOKENS")
		if tokensStr != "" {
			if tokens, err := strconv.Atoi(tokensStr); err == nil {
				maxTokens = tokens
			}
		}
		if maxTokens == 0 {
			maxTokens = 4096 // 默认值
		}
	}

	if err != nil {
		fmt.Printf("Warning: Error loading config file: %v\n", err)
		fmt.Println("Using environment variables for configuration")
	} else {
		fmt.Printf("Configuration loaded from config.toon (API type: %s)\n", apiType)
	}

	// 打印最终使用的配置
	if isDebug {
		fmt.Printf("Using API type: %s\n", apiType)
		fmt.Printf("Using base URL: %s\n", baseURL)
	}

	fmt.Printf("Using model: %s\n", modelID) // 所有模式下都打印模型ID

	var history []Message
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("GarClaw /> ")
		if !scanner.Scan() {
			break
		}
		var query string
		query = scanner.Text()
		// 去除空白字符
		trimmedQuery := strings.TrimSpace(query)
		if strings.ToLower(trimmedQuery) == "q" || strings.ToLower(trimmedQuery) == "exit" || trimmedQuery == "" {
			break
		}
		query = trimmedQuery

		// 正常处理查询
		history = append(history, Message{
			Role:    "user",
			Content: query,
		})

		agentLoop(history, apiType, baseURL, apiKey, modelID, temperature, maxTokens)

		// 流式输出已经在CallModel函数中实时打印，这里不再重复打印
		// 只打印一个空行作为分隔
		fmt.Println()
	}
}
