package main

import (
        "bytes"
        "context"
        "crypto/md5"
        "fmt"
        "log"
        "os"
        "os/exec"
        "path/filepath"
        "strings"
        "sync"
        "syscall"
        "time"

        "github.com/google/uuid"
)

// BackgroundTaskStatus 后台任务状态类型
type BackgroundTaskStatus string

const (
        BgTaskRunning    BackgroundTaskStatus = "running"
        BgTaskCompleted  BackgroundTaskStatus = "completed"
        BgTaskFailed     BackgroundTaskStatus = "failed"
        BgTaskTerminated BackgroundTaskStatus = "terminated"
)

// BackgroundTask 后台任务
type BackgroundTask struct {
        ID          string              `json:"id"`
        Command     string              `json:"command"`
        Description string              `json:"description,omitempty"`
        PID         int                 `json:"pid"`
        StartTime   time.Time           `json:"start_time"`
        Status      BackgroundTaskStatus `json:"status"`
        ExitCode    int                 `json:"exit_code,omitempty"`
        Stdout      *safeBuffer         `json:"-"`
        Stderr      *safeBuffer         `json:"-"`
        WakeAfter   time.Time           `json:"wake_after"`
        SessionID   string              `json:"session_id,omitempty"`

        mu       sync.RWMutex
        process  *os.Process
        done     chan struct{}
        wakeSent bool
}

// safeBuffer 线程安全的缓冲区
type safeBuffer struct {
        mu  sync.RWMutex
        buf bytes.Buffer
}

func (sb *safeBuffer) Write(p []byte) (int, error) {
        sb.mu.Lock()
        defer sb.mu.Unlock()
        return sb.buf.Write(p)
}

func (sb *safeBuffer) String() string {
        sb.mu.RLock()
        defer sb.mu.RUnlock()
        return sb.buf.String()
}

func (sb *safeBuffer) Len() int {
        sb.mu.RLock()
        defer sb.mu.RUnlock()
        return sb.buf.Len()
}

// TaskManager 后台任务管理器
type TaskManager struct {
        tasks       map[string]*BackgroundTask
        mu          sync.RWMutex
        wakeChan    chan string
        wakeHandler WakeHandlerFunc
        ctx         context.Context
        cancel      context.CancelFunc
}

// WakeHandlerFunc 唤醒处理函数类型
type WakeHandlerFunc func(task *BackgroundTask)

// NewTaskManager 创建任务管理器
func NewTaskManager() *TaskManager {
        ctx, cancel := context.WithCancel(context.Background())
        tm := &TaskManager{
                tasks:    make(map[string]*BackgroundTask),
                wakeChan: make(chan string, 100),
                ctx:      ctx,
                cancel:   cancel,
        }
        go tm.wakeScheduler()
        return tm
}

// generateTaskID 生成任务ID
func generateTaskID() string {
        id := uuid.New()
        return "task_" + id.String()[:8]
}

// StartDelayedExec 启动延迟执行任务
func (tm *TaskManager) StartDelayedExec(command string, wakeAfterMinutes int, description string, sessionID string) (*BackgroundTask, error) {
        if wakeAfterMinutes < 1 {
                wakeAfterMinutes = 1
        }
        if wakeAfterMinutes > 1440 {
                wakeAfterMinutes = 1440
        }

        taskID := generateTaskID()

        task := &BackgroundTask{
                ID:          taskID,
                Command:     command,
                Description: description,
                StartTime:   time.Now(),
                Status:      BgTaskRunning,
                Stdout:      &safeBuffer{},
                Stderr:      &safeBuffer{},
                WakeAfter:   time.Now().Add(time.Duration(wakeAfterMinutes) * time.Minute),
                SessionID:   sessionID,
                done:        make(chan struct{}),
        }

        cmd := exec.CommandContext(tm.ctx, "sh", "-c", command)
        cmd.Stdout = task.Stdout
        cmd.Stderr = task.Stderr
        cmd.SysProcAttr = getSysProcAttr()

        if err := cmd.Start(); err != nil {
                return nil, fmt.Errorf("failed to start command: %w", err)
        }

        task.process = cmd.Process
        task.PID = cmd.Process.Pid

        tm.mu.Lock()
        tm.tasks[taskID] = task
        tm.mu.Unlock()

        go tm.monitorTask(task, cmd)

        log.Printf("[TaskManager] Task %s started, PID: %d, wake after %d minutes", taskID, task.PID, wakeAfterMinutes)

        return task, nil
}

