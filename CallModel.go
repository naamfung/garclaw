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

// 全局 HTTP 客户端
var httpClient = &http.Client{
	Timeout: 10 * time.Minute, // 如DesspSeek的默认超时时间就是10分钟
}

// StreamReplacer 用于流式文本替换（最长匹配）
type StreamReplacer struct {
	buffer             []rune
	maxKeyLen          int
	sortedReplacements []StringReplacement
	out                func(r rune)
}

// NewStreamReplacer 创建流式替换器
func NewStreamReplacer(out func(r rune)) *StreamReplacer {
	sr := &StreamReplacer{
		buffer:             make([]rune, 0),
		sortedReplacements: sortedStringsReplacements.Replacements,
		out:                out,
	}
	// 计算最长键的字符数
	for _, rep := range sr.sortedReplacements {
		if len([]rune(rep.Key)) > sr.maxKeyLen {
			sr.maxKeyLen = len([]rune(rep.Key))
		}
	}
	return sr
}

// Write 处理新文本
func (sr *StreamReplacer) Write(text string) {
	runes := []rune(text)
	for _, r := range runes {
		sr.buffer = append(sr.buffer, r)
		sr.flushSafe()
	}
}

// Flush 输出缓冲区剩余内容
func (sr *StreamReplacer) Flush() {
	for _, r := range sr.buffer {
		sr.out(r)
	}
	sr.buffer = sr.buffer[:0]
}

// flushSafe 处理缓冲区，输出安全字符
func (sr *StreamReplacer) flushSafe() {
	for {
		if len(sr.buffer) == 0 {
			break
		}
		// 尝试从起始位置匹配最长键
		matched := false
		for _, rep := range sr.sortedReplacements {
			keyRunes := []rune(rep.Key)
			if len(keyRunes) <= len(sr.buffer) {
				eq := true
				for i := 0; i < len(keyRunes); i++ {
					if sr.buffer[i] != keyRunes[i] {
						eq = false
						break
					}
				}
				if eq {
					// 输出替换值
					for _, r := range []rune(rep.Value) {
						sr.out(r)
					}
					// 移除匹配部分
					sr.buffer = sr.buffer[len(keyRunes):]
					matched = true
					break
				}
			}
		}
		if matched {
			continue
		}

		// 检查起始位置是否是某个键的前缀
		isPrefix := false
		for _, rep := range sr.sortedReplacements {
			keyRunes := []rune(rep.Key)
			if len(keyRunes) > 0 && len(sr.buffer) < len(keyRunes) {
				eq := true
				for i := 0; i < len(sr.buffer); i++ {
					if sr.buffer[i] != keyRunes[i] {
						eq = false
						break
					}
				}
				if eq {
					isPrefix = true
					break
				}
			}
		}
		if isPrefix {
			// 是某个键的前缀，等待更多字符
			break
		}

		// 不是前缀，输出第一个字符
		sr.out(sr.buffer[0])
		sr.buffer = sr.buffer[1:]
		// 继续循环
	}
}

// applyReplacements 对字符串应用替换（最长匹配，非递归）
func applyReplacements(text string) string {
	runes := []rune(text)
	result := make([]rune, 0, len(runes))
	i := 0
	for i < len(runes) {
		matched := false
		for _, rep := range sortedStringsReplacements.Replacements {
			keyRunes := []rune(rep.Key)
			if i+len(keyRunes) <= len(runes) {
				eq := true
				for j := 0; j < len(keyRunes); j++ {
					if runes[i+j] != keyRunes[j] {
						eq = false
						break
					}
				}
				if eq {
					// 替换
					result = append(result, []rune(rep.Value)...)
					i += len(keyRunes)
					matched = true
					break
				}
			}
		}
		if !matched {
			result = append(result, runes[i])
			i++
		}
	}
	return string(result)
}

