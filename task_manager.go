package main

import (
        "bytes"
        "context"
        "fmt"
        "log"
        "os"
        "os/exec"
        "strings"
        "sync"
        "syscall"
        "time"

        "github.com/google/uuid"
)

// BackgroundTaskStatus 后台任务状态类型（区别于 message.go 中的 BackgroundTaskStatus）
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
        SessionID   string              `json:"session_id,omitempty"` // 关联的会话ID，用于唤醒时找到正确的通道

        mu       sync.RWMutex
        process  *os.Process
        done     chan struct{}
        wakeSent bool // 是否已发送过唤醒通知
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
        wakeChan    chan string        // 用于通知有任务需要唤醒
        wakeHandler WakeHandlerFunc    // 唤醒处理函数
        ctx         context.Context    // 上下文
        cancel      context.CancelFunc // 取消函数
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
        // 验证最小唤醒时间
        if wakeAfterMinutes < 1 {
                wakeAfterMinutes = 1
        }
        // 最大唤醒时间 24 小时
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

        // 启动命令
        cmd := exec.CommandContext(tm.ctx, "sh", "-c", command)
        cmd.Stdout = task.Stdout
        cmd.Stderr = task.Stderr

        // 设置进程组，便于终止整个进程树
        cmd.SysProcAttr = getSysProcAttr()

        if err := cmd.Start(); err != nil {
                return nil, fmt.Errorf("failed to start command: %w", err)
        }

        task.process = cmd.Process
        task.PID = cmd.Process.Pid

        tm.mu.Lock()
        tm.tasks[taskID] = task
        tm.mu.Unlock()

        // 启动监控协程
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

        // 任务完成或失败时立即触发唤醒通知
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

        // 调用唤醒处理函数
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
                // 强制终止 - SIGKILL
                err = killProcessGroup(task.PID)
                log.Printf("[TaskManager] Force killing task %s (PID: %d)", taskID, task.PID)
        } else {
                // 优雅终止 - SIGTERM
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

// GetTaskInfo 获取任务信息（用于返回给模型）
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

        // 终止所有运行中的任务
        for _, task := range tm.tasks {
                task.mu.Lock()
                if task.Status == BgTaskRunning && task.process != nil {
                        killProcessGroup(task.PID)
                        task.Status = BgTaskTerminated
                }
                task.mu.Unlock()
        }
}

// ExtendWakeTime 延长唤醒时间（模型决定继续等待时使用）
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

        // 验证时间范围
        if additionalMinutes < 1 {
                additionalMinutes = 1
        }
        if additionalMinutes > 1440 {
                additionalMinutes = 1440
        }

        task.WakeAfter = time.Now().Add(time.Duration(additionalMinutes) * time.Minute)
        task.wakeSent = false // 重置唤醒标志

        log.Printf("[TaskManager] Task %s wake time extended by %d minutes", taskID, additionalMinutes)
        return nil
}

// truncateTaskOutput 截断任务输出
func truncateTaskOutput(output string) string {
        const maxLen = 10000
        if len(output) > maxLen {
                return output[:maxLen] + "\n... (output truncated)"
        }
        return output
}

// 命令类型检测
type CommandSuggestion struct {
        Type             string `json:"type"`
        Message          string `json:"message"`
        Suggestion       string `json:"suggestion,omitempty"`         // 非交互式替代建议
        NonInteractiveEq string `json:"non_interactive_eq,omitempty"` // 非交互式等价命令
}

// ArgPatternType 参数模式类型
// 定义如何检测参数使交互式命令变成非交互式
type ArgPatternType string

const (
        ArgPatternFlag        ArgPatternType = "flag"         // 存在某个标志参数，如 --no-pager
        ArgPatternFlagValue   ArgPatternType = "flag_value"   // 标志+值，如 -n 10
        ArgPatternFileExt     ArgPatternType = "file_ext"     // 文件扩展名，如 .py, .js
        ArgPatternContains    ArgPatternType = "contains"     // 命令包含某字符串，如 sshpass
        ArgPatternHasArgAfter ArgPatternType = "has_arg_after" // 主机名后有参数（SSH专用）
        ArgPatternNoArgs      ArgPatternType = "no_args"      // 无参数时才是交互式
)

