package main

import (
        "encoding/json"
        "log"
        "os"
        "path/filepath"
        "sync"
        "time"

        "github.com/toon-format/toon-go"
)

// SessionState 会话状态
type SessionState struct {
        // 基本信息
        SessionID   string    `json:"session_id"`
        CreatedAt   time.Time `json:"created_at"`
        UpdatedAt   time.Time `json:"updated_at"`
        Description string    `json:"description,omitempty"`

        // 场景状态
        Stage StageState `json:"stage"`

        // 对话历史
        Messages []Message `json:"messages"`
}

// SessionManager 会话管理器
type SessionManager struct {
        mu           sync.RWMutex
        sessionsDir  string
        currentState *SessionState
}

// NewSessionManager 创建会话管理器
func NewSessionManager(sessionsDir string) (*SessionManager, error) {
        // 确保目录存在
        if err := os.MkdirAll(sessionsDir, 0755); err != nil {
                return nil, err
        }

        sm := &SessionManager{
                sessionsDir: sessionsDir,
        }

        // 创建新的会话状态
        sm.currentState = &SessionState{
                SessionID:  generateSessionID(),
                CreatedAt:  time.Now(),
                UpdatedAt:  time.Now(),
                Stage:      StageState{},
                Messages:   make([]Message, 0),
        }

        return sm, nil
}

// generateSessionID 生成会话ID
func generateSessionID() string {
        return time.Now().Format("20060102_150405")
}

// GetCurrentState 获取当前会话状态
func (sm *SessionManager) GetCurrentState() *SessionState {
        sm.mu.RLock()
        defer sm.mu.RUnlock()
        return sm.currentState
}

// UpdateMessages 更新消息历史
func (sm *SessionManager) UpdateMessages(messages []Message) {
        sm.mu.Lock()
        defer sm.mu.Unlock()
        sm.currentState.Messages = messages
        sm.currentState.UpdatedAt = time.Now()
}

// UpdateStage 更新场景状态
func (sm *SessionManager) UpdateStage(stage *Stage) {
        sm.mu.Lock()
        defer sm.mu.Unlock()

        sm.currentState.Stage = StageState{
                CurrentActor:   stage.GetCurrentActor(),
                PresentActors:  stage.GetPresentActors(),
                Setting:        stage.GetSetting(),
                AutoSwitch:     sm.currentState.Stage.AutoSwitch, // 保持原有的自动切换配置
        }
        sm.currentState.UpdatedAt = time.Now()
}

// Save 保存当前会话到文件
func (sm *SessionManager) Save(description string) error {
        sm.mu.Lock()
        defer sm.mu.Unlock()

        if description != "" {
                sm.currentState.Description = description
        }
        sm.currentState.UpdatedAt = time.Now()

        // 文件名使用会话ID
        filename := sm.currentState.SessionID + ".session.toon"
        filepath := filepath.Join(sm.sessionsDir, filename)

        data, err := toon.Marshal(sm.currentState)
        if err != nil {
                return err
        }

        return os.WriteFile(filepath, data, 0644)
}

// SaveAs 另存为新会话
func (sm *SessionManager) SaveAs(sessionID, description string) error {
        sm.mu.Lock()
        defer sm.mu.Unlock()

        // 创建新的会话状态（复制当前状态）
        newState := *sm.currentState
        newState.SessionID = sessionID
        newState.Description = description
        newState.UpdatedAt = time.Now()

        filename := sessionID + ".session.toon"
        filepath := filepath.Join(sm.sessionsDir, filename)

        data, err := toon.Marshal(&newState)
        if err != nil {
                return err
        }

        return os.WriteFile(filepath, data, 0644)
}

// Load 加载指定会话
func (sm *SessionManager) Load(sessionID string) error {
        sm.mu.Lock()
        defer sm.mu.Unlock()

        filename := sessionID + ".session.toon"
        filepath := filepath.Join(sm.sessionsDir, filename)

        data, err := os.ReadFile(filepath)
        if err != nil {
                return err
        }

        var state SessionState
        if err := toon.Unmarshal(data, &state); err != nil {
                return err
        }

        sm.currentState = &state
        return nil
}

