package main

import (
        "bufio"
        "fmt"
        "log"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "time"
)

// ============================================================
// 两层记忆系统
// MEMORY.md - 长期记忆（用户偏好、事实、项目信息）
// HISTORY.md - 会话历史记录
// ============================================================

// TwoLayerMemorySystem 两层记忆系统
type TwoLayerMemorySystem struct {
        memoryDir string
        mu        sync.RWMutex

        // 长期记忆（MEMORY.md）
        longTermMemory *LongTermMemory

        // 会话历史（HISTORY.md）
        sessionHistory *SessionHistory
}

// LongTermMemory 长期记忆
type LongTermMemory struct {
        filePath string
        entries  []LongTermEntry
        mu       sync.RWMutex
}

// LongTermEntry 长期记忆条目
type LongTermEntry struct {
        Category  string    // 分类：preference, fact, project, skill
        Key       string    // 键名
        Value     string    // 值
        CreatedAt time.Time // 创建时间
        UpdatedAt time.Time // 更新时间
        Source    string    // 来源（用户输入/系统推断）
}

// SessionHistory 会话历史
type SessionHistory struct {
        filePath string
        sessions []SessionRecord
        mu       sync.RWMutex
}

// SessionRecord 会话记录
type SessionRecord struct {
        SessionID   string    // 会话 ID
        StartTime   time.Time // 开始时间
        EndTime     time.Time // 结束时间
        MessageCount int      // 消息数量
        Summary     string    // 会话摘要
        Tags        []string  // 标签
}

// NewTwoLayerMemorySystem 创建两层记忆系统
func NewTwoLayerMemorySystem(workDir string) (*TwoLayerMemorySystem, error) {
        memoryDir := filepath.Join(workDir, "memory")
        if err := os.MkdirAll(memoryDir, 0755); err != nil {
                return nil, fmt.Errorf("failed to create memory directory: %w", err)
        }

        system := &TwoLayerMemorySystem{
                memoryDir: memoryDir,
        }

        // 初始化长期记忆
        longTerm, err := NewLongTermMemory(filepath.Join(memoryDir, "MEMORY.md"))
        if err != nil {
                return nil, fmt.Errorf("failed to init long-term memory: %w", err)
        }
        system.longTermMemory = longTerm

        // 初始化会话历史
        history, err := NewSessionHistory(filepath.Join(memoryDir, "HISTORY.md"))
        if err != nil {
                return nil, fmt.Errorf("failed to init session history: %w", err)
        }
        system.sessionHistory = history

        // 确保文件存在（如果不存在则创建初始文件）
        if err := system.ensureFilesExist(); err != nil {
                log.Printf("[Memory] Warning: failed to ensure memory files exist: %v", err)
        }

        return system, nil
}

// ensureFilesExist 确保记忆文件存在
func (t *TwoLayerMemorySystem) ensureFilesExist() error {
        // 确保 MEMORY.md 存在
        if _, err := os.Stat(t.longTermMemory.filePath); os.IsNotExist(err) {
                defaultContent := `# 长期记忆 (MEMORY.md)

此文件存储用户的长期记忆，包括偏好、事实、项目信息等。
由 GarClaw AI Agent 自动维护。

## Preferences



## Facts



## Projects



## Skills

`
                if err := os.WriteFile(t.longTermMemory.filePath, []byte(defaultContent), 0644); err != nil {
                        return fmt.Errorf("failed to create MEMORY.md: %w", err)
                }
                log.Printf("[Memory] Created default MEMORY.md")
        }

        // 确保 HISTORY.md 存在
        if _, err := os.Stat(t.sessionHistory.filePath); os.IsNotExist(err) {
                defaultContent := `# 会话历史记录 (HISTORY.md)

此文件记录 AI 与用户的对话会话历史摘要。
由 GarClaw AI Agent 自动维护。

## 会话记录

`
                if err := os.WriteFile(t.sessionHistory.filePath, []byte(defaultContent), 0644); err != nil {
                        return fmt.Errorf("failed to create HISTORY.md: %w", err)
                }
                log.Printf("[Memory] Created default HISTORY.md")
        }

        return nil
}

// ============================================================
// 长期记忆（MEMORY.md）
// ============================================================

// NewLongTermMemory 创建长期记忆
func NewLongTermMemory(filePath string) (*LongTermMemory, error) {
        lm := &LongTermMemory{
                filePath: filePath,
                entries:  make([]LongTermEntry, 0),
        }

        // 加载已有记忆
        if err := lm.load(); err != nil && !os.IsNotExist(err) {
                log.Printf("[Memory] Warning: failed to load long-term memory: %v", err)
        }

        return lm, nil
}

