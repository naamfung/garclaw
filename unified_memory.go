package main

import (
    "encoding/json"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "sync"
    "time"

    "github.com/google/uuid"
)

// ============================================================
// 统一记忆系统
// ============================================================

type MemoryCategory string

const (
    MemoryCategoryPreference MemoryCategory = "preference"
    MemoryCategoryFact       MemoryCategory = "fact"
    MemoryCategoryProject    MemoryCategory = "project"
    MemoryCategorySkill      MemoryCategory = "skill"
    MemoryCategoryContext    MemoryCategory = "context"
    MemoryCategoryExperience MemoryCategory = "experience"
)

type MemoryScope string

const (
    MemoryScopeUser   MemoryScope = "user"
    MemoryScopeGlobal MemoryScope = "global"
)

type MemoryEntry struct {
    ID         string
    Category   MemoryCategory
    Scope      MemoryScope
    Key        string
    Value      string
    Tags       []string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    AccessCnt  int
    Score      float64
    TaskDesc   string
    Actions    []ExperienceAction
    Result     bool
    Summary    string
    SessionID  string
    UsedCount  int
}

type ExperienceAction struct {
    ToolName string                 `json:"tool_name"`
    Input    map[string]interface{} `json:"input"`
    Output   string                 `json:"output"`
}

type SessionRecord struct {
    SessionID    string    `json:"session_id"`
    Channel      string    `json:"channel"`
    StartTime    time.Time `json:"start_time"`
    EndTime      time.Time `json:"end_time"`
    MessageCount int       `json:"message_count"`
    Summary      string    `json:"summary"`
    Tags         []string  `json:"tags"`
    Experiences  []string  `json:"experiences"`
}

type UnifiedMemory struct {
    mu             sync.RWMutex
    memoryDir      string
    memoryFilePath string
    historyFilePath string
    entries        []MemoryEntry
    sessions       []SessionRecord
    autoSave       bool
}

func NewUnifiedMemory(workDir string) (*UnifiedMemory, error) {
    memoryDir := filepath.Join(workDir, "memory")
    if err := os.MkdirAll(memoryDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create memory directory: %w", err)
    }

    m := &UnifiedMemory{
        memoryDir:      memoryDir,
        memoryFilePath: filepath.Join(memoryDir, "MEMORY.md"),
        historyFilePath: filepath.Join(memoryDir, "HISTORY.md"),
        entries:        []MemoryEntry{},
        sessions:       []SessionRecord{},
        autoSave:       true,
    }

    if err := m.loadAll(); err != nil && !os.IsNotExist(err) {
        log.Printf("[Memory] Warning: failed to load memory: %v", err)
    }
    m.ensureFilesExist()
    return m, nil
}

func (m *UnifiedMemory) ensureFilesExist() {
    if _, err := os.Stat(m.memoryFilePath); os.IsNotExist(err) {
        defaultContent := `# 长期记忆

此文件存储用户的长期记忆，包括事实、偏好、项目、技能、经验等。
由 GarClaw AI Agent 自动维护。

## Facts

## Preferences

## Projects

## Skills

## Contexts

## Experiences

`
        os.WriteFile(m.memoryFilePath, []byte(defaultContent), 0644)
    }
    if _, err := os.Stat(m.historyFilePath); os.IsNotExist(err) {
        defaultContent := `# 会话历史记录

此文件记录 AI 与用户的对话会话历史摘要。
由 GarClaw AI Agent 自动维护。

## 会话记录

`
        os.WriteFile(m.historyFilePath, []byte(defaultContent), 0644)
    }
}

func (m *UnifiedMemory) loadAll() error {
    if err := m.loadMemoryFile(); err != nil {
        return err
    }
    return m.loadHistoryFile()
}