// 生成系统提示
func generateSystemPrompt(apiType string) string {
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	toolOrFunction := "tool"
	if apiType == "openai" {
		toolOrFunction = "function"
	}
	return fmt.Sprintf("当前系统时间：%s\n", currentTime) + strings.ReplaceAll(SYSTEM_PROMPT, "{{tool_or_function}}", toolOrFunction)
}

// convertToAnthropicFormat 將內部 Message 轉換為 Anthropic API 要求的格式
func convertToAnthropicFormat(messages []Message) []map[string]interface{} {
	anthropicMessages := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role":    "user",
				"content": msg.Content,
			})
		case "assistant":
			if msg.ToolCalls != nil {
				// 构建 content 数组，包含 text 和 tool_use
				content := []map[string]interface{}{}
				if msg.Content != nil {
					if txt, ok := msg.Content.(string); ok && txt != "" {
						content = append(content, map[string]interface{}{
							"type": "text",
							"text": txt,
						})
					}
				}
				if toolCalls, ok := msg.ToolCalls.([]interface{}); ok {
					for _, tc := range toolCalls {
						if tcMap, ok := tc.(map[string]interface{}); ok {
							if function, ok := tcMap["function"].(map[string]interface{}); ok {
								toolUse := map[string]interface{}{
									"type": "tool_use",
									"id":   tcMap["id"],
									"name": function["name"],
								}
								// arguments 可能是字符串，尝试解析为对象
								if argsStr, ok := function["arguments"].(string); ok {
									var args map[string]interface{}
									if err := json.Unmarshal([]byte(argsStr), &args); err == nil {
										toolUse["input"] = args
									} else {
										toolUse["input"] = argsStr
									}
								}
								content = append(content, toolUse)
							}
						}
					}
				}
				anthropicMessages = append(anthropicMessages, map[string]interface{}{
					"role":    "assistant",
					"content": content,
				})
			} else {
				anthropicMessages = append(anthropicMessages, map[string]interface{}{
					"role":    "assistant",
					"content": msg.Content,
				})
			}
		case "tool":
			toolResult := map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     msg.Content,
			}
			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					toolResult,
				},
			})
		}
	}
	return anthropicMessages
}

// 转换为Ollama格式（支持工具消息）
func convertToOllamaFormat(messages []Message) []map[string]interface{} {
	ollamaMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		ollamaMsg := map[string]interface{}{
			"role": msg.Role,
		}
		if msg.Role == "assistant" && msg.ToolCalls != nil {
			ollamaMsg["tool_calls"] = msg.ToolCalls
			if msg.Content != nil {
				ollamaMsg["content"] = msg.Content
			}
		} else if msg.Role == "tool" {
			ollamaMsg["content"] = msg.Content
			// Ollama 可能期望 tool_call_id，但官方文档未明确，暂时只设 content
		} else {
			ollamaMsg["content"] = msg.Content
		}
		ollamaMessages[i] = ollamaMsg
	}
	return ollamaMessages
}

// 转换为OpenAI格式
func convertToOpenAIFormat(messages []Message) []map[string]interface{} {
	openaiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		openaiMsg := map[string]interface{}{
			"role": msg.Role,
		}

		if msg.Role == "tool" {
			openaiMsg["tool_call_id"] = msg.ToolCallID
			openaiMsg["content"] = msg.Content
		} else if msg.Role == "assistant" && msg.ToolCalls != nil {
			openaiMsg["tool_calls"] = msg.ToolCalls
			if msg.Content != nil {
				openaiMsg["content"] = msg.Content
			} else {
				openaiMsg["content"] = nil
			}
		} else {
			openaiMsg["content"] = msg.Content
		}

		openaiMessages[i] = openaiMsg
	}
	return openaiMessages
}

