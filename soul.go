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

// Load 加载灵魂信息
func (ss *SoulSystem) Load() {
	soulPath := filepath.Join(ss.workspaceDir, "SOUL.md")
	if content, err := os.ReadFile(soulPath); err == nil && len(content) > 0 {
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
