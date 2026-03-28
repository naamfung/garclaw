package main

import (
        "context"
        "log"
        "sync"
        "time"
)

// ============================================================
// 渠道会话管理器
// 为所有渠道提供统一的会话管理和持久化
// ============================================================

// ChannelSession 渠道会话
type ChannelSession struct {
        ID           string       // 会话 ID
        ChannelType  string       // 渠道类型: web, telegram, discord, slack, feishu, cmd
        ChannelID    string       // 渠道内的标识（如 chatID, channelID）
        History      []Message    // 对话历史
        CreatedAt    time.Time    // 创建时间
        UpdatedAt    time.Time    // 更新时间
        MessageCount int          // 消息计数
        mu           sync.RWMutex // 读写锁
}

// ChannelSessionManager 渠道会话管理器
type ChannelSessionManager struct {
        sessions map[string]*ChannelSession // sessionID -> Session
        mu       sync.RWMutex
        persist  *SessionPersistManager
}

// globalChannelSessionManager 全局渠道会话管理器
var globalChannelSessionManager *ChannelSessionManager

// InitChannelSessionManager 初始化全局渠道会话管理器
func InitChannelSessionManager() {
        if globalChannelSessionManager == nil {
                globalChannelSessionManager = &ChannelSessionManager{
                        sessions: make(map[string]*ChannelSession),
                        persist:  globalSessionPersist,
                }
                // 启动自动保存
                go globalChannelSessionManager.autoSaveLoop()
        }
}

// GetChannelSessionManager 获取全局渠道会话管理器
func GetChannelSessionManager() *ChannelSessionManager {
        return globalChannelSessionManager
}

// autoSaveLoop 自动保存循环
func (m *ChannelSessionManager) autoSaveLoop() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()

        for range ticker.C {
                m.SaveAllSessions()
        }
}

// GenerateSessionID 生成会话 ID
func GenerateSessionID(channelType, channelID string) string {
        return channelType + "_" + channelID + "_" + time.Now().Format("20060102_150405")
}

// GetOrCreateSession 获取或创建会话
func (m *ChannelSessionManager) GetOrCreateSession(channelType, channelID string) *ChannelSession {
        m.mu.Lock()
        defer m.mu.Unlock()

        // 查找现有会话（同一渠道同一ID）
        for id, sess := range m.sessions {
                if sess.ChannelType == channelType && sess.ChannelID == channelID {
                        // 检查会话是否过期（超过 24 小时无活动）
                        if time.Since(sess.UpdatedAt) < 24*time.Hour {
                                return sess
                        }
                        // 会话过期，保存并删除
                        go m.saveSession(id, sess)
                        delete(m.sessions, id)
                        break
                }
        }

        // 创建新会话
        sessionID := GenerateSessionID(channelType, channelID)
        session := &ChannelSession{
                ID:          sessionID,
                ChannelType: channelType,
                ChannelID:   channelID,
                History:     make([]Message, 0),
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
        }

        m.sessions[sessionID] = session
        log.Printf("[SessionManager] New session created: %s (%s)", sessionID, channelType)

        return session
}

// GetSession 获取会话
func (m *ChannelSessionManager) GetSession(sessionID string) (*ChannelSession, bool) {
        m.mu.RLock()
        defer m.mu.RUnlock()

        sess, ok := m.sessions[sessionID]
        return sess, ok
}

// GetSessionByChannel 通过渠道信息获取会话
func (m *ChannelSessionManager) GetSessionByChannel(channelType, channelID string) *ChannelSession {
        m.mu.RLock()
        defer m.mu.RUnlock()

        for _, sess := range m.sessions {
                if sess.ChannelType == channelType && sess.ChannelID == channelID {
                        return sess
                }
        }
        return nil
}

// UpdateHistory 更新会话历史
func (m *ChannelSessionManager) UpdateHistory(sessionID string, history []Message) {
        m.mu.RLock()
        sess, ok := m.sessions[sessionID]
        m.mu.RUnlock()

        if !ok {
                return
        }

        sess.mu.Lock()
        defer sess.mu.Unlock()

        sess.History = history
        sess.UpdatedAt = time.Now()
        sess.MessageCount = len(history)
}

// AddMessage 添加消息到会话
func (m *ChannelSessionManager) AddMessage(sessionID string, msg Message) {
        m.mu.RLock()
        sess, ok := m.sessions[sessionID]
        m.mu.RUnlock()

        if !ok {
                return
        }

        sess.mu.Lock()
        defer sess.mu.Unlock()

        sess.History = append(sess.History, msg)
        sess.UpdatedAt = time.Now()
        sess.MessageCount = len(sess.History)
}

// GetHistory 获取会话历史
func (m *ChannelSessionManager) GetHistory(sessionID string) []Message {
        m.mu.RLock()
        sess, ok := m.sessions[sessionID]
        m.mu.RUnlock()

        if !ok {
                return nil
        }

        sess.mu.RLock()
        defer sess.mu.RUnlock()

        // 返回副本
        history := make([]Message, len(sess.History))
        copy(history, sess.History)
        return history
}

