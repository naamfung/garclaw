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
	sessionFile string
	maxMessages int
	summaryThreshold int
}

// 会话消息
type SessionMessage struct {
	Role      string      `json:"role"`
	Content   interface{} `json:"content"`
	ToolCalls interface{} `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Timestamp time.Time    `json:"timestamp"`
}

// 初始化会话管理器
func NewSessionManager(sessionFile string, maxMessages, summaryThreshold int) *SessionManager {
	// 确保目录存在
	dir := filepath.Dir(sessionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating session directory: %v\n", err)
	}

	return &SessionManager{
		sessionFile: sessionFile,
		maxMessages: maxMessages,
		summaryThreshold: summaryThreshold,
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

	// 解析 JSONL 文件
	var sessionMessages []SessionMessage
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

		sessionMessages = append(sessionMessages, msg)
	}

	// 转换为 Message 类型
	messages := make([]Message, len(sessionMessages))
	for i, msg := range sessionMessages {
		messages[i] = Message{
			Role:      msg.Role,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
	}

	return messages
}

// 保存会话消息
func (sm *SessionManager) SaveMessage(msg Message) error {
	// 创建会话消息
	sessionMsg := SessionMessage{
		Role:      msg.Role,
		Content:   msg.Content,
		ToolCalls: msg.ToolCalls,
		ToolCallID: msg.ToolCallID,
		Timestamp: time.Now(),
	}

	// 序列化消息
	data, err := json.Marshal(sessionMsg)
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
			summaryPrompt += fmt.Sprintf("User: %v\n", msg.Content)
		case "assistant":
			summaryPrompt += fmt.Sprintf("Assistant: %v\n", msg.Content)
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