// load 加载长期记忆
func (lm *LongTermMemory) load() error {
        file, err := os.Open(lm.filePath)
        if err != nil {
                return err
        }
        defer file.Close()

        scanner := bufio.NewScanner(file)
        var currentCategory string

        for scanner.Scan() {
                line := strings.TrimSpace(scanner.Text())

                // 跳过空行和注释
                if line == "" || strings.HasPrefix(line, "#") {
                        // 解析分类标题 ## Preferences, ## Facts, etc.
                        if strings.HasPrefix(line, "## ") {
                                currentCategory = strings.ToLower(strings.TrimPrefix(line, "## "))
                                switch currentCategory {
                                case "preferences":
                                        currentCategory = "preference"
                                case "facts":
                                        currentCategory = "fact"
                                case "projects":
                                        currentCategory = "project"
                                case "skills":
                                        currentCategory = "skill"
                                }
                        }
                        continue
                }

                // 解析条目：- key: value
                if strings.HasPrefix(line, "- ") {
                        line = strings.TrimPrefix(line, "- ")
                        parts := strings.SplitN(line, ":", 2)
                        if len(parts) == 2 {
                                key := strings.TrimSpace(parts[0])
                                value := strings.TrimSpace(parts[1])
                                lm.entries = append(lm.entries, LongTermEntry{
                                        Category:  currentCategory,
                                        Key:       key,
                                        Value:     value,
                                        UpdatedAt: time.Now(),
                                })
                        }
                }
        }

        log.Printf("[Memory] Loaded %d long-term memory entries", len(lm.entries))
        return scanner.Err()
}

// save 保存长期记忆
func (lm *LongTermMemory) save() error {
        var sb strings.Builder

        sb.WriteString("# 长期记忆 (MEMORY.md)\n\n")
        sb.WriteString("此文件存储用户的长期记忆，包括偏好、事实、项目信息等。\n")
        sb.WriteString("由 GarClaw AI Agent 自动维护。\n\n")

        // 按分类组织
        categories := map[string][]LongTermEntry{
                "preference": {},
                "fact":       {},
                "project":    {},
                "skill":      {},
        }

        for _, entry := range lm.entries {
                if list, ok := categories[entry.Category]; ok {
                        categories[entry.Category] = append(list, entry)
                }
        }

        // 写入各分类
        categoryNames := map[string]string{
                "preference": "Preferences",
                "fact":       "Facts",
                "project":    "Projects",
                "skill":      "Skills",
        }

        for cat, name := range categoryNames {
                entries := categories[cat]
                if len(entries) > 0 {
                        sb.WriteString(fmt.Sprintf("## %s\n\n", name))
                        for _, e := range entries {
                                sb.WriteString(fmt.Sprintf("- %s: %s\n", e.Key, e.Value))
                        }
                        sb.WriteString("\n")
                }
        }

        return os.WriteFile(lm.filePath, []byte(sb.String()), 0644)
}

// Save 保存一条长期记忆
func (lm *LongTermMemory) Save(category, key, value string) error {
        lm.mu.Lock()
        defer lm.mu.Unlock()

        now := time.Now()

        // 查找现有条目
        for i, e := range lm.entries {
                if e.Key == key {
                        lm.entries[i].Value = value
                        lm.entries[i].UpdatedAt = now
                        return lm.save()
                }
        }

        // 添加新条目
        lm.entries = append(lm.entries, LongTermEntry{
                Category:  category,
                Key:       key,
                Value:     value,
                CreatedAt: now,
                UpdatedAt: now,
        })

        return lm.save()
}

// Get 获取长期记忆
func (lm *LongTermMemory) Get(key string) (string, bool) {
        lm.mu.RLock()
        defer lm.mu.RUnlock()

        for _, e := range lm.entries {
                if e.Key == key {
                        return e.Value, true
                }
        }
        return "", false
}

// Recall 模糊搜索长期记忆
func (lm *LongTermMemory) Recall(query string) []LongTermEntry {
        lm.mu.RLock()
        defer lm.mu.RUnlock()

        var results []LongTermEntry
        queryLower := strings.ToLower(query)

        for _, e := range lm.entries {
                if strings.Contains(strings.ToLower(e.Key), queryLower) ||
                        strings.Contains(strings.ToLower(e.Value), queryLower) {
                        results = append(results, e)
                }
        }

        return results
}

// Delete 删除长期记忆
func (lm *LongTermMemory) Delete(key string) error {
        lm.mu.Lock()
        defer lm.mu.Unlock()

        for i, e := range lm.entries {
                if e.Key == key {
                        lm.entries = append(lm.entries[:i], lm.entries[i+1:]...)
                        return lm.save()
                }
        }

        return fmt.Errorf("memory key not found: %s", key)
}

