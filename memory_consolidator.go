package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "strings"
    "sync"
    "time"
)

// ============================================================
// 记忆整合器
// 负责 token 预算控制和自动整合触发
// ============================================================

// MemoryConsolidatorConfig 整合器配置
type MemoryConsolidatorConfig struct {
    ContextWindowTokens int     `json:"context_window_tokens"` // 上下文窗口大小
    MaxCompletionTokens int     `json:"max_completion_tokens"` // 最大补全 tokens
    SafetyBuffer        int     `json:"safety_buffer"`         // 安全缓冲
    ConsolidationRatio  float64 `json:"consolidation_ratio"`   // 整合比例（当达到预算的多少时触发整合）
    MaxConsolidationRound int   `json:"max_consolidation_round"` // 最大整合轮数
    MinMessagesToConsolidate int `json:"min_messages_to_consolidate"` // 最小整合消息数
}

// DefaultMemoryConsolidatorConfig 默认配置
func DefaultMemoryConsolidatorConfig() MemoryConsolidatorConfig {
    return MemoryConsolidatorConfig{
        ContextWindowTokens:     128000,  // 默认 128k 上下文
        MaxCompletionTokens:     4096,    // 最大补全
        SafetyBuffer:            1024,    // 安全缓冲
        ConsolidationRatio:      0.01,    // 1% 时触发整合（约1200 tokens，一问一答后即可触发）
        MaxConsolidationRound:   5,       // 最大 5 轮整合
        MinMessagesToConsolidate: 2,      // 最少 2 条消息（一问一答）就整合
    }
}

// MemoryConsolidator 记忆整合器
type MemoryConsolidator struct {
    config     MemoryConsolidatorConfig
    memory     *TwoLayerMemorySystem
    mu         sync.RWMutex

    // 会话消息缓冲
    sessionMessages map[string][]ConsolidationMessage
    sessionOffset   map[string]int // 每个会话已整合的消息偏移

    // 整合锁（防止并发整合）
    consolidationLocks map[string]*sync.Mutex
}

