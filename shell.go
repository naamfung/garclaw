// shell.go - 完整修复版本
package main

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "runtime"
    "strings"
    "time"
)

type CmdResult struct {
    Stdout        string
    Stderr        string
    ExitCode      int
    Err           error
    ConfirmRequired bool     `json:"confirm_required,omitempty"`
    ConfirmMessage string    `json:"confirm_message,omitempty"`
    Suggestions   []string  `json:"suggestions,omitempty"`
}

type BlockingCommandInfo struct {
    IsBlocking    bool
    Reason        string
    Suggestions   []string
}

func detectBlockingCommand(command string) BlockingCommandInfo {
    lowerCmd := strings.ToLower(command)
    cmdFields := strings.Fields(lowerCmd)

    info := BlockingCommandInfo{IsBlocking: false}

    if len(cmdFields) == 0 {
        return info
    }

    cmdName := cmdFields[0]
    if idx := strings.LastIndex(cmdName, "/"); idx >= 0 {
        cmdName = cmdName[idx+1:]
    }

    // SSH
    if cmdName == "ssh" {
        if !strings.HasPrefix(strings.TrimSpace(command), "sshpass") {
            hasPasswordAuth := strings.Contains(lowerCmd, "passwordauthentication=yes") ||
                strings.Contains(lowerCmd, "passwordauthentication yes") ||
                strings.Contains(lowerCmd, "-o stricthostkeychecking=no") && !strings.Contains(lowerCmd, "-i ")
            if hasPasswordAuth || !strings.Contains(lowerCmd, "-i ") {
                info.IsBlocking = true
                info.Reason = "ssh 命令可能需要交互输入密码"
                info.Suggestions = []string{
                    "使用 sshpass -p '密码' ssh user@host ...",
                    "使用密钥认证: ssh -i /path/to/key user@host ...",
                    "若确认无需交互，可使用 force: true 强制执行",
                }
                return info
            }
        }
    }

    // SCP
    if cmdName == "scp" {
        if !strings.HasPrefix(strings.TrimSpace(command), "sshpass") {
            if !strings.Contains(lowerCmd, "-i ") {
                info.IsBlocking = true
                info.Reason = "scp 命令可能需要交互输入密码"
                info.Suggestions = []string{
                    "使用 sshpass -p '密码' scp source dest ...",
                    "使用密钥认证: scp -i /path/to/key source dest ...",
                    "若确认无需交互，可使用 force: true 强制执行",
                }
                return info
            }
        }
    }

    // rsync
    if cmdName == "rsync" {
        if strings.Contains(lowerCmd, "-e ssh") || strings.Contains(lowerCmd, "-e 'ssh") || strings.Contains(lowerCmd, "-e\"ssh") {
            if !strings.HasPrefix(strings.TrimSpace(command), "sshpass") && !strings.Contains(lowerCmd, "-i ") {
                info.IsBlocking = true
                info.Reason = "rsync 通过 ssh 传输可能需要交互输入密码"
                info.Suggestions = []string{
                    "使用 sshpass -p '密码' rsync -e 'ssh' ...",
                    "使用密钥认证",
                    "若确认无需交互，可使用 force: true 强制执行",
                }
                return info
            }
        }
    }

    // sudo/su
    if cmdName == "sudo" || cmdName == "su" {
        if !strings.Contains(lowerCmd, "-s") && !strings.Contains(lowerCmd, "-S") {
            info.IsBlocking = true
            info.Reason = fmt.Sprintf("%s 命令可能需要交互输入密码", cmdName)
            info.Suggestions = []string{
                fmt.Sprintf("使用 echo 'password' | %s -S ...", cmdName),
                "配置 sudoers 免密码",
                "若确认无需交互，可使用 force: true 强制执行",
            }
            return info
        }
    }

    // sftp/ftp
    if cmdName == "sftp" || cmdName == "ftp" {
        info.IsBlocking = true
        info.Reason = fmt.Sprintf("%s 命令通常需要交互输入", cmdName)
        info.Suggestions = []string{
            "使用 sshpass 配合 sftp",
            "使用 lftp 等支持脚本化的工具",
            "若确认无需交互，可使用 force: true 强制执行",
        }
        return info
    }

    interactivePrograms := []string{"vim", "vi", "nano", "emacs", "less", "more", "top", "htop", "screen", "tmux", "mysql", "psql", "sqlite3"}
    for _, prog := range interactivePrograms {
        if cmdName == prog {
            info.IsBlocking = true
            info.Reason = fmt.Sprintf("%s 是交互式程序，会阻塞 shell 执行", prog)
            info.Suggestions = []string{
                fmt.Sprintf("使用非交互模式运行 %s", prog),
                fmt.Sprintf("使用 %s 的批处理模式", prog),
                "若确认需要交互，请使用 shell_delayed 工具",
            }
            return info
        }
    }

    return info
}

