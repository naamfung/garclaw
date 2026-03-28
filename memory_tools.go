package main

import (
        "context"
        "fmt"
        "strings"

        "github.com/toon-format/toon-go"
)

// handleMemorySave 保存记忆
func handleMemorySave(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查记忆管理器是否已初始化
        if globalMemoryManager == nil {
                return "Error: memory manager not initialized", false
        }

        key, ok := argsMap["key"].(string)
        if !ok || key == "" {
                return "Error: missing or invalid 'key' parameter. Example: memory_save(key=\"user_name\", value=\"张三\")", false
        }
        value, ok := argsMap["value"].(string)
        if !ok || value == "" {
                return "Error: missing or invalid 'value' parameter.", false
        }

        // 解析分类
        category := MemoryCategoryFact
        if cat, ok := argsMap["category"].(string); ok && cat != "" {
                category = MemoryCategory(cat)
                // 验证分类有效性
                validCategories := map[MemoryCategory]bool{
                        MemoryCategoryPreference: true,
                        MemoryCategoryFact:       true,
                        MemoryCategoryProject:    true,
                        MemoryCategorySkill:      true,
                        MemoryCategoryContext:    true,
                }
                if !validCategories[category] {
                        return fmt.Sprintf("Error: invalid category '%s'. Valid options: preference, fact, project, skill, context", cat), false
                }
        }

        // 解析范围
        scope := MemoryScopeUser
        if s, ok := argsMap["scope"].(string); ok && s != "" {
                scope = MemoryScope(s)
                if scope != MemoryScopeUser && scope != MemoryScopeGlobal {
                        return fmt.Sprintf("Error: invalid scope '%s'. Valid options: user, global", s), false
                }
        }

        // 解析标签
        var tags []string
        if tagsRaw, ok := argsMap["tags"]; ok {
                switch v := tagsRaw.(type) {
                case []interface{}:
                        for _, t := range v {
                                if s, ok := t.(string); ok {
                                        tags = append(tags, s)
                                }
                        }
                case string:
                        // 支持 TOON 字符串或逗号分隔
                        if strings.HasPrefix(v, "[") {
                                var parsed []string
                                if err := toon.Unmarshal([]byte(v), &parsed); err == nil {
                                        tags = parsed
                                }
                        } else if v != "" {
                                tags = strings.Split(v, ",")
                                for i, t := range tags {
                                        tags[i] = strings.TrimSpace(t)
                                }
                        }
                }
        }

        mem, err := globalMemoryManager.Save(key, value, category, scope, tags)
        if err != nil {
                return fmt.Sprintf("Error saving memory: %v", err), false
        }

        return fmt.Sprintf("Memory saved: [%s] %s = %s", mem.Category, mem.Key, mem.Value), false
}

// handleMemoryRecall 检索记忆
func handleMemoryRecall(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查记忆管理器是否已初始化
        if globalMemoryManager == nil {
                return "Error: memory manager not initialized", false
        }

        query, _ := argsMap["query"].(string)
        category := MemoryCategory("")
        if cat, ok := argsMap["category"].(string); ok && cat != "" {
                category = MemoryCategory(cat)
        }

        limit := 10
        if l, ok := argsMap["limit"].(float64); ok && l > 0 {
                limit = int(l)
        }

        memories := globalMemoryManager.Recall(query, category, limit)
        if len(memories) == 0 {
                if query != "" {
                        return fmt.Sprintf("No memories found matching '%s'.", query), false
                }
                return "No memories stored yet.", false
        }

        var sb strings.Builder
        sb.WriteString(fmt.Sprintf("Found %d memory(ies):\n", len(memories)))
        for _, mem := range memories {
                sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", mem.Category, mem.Key, mem.Value))
                if len(mem.Tags) > 0 {
                        sb.WriteString(fmt.Sprintf("  Tags: %s\n", strings.Join(mem.Tags, ", ")))
                }
        }

        return sb.String(), false
}

// handleMemoryGet 按键名精确获取记忆
func handleMemoryGet(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查记忆管理器是否已初始化
        if globalMemoryManager == nil {
                return "Error: memory manager not initialized", false
        }

        key, ok := argsMap["key"].(string)
        if !ok || key == "" {
                return "Error: missing or invalid 'key' parameter. Example: memory_get(key=\"user_name\")", false
        }

        mem, found := globalMemoryManager.Get(key)
        if !found {
                return fmt.Sprintf("Memory '%s' not found.", key), false
        }

        data, err := toon.Marshal(mem)
        if err != nil {
                return fmt.Sprintf("Error formatting memory: %v", err), false
        }
        return string(data), false
}

// handleMemoryForget 删除记忆
func handleMemoryForget(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查记忆管理器是否已初始化
        if globalMemoryManager == nil {
                return "Error: memory manager not initialized", false
        }

        key, ok := argsMap["key"].(string)
        if !ok || key == "" {
                return "Error: missing or invalid 'key' parameter. Example: memory_forget(key=\"old_preference\")", false
        }

        if err := globalMemoryManager.Forget(key); err != nil {
                return fmt.Sprintf("Error: %v", err), false
        }

        return fmt.Sprintf("Memory '%s' has been forgotten.", key), false
}

// handleMemoryList 列出记忆
func handleMemoryList(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查记忆管理器是否已初始化
        if globalMemoryManager == nil {
                return "Error: memory manager not initialized", false
        }

        category := MemoryCategory("")
        if cat, ok := argsMap["category"].(string); ok && cat != "" {
                category = MemoryCategory(cat)
        }

        scope := MemoryScope("")
        if s, ok := argsMap["scope"].(string); ok && s != "" {
                scope = MemoryScope(s)
        }

        memories := globalMemoryManager.List(category, scope)
        if len(memories) == 0 {
                return "No memories found.", false
        }

        var sb strings.Builder
        sb.WriteString(fmt.Sprintf("Total %d memory(ies):\n\n", len(memories)))

        // 按分类分组输出
        categories := map[MemoryCategory][]*Memory{}
        for _, mem := range memories {
                categories[mem.Category] = append(categories[mem.Category], mem)
        }

        categoryOrder := []MemoryCategory{
                MemoryCategoryFact,
                MemoryCategoryPreference,
                MemoryCategoryProject,
                MemoryCategorySkill,
                MemoryCategoryContext,
        }

        for _, cat := range categoryOrder {
                if mems, ok := categories[cat]; ok && len(mems) > 0 {
                        sb.WriteString(fmt.Sprintf("## %s\n", cat))
                        for _, mem := range mems {
                                sb.WriteString(fmt.Sprintf("- %s: %s\n", mem.Key, mem.Value))
                        }
                        sb.WriteString("\n")
                }
        }

        return sb.String(), false
}

// handleMemorySummarize 生成记忆摘要
func handleMemorySummarize(ctx context.Context, argsMap map[string]interface{}, ch Channel) (string, bool) {
        // 检查记忆管理器是否已初始化
        if globalMemoryManager == nil {
                return "Error: memory manager not initialized", false
        }

        category := MemoryCategory("")
        if cat, ok := argsMap["category"].(string); ok && cat != "" {
                category = MemoryCategory(cat)
        }

        recentDays := 7
        if d, ok := argsMap["recent_days"].(float64); ok && d > 0 {
                recentDays = int(d)
        }

        summary := globalMemoryManager.Summarize(category, recentDays)
        if summary == "" {
                return "No memories to summarize.", false
        }

        return summary, false
}