// List 列出所有长期记忆
func (lm *LongTermMemory) List(category string) []LongTermEntry {
        lm.mu.RLock()
        defer lm.mu.RUnlock()

        if category == "" {
                return lm.entries
        }

        var results []LongTermEntry
        for _, e := range lm.entries {
                if e.Category == category {
                        results = append(results, e)
                }
        }
        return results
}

// GetContextForPrompt 获取用于注入提示的长期记忆上下文
func (lm *LongTermMemory) GetContextForPrompt() string {
        lm.mu.RLock()
        defer lm.mu.RUnlock()

        if len(lm.entries) == 0 {
                return ""
        }

        var sb strings.Builder
        sb.WriteString("# 关于用户的记忆\n\n")

        // 按分类组织
        categories := map[string][]LongTermEntry{}
        for _, e := range lm.entries {
                categories[e.Category] = append(categories[e.Category], e)
        }

        categoryTitles := map[string]string{
                "preference": "用户偏好",
                "fact":       "基本事实",
                "project":    "项目信息",
                "skill":      "技能/能力",
        }

        for cat, title := range categoryTitles {
                entries := categories[cat]
                if len(entries) > 0 {
                        sb.WriteString(fmt.Sprintf("## %s\n", title))
                        for _, e := range entries {
                                sb.WriteString(fmt.Sprintf("- %s: %s\n", e.Key, e.Value))
                        }
                        sb.WriteString("\n")
                }
        }

        return sb.String()
}

// ============================================================
// 会话历史（HISTORY.md）
// ============================================================

// NewSessionHistory 创建会话历史
func NewSessionHistory(filePath string) (*SessionHistory, error) {
        sh := &SessionHistory{
                filePath: filePath,
                sessions: make([]SessionRecord, 0),
        }

        // 加载已有历史
        if err := sh.load(); err != nil && !os.IsNotExist(err) {
                log.Printf("[History] Warning: failed to load session history: %v", err)
        }

        return sh, nil
}

// load 加载会话历史
func (sh *SessionHistory) load() error {
        file, err := os.Open(sh.filePath)
        if err != nil {
                return err
        }
        defer file.Close()

        scanner := bufio.NewScanner(file)

        for scanner.Scan() {
                line := strings.TrimSpace(scanner.Text())

                // 解析会话记录行
                // 格式: [2024-01-15 10:30] session_abc123 | 15 messages | 总结内容... | tag1,tag2
                if strings.HasPrefix(line, "[") {
                        sh.parseSessionLine(line)
                }
        }

        log.Printf("[History] Loaded %d session records", len(sh.sessions))
        return scanner.Err()
}

// parseSessionLine 解析会话行
func (sh *SessionHistory) parseSessionLine(line string) {
        // 提取时间戳
        endBracket := strings.Index(line, "]")
        if endBracket == -1 {
                return
        }

        timeStr := line[1:endBracket]
        startTime, _ := time.Parse("2006-01-02 15:04", timeStr)

        // 解析其余部分
        rest := strings.TrimSpace(line[endBracket+1:])
        parts := strings.Split(rest, "|")

        if len(parts) < 2 {
                return
        }

        record := SessionRecord{
                StartTime: startTime,
        }

        // 解析会话 ID 和消息数
        info := strings.TrimSpace(parts[0])
        if strings.Contains(info, "|") {
                infoParts := strings.Split(info, "|")
                record.SessionID = strings.TrimSpace(infoParts[0])
                if len(infoParts) > 1 {
                        fmt.Sscanf(strings.TrimSpace(infoParts[1]), "%d messages", &record.MessageCount)
                }
        } else {
                record.SessionID = info
        }

        // 解析摘要
        if len(parts) >= 2 {
                record.Summary = strings.TrimSpace(parts[1])
        }

        // 解析标签
        if len(parts) >= 3 {
                tagsStr := strings.TrimSpace(parts[2])
                for _, tag := range strings.Split(tagsStr, ",") {
                        tag = strings.TrimSpace(tag)
                        if tag != "" {
                                record.Tags = append(record.Tags, tag)
                        }
                }
        }

        sh.sessions = append(sh.sessions, record)
}

// save 保存会话历史
func (sh *SessionHistory) save() error {
        var sb strings.Builder

        sb.WriteString("# 会话历史记录 (HISTORY.md)\n\n")
        sb.WriteString("此文件记录 AI 与用户的对话会话历史摘要。\n")
        sb.WriteString("由 GarClaw AI Agent 自动维护。\n\n")

        sb.WriteString("## 会话记录\n\n")

        for _, s := range sh.sessions {
                timeStr := s.StartTime.Format("2006-01-02 15:04")
                tagsStr := strings.Join(s.Tags, ", ")
                sb.WriteString(fmt.Sprintf("[%s] %s | %d messages | %s | %s\n",
                        timeStr, s.SessionID, s.MessageCount, s.Summary, tagsStr))
        }

        return os.WriteFile(sh.filePath, []byte(sb.String()), 0644)
}