// monitorTask 监控任务执行
func (tm *TaskManager) monitorTask(task *BackgroundTask, cmd *exec.Cmd) {
        defer close(task.done)

        err := cmd.Wait()

        task.mu.Lock()
        defer task.mu.Unlock()

        if err != nil {
                if exitErr, ok := err.(*exec.ExitError); ok {
                        task.ExitCode = exitErr.ExitCode()
                        if task.Status == BgTaskRunning {
                                task.Status = BgTaskFailed
                        }
                } else if task.Status == BgTaskRunning {
                        task.Status = BgTaskFailed
                        task.ExitCode = -1
                }
        } else {
                task.ExitCode = 0
                if task.Status == BgTaskRunning {
                        task.Status = BgTaskCompleted
                }
        }

        log.Printf("[TaskManager] Task %s finished with status: %s, exit code: %d", task.ID, task.Status, task.ExitCode)

        if !task.wakeSent && (task.Status == BgTaskCompleted || task.Status == BgTaskFailed) {
                select {
                case tm.wakeChan <- task.ID:
                        log.Printf("[TaskManager] Task %s finished, triggering immediate wake notification", task.ID)
                default:
                        log.Printf("[TaskManager] Wake channel full, cannot send immediate wake for task %s", task.ID)
                }
        }
}

// wakeScheduler 唤醒调度器
func (tm *TaskManager) wakeScheduler() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()

        for {
                select {
                case <-tm.ctx.Done():
                        return
                case <-ticker.C:
                        tm.checkWakeUps()
                case taskID := <-tm.wakeChan:
                        tm.processWakeUp(taskID)
                }
        }
}

// checkWakeUps 检查是否需要唤醒
func (tm *TaskManager) checkWakeUps() {
        tm.mu.RLock()
        defer tm.mu.RUnlock()

        now := time.Now()
        for _, task := range tm.tasks {
                task.mu.RLock()
                shouldWake := task.Status == BgTaskRunning &&
                        now.After(task.WakeAfter) &&
                        !task.wakeSent
                task.mu.RUnlock()

                if shouldWake {
                        select {
                        case tm.wakeChan <- task.ID:
                        default:
                                log.Printf("[TaskManager] Wake channel full, skipping wake for task %s", task.ID)
                        }
                }
        }
}

// processWakeUp 处理唤醒
func (tm *TaskManager) processWakeUp(taskID string) {
        tm.mu.RLock()
        task, exists := tm.tasks[taskID]
        tm.mu.RUnlock()

        if !exists {
                return
        }

        task.mu.Lock()
        if task.wakeSent {
                task.mu.Unlock()
                return
        }
        task.wakeSent = true
        task.mu.Unlock()

        log.Printf("[TaskManager] Waking up for task %s", taskID)

        if tm.wakeHandler != nil {
                tm.wakeHandler(task)
        }
}

// SetWakeHandler 设置唤醒处理函数
func (tm *TaskManager) SetWakeHandler(handler WakeHandlerFunc) {
        tm.wakeHandler = handler
}

// CheckTask 检查任务状态
func (tm *TaskManager) CheckTask(taskID string) (*BackgroundTask, error) {
        tm.mu.RLock()
        defer tm.mu.RUnlock()

        task, exists := tm.tasks[taskID]
        if !exists {
                return nil, fmt.Errorf("task %s not found", taskID)
        }

        return task, nil
}

// TerminateTask 终止任务
func (tm *TaskManager) TerminateTask(taskID string, force bool) error {
        tm.mu.RLock()
        task, exists := tm.tasks[taskID]
        tm.mu.RUnlock()

        if !exists {
                return fmt.Errorf("task %s not found", taskID)
        }

        task.mu.Lock()
        defer task.mu.Unlock()

        if task.Status != BgTaskRunning {
                return fmt.Errorf("task %s is not running (status: %s)", taskID, task.Status)
        }

        if task.process == nil {
                return fmt.Errorf("task %s has no process", taskID)
        }

        var err error
        if force {
                err = killProcessGroup(task.PID)
                log.Printf("[TaskManager] Force killing task %s (PID: %d)", taskID, task.PID)
        } else {
                err = terminateProcessGroup(task.PID)
                log.Printf("[TaskManager] Terminating task %s (PID: %d)", taskID, task.PID)
        }

        task.Status = BgTaskTerminated

        return err
}