func (m *UnifiedMemory) loadMemoryFile() error {
    data, err := os.ReadFile(m.memoryFilePath)
    if err != nil {
        return err
    }

    lines := strings.Split(string(data), "\n")
    var currentCategory string
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        if strings.HasPrefix(line, "## ") {
            currentCategory = strings.ToLower(strings.TrimPrefix(line, "## "))
            switch currentCategory {
            case "facts": currentCategory = "fact"
            case "preferences": currentCategory = "preference"
            case "projects": currentCategory = "project"
            case "skills": currentCategory = "skill"
            case "contexts": currentCategory = "context"
            case "experiences": currentCategory = "experience"
            }
            continue
        }
        if strings.HasPrefix(line, "- ") {
            entry := MemoryEntry{
                Category: MemoryCategory(currentCategory),
                Scope:    MemoryScopeGlobal,
            }
            content := strings.TrimPrefix(line, "- ")
            parts := strings.SplitN(content, ":", 2)
            if len(parts) != 2 {
                continue
            }
            entry.Key = strings.TrimSpace(parts[0])
            rest := strings.TrimSpace(parts[1])

            if idx := strings.Index(rest, "[tags:"); idx != -1 {
                tagPart := rest[idx+6:]
                if end := strings.Index(tagPart, "]"); end != -1 {
                    tagStr := tagPart[:end]
                    for _, t := range strings.Split(tagStr, ",") {
                        entry.Tags = append(entry.Tags, strings.TrimSpace(t))
                    }
                    rest = strings.TrimSpace(rest[:idx])
                }
            }
            if idx := strings.Index(rest, "[score:"); idx != -1 {
                scorePart := rest[idx+7:]
                if end := strings.Index(scorePart, "]"); end != -1 {
                    fmt.Sscanf(scorePart[:end], "%f", &entry.Score)
                    rest = strings.TrimSpace(rest[:idx])
                }
            }
            entry.Value = rest
            entry.ID = uuid.New().String()
            entry.CreatedAt = time.Now()
            entry.UpdatedAt = time.Now()
            entry.AccessCnt = 0

            if entry.Category == MemoryCategoryExperience {
                var exp struct {
                    TaskDesc  string              `json:"task_desc"`
                    Actions   []ExperienceAction  `json:"actions"`
                    Result    bool                `json:"result"`
                    Summary   string              `json:"summary"`
                    SessionID string              `json:"session_id"`
                    Score     float64             `json:"score"`
                    UsedCount int                 `json:"used_count"`
                }
                if err := json.Unmarshal([]byte(entry.Value), &exp); err == nil {
                    entry.TaskDesc = exp.TaskDesc
                    entry.Actions = exp.Actions
                    entry.Result = exp.Result
                    entry.Summary = exp.Summary
                    entry.SessionID = exp.SessionID
                    entry.UsedCount = exp.UsedCount
                    if exp.Score > 0 {
                        entry.Score = exp.Score
                    }
                }
            }

            m.entries = append(m.entries, entry)
        }
    }
    return nil
}

func (m *UnifiedMemory) loadHistoryFile() error {
    data, err := os.ReadFile(m.historyFilePath)
    if err != nil {
        return err
    }

    lines := strings.Split(string(data), "\n")
    m.sessions = []SessionRecord{}
    inSessionSection := false
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        if strings.HasPrefix(line, "## 会话记录") {
            inSessionSection = true
            continue
        }
        if !inSessionSection {
            continue
        }
        if !strings.HasPrefix(line, "[") {
            continue
        }
        endBracket := strings.Index(line, "]")
        if endBracket == -1 {
            continue
        }
        timeStr := line[1:endBracket]
        startTime, err := time.Parse("2006-01-02 15:04", timeStr)
        if err != nil {
            startTime, err = time.Parse("2006-01-02 15:04:05", timeStr)
            if err != nil {
                log.Printf("[Memory] Failed to parse time %s: %v", timeStr, err)
                continue
            }
        }
        rest := strings.TrimSpace(line[endBracket+1:])
        parts := strings.Split(rest, "|")
        if len(parts) < 3 {
            continue
        }
        sessionID := strings.TrimSpace(parts[0])
        msgPart := strings.TrimSpace(parts[1])
        var msgCount int
        fmt.Sscanf(msgPart, "%d messages", &msgCount)
        summary := strings.TrimSpace(parts[2])
        var tags []string
        if len(parts) > 3 {
            tagPart := strings.TrimSpace(parts[3])
            if strings.HasPrefix(tagPart, "tags:") {
                tagPart = strings.TrimPrefix(tagPart, "tags:")
                for _, t := range strings.Split(tagPart, ",") {
                    tags = append(tags, strings.TrimSpace(t))
                }
            }
        }
        session := SessionRecord{
            SessionID:    sessionID,
            StartTime:    startTime,
            EndTime:      startTime,
            MessageCount: msgCount,
            Summary:      summary,
            Tags:         tags,
            Experiences:  []string{},
        }
        m.sessions = append(m.sessions, session)
    }

    sort.Slice(m.sessions, func(i, j int) bool {
        return m.sessions[i].StartTime.Before(m.sessions[j].StartTime)
    })
    log.Printf("[Memory] Loaded %d session records from %s", len(m.sessions), m.historyFilePath)
    return nil
}