// ArgPattern 非交互式参数模式
// 描述什么参数使命令变成非交互式
type ArgPattern struct {
        Type    ArgPatternType // 模式类型
        Pattern string         // 匹配模式
}

// InteractiveCommandInfo 交互式命令信息
type InteractiveCommandInfo struct {
        Pattern             string       // 检测模式
        Suggestion          string       // 变换建议
        NonInteractiveCmd   string       // 非交互式等价命令模板
        NonInteractiveArgs  []ArgPattern // 非交互式参数模式（任一匹配则为非交互式）
}

// interactiveCommands 交互式命令定义
// 使用声明式配置，每个命令定义"什么参数让它变成非交互式"
var interactiveCommands = map[string]InteractiveCommandInfo{
        // 编辑器类 - 无参数也是交互式，无解
        "vim": {
                Pattern:    "vim",
                Suggestion: "vim 是交互式编辑器。建议使用 sed/awk 进行文本处理，或 cat/head/tail 查看。",
        },
        "nano": {
                Pattern:    "nano",
                Suggestion: "nano 是交互式编辑器。建议使用 sed 进行文本替换。",
        },
        "vi ": {
                Pattern:    "vi ",
                Suggestion: "vi 是交互式编辑器。建议使用 sed 进行文本处理。",
        },
        
        // 分页器类 - 有文件参数时非交互
        "less": {
                Pattern:           "less",
                Suggestion:        "less 是分页器。建议使用 cat 或 head -n 100 查看。",
                NonInteractiveCmd: "cat",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternNoArgs, Pattern: ""}, // 无参数时才是交互式
                },
        },
        "more": {
                Pattern:           "more",
                Suggestion:        "more 是分页器。建议使用 cat 查看。",
                NonInteractiveCmd: "cat",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternNoArgs, Pattern: ""},
                },
        },
        
        // 系统监控类 - 有 -b 或 -n 参数时非交互
        "top": {
                Pattern:           "top",
                Suggestion:        "top 是交互式进程监控。建议使用 top -b -n 1 或 ps aux。",
                NonInteractiveCmd: "top -b -n 1",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternFlag, Pattern: "-b"},
                        {Type: ArgPatternFlag, Pattern: "-n"},
                },
        },
        "htop": {
                Pattern:    "htop",
                Suggestion: "htop 是交互式监控。建议使用 top -b -n 1 或 ps aux。",
        },
        
        // Git - 有 --no-pager 或 -n 时非交互
        "git log": {
                Pattern:           "git log",
                Suggestion:        "git log 会分页。建议使用 git --no-pager log -n 20。",
                NonInteractiveCmd: "git --no-pager log -n 20",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternContains, Pattern: "--no-pager"},
                        {Type: ArgPatternFlagValue, Pattern: "-n"},
                        {Type: ArgPatternFlag, Pattern: "--oneline"},
                },
        },
        "git diff": {
                Pattern:           "git diff",
                Suggestion:        "git diff 会分页。建议使用 git --no-pager diff。",
                NonInteractiveCmd: "git --no-pager diff",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternContains, Pattern: "--no-pager"},
                },
        },
        "git commit": {
                Pattern:           "git commit",
                Suggestion:        "git commit 会打开编辑器。建议使用 git commit -m \"message\"。",
                NonInteractiveCmd: "git commit -m \"\"",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternFlagValue, Pattern: "-m"},
                        {Type: ArgPatternFlag, Pattern: "-F"}, // -F file
                        {Type: ArgPatternFlag, Pattern: "-C"}, // -C commit
                },
        },
        
        // SSH - 有命令参数或使用 sshpass 时非交互
        "ssh ": {
                Pattern:           "ssh ",
                Suggestion:        "ssh 不带命令会进入交互式 shell。使用方式：\n1. sshpass -p 'password' ssh user@host 'command'\n2. ssh user@host 'command'（需密钥认证）",
                NonInteractiveCmd: "sshpass -p 'password' ssh user@host 'command'",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternContains, Pattern: "sshpass"},
                        {Type: ArgPatternHasArgAfter, Pattern: ""}, // 主机后有参数
                },
        },
        "scp ": {
                Pattern:           "scp ",
                Suggestion:        "scp 需要密码。建议使用 sshpass -p 'password' scp 或配置密钥。",
                NonInteractiveCmd: "sshpass -p 'password' scp",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternContains, Pattern: "sshpass"},
                },
        },
        
        // REPL 类 - 有文件参数或 -c/-e 时非交互
        "python": {
                Pattern:           "python",
                Suggestion:        "python 无参数会进入 REPL。建议使用 python script.py 或 python -c 'code'。",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternFileExt, Pattern: ".py"},
                        {Type: ArgPatternFlagValue, Pattern: "-c"},
                        {Type: ArgPatternFlag, Pattern: "-m"},
                },
        },
        "python3": {
                Pattern:           "python3",
                Suggestion:        "python3 无参数会进入 REPL。建议使用 python3 script.py 或 python3 -c 'code'。",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternFileExt, Pattern: ".py"},
                        {Type: ArgPatternFlagValue, Pattern: "-c"},
                        {Type: ArgPatternFlag, Pattern: "-m"},
                },
        },
        "node": {
                Pattern:           "node",
                Suggestion:        "node 无参数会进入 REPL。建议使用 node script.js 或 node -e 'code'。",
                NonInteractiveArgs: []ArgPattern{
                        {Type: ArgPatternFileExt, Pattern: ".js"},
                        {Type: ArgPatternFlagValue, Pattern: "-e"},
                        {Type: ArgPatternFlag, Pattern: "-r"},
                },
        },
        
        // 权限切换 - 完全交互
        "sudo -i": {
                Pattern:    "sudo -i",
                Suggestion: "sudo -i 会启动 root shell。建议使用 sudo command 执行命令。",
        },
        "sudo su": {
                Pattern:    "sudo su",
                Suggestion: "sudo su 会启动交互式 shell。建议使用 sudo command。",
        },
        "su ": {
                Pattern:    "su ",
                Suggestion: "su 会启动交互式 shell。建议使用 sudo command。",
        },
        
        // 终端复用
        "screen": {Pattern: "screen", Suggestion: "screen 是终端复用器，需要交互。"},
        "tmux":   {Pattern: "tmux", Suggestion: "tmux 是终端复用器，需要交互。"},
}

