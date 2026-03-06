package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
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

// 执行shell命令，返回结构化的结果
func runShell(command string) CmdResult {
	// 调试：打印实际执行的命令
	if isDebug {
		fmt.Printf("[runShell] executing: %q\n", command)
	}

	// 危险命令拦截
	dangerous := []string{"rm -rf /", "sudo", "shutdown", "reboot", "> /dev/"}
	for _, d := range dangerous {
		if strings.Contains(command, d) {
			return CmdResult{
				Err: errors.New("dangerous command blocked"),
			}
		}
	}

	// 在Windows上特殊处理touch命令
	if runtime.GOOS == "windows" && strings.HasPrefix(strings.TrimSpace(strings.ToLower(command)), "touch ") {
		return handleWindowsTouch(command)
	}

	// 准备执行命令
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// 检查是否在Unix风格的终端中运行
		if isUnixLikeTerminal() {
			// 在Unix风格终端中，直接使用sh执行命令
			cmd = exec.Command("sh", "-c", command)
		} else {
			// 否则转换为Windows命令
			command = translateUnixToWindows(command)
			cmd = exec.Command("cmd.exe", "/c", command)
		}
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 设置超时（3分钟）
	timeout := time.AfterFunc(3*time.Minute, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	defer timeout.Stop()

	err := cmd.Run()
	// 超时后可能已触发kill，此时err可能为"signal: killed"等
	if err != nil {
		// 尝试获取退出码
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		// 命令执行但返回了非零退出码
		// 注意：即使err!=nil，stderr/stdout中可能仍有内容
		return CmdResult{
			Stdout:   truncateOutput(stdout.String()),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
			Err:      err, // 这里保留原始错误，但调用方可通过ExitCode判断是否是非零退出
		}
	}

	// 命令成功执行
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

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// 文件不存在，创建新文件
		file, err := os.Create(filePath)
		if err != nil {
			return CmdResult{
				Err: fmt.Errorf("failed to create file: %w", err),
			}
		}
		file.Close()
	} else {
		// 文件存在，更新时间戳
		now := time.Now()
		err := os.Chtimes(filePath, now, now)
		if err != nil {
			return CmdResult{
				Err: fmt.Errorf("failed to update timestamps: %w", err),
			}
		}
	}
	// touch 通常无输出
	return CmdResult{
		Stdout:   "(no output)", // 保持与原行为一致，可考虑返回空字符串
		Stderr:   "",
		ExitCode: 0,
		Err:      nil,
	}
}

// truncateOutput 截断过长的输出（仅当isDebug为true时截断，否则保留完整）
func truncateOutput(output string) string {
	if len(output) > 50000 && isDebug { // 仅在调试模式下截断　防止过多信息干扰排查其他问题　此处可手工修改切换
		return TruncateString(output, 50000)
	}
	return output // 非调试模式下，返回完整输出 以便模型获得完整信息
}

// isUnixLikeTerminal 检测当前是否在支持Unix命令的终端（如gitbash）中运行
func isUnixLikeTerminal() bool {
	// 检查环境变量
	shell := os.Getenv("SHELL")
	term := os.Getenv("TERM")

	// 检查是否有SHELL环境变量且包含bash或sh
	if strings.Contains(strings.ToLower(shell), "bash") || strings.Contains(strings.ToLower(shell), "sh") {
		return true
	}

	// 检查TERM环境变量是否设置（通常Unix终端会设置）
	if term != "" && !strings.Contains(strings.ToLower(term), "dumb") {
		return true
	}

	// 检查当前可执行文件路径是否在gitbash目录中
	currentExe, err := os.Executable()
	if err == nil {
		if strings.Contains(strings.ToLower(currentExe), "git/bin") || strings.Contains(strings.ToLower(currentExe), "git/usr/bin") {
			return true
		}
	}

	// 检查是否存在gitbash的典型路径
	usr, err := user.Current()
	if err == nil {
		gitbashPath := usr.HomeDir + "/git/bin/bash.exe"
		if _, err := os.Stat(gitbashPath); err == nil {
			// 检查父进程是否是bash.exe
			// 这里简化处理，实际可以通过进程树检查
			return true
		}
	}

	return false
}

// translateUnixToWindows 将Unix命令转换为等效的Windows命令
func translateUnixToWindows(command string) string {
	// 去除命令前后的空格
	command = strings.TrimSpace(command)

	// 分割命令与参数
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return command
	}

	cmd := parts[0]
	args := parts[1:]

	switch strings.ToLower(cmd) {
	case "ls":
		// ls 命令转换为 dir
		// 转换常见的 ls 参数
		dirArgs := []string{}
		for _, arg := range args {
			switch strings.ToLower(arg) {
			case "-l":
				dirArgs = append(dirArgs, "/l")
			case "-a":
				dirArgs = append(dirArgs, "/a")
			case "-la":
				dirArgs = append(dirArgs, "/l", "/a")
			case "-al":
				dirArgs = append(dirArgs, "/l", "/a")
			default:
				dirArgs = append(dirArgs, arg)
			}
		}
		return "dir " + strings.Join(dirArgs, " ")
	case "pwd":
		// pwd 命令转换为 cd
		return "cd"
	case "mkdir":
		// mkdir 命令转换为 md，跳过 -p 参数
		mdArgs := []string{}
		for _, arg := range args {
			if arg != "-p" {
				mdArgs = append(mdArgs, arg)
			}
		}
		return "md " + strings.Join(mdArgs, " ")
	case "rm":
		// rm 命令转换为 del
		return "del " + strings.Join(args, " ")
	case "rmdir":
		// rmdir 命令转换为 rd
		return "rd " + strings.Join(args, " ")
	case "cp":
		// cp 命令转换为 copy
		return "copy " + strings.Join(args, " ")
	case "mv":
		// mv 命令转换为 move
		return "move " + strings.Join(args, " ")
	case "cat":
		// cat 命令转换为 type
		return "type " + strings.Join(args, " ")
	case "echo":
		// echo 命令在Windows中也可用
		return command
	case "date":
		// date 命令转换为 date /t
		return "date /t"
	case "df":
		// df 命令转换为 wmic logicaldisk get size,freespace,caption
		// 忽略参数，直接返回 Windows 等效命令
		return "wmic logicaldisk get size,freespace,caption"
	default:
		// 其他命令保持不变
		return command
	}
}
