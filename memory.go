package main

import (
        "fmt"
        "log"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "sync"
        "time"

        "github.com/google/uuid"
        "github.com/toon-format/toon-go"
)

// MemoryCategory 记忆分类
type MemoryCategory string

const (
        MemoryCategoryPreference MemoryCategory = "preference" // 用户偏好
        MemoryCategoryFact       MemoryCategory = "fact"       // 事实信息
        MemoryCategoryProject    MemoryCategory = "project"    // 项目相关
        MemoryCategorySkill      MemoryCategory = "skill"      // 技能/能力
        MemoryCategoryContext    MemoryCategory = "context"    // 上下文
)

// MemoryScope 记忆范围
type MemoryScope string

const (
        MemoryScopeUser   MemoryScope = "user"   // 用户级（默认）
        MemoryScopeGlobal MemoryScope = "global" // 全局
)

// Memory 一条记忆（内存中使用）
type Memory struct {
        ID          string
        Key         string
        Value       string
        Category    MemoryCategory
        Scope       MemoryScope
        Tags        []string
        CreatedAt   time.Time
        UpdatedAt   time.Time
        AccessCount int
}

// MemoryEntry TOON 兼容的记忆条目（用于序列化）
// 所有字段都使用 TOML 原生类型
type MemoryEntry struct {
        ID          string   `toon:"id"`
        Key         string   `toon:"key"`
        Value       string   `toon:"value"`
        Category    string   `toon:"category"`
        Scope       string   `toon:"scope"`
        Tags        []string `toon:"tags,omitempty"`
        CreatedAt   string   `toon:"created_at"`
        UpdatedAt   string   `toon:"updated_at"`
        AccessCount int      `toon:"access_count"`
}

// ToEntry 将 Memory 转换为 TOON 兼容的 MemoryEntry
func (m *Memory) ToEntry() MemoryEntry {
        return MemoryEntry{
                ID:          m.ID,
                Key:         m.Key,
                Value:       m.Value,
                Category:    string(m.Category),
                Scope:       string(m.Scope),
                Tags:        m.Tags,
                CreatedAt:   m.CreatedAt.Format(time.RFC3339),
                UpdatedAt:   m.UpdatedAt.Format(time.RFC3339),
                AccessCount: m.AccessCount,
        }
}

// ToMemory 将 MemoryEntry 转换回 Memory
func (e *MemoryEntry) ToMemory() Memory {
        m := Memory{
                ID:          e.ID,
                Key:         e.Key,
                Value:       e.Value,
                Category:    MemoryCategory(e.Category),
                Scope:       MemoryScope(e.Scope),
                Tags:        e.Tags,
                AccessCount: e.AccessCount,
        }
        if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
                m.CreatedAt = t
        }
        if t, err := time.Parse(time.RFC3339, e.UpdatedAt); err == nil {
                m.UpdatedAt = t
        }
        return m
}

// MemoryFile TOON 文件结构
type MemoryFile struct {
        Memories []MemoryEntry `toon:"memories"`
}

// MemoryManager 记忆管理器
type MemoryManager struct {
        memories map[string]*Memory // key -> Memory
        filePath string
        mu       sync.RWMutex
}

// globalMemoryManager 全局记忆管理器实例
var globalMemoryManager *MemoryManager

// NewMemoryManager 创建记忆管理器
func NewMemoryManager(filePath string) (*MemoryManager, error) {
        mm := &MemoryManager{
                memories: make(map[string]*Memory),
                filePath: filePath,
        }

        // 确保目录存在
        dir := filepath.Dir(filePath)
        if err := os.MkdirAll(dir, 0755); err != nil {
                return nil, fmt.Errorf("failed to create memory directory: %w", err)
        }

        // 加载已有记忆
        if err := mm.load(); err != nil && !os.IsNotExist(err) {
                log.Printf("Warning: failed to load memories: %v", err)
        }

        return mm, nil
}

// load 从文件加载记忆
func (mm *MemoryManager) load() error {
        data, err := os.ReadFile(mm.filePath)
        if err != nil {
                return err
        }

        // TOON 格式解析
        var mf MemoryFile
        if err := toon.Unmarshal(data, &mf); err != nil {
                return fmt.Errorf("failed to parse memory file: %w", err)
        }

        // 转换为内存格式
        for _, entry := range mf.Memories {
                memory := entry.ToMemory()
                if memory.Key != "" {
                        mm.memories[memory.Key] = &memory
                }
        }

        log.Printf("Loaded %d memories from %s", len(mm.memories), mm.filePath)
        return nil
}

