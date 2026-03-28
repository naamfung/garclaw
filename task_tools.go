package main

import (
        "context"
        "fmt"
        "time"

        "github.com/toon-format/toon-go"
)

// handleSmartShell 处理 smart_shell 工具调用
// 智能判断命令执行模式：同步或异步
// 混合策略：
//   - 快速命令（白名单）→ 同步执行，标准超时（60秒）
//   - 长时命令（黑名单）→ 异步执行
//   - 交互式命令 → 异步执行 + 变换建议
//   - 未知命令 → 同步执行，长超时（120秒）+ 超时建议
func handleSmartShell(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 获取命令
        command, ok := argsMap["command"].(string)
        if !ok || command == "" {
                return "Error: missing or invalid 'command' parameter", false
        }

        // 获取强制模式参数
        forceAsync := false
        if a, ok := argsMap["async"].(bool); ok {
                forceAsync = a
        }
        forceSync := false
        if s, ok := argsMap["sync"].(bool); ok {
                forceSync = s
        }

        // 参数冲突检查
        if forceAsync && forceSync {
                return "Error: cannot specify both async=true and sync=true", false
        }

        // 获取唤醒时间（分钟）
        wakeAfterMinutes := globalToolsConfig.SmartShell.DefaultWakeMins
        if wakeAfterMinutes <= 0 {
                wakeAfterMinutes = 5 // 默认 5 分钟
        }
        if waf, ok := argsMap["wake_after_minutes"].(float64); ok && waf > 0 {
                wakeAfterMinutes = int(waf)
        }

        // 判断执行模式
        var execMode string // "quick", "long_running", "interactive", "unknown"
        var suggestion CommandSuggestion

        if forceAsync {
                // 强制异步
                execMode = "async_forced"
        } else if forceSync {
                // 强制同步
                execMode = "sync_forced"
        } else {
                // 智能判断：检测命令类型
                suggestion = DetectCommandType(command)
                execMode = suggestion.Type
        }

        // 根据执行模式选择处理方式
        switch execMode {
        case "quick", "sync_forced":
                // 快速命令或强制同步 → 同步执行，标准超时
                return handleSmartShellSync(ctx, command, ch, false)

        case "interactive":
                // 交互式命令 → 异步执行 + 变换建议
                return handleSmartShellInteractive(command, suggestion, wakeAfterMinutes, ch)

        case "long_running", "async_forced":
                // 长时命令、强制异步 → 异步执行
                return handleSmartShellAsync(command, wakeAfterMinutes, ch)

        case "unknown":
                // 未知命令 → 同步执行，长超时 + 超时建议
                return handleSmartShellSync(ctx, command, ch, true)

        default:
                // 默认：未知命令处理
                return handleSmartShellSync(ctx, command, ch, true)
        }
}

// handleSmartShellInteractive 处理交互式命令
// 异步执行并提供变换建议
func handleSmartShellInteractive(command string, suggestion CommandSuggestion, wakeAfterMinutes int, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        // 从 Channel 获取会话ID
        sessionID := ch.GetSessionID()

        // 启动后台任务
        task, err := globalTaskManager.StartDelayedExec(command, wakeAfterMinutes, "", sessionID)
        if err != nil {
                return fmt.Sprintf("Error: failed to start async execution: %v", err), false
        }

        // 构建响应消息
        msg := fmt.Sprintf("⚠️ 检测到交互式命令\n\n")
        msg += fmt.Sprintf("**命令**: `%s`\n\n", command)
        msg += fmt.Sprintf("**问题**: %s\n\n", suggestion.Message)
        
        if suggestion.Suggestion != "" {
                msg += fmt.Sprintf("**建议**: %s\n\n", suggestion.Suggestion)
        }
        
        if suggestion.NonInteractiveEq != "" {
                msg += fmt.Sprintf("**非交互式等价命令**: `%s`\n\n", suggestion.NonInteractiveEq)
        }
        
        msg += fmt.Sprintf("---\n\n")
        msg += fmt.Sprintf("✅ 命令已异步启动（PID: %d），将在 %d 分钟后唤醒。\n", task.PID, wakeAfterMinutes)
        msg += fmt.Sprintf("如果命令卡在交互状态，你可以使用 `task_terminate` 终止它。\n\n")
        msg += fmt.Sprintf("💡 建议：下次使用非交互式等价命令以避免此问题。")

        // 返回任务信息
        result := map[string]interface{}{
                "mode":               "interactive",
                "task_id":            task.ID,
                "pid":                task.PID,
                "status":             "running",
                "command":            command,
                "wake_after_minutes": wakeAfterMinutes,
                "suggestion":         suggestion.Suggestion,
                "non_interactive_eq": suggestion.NonInteractiveEq,
                "message":            msg,
        }

        resultTOON, _ := toon.Marshal(result)
        return string(resultTOON), false
}