func (m *UnifiedMemory) saveMemoryFile() error {
    var sb strings.Builder
    sb.WriteString("# 长期记忆\n\n此文件存储用户的长期记忆，包括事实、偏好、项目、技能、经验等。\n由 GarClaw AI Agent 自动维护。\n\n")

    groups := map[MemoryCategory][]MemoryEntry{
        MemoryCategoryPreference: {},
        MemoryCategoryFact:       {},
        MemoryCategoryProject:    {},
        MemoryCategorySkill:      {},
        MemoryCategoryContext:    {},
        MemoryCategoryExperience: {},
    }
    for _, e := range m.entries {
        groups[e.Category] = append(groups[e.Category], e)
    }

    titles := map[MemoryCategory]string{
        MemoryCategoryPreference: "Preferences",
        MemoryCategoryFact:       "Facts",
        MemoryCategoryProject:    "Projects",
        MemoryCategorySkill:      "Skills",
        MemoryCategoryContext:    "Contexts",
        MemoryCategoryExperience: "Experiences",
    }
    for cat, title := range titles {
        entries := groups[cat]
        if len(entries) == 0 {
            continue
        }
        sb.WriteString(fmt.Sprintf("## %s\n\n", title))
        for _, e := range entries {
            line := fmt.Sprintf("- %s: %s", e.Key, e.Value)
            if len(e.Tags) > 0 {
                line += fmt.Sprintf(" [tags: %s]", strings.Join(e.Tags, ","))
            }
            if e.Score > 0 {
                line += fmt.Sprintf(" [score: %.2f]", e.Score)
            }
            sb.WriteString(line + "\n")
        }
        sb.WriteString("\n")
    }

    tmp := m.memoryFilePath + ".tmp"
    if err := os.WriteFile(tmp, []byte(sb.String()), 0644); err != nil {
        return err
    }
    return os.Rename(tmp, m.memoryFilePath)
}

func (m *UnifiedMemory) saveHistoryFile() error {
    var sb strings.Builder
    sb.WriteString("# 会话历史记录\n\n此文件记录 AI 与用户的对话会话历史摘要。\n由 GarClaw AI Agent 自动维护。\n\n## 会话记录\n\n")
    for _, s := range m.sessions {
        sb.WriteString(fmt.Sprintf("[%s] %s | %d messages | %s | tags: %v\n",
            s.StartTime.Format("2006-01-02 15:04"), s.SessionID, s.MessageCount, s.Summary, s.Tags))
    }
    return os.WriteFile(m.historyFilePath, []byte(sb.String()), 0644)
}

// ========== 公开操作 ==========

func (m *UnifiedMemory) SaveEntry(category MemoryCategory, key, value string, tags []string, scope MemoryScope) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    now := time.Now()
    for i, e := range m.entries {
        if e.Category == category && e.Key == key {
            m.entries[i].Value = value
            m.entries[i].Tags = tags
            m.entries[i].Scope = scope
            m.entries[i].UpdatedAt = now
            m.entries[i].AccessCnt++
            return m.saveMemoryFile()
        }
    }
    newEntry := MemoryEntry{
        ID:        uuid.New().String(),
        Category:  category,
        Key:       key,
        Value:     value,
        Tags:      tags,
        Scope:     scope,
        CreatedAt: now,
        UpdatedAt: now,
        AccessCnt: 1,
    }
    m.entries = append(m.entries, newEntry)
    return m.saveMemoryFile()
}

