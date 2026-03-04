package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/toon-format/toon-go"
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

	// 打印解析后的配置
	if isDebug {
		fmt.Printf("Loaded config: %+v\n", config)
	}

	return config, nil
}
