package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	MAX_FILE_CHARS   = 20000
	MAX_TOTAL_CHARS = 150000
)

// Bootstrap 文件名 -- 每个 agent 启动时加载这些文件
var BOOTSTRAP_FILES = []string{
	"SOUL.md", "IDENTITY.md", "TOOLS.md", "USER.md",
	"HEARTBEAT.md", "BOOTSTRAP.md", "AGENTS.md", "MEMORY.md",
}

// BootstrapLoader 加载工作区的 Bootstrap 文件
type BootstrapLoader struct {
	workspaceDir string
}

// NewBootstrapLoader 创建一个新的 BootstrapLoader 实例
func NewBootstrapLoader(workspaceDir string) *BootstrapLoader {
	return &BootstrapLoader{
		workspaceDir: workspaceDir,
	}
}

// loadFile 加载单个文件
func (bl *BootstrapLoader) loadFile(name string) string {
	path := filepath.Join(bl.workspaceDir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// truncateFile 截断超长文件内容
func (bl *BootstrapLoader) truncateFile(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	cut := strings.LastIndex(content[:maxChars], "\n")
	if cut <= 0 {
		cut = maxChars
	}
	return content[:cut] + "\n\n[... truncated (" + strconv.Itoa(len(content)) + " chars total, showing first " + strconv.Itoa(cut) + ") ...]"
}

// loadAll 加载所有 Bootstrap 文件
func (bl *BootstrapLoader) loadAll(mode string) map[string]string {
	if mode == "none" {
		return map[string]string{}
	}
	
	var names []string
	if mode == "minimal" {
		names = []string{"AGENTS.md", "TOOLS.md"}
	} else {
		names = BOOTSTRAP_FILES
	}
	
	result := make(map[string]string)
	total := 0
	
	for _, name := range names {
		raw := bl.loadFile(name)
		if raw == "" {
			continue
		}
		
		truncated := bl.truncateFile(raw, MAX_FILE_CHARS)
		if total+len(truncated) > MAX_TOTAL_CHARS {
			remaining := MAX_TOTAL_CHARS - total
			if remaining > 0 {
				truncated = bl.truncateFile(raw, remaining)
			} else {
				break
			}
		}
		
		result[name] = truncated
		total += len(truncated)
	}
	
	return result
}
