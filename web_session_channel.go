package main

import (
        "sync"
)

// SessionChannel 会话输出通道
// 后台任务通过此 Channel 将输出写入会话的输出队列
// 即使 WebSocket 断开，输出也会被缓冲，用户重连后可以继续接收
type SessionChannel struct {
        *BaseChannel
        session *WebSession
}

// NewSessionChannel 创建会话输出通道
func NewSessionChannel(session *WebSession) *SessionChannel {
        return &SessionChannel{
                BaseChannel: NewBaseChannel("session:" + session.ID),
                session:     session,
        }
}

// WriteChunk 将输出写入会话的输出队列
func (sc *SessionChannel) WriteChunk(chunk StreamChunk) error {
        sc.mu.Lock()
        defer sc.mu.Unlock()

        // 应用流式字符串替换
        processed := sc.ProcessChunkWithReplacement(chunk)

        // 写入会话的输出队列（非阻塞，队列满时丢弃旧数据）
        sc.session.EnqueueOutput(processed)

        return nil
}

// Close 关闭通道（标记完成）
func (sc *SessionChannel) Close() error {
        sc.mu.Lock()
        defer sc.mu.Unlock()
        // 发送完成信号
        sc.session.EnqueueOutput(StreamChunk{Done: true})
        return nil
}

// GetSessionID 返回会话ID
func (sc *SessionChannel) GetSessionID() string {
        return sc.session.ID
}

// CompositeOutputChannel 复合输出通道
// 同时写入 WebSocket（如果连接）和会话输出队列
type CompositeOutputChannel struct {
        *BaseChannel
        wsChannel    *WSChannel
        session      *WebSession
        wsConnected  bool
        wsMu         sync.RWMutex
}

// NewCompositeOutputChannel 创建复合输出通道
func NewCompositeOutputChannel(wsChannel *WSChannel, session *WebSession) *CompositeOutputChannel {
        return &CompositeOutputChannel{
                BaseChannel: NewBaseChannel("composite:" + session.ID),
                wsChannel:   wsChannel,
                session:     session,
                wsConnected: wsChannel != nil,
        }
}

// SetWSConnection 设置/更新 WebSocket 连接
func (c *CompositeOutputChannel) SetWSConnection(wsChannel *WSChannel) {
        c.wsMu.Lock()
        defer c.wsMu.Unlock()
        c.wsChannel = wsChannel
        c.wsConnected = wsChannel != nil
}

// DisconnectWS 标记 WebSocket 断开
func (c *CompositeOutputChannel) DisconnectWS() {
        c.wsMu.Lock()
        defer c.wsMu.Unlock()
        c.wsConnected = false
}

// WriteChunk 同时写入 WebSocket 和会话输出队列
func (c *CompositeOutputChannel) WriteChunk(chunk StreamChunk) error {
        c.mu.Lock()
        processed := c.ProcessChunkWithReplacement(chunk)
        c.mu.Unlock()

        // 总是写入会话输出队列（确保后台任务输出不丢失）
        c.session.EnqueueOutput(processed)

        // 如果 WebSocket 连接，同时写入 WebSocket（实时推送）
        c.wsMu.RLock()
        if c.wsConnected && c.wsChannel != nil {
                c.wsChannel.WriteChunk(processed)
        }
        c.wsMu.RUnlock()

        return nil
}

// Close 关闭通道
func (c *CompositeOutputChannel) Close() error {
        c.session.EnqueueOutput(StreamChunk{Done: true})
        return nil
}

// GetSessionID 返回会话ID
func (c *CompositeOutputChannel) GetSessionID() string {
        return c.session.ID
}