// handleSmartShellSync 同步执行命令
// isUnknown: 是否为未知命令（未知命令使用更长超时）
func handleSmartShellSync(ctx context.Context, command string, ch Channel, isUnknown bool) (string, bool) {
        // 确定超时时间
        timeout := globalToolsConfig.SmartShell.SyncTimeout
        if timeout <= 0 {
                timeout = 60 // 默认 60 秒
        }

        // 未知命令使用更长超时
        if isUnknown {
                unknownTimeout := globalToolsConfig.SmartShell.UnknownTimeout
                if unknownTimeout <= 0 {
                        unknownTimeout = 120 // 默认 120 秒
                }
                timeout = unknownTimeout
        }

        // 创建带超时的 context
        ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
        defer cancel()

        // 执行命令
        result := runShellWithTimeout(ctxWithTimeout, command, false, false)

        // 检查是否需要确认
        if result.ConfirmRequired {
                response := map[string]interface{}{
                        "mode":            "confirm_required",
                        "confirm_message": result.ConfirmMessage,
                        "suggestions":     result.Suggestions,
                        "message":         "⚠️ 此命令可能需要交互确认。请确认后重新执行，或使用 smart_shell(command, async=true) 异步执行。",
                }
                resultTOON, _ := toon.Marshal(response)
                return string(resultTOON), false
        }

        // 检查是否超时
        if ctxWithTimeout.Err() == context.DeadlineExceeded {
                var message string
                if isUnknown {
                        // 未知命令超时，提供更详细的建议
                        message = fmt.Sprintf("⏱️ 命令执行超时（%d秒）。\n\n"+
                                "此命令不在已知命令列表中，系统尝试同步执行但超时。\n\n"+
                                "建议：\n"+
                                "1. 如果这是一个长时间运行的命令，请使用异步模式：\n"+
                                "   smart_shell(command, async=true)\n\n"+
                                "2. 如果此命令应该快速完成但卡住了，请检查命令是否正确。", timeout)
                } else {
                        message = fmt.Sprintf("⏱️ 命令执行超时（%d秒）。建议使用异步模式：smart_shell(command, async=true)", timeout)
                }
                response := map[string]interface{}{
                        "mode":            "timeout",
                        "command":         command,
                        "timeout_seconds": timeout,
                        "is_unknown_cmd":  isUnknown,
                        "message":         message,
                }
                resultTOON, _ := toon.Marshal(response)
                return string(resultTOON), false
        }

        // 返回结果
        response := map[string]interface{}{
                "mode":       "sync",
                "command":    command,
                "stdout":     result.Stdout,
                "stderr":     result.Stderr,
                "exit_code":  result.ExitCode,
        }

        if result.Err != nil {
                response["error"] = result.Err.Error()
        }

        resultTOON, _ := toon.Marshal(response)
        return string(resultTOON), false
}

// handleSmartShellAsync 异步执行命令
func handleSmartShellAsync(command string, wakeAfterMinutes int, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        // 从 Channel 获取会话ID
        sessionID := ch.GetSessionID()

        // 启动后台任务
        task, err := globalTaskManager.StartDelayedExec(command, wakeAfterMinutes, "", sessionID)
        if err != nil {
                return fmt.Sprintf("Error: failed to start async execution: %v", err), false
        }

        // 返回任务信息
        result := map[string]interface{}{
                "mode":              "async",
                "task_id":           task.ID,
                "pid":               task.PID,
                "status":            "running",
                "command":           command,
                "wake_after_minutes": wakeAfterMinutes,
                "message": fmt.Sprintf("✅ 任务已异步启动（PID: %d），将在 %d 分钟后唤醒你。\n\n"+
                        "⏳ **重要提示**：你不需要轮询任务状态。\n"+
                        "系统会在 %d 分钟后主动通知你任务的执行结果。\n\n"+
                        "你可以继续处理其他工作。", task.PID, wakeAfterMinutes, wakeAfterMinutes),
        }

        resultTOON, _ := toon.Marshal(result)
        return string(resultTOON), false
}