// RecordSession 记录一个会话
func (sh *SessionHistory) RecordSession(sessionID string, messageCount int, summary string, tags []string) error {
        sh.mu.Lock()
        defer sh.mu.Unlock()

        now := time.Now()

        // 查找现有会话
        for i, s := range sh.sessions {
                if s.SessionID == sessionID {
                        sh.sessions[i].EndTime = now
                        sh.sessions[i].MessageCount = messageCount
                        if summary != "" {
                                sh.sessions[i].Summary = summary
                        }
                        if tags != nil {
                                sh.sessions[i].Tags = tags
                        }
                        return sh.save()
                }
        }

        // 添加新会话记录
        sh.sessions = append(sh.sessions, SessionRecord{
                SessionID:    sessionID,
                StartTime:    now,
                EndTime:      now,
                MessageCount: messageCount,
                Summary:      summary,
                Tags:         tags,
        })

        // 只保留最近 100 个会话
        if len(sh.sessions) > 100 {
                sh.sessions = sh.sessions[len(sh.sessions)-100:]
        }

        return sh.save()
}

// GetRecentSessions 获取最近的会话
func (sh *SessionHistory) GetRecentSessions(limit int) []SessionRecord {
        sh.mu.RLock()
        defer sh.mu.RUnlock()

        if limit <= 0 || limit > len(sh.sessions) {
                limit = len(sh.sessions)
        }

        // 返回最近的会话（倒序）
        result := make([]SessionRecord, limit)
        for i := 0; i < limit; i++ {
                result[i] = sh.sessions[len(sh.sessions)-1-i]
        }
        return result
}

// SearchSessions 搜索会话历史
func (sh *SessionHistory) SearchSessions(query string) []SessionRecord {
        sh.mu.RLock()
        defer sh.mu.RUnlock()

        var results []SessionRecord
        queryLower := strings.ToLower(query)

        for _, s := range sh.sessions {
                if strings.Contains(strings.ToLower(s.Summary), queryLower) {
                        results = append(results, s)
                        continue
                }
                for _, tag := range s.Tags {
                        if strings.Contains(strings.ToLower(tag), queryLower) {
                                results = append(results, s)
                                break
                        }
                }
        }

        return results
}

// GetContextForPrompt 获取用于注入提示的会话历史上下文
func (sh *SessionHistory) GetContextForPrompt(limit int) string {
        sh.mu.RLock()
        defer sh.mu.RUnlock()

        if len(sh.sessions) == 0 {
                return ""
        }

        if limit <= 0 || limit > 10 {
                limit = 5
        }

        var sb strings.Builder
        sb.WriteString("# 最近的对话记录\n\n")

        // 获取最近的会话
        start := len(sh.sessions) - limit
        if start < 0 {
                start = 0
        }

        for i := len(sh.sessions) - 1; i >= start; i-- {
                s := sh.sessions[i]
                timeStr := s.StartTime.Format("2006-01-02 15:04")
                sb.WriteString(fmt.Sprintf("- [%s] %s\n", timeStr, s.Summary))
        }

        return sb.String()
}

// ============================================================
// 两层记忆系统便捷方法
// ============================================================

// SaveLongTerm 保存长期记忆
func (t *TwoLayerMemorySystem) SaveLongTerm(category, key, value string) error {
        return t.longTermMemory.Save(category, key, value)
}

// GetLongTerm 获取长期记忆
func (t *TwoLayerMemorySystem) GetLongTerm(key string) (string, bool) {
        return t.longTermMemory.Get(key)
}

// RecallLongTerm 模糊搜索长期记忆
func (t *TwoLayerMemorySystem) RecallLongTerm(query string) []LongTermEntry {
        return t.longTermMemory.Recall(query)
}

// RecordSession 记录会话
func (t *TwoLayerMemorySystem) RecordSession(sessionID string, messageCount int, summary string, tags []string) error {
        return t.sessionHistory.RecordSession(sessionID, messageCount, summary, tags)
}

// GetRecentSessions 获取最近会话
func (t *TwoLayerMemorySystem) GetRecentSessions(limit int) []SessionRecord {
        return t.sessionHistory.GetRecentSessions(limit)
}

// GetFullContext 获取完整的记忆上下文（用于注入提示）
func (t *TwoLayerMemorySystem) GetFullContext() string {
        var sb strings.Builder

        // 长期记忆
        if context := t.longTermMemory.GetContextForPrompt(); context != "" {
                sb.WriteString(context)
                sb.WriteString("\n")
        }

        // 会话历史
        if context := t.sessionHistory.GetContextForPrompt(5); context != "" {
                sb.WriteString(context)
        }

        return sb.String()
}
