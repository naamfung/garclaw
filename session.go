package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 会话管理器
type SessionManager struct {
	sessionFile      string
	indexFile        string
	sessions         map[string]SessionMeta
	maxMessages      int
	summaryThreshold int
	currentSessionID string
}

// 会话消息类型
const (
	MessageTypeUser       = "user"
	MessageTypeAssistant  = "assistant"
	MessageTypeToolUse    = "tool_use"
	MessageTypeToolResult = "tool_result"
)

// 会话消息
type SessionMessage struct {
	Type      string      `json:"type"`
	Content   interface{} `json:"content"`
	ToolUseID string      `json:"tool_use_id,omitempty"`
	ToolName  string      `json:"name,omitempty"`
	ToolInput interface{} `json:"input,omitempty"`
	Timestamp int64       `json:"ts"` // Unix 时间戳
}

// 会话元数据
type SessionMeta struct {
	Label        string `json:"label"`
	CreatedAt    string `json:"created_at"`
	LastActive   string `json:"last_active"`
	MessageCount int    `json:"message_count"`
}

// 初始化会话管理器
func NewSessionManager(sessionFile string, maxMessages, summaryThreshold int) *SessionManager {
	// 确保目录存在
	dir := filepath.Dir(sessionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating session directory: %v\n", err)
	}

	// 会话索引文件
	indexFile := filepath.Join(dir, "sessions.json")

	// 加载会话索引
	sessions := loadSessionIndex(indexFile)

	// 选择当前会话
	currentSessionID := "default"
	if len(sessions) == 0 {
		// 创建默认会话
		sessions[currentSessionID] = SessionMeta{
			Label:        "Default Session",
			CreatedAt:    time.Now().Format(time.RFC3339),
			LastActive:   time.Now().Format(time.RFC3339),
			MessageCount: 0,
		}
		saveSessionIndex(indexFile, sessions)
	}

	return &SessionManager{
		sessionFile:      sessionFile,
		indexFile:        indexFile,
		sessions:         sessions,
		maxMessages:      maxMessages,
		summaryThreshold: summaryThreshold,
		currentSessionID: currentSessionID,
	}
}

// 加载会话索引
func loadSessionIndex(indexFile string) map[string]SessionMeta {
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		return make(map[string]SessionMeta)
	}

	content, err := os.ReadFile(indexFile)
	if err != nil {
		fmt.Printf("Error reading session index: %v\n", err)
		return make(map[string]SessionMeta)
	}

	var sessions map[string]SessionMeta
	if err := json.Unmarshal(content, &sessions); err != nil {
		fmt.Printf("Error unmarshaling session index: %v\n", err)
		return make(map[string]SessionMeta)
	}

	return sessions
}

// 保存会话索引
func saveSessionIndex(indexFile string, sessions map[string]SessionMeta) {
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling session index: %v\n", err)
		return
	}

	if err := os.WriteFile(indexFile, data, 0644); err != nil {
		fmt.Printf("Error writing session index: %v\n", err)
	}
}

// 加载会话历史
func (sm *SessionManager) LoadHistory() []Message {
	// 检查文件是否存在
	if _, err := os.Stat(sm.sessionFile); os.IsNotExist(err) {
		return []Message{}
	}

	// 读取文件内容
	content, err := os.ReadFile(sm.sessionFile)
	if err != nil {
		fmt.Printf("Error reading session file: %v\n", err)
		return []Message{}
	}

	// 解析 JSONL 文件并重建消息历史
	return sm._rebuildHistory(content)
}