// save 保存记忆到文件
// 注意：此方法假设调用者已持有锁（或不需要锁），不会再获取锁
func (mm *MemoryManager) save() error {
        // 转换为 TOON 兼容格式
        entries := make([]MemoryEntry, 0, len(mm.memories))
        for _, m := range mm.memories {
                entries = append(entries, m.ToEntry())
        }

        // 按更新时间排序
        sort.Slice(entries, func(i, j int) bool {
                t1, _ := time.Parse(time.RFC3339, entries[i].UpdatedAt)
                t2, _ := time.Parse(time.RFC3339, entries[j].UpdatedAt)
                return t1.After(t2)
        })

        mf := MemoryFile{Memories: entries}
        // 使用 TOON 序列化
        data, err := toon.Marshal(mf)
        if err != nil {
                return fmt.Errorf("failed to marshal memories: %w", err)
        }

        // 写入临时文件再重命名，保证原子性
        tmp := mm.filePath + ".tmp"
        if err := os.WriteFile(tmp, data, 0644); err != nil {
                return err
        }
        return os.Rename(tmp, mm.filePath)
}

// saveWithLock 保存记忆到文件（先获取读锁）
func (mm *MemoryManager) saveWithLock() error {
        mm.mu.RLock()
        defer mm.mu.RUnlock()
        return mm.save()
}

// Save 保存一条记忆
func (mm *MemoryManager) Save(key, value string, category MemoryCategory, scope MemoryScope, tags []string) (*Memory, error) {
        if key == "" {
                return nil, fmt.Errorf("memory key cannot be empty")
        }
        if value == "" {
                return nil, fmt.Errorf("memory value cannot be empty")
        }

        // 默认值
        if category == "" {
                category = MemoryCategoryFact
        }
        if scope == "" {
                scope = MemoryScopeUser
        }

        mm.mu.Lock()
        defer mm.mu.Unlock()

        now := time.Now()
        var mem *Memory

        // 检查是否已存在
        if existing, ok := mm.memories[key]; ok {
                // 更新现有记忆
                existing.Value = value
                existing.Category = category
                if tags != nil {
                        existing.Tags = tags
                }
                existing.UpdatedAt = now
                existing.AccessCount++
                mem = existing
        } else {
                // 创建新记忆
                mem = &Memory{
                        ID:        uuid.New().String()[:8],
                        Key:       key,
                        Value:     value,
                        Category:  category,
                        Scope:     scope,
                        Tags:      tags,
                        CreatedAt: now,
                        UpdatedAt: now,
                        AccessCount: 1,
                }
                mm.memories[key] = mem
        }

        // 持久化
        if err := mm.save(); err != nil {
                log.Printf("Warning: failed to save memory to disk: %v", err)
        }

        return mem, nil
}

// Recall 检索记忆
func (mm *MemoryManager) Recall(query string, category MemoryCategory, limit int) []*Memory {
        mm.mu.RLock()
        defer mm.mu.RUnlock()

        if limit <= 0 {
                limit = 10
        }

        var results []*Memory
        queryLower := strings.ToLower(query)

        for _, mem := range mm.memories {
                // 分类过滤
                if category != "" && mem.Category != category {
                        continue
                }

                // 查询匹配
                if query != "" {
                        keyMatch := strings.Contains(strings.ToLower(mem.Key), queryLower)
                        valueMatch := strings.Contains(strings.ToLower(mem.Value), queryLower)
                        tagMatch := false
                        for _, tag := range mem.Tags {
                                if strings.Contains(strings.ToLower(tag), queryLower) {
                                        tagMatch = true
                                        break
                                }
                        }
                        if !keyMatch && !valueMatch && !tagMatch {
                                continue
                        }
                }

                results = append(results, mem)
        }

        // 按访问次数和更新时间排序
        sort.Slice(results, func(i, j int) bool {
                if results[i].AccessCount != results[j].AccessCount {
                        return results[i].AccessCount > results[j].AccessCount
                }
                return results[i].UpdatedAt.After(results[j].UpdatedAt)
        })

        // 限制返回数量
        if len(results) > limit {
                results = results[:limit]
        }

        // 更新访问计数
        for _, mem := range results {
                mem.AccessCount++
        }

        return results
}