func CheckBlockingCommand(command string) BlockingCommandInfo {
    return detectBlockingCommand(command)
}

func BuildConfirmMessage(info BlockingCommandInfo, originalCmd string) string {
    var sb strings.Builder
    sb.WriteString("⚠️ 此命令可能需要交互输入进而导致操作阻塞。\n\n")
    sb.WriteString(fmt.Sprintf("**原因**：%s\n\n", info.Reason))
    sb.WriteString("**建议**：\n")
    for i, sug := range info.Suggestions {
        sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, sug))
    }
    sb.WriteString("\n是否仍要执行原命令？")
    return sb.String()
}

func isDangerousCommand(command string) bool {
    lowerCmd := strings.ToLower(command)

    dangerousPatterns := []string{
        "rm -rf /", "rm -rf /*", "mkfs", "dd if=", "format",
        ":(){ :|:& };:", "chmod 777 /", "chown -R", "> /dev/sda",
        "shutdown", "reboot", "halt", "init 0", "poweroff",
    }
    for _, pattern := range dangerousPatterns {
        if strings.Contains(lowerCmd, pattern) {
            return true
        }
    }

    luaBlacklist := []string{
        "lua", "luajit", "luarocks", "moon", "moonc", "fengari", "luvit", "lit",
    }
    cmdParts := strings.Fields(lowerCmd)
    if len(cmdParts) > 0 {
        firstWord := cmdParts[0]
        if idx := strings.LastIndex(firstWord, "/"); idx >= 0 {
            firstWord = firstWord[idx+1:]
        }
        for _, blocked := range luaBlacklist {
            if firstWord == blocked ||
                strings.HasPrefix(firstWord, blocked+"5.") ||
                strings.HasPrefix(firstWord, blocked+"5") ||
                strings.HasPrefix(firstWord, blocked+"-") {
                return true
            }
        }
    }

    luaCallPatterns := []string{
        "; lua", "&& lua", "|| lua", "| lua", "`lua", "$(lua",
        "; luajit", "&& luajit", "|| luajit", "| luajit", "`luajit", "$(luajit",
        "; luarocks", "&& luarocks", "|| luarocks", "| luarocks", "`luarocks", "$(luarocks",
    }
    for _, pattern := range luaCallPatterns {
        if strings.Contains(lowerCmd, pattern) {
            return true
        }
    }

    return false
}

func runShell(ctx context.Context, command string) CmdResult {
    return runShellWithTimeout(ctx, command, false, false)
}

