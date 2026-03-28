package main

import (
        "strings"
        "sync"
)

// Channel 是所有前端频道的统一接口
type Channel interface {
        // WriteChunk 向客户端发送一个流式数据块
        WriteChunk(chunk StreamChunk) error
        // ID 返回频道的唯一标识
        ID() string
        // Close 关闭频道，释放资源
        Close() error
        // GetSessionID 返回关联的会话ID（可选，如果没有返回空字符串）
        GetSessionID() string
}

// HiddenSummaryFilter 流式隐式总结过滤器
// 用于在流式输出过程中实时过滤 <隐式总结>...<隐式总结/> 标签
type HiddenSummaryFilter struct {
        buffer      strings.Builder // 累积缓冲区
        inSummary   bool            // 是否正在过滤隐式总结
        pending     strings.Builder // 待处理的可能匹配前缀
}

// NewHiddenSummaryFilter 创建隐式总结过滤器
func NewHiddenSummaryFilter() *HiddenSummaryFilter {
        return &HiddenSummaryFilter{}
}

// Filter 过滤输入内容，返回应该发送给用户的部分
func (f *HiddenSummaryFilter) Filter(input string) string {
        var output strings.Builder
        data := f.pending.String() + input
        f.pending.Reset()
        
        i := 0
        for i < len(data) {
                if f.inSummary {
                        // 正在过滤，寻找结束标签
                        endIdx := strings.Index(data[i:], SummaryEndTag)
                        if endIdx == -1 {
                                // 没有找到结束标签，缓存等待更多数据
                                f.buffer.WriteString(data[i:])
                                break
                        }
                        // 找到结束标签，跳过整个隐式总结
                        f.inSummary = false
                        i += endIdx + len(SummaryEndTag)
                        f.buffer.Reset()
                } else {
                        // 寻找开始标签
                        startIdx := strings.Index(data[i:], SummaryStartTag)
                        if startIdx == -1 {
                                // 没有找到开始标签
                                // 需要检查末尾是否有部分匹配
                                safeLen := len(data) - i - len(SummaryStartTag) + 1
                                if safeLen < 0 {
                                        safeLen = 0
                                }
                                // 检查末尾是否可能是开始标签的前缀
                                for j := max(0, len(data)-len(SummaryStartTag)); j < len(data); j++ {
                                        if strings.HasPrefix(SummaryStartTag, data[j:]) {
                                                f.pending.WriteString(data[j:])
                                                output.WriteString(data[i:j])
                                                i = len(data)
                                                break
                                        }
                                }
                                if f.pending.Len() == 0 {
                                        output.WriteString(data[i:])
                                        i = len(data)
                                }
                        } else {
                                // 找到开始标签
                                output.WriteString(data[i : i+startIdx])
                                i += startIdx + len(SummaryStartTag)
                                f.inSummary = true
                        }
                }
        }
        
        return output.String()
}

// Flush 刷新过滤器，返回任何剩余的内容
func (f *HiddenSummaryFilter) Flush() string {
        var result strings.Builder
        if f.inSummary {
                // 正在过滤但没有找到结束标签，丢弃缓存
                // (隐式总结不完整，可能是模型输出问题)
                f.buffer.Reset()
        } else {
                result.WriteString(f.pending.String())
        }
        f.pending.Reset()
        f.inSummary = false
        return result.String()
}

// Reset 重置过滤器状态
func (f *HiddenSummaryFilter) Reset() {
        f.buffer.Reset()
        f.pending.Reset()
        f.inSummary = false
}

// BaseChannel 提供基础实现，包含流式替换器
type BaseChannel struct {
        id                string
        mu                sync.Mutex // 用于 WriteChunk 的并发控制
        contentReplacer   *StreamReplacer
        reasoningReplacer *StreamReplacer
        contentBuffer     *strings.Builder
        reasoningBuffer   *strings.Builder
        summaryFilter     *HiddenSummaryFilter // 隐式总结过滤器
}

// NewBaseChannel 创建带有流式替换器的基础频道
func NewBaseChannel(id string) *BaseChannel {
        bc := &BaseChannel{
                id:              id,
                contentBuffer:   &strings.Builder{},
                reasoningBuffer: &strings.Builder{},
                summaryFilter:   NewHiddenSummaryFilter(),
        }
        // 创建 Content 的流式替换器
        bc.contentReplacer = NewStreamReplacer(func(r rune) {
                bc.contentBuffer.WriteRune(r)
        })
        // 创建 ReasoningContent 的流式替换器
        bc.reasoningReplacer = NewStreamReplacer(func(r rune) {
                bc.reasoningBuffer.WriteRune(r)
        })
        return bc
}

func max(a, b int) int {
        if a > b {
                return a
        }
        return b
}

func (bc *BaseChannel) ID() string { return bc.id }

func (bc *BaseChannel) Close() error { return nil }

// GetSessionID 默认实现，返回空字符串
func (bc *BaseChannel) GetSessionID() string { return "" }

// ProcessChunkWithReplacement 对 chunk 应用流式字符串替换（最长匹配）
// 并过滤隐式总结标签（对用户不可见）
// 返回处理后的新 chunk，不会修改原始 chunk
func (bc *BaseChannel) ProcessChunkWithReplacement(chunk StreamChunk) StreamChunk {
        result := StreamChunk{
                Done:             chunk.Done,
                Error:            chunk.Error,
                FinishReason:     chunk.FinishReason,
                ToolCalls:        chunk.ToolCalls,
                SessionID:        chunk.SessionID,       // 保留会话 ID
                TaskRunning:      chunk.TaskRunning,     // 保留任务状态
        }

        // 处理 Content
        if chunk.Content != "" {
                bc.contentReplacer.Write(chunk.Content)
                content := bc.contentBuffer.String()
                bc.contentBuffer.Reset()
                // 流式过滤隐式总结
                result.Content = bc.summaryFilter.Filter(content)
        }

        // 处理 ReasoningContent
        if chunk.ReasoningContent != "" {
                bc.reasoningReplacer.Write(chunk.ReasoningContent)
                result.ReasoningContent = bc.reasoningBuffer.String()
                bc.reasoningBuffer.Reset()
        }

        // 如果结束，刷新缓冲区
        if chunk.Done {
                bc.contentReplacer.Flush()
                if bc.contentBuffer.Len() > 0 {
                        content := bc.contentBuffer.String()
                        bc.contentBuffer.Reset()
                        // 过滤隐式总结
                        result.Content += bc.summaryFilter.Filter(content)
                }
                // 刷新过滤器
                remaining := bc.summaryFilter.Flush()
                if remaining != "" {
                        result.Content += remaining
                }
                
                bc.reasoningReplacer.Flush()
                if bc.reasoningBuffer.Len() > 0 {
                        result.ReasoningContent += bc.reasoningBuffer.String()
                        bc.reasoningBuffer.Reset()
                }
        }

        return result
}

// ResetReplacers 重置替换器状态（用于新会话）
func (bc *BaseChannel) ResetReplacers() {
        bc.contentBuffer.Reset()
        bc.reasoningBuffer.Reset()
        // 重置隐式总结过滤器
        bc.summaryFilter.Reset()
        // 重新创建替换器以清除缓冲区
        bc.contentReplacer = NewStreamReplacer(func(r rune) {
                bc.contentBuffer.WriteRune(r)
        })
        bc.reasoningReplacer = NewStreamReplacer(func(r rune) {
                bc.reasoningBuffer.WriteRune(r)
        })
}
