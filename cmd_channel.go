package main

import (
	"fmt"
	"io"
	"os"
)

// CmdChannel 实现命令行频道
type CmdChannel struct {
	*BaseChannel
	writer io.Writer
}

// NewCmdChannel 创建命令行频道
func NewCmdChannel() *CmdChannel {
	return &CmdChannel{
		BaseChannel: &BaseChannel{id: "cmd"},
		writer:      os.Stdout,
	}
}

// WriteChunk 将数据块写入标准输出
func (c *CmdChannel) WriteChunk(chunk StreamChunk) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if chunk.Error != "" {
		fmt.Fprintf(c.writer, "Error: %s\n", chunk.Error)
		return nil
	}
	if chunk.Content != "" {
		fmt.Fprint(c.writer, chunk.Content)
	}
	if chunk.ReasoningContent != "" {
		fmt.Fprint(c.writer, chunk.ReasoningContent)
	}
	if chunk.Done {
		fmt.Fprintln(c.writer)
	}
	return nil
}
