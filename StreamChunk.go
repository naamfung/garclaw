package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamChunk 流式响应块
type StreamChunk struct {
	Content          string
	ToolCalls        []map[string]interface{} // 用于存放工具调用（可能是多个，每个可能不完整）
	Done             bool
	Error            error
	FinishReason     string
	ReasoningContent string
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

				// 处理 OpenAI 格式（包括深度求索的 OpenAI 兼容 API）
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