// handleDelayedExec 处理延迟执行工具调用
func handleDelayedExec(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        // 获取命令
        command, ok := argsMap["command"].(string)
        if !ok || command == "" {
                return "Error: missing or invalid 'command' parameter", false
        }

        // 获取唤醒时间（分钟）
        wakeAfterMinutes := 5 // 默认 5 分钟
        if waf, ok := argsMap["wake_after_minutes"].(float64); ok {
                wakeAfterMinutes = int(waf)
        }

        // 获取描述（可选）
        description := ""
        if desc, ok := argsMap["description"].(string); ok {
                description = desc
        }

        // 从 Channel 获取会话ID（用于唤醒时找到正确的通道）
        sessionID := ch.GetSessionID()

        // 启动后台任务
        task, err := globalTaskManager.StartDelayedExec(command, wakeAfterMinutes, description, sessionID)
        if err != nil {
                return fmt.Sprintf("Error: failed to start delayed execution: %v", err), false
        }

        // 返回任务信息
        result := map[string]interface{}{
                "task_id":            task.ID,
                "pid":                task.PID,
                "status":             "running",
                "command":            command,
                "wake_after_minutes": wakeAfterMinutes,
                "message": fmt.Sprintf("✅ 任务已启动（PID: %d），将在 %d 分钟后唤醒你。\n\n"+
                        "⏳ **重要提示**：你现在不需要调用 check 工具轮询任务状态。\n"+
                        "系统会在 %d 分钟后主动通知你任务的执行结果。\n\n"+
                        "你可以继续处理其他工作。如需提前检查或终止，可使用：\n"+
                        "• shell_delayed_check - 检查任务状态（不建议频繁调用）\n"+
                        "• shell_delayed_wait - 延长等待时间\n"+
                        "• shell_delayed_terminate - 终止任务", task.PID, wakeAfterMinutes, wakeAfterMinutes),
        }

        resultTOON, _ := toon.Marshal(result)
        return string(resultTOON), false
}

// handleTaskCheck 处理任务检查工具调用
func handleTaskCheck(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        // 获取任务ID
        taskID, ok := argsMap["task_id"].(string)
        if !ok || taskID == "" {
                return "Error: missing or invalid 'task_id' parameter", false
        }

        // 获取任务信息
        info, err := globalTaskManager.GetTaskInfo(taskID)
        if err != nil {
                return fmt.Sprintf("Error: %v", err), false
        }

        // 根据任务状态添加提示信息
        status := info["status"].(string)
        var message string
        switch status {
        case "running":
                runtimeMinutes := info["runtime_minutes"].(float64)
                message = fmt.Sprintf("\n\n⏳ 任务仍在运行中（已运行 %.1f 分钟）。\n\n"+
                        "📋 可选操作：\n"+
                        "• 如需继续等待：调用 shell_delayed_wait 工具设置等待时间，**然后停止检查，等待系统通知**\n"+
                        "• 如需终止任务：使用 shell_delayed_terminate 工具\n\n"+
                        "⚠️ **注意**：调用 wait 工具后，不要继续调用 check 工具轮询，系统会在唤醒时间主动通知你。", runtimeMinutes)
        case "completed":
                message = "\n\n✅ 任务已完成！退出码为 0。"
        case "failed":
                message = "\n\n❌ 任务执行失败。请检查 stderr 了解错误详情。"
        case "terminated":
                message = "\n\n⏹️ 任务已被终止。"
        }

        info["message"] = message

        resultTOON, _ := toon.Marshal(info)
        return string(resultTOON), false
}

// handleTaskTerminate 处理任务终止工具调用
func handleTaskTerminate(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        // 获取任务ID
        taskID, ok := argsMap["task_id"].(string)
        if !ok || taskID == "" {
                return "Error: missing or invalid 'task_id' parameter", false
        }

        // 获取是否强制终止
        force := false
        if f, ok := argsMap["force"].(bool); ok {
                force = f
        }

        // 终止任务
        err := globalTaskManager.TerminateTask(taskID, force)
        if err != nil {
                return fmt.Sprintf("Error: %v", err), false
        }

        forceStr := "优雅终止 (SIGTERM)"
        if force {
                forceStr = "强制终止 (SIGKILL)"
        }

        result := map[string]interface{}{
                "task_id":   taskID,
                "status":    "terminated",
                "method":    forceStr,
                "timestamp": time.Now().Format(time.RFC3339),
                "message":   fmt.Sprintf("任务 %s 已%s。", taskID, forceStr),
        }

        resultTOON, _ := toon.Marshal(result)
        return string(resultTOON), false
}

// handleTaskList 处理任务列表工具调用
func handleTaskList(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        tasks := globalTaskManager.ListTasks()

        if len(tasks) == 0 {
                return "当前没有后台任务。", false
        }

        // 构建任务列表摘要
        type taskSummary struct {
                ID          string    `toon:"task_id"`
                Command     string    `toon:"command"`
                Status      string    `toon:"status"`
                PID         int       `toon:"pid"`
                StartTime   time.Time `toon:"start_time"`
                RuntimeMin  float64   `toon:"runtime_minutes"`
                Description string    `toon:"description,omitempty"`
        }

        summaries := make([]taskSummary, 0, len(tasks))
        for _, task := range tasks {
                task.mu.RLock()
                summary := taskSummary{
                        ID:         task.ID,
                        Command:    task.Command,
                        Status:     string(task.Status),
                        PID:        task.PID,
                        StartTime:  task.StartTime,
                        RuntimeMin: time.Since(task.StartTime).Minutes(),
                }
                if task.Description != "" {
                        summary.Description = task.Description
                }
                task.mu.RUnlock()
                summaries = append(summaries, summary)
        }

        resultTOON, _ := toon.Marshal(summaries)
        return string(resultTOON), false
}