// interactivePatterns 兼容旧代码，保留模式列表
var interactivePatterns = func() []string {
        patterns := make([]string, 0, len(interactiveCommands))
        for p := range interactiveCommands {
                patterns = append(patterns, p)
        }
        return patterns
}()

// quickCommandPatterns 快速命令白名单
// 这些命令通常在几秒内完成，可以安全地同步执行
var quickCommandPatterns = []string{
        // 文件操作
        "ls", "cat ", "head ", "tail ", "wc ", "touch ", "file ",
        "mkdir ", "rmdir ", "rm ", "cp ", "mv ", "ln ",
        // 文本处理
        "echo ", "printf ", "grep ", "sed ", "awk ", "cut ", "sort ", "uniq ",
        // 系统信息
        "pwd", "whoami", "hostname", "uname", "date", "uptime", "df ", "du ",
        "ps ", "pgrep ", "pkill ", "kill ", "killall ",
        // 网络（查询类）
        "ping -c ", "host ", "nslookup ", "dig ", "ip ", "ifconfig",
        // 进程/服务状态
        "which ", "whereis ", "type ", "stat ", "realpath ", "readlink ",
        // Git 快速操作
        "git status", "git log", "git diff", "git branch", "git remote",
        "git rev", "git show", "git tag",
        // 环境变量
        "env", "export ", "printenv", "set ", "unset ",
        // 简单计算
        "expr ", "bc ", "let ",
}