func (m *UnifiedMemory) GetEntry(category MemoryCategory, key string) (MemoryEntry, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    for _, e := range m.entries {
        if e.Category == category && e.Key == key {
            return e, true
        }
    }
    return MemoryEntry{}, false
}

func (m *UnifiedMemory) DeleteEntry(category MemoryCategory, key string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    for i, e := range m.entries {
        if e.Category == category && e.Key == key {
            m.entries = append(m.entries[:i], m.entries[i+1:]...)
            return m.saveMemoryFile()
        }
    }
    return fmt.Errorf("memory not found: %s/%s", category, key)
}

func (m *UnifiedMemory) UpdateEntry(category MemoryCategory, key, newValue string, newTags []string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    for i, e := range m.entries {
        if e.Category == category && e.Key == key {
            m.entries[i].Value = newValue
            if newTags != nil {
                m.entries[i].Tags = newTags
            }
            m.entries[i].UpdatedAt = time.Now()
            m.entries[i].AccessCnt++
            return m.saveMemoryFile()
        }
    }
    return fmt.Errorf("memory not found: %s/%s", category, key)
}

func (m *UnifiedMemory) SearchEntries(category MemoryCategory, query string, limit int) []MemoryEntry {
    m.mu.RLock()
    defer m.mu.RUnlock()
    var results []MemoryEntry
    query = strings.ToLower(query)
    for _, e := range m.entries {
        if category != "" && e.Category != category {
            continue
        }
        if query != "" && !strings.Contains(strings.ToLower(e.Key), query) && !strings.Contains(strings.ToLower(e.Value), query) {
            continue
        }
        results = append(results, e)
    }
    sort.Slice(results, func(i, j int) bool {
        if results[i].Score != results[j].Score {
            return results[i].Score > results[j].Score
        }
        return results[i].AccessCnt > results[j].AccessCnt
    })
    if limit > 0 && len(results) > limit {
        results = results[:limit]
    }
    return results
}

func (m *UnifiedMemory) RecordExperience(taskDesc string, actions []ExperienceAction, result bool, sessionID string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    expID := uuid.New().String()
    exp := struct {
        TaskDesc  string              `json:"task_desc"`
        Actions   []ExperienceAction  `json:"actions"`
        Result    bool                `json:"result"`
        Summary   string              `json:"summary"`
        SessionID string              `json:"session_id"`
        Score     float64             `json:"score"`
        UsedCount int                 `json:"used_count"`
    }{
        TaskDesc:  taskDesc,
        Actions:   actions,
        Result:    result,
        Summary:   fmt.Sprintf("%s → %s", taskDesc, mapResult(result)),
        SessionID: sessionID,
        Score:     0.5,
        UsedCount: 0,
    }
    data, _ := json.Marshal(exp)
    entry := MemoryEntry{
        ID:        expID,
        Category:  MemoryCategoryExperience,
        Key:       expID,
        Value:     string(data),
        Tags:      []string{taskDesc},
        Scope:     MemoryScopeGlobal,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
        AccessCnt: 0,
        Score:     0.5,
        TaskDesc:  taskDesc,
        Actions:   actions,
        Result:    result,
        Summary:   exp.Summary,
        SessionID: sessionID,
        UsedCount: 0,
    }
    m.entries = append(m.entries, entry)
    for i, s := range m.sessions {
        if s.SessionID == sessionID {
            m.sessions[i].Experiences = append(m.sessions[i].Experiences, expID)
            break
        }
    }
    return m.saveMemoryFile()
}

func (m *UnifiedMemory) RetrieveExperiences(taskDesc string, limit int) []MemoryEntry {
    return m.SearchEntries(MemoryCategoryExperience, taskDesc, limit)
}

