package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MemoryStore 统一管理记忆功能
type MemoryStore struct {
	workspace      string
	memoryPath     string
	dailyMemoryDir string
}

// NewMemoryStore 创建新的 MemoryStore 实例
func NewMemoryStore(workspace string) *MemoryStore {
	return &MemoryStore{
		workspace:      workspace,
		memoryPath:     filepath.Join(workspace, "MEMORY.md"),
		dailyMemoryDir: filepath.Join(workspace, "memory", "daily"),
	}
}

// LoadEvergreen 加载永久记忆
func (ms *MemoryStore) LoadEvergreen() string {
	if _, err := os.Stat(ms.memoryPath); os.IsNotExist(err) {
		// 如果文件不存在，创建空文件
		if err := os.WriteFile(ms.memoryPath, []byte(""), 0644); err != nil {
			fmt.Printf("Error creating MEMORY.md: %v\n", err)
			return ""
		}
		return ""
	}

	content, err := os.ReadFile(ms.memoryPath)
	if err != nil {
		fmt.Printf("Error reading MEMORY.md: %v\n", err)
		return ""
	}

	return string(content)
}

// WriteMemory 写入永久记忆
func (ms *MemoryStore) WriteMemory(content string) string {
	existing := ms.LoadEvergreen()
	updated := existing + "\n\n" + strings.TrimSpace(content)
	if existing == "" {
		updated = strings.TrimSpace(content)
	}

	if err := os.WriteFile(ms.memoryPath, []byte(updated), 0644); err != nil {
		return "Error writing to memory: " + err.Error()
	}

	return fmt.Sprintf("Successfully wrote %d characters to memory", len(content))
}

// WriteDailyMemory 写入每日记忆
func (ms *MemoryStore) WriteDailyMemory(content string, category string) string {
	// 确保目录存在
	if err := os.MkdirAll(ms.dailyMemoryDir, 0755); err != nil {
		return "Error creating memory directory: " + err.Error()
	}

	// 写入每日JSONL文件
	today := time.Now().Format("2006-01-02")
	jsonlPath := filepath.Join(ms.dailyMemoryDir, today+".jsonl")

	entry := map[string]interface{}{
		"ts":       time.Now().Unix(),
		"content":  content,
		"category": category,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return "Error marshaling memory entry: " + err.Error()
	}

	f, err := os.OpenFile(jsonlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "Error opening memory file: " + err.Error()
	}
	defer f.Close()

	_, err = f.WriteString(string(data) + "\n")
	if err != nil {
		return "Error writing to memory file: " + err.Error()
	}

	return fmt.Sprintf("Successfully wrote %d characters to daily memory", len(content))
}

// SearchMemory 搜索记忆
func (ms *MemoryStore) SearchMemory(query string) string {
	matches := []string{}

	// 搜索永久记忆
	evergreen := ms.LoadEvergreen()
	if evergreen != "" {
		for _, line := range strings.Split(evergreen, "\n") {
			if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
				matches = append(matches, fmt.Sprintf("[MEMORY.md] %s", line))
			}
		}
	}

	// 搜索每日记忆
	if _, err := os.Stat(ms.dailyMemoryDir); err == nil {
		files, err := os.ReadDir(ms.dailyMemoryDir)
		if err == nil {
			for _, file := range files {
				if strings.HasSuffix(file.Name(), ".jsonl") {
					filePath := filepath.Join(ms.dailyMemoryDir, file.Name())
					data, err := os.ReadFile(filePath)
					if err == nil {
						lines := strings.Split(string(data), "\n")
						for _, line := range lines {
							if line == "" {
								continue
							}
							var entry map[string]interface{}
							if err := json.Unmarshal([]byte(line), &entry); err == nil {
								if content, ok := entry["content"].(string); ok {
									if strings.Contains(strings.ToLower(content), strings.ToLower(query)) {
										matches = append(matches, fmt.Sprintf("[%s] %s", file.Name(), content))
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if len(matches) == 0 {
		return "No memories matching '" + query + "'."
	}

	// 限制返回结果数量
	maxMatches := 10
	if len(matches) > maxMatches {
		matches = matches[:maxMatches]
	}

	result := "Search results:\n"
	for i, match := range matches {
		result += fmt.Sprintf("%d: %s\n", i+1, match)
	}

	return result
}
