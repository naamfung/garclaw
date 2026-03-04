package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// 调用LLM API
func CallModel(messages []Message, apiType, baseURL, apiKey, modelID string, temperature float64, maxTokens int, stream bool) (Response, error) {
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
		currentTime := time.Now().Format("2006-01-02 15:04:05") // 添加当前时间信息
		systemPrompt := fmt.Sprintf("当前系统时间：%s\n", currentTime) + strings.ReplaceAll(SYSTEM_PROMPT_TEMPLATE, "{{tool_or_function}}", "tool")
		data = map[string]interface{}{
			"model":       modelID,
			"system":      systemPrompt,
			"messages":    messages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      stream,
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
		currentTime := time.Now().Format("2006-01-02 15:04:05") // 添加当前时间信息
		systemPrompt := fmt.Sprintf("当前系统时间：%s\n", currentTime) + strings.ReplaceAll(SYSTEM_PROMPT_TEMPLATE, "{{tool_or_function}}", "tool")
		data = map[string]interface{}{
			"model":       modelID,
			"messages":    ollamaMessages,
			"tools":       getTools(apiType),
			"stream":      stream,
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
		currentTime := time.Now().Format("2006-01-02 15:04:05") // 添加当前时间信息
		systemPrompt := fmt.Sprintf("当前系统时间：%s\n", currentTime) + strings.ReplaceAll(SYSTEM_PROMPT_TEMPLATE, "{{tool_or_function}}", "function")
		data = map[string]interface{}{
			"model":       modelID,
			"messages":    openaiMessages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      stream, // 启用流式
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