// ListTasks 列出所有任务
func (tm *TaskManager) ListTasks() []*BackgroundTask {
        tm.mu.RLock()
        defer tm.mu.RUnlock()

        tasks := make([]*BackgroundTask, 0, len(tm.tasks))
        for _, task := range tm.tasks {
                tasks = append(tasks, task)
        }
        return tasks
}

// GetTaskInfo 获取任务信息
func (tm *TaskManager) GetTaskInfo(taskID string) (map[string]interface{}, error) {
        task, err := tm.CheckTask(taskID)
        if err != nil {
                return nil, err
        }

        task.mu.RLock()
        defer task.mu.RUnlock()

        info := map[string]interface{}{
                "task_id":         task.ID,
                "command":         task.Command,
                "description":     task.Description,
                "pid":             task.PID,
                "status":          string(task.Status),
                "exit_code":       task.ExitCode,
                "start_time":      task.StartTime.Format(time.RFC3339),
                "runtime_minutes": time.Since(task.StartTime).Minutes(),
                "stdout":          truncateTaskOutput(task.Stdout.String()),
                "stderr":          truncateTaskOutput(task.Stderr.String()),
        }

        return info, nil
}

// RemoveTask 移除已完成或已终止的任务
func (tm *TaskManager) RemoveTask(taskID string) error {
        tm.mu.Lock()
        defer tm.mu.Unlock()

        task, exists := tm.tasks[taskID]
        if !exists {
                return fmt.Errorf("task %s not found", taskID)
        }

        task.mu.RLock()
        status := task.Status
        task.mu.RUnlock()

        if status == BgTaskRunning {
                return fmt.Errorf("cannot remove running task %s, terminate it first", taskID)
        }

        delete(tm.tasks, taskID)
        log.Printf("[TaskManager] Task %s removed", taskID)
        return nil
}

// Stop 停止任务管理器
func (tm *TaskManager) Stop() {
        tm.cancel()

        tm.mu.Lock()
        defer tm.mu.Unlock()

        for _, task := range tm.tasks {
                task.mu.Lock()
                if task.Status == BgTaskRunning && task.process != nil {
                        killProcessGroup(task.PID)
                        task.Status = BgTaskTerminated
                }
                task.mu.Unlock()
        }
}

// ExtendWakeTime 延长唤醒时间
func (tm *TaskManager) ExtendWakeTime(taskID string, additionalMinutes int) error {
        tm.mu.RLock()
        task, exists := tm.tasks[taskID]
        tm.mu.RUnlock()

        if !exists {
                return fmt.Errorf("task %s not found", taskID)
        }

        task.mu.Lock()
        defer task.mu.Unlock()

        if task.Status != BgTaskRunning {
                return fmt.Errorf("task %s is not running", taskID)
        }

        if additionalMinutes < 1 {
                additionalMinutes = 1
        }
        if additionalMinutes > 1440 {
                additionalMinutes = 1440
        }

        task.WakeAfter = time.Now().Add(time.Duration(additionalMinutes) * time.Minute)
        task.wakeSent = false

        log.Printf("[TaskManager] Task %s wake time extended by %d minutes", taskID, additionalMinutes)
        return nil
}

// truncateTaskOutput 截断任务输出（用于 info 接口）
func truncateTaskOutput(output string) string {
        const maxLen = 10000
        if len(output) > maxLen {
                return output[:maxLen] + "\n... (output truncated)"
        }
        return output
}

// tailLines 返回字符串的最后 n 行
func tailLines(s string, n int) string {
        lines := strings.Split(s, "\n")
        if len(lines) <= n {
                return s
        }
        return strings.Join(lines[len(lines)-n:], "\n")
}