// ConsolidationMessage 待整合消息
type ConsolidationMessage struct {
    Role      string                 `json:"role"`
    Content   string                 `json:"content"`
    Summary   string                 `json:"summary,omitempty"`  // 精华总结（从响应中提取）
    Timestamp time.Time              `json:"timestamp"`
    ToolsUsed []string               `json:"tools_used,omitempty"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ConsolidationResult 整合结果
type ConsolidationResult struct {
    HistoryEntry string `json:"history_entry"` // 写入 HISTORY.md 的条目
    MemoryUpdate string `json:"memory_update"` // 更新后的 MEMORY.md 内容
}

// NewMemoryConsolidator 创建记忆整合器
func NewMemoryConsolidator(config MemoryConsolidatorConfig, memory *TwoLayerMemorySystem) *MemoryConsolidator {
    return &MemoryConsolidator{
        config:             config,
        memory:             memory,
        sessionMessages:    make(map[string][]ConsolidationMessage),
        sessionOffset:      make(map[string]int),
        consolidationLocks: make(map[string]*sync.Mutex),
    }
}

// ============================================================
// 消息管理
// ============================================================

// AddMessage 添加消息到缓冲
func (mc *MemoryConsolidator) AddMessage(sessionKey string, msg ConsolidationMessage) {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    mc.sessionMessages[sessionKey] = append(mc.sessionMessages[sessionKey], msg)

    // 确保有整合锁
    if _, ok := mc.consolidationLocks[sessionKey]; !ok {
        mc.consolidationLocks[sessionKey] = &sync.Mutex{}
    }
}

// GetMessages 获取会话消息
func (mc *MemoryConsolidator) GetMessages(sessionKey string) []ConsolidationMessage {
    mc.mu.RLock()
    defer mc.mu.RUnlock()

    return mc.sessionMessages[sessionKey]
}

// GetMessageCount 获取消息数量
func (mc *MemoryConsolidator) GetMessageCount(sessionKey string) int {
    mc.mu.RLock()
    defer mc.mu.RUnlock()

    return len(mc.sessionMessages[sessionKey])
}

// ============================================================
// Token 估算
// ============================================================

// 隐式总结标记
const (
    SummaryStartTag = "<隐式总结>"
    SummaryEndTag   = "<隐式总结/>"
)

// ExtractSummary 从消息内容中提取隐式总结
// 如果存在 <隐式总结>...<隐式总结/> 标记，提取其内容作为总结
// 同时返回去除总结标记的原始内容（用于显示给用户）
func ExtractSummary(content string) (cleanContent string, summary string) {
    startIdx := strings.Index(content, SummaryStartTag)
    if startIdx == -1 {
        return content, ""
    }
    
    endIdx := strings.Index(content, SummaryEndTag)
    if endIdx == -1 || endIdx <= startIdx {
        // 没有结束标记或位置错误，尝试提取到末尾
        summary = strings.TrimSpace(content[startIdx+len(SummaryStartTag):])
        cleanContent = strings.TrimSpace(content[:startIdx])
        return cleanContent, summary
    }
    
    // 提取总结内容
    summary = strings.TrimSpace(content[startIdx+len(SummaryStartTag):endIdx])
    // 清理原始内容（去除总结部分）
    cleanContent = strings.TrimSpace(content[:startIdx] + content[endIdx+len(SummaryEndTag):])
    // 清理可能的多余空白
    cleanContent = strings.TrimSpace(cleanContent)
    
    return cleanContent, summary
}

// FilterHiddenSummary 过滤隐式总结标签，返回清理后的内容
// 此函数用于在发送给前端之前移除隐式总结
func FilterHiddenSummary(content string) string {
    cleanContent, _ := ExtractSummary(content)
    return cleanContent
}

// EstimateTokens 估算文本的 token 数（简化估算：约 4 字符 = 1 token）
func EstimateTokens(text string) int {
    // 简化估算：英文约 4 字符 = 1 token，中文约 2 字符 = 1 token
    // 这里使用保守估算
    charCount := len([]rune(text))
    // 假设平均 3 字符 = 1 token
    return charCount / 3
}

// EstimateMessagesTokens 估算消息列表的 token 数
// 优先使用精华总结进行估算，以获得更准确的 token 消耗预测
func (mc *MemoryConsolidator) EstimateMessagesTokens(messages []ConsolidationMessage) int {
    total := 0
    for _, msg := range messages {
        // 对于助手消息，优先使用精华总结估算
        if msg.Role == "assistant" {
            // 先检查 Summary 字段
            if msg.Summary != "" {
                total += EstimateTokens(msg.Summary)
            } else {
                // 尝试从内容中提取总结
                _, summary := ExtractSummary(msg.Content)
                if summary != "" {
                    total += EstimateTokens(summary)
                } else {
                    // 没有总结，使用完整内容
                    total += EstimateTokens(msg.Content)
                }
            }
        } else {
            // 用户消息使用完整内容
            total += EstimateTokens(msg.Content)
        }
        total += 10 // 角色、时间戳等开销
        for _, tool := range msg.ToolsUsed {
            total += EstimateTokens(tool)
        }
    }
    return total
}

// EstimatePromptTokens 估算整个提示的 token 数
func (mc *MemoryConsolidator) EstimatePromptTokens(sessionKey string) int {
    mc.mu.RLock()
    defer mc.mu.RUnlock()

    messages := mc.sessionMessages[sessionKey]
    offset := mc.sessionOffset[sessionKey]

    // 只计算未整合的消息
    if offset >= len(messages) {
        return 0
    }

    return mc.EstimateMessagesTokens(messages[offset:])
}

// ============================================================
// 整合触发检查
// ============================================================

// ShouldConsolidate 检查是否需要整合
func (mc *MemoryConsolidator) ShouldConsolidate(sessionKey string) (bool, int) {
    mc.mu.RLock()
    defer mc.mu.RUnlock()

    messages := mc.sessionMessages[sessionKey]
    offset := mc.sessionOffset[sessionKey]

    // 未整合的消息
    unconsolidatedCount := len(messages) - offset

    // 消息数太少不整合
    if unconsolidatedCount < mc.config.MinMessagesToConsolidate {
        return false, 0
    }

    // 计算 token 预算
    budget := mc.config.ContextWindowTokens - mc.config.MaxCompletionTokens - mc.config.SafetyBuffer
    threshold := int(float64(budget) * mc.config.ConsolidationRatio)

    // 估算当前提示 token 数
    estimated := mc.EstimateMessagesTokens(messages[offset:])

    if estimated >= threshold {
        return true, estimated - budget/2 // 返回需要移除的 token 数
    }

    return false, 0
}

// GetBudgetInfo 获取预算信息
func (mc *MemoryConsolidator) GetBudgetInfo(sessionKey string) map[string]interface{} {
    mc.mu.RLock()
    defer mc.mu.RUnlock()

    budget := mc.config.ContextWindowTokens - mc.config.MaxCompletionTokens - mc.config.SafetyBuffer
    threshold := int(float64(budget) * mc.config.ConsolidationRatio)

    messages := mc.sessionMessages[sessionKey]
    offset := mc.sessionOffset[sessionKey]
    estimated := 0
    if offset < len(messages) {
        estimated = mc.EstimateMessagesTokens(messages[offset:])
    }

    return map[string]interface{}{
        "budget":           budget,
        "threshold":        threshold,
        "current_tokens":   estimated,
        "usage_ratio":      float64(estimated) / float64(budget),
        "total_messages":   len(messages),
        "consolidated":     offset,
        "unconsolidated":   len(messages) - offset,
        "should_consolidate": estimated >= threshold,
    }
}

// ============================================================
// 整合执行
// ============================================================

// Consolidate 整合消息到长期记忆
func (mc *MemoryConsolidator) Consolidate(ctx context.Context, sessionKey string, messages []ConsolidationMessage) error {
    // 获取整合锁
    mc.mu.RLock()
    lock, ok := mc.consolidationLocks[sessionKey]
    mc.mu.RUnlock()

    if !ok {
        lock = &sync.Mutex{}
        mc.mu.Lock()
        mc.consolidationLocks[sessionKey] = lock
        mc.mu.Unlock()
    }

    lock.Lock()
    defer lock.Unlock()

    // 调用 LLM 进行整合
    result, err := mc.callLLMForConsolidation(ctx, messages)
    if err != nil {
        return fmt.Errorf("LLM consolidation failed: %w", err)
    }

    // 写入 HISTORY.md
    if result.HistoryEntry != "" {
        mc.memory.sessionHistory.mu.Lock()
        // 追加到历史文件
        historyFile := mc.memory.sessionHistory.filePath
        f, err := openForAppend(historyFile)
        if err == nil {
            f.WriteString(result.HistoryEntry + "\n\n")
            f.Close()
        }
        mc.memory.sessionHistory.mu.Unlock()
    }

    // 更新 MEMORY.md
    if result.MemoryUpdate != "" {
        mc.memory.longTermMemory.mu.Lock()
        // 重写 MEMORY.md
        mc.memory.longTermMemory.save()
        mc.memory.longTermMemory.mu.Unlock()
    }

    // 更新偏移
    mc.mu.Lock()
    mc.sessionOffset[sessionKey] = mc.sessionOffset[sessionKey] + len(messages)
    mc.mu.Unlock()

    log.Printf("[MemoryConsolidator] Consolidated %d messages for session %s", len(messages), sessionKey)
    return nil
}

// openForAppend 以追加模式打开文件
func openForAppend(path string) (*os.File, error) {
    return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

// callLLMForConsolidation 调用 LLM 进行整合
func (mc *MemoryConsolidator) callLLMForConsolidation(ctx context.Context, messages []ConsolidationMessage) (*ConsolidationResult, error) {
    // 构建整合提示
    currentMemory := mc.memory.longTermMemory.GetContextForPrompt()
    if currentMemory == "" {
        currentMemory = "(empty)"
    }

    // 格式化消息 - 优先使用精华总结
    var msgTexts []string
    var summaryTexts []string
    hasSummaries := false
    
    for _, msg := range messages {
        timeStr := msg.Timestamp.Format("2006-01-02 15:04")
        toolsStr := ""
        if len(msg.ToolsUsed) > 0 {
            toolsStr = fmt.Sprintf(" [tools: %s]", strings.Join(msg.ToolsUsed, ", "))
        }
        
        // 提取精华总结
        cleanContent, extractedSummary := ExtractSummary(msg.Content)
        
        // 如果消息本身有 Summary 字段，优先使用
        summary := msg.Summary
        if summary == "" && extractedSummary != "" {
            summary = extractedSummary
        }
        
        if msg.Role == "assistant" && summary != "" {
            // 助手消息有精华总结，使用总结代替完整内容
            msgTexts = append(msgTexts, fmt.Sprintf("[%s] %s%s: [总结] %s", timeStr, strings.ToUpper(msg.Role), toolsStr, summary))
            summaryTexts = append(summaryTexts, fmt.Sprintf("- %s", summary))
            hasSummaries = true
        } else {
            // 用户消息或没有总结的助手消息，保留原内容（但使用清理后的内容）
            content := cleanContent
            if content == "" {
                content = msg.Content
            }
            msgTexts = append(msgTexts, fmt.Sprintf("[%s] %s%s: %s", timeStr, strings.ToUpper(msg.Role), toolsStr, content))
        }
    }

    // 构建提示词
    var prompt string
    if hasSummaries {
        // 有精华总结时，使用更高效的整合提示
        prompt = fmt.Sprintf(`基于对话精华总结进行记忆整合。

## 当前长期记忆
%s

## 本次对话精华
%s

## 完整对话记录（供参考）
%s

请基于精华总结进行整合，输出：
1. 历史条目：总结本次对话的关键事件/决策。以 [YYYY-MM-DD HH:MM] 开头。
2. 记忆更新：完整的更新后长期记忆（markdown 格式）。整合新信息到现有记忆中。

输出格式：
HISTORY: <历史条目>
MEMORY: <记忆更新>`, currentMemory, strings.Join(summaryTexts, "\n"), strings.Join(msgTexts, "\n"))
    } else {
        // 没有精华总结，使用传统方式
        prompt = fmt.Sprintf(`处理以下对话并输出整合结果。

## 当前长期记忆
%s

## 待处理对话
%s

请分析对话，提取重要信息并按照以下格式输出：
1. 历史条目：一段总结关键事件/决策/主题的段落。以 [YYYY-MM-DD HH:MM] 开头。
2. 记忆更新：完整的更新后长期记忆（markdown 格式）。包含所有现有事实和新事实。如果没有新内容，返回原内容。

输出格式：
HISTORY: <历史条目>
MEMORY: <记忆更新>`, currentMemory, strings.Join(msgTexts, "\n"))
    }

    // 调用模型
    chatMessages := []Message{
        {Role: "system", Content: "你是一个记忆整合代理。请按照要求输出整合结果。"},
        {Role: "user", Content: prompt},
    }

    response, err := CallModelSync(ctx, chatMessages, apiType, baseURL, apiKey, modelID, temperature, maxTokens, false, false)
    if err != nil {
        return nil, fmt.Errorf("model call failed: %w", err)
    }

    // 解析响应文本
    result := &ConsolidationResult{}
    content, ok := response.Content.(string)
    if !ok {
        return result, nil
    }

    // 简单解析
    lines := strings.Split(content, "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "HISTORY:") {
            result.HistoryEntry = strings.TrimSpace(strings.TrimPrefix(line, "HISTORY:"))
        } else if strings.HasPrefix(line, "MEMORY:") {
            result.MemoryUpdate = strings.TrimSpace(strings.TrimPrefix(line, "MEMORY:"))
        }
    }

    // 如果没有提取到，回退
    if result.HistoryEntry == "" && content != "" {
        now := time.Now()
        result.HistoryEntry = fmt.Sprintf("[%s] %s", now.Format("2006-01-02 15:04"), content)
    }

    return result, nil
}

// ============================================================
// 自动整合
// ============================================================

// MaybeConsolidate 检查并在需要时执行整合
func (mc *MemoryConsolidator) MaybeConsolidate(ctx context.Context, sessionKey string) error {
    shouldConsolidate, _ := mc.ShouldConsolidate(sessionKey)
    if !shouldConsolidate {
        return nil
    }

    mc.mu.RLock()
    messages := mc.sessionMessages[sessionKey]
    offset := mc.sessionOffset[sessionKey]
    mc.mu.RUnlock()

    if offset >= len(messages) {
        return nil
    }

    // 找到合适的整合边界（用户消息边界）
    boundary := mc.findConsolidationBoundary(sessionKey, offset)
    if boundary <= offset {
        return nil
    }

    // 执行整合
    toConsolidate := messages[offset:boundary]
    return mc.Consolidate(ctx, sessionKey, toConsolidate)
}

// findConsolidationBoundary 找到整合边界
func (mc *MemoryConsolidator) findConsolidationBoundary(sessionKey string, startIdx int) int {
    mc.mu.RLock()
    defer mc.mu.RUnlock()

    messages := mc.sessionMessages[sessionKey]
    if startIdx >= len(messages) {
        return startIdx
    }

    // 找到下一个用户消息之前的位置
    for i := startIdx + 1; i < len(messages); i++ {
        if messages[i].Role == "user" {
            return i
        }
    }

    return len(messages)
}

// ============================================================
// 会话管理
// ============================================================

// ClearSession 清理会话
func (mc *MemoryConsolidator) ClearSession(sessionKey string) {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    delete(mc.sessionMessages, sessionKey)
    delete(mc.sessionOffset, sessionKey)
    delete(mc.consolidationLocks, sessionKey)
}

// ResetSessionOffset 重置会话偏移（用于重新加载会话）
func (mc *MemoryConsolidator) ResetSessionOffset(sessionKey string) {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    mc.sessionOffset[sessionKey] = 0
}

// ============================================================
// 全局记忆整合器
// ============================================================

var globalMemoryConsolidator *MemoryConsolidator

// InitMemoryConsolidator 初始化全局记忆整合器
func InitMemoryConsolidator(config MemoryConsolidatorConfig, memory *TwoLayerMemorySystem) {
    if globalMemoryConsolidator == nil {
        globalMemoryConsolidator = NewMemoryConsolidator(config, memory)
    }
}

// GetMemoryConsolidator 获取全局记忆整合器
func GetMemoryConsolidator() *MemoryConsolidator {
    return globalMemoryConsolidator
}

// ============================================================
// 整合工具定义（供 LLM 调用）
// ============================================================

// GetConsolidationTools 获取整合相关工具定义
func GetConsolidationTools() []map[string]interface{} {
    return []map[string]interface{}{
        {
            "type": "function",
            "function": map[string]interface{}{
                "name":        "save_memory",
                "description": "保存记忆整合结果到持久化存储。",
                "parameters": map[string]interface{}{
                    "type": "object",
                    "properties": map[string]interface{}{
                        "history_entry": map[string]interface{}{
                            "type":        "string",
                            "description": "一段总结关键事件/决策/主题的段落。以 [YYYY-MM-DD HH:MM] 开头。",
                        },
                        "memory_update": map[string]interface{}{
                            "type":        "string",
                            "description": "完整的更新后长期记忆（markdown 格式）。",
                        },
                    },
                    "required": []string{"history_entry", "memory_update"},
                },
            },
        },
    }
}

// HandleSaveMemoryTool 处理 save_memory 工具调用
func HandleSaveMemoryTool(args map[string]interface{}) (string, error) {
    historyEntry, _ := args["history_entry"].(string)
    memoryUpdate, _ := args["memory_update"].(string)

    if historyEntry == "" {
        return "", fmt.Errorf("history_entry is required")
    }

    // 写入历史
    if globalTwoLayerMemory != nil {
        // 追加到历史文件
        historyFile := globalTwoLayerMemory.sessionHistory.filePath
        f, err := os.OpenFile(historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
        if err == nil {
            f.WriteString(historyEntry + "\n\n")
            f.Close()
        }

        // 更新长期记忆
        if memoryUpdate != "" {
            globalTwoLayerMemory.longTermMemory.mu.Lock()
            os.WriteFile(globalTwoLayerMemory.longTermMemory.filePath, []byte(memoryUpdate), 0644)
            globalTwoLayerMemory.longTermMemory.mu.Unlock()

            // 重新加载
            globalTwoLayerMemory.longTermMemory.load()
        }
    }

    return "Memory saved successfully", nil
}