func (m *UnifiedMemory) UpdateExperienceRating(expID string, success bool) {
    m.mu.Lock()
    defer m.mu.Unlock()
    for i, e := range m.entries {
        if e.Category == MemoryCategoryExperience && e.Key == expID {
            if success {
                m.entries[i].Score += 0.1
                if m.entries[i].Score > 1.0 {
                    m.entries[i].Score = 1.0
                }
            } else {
                m.entries[i].Score -= 0.1
                if m.entries[i].Score < 0.0 {
                    m.entries[i].Score = 0.0
                }
            }
            m.entries[i].UsedCount++
            var expData struct {
                TaskDesc  string              `json:"task_desc"`
                Actions   []ExperienceAction  `json:"actions"`
                Result    bool                `json:"result"`
                Summary   string              `json:"summary"`
                SessionID string              `json:"session_id"`
                Score     float64             `json:"score"`
                UsedCount int                 `json:"used_count"`
            }
            if err := json.Unmarshal([]byte(m.entries[i].Value), &expData); err == nil {
                expData.Score = m.entries[i].Score
                expData.UsedCount = m.entries[i].UsedCount
                if newJSON, err := json.Marshal(expData); err == nil {
                    m.entries[i].Value = string(newJSON)
                }
            }
            m.saveMemoryFile()
            return
        }
    }
}

func (m *UnifiedMemory) RecordSession(sessionID, channel, summary string, messageCount int, tags []string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    now := time.Now()
    for i, s := range m.sessions {
        if s.SessionID == sessionID {
            m.sessions[i].EndTime = now
            m.sessions[i].MessageCount = messageCount
            m.sessions[i].Summary = summary
            m.sessions[i].Tags = tags
            m.saveHistoryFile()
            return
        }
    }
    m.sessions = append(m.sessions, SessionRecord{
        SessionID:    sessionID,
        Channel:      channel,
        StartTime:    now,
        EndTime:      now,
        MessageCount: messageCount,
        Summary:      summary,
        Tags:         tags,
        Experiences:  []string{},
    })
    m.saveHistoryFile()
}

func (m *UnifiedMemory) GetRecentSessions(limit int) []SessionRecord {
    m.mu.RLock()
    defer m.mu.RUnlock()
    if limit <= 0 || limit > len(m.sessions) {
        limit = len(m.sessions)
    }
    result := make([]SessionRecord, limit)
    for i := 0; i < limit; i++ {
        result[i] = m.sessions[len(m.sessions)-1-i]
    }
    return result
}

func (m *UnifiedMemory) GetContextForPrompt(taskDesc string) string {
    var sb strings.Builder
    facts := m.SearchEntries(MemoryCategoryFact, "", 5)
    prefs := m.SearchEntries(MemoryCategoryPreference, "", 3)
    projects := m.SearchEntries(MemoryCategoryProject, "", 3)
    skills := m.SearchEntries(MemoryCategorySkill, "", 3)
    if len(facts) > 0 || len(prefs) > 0 || len(projects) > 0 || len(skills) > 0 {
        sb.WriteString("## 关于用户的记忆\n\n")
        for _, f := range facts {
            sb.WriteString(fmt.Sprintf("- %s: %s\n", f.Key, f.Value))
        }
        for _, p := range prefs {
            sb.WriteString(fmt.Sprintf("- 偏好: %s: %s\n", p.Key, p.Value))
        }
        for _, pr := range projects {
            sb.WriteString(fmt.Sprintf("- 项目: %s: %s\n", pr.Key, pr.Value))
        }
        for _, s := range skills {
            sb.WriteString(fmt.Sprintf("- 技能: %s: %s\n", s.Key, s.Value))
        }
        sb.WriteString("\n")
    }
    exps := m.RetrieveExperiences(taskDesc, 3)
    if len(exps) > 0 {
        sb.WriteString("## 历史经验参考\n\n")
        for i, exp := range exps {
            status := "✅ 成功"
            if !exp.Result {
                status = "❌ 失败"
            }
            sb.WriteString(fmt.Sprintf("%d. %s (评分: %.2f)\n", i+1, exp.Summary, exp.Score))
            if len(exp.Actions) > 0 {
                sb.WriteString(fmt.Sprintf("   行动: %s\n", formatActions(exp.Actions)))
            }
            sb.WriteString(fmt.Sprintf("   结果: %s\n\n", status))
        }
    }
    return sb.String()
}

func mapResult(success bool) string {
    if success {
        return "成功"
    }
    return "失败"
}

func formatActions(actions []ExperienceAction) string {
    if len(actions) == 0 {
        return "无"
    }
    var sb strings.Builder
    for i, a := range actions {
        if i > 0 {
            sb.WriteString(" → ")
        }
        sb.WriteString(a.ToolName)
    }
    return sb.String()
}
