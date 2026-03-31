package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type WebInput struct {
	Content   string
	Timestamp time.Time
}

type WebSession struct {
	ID        string
	History   []Message
	CreatedAt time.Time
	LastSeen  time.Time

	InputQueue  chan WebInput
	OutputQueue chan StreamChunk

	TaskRunning   bool
	currentTaskID string
	TaskCtx       context.Context
	TaskCancel    context.CancelFunc

	WsCtx    context.Context
	WsCancel context.CancelFunc

	Connected   bool
	OutputDone  chan struct{}
	LastSentIndex int

	// 持久化相关
	persistID string   // 已保存的会话 ID（文件名前缀）
	persistMu sync.Mutex

	mu sync.RWMutex
}

func NewWebSession() *WebSession {
	taskCtx, taskCancel := context.WithCancel(context.Background())
	wsCtx, wsCancel := context.WithCancel(context.Background())
	s := &WebSession{
		ID:          uuid.New().String()[:8],
		History:     make([]Message, 0),
		CreatedAt:   time.Now(),
		LastSeen:    time.Now(),
		InputQueue:  make(chan WebInput, 100),
		OutputQueue: make(chan StreamChunk, 500),
		TaskCtx:     taskCtx,
		TaskCancel:  taskCancel,
		WsCtx:       wsCtx,
		WsCancel:    wsCancel,
		OutputDone:  make(chan struct{}),
	}
	return s
}

func (s *WebSession) AddToHistory(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = append(s.History, Message{Role: role, Content: content})
	s.LastSeen = time.Now()
}

func (s *WebSession) GetHistory() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := make([]Message, len(s.History))
	copy(h, s.History)
	return h
}

// SetHistory 更新历史并触发自动保存
func (s *WebSession) SetHistory(h []Message) {
	s.mu.Lock()
	s.History = h
	s.LastSeen = time.Now()
	s.mu.Unlock()

	// 异步自动保存，避免阻塞
	go s.autoSaveHistory()
}

func (s *WebSession) GetNewMessages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.LastSentIndex >= len(s.History) {
		return nil
	}
	newMsgs := make([]Message, len(s.History)-s.LastSentIndex)
	copy(newMsgs, s.History[s.LastSentIndex:])
	return newMsgs
}

func (s *WebSession) MarkAllSent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastSentIndex = len(s.History)
}

func (s *WebSession) EnqueueInput(content string) bool {
	select {
	case s.InputQueue <- WebInput{Content: content, Timestamp: time.Now()}:
		s.mu.Lock()
		s.LastSeen = time.Now()
		s.mu.Unlock()
		return true
	default:
		log.Printf("Session %s: input queue full", s.ID)
		return false
	}
}

func (s *WebSession) EnqueueOutput(chunk StreamChunk) {
	select {
	case s.OutputQueue <- chunk:
	default:
		select {
		case <-s.OutputQueue:
		default:
		}
		s.OutputQueue <- chunk
		log.Printf("Session %s: output queue full, dropped old chunk", s.ID)
	}
}

func (s *WebSession) TryStartTask() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TaskRunning {
		return false, ""
	}
	s.TaskRunning = true
	taskID := uuid.New().String()
	s.currentTaskID = taskID
	return true, taskID
}

func (s *WebSession) SetTaskRunning(running bool, taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentTaskID != taskID {
		return
	}
	s.TaskRunning = running
	if !running {
		s.currentTaskID = ""
	}
}

func (s *WebSession) IsTaskRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TaskRunning
}

func (s *WebSession) GetTaskCtx() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TaskCtx
}

func (s *WebSession) SetConnected(connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Connected = connected
	if !connected {
		s.LastSentIndex = len(s.History)
	}
}

func (s *WebSession) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Connected
}

func (s *WebSession) CancelWs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.WsCancel != nil {
		s.WsCancel()
	}
}

func (s *WebSession) ResetWsCtx() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.WsCtx.Done():
		s.WsCtx, s.WsCancel = context.WithCancel(context.Background())
	default:
	}
}

func (s *WebSession) CancelTask() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TaskCancel != nil && s.TaskRunning {
		log.Printf("[Session %s] CancelTask: cancelling task (taskID=%s)", s.ID, s.currentTaskID)
		s.TaskCancel()
		s.TaskCtx, s.TaskCancel = context.WithCancel(context.Background())
		s.TaskRunning = false
		s.currentTaskID = ""
	}
}

