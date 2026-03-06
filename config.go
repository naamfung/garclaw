package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/toon-format/toon-go"
)

// 加载 .env 文件
func loadEnv() {
	// 检查 .env 文件是否存在
	envPath := ".env"
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		// .env 文件不存在，尝试在工作目录中查找
		execPath, err := os.Executable()
		if err == nil {
			execDir := filepath.Dir(execPath)
			envPath = filepath.Join(execDir, ".env")
		}
	}

	// 读取 .env 文件
	file, err := os.Open(envPath)
	if err != nil {
		// .env 文件不存在，不做处理
		return
	}
	defer file.Close()

	// 扫描文件内容
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// 跳过空行和注释
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析键值对
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除注释部分
		if commentIndex := strings.Index(value, "#"); commentIndex != -1 {
			value = strings.TrimSpace(value[:commentIndex])
		}

		// 移除引号
		value = strings.Trim(value, `"'`)

		// 设置环境变量
		os.Setenv(key, value)
	}

	if scanner.Err() != nil {
		fmt.Printf("Error scanning .env file: %v\n", scanner.Err())
	}
}

// 配置
const (
	DEFAULT_API_TYPE   = "openai" // 可选值: anthropic, ollama, openai
	ANTHROPIC_BASE_URL = "https://api.anthropic.com/v1"
	OLLAMA_BASE_URL    = "http://localhost:11434/api"
	OPENAI_BASE_URL    = "https://api.openai.com/v1"
	DEFAULT_MODEL_ID   = "claude-3-opus-20240229"
	CONFIG_FILE        = "config.toon"
)

// 配置结构体
type Config struct {
	APIConfig struct {
		APIType     string  `json:"api_type"`
		BaseURL     string  `json:"base_url"`
		APIKey      string  `json:"api_key"`
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		Stream      bool    `json:"stream"`
		Thinking    bool    `json:"thinking"`
		Timeout     int     `json:"timeout"` // 超时时间（分钟）
	} `json:"api_config"`
}