// ClearSession 清除会话（开始新对话）
func (m *ChannelSessionManager) ClearSession(channelType, channelID string) *ChannelSession {
        m.mu.Lock()
        defer m.mu.Unlock()

        // 查找并保存现有会话
        for id, sess := range m.sessions {
                if sess.ChannelType == channelType && sess.ChannelID == channelID {
                        go m.saveSession(id, sess)
                        delete(m.sessions, id)
                        break
                }
        }

        // 创建新会话
        sessionID := GenerateSessionID(channelType, channelID)
        session := &ChannelSession{
                ID:          sessionID,
                ChannelType: channelType,
                ChannelID:   channelID,
                History:     make([]Message, 0),
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
        }

        m.sessions[sessionID] = session
        log.Printf("[SessionManager] New session started: %s (%s)", sessionID, channelType)

        return session
}

// SaveSession 保存会话到持久化存储
func (m *ChannelSessionManager) saveSession(sessionID string, sess *ChannelSession) {
        if m.persist == nil {
                return
        }

        sess.mu.RLock()
        history := make([]Message, len(sess.History))
        copy(history, sess.History)
        channelType := sess.ChannelType
        sess.mu.RUnlock()

        if len(history) == 0 {
                return // 空会话不保存
        }

        description := channelType + " session"
        // 尝试从第一条用户消息提取描述
        for _, msg := range history {
                if msg.Role == "user" {
                        if content, ok := msg.Content.(string); ok && len(content) > 0 {
                                if len(content) > 50 {
                                        description = content[:50] + "..."
                                } else {
                                        description = content
                                }
                        }
                        break
                }
        }

        _, err := m.persist.SaveSession(sessionID, history, description)
        if err != nil {
                log.Printf("[SessionManager] Failed to save session %s: %v", sessionID, err)
        } else {
                log.Printf("[SessionManager] Session saved: %s (%d messages)", sessionID, len(history))
        }

        // 同时记录到 TwoLayerMemory
        if globalTwoLayerMemory != nil && len(history) > 0 {
                summary := generateSessionSummary(history)
                globalTwoLayerMemory.RecordSession(sessionID, len(history), summary, []string{channelType})
        }
}

// SaveAllSessions 保存所有会话
func (m *ChannelSessionManager) SaveAllSessions() {
        m.mu.RLock()
        sessions := make(map[string]*ChannelSession)
        for k, v := range m.sessions {
                sessions[k] = v
        }
        m.mu.RUnlock()

        for id, sess := range sessions {
                m.saveSession(id, sess)
        }
}

// generateSessionSummary 生成会话摘要
func generateSessionSummary(history []Message) string {
        if len(history) == 0 {
                return "Empty session"
        }

        // 简单摘要：取第一条用户消息的前 100 字符
        for _, msg := range history {
                if msg.Role == "user" {
                        content, ok := msg.Content.(string)
                        if ok && len(content) > 0 {
                                if len(content) > 100 {
                                        return content[:100] + "..."
                                }
                                return content
                        }
                }
        }

        return "Session"
}

// ProcessChannelMessage 处理渠道消息（统一入口）
func ProcessChannelMessage(ctx context.Context, channelType, channelID, content string, metadata map[string]interface{}, ch Channel) {
        // 获取或创建会话
        session := globalChannelSessionManager.GetOrCreateSession(channelType, channelID)

        // 检查是否是重置命令
        if content == "/new" || content == "/reset" {
                session = globalChannelSessionManager.ClearSession(channelType, channelID)
                ch.WriteChunk(StreamChunk{
                        SessionID: session.ID,
                        Content:   "✅ 已开始新对话\n",
                        Done:      true,
                })
                return
        }

        // 检查是否是停止命令
        if content == "/stop" || content == "/cancel" {
                ch.WriteChunk(StreamChunk{
                        SessionID: session.ID,
                        Content:   "[已取消]\n",
                        Done:      true,
                })
                return
        }

        // 检查是否是保存命令
        if content == "/save" {
                go globalChannelSessionManager.saveSession(session.ID, session)
                ch.WriteChunk(StreamChunk{
                        SessionID: session.ID,
                        Content:   "✅ 会话已保存\n",
                        Done:      true,
                })
                return
        }

        // 获取当前历史
        history := globalChannelSessionManager.GetHistory(session.ID)

        // 添加用户消息
        userMsg := Message{
                Role:    "user",
                Content: content,
        }
        history = append(history, userMsg)

        // 调用 AgentLoop
        newHistory, err := AgentLoop(ctx, ch, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
        if err != nil {
                log.Printf("[SessionManager] AgentLoop error for %s: %v", session.ID, err)
                // 即使发生错误，也要保存已生成的消息历史（防止消息丢失）
                if len(newHistory) > len(history) {
                        globalChannelSessionManager.UpdateHistory(session.ID, newHistory)
                        log.Printf("[SessionManager] Saved partial history after error for %s (old: %d, new: %d)", session.ID, len(history), len(newHistory))
                }
                return
        }

        // 更新历史
        globalChannelSessionManager.UpdateHistory(session.ID, newHistory)

        // 添加消息到记忆整合器
        if globalMemoryConsolidator != nil {
                globalMemoryConsolidator.AddMessage(session.ID, ConsolidationMessage{
                        Role:      "user",
                        Content:   content,
                        Timestamp: time.Now(),
                        Metadata:  metadata,
                })

                // 检查是否需要整合
                go globalMemoryConsolidator.MaybeConsolidate(ctx, session.ID)
        }
}