func (s *WebSession) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TaskCancel != nil {
		s.TaskCancel()
	}
	if s.WsCancel != nil {
		s.WsCancel()
	}
	close(s.InputQueue)
	close(s.OutputQueue)

	if globalBrowserSessionManager != nil {
		if err := globalBrowserSessionManager.CloseSession(s.ID); err != nil {
			log.Printf("[Session %s] Failed to close browser session: %v", s.ID, err)
		}
	}
}

// autoSaveHistory 自动保存当前会话到持久化存储
func (s *WebSession) autoSaveHistory() {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	s.mu.RLock()
	historyCopy := make([]Message, len(s.History))
	copy(historyCopy, s.History)
	sessionID := s.ID
	s.mu.RUnlock()

	if len(historyCopy) == 0 {
		return
	}

	// 生成描述：取第一条用户消息的前50字符
	description := "自动保存的会话"
	for _, msg := range historyCopy {
		if msg.Role == "user" {
			if content, ok := msg.Content.(string); ok && content != "" {
				desc := content
				if len(desc) > 50 {
					desc = desc[:50] + "..."
				}
				description = desc
				break
			}
		}
	}

	if s.persistID == "" {
		// 首次保存，创建新文件
		saved, err := globalSessionPersist.SaveSession(sessionID, historyCopy, description)
		if err != nil {
			log.Printf("[Session %s] Auto save failed (create): %v", sessionID, err)
			return
		}
		s.persistID = saved.ID
		log.Printf("[Session %s] Auto saved (new) with ID %s", sessionID, s.persistID)
	} else {
		// 已存在，更新文件
		err := globalSessionPersist.UpdateSession(s.persistID, historyCopy)
		if err != nil {
			// 更新失败（如文件被删除），尝试重新创建
			log.Printf("[Session %s] Auto save update failed: %v, trying to create new", sessionID, err)
			saved, err2 := globalSessionPersist.SaveSession(sessionID, historyCopy, description)
			if err2 != nil {
				log.Printf("[Session %s] Auto save re-create failed: %v", sessionID, err2)
				return
			}
			s.persistID = saved.ID
			log.Printf("[Session %s] Auto saved (re-created) with ID %s", sessionID, s.persistID)
		} else {
			log.Printf("[Session %s] Auto saved (update)", sessionID)
		}
	}
}

// WebSessionManager 保持不变
type WebSessionManager struct {
	sessions map[string]*WebSession
	mu       sync.RWMutex

	SessionTimeout time.Duration
	MaxSessions    int
}

func NewWebSessionManager() *WebSessionManager {
	sm := &WebSessionManager{
		sessions:      make(map[string]*WebSession),
		SessionTimeout: 30 * time.Minute,
		MaxSessions:    100,
	}
	go sm.cleanupLoop()
	return sm
}

func (sm *WebSessionManager) GetOrCreate(sessionID string) *WebSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sessionID != "" {
		if s, ok := sm.sessions[sessionID]; ok {
			s.LastSeen = time.Now()
			return s
		}
	}

	if len(sm.sessions) >= sm.MaxSessions {
		sm.cleanupOldestLocked()
	}

	s := NewWebSession()
	sm.sessions[s.ID] = s
	return s
}

func (sm *WebSessionManager) Get(sessionID string) *WebSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if s, ok := sm.sessions[sessionID]; ok {
		return s
	}
	return nil
}

func (sm *WebSessionManager) Remove(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[sessionID]; ok {
		s.Stop()
		delete(sm.sessions, sessionID)
	}
}

func (sm *WebSessionManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		sm.cleanup()
	}
}

func (sm *WebSessionManager) cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for id, s := range sm.sessions {
		s.mu.RLock()
		lastSeen := s.LastSeen
		running := s.TaskRunning
		connected := s.Connected
		s.mu.RUnlock()

		if running || connected {
			continue
		}
		if now.Sub(lastSeen) > sm.SessionTimeout {
			log.Printf("Session %s expired, removing", id)
			s.Stop()
			delete(sm.sessions, id)
		}
	}
}

func (sm *WebSessionManager) cleanupOldestLocked() {
	var oldestID string
	var oldestTime time.Time

	for id, s := range sm.sessions {
		s.mu.RLock()
		lastSeen := s.LastSeen
		running := s.TaskRunning
		s.mu.RUnlock()

		if running {
			continue
		}
		if oldestID == "" || lastSeen.Before(oldestTime) {
			oldestID = id
			oldestTime = lastSeen
		}
	}

	if oldestID != "" {
		log.Printf("Max sessions reached, removing oldest session %s", oldestID)
		sm.sessions[oldestID].Stop()
		delete(sm.sessions, oldestID)
	}
}

func (sm *WebSessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}