var longRunningPatterns = []string{
        // Linux package managers
        "apt update", "apt upgrade", "apt install", "apt-get",
        "yum update", "yum upgrade", "yum install",
        "dnf update", "dnf upgrade", "dnf install",
        "pacman -S", "pacman -Syu",
        // FreeBSD/GhostBSD package management
        "pkg install", "pkg update", "pkg upgrade", "pkg bootstrap",
        "portsnap fetch", "portsnap extract", "portsnap update",
        "freebsd-update fetch", "freebsd-update install",
        "portmaster ", "portinstall ",
        "make install", "make build", "make clean",
        "make config-recursive", "make rmconfig",
        // Build systems
        "make", "cmake", "ninja",
        // Node.js
        "npm install", "npm update", "npm run build",
        "yarn install", "yarn build",
        "pnpm install", "pnpm build",
        // Python
        "pip install", "pip3 install",
        // Rust
        "cargo build", "cargo install",
        // Go
        "go build", "go install", "go get",
        // Docker
        "docker build", "docker-compose build",
        // Git
        "git clone", "git fetch", "git pull --rebase",
        // Network transfers
        "rsync", "scp ", "sftp ",
        "wget ", "curl -O", "curl -o",
        // Archives
        "tar ", "unzip ", "7z ",
        // Media encoding
        "ffmpeg", "handbrake",
        // Services
        "systemctl start", "systemctl restart",
        "service ", "/etc/init.d/",
        "rcctl start", "rcctl restart", // OpenBSD
        "service ", "rc-service ",       // Alpine/Gentoo
}

// DetectCommandType 检测命令类型
// 返回类型: "quick"（快速命令）, "long_running"（长时命令）, "interactive"（交互式命令）, "unknown"（未知命令）
func DetectCommandType(command string) CommandSuggestion {
        lowerCmd := strings.ToLower(command)
        
        // 1. 检测快速命令（白名单）
        for _, p := range quickCommandPatterns {
                if strings.Contains(lowerCmd, strings.ToLower(p)) {
                        return CommandSuggestion{
                                Type:    "quick",
                                Message: "快速命令，将同步执行",
                        }
                }
        }

        // 2. 检测交互式命令 - 使用参数模式匹配
        for pattern, info := range interactiveCommands {
                if strings.Contains(lowerCmd, strings.ToLower(pattern)) {
                        // 检查是否匹配了非交互式参数模式
                        if hasNonInteractiveArg(command, info.NonInteractiveArgs) {
                                // 已有非交互式参数，当作快速命令
                                return CommandSuggestion{
                                        Type:    "quick",
                                        Message: "已包含非交互式参数",
                                }
                        }
                        
                        // 无非交互式参数，是交互式命令
                        return CommandSuggestion{
                                Type:             "interactive",
                                Message:          fmt.Sprintf("检测到交互式命令: %s", pattern),
                                Suggestion:       info.Suggestion,
                                NonInteractiveEq: info.NonInteractiveCmd,
                        }
                }
        }

        // 3. 检测长时间运行命令
        for _, p := range longRunningPatterns {
                if strings.Contains(lowerCmd, strings.ToLower(p)) {
                        return CommandSuggestion{
                                Type:    "long_running",
                                Message: fmt.Sprintf("检测到长时命令: %s，将异步执行", p),
                        }
                }
        }

        // 4. 未知命令
        return CommandSuggestion{
                Type:    "unknown",
                Message: "未知命令类型，将使用保守策略执行",
        }
}