// 从 JSONL 内容重建 API 格式的消息列表
func (sm *SessionManager) _rebuildHistory(content []byte) []Message {
	messages := []Message{}
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var msg SessionMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			fmt.Printf("Error unmarshaling session message: %v\n", err)
			continue
		}

		switch msg.Type {
		case MessageTypeUser:
			messages = append(messages, Message{
				Role:    "user",
				Content: msg.Content,
			})

		case MessageTypeAssistant:
			content := msg.Content
			if contentStr, ok := content.(string); ok {
				content = []map[string]interface{}{
					{
						"type": "text",
						"text": contentStr,
					},
				}
			}
			messages = append(messages, Message{
				Role:    "assistant",
				Content: content,
			})

		case MessageTypeToolUse:
			toolUseBlock := map[string]interface{}{
				"type":  "tool_use",
				"id":    msg.ToolUseID,
				"name":  msg.ToolName,
				"input": msg.ToolInput,
			}

			// 将工具调用添加到最后一条助手消息中
			if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
				if content, ok := messages[len(messages)-1].Content.([]map[string]interface{}); ok {
					messages[len(messages)-1].Content = append(content, toolUseBlock)
				} else {
					// 如果助手消息内容不是列表，转换为列表
					messages[len(messages)-1].Content = []map[string]interface{}{
						{
							"type": "text",
							"text": fmt.Sprintf("%v", messages[len(messages)-1].Content),
						},
						toolUseBlock,
					}
				}
			} else {
				// 如果没有助手消息，创建一个新的
				messages = append(messages, Message{
					Role:    "assistant",
					Content: []map[string]interface{}{toolUseBlock},
				})
			}

		case MessageTypeToolResult:
			toolResultBlock := map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": msg.ToolUseID,
				"content":     msg.Content,
			}

			// 将工具结果添加到最后一条用户消息中
			if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
				if content, ok := messages[len(messages)-1].Content.([]map[string]interface{}); ok {
					messages[len(messages)-1].Content = append(content, toolResultBlock)
				} else {
					// 如果用户消息内容不是列表，转换为列表
					messages[len(messages)-1].Content = []map[string]interface{}{
						{
							"type": "text",
							"text": fmt.Sprintf("%v", messages[len(messages)-1].Content),
						},
						toolResultBlock,
					}
				}
			} else {
				// 如果没有用户消息，创建一个新的
				messages = append(messages, Message{
					Role:    "user",
					Content: []map[string]interface{}{toolResultBlock},
				})
			}
		}
	}

	return messages
}

// 保存会话消息
func (sm *SessionManager) SaveMessage(msg Message) error {
	// 根据消息类型创建会话消息
	var sessionMsg SessionMessage
	switch msg.Role {
	case "user":
		sessionMsg = SessionMessage{
			Type:      MessageTypeUser,
			Content:   msg.Content,
			Timestamp: time.Now().Unix(),
		}
	case "assistant":
		// 检查是否包含工具调用
		if toolCalls, ok := msg.ToolCalls.([]interface{}); ok && len(toolCalls) > 0 {
			// 保存助手消息文本（如果有）
			if msg.Content != nil {
				sessionMsg = SessionMessage{
					Type:      MessageTypeAssistant,
					Content:   msg.Content,
					Timestamp: time.Now().Unix(),
				}
				// 序列化并写入消息
				if err := sm.writeSessionMessage(sessionMsg); err != nil {
					return err
				}
			}

			// 保存工具调用
			for _, toolCall := range toolCalls {
				if tc, ok := toolCall.(map[string]interface{}); ok {
					toolUseID := ""
					if id, ok := tc["id"].(string); ok {
						toolUseID = id
					}
					toolName := ""
					if name, ok := tc["name"].(string); ok {
						toolName = name
					} else if function, ok := tc["function"].(map[string]interface{}); ok {
						if name, ok := function["name"].(string); ok {
							toolName = name
						}
					}
					toolInput := map[string]interface{}{}
					if input, ok := tc["input"].(map[string]interface{}); ok {
						toolInput = input
					} else if function, ok := tc["function"].(map[string]interface{}); ok {
						if args, ok := function["arguments"].(string); ok {
							var argsMap map[string]interface{}
							if err := json.Unmarshal([]byte(args), &argsMap); err == nil {
								toolInput = argsMap
							}
						}
					}

					toolUseMsg := SessionMessage{
						Type:      MessageTypeToolUse,
						ToolUseID: toolUseID,
						ToolName:  toolName,
						ToolInput: toolInput,
						Timestamp: time.Now().Unix(),
					}
					if err := sm.writeSessionMessage(toolUseMsg); err != nil {
						return err
					}
				}
			}
			return nil
		} else {
			sessionMsg = SessionMessage{
				Type:      MessageTypeAssistant,
				Content:   msg.Content,
				Timestamp: time.Now().Unix(),
			}
		}
	case "tool":
		sessionMsg = SessionMessage{
			Type:      MessageTypeToolResult,
			Content:   msg.Content,
			ToolUseID: msg.ToolCallID,
			Timestamp: time.Now().Unix(),
		}
	default:
		return fmt.Errorf("unknown message role: %s", msg.Role)
	}

	// 写入消息
	if err := sm.writeSessionMessage(sessionMsg); err != nil {
		return err
	}

	// 更新会话元数据
	sm.updateSessionMeta()

	return nil
}

