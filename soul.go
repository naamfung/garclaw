package main

import (
	"os"
	"path/filepath"
)

// SoulSystem 灵魂系统

type SoulSystem struct {
	workspaceDir string
	soulContent string
}

// NewSoulSystem 创建一个新的 SoulSystem 实例
func NewSoulSystem(workspaceDir string) *SoulSystem {
	return &SoulSystem{
		workspaceDir: workspaceDir,
		soulContent: "",
	}
}

// Load 加载灵魂信息，不存在时生成模板
func (ss *SoulSystem) Load() {
	soulPath := filepath.Join(ss.workspaceDir, "SOUL.md")
	if _, err := os.Stat(soulPath); os.IsNotExist(err) {
		// 确保目录存在
		if err := os.MkdirAll(ss.workspaceDir, 0755); err != nil {
			return
		}
		// 写入模板文件
		if err := os.WriteFile(soulPath, []byte(SOUL_TEMPLATE), 0644); err != nil {
			return
		}
		ss.soulContent = SOUL_TEMPLATE
	} else if content, err := os.ReadFile(soulPath); err == nil && len(content) > 0 {
		ss.soulContent = string(content)
	}
}

// GetSoulContent 获取灵魂内容
func (ss *SoulSystem) GetSoulContent() string {
	return ss.soulContent
}

// HasSoul 检查是否有灵魂信息
func (ss *SoulSystem) HasSoul() bool {
	return ss.soulContent != ""
}
