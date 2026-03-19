package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/toon-format/toon-go"
)

// 配置常量
const (
	DEFAULT_API_TYPE   = "openai"
	ANTHROPIC_BASE_URL = "https://api.anthropic.com/v1"
	OLLAMA_BASE_URL    = "http://localhost:11434/api"
	OPENAI_BASE_URL    = "https://api.openai.com/v1"
	DEFAULT_MODEL_ID   = "claude-3-opus-20240229"
	CONFIG_FILE        = "config.toon"
)

// HTTP服务器配置
type HTTPServerConfig struct {
	Listen string `json:"listen"` // 例如 "0.0.0.0:10086"
}

// 邮件配置
type EmailConfig struct {
	IMAPServer   string `json:"imap_server"`
	IMAPPort     int    `json:"imap_port"`
	IMAPUseTLS   bool   `json:"imap_use_tls"`
	IMAPUser     string `json:"imap_user"`
	IMAPPassword string `json:"imap_password"`
	SMTPServer   string `json:"smtp_server"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
	SMTPUser     string `json:"smtp_user"`
	SMTPPassword string `json:"smtp_password"`
	PollInterval int    `json:"poll_interval"` // 秒
}

// API配置
type APIConfig struct {
	APIType                string  `json:"api_type"`
	BaseURL                string  `json:"base_url"`
	APIKey                 string  `json:"api_key"`
	Model                  string  `json:"model"`
	Temperature            float64 `json:"temperature"`
	MaxTokens              int     `json:"max_tokens"`
	Stream                 bool    `json:"stream"`
	Thinking               bool    `json:"thinking"`
	BlockDangerousCommands bool    `json:"block_dangerous_commands"` // 新增：是否拦截危险命令
}

// 主配置结构
type Config struct {
	APIConfig   APIConfig       `json:"api_config"`
	HTTPServer  HTTPServerConfig `json:"http_server"`
	EmailConfig *EmailConfig     `json:"email_config,omitempty"`
}

// 加载配置文件
func loadConfig() (Config, error) {
	var config Config

	// 获取程序自身路径
	execPath, err := os.Executable()
	if err != nil {
		return config, fmt.Errorf("error getting executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)
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
		defaultConfig.APIConfig.BlockDangerousCommands = false // 修改为默认 false，即不拦截
		defaultConfig.HTTPServer.Listen = "0.0.0.0:10086"

		toonData, err := toon.Marshal(defaultConfig)
		if err == nil {
			os.WriteFile(configPath, toonData, 0644)
			fmt.Printf("Generated default config file at: %s\n", configPath)
		}
		return config, fmt.Errorf("error reading config file: %v", err)
	}

	// 解析 TOON
	parsed, err := toon.Decode(data)
	if err != nil {
		return config, fmt.Errorf("error parsing TOON config: %v", err)
	}

	// 手动解析 api_config
	if apiConfig, ok := parsed.(map[string]interface{})["api_config"]; ok {
		if apiMap, ok := apiConfig.(map[string]interface{}); ok {
			if v, ok := apiMap["api_type"]; ok {
				config.APIConfig.APIType = toString(v)
			}
			if v, ok := apiMap["base_url"]; ok {
				config.APIConfig.BaseURL = toString(v)
			}
			if v, ok := apiMap["api_key"]; ok {
				config.APIConfig.APIKey = toString(v)
			}
			if v, ok := apiMap["model"]; ok {
				config.APIConfig.Model = toString(v)
			}
			if v, ok := apiMap["temperature"]; ok {
				config.APIConfig.Temperature = toFloat(v)
			}
			if v, ok := apiMap["max_tokens"]; ok {
				config.APIConfig.MaxTokens = toInt(v)
			}
			if v, ok := apiMap["stream"]; ok {
				config.APIConfig.Stream = toBool(v)
			}
			if v, ok := apiMap["thinking"]; ok {
				config.APIConfig.Thinking = toBool(v)
			}
			// 解析新增字段
			if v, ok := apiMap["block_dangerous_commands"]; ok {
				config.APIConfig.BlockDangerousCommands = toBool(v)
			}
		}
	}

	// 解析 http_server
	if httpCfg, ok := parsed.(map[string]interface{})["http_server"]; ok {
		if httpMap, ok := httpCfg.(map[string]interface{}); ok {
			if v, ok := httpMap["listen"]; ok {
				config.HTTPServer.Listen = toString(v)
			}
		}
	}
	if config.HTTPServer.Listen == "" {
		config.HTTPServer.Listen = "0.0.0.0:10086"
	}

	// 解析 email_config
	if emailCfg, ok := parsed.(map[string]interface{})["email_config"]; ok {
		if emailMap, ok := emailCfg.(map[string]interface{}); ok {
			ec := &EmailConfig{}
			if v, ok := emailMap["imap_server"]; ok {
				ec.IMAPServer = toString(v)
			}
			if v, ok := emailMap["imap_port"]; ok {
				ec.IMAPPort = toInt(v)
			}
			if v, ok := emailMap["imap_use_tls"]; ok {
				ec.IMAPUseTLS = toBool(v)
			}
			if v, ok := emailMap["imap_user"]; ok {
				ec.IMAPUser = toString(v)
			}
			if v, ok := emailMap["imap_password"]; ok {
				ec.IMAPPassword = toString(v)
			}
			if v, ok := emailMap["smtp_server"]; ok {
				ec.SMTPServer = toString(v)
			}
			if v, ok := emailMap["smtp_port"]; ok {
				ec.SMTPPort = toInt(v)
			}
			if v, ok := emailMap["smtp_use_tls"]; ok {
				ec.SMTPUseTLS = toBool(v)
			}
			if v, ok := emailMap["smtp_user"]; ok {
				ec.SMTPUser = toString(v)
			}
			if v, ok := emailMap["smtp_password"]; ok {
				ec.SMTPPassword = toString(v)
			}
			if v, ok := emailMap["poll_interval"]; ok {
				ec.PollInterval = toInt(v)
			}
			config.EmailConfig = ec
		}
	}

	// 环境变量覆盖
	if v := os.Getenv("API_TYPE"); v != "" {
		config.APIConfig.APIType = v
	}
	if v := os.Getenv("BASE_URL"); v != "" {
		config.APIConfig.BaseURL = v
	}
	if v := os.Getenv("API_KEY"); v != "" {
		config.APIConfig.APIKey = v
	}
	if v := os.Getenv("MODEL_ID"); v != "" {
		config.APIConfig.Model = v
	}
	if v := os.Getenv("TEMPERATURE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			config.APIConfig.Temperature = f
		}
	}
	if v := os.Getenv("MAX_TOKENS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			config.APIConfig.MaxTokens = i
		}
	}
	if v := os.Getenv("STREAM"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			config.APIConfig.Stream = b
		}
	}
	if v := os.Getenv("THINKING"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			config.APIConfig.Thinking = b
		}
	}
	// 环境变量覆盖危险命令拦截开关
	if v := os.Getenv("BLOCK_DANGEROUS_COMMANDS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			config.APIConfig.BlockDangerousCommands = b
		}
	}

	// 默认值
	if config.APIConfig.Model == "" {
		config.APIConfig.Model = DEFAULT_MODEL_ID
	}
	if config.APIConfig.MaxTokens == 0 {
		config.APIConfig.MaxTokens = 4096
	}
	if config.APIConfig.APIType == "" {
		config.APIConfig.APIType = DEFAULT_API_TYPE
	}
	// 危险命令拦截开关默认为 false，无需额外设置

	if IsDebug {
		fmt.Printf("Loaded config: %+v\n", config)
	}

	return config, nil
}

// 辅助转换函数
func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toFloat(v interface{}) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func toInt(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func toBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}