package main

import (
	"fmt"
	"os"
	"path/filepath"

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
		}
	}

	// 打印解析后的配置
	if isDebug {
		fmt.Printf("Loaded config: %+v\n", config)
	}

	return config, nil
}
