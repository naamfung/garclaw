package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// 配置
const (
	DEFAULT_API_TYPE       = "openai" // 可选值: anthropic, ollama, openai
	ANTHROPIC_BASE_URL     = "https://api.anthropic.com/v1"
	OLLAMA_BASE_URL        = "http://localhost:11434/api"
	OPENAI_BASE_URL        = "https://api.openai.com/v1"
	DEFAULT_MODEL_ID       = "claude-3-opus-20240229"
	CONFIG_FILE            = "config.toon"
	isDebug                = true // 控制调试信息的显示
	SYSTEM_PROMPT_TEMPLATE = "You are a coding agent. When the user asks to list files, run commands, or interact with the system, you MUST use the shell {{tool_or_function}}. When you need to read a specific line from a file, use the read_file_line {{tool_or_function}}. When you need to write content to a specific line in a file, use the write_file_line {{tool_or_function}}. When you need to read all lines from a file, use the read_all_lines {{tool_or_function}}. When you need to write all lines to a file, use the write_all_lines {{tool_or_function}}. When you need to manage tasks, use the todo {{tool_or_function}}. IMPORTANT: The current system time is provided at the end of this prompt. When asked about the current date or time, you MUST use this provided time information directly and NOT attempt to execute any commands to get the date or time. When you need to search for time-sensitive information like news, you MUST use this current system time to construct your search query. Do NOT explain how to run the command, do NOT provide alternative methods, just use the {{tool_or_function}} directly. For example, when asked to list files, use the shell {{tool_or_function}} with command 'ls' or 'ls -la' (Unix/Linux). Your response MUST be a {{tool_or_function}} call, not a regular message. Under no circumstances should you provide explanations or instructions to the user - only use the {{tool_or_function}}."
)

// TruncateString 安全地截断 UTF-8 字符串，确保不会在字符中间切断
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	// 确保我们不会在 UTF-8 字符的中间截断
	for i := maxLen; i > 0; i-- {
		if utf8.RuneStart(s[i]) {
			return s[:i] + "..."
		}
	}

	return "..."
}

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
