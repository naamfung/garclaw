package main

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// WSChannel 实现 WebSocket 频道
type WSChannel struct {
	*BaseChannel
	conn *websocket.Conn
	mu   sync.Mutex
}

// NewWSChannel 创建 WebSocket 频道
func NewWSChannel(conn *websocket.Conn) *WSChannel {
	return &WSChannel{
		BaseChannel: &BaseChannel{id: conn.RemoteAddr().String()},
		conn:        conn,
	}
}

// WriteChunk 将数据块通过 WebSocket 发送 JSON，返回错误以便上层停止发送
func (wsc *WSChannel) WriteChunk(chunk StreamChunk) error {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	
	err := wsc.conn.WriteJSON(chunk)
	if err != nil {
		log.Printf("WebSocket write error: %v", err)
	}
	return err
}

// Close 关闭 WebSocket 连接
func (wsc *WSChannel) Close() error {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	return wsc.conn.Close()
}