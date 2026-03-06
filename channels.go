package main

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// InboundMessage 所有通道都规范化为此结构。Agent 循环只看到 InboundMessage。
type InboundMessage struct {
	Text      string
	SenderID  string
	Channel   string
	AccountID string
	PeerID    string
	IsGroup   bool
	Media     []map[string]interface{}
	Raw       map[string]interface{}
}

// ChannelAccount 每个通道账户的配置。同一通道类型可以运行多个账户。
type ChannelAccount struct {
	Channel   string
	AccountID string
	Token     string
	Config    map[string]interface{}
}

// Channel 通道抽象接口
type Channel interface {
	Name() string
	Receive() *InboundMessage
	Send(to string, text string, kwargs map[string]interface{}) bool
	Close()
}

// CLIChannel 命令行通道
type CLIChannel struct {
	AccountID string
}

// Name 返回通道名称
func (c *CLIChannel) Name() string {
	return "cli"
}

// Receive 接收命令行输入
func (c *CLIChannel) Receive() *InboundMessage {
	// 命令行输入在 main.go 中处理，这里返回 nil
	return nil
}

// Send 发送消息到命令行
func (c *CLIChannel) Send(to string, text string, kwargs map[string]interface{}) bool {
	fmt.Println(text)
	return true
}

// Close 关闭通道
func (c *CLIChannel) Close() {
	// 不需要关闭
}

// MailChannel 邮件通道
type MailChannel struct {
	AccountID    string
	SMTPHost     string
	SMTPPort     string
	Username     string
	Password     string
	From         string
	IsMailHog    bool // 是否为MailHog测试环境
	IMAPHost     string
	IMAPPort     string
	LastCheck    time.Time       // 上次检查邮件的时间
	ProcessedIDs map[string]bool // 已处理的邮件ID
}

// Name 返回通道名称
func (m *MailChannel) Name() string {
	return "mail"
}

// Receive 接收邮件
func (m *MailChannel) Receive() *InboundMessage {
	// 如果没有配置IMAP主机，返回nil
	if m.IMAPHost == "" {
		return nil
	}

	// 连接到IMAP服务器
	c, err := client.Dial(fmt.Sprintf("%s:%s", m.IMAPHost, m.IMAPPort))
	if err != nil {
		fmt.Printf("Error connecting to IMAP server: %v\n", err)
		return nil
	}
	defer c.Logout()

	// 登录
	if m.Username != "" && m.Password != "" {
		if err := c.Login(m.Username, m.Password); err != nil {
			fmt.Printf("Error logging in to IMAP server: %v\n", err)
			return nil
		}
	} else if m.IsMailHog {
		// MailHog不需要认证
	} else {
		fmt.Println("No IMAP credentials provided")
		return nil
	}

	// 选择收件箱
	_, err = c.Select("INBOX", false)
	if err != nil {
		fmt.Printf("Error selecting inbox: %v\n", err)
		return nil
	}

	// 搜索新邮件
	criteria := imap.NewSearchCriteria()
	criteria.Since = m.LastCheck
	ids, err := c.Search(criteria)
	if err != nil {
		fmt.Printf("Error searching for emails: %v\n", err)
		return nil
	}

	// 处理新邮件
	if len(ids) > 0 {
		// 获取邮件
		seqset := new(imap.SeqSet)
		seqset.AddNum(ids...)

		// 获取邮件内容
		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)
		go func() {
			done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchBody}, messages)
		}()

		// 处理邮件
		for msg := range messages {
			// 检查是否已经处理过
			msgID := msg.Envelope.MessageId
			if msgID == "" {
				// 如果没有MessageID，使用日期和主题生成一个
				msgID = fmt.Sprintf("%d_%s", msg.Envelope.Date.Unix(), msg.Envelope.Subject)
			}

			if m.ProcessedIDs[msgID] {
				continue
			}

			// 标记为已处理
			m.ProcessedIDs[msgID] = true

			// 提取发件人
			sender := ""
			if len(msg.Envelope.From) > 0 {
				sender = msg.Envelope.From[0].Address()
			}

			// 提取邮件内容
			var body string
			for _, part := range msg.Body {
				if part != nil {
					buf := make([]byte, 1024)
					n, err := part.Read(buf)
					if err == nil {
						body = string(buf[:n])
					}
				}
			}

			// 创建InboundMessage
			inboundMsg := &InboundMessage{
				Text:      body,
				SenderID:  sender,
				Channel:   "mail",
				AccountID: m.AccountID,
				PeerID:    sender,
				IsGroup:   false,
				Media:     []map[string]interface{}{},
				Raw: map[string]interface{}{
					"message_id": msgID,
					"subject":    msg.Envelope.Subject,
					"date":       msg.Envelope.Date,
				},
			}

			// 更新最后检查时间
			m.LastCheck = time.Now()

			return inboundMsg
		}

		// 等待获取完成
		if err := <-done; err != nil {
			fmt.Printf("Error fetching emails: %v\n", err)
		}
	}

	// 更新最后检查时间
	m.LastCheck = time.Now()

	return nil
}