// Get 按键名精确获取记忆
func (mm *MemoryManager) Get(key string) (*Memory, bool) {
        mm.mu.RLock()
        defer mm.mu.RUnlock()

        mem, ok := mm.memories[key]
        if ok {
                mem.AccessCount++
        }
        return mem, ok
}

// Forget 删除记忆
func (mm *MemoryManager) Forget(key string) error {
        mm.mu.Lock()
        defer mm.mu.Unlock()

        if _, ok := mm.memories[key]; !ok {
                return fmt.Errorf("memory '%s' not found", key)
        }

        delete(mm.memories, key)

        if err := mm.save(); err != nil {
                log.Printf("Warning: failed to save memory to disk: %v", err)
        }

        return nil
}

// List 列出所有记忆
func (mm *MemoryManager) List(category MemoryCategory, scope MemoryScope) []*Memory {
        mm.mu.RLock()
        defer mm.mu.RUnlock()

        var results []*Memory
        for _, mem := range mm.memories {
                if category != "" && mem.Category != category {
                        continue
                }
                if scope != "" && mem.Scope != scope {
                        continue
                }
                results = append(results, mem)
        }

        // 按更新时间排序
        sort.Slice(results, func(i, j int) bool {
                return results[i].UpdatedAt.After(results[j].UpdatedAt)
        })

        return results
}

// Summarize 生成记忆摘要（用于注入系统提示）
func (mm *MemoryManager) Summarize(category MemoryCategory, recentDays int) string {
        mm.mu.RLock()
        defer mm.mu.RUnlock()

        var sb strings.Builder
        sb.WriteString("# 关于用户的记忆\n\n")

        // 按分类组织
        categories := map[MemoryCategory][]*Memory{
                MemoryCategoryPreference: {},
                MemoryCategoryFact:       {},
                MemoryCategoryProject:    {},
                MemoryCategorySkill:      {},
                MemoryCategoryContext:    {},
        }

        now := time.Now()
        cutoff := now.AddDate(0, 0, -recentDays)

        for _, mem := range mm.memories {
                if category != "" && mem.Category != category {
                        continue
                }
                if recentDays > 0 && mem.UpdatedAt.Before(cutoff) {
                        continue
                }
                if list, ok := categories[mem.Category]; ok {
                        categories[mem.Category] = append(list, mem)
                }
        }

        // 输出各分类
        if len(categories[MemoryCategoryFact]) > 0 {
                sb.WriteString("## 基本信息\n")
                for _, mem := range categories[MemoryCategoryFact] {
                        sb.WriteString(fmt.Sprintf("- %s: %s\n", mem.Key, mem.Value))
                }
                sb.WriteString("\n")
        }

        if len(categories[MemoryCategoryPreference]) > 0 {
                sb.WriteString("## 用户偏好\n")
                for _, mem := range categories[MemoryCategoryPreference] {
                        sb.WriteString(fmt.Sprintf("- %s: %s\n", mem.Key, mem.Value))
                }
                sb.WriteString("\n")
        }

        if len(categories[MemoryCategoryProject]) > 0 {
                sb.WriteString("## 当前项目\n")
                for _, mem := range categories[MemoryCategoryProject] {
                        sb.WriteString(fmt.Sprintf("- %s: %s\n", mem.Key, mem.Value))
                }
                sb.WriteString("\n")
        }

        if len(categories[MemoryCategorySkill]) > 0 {
                sb.WriteString("## 技能/能力\n")
                for _, mem := range categories[MemoryCategorySkill] {
                        sb.WriteString(fmt.Sprintf("- %s: %s\n", mem.Key, mem.Value))
                }
                sb.WriteString("\n")
        }

        if len(categories[MemoryCategoryContext]) > 0 {
                sb.WriteString("## 上下文信息\n")
                for _, mem := range categories[MemoryCategoryContext] {
                        sb.WriteString(fmt.Sprintf("- %s: %s\n", mem.Key, mem.Value))
                }
                sb.WriteString("\n")
        }

        result := sb.String()
        if result == "# 关于用户的记忆\n\n" {
                return ""
        }
        return result
}

// GetContextForPrompt 获取用于注入到系统提示的记忆上下文
func (mm *MemoryManager) GetContextForPrompt() string {
        return mm.Summarize("", 30) // 最近30天的记忆
}

// Count 返回记忆总数
func (mm *MemoryManager) Count() int {
        mm.mu.RLock()
        defer mm.mu.RUnlock()
        return len(mm.memories)
}
