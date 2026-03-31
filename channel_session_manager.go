package main

import (
	"context"
	"log"
	"sync"
	"time"
)

type ChannelSession struct {
	ID           string
	ChannelType  string
	ChannelID    string
	History      []Message
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MessageCount int
	mu           sync.RWMutex
}

type ChannelSessionManager struct {
	sessions map[string]*ChannelSession
	mu       sync.RWMutex
	persist  *SessionPersistManager
}

var globalChannelSessionManager *ChannelSessionManager

func InitChannelSessionManager() {
	if globalChannelSessionManager == nil {
		globalChannelSessionManager = &ChannelSessionManager{
			sessions: make(map[string]*ChannelSession),
			persist:  globalSessionPersist,
		}
		go globalChannelSessionManager.autoSaveLoop()
	}
}

func GetChannelSessionManager() *ChannelSessionManager {
	return globalChannelSessionManager
}

func (m *ChannelSessionManager) autoSaveLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.SaveAllSessions()
	}
}

func GenerateSessionID(channelType, channelID string) string {
	return channelType + "_" + channelID + "_" + time.Now().Format("20060102_150405")
}

func (m *ChannelSessionManager) GetOrCreateSession(channelType, channelID string) *ChannelSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sess := range m.sessions {
		if sess.ChannelType == channelType && sess.ChannelID == channelID {
			if time.Since(sess.UpdatedAt) < 24*time.Hour {
				return sess
			}
			go m.saveSession(id, sess)
			delete(m.sessions, id)
			break
		}
	}

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

func (m *ChannelSessionManager) GetSession(sessionID string) (*ChannelSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[sessionID]
	return sess, ok
}

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

// UpdateHistory 更新会话历史并立即保存
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

	// 立即异步保存
	go m.saveSession(sessionID, sess)
}

// AddMessage 添加单条消息并立即保存
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

	// 立即异步保存
	go m.saveSession(sessionID, sess)
}

func (m *ChannelSessionManager) GetHistory(sessionID string) []Message {
	m.mu.RLock()
	sess, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	history := make([]Message, len(sess.History))
	copy(history, sess.History)
	return history
}

func (m *ChannelSessionManager) ClearSession(channelType, channelID string) *ChannelSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sess := range m.sessions {
		if sess.ChannelType == channelType && sess.ChannelID == channelID {
			go m.saveSession(id, sess)
			delete(m.sessions, id)
			break
		}
	}

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
		return
	}

	description := channelType + " session"
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

	// 使用会话ID作为文件名前缀
	_, err := m.persist.SaveSession(sessionID, history, description)
	if err != nil {
		log.Printf("[SessionManager] Failed to save session %s: %v", sessionID, err)
	} else {
		log.Printf("[SessionManager] Session saved: %s (%d messages)", sessionID, len(history))
	}

	// 使用统一记忆系统记录会话摘要
	if globalUnifiedMemory != nil && len(history) > 0 {
		summary := generateSessionSummary(history)
		globalUnifiedMemory.RecordSession(sessionID, channelType, summary, len(history), []string{channelType})
	}
}

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

func generateSessionSummary(history []Message) string {
	if len(history) == 0 {
		return "Empty session"
	}
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

// ProcessChannelMessage 处理渠道消息（自动管理会话）
func ProcessChannelMessage(ctx context.Context, channelType, channelID, content string, metadata map[string]interface{}, ch Channel) {
	session := globalChannelSessionManager.GetOrCreateSession(channelType, channelID)

	if content == "/new" || content == "/reset" {
		session = globalChannelSessionManager.ClearSession(channelType, channelID)
		ch.WriteChunk(StreamChunk{
			SessionID: session.ID,
			Content:   "✅ 已开始新对话\n",
			Done:      true,
		})
		return
	}

	if content == "/stop" || content == "/cancel" {
		ch.WriteChunk(StreamChunk{
			SessionID: session.ID,
			Content:   "[已取消]\n",
			Done:      true,
		})
		return
	}

	if content == "/save" {
		go globalChannelSessionManager.saveSession(session.ID, session)
		ch.WriteChunk(StreamChunk{
			SessionID: session.ID,
			Content:   "✅ 会话已保存\n",
			Done:      true,
		})
		return
	}

	history := globalChannelSessionManager.GetHistory(session.ID)
	userMsg := Message{
		Role:    "user",
		Content: content,
	}
	history = append(history, userMsg)

	newHistory, err := AgentLoop(ctx, ch, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
	if err != nil {
		log.Printf("[SessionManager] AgentLoop error for %s: %v", session.ID, err)
		if len(newHistory) > len(history) {
			globalChannelSessionManager.UpdateHistory(session.ID, newHistory)
			log.Printf("[SessionManager] Saved partial history after error for %s (old: %d, new: %d)", session.ID, len(history), len(newHistory))
		}
		return
	}

	globalChannelSessionManager.UpdateHistory(session.ID, newHistory)

	if globalMemoryConsolidator != nil {
		globalMemoryConsolidator.AddMessage(session.ID, ConsolidationMessage{
			Role:      "user",
			Content:   content,
			Timestamp: time.Now(),
			Metadata:  metadata,
		})
		go globalMemoryConsolidator.MaybeConsolidate(ctx, session.ID)
	}
}
