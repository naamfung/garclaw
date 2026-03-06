package main

import (
	"fmt"
	"net/smtp"
	"os"
	"time"
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
	AccountID string
	SMTPHost  string
	SMTPPort  string
	Username  string
	Password  string
	From      string
}

// Name 返回通道名称
func (m *MailChannel) Name() string {
	return "mail"
}

// Receive 接收邮件
func (m *MailChannel) Receive() *InboundMessage {
	// 邮件接收需要实现 IMAP 或 POP3 协议，这里暂时返回 nil
	// 实际实现时需要定期检查邮件服务器
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
	auth := smtp.PlainAuth("", m.Username, m.Password, m.SMTPHost)
	serverAddr := fmt.Sprintf("%s:%s", m.SMTPHost, m.SMTPPort)

	err := smtp.SendMail(serverAddr, auth, m.From, []string{to}, []byte(message))
	if err != nil {
		fmt.Printf("Error sending email: %v\n", err)
		return false
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

		if username != "" && password != "" && from != "" {
			mailChannel := &MailChannel{
				AccountID: "mail-primary",
				SMTPHost:  smtpHost,
				SMTPPort:  smtpPort,
				Username:  username,
				Password:  password,
				From:      from,
			}
			cm.Register(mailChannel)

			// 添加到账户列表
			cm.accounts = append(cm.accounts, ChannelAccount{
				Channel:   "mail",
				AccountID: "mail-primary",
				Config: map[string]interface{}{
					"smtp_host": smtpHost,
					"smtp_port": smtpPort,
					"from":      from,
				},
			})
		}
	}

	return cm
}
