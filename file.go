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
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine == lineNum {
			// 返回该行内容（去掉行尾的换行符，scanner自动去掉了\n）
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

	// 读取文件所有行
	lines, err := ReadAllLines(filename)
	if err != nil {
		return err
	}

	// 扩展行切片至足够长度
	if lineNum > len(lines) {
		// 补充空行直到目标行（注意：目标行之前一行也要存在）
		needed := lineNum - len(lines)
		lines = append(lines, make([]string, needed)...)
	}
	// 替换目标行（切片索引从0开始，所以lineNum-1）
	lines[lineNum-1] = content

	// 将修改后的内容写回文件
	return WriteAllLines(filename, lines)
}

// ReadAllLines 读取文件所有行，返回字符串切片
func ReadAllLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		// 如果文件不存在，返回空切片，让上层决定如何处理
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
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
	// 使用 os.O_TRUNC 清空文件，os.O_CREATE 自动创建
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		// 注意：需要手动添加换行符，最后一行通常也保留换行（根据需求可调整）
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}
	return writer.Flush()
}