// 写入会话消息到文件
func (sm *SessionManager) writeSessionMessage(msg SessionMessage) error {
	// 序列化消息
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal session message: %w", err)
	}

	// 追加到文件
	f, err := os.OpenFile(sm.sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(string(data) + "\n")
	if err != nil {
		return fmt.Errorf("failed to write session message: %w", err)
	}

	return nil
}

// 更新会话元数据
func (sm *SessionManager) updateSessionMeta() {
	if meta, ok := sm.sessions[sm.currentSessionID]; ok {
		meta.LastActive = time.Now().Format(time.RFC3339)
		meta.MessageCount++
		sm.sessions[sm.currentSessionID] = meta
		saveSessionIndex(sm.indexFile, sm.sessions)
	}
}

// 清理会话文件
func (sm *SessionManager) ClearSession() error {
	if err := os.Remove(sm.sessionFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear session: %w", err)
	}
	return nil
}

// 检查并处理上下文溢出
func (sm *SessionManager) CheckOverflow(messages []Message) ([]Message, error) {
	// 检查消息数量是否超过阈值
	if len(messages) <= sm.maxMessages {
		return messages, nil
	}

	// 需要总结的消息数量
	messagesToSummarize := len(messages) - sm.summaryThreshold
	if messagesToSummarize <= 0 {
		return messages, nil
	}

	// 提取需要总结的消息
	messagesToSummarizeSlice := messages[:messagesToSummarize]

	// 构建总结提示
	summaryPrompt := "Please summarize the following conversation history in a concise way. Focus on the key points and important information.\n\n"
	for _, msg := range messagesToSummarizeSlice {
		switch msg.Role {
		case "user":
			if contentStr, ok := msg.Content.(string); ok {
				summaryPrompt += fmt.Sprintf("User: %s\n", contentStr)
			} else if contentList, ok := msg.Content.([]map[string]interface{}); ok {
				for _, block := range contentList {
					if block["type"] == "text" {
						summaryPrompt += fmt.Sprintf("User: %s\n", block["text"])
					} else if block["type"] == "tool_result" {
						summaryPrompt += fmt.Sprintf("Tool Result: %s\n", block["content"])
					}
				}
			}
		case "assistant":
			if contentStr, ok := msg.Content.(string); ok {
				summaryPrompt += fmt.Sprintf("Assistant: %s\n", contentStr)
			} else if contentList, ok := msg.Content.([]map[string]interface{}); ok {
				for _, block := range contentList {
					if block["type"] == "text" {
						summaryPrompt += fmt.Sprintf("Assistant: %s\n", block["text"])
					} else if block["type"] == "tool_use" {
						summaryPrompt += fmt.Sprintf("Assistant called tool: %s\n", block["name"])
					}
				}
			}
		case "tool":
			summaryPrompt += fmt.Sprintf("Tool: %v\n", msg.Content)
		}
	}

	// 使用全局配置
	config := globalConfig

	// 构建消息
	summaryMessages := []Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant that summarizes conversation histories.",
		},
		{
			Role:    "user",
			Content: summaryPrompt,
		},
	}

	// 调用模型进行总结
	response, err := CallModel(
		summaryMessages,
		config.APIConfig.APIType,
		config.APIConfig.BaseURL,
		config.APIConfig.APIKey,
		config.APIConfig.Model,
		config.APIConfig.Temperature,
		config.APIConfig.MaxTokens,
		false, // 禁用流式输出
		config.APIConfig.Thinking,
	)
	if err != nil {
		return messages, fmt.Errorf("failed to generate summary: %w", err)
	}

	// 提取总结内容
	summary := ""
	if content, ok := response.Content.(string); ok {
		summary = content
	} else {
		summary = "Previous conversation summarized."
	}

	// 构建新的消息列表
	newMessages := []Message{
		{
			Role:    "system",
			Content: fmt.Sprintf("This is a summary of previous conversation:\n%s", summary),
		},
	}

	// 添加剩余的消息
	newMessages = append(newMessages, messages[messagesToSummarize:]...)

	// 清空会话文件
	if err := sm.ClearSession(); err != nil {
		return messages, fmt.Errorf("failed to clear session: %w", err)
	}

	// 保存新的消息到会话文件
	for _, msg := range newMessages {
		if err := sm.SaveMessage(msg); err != nil {
			return messages, fmt.Errorf("failed to save message: %w", err)
		}
	}

	return newMessages, nil
}