// saveOutputToFileForWake 将过长内容保存到文件（供唤醒消息使用）
func saveOutputToFileForWake(content, prefix, command string) (string, error) {
        const maxDirectOutput = 1000 // 与 task_tools.go 保持一致
        if len(content) <= maxDirectOutput {
                return "", nil
        }

        outputDir := filepath.Join(globalExecDir, "output")
        if err := os.MkdirAll(outputDir, 0755); err != nil {
                return "", err
        }

        timestamp := time.Now().UnixNano()
        hash := md5.Sum([]byte(command))
        safeCmd := fmt.Sprintf("%x", hash)[:8]
        filename := fmt.Sprintf("%s_%d_%s.txt", prefix, timestamp, safeCmd)
        filePath := filepath.Join(outputDir, filename)

        if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
                return "", err
        }
        return filePath, nil
}

// GetTaskWakeMessage 生成任务唤醒消息（含大输出保存和尾部展示）
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

        // 处理 stdout
        stdout := task.Stdout.String()
        stdoutFile, err := saveOutputToFileForWake(stdout, "async_stdout", task.Command)
        if err == nil && stdoutFile != "" {
                tail := tailLines(stdout, 10)
                msg += fmt.Sprintf("\n📤 标准输出:\n[输出过长，完整内容已保存至: %s]\n\n--- 最后 10 行 ---\n%s\n--- 结束 ---\n原始长度: %d 字符\n",
                        stdoutFile, tail, len(stdout))
        } else if len(stdout) > 1000 {
                tail := tailLines(stdout, 10)
                msg += fmt.Sprintf("\n📤 标准输出:\n[输出过长已截断（无法保存文件）]\n\n--- 最后 10 行 ---\n%s\n--- 结束 ---\n原始长度: %d 字符\n",
                        tail, len(stdout))
        } else if stdout != "" {
                msg += fmt.Sprintf("\n📤 标准输出:\n%s\n", stdout)
        }

        // 处理 stderr
        stderr := task.Stderr.String()
        stderrFile, err := saveOutputToFileForWake(stderr, "async_stderr", task.Command)
        if err == nil && stderrFile != "" {
                tail := tailLines(stderr, 10)
                msg += fmt.Sprintf("\n⚠️ 标准错误:\n[输出过长，完整内容已保存至: %s]\n\n--- 最后 10 行 ---\n%s\n--- 结束 ---\n原始长度: %d 字符\n",
                        stderrFile, tail, len(stderr))
        } else if len(stderr) > 1000 {
                tail := tailLines(stderr, 10)
                msg += fmt.Sprintf("\n⚠️ 标准错误:\n[输出过长已截断（无法保存文件）]\n\n--- 最后 10 行 ---\n%s\n--- 结束 ---\n原始长度: %d 字符\n",
                        tail, len(stderr))
        } else if stderr != "" {
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

// ==================== 命令检测相关 ====================

// CommandSuggestion 命令类型检测结果
type CommandSuggestion struct {
        Type             string `json:"type"`
        Message          string `json:"message"`
        Suggestion       string `json:"suggestion,omitempty"`
        NonInteractiveEq string `json:"non_interactive_eq,omitempty"`
}

// DetectCommandType 检测命令类型
func DetectCommandType(command string) CommandSuggestion {
        lowerCmd := strings.ToLower(command)

        // 快速命令白名单
        quickPatterns := []string{
                "ls", "cat ", "head ", "tail ", "wc ", "touch ", "file ",
                "mkdir ", "rmdir ", "rm ", "cp ", "mv ", "ln ",
                "echo ", "printf ", "grep ", "sed ", "awk ", "cut ", "sort ", "uniq ",
                "pwd", "whoami", "hostname", "uname", "date", "uptime", "df ", "du ",
                "ps ", "pgrep ", "pkill ", "kill ", "killall ",
                "ping -c ", "host ", "nslookup ", "dig ", "ip ", "ifconfig",
                "which ", "whereis ", "type ", "stat ", "realpath ", "readlink ",
                "git status", "git log", "git diff", "git branch", "git remote",
                "git rev", "git show", "git tag",
                "env", "export ", "printenv", "set ", "unset ",
                "expr ", "bc ", "let ",
        }
        for _, p := range quickPatterns {
                if strings.Contains(lowerCmd, p) {
                        return CommandSuggestion{Type: "quick", Message: "快速命令，将同步执行"}
                }
        }

        // SSH/SCP 特殊检测 - 需要区分是否真正交互式
        if strings.Contains(lowerCmd, "ssh ") || strings.Contains(lowerCmd, "scp ") {
                // 检查是否使用 sshpass（非交互式密码输入）
                hasSshpass := strings.Contains(lowerCmd, "sshpass")

                // 检查 SSH 是否带有远程命令（非交互式）
                // 模式: ssh [options] user@host 'command' 或 ssh [options] user@host "command"
                hasRemoteCommand := false
                if strings.Contains(lowerCmd, "ssh ") {
                        // 查找用户@主机后面的命令参数
                        // 常见模式: ssh ... user@host 'cmd' 或 ssh ... user@host "cmd"
                        sshIdx := strings.Index(lowerCmd, "ssh ")
                        afterSsh := lowerCmd[sshIdx:]

                        // 检查是否有单引号或双引号包裹的远程命令
                        // 注意: sshpass -p 'password' 中的引号是密码的一部分，不是远程命令
                        // 远程命令通常出现在 user@host 之后

                        // 简化检测: 如果命令中包含 @ 并且在 @ 后面有引号，则认为是远程命令
                        atIdx := strings.Index(afterSsh, "@")
                        if atIdx > 0 {
                                afterAt := afterSsh[atIdx:]
                                // 检查 @ 后面是否有引号（排除 sshpass -p 'xxx' 中的引号）
                                // 远程命令通常格式: user@host 'command' 或 user@host "command"
                                singleQuoteIdx := strings.Index(afterAt, "'")
                                doubleQuoteIdx := strings.Index(afterAt, "\"")

                                // 如果有引号，检查引号位置是否合理（在主机名后面）
                                if singleQuoteIdx > 0 || doubleQuoteIdx > 0 {
                                        // 找到引号后，检查引号内是否有内容（远程命令）
                                        // 这是一个简化的检测，假设 @ 后面的引号包含远程命令
                                        hasRemoteCommand = true
                                }
                        }
                }

                // 判断逻辑：
                // 1. 使用 sshpass 且有远程命令 → 快速命令
                // 2. 有远程命令（无论是否有 sshpass）→ 快速命令（SSH 会执行完命令后自动退出）
                // 3. 只有 ssh user@host（无远程命令）→ 交互式
                if strings.Contains(lowerCmd, "ssh ") {
                        if hasRemoteCommand {
                                // SSH 带远程命令，非交互式
                                return CommandSuggestion{Type: "quick", Message: "SSH 远程命令，将同步执行"}
                        } else if hasSshpass {
                                // 有 sshpass 但没检测到远程命令，可能是配置问题，但允许同步执行
                                return CommandSuggestion{Type: "quick", Message: "SSH 命令（已使用 sshpass），将同步执行"}
                        }
                        // 纯 SSH 不带远程命令，交互式
                        return CommandSuggestion{
                                Type:             "interactive",
                                Message:          "ssh 不带命令会进入交互式 shell",
                                Suggestion:       "使用 sshpass 或密钥认证，并添加远程命令",
                                NonInteractiveEq: "sshpass -p 'password' ssh user@host 'command'",
                        }
                }

                // SCP 检测
                if strings.Contains(lowerCmd, "scp ") {
                        if hasSshpass {
                                return CommandSuggestion{Type: "long_running", Message: "SCP 文件传输（已使用 sshpass），将异步执行"}
                        }
                        return CommandSuggestion{
                                Type:             "interactive",
                                Message:          "scp 需要密码",
                                Suggestion:       "使用 sshpass 或密钥认证",
                                NonInteractiveEq: "sshpass -p 'password' scp",
                        }
                }
        }

        // 交互式命令检测
        interactiveMap := map[string]CommandSuggestion{
                "vim":     {Type: "interactive", Message: "vim 是交互式编辑器", Suggestion: "使用 sed/awk 进行文本处理"},
                "nano":    {Type: "interactive", Message: "nano 是交互式编辑器", Suggestion: "使用 sed 进行文本替换"},
                "less":    {Type: "interactive", Message: "less 是分页器", Suggestion: "使用 cat 或 head -n 100 查看", NonInteractiveEq: "cat"},
                "more":    {Type: "interactive", Message: "more 是分页器", Suggestion: "使用 cat 查看", NonInteractiveEq: "cat"},
                "top":     {Type: "interactive", Message: "top 是交互式监控", Suggestion: "使用 top -b -n 1 或 ps aux", NonInteractiveEq: "top -b -n 1"},
                "htop":    {Type: "interactive", Message: "htop 是交互式监控", Suggestion: "使用 top -b -n 1 或 ps aux"},
                "git log": {Type: "interactive", Message: "git log 会分页", Suggestion: "使用 git --no-pager log -n 20", NonInteractiveEq: "git --no-pager log -n 20"},
                "git diff": {Type: "interactive", Message: "git diff 会分页", Suggestion: "使用 git --no-pager diff", NonInteractiveEq: "git --no-pager diff"},
                "git commit": {Type: "interactive", Message: "git commit 会打开编辑器", Suggestion: "使用 git commit -m \"message\"", NonInteractiveEq: "git commit -m \"\""},
                "python":  {Type: "interactive", Message: "python 无参数会进入 REPL", Suggestion: "使用 python script.py 或 python -c 'code'"},
                "python3": {Type: "interactive", Message: "python3 无参数会进入 REPL", Suggestion: "使用 python3 script.py 或 python3 -c 'code'"},
                "node":    {Type: "interactive", Message: "node 无参数会进入 REPL", Suggestion: "使用 node script.js 或 node -e 'code'"},
                "sudo -i": {Type: "interactive", Message: "sudo -i 会启动 root shell", Suggestion: "使用 sudo command"},
                "sudo su": {Type: "interactive", Message: "sudo su 会启动交互式 shell", Suggestion: "使用 sudo command"},
                "su ":     {Type: "interactive", Message: "su 会启动交互式 shell", Suggestion: "使用 sudo command"},
                "screen":  {Type: "interactive", Message: "screen 是终端复用器", Suggestion: "需要交互"},
                "tmux":    {Type: "interactive", Message: "tmux 是终端复用器", Suggestion: "需要交互"},
        }
        for pattern, sug := range interactiveMap {
                if strings.Contains(lowerCmd, pattern) {
                        return sug
                }
        }

        // 长时间运行命令
        longPatterns := []string{
                "apt update", "apt upgrade", "apt install", "apt-get",
                "yum update", "yum upgrade", "yum install",
                "dnf update", "dnf upgrade", "dnf install",
                "pacman -S", "pacman -Syu",
                "pkg install", "pkg update", "pkg upgrade",
                "portsnap fetch", "portsnap extract", "portsnap update",
                "freebsd-update fetch", "freebsd-update install",
                "make", "cmake", "ninja",
                "npm install", "npm update", "npm run build",
                "yarn install", "yarn build",
                "pnpm install", "pnpm build",
                "pip install", "pip3 install",
                "cargo build", "cargo install",
                "go build", "go install", "go get",
                "docker build", "docker-compose build",
                "git clone", "git fetch", "git pull --rebase",
                "rsync", "scp ", "sftp ",
                "wget ", "curl -O", "curl -o",
                "tar ", "unzip ", "7z ",
                "ffmpeg", "handbrake",
                "systemctl start", "systemctl restart",
                "service ", "/etc/init.d/",
        }
        for _, p := range longPatterns {
                if strings.Contains(lowerCmd, p) {
                        return CommandSuggestion{Type: "long_running", Message: fmt.Sprintf("检测到长时命令: %s，将异步执行", p)}
                }
        }

        return CommandSuggestion{Type: "unknown", Message: "未知命令类型，将使用保守策略执行"}
}

// ==================== 平台相关函数 ====================
func getSysProcAttr() *syscall.SysProcAttr {
        return &syscall.SysProcAttr{
                Setpgid: true,
        }
}

func killProcessGroup(pid int) error {
        return syscall.Kill(-pid, syscall.SIGKILL)
}

func terminateProcessGroup(pid int) error {
        return syscall.Kill(-pid, syscall.SIGTERM)
}
