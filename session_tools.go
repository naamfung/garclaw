package main

import (
	"fmt"
	"strings"
	"time"
)

// ProcessSessionCommand 处理会话相关的斜杠命令
func ProcessSessionCommand(line string, sm *SessionManager, stage *Stage) (handled bool, response string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false, ""
	}

	switch parts[0] {
	case "/save":
		return handleSaveCommand(parts[1:], sm, stage)
	case "/load":
		return handleLoadCommand(parts[1:], sm, stage)
	case "/session":
		return handleSessionCommand(parts[1:], sm, stage)
	case "/new":
		return handleNewCommand(parts[1:], sm, stage)
	}

	return false, ""
}

// handleSaveCommand 处理 /save 命令
func handleSaveCommand(args []string, sm *SessionManager, stage *Stage) (bool, string) {
	description := ""
	if len(args) > 0 {
		description = strings.Join(args, " ")
	}

	// 更新场景状态到会话
	state := sm.GetCurrentState()
	if state != nil {
		state.Stage = stage.ToState()
	}

	if err := sm.Save(description); err != nil {
		return true, fmt.Sprintf("❌ 保存失败: %v", err)
	}

	state = sm.GetCurrentState()
	return true, fmt.Sprintf("✅ 会话已保存\n   会话ID: %s\n   时间: %s", 
		state.SessionID, 
		state.UpdatedAt.Format("2006-01-02 15:04:05"))
}

// handleLoadCommand 处理 /load 命令
func handleLoadCommand(args []string, sm *SessionManager, stage *Stage) (bool, string) {
	if len(args) == 0 {
		// 列出可用会话
		sessions, err := sm.ListSessions()
		if err != nil {
			return true, fmt.Sprintf("❌ 列出会话失败: %v", err)
		}

		if len(sessions) == 0 {
			return true, "📭 没有保存的会话\n\n使用 /save [描述] 保存当前会话"
		}

		var sb strings.Builder
		sb.WriteString("📋 已保存的会话:\n\n")
		for i, s := range sessions {
			desc := s.Description
			if desc == "" {
				desc = "(无描述)"
			}
			sb.WriteString(fmt.Sprintf("%d. [%s] %s\n   %s\n\n", 
				i+1, s.SessionID, desc, s.UpdatedAt.Format("2006-01-02 15:04")))
		}
		sb.WriteString("使用 /load <会话ID> 加载指定会话")
		return true, sb.String()
	}

	sessionID := args[0]
	if err := sm.Load(sessionID); err != nil {
		return true, fmt.Sprintf("❌ 加载会话失败: %v", err)
	}

	// 恢复场景状态
	state := sm.GetCurrentState()
	if state != nil {
		stage.RestoreFromState(state.Stage)
	}

	return true, fmt.Sprintf("✅ 已加载会话: %s\n   描述: %s\n   消息数: %d",
		sessionID, 
		func() string { if state.Description != "" { return state.Description } else { return "(无描述)" } }(),
		len(state.Messages))
}

// handleSessionCommand 处理 /session 命令
func handleSessionCommand(args []string, sm *SessionManager, stage *Stage) (bool, string) {
	if len(args) == 0 {
		// 显示当前会话信息
		state := sm.GetCurrentState()
		var sb strings.Builder
		sb.WriteString("📌 当前会话信息:\n\n")
		sb.WriteString(fmt.Sprintf("会话ID: %s\n", state.SessionID))
		sb.WriteString(fmt.Sprintf("创建时间: %s\n", state.CreatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("更新时间: %s\n", state.UpdatedAt.Format("2006-01-02 15:04:05")))
		if state.Description != "" {
			sb.WriteString(fmt.Sprintf("描述: %s\n", state.Description))
		}
		sb.WriteString(fmt.Sprintf("消息数: %d\n", len(state.Messages)))
		sb.WriteString(fmt.Sprintf("当前演员: %s\n", state.Stage.CurrentActor))
		return true, sb.String()
	}

	subCmd := args[0]
	switch subCmd {
	case "list", "ls":
		return handleLoadCommand([]string{}, sm, stage)
	case "delete", "rm":
		if len(args) < 2 {
			return true, "用法: /session delete <会话ID>"
		}
		if err := sm.DeleteSession(args[1]); err != nil {
			return true, fmt.Sprintf("❌ 删除失败: %v", err)
		}
		return true, fmt.Sprintf("✅ 已删除会话: %s", args[1])
	case "export":
		if len(args) < 2 {
			return true, "用法: /session export <文件路径>"
		}
		if err := sm.ExportToJSON(args[1]); err != nil {
			return true, fmt.Sprintf("❌ 导出失败: %v", err)
		}
		return true, fmt.Sprintf("✅ 已导出到: %s", args[1])
	case "import":
		if len(args) < 2 {
			return true, "用法: /session import <文件路径>"
		}
		if err := sm.ImportFromJSON(args[1]); err != nil {
			return true, fmt.Sprintf("❌ 导入失败: %v", err)
		}
		state := sm.GetCurrentState()
		stage.RestoreFromState(state.Stage)
		return true, fmt.Sprintf("✅ 已导入会话: %s", state.SessionID)
	default:
		return true, "未知命令。可用: list, delete, export, import"
	}
}

// handleNewCommand 处理 /new 命令
func handleNewCommand(args []string, sm *SessionManager, stage *Stage) (bool, string) {
	// 可选：先保存当前会话
	// sm.QuickSave()

	// 创建新会话
	sm.NewSession()

	// 重置场景
	*stage = *NewStage()

	state := sm.GetCurrentState()
	return true, fmt.Sprintf("✅ 已创建新会话\n   会话ID: %s", state.SessionID)
}

// RegisterSessionCommands 注册会话命令帮助
func GetSessionCommandsHelp() string {
	return `
📋 会话管理命令:

  /save [描述]         保存当前会话
  /load [会话ID]       加载会话（不带ID则列出所有会话）
  /session             显示当前会话信息
  /session list        列出所有保存的会话
  /session delete <ID> 删除指定会话
  /session export <文件> 导出会话到JSON
  /session import <文件> 从JSON导入会话
  /new                 创建新会话

💡 提示:
  - 会话自动保存间隔: 5分钟
  - 会话文件存储在 sessions/ 目录
`
}

// AutoSaveLoop 自动保存循环（在后台运行）
func AutoSaveLoop(sm *SessionManager, stage *Stage, stopCh <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			// 退出前保存
			state := sm.GetCurrentState()
			if state != nil {
				state.Stage = stage.ToState()
			}
			sm.QuickSave()
			return
		case <-ticker.C:
			state := sm.GetCurrentState()
			if state != nil {
				state.Stage = stage.ToState()
			}
			sm.QuickSave()
		}
	}
}
