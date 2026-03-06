package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	isDebug = false // 控制调试信息的显示
)

// 全局配置变量
var globalConfig Config

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
	var err error
	globalConfig, err = loadConfig()

	// 从配置中获取值
	apiType := globalConfig.APIConfig.APIType
	baseURL := globalConfig.APIConfig.BaseURL
	apiKey := globalConfig.APIConfig.APIKey
	modelID := globalConfig.APIConfig.Model
	temperature := globalConfig.APIConfig.Temperature
	maxTokens := globalConfig.APIConfig.MaxTokens
	stream := globalConfig.APIConfig.Stream
	thinking := globalConfig.APIConfig.Thinking

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

	// 初始化车道锁
	laneLock := &sync.Mutex{}

	// 初始化心跳运行器
	heartbeat := NewHeartbeatRunner(laneLock)
	heartbeat.Start()
	defer heartbeat.Stop()

	// 初始化定时任务服务
	cronService := NewCronService()
	defer cronService.Stop()

	var history []Message
	scanner := bufio.NewScanner(os.Stdin)

	// 打印帮助信息
	printHelp()

	// 打印初始命令提示符
	fmt.Print("GarClaw /> ")

	for {
		// 处理心跳和定时任务的输出
		hasOutput := false

		// 处理心跳输出
		heartbeatMsgs := heartbeat.DrainOutput()
		for _, msg := range heartbeatMsgs {
			fmt.Println() // 先换行
			fmt.Printf("[heartbeat] %s\n", msg)
			hasOutput = true
		}

		// 处理定时任务输出
		cronMsgs := cronService.DrainOutput()
		for _, msg := range cronMsgs {
			fmt.Println() // 先换行
			fmt.Printf("[cron] %s\n", msg)
			hasOutput = true
		}

		// 如果有输出，重新打印命令提示符
		if hasOutput {
			fmt.Print("GarClaw /> ")
		}

		// 处理用户输入
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

		// 处理命令
		if strings.HasPrefix(query, "/") {
			handleCommand(query, heartbeat, cronService, laneLock)
			// 命令执行后重新打印提示符
			fmt.Print("GarClaw /> ")
			continue
		}

		// 正常处理查询
		laneLock.Lock()
		defer laneLock.Unlock()

		history = append(history, Message{
			Role:    "user",
			Content: query,
		})

		AgentLoop(history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
		// 输出逻辑在CallModel函数中实时打印，这里不再重复打印
		// 只打印一个空行作为分隔
		fmt.Println()

		// 处理完查询后重新打印命令提示符
		fmt.Print("GarClaw /> ")
	}
}

// 打印帮助信息
func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  /heartbeat         -- Show heartbeat status")
	fmt.Println("  /trigger           -- Force heartbeat now")
	fmt.Println("  /cron              -- List cron jobs")
	fmt.Println("  /cron-trigger <id> -- Trigger a cron job")
	fmt.Println("  /lanes             -- Show lane lock status")
	fmt.Println("  /help              -- Show this help")
	fmt.Println("  q / exit           -- Exit")
	fmt.Println()
}

// 处理命令
func handleCommand(cmd string, heartbeat *HeartbeatRunner, cronService *CronService, laneLock *sync.Mutex) {
	parts := strings.Split(cmd, " ")
	command := strings.ToLower(parts[0])

	switch command {
	case "/help":
		printHelp()
	case "/heartbeat":
		status := heartbeat.Status()
		for k, v := range status {
			fmt.Printf("  %s: %v\n", k, v)
		}
	case "/trigger":
		result := heartbeat.Trigger()
		fmt.Printf("  %s\n", result)
		// 处理触发后的输出
		for _, msg := range heartbeat.DrainOutput() {
			fmt.Printf("[heartbeat] %s\n", msg)
		}
	case "/cron":
		jobs := cronService.ListJobs()
		if len(jobs) == 0 {
			fmt.Println("  No cron jobs.")
			return
		}
		for _, j := range jobs {
			enabled := "ON"
			if !j["enabled"].(bool) {
				enabled = "OFF"
			}
			errors := j["errors"].(int)
			errorStr := ""
			if errors > 0 {
				errorStr = fmt.Sprintf(" err:%d", errors)
			}
			nextIn := ""
			if j["next_in"] != nil {
				nextIn = fmt.Sprintf(" in %s", j["next_in"])
			}
			fmt.Printf("  [%s] %s - %s%s%s\n", enabled, j["id"], j["name"], errorStr, nextIn)
		}
	case "/cron-trigger":
		if len(parts) < 2 {
			fmt.Println("  Usage: /cron-trigger <job_id>")
			return
		}
		jobID := parts[1]
		result := cronService.TriggerJob(jobID)
		fmt.Printf("  %s\n", result)
		// 处理触发后的输出
		for _, msg := range cronService.DrainOutput() {
			fmt.Printf("[cron] %s\n", msg)
		}
	case "/lanes":
		locked := !laneLock.TryLock()
		if !locked {
			laneLock.Unlock()
		}
		fmt.Printf("  main_locked: %v  heartbeat_running: %v\n", locked, false)
	default:
		fmt.Printf("  Unknown command: %s\n", command)
		printHelp()
	}
}
