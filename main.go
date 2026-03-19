package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	isDebug = false // 控制调试信息的显示
)

// 消息结构
type Message struct {
	Role             string      `json:"role"`
	Content          interface{} `json:"content,omitempty"`
	ToolCalls        interface{} `json:"tool_calls,omitempty"`
	ToolCallID       string      `json:"tool_call_id,omitempty"`
	ReasoningContent interface{} `json:"reasoning_content,omitempty"`
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
	Content          interface{} `json:"content"`
	StopReason       string      `json:"stop_reason"`
	ReasoningContent interface{} `json:"reasoning_content,omitempty"`
}

func main() {
	// 读取配置文件
	config, err := loadConfig()

	// 从配置中获取值
	apiType := config.APIConfig.APIType
	baseURL := config.APIConfig.BaseURL
	apiKey := config.APIConfig.APIKey
	modelID := config.APIConfig.Model
	temperature := config.APIConfig.Temperature
	maxTokens := config.APIConfig.MaxTokens
	stream := config.APIConfig.Stream
	thinking := config.APIConfig.Thinking

	if err != nil {
		fmt.Printf("Warning: Error loading config file: %v\n", err)
		fmt.Println("Using environment variables for configuration")
	} else {
		fmt.Println("Configuration loaded from config.toon")
		if isDebug {
			fmt.Printf("API type: %s\n", apiType)
		}
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

		AgentLoop(history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
		// 输出逻辑在CallModel函数中实时打印，这里不再重复打印
		// 只打印一个空行作为分隔
		fmt.Println()
	}
}