// runShellWithTimeout 执行命令，增加别名展开
func runShellWithTimeout(ctx context.Context, command string, force bool, isBlockingConfirmed bool) CmdResult {
    // ========== 新增：展开命令别名 ==========
    if globalToolsAliases != nil && len(globalToolsAliases) > 0 {
        expanded := ExpandAlias(command, globalToolsAliases)
        if expanded != command {
            if IsDebug {
                fmt.Printf("[runShell] Alias expanded: %q -> %q\n", command, expanded)
            }
            command = expanded
        }
    }
    // ======================================

    if IsDebug {
        fmt.Printf("[runShell] executing: %q, force=%v, isBlockingConfirmed=%v\n", command, force, isBlockingConfirmed)
    }

    blockingInfo := detectBlockingCommand(command)

    if blockingInfo.IsBlocking && !force && !isBlockingConfirmed {
        return CmdResult{
            ConfirmRequired: true,
            ConfirmMessage:  BuildConfirmMessage(blockingInfo, command),
            Suggestions:     blockingInfo.Suggestions,
        }
    }

    var cancel context.CancelFunc
    if _, hasDeadline := ctx.Deadline(); !hasDeadline {
        var timeout time.Duration
        if isBlockingConfirmed || blockingInfo.IsBlocking {
            timeout = time.Duration(DefaultBlockingCmdTimeout) * time.Second
        } else {
            timeout = time.Duration(globalTimeoutConfig.Shell) * time.Second
            if timeout <= 0 {
                timeout = time.Duration(DefaultShellTimeout) * time.Second
            }
        }
        ctx, cancel = context.WithTimeout(ctx, timeout)
        defer cancel()
    }

    if BlockDangerousCommands {
        if isDangerousCommand(command) {
            return CmdResult{Err: errors.New("dangerous command blocked")}
        }
    } else {
        if IsDebug {
            fmt.Println("Dangerous command blocking is disabled, allowing all commands.")
        }
    }

    if runtime.GOOS == "windows" && strings.HasPrefix(strings.TrimSpace(strings.ToLower(command)), "touch ") {
        return handleWindowsTouch(command)
    }

    var cmd *exec.Cmd
    if runtime.GOOS == "windows" {
        command = translateUnixToWindows(command)
        cmd = exec.CommandContext(ctx, "cmd.exe", "/c", command)
    } else {
        cmd = exec.CommandContext(ctx, "sh", "-c", command)
    }

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err := cmd.Run()
    if err != nil {
        exitCode := -1
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        }
        return CmdResult{
            Stdout:   truncateOutput(stdout.String()),
            Stderr:   stderr.String(),
            ExitCode: exitCode,
            Err:      err,
        }
    }

    return CmdResult{
        Stdout:   truncateOutput(stdout.String()),
        Stderr:   stderr.String(),
        ExitCode: 0,
        Err:      nil,
    }
}

func handleWindowsTouch(command string) CmdResult {
    parts := strings.Fields(command)
    if len(parts) < 2 {
        return CmdResult{Err: errors.New("touch command requires a file path")}
    }
    filePath := strings.Join(parts[1:], " ")

    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        file, err := os.Create(filePath)
        if err != nil {
            return CmdResult{Err: fmt.Errorf("failed to create file: %w", err)}
        }
        file.Close()
    } else {
        now := time.Now()
        err := os.Chtimes(filePath, now, now)
        if err != nil {
            return CmdResult{Err: fmt.Errorf("failed to update timestamps: %w", err)}
        }
    }
    return CmdResult{Stdout: "(no output)", ExitCode: 0}
}

func truncateOutput(output string) string {
    if len(output) > 50000 && IsDebug {
        return TruncateString(output, 50000)
    }
    return output
}

func translateUnixToWindows(command string) string {
    command = strings.TrimSpace(command)
    parts := strings.Fields(command)
    if len(parts) == 0 {
        return command
    }
    cmd := parts[0]
    args := parts[1:]

    switch strings.ToLower(cmd) {
    case "ls":
        dirArgs := []string{}
        for _, arg := range args {
            switch strings.ToLower(arg) {
            case "-l":
                dirArgs = append(dirArgs, "")
            case "-a":
                dirArgs = append(dirArgs, "/a")
            case "-la", "-al":
                dirArgs = append(dirArgs, "/a")
            default:
                dirArgs = append(dirArgs, arg)
            }
        }
        return "dir " + strings.Join(dirArgs, " ")
    case "pwd":
        return "cd"
    case "mkdir":
        return "md " + strings.Join(args, " ")
    case "rm":
        return "del " + strings.Join(args, " ")
    case "rmdir":
        return "rd " + strings.Join(args, " ")
    case "cp":
        return "copy " + strings.Join(args, " ")
    case "mv":
        return "move " + strings.Join(args, " ")
    case "cat":
        return "type " + strings.Join(args, " ")
    case "echo":
        return command
    case "date":
        return "date /t"
    default:
        return command
    }
}