// ListSessions 列出所有会话
func (sm *SessionManager) ListSessions() ([]SessionState, error) {
        sm.mu.RLock()
        defer sm.mu.RUnlock()

        files, err := os.ReadDir(sm.sessionsDir)
        if err != nil {
                return nil, err
        }

        var sessions []SessionState
        for _, file := range files {
                if !file.IsDir() && filepath.Ext(file.Name()) == ".toon" {
                        // 只读取基本信息，不加载消息历史
                        filepath := filepath.Join(sm.sessionsDir, file.Name())
                        data, err := os.ReadFile(filepath)
                        if err != nil {
                                continue
                        }

                        // 解析基本信息
                        var basicInfo struct {
                                SessionID   string    `json:"session_id"`
                                CreatedAt   time.Time `json:"created_at"`
                                UpdatedAt   time.Time `json:"updated_at"`
                                Description string    `json:"description"`
                        }

                        if err := json.Unmarshal(data, &basicInfo); err != nil {
                                // 尝试用 toon 格式解析
                                if err := toon.Unmarshal(data, &basicInfo); err != nil {
                                        continue
                                }
                        }

                        sessions = append(sessions, SessionState{
                                SessionID:   basicInfo.SessionID,
                                CreatedAt:   basicInfo.CreatedAt,
                                UpdatedAt:   basicInfo.UpdatedAt,
                                Description: basicInfo.Description,
                        })
                }
        }

        return sessions, nil
}

// DeleteSession 删除指定会话
func (sm *SessionManager) DeleteSession(sessionID string) error {
        sm.mu.Lock()
        defer sm.mu.Unlock()

        filename := sessionID + ".session.toon"
        filepath := filepath.Join(sm.sessionsDir, filename)

        return os.Remove(filepath)
}

// NewSession 创建新会话
func (sm *SessionManager) NewSession() {
        sm.mu.Lock()
        defer sm.mu.Unlock()

        sm.currentState = &SessionState{
                SessionID:  generateSessionID(),
                CreatedAt:  time.Now(),
                UpdatedAt:  time.Now(),
                Stage:      StageState{},
                Messages:   make([]Message, 0),
        }
}

// GetSessionFilePath 获取会话文件路径
func (sm *SessionManager) GetSessionFilePath() string {
        return filepath.Join(sm.sessionsDir, sm.currentState.SessionID+".session.toon")
}

// QuickSave 快速保存（自动保存）
func (sm *SessionManager) QuickSave() {
        if err := sm.Save(""); err != nil {
                log.Printf("Quick save failed: %v", err)
        }
}

// AutoSaveEnabled 是否启用自动保存
var AutoSaveEnabled = true

// AutoSaveInterval 自动保存间隔
var AutoSaveInterval = 5 * time.Minute

// StartAutoSave 启动自动保存
func (sm *SessionManager) StartAutoSave(stopCh <-chan struct{}) {
        ticker := time.NewTicker(AutoSaveInterval)
        go func() {
                for {
                        select {
                        case <-stopCh:
                                ticker.Stop()
                                return
                        case <-ticker.C:
                                if AutoSaveEnabled {
                                        sm.QuickSave()
                                }
                        }
                }
        }()
}

// ExportToJSON 导出会话为JSON（用于备份或迁移）
func (sm *SessionManager) ExportToJSON(filepath string) error {
        sm.mu.RLock()
        defer sm.mu.RUnlock()

        data, err := json.MarshalIndent(sm.currentState, "", "  ")
        if err != nil {
                return err
        }

        return os.WriteFile(filepath, data, 0644)
}

// ImportFromJSON 从JSON导入会话
func (sm *SessionManager) ImportFromJSON(filepath string) error {
        sm.mu.Lock()
        defer sm.mu.Unlock()

        data, err := os.ReadFile(filepath)
        if err != nil {
                return err
        }

        var state SessionState
        if err := json.Unmarshal(data, &state); err != nil {
                return err
        }

        sm.currentState = &state
        return nil
}
