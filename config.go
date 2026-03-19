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
	DEFAULT_MODEL_ID   = "deepseek-chat" // 修改默认模型以匹配您实际使用的
	CONFIG_FILE        = "config.toon"
)

// HTTP服务器配置
type HTTPServerConfig struct {
	Listen string `json:"listen"`
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
	PollInterval int    `json:"poll_interval"`
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
	BlockDangerousCommands bool    `json:"block_dangerous_commands"`
}

// 主配置结构
type Config struct {
	APIConfig   APIConfig       `json:"api_config"`
	HTTPServer  HTTPServerConfig `json:"http_server"`
	EmailConfig *EmailConfig     `json:"email_config,omitempty"`
}

// 辅助函数：从 map 中获取字符串值，支持多个键名
func getString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok {
				return s
			}
		}
	}
	return ""
}

func getFloat(m map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if f, ok := val.(float64); ok {
				return f
			}
		}
	}
	return 0
}

func getInt(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if f, ok := val.(float64); ok {
				return int(f)
			}
		}
	}
	return 0
}

func getBool(m map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if b, ok := val.(bool); ok {
				return b
			}
		}
	}
	return false
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
		defaultConfig.APIConfig.BlockDangerousCommands = false
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

	// 打印原始解析结果用于调试
	if IsDebug {
		fmt.Printf("Raw parsed config: %+v\n", parsed)
	}

	// 尝试获取 api_config 部分，支持多种键名
	var apiConfigMap map[string]interface{}
	if apiConfig, ok := parsed.(map[string]interface{})["api_config"]; ok {
		if m, ok := apiConfig.(map[string]interface{}); ok {
			apiConfigMap = m
		}
	} else if apiConfig, ok := parsed.(map[string]interface{})["APIConfig"]; ok {
		if m, ok := apiConfig.(map[string]interface{}); ok {
			apiConfigMap = m
		}
	}

	if apiConfigMap != nil {
		// 使用辅助函数获取值，支持多种键名
		config.APIConfig.APIType = getString(apiConfigMap, "api_type", "APIType", "apitype")
		config.APIConfig.BaseURL = getString(apiConfigMap, "base_url", "BaseURL", "baseurl")
		config.APIConfig.APIKey = getString(apiConfigMap, "api_key", "APIKey", "apikey")
		config.APIConfig.Model = getString(apiConfigMap, "model", "Model")
		config.APIConfig.Temperature = getFloat(apiConfigMap, "temperature", "Temperature")
		config.APIConfig.MaxTokens = getInt(apiConfigMap, "max_tokens", "MaxTokens", "maxtokens")
		config.APIConfig.Stream = getBool(apiConfigMap, "stream", "Stream")
		config.APIConfig.Thinking = getBool(apiConfigMap, "thinking", "Thinking")
		config.APIConfig.BlockDangerousCommands = getBool(apiConfigMap, "block_dangerous_commands", "BlockDangerousCommands", "blockdangerouscommands")
	}

	// 解析 http_server
	if httpCfg, ok := parsed.(map[string]interface{})["http_server"]; ok {
		if httpMap, ok := httpCfg.(map[string]interface{}); ok {
			config.HTTPServer.Listen = getString(httpMap, "listen", "Listen")
		}
	} else if httpCfg, ok := parsed.(map[string]interface{})["HTTPServer"]; ok {
		if httpMap, ok := httpCfg.(map[string]interface{}); ok {
			config.HTTPServer.Listen = getString(httpMap, "listen", "Listen")
		}
	}
	if config.HTTPServer.Listen == "" {
		config.HTTPServer.Listen = "0.0.0.0:10086"
	}

	// 解析 email_config
	if emailCfg, ok := parsed.(map[string]interface{})["email_config"]; ok {
		if emailMap, ok := emailCfg.(map[string]interface{}); ok {
			ec := &EmailConfig{}
			ec.IMAPServer = getString(emailMap, "imap_server", "IMAPServer")
			ec.IMAPPort = getInt(emailMap, "imap_port", "IMAPPort")
			ec.IMAPUseTLS = getBool(emailMap, "imap_use_tls", "IMAPUseTLS")
			ec.IMAPUser = getString(emailMap, "imap_user", "IMAPUser")
			ec.IMAPPassword = getString(emailMap, "imap_password", "IMAPPassword")
			ec.SMTPServer = getString(emailMap, "smtp_server", "SMTPServer")
			ec.SMTPPort = getInt(emailMap, "smtp_port", "SMTPPort")
			ec.SMTPUseTLS = getBool(emailMap, "smtp_use_tls", "SMTPUseTLS")
			ec.SMTPUser = getString(emailMap, "smtp_user", "SMTPUser")
			ec.SMTPPassword = getString(emailMap, "smtp_password", "SMTPPassword")
			ec.PollInterval = getInt(emailMap, "poll_interval", "PollInterval")
			config.EmailConfig = ec
		}
	} else if emailCfg, ok := parsed.(map[string]interface{})["EmailConfig"]; ok {
		if emailMap, ok := emailCfg.(map[string]interface{}); ok {
			ec := &EmailConfig{}
			ec.IMAPServer = getString(emailMap, "imap_server", "IMAPServer")
			ec.IMAPPort = getInt(emailMap, "imap_port", "IMAPPort")
			ec.IMAPUseTLS = getBool(emailMap, "imap_use_tls", "IMAPUseTLS")
			ec.IMAPUser = getString(emailMap, "imap_user", "IMAPUser")
			ec.IMAPPassword = getString(emailMap, "imap_password", "IMAPPassword")
			ec.SMTPServer = getString(emailMap, "smtp_server", "SMTPServer")
			ec.SMTPPort = getInt(emailMap, "smtp_port", "SMTPPort")
			ec.SMTPUseTLS = getBool(emailMap, "smtp_use_tls", "SMTPUseTLS")
			ec.SMTPUser = getString(emailMap, "smtp_user", "SMTPUser")
			ec.SMTPPassword = getString(emailMap, "smtp_password", "SMTPPassword")
			ec.PollInterval = getInt(emailMap, "poll_interval", "PollInterval")
			config.EmailConfig = ec
		}
	}

	// 环境变量覆盖（仅当环境变量非空时覆盖）
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
	if v := os.Getenv("BLOCK_DANGEROUS_COMMANDS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			config.APIConfig.BlockDangerousCommands = b
		}
	}

	// 设置默认值（如果配置文件中未提供）
	if config.APIConfig.APIType == "" {
		config.APIConfig.APIType = DEFAULT_API_TYPE
	}
	if config.APIConfig.Model == "" {
		config.APIConfig.Model = DEFAULT_MODEL_ID
	}
	if config.APIConfig.MaxTokens == 0 {
		config.APIConfig.MaxTokens = 4096
	}
	// Temperature 可能为0，所以不设置默认值

	if IsDebug {
		fmt.Printf("Loaded config: %+v\n", config)
	}

	return config, nil
}