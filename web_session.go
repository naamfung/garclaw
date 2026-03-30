package main

import (
        "context"
        "log"
        "sync"
        "time"

        "github.com/google/uuid"
)

// WebInput 用户输入
type WebInput struct {
        Content   string
        Timestamp time.Time
}

// WebSession 会话对象，独立于 WebSocket 连接
type WebSession struct {
        ID        string
        History   []Message
        CreatedAt time.Time
        LastSeen  time.Time

        // 输入输出队列
        InputQueue  chan WebInput     // 用户输入队列
        OutputQueue chan StreamChunk  // 输出缓冲队列

        // 任务控制（用于取消当前正在执行的任务）
        TaskRunning   bool
        currentTaskID string            // 当前任务唯一ID
        TaskCtx       context.Context
        TaskCancel    context.CancelFunc

        // WebSocket 连接控制（独立于任务控制，用于管理 WebSocket 生命周期）
        WsCtx    context.Context
        WsCancel context.CancelFunc

        // 连接状态
        Connected   bool
        OutputDone  chan struct{} // 通知输出协程结束

        // 消息同步：跟踪已发送给前端的消息索引
        LastSentIndex int

        mu sync.RWMutex
}

// NewWebSession 创建新会话
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

// AddToHistory 添加消息到历史
func (s *WebSession) AddToHistory(role, content string) {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.History = append(s.History, Message{Role: role, Content: content})
        s.LastSeen = time.Now()
}

// GetHistory 获取历史副本
func (s *WebSession) GetHistory() []Message {
        s.mu.RLock()
        defer s.mu.RUnlock()
        h := make([]Message, len(s.History))
        copy(h, s.History)
        return h
}

// SetHistory 设置历史（任务完成后更新）
func (s *WebSession) SetHistory(h []Message) {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.History = h
        s.LastSeen = time.Now()
}

// GetNewMessages 获取未发送给前端的新消息（用于重连时同步）
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

// MarkAllSent 标记所有消息已发送给前端
func (s *WebSession) MarkAllSent() {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.LastSentIndex = len(s.History)
}

// EnqueueInput 将用户输入加入队列
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

// EnqueueOutput 将输出加入队列（非阻塞，队列满时丢弃旧数据）
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

// TryStartTask 尝试启动任务（原子操作）
// 返回 (是否成功, 任务ID)
func (s *WebSession) TryStartTask() (bool, string) {
        s.mu.Lock()
        defer s.mu.Unlock()
        if s.TaskRunning {
                log.Printf("[Session %s] TryStartTask: rejected (TaskRunning=true)", s.ID)
                return false, ""
        }
        s.TaskRunning = true
        taskID := uuid.New().String()
        s.currentTaskID = taskID
        log.Printf("[Session %s] TryStartTask: accepted, taskID=%s", s.ID, taskID)
        return true, taskID
}

// SetTaskRunning 设置任务运行状态，仅当 taskID 匹配时才执行
func (s *WebSession) SetTaskRunning(running bool, taskID string) {
        s.mu.Lock()
        defer s.mu.Unlock()
        if s.currentTaskID != taskID {
                log.Printf("[Session %s] SetTaskRunning: taskID mismatch (got %s, expected %s), ignoring", s.ID, taskID, s.currentTaskID)
                return
        }
        old := s.TaskRunning
        s.TaskRunning = running
        if !running {
                s.currentTaskID = ""
        }
        log.Printf("[Session %s] SetTaskRunning: %v -> %v (taskID=%s)", s.ID, old, running, taskID)
}

// IsTaskRunning 检查任务是否运行中
func (s *WebSession) IsTaskRunning() bool {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return s.TaskRunning
}

// GetTaskCtx 获取当前任务 context（用于判断是否被取消）
func (s *WebSession) GetTaskCtx() context.Context {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return s.TaskCtx
}

// SetConnected 设置连接状态
func (s *WebSession) SetConnected(connected bool) {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.Connected = connected
        if !connected {
                s.LastSentIndex = len(s.History)
        }
}

// IsConnected 检查是否已连接
func (s *WebSession) IsConnected() bool {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return s.Connected
}

// CancelWs 取消 WebSocket 连接（当连接断开时调用）
func (s *WebSession) CancelWs() {
        s.mu.Lock()
        defer s.mu.Unlock()
        if s.WsCancel != nil {
                s.WsCancel()
        }
}

// ResetWsCtx 重置 WebSocket context（当重新连接时调用）
func (s *WebSession) ResetWsCtx() {
        s.mu.Lock()
        defer s.mu.Unlock()
        select {
        case <-s.WsCtx.Done():
                s.WsCtx, s.WsCancel = context.WithCancel(context.Background())
        default:
        }
}

// CancelTask 取消当前任务
func (s *WebSession) CancelTask() {
        s.mu.Lock()
        defer s.mu.Unlock()
        if s.TaskCancel != nil && s.TaskRunning {
                log.Printf("[Session %s] CancelTask: cancelling task (taskID=%s)", s.ID, s.currentTaskID)
                s.TaskCancel()
                // 重置 context 和状态
                s.TaskCtx, s.TaskCancel = context.WithCancel(context.Background())
                s.TaskRunning = false
                s.currentTaskID = ""
                log.Printf("[Session %s] CancelTask: TaskRunning set to false", s.ID)
        } else {
                log.Printf("[Session %s] CancelTask: no task to cancel (TaskRunning=%v)", s.ID, s.TaskRunning)
        }
}

// Stop 停止会话（清理资源）
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
}

// WebSessionManager 会话管理器
type WebSessionManager struct {
        sessions map[string]*WebSession
        mu       sync.RWMutex

        SessionTimeout time.Duration
        MaxSessions    int
}

// NewWebSessionManager 创建会话管理器
func NewWebSessionManager() *WebSessionManager {
        sm := &WebSessionManager{
                sessions:      make(map[string]*WebSession),
                SessionTimeout: 30 * time.Minute,
                MaxSessions:    100,
        }
        go sm.cleanupLoop()
        return sm
}

// GetOrCreate 获取或创建会话
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

// Get 获取会话（不存在返回 nil）
func (sm *WebSessionManager) Get(sessionID string) *WebSession {
        sm.mu.RLock()
        defer sm.mu.RUnlock()
        if s, ok := sm.sessions[sessionID]; ok {
                return s
        }
        return nil
}

// Remove 移除会话
func (sm *WebSessionManager) Remove(sessionID string) {
        sm.mu.Lock()
        defer sm.mu.Unlock()
        if s, ok := sm.sessions[sessionID]; ok {
                s.Stop()
                delete(sm.sessions, sessionID)
        }
}

// cleanupLoop 定期清理过期会话
func (sm *WebSessionManager) cleanupLoop() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
                sm.cleanup()
        }
}

// cleanup 清理过期会话
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

// cleanupOldestLocked 清理最旧的会话（已持有锁）
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

// Count 返回当前会话数
func (sm *WebSessionManager) Count() int {
        sm.mu.RLock()
        defer sm.mu.RUnlock()
        return len(sm.sessions)
}