// Send 发送邮件
func (m *MailChannel) Send(to string, text string, kwargs map[string]interface{}) bool {
	// 构建邮件内容
	subject := "GarClaw Response"
	if subj, ok := kwargs["subject"].(string); ok && subj != "" {
		subject = subj
	}

	message := fmt.Sprintf("From: %s\r\n", m.From)
	message += fmt.Sprintf("To: %s\r\n", to)
	message += fmt.Sprintf("Subject: %s\r\n", subject)
	message += fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123))
	message += "Content-Type: text/plain; charset=utf-8\r\n"
	message += "\r\n"
	message += text

	// 连接到 SMTP 服务器
	serverAddr := fmt.Sprintf("%s:%s", m.SMTPHost, m.SMTPPort)

	// 直接连接到 SMTP 服务器
	c, err := smtp.Dial(serverAddr)
	if err != nil {
		fmt.Printf("Error dialing SMTP server: %v\n", err)
		return false
	}
	defer c.Close()

	// 尝试升级到 TLS 连接
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // 跳过证书验证，适用于测试环境
		ServerName:         m.SMTPHost,
	}

	// 尝试 TLS 连接
	tlsSuccess := true
	if err = c.StartTLS(tlsConfig); err != nil {
		if m.IsMailHog {
			if isDebug {
				fmt.Printf("Warning: Failed to start TLS, continuing without encryption: %v\n", err)
			}
			tlsSuccess = false
			// 不返回错误，继续使用非 TLS 连接
		} else {
			fmt.Printf("Error starting TLS: %v\n", err)
			return false
		}
	}

	// 如果有认证信息且 TLS 成功，使用认证
	// 对于非 TLS 连接，跳过认证（如 MailHog 等测试服务器）
	if m.Username != "" && m.Password != "" && tlsSuccess {
		auth := smtp.PlainAuth("", m.Username, m.Password, m.SMTPHost)
		if err = c.Auth(auth); err != nil {
			fmt.Printf("Error authenticating: %v\n", err)
			return false
		}
	}

	// 设置发件人
	if err = c.Mail(m.From); err != nil {
		fmt.Printf("Error setting sender: %v\n", err)
		return false
	}

	// 设置收件人
	if err = c.Rcpt(to); err != nil {
		fmt.Printf("Error setting recipient: %v\n", err)
		return false
	}

	// 发送邮件内容
	data, err := c.Data()
	if err != nil {
		fmt.Printf("Error getting data writer: %v\n", err)
		return false
	}

	_, err = data.Write([]byte(message))
	if err != nil {
		fmt.Printf("Error writing message: %v\n", err)
		return false
	}

	err = data.Close()
	if err != nil {
		fmt.Printf("Error closing data writer: %v\n", err)
		return false
	}

	err = c.Quit()
	if err != nil {
		// 有些 SMTP 服务器（如 MailHog）在非 TLS 连接下可能不支持 QUIT 命令
		// 这里不返回错误，因为邮件已经发送成功
		fmt.Printf("Warning: Error quitting SMTP session: %v\n", err)
	}

	return true
}