// 准备请求数据
func prepareRequestData(messages []Message, apiType, baseURL, modelID string, temperature float64, maxTokens int, stream bool, thinking bool) (map[string]interface{}, string, error) {
	var data map[string]interface{}
	var endpoint string

	systemPrompt := generateSystemPrompt(apiType)

	switch apiType {
	case "anthropic":
		if baseURL == "" {
			baseURL = ANTHROPIC_BASE_URL
		}
		anthropicMessages := convertToAnthropicFormat(messages)
		data = map[string]interface{}{
			"model":       modelID,
			"system":      systemPrompt,
			"messages":    anthropicMessages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      stream,
		}
		if thinking {
			data["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		}
		endpoint = "/messages"

	case "ollama":
		baseURL = OLLAMA_BASE_URL
		ollamaMessages := convertToOllamaFormat(messages)
		data = map[string]interface{}{
			"model":       modelID,
			"messages":    ollamaMessages,
			"tools":       getTools(apiType), // Ollama 使用与 OpenAI 相同的 tools 格式
			"stream":      stream,
			"system":      systemPrompt,
			"temperature": temperature,
		}
		endpoint = "/chat"

	case "openai":
		if baseURL == "" {
			baseURL = OPENAI_BASE_URL
		}
		openaiMessages := convertToOpenAIFormat(messages)
		data = map[string]interface{}{
			"model":       modelID,
			"messages":    openaiMessages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      stream,
			"system":      systemPrompt,
		}
		if thinking {
			data["extra_body"] = map[string]interface{}{
				"thinking": map[string]interface{}{
					"type": "enabled",
				},
			}
		}
		endpoint = "/chat/completions"

	default:
		return nil, "", fmt.Errorf("unsupported API type: %s", apiType)
	}

	return data, baseURL + endpoint, nil
}

// 发送请求
func sendRequest(data map[string]interface{}, endpoint, apiKey, apiType string) (*http.Response, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		if apiType == "openai" || apiType == "ollama" { // Ollama 可能不需要 key，但保留
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else if apiType == "anthropic" {
			req.Header.Set("x-api-key", apiKey)
		}
	}

	if isDebug {
		fmt.Printf("Sending request to: %s\n", endpoint)
		fmt.Printf("Request data: %v\n", data)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if isDebug {
			fmt.Printf("Error sending request: %v\n", err)
		}
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if isDebug {
			fmt.Printf("Error response status: %d\n", resp.StatusCode)
			fmt.Printf("Error response body: %s\n", string(errorBody))
		}
		return nil, fmt.Errorf("API returned error status: %d, body: %s", resp.StatusCode, string(errorBody))
	}

	return resp, nil
}

// 处理流式响应
func handleStreamResponse(chunkChan <-chan StreamChunk) (Response, error) {
	var fullContent strings.Builder
	var fullReasoningContent strings.Builder
	var finishReason string = "stop"

	type pendingToolCall struct {
		ID       string
		Type     string
		Function struct {
			Name      string
			Arguments strings.Builder
		}
	}
	pendingTools := make(map[int]*pendingToolCall)

	contentReplacer := NewStreamReplacer(func(r rune) {
		fmt.Print(string(r))
		os.Stdout.Sync()
		fullContent.WriteRune(r)
	})

	reasoningReplacer := NewStreamReplacer(func(r rune) {
		fmt.Print(string(r))
		os.Stdout.Sync()
		fullReasoningContent.WriteRune(r)
	})

	for chunk := range chunkChan {
		if chunk.Error != nil {
			return Response{}, chunk.Error
		}

		if chunk.Content != "" {
			contentReplacer.Write(chunk.Content)
		}

		if chunk.ReasoningContent != "" {
			reasoningReplacer.Write(chunk.ReasoningContent)
		}

		if len(chunk.ToolCalls) > 0 {
			for _, tc := range chunk.ToolCalls {
				idxVal, hasIdx := tc["index"]
				if !hasIdx {
					continue
				}
				idx, ok := idxVal.(float64)
				if !ok {
					continue
				}
				intIdx := int(idx)

				pt, exists := pendingTools[intIdx]
				if !exists {
					pt = &pendingToolCall{}
					pendingTools[intIdx] = pt
				}

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
						pt.Function.Arguments.WriteString(args)
					}
				}
			}
		}

		if chunk.Done {
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
			contentReplacer.Flush()
			reasoningReplacer.Flush()
			fmt.Println()
			os.Stdout.Sync()
			break
		}
	}

	if len(pendingTools) > 0 {
		var toolCalls []map[string]interface{}
		// 找出最大索引
		maxIdx := 0
		for idx := range pendingTools {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		// 按索引顺序遍历
		for i := 0; i <= maxIdx; i++ {
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
			Content:          toolCalls,
			StopReason:       finishReason,
			ReasoningContent: fullReasoningContent.String(),
		}, nil
	}

	return Response{
		Content:          fullContent.String(),
		StopReason:       finishReason,
		ReasoningContent: fullReasoningContent.String(),
	}, nil
}

// 处理OpenAI响应
func handleOpenAIResponse(resp *http.Response) (Response, error) {
	var result Response
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
				ReasoningContent interface{} `json:"reasoning_content,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}

	err := json.NewDecoder(resp.Body).Decode(&openaiResp)
	if err != nil {
		return Response{}, fmt.Errorf("failed to decode OpenAI response: %w", err)
	}

	if len(openaiResp.Choices) > 0 {
		choice := openaiResp.Choices[0]
		result.StopReason = choice.FinishReason

		if isDebug {
			messageJson, _ := json.Marshal(choice.Message)
			fmt.Printf("Message structure: %s\n", string(messageJson))
		}

		if content, ok := choice.Message.Content.(string); ok && content != "" {
			fmt.Println(content)
		}

		if len(choice.Message.ToolCalls) > 0 {
			var content []map[string]interface{}
			for _, toolCall := range choice.Message.ToolCalls {
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
			result.StopReason = "function_call"
		} else {
			if choice.Message.FunctionCall.Name != "" {
				var args map[string]interface{}
				json.Unmarshal([]byte(choice.Message.FunctionCall.Arguments), &args)

				toolUse := map[string]interface{}{
					"type":  "function",
					"id":    "1",
					"name":  choice.Message.FunctionCall.Name,
					"input": args,
				}
				result.Content = []map[string]interface{}{toolUse}
				result.StopReason = "function_call"
			} else {
				if contentStr, ok := choice.Message.Content.(string); ok {
					result.Content = applyReplacements(contentStr)
				} else {
					result.Content = choice.Message.Content
				}
				if reasoningStr, ok := choice.Message.ReasoningContent.(string); ok {
					result.ReasoningContent = applyReplacements(reasoningStr)
				} else {
					result.ReasoningContent = choice.Message.ReasoningContent
				}
			}
		}
	}

	return result, nil
}

// 处理Ollama响应
func handleOllamaResponse(resp *http.Response) (Response, error) {
	var result Response
	var ollamaResp struct {
		Message struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
		} `json:"message"`
		Done bool `json:"done"`
	}

	err := json.NewDecoder(resp.Body).Decode(&ollamaResp)
	if err != nil {
		return Response{}, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	if content, ok := ollamaResp.Message.Content.(string); ok && content != "" {
		fmt.Println(content)
	}

	result.Content = ollamaResp.Message.Content
	if contentStr, ok := result.Content.(string); ok {
		result.Content = applyReplacements(contentStr)
	}
	if ollamaResp.Done {
		result.StopReason = "stop"
	} else {
		result.StopReason = "tool_use"
	}

	return result, nil
}

// 处理Anthropic响应
func handleAnthropicResponse(resp *http.Response) (Response, error) {
	var result Response
	var anthropicResp struct {
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text,omitempty"`
			ToolUse struct {
				ID    string                 `json:"id"`
				Name  string                 `json:"name"`
				Input map[string]interface{} `json:"input"`
			} `json:"tool_use,omitempty"`
			Thinking string `json:"thinking,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}

	err := json.NewDecoder(resp.Body).Decode(&anthropicResp)
	if err != nil {
		return Response{}, fmt.Errorf("failed to decode Anthropic response: %w", err)
	}

	var content interface{}
	var hasToolUse bool
	var toolCalls []map[string]interface{}
	var reasoningContent strings.Builder

	for _, item := range anthropicResp.Content {
		if item.Type == "text" && item.Text != "" {
			fmt.Println(item.Text)
			if content == nil {
				content = item.Text
			} else if str, ok := content.(string); ok {
				content = str + "\n" + item.Text
			}
		} else if item.Type == "tool_use" {
			hasToolUse = true
			toolCall := map[string]interface{}{
				"id":   item.ToolUse.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      item.ToolUse.Name,
					"arguments": item.ToolUse.Input,
				},
			}
			toolCalls = append(toolCalls, toolCall)
		} else if item.Type == "thinking" && item.Thinking != "" {
			reasoningContent.WriteString(item.Thinking)
			reasoningContent.WriteString("\n")
		}
	}

	if reasoningContent.Len() > 0 {
		result.ReasoningContent = reasoningContent.String()
	}

	if hasToolUse {
		result.Content = toolCalls
		result.StopReason = "function_call"
	} else {
		result.StopReason = anthropicResp.StopReason
		if str, ok := content.(string); ok {
			result.Content = applyReplacements(str)
		} else {
			result.Content = content
		}
	}

	return result, nil
}

// 处理非流式响应
func handleNonStreamResponse(resp *http.Response, apiType string) (Response, error) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if isDebug {
			fmt.Printf("Error reading response body: %v\n", err)
		}
		return Response{}, fmt.Errorf("failed to read response body: %w", err)
	}

	if isDebug {
		fmt.Printf("Response body: %s\n", string(responseBody))
		debugFile := fmt.Sprintf("debug_response_%d.json", time.Now().Unix())
		if err := os.WriteFile(debugFile, responseBody, 0644); err == nil {
			fmt.Printf("Debug response data written to: %s\n", debugFile)
		}
	}

	r := bytes.NewReader(responseBody)
	resp.Body = io.NopCloser(r)

	switch apiType {
	case "openai":
		return handleOpenAIResponse(resp)
	case "ollama":
		return handleOllamaResponse(resp)
	default:
		return handleAnthropicResponse(resp)
	}
}

// 处理响应
func handleResponse(resp *http.Response, apiType string, stream bool) (Response, error) {
	if stream {
		chunkChan, err := getStreamChunks(resp.Body, apiType) // 傳入 apiType
		if err != nil {
			return Response{}, fmt.Errorf("failed to get stream chunks: %w", err)
		}
		return handleStreamResponse(chunkChan)
	} else {
		return handleNonStreamResponse(resp, apiType)
	}
}

// 调用LLM API
func CallModel(messages []Message, apiType, baseURL, apiKey, modelID string, temperature float64, maxTokens int, stream bool, thinking bool) (Response, error) {
	if apiType == "" {
		apiType = DEFAULT_API_TYPE
	}
	if modelID == "" {
		modelID = DEFAULT_MODEL_ID
	}

	data, endpoint, err := prepareRequestData(messages, apiType, baseURL, modelID, temperature, maxTokens, stream, thinking)
	if err != nil {
		return Response{}, err
	}

	if isDebug {
		debugData, err := json.MarshalIndent(data, "", "  ")
		if err == nil {
			debugFile := fmt.Sprintf("debug_request_%d.json", time.Now().Unix())
			if err := os.WriteFile(debugFile, debugData, 0644); err == nil {
				fmt.Printf("Debug request data written to: %s\n", debugFile)
			}
		}
	}

	resp, err := sendRequest(data, endpoint, apiKey, apiType)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	if isDebug {
		fmt.Printf("Response status code: %d\n", resp.StatusCode)
	}

	return handleResponse(resp, apiType, stream)
}

