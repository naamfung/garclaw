package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chzyer/readline"
)

// 全局 API 配置变量，供其他包使用
var (
	apiType     string
	baseURL     string
	apiKey      string
	modelID     string
	temperature float64
	maxTokens   int
	stream      bool
	thinking    bool
)

// 全局变量：是否拦截危险命令
var BlockDangerousCommands bool

// 调试开关，全局可见
var IsDebug = false

func main() {
	// 加载配置
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
		fmt.Println("Please edit the generated config file and restart the program.")
		os.Exit(0) // 生成默认配置后直接退出
	}

	// 从配置中赋值全局变量
	apiType = config.APIConfig.APIType
	baseURL = config.APIConfig.BaseURL
	apiKey = config.APIConfig.APIKey
	modelID = config.APIConfig.Model
	temperature = config.APIConfig.Temperature
	maxTokens = config.APIConfig.MaxTokens
	stream = config.APIConfig.Stream
	thinking = config.APIConfig.Thinking
	BlockDangerousCommands = config.APIConfig.BlockDangerousCommands

	fmt.Printf("Using model: %s\n", modelID)
	if !BlockDangerousCommands {
		fmt.Println("Dangerous command blocking is DISABLED. The model can execute any command.")
	}

	// 启动 HTTP 服务器（如果配置了监听地址）
	if config.HTTPServer.Listen != "" {
		httpServer := NewHTTPServer(config.HTTPServer.Listen)
		go func() {
			httpServer.Start()
		}()
	}

	// 启动邮件轮询（如果配置了邮件）
	var emailPoller *EmailPoller
	if config.EmailConfig != nil {
		emailPoller = &EmailPoller{config: config.EmailConfig, stop: make(chan struct{})}
		emailPoller.Start()
		log.Println("Email polling started")
	}

	// 命令行界面（使用 readline）
	rl, err := readline.New("GarClaw /> ")
	if err != nil {
		log.Fatalf("Failed to create readline: %v", err)
	}
	defer rl.Close()

	cmdChan := NewCmdChannel()
	var history []Message

	// 捕获 Ctrl+C 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		if emailPoller != nil {
			emailPoller.Stop()
		}
		os.Exit(0)
	}()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == io.EOF {
				break // Ctrl+D 正常退出
			}
			fmt.Printf("Readline error: %v\n", err)
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 检查是否为命令（以 / 开头）
		if strings.HasPrefix(line, "/") {
			lowerCmd := strings.ToLower(line)
			if lowerCmd == "/exit" {
				break
			}
			// 其他以 / 开头的命令暂不处理，作为普通消息
		}

		// 普通用户输入，加入历史并调用 AgentLoop
		history = append(history, Message{Role: "user", Content: line})
		newHistory, err := AgentLoop(cmdChan, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
		if err != nil {
			fmt.Printf("Agent error: %v\n", err)
		} else {
			history = newHistory
		}
		fmt.Println() // 分隔
	}
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