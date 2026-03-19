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
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error // 仅在真正无法执行命令时设置（如命令不存在、危险命令被拦截等）
}

// isDangerousCommand 检查命令是否包含危险模式
func isDangerousCommand(command string) bool {
	lowerCmd := strings.ToLower(command)

	// 危险模式列表
	dangerousPatterns := []string{
		"rm -rf /",
		"rm -rf /*",
		"mkfs",
		"dd if=",
		"format",
		":(){ :|:& };:", // fork bomb
		"chmod 777 /",
		"chown -R",
		"> /dev/sda",
		"shutdown",
		"reboot",
		"halt",
		"init 0",
		"poweroff",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return true
		}
	}
	return false
}

// 执行shell命令，返回结构化的结果，支持通过 Context 取消
func runShell(ctx context.Context, command string) CmdResult {
	if IsDebug {
		fmt.Printf("[runShell] executing: %q\n", command)
	}

	// 如果启用了拦截，则检查危险命令
	if BlockDangerousCommands {
		if isDangerousCommand(command) {
			return CmdResult{
				Err: errors.New("dangerous command blocked"),
			}
		}
	} else {
		if IsDebug {
			fmt.Println("Dangerous command blocking is disabled, allowing all commands.")
		}
	}

	// 在Windows上特殊处理touch命令
	if runtime.GOOS == "windows" && strings.HasPrefix(strings.TrimSpace(strings.ToLower(command)), "touch ") {
		return handleWindowsTouch(command)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		command = translateUnixToWindows(command)
		// 使用 CommandContext 支持取消
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 执行命令，无超时，但可以通过 Context 取消
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

// handleWindowsTouch 在Windows上模拟touch命令
func handleWindowsTouch(command string) CmdResult {
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return CmdResult{
			Err: errors.New("touch command requires a file path"),
		}
	}
	filePath := strings.Join(parts[1:], " ")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		file, err := os.Create(filePath)
		if err != nil {
			return CmdResult{
				Err: fmt.Errorf("failed to create file: %w", err),
			}
		}
		file.Close()
	} else {
		now := time.Now()
		err := os.Chtimes(filePath, now, now)
		if err != nil {
			return CmdResult{
				Err: fmt.Errorf("failed to update timestamps: %w", err),
			}
		}
	}
	return CmdResult{
		Stdout:   "(no output)",
		Stderr:   "",
		ExitCode: 0,
		Err:      nil,
	}
}

// truncateOutput 截断过长的输出（仅当IsDebug为true时截断，否则保留完整）
func truncateOutput(output string) string {
	if len(output) > 50000 && IsDebug {
		return TruncateString(output, 50000)
	}
	return output
}

// translateUnixToWindows 将Unix命令转换为等效的Windows命令（保持不变）
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