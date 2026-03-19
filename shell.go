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
	Err      error
}

func isDangerousCommand(command string) bool {
	lowerCmd := strings.ToLower(command)
	dangerousPatterns := []string{
		"rm -rf /",
		"rm -rf /*",
		"mkfs",
		"dd if=",
		"format",
		":(){ :|:& };:",
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

func runShell(ctx context.Context, command string) CmdResult {
	if IsDebug {
		fmt.Printf("[runShell] executing: %q\n", command)
	}

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