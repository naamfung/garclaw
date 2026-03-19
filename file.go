package main

import (
	"bufio"
	"errors"
	"os"
)

// ReadFileLine 读取文件的指定行（行号从1开始）
func ReadFileLine(filename string, lineNum int) (string, error) {
	if lineNum < 1 {
		return "", errors.New("line number must be >= 1")
	}

	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	const initialBufSize = 1024 * 1024   // 1MB
	const maxBufSize = 100 * 1024 * 1024 // 100MB
	scanner.Buffer(make([]byte, initialBufSize), maxBufSize)

	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine == lineNum {
			return scanner.Text(), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", errors.New("line number out of range")
}

// WriteFileLine 写入文件的指定行（替换原内容），若行号超出则自动扩展
func WriteFileLine(filename string, lineNum int, content string) error {
	if lineNum < 1 {
		return errors.New("line number must be >= 1")
	}

	// 读取文件所有行，如果文件不存在则视为空
	lines, err := ReadAllLines(filename)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		lines = []string{}
	}

	// 扩展行切片至足够长度
	if lineNum > len(lines) {
		needed := lineNum - len(lines)
		lines = append(lines, make([]string, needed)...)
	}
	lines[lineNum-1] = content

	return WriteAllLines(filename, lines)
}

// ReadAllLines 读取文件所有行，返回字符串切片
// 若文件不存在，返回空切片和 nil 错误（為兼容 WriteFileLine 行為）
func ReadAllLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 10*1024*1024)
	const maxBufSize = 100 * 1024 * 1024
	scanner.Buffer(buf, maxBufSize)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// WriteAllLines 将字符串切片写入文件（覆盖原有内容）
func WriteAllLines(filename string, lines []string) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}
	return writer.Flush()
}

