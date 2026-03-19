package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// EmailPoller 轮询邮箱并处理新邮件
type EmailPoller struct {
	config *EmailConfig
	stop   chan struct{}
}

// Start 启动轮询
func (p *EmailPoller) Start() {
	ticker := time.NewTicker(time.Duration(p.config.PollInterval) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				p.poll()
			case <-p.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop 停止轮询
func (p *EmailPoller) Stop() {
	close(p.stop)
}

// poll 执行一次邮件检查
func (p *EmailPoller) poll() {
	log.Println("Checking for new emails...")

	// 连接 IMAP 服务器
	var c *client.Client
	var err error
	if p.config.IMAPUseTLS {
		c, err = client.DialTLS(fmt.Sprintf("%s:%d", p.config.IMAPServer, p.config.IMAPPort), &tls.Config{ServerName: p.config.IMAPServer})
	} else {
		c, err = client.Dial(fmt.Sprintf("%s:%d", p.config.IMAPServer, p.config.IMAPPort))
	}
	if err != nil {
		log.Printf("IMAP connection error: %v", err)
		return
	}
	defer c.Logout()

	// 登录
	if err := c.Login(p.config.IMAPUser, p.config.IMAPPassword); err != nil {
		log.Printf("IMAP login error: %v", err)
		return
	}

	// 选择收件箱
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		log.Printf("IMAP select error: %v", err)
		return
	}
	if mbox.Messages == 0 {
		return
	}

	// 搜索未读邮件
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.UidSearch(criteria)
	if err != nil {
		log.Printf("IMAP search error: %v", err)
		return
	}
	if len(uids) == 0 {
		return
	}

	// 获取邮件内容
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		// 请求信封和正文
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchBody}, messages)
	}()

	for msg := range messages {
		// 处理每封邮件
		go p.handleEmail(msg)
	}

	if err := <-done; err != nil {
		log.Printf("IMAP fetch error: %v", err)
	}
}

// handleEmail 处理单封邮件
func (p *EmailPoller) handleEmail(msg *imap.Message) {
	// 提取发件人、主题、正文
	var from, to, subject string
	if msg.Envelope != nil && len(msg.Envelope.From) > 0 {
		from = msg.Envelope.From[0].Address()
	}
	if msg.Envelope != nil && len(msg.Envelope.To) > 0 {
		to = msg.Envelope.To[0].Address()
	}
	if msg.Envelope != nil {
		subject = msg.Envelope.Subject
	}

	// 遍历 msg.Body 获取第一个非空正文（无需关心 key 类型）
	var body string
	for _, literal := range msg.Body {
		if literal != nil {
			b, err := io.ReadAll(literal)
			if err == nil {
				body = string(b)
				break
			}
		}
	}
	if body == "" {
		log.Printf("Empty body for email from %s", from)
		return
	}

	// 创建邮件频道
	ch := NewEmailChannel(from, to, subject, p.config)

	// 构建历史
	history := []Message{
		{Role: "user", Content: body},
	}

	// 启动 AgentLoop
	_, err := AgentLoop(ch, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
	if err != nil {
		log.Printf("AgentLoop error for email from %s: %v", from, err)
	}
}