// 读取配置文件
func loadConfig() (Config, error) {
	var config Config

	// 获取程序自身路径
	execPath, err := os.Executable()
	if err != nil {
		return config, fmt.Errorf("error getting executable path: %v", err)
	}

	// 获取程序所在目录
	execDir := filepath.Dir(execPath)

	// 拼接配置文件路径
	configPath := filepath.Join(execDir, CONFIG_FILE)

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 生成默认配置
		defaultConfig := Config{}
		defaultConfig.APIConfig.APIType = DEFAULT_API_TYPE
		defaultConfig.APIConfig.Model = DEFAULT_MODEL_ID
		defaultConfig.APIConfig.Temperature = 0.7
		defaultConfig.APIConfig.MaxTokens = 4096
		defaultConfig.APIConfig.Stream = true
		defaultConfig.APIConfig.Thinking = false

		// 直接序列化为 TOON 格式
		toonData, err := toon.Marshal(defaultConfig)
		if err == nil {
			// 写入默认配置文件
			err = os.WriteFile(configPath, toonData, 0644)
			if err == nil {
				fmt.Printf("Generated default config file at: %s\n", configPath)
			}
		}

		return config, fmt.Errorf("error reading config file: %v", err)
	}

	// 解析TOON格式
	parsed, err := toon.Decode(data)
	if err != nil {
		return config, fmt.Errorf("error parsing TOON config: %v", err)
	}

	// 打印解析结果，用于调试
	if isDebug {
		fmt.Printf("Parsed config: %v\n", parsed)
	}

	// 手动解析配置
	var apiConfig interface{}
	var ok bool

	// 尝试解析小写形式的 api_config
	if apiConfig, ok = parsed.(map[string]interface{})["api_config"]; !ok {
		// 如果失败，尝试解析大写形式的 APIConfig
		apiConfig, ok = parsed.(map[string]interface{})["APIConfig"]
	}

	if ok {
		if apiConfigMap, ok := apiConfig.(map[string]interface{}); ok {
			// 尝试解析小写形式的字段
			if apiType, ok := apiConfigMap["api_type"].(string); ok {
				config.APIConfig.APIType = apiType
			} else if apiType, ok := apiConfigMap["APIType"].(string); ok {
				// 如果失败，尝试解析大写形式的字段
				config.APIConfig.APIType = apiType
			}

			if baseURL, ok := apiConfigMap["base_url"].(string); ok {
				config.APIConfig.BaseURL = baseURL
			} else if baseURL, ok := apiConfigMap["BaseURL"].(string); ok {
				config.APIConfig.BaseURL = baseURL
			}

			if apiKey, ok := apiConfigMap["api_key"].(string); ok {
				config.APIConfig.APIKey = apiKey
			} else if apiKey, ok := apiConfigMap["APIKey"].(string); ok {
				config.APIConfig.APIKey = apiKey
			}

			if model, ok := apiConfigMap["model"].(string); ok {
				config.APIConfig.Model = model
			} else if model, ok := apiConfigMap["Model"].(string); ok {
				config.APIConfig.Model = model
			}

			if temperature, ok := apiConfigMap["temperature"].(float64); ok {
				config.APIConfig.Temperature = temperature
			} else if temperature, ok := apiConfigMap["Temperature"].(float64); ok {
				config.APIConfig.Temperature = temperature
			}

			if maxTokens, ok := apiConfigMap["max_tokens"].(float64); ok {
				config.APIConfig.MaxTokens = int(maxTokens)
			} else if maxTokens, ok := apiConfigMap["MaxTokens"].(float64); ok {
				config.APIConfig.MaxTokens = int(maxTokens)
			}

			// 设置默认值为 true
			config.APIConfig.Stream = true
			// 如果配置文件中有 stream 字段，则覆盖默认值
			if stream, ok := apiConfigMap["stream"].(bool); ok {
				config.APIConfig.Stream = stream
			} else if stream, ok := apiConfigMap["Stream"].(bool); ok {
				config.APIConfig.Stream = stream
			}

			// 设置默认值为 false
			config.APIConfig.Thinking = false
			// 如果配置文件中有 thinking 字段，则覆盖默认值
			if thinking, ok := apiConfigMap["thinking"].(bool); ok {
				config.APIConfig.Thinking = thinking
			} else if thinking, ok := apiConfigMap["Thinking"].(bool); ok {
				config.APIConfig.Thinking = thinking
			}

			// 设置默认值为 10 分钟
			config.APIConfig.Timeout = 10
			// 如果配置文件中有 timeout 字段，则覆盖默认值
			if timeout, ok := apiConfigMap["timeout"].(float64); ok {
				config.APIConfig.Timeout = int(timeout)
			} else if timeout, ok := apiConfigMap["Timeout"].(float64); ok {
				config.APIConfig.Timeout = int(timeout)
			}
		}
	}

	// 处理环境变量覆盖
	// API类型
	if apiTypeStr := os.Getenv("API_TYPE"); apiTypeStr != "" {
		config.APIConfig.APIType = apiTypeStr
	}
	// 默认值
	if config.APIConfig.APIType == "" {
		config.APIConfig.APIType = "openai" // 默认值
	}

	// BaseURL
	if baseURLStr := os.Getenv("BASE_URL"); baseURLStr != "" {
		config.APIConfig.BaseURL = baseURLStr
	} else if config.APIConfig.APIType == "openai" {
		if openaiBaseURL := os.Getenv("OPENAI_BASE_URL"); openaiBaseURL != "" {
			config.APIConfig.BaseURL = openaiBaseURL
		}
	} else if config.APIConfig.APIType == "anthropic" {
		if anthropicBaseURL := os.Getenv("ANTHROPIC_BASE_URL"); anthropicBaseURL != "" {
			config.APIConfig.BaseURL = anthropicBaseURL
		}
	}

	// APIKey
	if apiKeyStr := os.Getenv("API_KEY"); apiKeyStr != "" {
		config.APIConfig.APIKey = apiKeyStr
	} else if config.APIConfig.APIType == "openai" {
		if openaiAPIKey := os.Getenv("OPENAI_API_KEY"); openaiAPIKey != "" {
			config.APIConfig.APIKey = openaiAPIKey
		}
	} else if config.APIConfig.APIType == "anthropic" {
		if anthropicAPIKey := os.Getenv("ANTHROPIC_API_KEY"); anthropicAPIKey != "" {
			config.APIConfig.APIKey = anthropicAPIKey
		}
	}

	// ModelID
	if modelIDStr := os.Getenv("MODEL_ID"); modelIDStr != "" {
		config.APIConfig.Model = modelIDStr
	}
	// 默认值
	if config.APIConfig.Model == "" {
		config.APIConfig.Model = DEFAULT_MODEL_ID
	}

	// Temperature
	if tempStr := os.Getenv("TEMPERATURE"); tempStr != "" {
		if temp, err := strconv.ParseFloat(tempStr, 64); err == nil {
			config.APIConfig.Temperature = temp
		}
	}
	// 如深度求索的 temperature 默认值有可能取值为零，所以此处不设置默认值

	// MaxTokens
	if tokensStr := os.Getenv("MAX_TOKENS"); tokensStr != "" {
		if tokens, err := strconv.Atoi(tokensStr); err == nil {
			config.APIConfig.MaxTokens = tokens
		}
	}
	// 默认值
	if config.APIConfig.MaxTokens == 0 {
		config.APIConfig.MaxTokens = 4096 // 默认值
	}

	// Stream
	if streamStr := os.Getenv("STREAM"); streamStr != "" {
		if streamVal, err := strconv.ParseBool(streamStr); err == nil {
			config.APIConfig.Stream = streamVal
		}
	}

	// Thinking
	if thinkingStr := os.Getenv("THINKING"); thinkingStr != "" {
		if thinkingVal, err := strconv.ParseBool(thinkingStr); err == nil {
			config.APIConfig.Thinking = thinkingVal
		}
	}

	// Timeout
	if timeoutStr := os.Getenv("TIMEOUT"); timeoutStr != "" {
		if timeout, err := strconv.Atoi(timeoutStr); err == nil {
			config.APIConfig.Timeout = timeout
		}
	}
	// 默认值
	if config.APIConfig.Timeout == 0 {
		config.APIConfig.Timeout = 10 // 默认值 10 分钟
	}

	// 打印解析后的配置
	if isDebug {
		fmt.Printf("Loaded config: %+v\n", config)
	}

	return config, nil
}
