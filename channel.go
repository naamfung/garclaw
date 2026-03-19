package main

import "sync"

// Channel 是所有前端频道的统一接口
type Channel interface {
	// WriteChunk 向客户端发送一个流式数据块
	WriteChunk(chunk StreamChunk) error
	// ID 返回频道的唯一标识
	ID() string
	// Close 关闭频道，释放资源
	Close() error
}

// BaseChannel 提供基础实现
type BaseChannel struct {
	id string
	mu sync.Mutex // 用于 WriteChunk 的并发控制（子类可嵌入并重用）
}

func (bc *BaseChannel) ID() string { return bc.id }

func (bc *BaseChannel) Close() error { return nil }

