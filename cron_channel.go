package main

import (
	"log"
)

// LogChannel 将输出写入日志
type LogChannel struct {
	*BaseChannel
}

func NewLogChannel() *LogChannel {
	return &LogChannel{BaseChannel: NewBaseChannel("log")}
}

func (lc *LogChannel) WriteChunk(chunk StreamChunk) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	// 应用流式字符串替换
	processed := lc.ProcessChunkWithReplacement(chunk)

	if processed.Error != "" {
		log.Printf("[Cron Log] Error: %s", processed.Error)
	} else if processed.Content != "" {
		log.Printf("[Cron Log] %s", processed.Content)
	} else if processed.ReasoningContent != "" {
		log.Printf("[Cron Log] Reasoning: %s", processed.ReasoningContent)
	} else if processed.Done {
		log.Printf("[Cron Log] Task completed.")
	}
	return nil
}

// CompositeChannel 将输出同时发送到多个子 Channel
// 注意：子 Channel 会各自处理字符串替换，所以 CompositeChannel 不再重复处理
type CompositeChannel struct {
	*BaseChannel
	channels []Channel
}

func NewCompositeChannel(channels ...Channel) *CompositeChannel {
	return &CompositeChannel{
		BaseChannel: NewBaseChannel("composite"),
		channels:    channels,
	}
}

func (cc *CompositeChannel) WriteChunk(chunk StreamChunk) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	for _, ch := range cc.channels {
		if err := ch.WriteChunk(chunk); err != nil {
			log.Printf("CompositeChannel: sub channel write error: %v", err)
		}
	}
	return nil
}

func (cc *CompositeChannel) Close() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	for _, ch := range cc.channels {
		ch.Close()
	}
	return nil
}