// Close 关闭通道
func (m *MailChannel) Close() {
	// 不需要关闭
}

// ChannelManager 通道管理器
type ChannelManager struct {
	channels map[string]Channel
	accounts []ChannelAccount
}

// NewChannelManager 创建通道管理器
func NewChannelManager() *ChannelManager {
	return &ChannelManager{
		channels: make(map[string]Channel),
		accounts: make([]ChannelAccount, 0),
	}
}

// Register 注册通道
func (cm *ChannelManager) Register(channel Channel) {
	cm.channels[channel.Name()] = channel
	fmt.Printf("[Channel] Registered: %s\n", channel.Name())
}

// ListChannels 列出所有通道
func (cm *ChannelManager) ListChannels() []string {
	channels := make([]string, 0, len(cm.channels))
	for name := range cm.channels {
		channels = append(channels, name)
	}
	return channels
}

// Get 获取通道
func (cm *ChannelManager) Get(name string) Channel {
	return cm.channels[name]
}

// CloseAll 关闭所有通道
func (cm *ChannelManager) CloseAll() {
	for _, channel := range cm.channels {
		channel.Close()
	}
}

// BuildSessionKey 构建会话键
func BuildSessionKey(channel, accountID, peerID string) string {
	return fmt.Sprintf("agent:main:direct:%s:%s", channel, peerID)
}

// InitializeChannels 初始化通道
func InitializeChannels() *ChannelManager {
	cm := NewChannelManager()

	// 注册 CLI 通道
	cliChannel := &CLIChannel{
		AccountID: "cli-local",
	}
	cm.Register(cliChannel)

	// 注册邮件通道（如果配置了）
	if smtpHost := os.Getenv("SMTP_HOST"); smtpHost != "" {
		smtpPort := os.Getenv("SMTP_PORT")
		if smtpPort == "" {
			smtpPort = "587" // 默认 SMTP 端口
		}

		username := os.Getenv("SMTP_USERNAME")
		password := os.Getenv("SMTP_PASSWORD")
		from := os.Getenv("SMTP_FROM")

		// 检查是否为MailHog环境
		isMailHog := false
		if isMailHogStr := os.Getenv("IS_MAILHOG"); isMailHogStr != "" {
			if val, err := strconv.ParseBool(isMailHogStr); err == nil {
				isMailHog = val
			}
		}

		// 读取IMAP配置
		imapHost := os.Getenv("IMAP_HOST")
		imapPort := os.Getenv("IMAP_PORT")
		if imapPort == "" {
			imapPort = "143" // 默认 IMAP 端口
			if isMailHog {
				imapPort = "143" // MailHog默认IMAP端口
			}
		}

		// 即使没有认证信息，也允许注册邮件通道（例如 MailHog测试时可以不配置认证信息）
		if from != "" {
			mailChannel := &MailChannel{
				AccountID:    "mail-primary",
				SMTPHost:     smtpHost,
				SMTPPort:     smtpPort,
				Username:     username,
				Password:     password,
				From:         from,
				IsMailHog:    isMailHog,
				IMAPHost:     imapHost,
				IMAPPort:     imapPort,
				LastCheck:    time.Now(),
				ProcessedIDs: make(map[string]bool),
			}
			cm.Register(mailChannel)

			// 添加到账户列表
			cm.accounts = append(cm.accounts, ChannelAccount{
				Channel:   "mail",
				AccountID: "mail-primary",
				Config: map[string]interface{}{
					"smtp_host":  smtpHost,
					"smtp_port":  smtpPort,
					"from":       from,
					"is_mailhog": isMailHog,
					"imap_host":  imapHost,
					"imap_port":  imapPort,
				},
			})
		}
	}

	return cm
}