// handleTaskWait 处理继续等待工具调用
func handleTaskWait(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        // 获取任务ID
        taskID, ok := argsMap["task_id"].(string)
        if !ok || taskID == "" {
                return "Error: missing or invalid 'task_id' parameter", false
        }

        // 获取等待时间（分钟）
        waitMinutes := 5 // 默认 5 分钟
        if wm, ok := argsMap["wait_minutes"].(float64); ok {
                waitMinutes = int(wm)
        }

        // 延长唤醒时间
        err := globalTaskManager.ExtendWakeTime(taskID, waitMinutes)
        if err != nil {
                return fmt.Sprintf("Error: %v", err), false
        }

        nextWakeTime := time.Now().Add(time.Duration(waitMinutes) * time.Minute)

        result := map[string]interface{}{
                "task_id":         taskID,
                "status":          "waiting",
                "wait_minutes":    waitMinutes,
                "next_wake_after": nextWakeTime.Format(time.RFC3339),
                "message": fmt.Sprintf("✅ 已设置 %d 分钟后唤醒（预计时间: %s）。\n\n"+
                        "⏳ **重要提示**：你现在不需要再调用任何任务相关工具（check/wait）。\n"+
                        "系统会在任务完成或到达唤醒时间时主动通知你。\n\n"+
                        "你可以继续处理其他工作，或向用户报告当前状态。", waitMinutes, nextWakeTime.Format("15:04:05")),
        }

        resultTOON, _ := toon.Marshal(result)
        return string(resultTOON), false
}

// handleTaskRemove 处理移除任务工具调用
func handleTaskRemove(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查任务管理器是否已初始化
        if globalTaskManager == nil {
                return "Error: task manager not initialized", false
        }

        // 获取任务ID
        taskID, ok := argsMap["task_id"].(string)
        if !ok || taskID == "" {
                return "Error: missing or invalid 'task_id' parameter", false
        }

        // 移除任务
        err := globalTaskManager.RemoveTask(taskID)
        if err != nil {
                return fmt.Sprintf("Error: %v", err), false
        }

        return fmt.Sprintf("任务 %s 已从列表中移除。", taskID), false
}

// GetTaskWakeMessage 生成任务唤醒消息
func GetTaskWakeMessage(task *BackgroundTask) string {
        task.mu.RLock()
        defer task.mu.RUnlock()

        var statusEmoji string
        switch task.Status {
        case BgTaskRunning:
                statusEmoji = "⏳"
        case BgTaskCompleted:
                statusEmoji = "✅"
        case BgTaskFailed:
                statusEmoji = "❌"
        case BgTaskTerminated:
                statusEmoji = "⏹️"
        default:
                statusEmoji = "❓"
        }

        runtime := time.Since(task.StartTime)

        msg := fmt.Sprintf(`
⏰ 任务唤醒通知

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
%s 任务ID: %s
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📋 基本信息:
  • 命令: %s
  • 进程ID: %d
  • 状态: %s
  • 已运行: %.1f 分钟
`, statusEmoji, task.ID, task.Command, task.PID, task.Status, runtime.Minutes())

        if task.Description != "" {
                msg += fmt.Sprintf("  • 描述: %s\n", task.Description)
        }

        // 截断输出显示
        stdout := truncateTaskOutput(task.Stdout.String())
        stderr := truncateTaskOutput(task.Stderr.String())

        if stdout != "" {
                msg += fmt.Sprintf("\n📤 标准输出:\n%s\n", stdout)
        }

        if stderr != "" {
                msg += fmt.Sprintf("\n⚠️ 标准错误:\n%s\n", stderr)
        }

        // 根据状态提供操作建议
        switch task.Status {
        case BgTaskRunning:
                msg += `
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
可执行操作:
  1. 继续等待 - 告诉我等待多少分钟（如 "等待10分钟"）
  2. 检查结果 - 使用 task_check 工具
  3. 终止任务 - 使用 task_terminate 工具
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━`
        case BgTaskCompleted:
                msg += `
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ 任务已成功完成！
如需清理任务记录，可使用 task_remove 工具。
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━`
        case BgTaskFailed:
                msg += `
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
❌ 任务执行失败，请检查上方输出了解错误原因。
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━`
        }

        return msg
}
