package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 创建 HTTP 客户端
func createHTTPClient(timeout int) *http.Client {
	return &http.Client{
		Timeout: time.Duration(timeout) * time.Minute,
	}
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

// 自动搜索相关记忆
func autoRecallMemories(query string) string {
	// 搜索MEMORY.md文件
	memPath := "workspace/MEMORY.md"
	text := ""
	if mem, err := os.ReadFile(memPath); err == nil && len(mem) > 0 {
		text = string(mem)
	}

	// 搜索每日JSONL文件
	memoryDir := "workspace/memory/daily"
	// 确保memory目录存在
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		// 目录创建失败，只搜索MEMORY.md
		matches := []string{}
		// 搜索MEMORY.md
		if text != "" {
			for _, line := range strings.Split(text, "\n") {
				if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
					matches = append(matches, fmt.Sprintf("[MEMORY.md] %s", line))
				}
			}
		}
		// 限制返回结果数量
		maxMatches := 3
		if len(matches) > maxMatches {
			matches = matches[:maxMatches]
		}
		if len(matches) > 0 {
			return "\n\n### 相关记忆\n" + strings.Join(matches, "\n")
		}
		return ""
	}
	matches := []string{}

	// 搜索MEMORY.md
	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
				matches = append(matches, fmt.Sprintf("[MEMORY.md] %s", line))
			}
		}
	}

	// 搜索每日JSONL文件
	if _, err := os.Stat(memoryDir); err == nil {
		files, err := os.ReadDir(memoryDir)
		if err == nil {
			for _, file := range files {
				if strings.HasSuffix(file.Name(), ".jsonl") {
					filePath := filepath.Join(memoryDir, file.Name())
					data, err := os.ReadFile(filePath)
					if err == nil {
						lines := strings.Split(string(data), "\n")
						for _, line := range lines {
							if line == "" {
								continue
							}
							var entry map[string]interface{}
							if err := json.Unmarshal([]byte(line), &entry); err == nil {
								if content, ok := entry["content"].(string); ok {
									if strings.Contains(strings.ToLower(content), strings.ToLower(query)) {
										matches = append(matches, fmt.Sprintf("[%s] %s", file.Name(), content))
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 限制返回结果数量
	maxMatches := 3
	if len(matches) > maxMatches {
		matches = matches[:maxMatches]
	}

	if len(matches) > 0 {
		return "\n\n### 相关记忆\n" + strings.Join(matches, "\n")
	}

	return ""
}

// 生成系统提示
func generateSystemPrompt(apiType string, userQuery string) string {
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	toolOrFunction := "tool"
	if apiType == "openai" {
		toolOrFunction = "function"
	}

	// 自动搜索相关记忆
	memoryContext := autoRecallMemories(userQuery)

	return fmt.Sprintf("当前系统时间：%s\n", currentTime) + strings.ReplaceAll(SYSTEM_PROMPT, "{{tool_or_function}}", toolOrFunction) + memoryContext
}

// 转换为Ollama格式
func convertToOllamaFormat(messages []Message) []map[string]interface{} {
	ollamaMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
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

		openaiMessages[i] = openaiMsg
	}
	return openaiMessages
}

// 准备请求数据
func prepareRequestData(messages []Message, apiType, baseURL, modelID string, temperature float64, maxTokens int, stream bool, thinking bool) (map[string]interface{}, string, error) {
	var data map[string]interface{}
	var endpoint string

	// 提取用户查询
	userQuery := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			if content, ok := messages[i].Content.(string); ok {
				userQuery = content
				break
			}
		}
	}

	// 生成系统提示
	systemPrompt := generateSystemPrompt(apiType, userQuery)

	switch apiType {
	case "anthropic":
		// 如果baseURL为空，使用默认值
		if baseURL == "" {
			baseURL = ANTHROPIC_BASE_URL
		}
		data = map[string]interface{}{
			"model":       modelID,
			"system":      systemPrompt,
			"messages":    messages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      stream,
		}
		// 添加thinking参数（Anthropic格式）
		if thinking {
			data["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		}
		endpoint = "/messages"

	case "ollama":
		// Ollama使用固定的baseURL
		baseURL = OLLAMA_BASE_URL
		// 转换messages格式为Ollama格式
		ollamaMessages := convertToOllamaFormat(messages)
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
		openaiMessages := convertToOpenAIFormat(messages)
		data = map[string]interface{}{
			"model":       modelID,
			"messages":    openaiMessages,
			"tools":       getTools(apiType),
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"stream":      stream, // 启用流式
			"system":      systemPrompt,
		}
		// 添加thinking参数（OpenAI格式，使用extra_body）
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
func sendRequest(data map[string]interface{}, endpoint, apiKey, apiType string, timeout int) (*http.Response, error) {
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
		if apiType == "openai" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else if apiType == "anthropic" {
			req.Header.Set("x-api-key", apiKey)
		}
	}

	if isDebug {
		fmt.Printf("Sending request to: %s\n", endpoint)
		fmt.Printf("Request data: %v\n", data)
	}

	// 使用配置的超时时间创建HTTP客户端
	httpClient := createHTTPClient(timeout)
	resp, err := httpClient.Do(req)
	if err != nil {
		if isDebug {
			fmt.Printf("Error sending request: %v\n", err)
		}
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		// 读取错误响应体
		errorBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close() // 关闭响应体
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

	// 创建内容替换器
	contentReplacer := NewStreamReplacer(func(r rune) {
		fmt.Print(string(r))
		os.Stdout.Sync()
		fullContent.WriteRune(r)
	})

	// 创建思考内容替换器
	reasoningReplacer := NewStreamReplacer(func(r rune) {
		fmt.Print(string(r))
		os.Stdout.Sync()
		fullReasoningContent.WriteRune(r)
	})

	for chunk := range chunkChan {
		if chunk.Error != nil {
			return Response{}, chunk.Error
		}

		// 处理文本内容
		if chunk.Content != "" {
			contentReplacer.Write(chunk.Content)
		}

		// 处理思考内容
		if chunk.ReasoningContent != "" {
			reasoningReplacer.Write(chunk.ReasoningContent)
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
			// 刷新替换器，输出缓冲区剩余内容
			contentReplacer.Flush()
			reasoningReplacer.Flush()
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
			Content:          toolCalls,
			StopReason:       finishReason,
			ReasoningContent: fullReasoningContent.String(),
		}, nil
	}

	// 没有工具调用，返回普通文本
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

		// 打印message的完整结构，用于调试
		if isDebug {
			messageJson, _ := json.Marshal(choice.Message)
			fmt.Printf("Message structure: %s\n", string(messageJson))
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
				// 纯文本回复，应用替换
				if contentStr, ok := choice.Message.Content.(string); ok {
					result.Content = applyReplacements(contentStr)
				} else {
					result.Content = choice.Message.Content
				}
				// 保存思考内容并应用替换
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

	// 处理响应内容
	var content interface{}
	var hasToolUse bool
	var toolCalls []map[string]interface{}
	var reasoningContent strings.Builder

	for _, item := range anthropicResp.Content {
		if item.Type == "text" && item.Text != "" {
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
			// 处理思考内容
			reasoningContent.WriteString(item.Thinking)
			reasoningContent.WriteString("\n")
		}
	}

	// 保存思考内容
	if reasoningContent.Len() > 0 {
		result.ReasoningContent = reasoningContent.String()
	}

	if hasToolUse {
		result.Content = toolCalls
		result.StopReason = "function_call"
	} else {
		result.StopReason = anthropicResp.StopReason
		// 对文本内容应用替换
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
	// 读取响应体
	var responseBody []byte
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if isDebug {
			fmt.Printf("Error reading response body: %v\n", err)
		}
		return Response{}, fmt.Errorf("failed to read response body: %w", err)
	}

	// 打印响应体
	if isDebug {
		fmt.Printf("Response body: %s\n", string(responseBody))
		// 调试模式：将响应数据写入本地文件
		debugFile := fmt.Sprintf("debug_response_%d.json", time.Now().Unix())
		if err := os.WriteFile(debugFile, responseBody, 0644); err == nil {
			fmt.Printf("Debug response data written to: %s\n", debugFile)
		}
	}

	// 重置响应体，以便后续解码
	r := bytes.NewReader(responseBody)
	resp.Body = io.NopCloser(r)

	// 处理不同API的响应格式
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
	// 处理流式输出
	if stream {
		// 获取流式响应通道
		chunkChan, err := getStreamChunks(resp.Body, apiType)
		if err != nil {
			return Response{}, fmt.Errorf("failed to get stream chunks: %w", err)
		}

		return handleStreamResponse(chunkChan)
	} else {
		// 处理非流式输出
		return handleNonStreamResponse(resp, apiType)
	}
}

// 调用LLM API
func CallModel(messages []Message, apiType, baseURL, apiKey, modelID string, temperature float64, maxTokens int, stream bool, thinking bool) (Response, error) {
	// 确保有默认值
	if apiType == "" {
		apiType = DEFAULT_API_TYPE
	}
	if modelID == "" {
		modelID = DEFAULT_MODEL_ID
	}

	// 准备请求数据
	data, endpoint, err := prepareRequestData(messages, apiType, baseURL, modelID, temperature, maxTokens, stream, thinking)
	if err != nil {
		return Response{}, err
	}

	// 调试模式：将请求数据写入本地文件
	if isDebug {
		debugData, err := json.MarshalIndent(data, "", "  ")
		if err == nil {
			debugFile := fmt.Sprintf("debug_request_%d.json", time.Now().Unix())
			if err := os.WriteFile(debugFile, debugData, 0644); err == nil {
				fmt.Printf("Debug request data written to: %s\n", debugFile)
			}
		}
	}

	// 发送请求
	resp, err := sendRequest(data, endpoint, apiKey, apiType, globalConfig.APIConfig.Timeout)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	// 打印响应状态码
	if isDebug {
		fmt.Printf("Response status code: %d\n", resp.StatusCode)
	}

	// 处理响应
	return handleResponse(resp, apiType, stream)
}
