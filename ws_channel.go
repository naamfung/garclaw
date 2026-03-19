package main

import (
	"sync"

	"github.com/gorilla/websocket"
)

// WSChannel 实现 WebSocket 频道
type WSChannel struct {
	*BaseChannel
	conn *websocket.Conn
	mu   sync.Mutex // 覆盖 BaseChannel 的 mu 或单独使用
}

// NewWSChannel 创建 WebSocket 频道
func NewWSChannel(conn *websocket.Conn) *WSChannel {
	return &WSChannel{
		BaseChannel: &BaseChannel{id: conn.RemoteAddr().String()},
		conn:        conn,
	}
}

// WriteChunk 将数据块通过 WebSocket 发送 JSON
func (wsc *WSChannel) WriteChunk(chunk StreamChunk) error {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	return wsc.conn.WriteJSON(chunk)
}

// Close 关闭 WebSocket 连接
func (wsc *WSChannel) Close() error {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	return wsc.conn.Close()
}