// hasNonInteractiveArg 检查命令是否包含非交互式参数
// 使用声明式参数模式，通用处理所有命令
func hasNonInteractiveArg(command string, patterns []ArgPattern) bool {
        fields := strings.Fields(command)
        lowerCmd := strings.ToLower(command)
        
        for _, p := range patterns {
                switch p.Type {
                case ArgPatternFlag:
                        // 检查是否存在某个标志参数
                        // 例如：--no-pager, -b
                        for _, f := range fields {
                                if strings.ToLower(f) == strings.ToLower(p.Pattern) {
                                        return true
                                }
                        }
                        
                case ArgPatternFlagValue:
                        // 检查是否存在标志+值的参数
                        // 例如：-n 10, -c 'code'
                        for i, f := range fields {
                                if strings.ToLower(f) == strings.ToLower(p.Pattern) {
                                        // 找到了标志，检查后面是否有值
                                        if i+1 < len(fields) {
                                                return true
                                        }
                                }
                        }
                        
                case ArgPatternFileExt:
                        // 检查是否有指定扩展名的文件参数
                        // 例如：.py, .js
                        for _, f := range fields {
                                if strings.HasSuffix(strings.ToLower(f), strings.ToLower(p.Pattern)) {
                                        return true
                                }
                        }
                        
                case ArgPatternContains:
                        // 检查命令是否包含某字符串
                        // 例如：sshpass
                        if strings.Contains(lowerCmd, strings.ToLower(p.Pattern)) {
                                return true
                        }
                        
                case ArgPatternHasArgAfter:
                        // SSH 专用：主机名后是否有参数
                        return hasSSHRemoteCommand(fields)
                        
                case ArgPatternNoArgs:
                        // 无参数时才是交互式，所以有参数就是非交互式
                        // 例如：less file.txt 是非交互式
                        return len(fields) > 1
                }
        }
        
        return false
}

// hasSSHRemoteCommand 检查 SSH 命令是否带了远程命令
// ssh [options] [user@]host [command]
func hasSSHRemoteCommand(fields []string) bool {
        sshIdx := -1
        for i, f := range fields {
                if strings.ToLower(f) == "ssh" || strings.HasSuffix(strings.ToLower(f), "/ssh") {
                        sshIdx = i
                        break
                }
        }
        if sshIdx == -1 {
                return false
        }
        
        // 解析 SSH 参数，找到主机名位置
        i := sshIdx + 1
        hasHost := false
        
        for i < len(fields) {
                arg := fields[i]
                
                if strings.HasPrefix(arg, "-") {
                        // 跳过需要值的选项
                        if isSSHOptionWithValue(arg) {
                                i += 2
                                continue
                        }
                        i++
                        continue
                }
                
                if !hasHost {
                        hasHost = true
                } else {
                        // 主机后还有参数，就是远程命令
                        return true
                }
                i++
        }
        
        return false
}

// isSSHOptionWithValue 判断 SSH 选项是否需要值
func isSSHOptionWithValue(arg string) bool {
        // 需要值的选项
        optsWithValue := []string{"-p", "-i", "-l", "-o", "-L", "-R", "-D", "-J", "-S", "-b", "-F", "-c", "-m", "-E", "-e", "-w"}
        argLower := strings.ToLower(arg)
        for _, opt := range optsWithValue {
                if argLower == opt || strings.HasPrefix(argLower, opt) {
                        return true
                }
        }
        return false
}

// 平台相关函数

// getSysProcAttr 获取平台相关的进程属性
// 这个函数在不同平台有不同实现
func getSysProcAttr() *syscall.SysProcAttr {
        return &syscall.SysProcAttr{
                Setpgid: true, // 设置进程组
        }
}

// killProcessGroup 强制终止进程组
func killProcessGroup(pid int) error {
        return syscall.Kill(-pid, syscall.SIGKILL)
}

// terminateProcessGroup 优雅终止进程组
func terminateProcessGroup(pid int) error {
        return syscall.Kill(-pid, syscall.SIGTERM)
}
