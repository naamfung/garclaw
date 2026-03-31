package main

import (
    "context"
    "errors"
    "flag"
    "fmt"
    "io"
    "log"
    "os"
    "os/signal"
    "path/filepath"
    "strings"
    "syscall"
    "time"

    "github.com/chzyer/readline"
)

// 全局 API 配置变量，供其他包使用
var (
    apiType     string
    baseURL     string
    apiKey      string
    modelID     string
    temperature float64
    maxTokens   int
    stream      bool
    thinking    bool
)

// 全局变量：默认人格
var defaultRole string

// 全局变量：是否拦截危险命令
var BlockDangerousCommands bool

// 全局变量：是否使用用户模式启动浏览器
var UserModeBrowser bool

// 调试开关，全局可见
var IsDebug = false

var globalAPIConfig APIConfig

// 全局变量：邮件配置（供 cron 等使用）
var globalEmailConfig *EmailConfig

// 全局变量：Telegram 配置
var globalTelegramConfig *TelegramConfig

// 全局变量：Telegram Channel
var globalTelegramChannel *TelegramChannel

// 全局变量：Discord 配置
var globalDiscordConfig *DiscordConfig

// 全局变量：Discord Channel
var globalDiscordChannel *DiscordChannel

// 全局变量：Slack 配置
var globalSlackConfig *SlackConfig

// 全局变量：Slack Channel
var globalSlackChannel *SlackChannel

// 全局变量：Feishu 配置
var globalFeishuConfig *FeishuConfig

// 全局变量：Feishu Channel
var globalFeishuChannel *FeishuChannel

// 全局变量：IRC 配置
var globalIRCConfig *IRCConfig

// 全局变量：IRC Channel
var globalIRCChannel *IRCChannel

// 全局变量：Webhook 配置
var globalWebhookConfig *WebhookConfig

// 全局变量：Webhook Channel
var globalWebhookChannel *WebhookChannel

// 全局变量：XMPP 配置
var globalXMPPConfig *XMPPConfig

// 全局变量：XMPP Channel
var globalXMPPChannel *XMPPChannel

// 全局变量：Matrix 配置
var globalMatrixConfig *MatrixConfig

// 全局变量：Matrix Channel
var globalMatrixChannel *MatrixChannel

// 全局变量：超时配置
var globalTimeoutConfig TimeoutConfig

// 全局变量：工具开关配置
var globalToolsConfig ToolsConfig

// 全局变量：定时任务管理器
var globalCronManager *CronManager

// 全局变量：任务进度追踪器
var globalTaskTracker *TaskTracker

// 全局变量：用于优雅关闭的 context 和取消函数
var globalCancel context.CancelFunc

// 全局变量：角色模板管理器
var globalRoleManager *RoleManager

// 全局变量：演员管理器
var globalActorManager *ActorManager

// 全局变量：场景管理器
var globalStage *Stage

// 全局变量：技能管理器
var globalSkillManager *SkillManager

// 全局变量：后台任务管理器
var globalTaskManager *TaskManager

// 全局变量：MCP 服务器
var globalMCPServer *MCPServer

// 全局变量：统一记忆系统
var globalUnifiedMemory *UnifiedMemory

// 全局变量：认证配置
var globalAuthConfig AuthConfig

// 全局变量：程序所在目录（用于构建相对路径）
var globalExecDir string

// 全局变量：Profile 加载器
var globalProfileLoader *ProfileLoader

// 全局变量：群聊策略全局配置（指针类型，与 Config 中一致）
var globalGroupChatConfig *GroupChatConfig

// 消息结构
type Message struct {
    Role             string      `json:"role"`
    Content          interface{} `json:"content,omitempty"`
    ToolCalls        interface{} `json:"tool_calls,omitempty"`
    ToolCallID       string      `json:"tool_call_id,omitempty"`
    ReasoningContent interface{} `json:"reasoning_content,omitempty"`
}

// 工具调用结构
type ToolUse struct {
    Type  string                 `json:"type"`
    ID    string                 `json:"id"`
    Name  string                 `json:"name"`
    Input map[string]interface{} `json:"input"`
}

// 响应结构
type Response struct {
    Content          interface{} `json:"content"`
    StopReason       string      `json:"stop_reason"`
    ReasoningContent interface{} `json:"reasoning_content,omitempty"`
    ToolCalls        interface{} `json:"tool_calls,omitempty"`
}

func main() {
    // 命令行参数
    promptFlag := flag.String("p", "", "调试模式：直接传入提示词，模型输出完成后自动退出")
    promptFlagLong := flag.String("prompt", "", "调试模式：直接传入提示词，模型输出完成后自动退出（长格式）")
    debugFlag := flag.Bool("debug", false, "启用调试输出")
    flag.Parse()

    // 合并短格式和长格式参数
    prompt := *promptFlag
    if prompt == "" {
        prompt = *promptFlagLong
    }

    // 设置调试模式
    if *debugFlag {
        IsDebug = true
    }

    // 初始化程序所在目录（必须在其他初始化之前）
    execPath, err := os.Executable()
    if err != nil {
        log.Printf("Warning: cannot get executable path: %v", err)
        execPath = "."
    }
    globalExecDir = filepath.Dir(execPath)

    // 加载配置
    config, err := loadConfig()
    if err != nil {
        fmt.Printf("Warning: %v\n", err)
    }

    // 检查是否需要配置向导
    if NeedsSetup(config) {
        result := RunConfigWizard(config)
        if !result.IsCompleted {
            fmt.Println("配置未完成，程序退出。")
            os.Exit(0)
        }
        config = result.Config
    }

    // 从配置中赋值全局变量
    apiType = config.APIConfig.APIType
    globalAPIConfig = config.APIConfig
    baseURL = config.APIConfig.BaseURL
    apiKey = config.APIConfig.APIKey
    modelID = config.APIConfig.Model
    temperature = config.APIConfig.Temperature
    maxTokens = config.APIConfig.MaxTokens
    stream = config.APIConfig.Stream
    thinking = config.APIConfig.Thinking
    BlockDangerousCommands = config.APIConfig.BlockDangerousCommands
    UserModeBrowser = config.BrowserConfig.UserMode
    globalEmailConfig = config.EmailConfig
    globalTelegramConfig = config.TelegramConfig
    globalDiscordConfig = config.DiscordConfig
    globalSlackConfig = config.SlackConfig
    globalFeishuConfig = config.FeishuConfig
    globalIRCConfig = config.IRCConfig
    globalWebhookConfig = config.WebhookConfig
    globalXMPPConfig = config.XMPPConfig
    globalMatrixConfig = config.MatrixConfig
    globalTimeoutConfig = config.Timeout
    globalToolsConfig = config.Tools
    defaultRole = config.DefaultRole
    globalAuthConfig = config.Auth

    // 设置全局群聊策略（注意：config.GroupChatConfig 已经是指针类型）
    globalGroupChatConfig = config.GroupChatConfig

    // 初始化安全配置
    SetSecurityConfig(config.Security)
    if config.Security.EnableSSRFProtection {
        log.Println("SSRF protection is ENABLED.")
    } else {
        log.Println("WARNING: SSRF protection is DISABLED. This is not recommended for production.")
    }

    fmt.Printf("Using model: %s\n", modelID)
    if !BlockDangerousCommands {
        fmt.Println("Dangerous command blocking is DISABLED. The model can execute any command.")
    }
    if UserModeBrowser {
        fmt.Println("Browser user mode is ENABLED. Using existing browser session.")
    }

    // 初始化插件管理器
    pluginsDir := config.PluginsDir
    globalPluginManager = NewPluginManager(pluginsDir)
    globalPluginManager.SetToolExecutor(callToolInternal)
    if err := globalPluginManager.LoadPluginsFromDir(); err != nil {
        log.Printf("Warning: failed to load plugins: %v", err)
    }
    plugins := globalPluginManager.ListPlugins()
    if len(plugins) > 0 {
        log.Printf("Loaded %d plugin(s):", len(plugins))
        for _, p := range plugins {
            log.Printf("  - %s (%s)", p["name"], p["file"])
        }
    } else {
        log.Println("No plugins loaded. Plugins directory:", pluginsDir)
    }
    defer func() {
        if globalPluginManager != nil {
            globalPluginManager.Close()
        }
    }()

    // 初始化 CronManager
    cronFilePath := filepath.Join(globalExecDir, "cron.toon")
    globalCronManager, err = NewCronManager(cronFilePath, &config.CronConfig)
    if err != nil {
        log.Printf("Warning: failed to start cron manager: %v", err)
    } else {
        defer globalCronManager.Stop()
        log.Println("Cron manager started.")
    }

    // 初始化统一记忆系统
    globalUnifiedMemory, err = NewUnifiedMemory(globalExecDir)
    if err != nil {
        log.Printf("Warning: failed to start unified memory: %v", err)
    } else {
        log.Printf("Unified memory system started.")
    }

    // 初始化任务进度追踪器
    globalTaskTracker = NewTaskTracker()

    // 初始化循环检测器
    InitGlobalLoopDetector()
    log.Println("Loop detector initialized.")

    // 初始化后台任务管理器
    globalTaskManager = NewTaskManager()
    globalTaskManager.SetWakeHandler(func(task *BackgroundTask) {
        log.Printf("[TaskManager] Task %s wake up, status: %s", task.ID, task.Status)

        task.mu.RLock()
        output := truncateTaskOutput(task.Stdout.String())
        _ = truncateTaskOutput(task.Stderr.String())
        task.mu.RUnlock()

        wakeMsg := GetTaskWakeMessage(task)

        if task.SessionID != "" {
            GetBus().NotifyDelayedTask(
                task.ID,
                task.Command,
                string(task.Status),
                output,
                task.SessionID,
            )
            log.Printf("[TaskManager] Wake notification sent for task %s to session %s", task.ID, task.SessionID)

            if globalWebSessionManager != nil {
                if session := globalWebSessionManager.Get(task.SessionID); session != nil {
                    if session.IsConnected() && !session.IsTaskRunning() {
                        session.AddToHistory("user", wakeMsg)
                        log.Printf("[TaskManager] Triggering model call for session %s", task.SessionID)
                        go TriggerDelayedTaskWake(session, wakeMsg)
                    } else if session.IsTaskRunning() {
                        session.EnqueueOutput(StreamChunk{
                            Content: "\n\n" + wakeMsg + "\n\n",
                        })
                    } else {
                        log.Printf("[TaskManager] Session %s not connected, wake message will be delivered on reconnect", task.SessionID)
                    }
                }
            }
        } else {
            log.Printf("[TaskManager] Task %s has no session ID, cannot send wake notification", task.ID)
        }
    })
    log.Println("Task manager started.")
    defer func() {
        if globalTaskManager != nil {
            globalTaskManager.Stop()
        }
    }()

    // 初始化消息总线
    initMessageBus()
    log.Println("Message bus initialized.")

    // 初始化子代理管理器
    globalSubagentManager = NewSubagentManager()
    globalSubagentManager.SetResultHandler(func(task *SubagentTask) {
        log.Printf("[Subagent] Task %s completed: %s", task.ID, task.Status)
        if task.SessionID != "" {
            GetBus().NotifySubagent(task.ID, string(task.Status), task.Result, task.SessionID)
        }
    })
    log.Println("Subagent manager started.")
    defer func() {
        if globalSubagentManager != nil {
            globalSubagentManager.Stop()
        }
    }()

    // 初始化心跳服务
    if config.Heartbeat.Enabled {
        globalHeartbeatService = NewHeartbeatService(config.Heartbeat, globalExecDir)
        SetHeartbeatNotifier(NewBusHeartbeatNotifier())
        if err := globalHeartbeatService.Start(); err != nil {
            log.Printf("Warning: failed to start heartbeat service: %v", err)
        }
        defer func() {
            if globalHeartbeatService != nil {
                globalHeartbeatService.Stop()
            }
        }()
    } else {
        log.Println("Heartbeat service is disabled")
    }

    // 初始化角色模板管理器
    roleFilePath := filepath.Join(globalExecDir, "role.toon")
    globalRoleManager, err = NewRoleManager(roleFilePath)
    if err != nil {
        log.Printf("Warning: failed to start role manager: %v", err)
    } else {
        log.Printf("Role manager started. %d roles available.", globalRoleManager.Count())
    }

    // 初始化演员管理器
    actorFilePath := filepath.Join(globalExecDir, "actor.toon")
    globalActorManager, err = NewActorManager(actorFilePath, apiType, baseURL, apiKey, modelID, temperature, maxTokens, config.DefaultRole)
    if err != nil {
        log.Printf("Warning: failed to start actor manager: %v", err)
    } else {
        log.Printf("Actor manager started. %d actors available.", len(globalActorManager.ListActors()))
    }

    // 初始化场景管理器
    globalStage = NewStage()

    // 初始化 ProfileLoader（加载 profiles/USER.md, SOUL.md, AGENT.md, TOOLS.md 及 actors/*/IDENTITY.md）
    profilesDir := filepath.Join(globalExecDir, "profiles")
    globalProfileLoader, err = NewProfileLoader(profilesDir)
    if err != nil {
        log.Printf("Warning: failed to start profile loader: %v", err)
    } else {
        defer globalProfileLoader.Stop()
        log.Println("Profile loader started.")
    }

    // 加载工具别名（tools.toon）
    toolsAliasPath := filepath.Join(globalExecDir, "tools.toon")
    globalToolsAliases, err = LoadToolsAliases(toolsAliasPath)
    if err != nil {
        log.Printf("Tools aliases not loaded: %v", err)
    } else {
        log.Printf("Tools aliases loaded: %d entries", len(globalToolsAliases))
    }

    // 初始化技能管理器
    skillsDir := filepath.Join(globalExecDir, "skills")
    globalSkillManager, err = NewSkillManager(skillsDir)
    if err != nil {
        log.Printf("Warning: failed to start skill manager: %v", err)
    } else {
        log.Printf("Skill manager started. %d skills available.", globalSkillManager.Count())
    }

    // 初始化 MCP 服务器
    if config.MCP.Enabled {
        globalMCPServer = NewMCPServer("GarClaw", "1.0.0")
        initMCPTools(globalMCPServer)
        log.Printf("MCP server started (transport: %s)", config.MCP.Transport)

        if config.MCP.Transport == "stdio" {
            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()
            log.Println("MCP server running in stdio mode")
            if err := globalMCPServer.StartStdio(ctx); err != nil {
                log.Fatalf("MCP stdio error: %v", err)
            }
            return
        }
    }

    // 初始化 MCP 客户端管理器
    if err := InitMCPClients(globalExecDir); err != nil {
        log.Printf("Warning: failed to init MCP clients: %v", err)
    } else if globalMCPClientManager != nil && globalMCPClientManager.Count() > 0 {
        log.Printf("MCP client manager started with %d server(s)", globalMCPClientManager.Count())
    }
    defer func() {
        if globalMCPClientManager != nil {
            globalMCPClientManager.DisconnectAll()
        }
    }()

    // 初始化记忆整合器
    consolidatorConfig := DefaultMemoryConsolidatorConfig()
    if config.Memory != nil {
        if config.Memory.MinMessagesToConsolidate > 0 {
            consolidatorConfig.MinMessagesToConsolidate = config.Memory.MinMessagesToConsolidate
        }
        if config.Memory.ConsolidationRatio > 0 {
            consolidatorConfig.ConsolidationRatio = config.Memory.ConsolidationRatio
        }
        if config.Memory.ContextWindowTokens > 0 {
            consolidatorConfig.ContextWindowTokens = config.Memory.ContextWindowTokens
        }
    }
    InitMemoryConsolidator(consolidatorConfig, globalUnifiedMemory)
    log.Printf("Memory consolidator initialized (MinMsgs: %d, Ratio: %.2f%%)",
        consolidatorConfig.MinMessagesToConsolidate,
        consolidatorConfig.ConsolidationRatio*100)

    // 初始化会话持久化管理器
    InitSessionPersist()
    log.Println("Session persistence initialized")

    // 初始化渠道会话管理器
    InitChannelSessionManager()
    log.Println("Channel session manager initialized")

    // 初始化 Hook 管理器
    InitHookManager(&config)
    if globalHookManager != nil {
        hooks := globalHookManager.List()
        enabledCount := 0
        for _, h := range hooks {
            if h.Enabled {
                enabledCount++
            }
        }
        log.Printf("Hook manager started. %d hooks found, %d enabled", len(hooks), enabledCount)
    }

    // 启动 HTTP 服务器
    if config.HTTPServer.Listen != "" {
        httpServer := NewHTTPServer(config.HTTPServer.Listen)
        go func() {
            httpServer.Start()
        }()
    }

    // 启动邮件轮询
    var emailPoller *EmailPoller
    if config.EmailConfig != nil {
        emailPoller = &EmailPoller{config: config.EmailConfig, stop: make(chan struct{})}
        emailPoller.Start()
        log.Println("Email polling started")
    }

    // 启动 Telegram Bot
    if config.TelegramConfig != nil && config.TelegramConfig.Enabled {
        telegramChannel, err := NewTelegramChannel(config.TelegramConfig)
        if err != nil {
            log.Printf("Warning: failed to create Telegram channel: %v", err)
        } else {
            globalTelegramChannel = telegramChannel
            err = telegramChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("Telegram message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "telegram")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "telegram", senderID, content, metadata, telegramChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start Telegram bot: %v", err)
            } else {
                log.Println("Telegram bot started")
                telegramChannel.RegisterToBus()
            }
        }
    }

    // 启动 Discord Bot
    if config.DiscordConfig != nil && config.DiscordConfig.Enabled {
        discordChannel, err := NewDiscordChannel(config.DiscordConfig)
        if err != nil {
            log.Printf("Warning: failed to create Discord channel: %v", err)
        } else {
            globalDiscordChannel = discordChannel
            err = discordChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("Discord message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "discord")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "discord", senderID, content, metadata, discordChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start Discord bot: %v", err)
            } else {
                log.Println("Discord bot started")
                discordChannel.RegisterToBus()
            }
        }
    }

    // 启动 Slack Bot
    if config.SlackConfig != nil && config.SlackConfig.Enabled {
        slackChannel, err := NewSlackChannel(config.SlackConfig)
        if err != nil {
            log.Printf("Warning: failed to create Slack channel: %v", err)
        } else {
            globalSlackChannel = slackChannel
            err = slackChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("Slack message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "slack")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "slack", senderID, content, metadata, slackChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start Slack bot: %v", err)
            } else {
                log.Println("Slack bot started")
                slackChannel.RegisterToBus()
            }
        }
    }

    // 启动 Feishu Bot
    if config.FeishuConfig != nil && config.FeishuConfig.Enabled {
        feishuChannel, err := NewFeishuChannel(config.FeishuConfig)
        if err != nil {
            log.Printf("Warning: failed to create Feishu channel: %v", err)
        } else {
            globalFeishuChannel = feishuChannel
            err = feishuChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("Feishu message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "feishu")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "feishu", senderID, content, metadata, feishuChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start Feishu bot: %v", err)
            } else {
                log.Println("Feishu bot started")
                feishuChannel.RegisterToBus()
            }
        }
    }

    // 启动 IRC Bot
    if config.IRCConfig != nil && config.IRCConfig.Enabled {
        ircChannel, err := NewIRCChannel(config.IRCConfig)
        if err != nil {
            log.Printf("Warning: failed to create IRC channel: %v", err)
        } else {
            globalIRCChannel = ircChannel
            err = ircChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("IRC message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "irc")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "irc", senderID, content, metadata, ircChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start IRC bot: %v", err)
            } else {
                log.Println("IRC bot started")
                ircChannel.RegisterToBus()
            }
        }
    }

    // 启动 Webhook 服务
    if config.WebhookConfig != nil && config.WebhookConfig.Enabled {
        webhookChannel, err := NewWebhookChannel(config.WebhookConfig)
        if err != nil {
            log.Printf("Warning: failed to create Webhook channel: %v", err)
        } else {
            globalWebhookChannel = webhookChannel
            err = webhookChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("Webhook message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "webhook")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "webhook", senderID, content, metadata, webhookChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start Webhook server: %v", err)
            } else {
                log.Println("Webhook server started")
                webhookChannel.RegisterToBus()
            }
        }
    }

    // 启动 XMPP Bot
    if config.XMPPConfig != nil && config.XMPPConfig.Enabled {
        xmppChannel, err := NewXMPPChannel(config.XMPPConfig)
        if err != nil {
            log.Printf("Warning: failed to create XMPP channel: %v", err)
        } else {
            globalXMPPChannel = xmppChannel
            err = xmppChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("XMPP message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "xmpp")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "xmpp", senderID, content, metadata, xmppChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start XMPP bot: %v", err)
            } else {
                log.Println("XMPP bot started")
                xmppChannel.RegisterToBus()
            }
        }
    }

    // 启动 Matrix Bot
    if config.MatrixConfig != nil && config.MatrixConfig.Enabled {
        matrixChannel, err := NewMatrixChannel(config.MatrixConfig)
        if err != nil {
            log.Printf("Warning: failed to create Matrix channel: %v", err)
        } else {
            globalMatrixChannel = matrixChannel
            err = matrixChannel.Start(func(chatID, senderID, content string, metadata map[string]interface{}) {
                log.Printf("Matrix message from %s: %s", senderID, content)
                GetBus().RegisterUserChannel(senderID, "matrix")
                go func() {
                    ctx := context.Background()
                    ProcessChannelMessage(ctx, "matrix", senderID, content, metadata, matrixChannel)
                }()
            })
            if err != nil {
                log.Printf("Warning: failed to start Matrix bot: %v", err)
            } else {
                log.Println("Matrix bot started")
                matrixChannel.RegisterToBus()
            }
        }
    }

    // 调试模式：直接执行提示词并退出
    if prompt != "" {
        runDebugMode(prompt)
        return
    }

    // 命令行界面
    rl, err := readline.New("GarClaw /> ")
    if err != nil {
        log.Fatalf("Failed to create readline: %v", err)
    }
    defer rl.Close()

    cmdChan := NewCmdChannel()
    var history []Message

    ctx, cancel := context.WithCancel(context.Background())
    globalCancel = cancel

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        fmt.Println("\nShutting down...")

        cancel()

        if globalChannelSessionManager != nil {
            log.Println("Saving all sessions...")
            globalChannelSessionManager.SaveAllSessions()
        }

        if emailPoller != nil {
            emailPoller.Stop()
        }
        if globalTelegramChannel != nil {
            globalTelegramChannel.Stop()
        }
        if globalDiscordChannel != nil {
            globalDiscordChannel.Stop()
        }
        if globalSlackChannel != nil {
            globalSlackChannel.Stop()
        }
        if globalFeishuChannel != nil {
            globalFeishuChannel.Stop()
        }
        if globalIRCChannel != nil {
            globalIRCChannel.Stop()
        }
        if globalWebhookChannel != nil {
            globalWebhookChannel.Stop()
        }
        if globalXMPPChannel != nil {
            globalXMPPChannel.Stop()
        }
        if globalMatrixChannel != nil {
            globalMatrixChannel.Stop()
        }
        if globalCronManager != nil {
            globalCronManager.Stop()
        }
        rl.Close()
    }()

    for {
        line, err := rl.Readline()
        if err != nil {
            if err == io.EOF {
                break
            }
            if errors.Is(err, readline.ErrInterrupt) {
                break
            }
            fmt.Printf("Readline error: %v\n", err)
            break
        }
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }

        if strings.HasPrefix(line, "/") {
            if globalRoleManager != nil && globalActorManager != nil && globalStage != nil {
                result := ProcessSlashCommand(line, globalRoleManager, globalActorManager, globalStage)
                if result.Handled {
                    fmt.Println(result.Response)
                    if result.IsExit {
                        break
                    }
                    continue
                }
            }
        }

        history = append(history, Message{Role: "user", Content: line})
        if globalTaskTracker != nil {
            globalTaskTracker.StartNewTask(line)
        }
        newHistory, err := AgentLoop(ctx, cmdChan, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
        if err != nil {
            fmt.Printf("Agent error: %v\n", err)
        } else {
            history = newHistory
        }
        if globalTaskTracker != nil {
            globalTaskTracker.MarkCompleted()
        }
        fmt.Println()
    }

    if len(history) > 0 && globalSessionPersist != nil {
        sessionID := fmt.Sprintf("cli_%s", time.Now().Format("20060102_150405"))
        description := "CLI session"
        for _, msg := range history {
            if msg.Role == "user" {
                if content, ok := msg.Content.(string); ok && len(content) > 0 {
                    if len(content) > 50 {
                        description = content[:50] + "..."
                    } else {
                        description = content
                    }
                }
                break
            }
        }
        if _, err := globalSessionPersist.SaveSession(sessionID, history, description); err != nil {
            log.Printf("Failed to save CLI session: %v", err)
        } else {
            log.Printf("CLI session saved: %s", sessionID)
            if globalUnifiedMemory != nil {
                summary := description
                globalUnifiedMemory.RecordSession(sessionID, "cli", summary, len(history), []string{"cli"})
            }
        }
    }

    if emailPoller != nil {
        emailPoller.Stop()
    }
}

func runDebugMode(prompt string) {
    log.Println("[Debug Mode] Starting...")

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    cmdChan := NewCmdChannel()

    var history []Message
    history = append(history, Message{Role: "user", Content: prompt})

    if globalTaskTracker != nil {
        globalTaskTracker.StartNewTask(prompt)
    }

    newHistory, err := AgentLoop(ctx, cmdChan, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
    if err != nil {
        log.Printf("[Debug Mode] Agent error: %v", err)
        os.Exit(1)
    }

    if globalTaskTracker != nil {
        globalTaskTracker.MarkCompleted()
    }

    if len(newHistory) > 0 {
        lastMsg := newHistory[len(newHistory)-1]
        if content, ok := lastMsg.Content.(string); ok && content != "" {
            fmt.Println("\n[Debug Mode] Final response:")
            fmt.Println(content)
        }
    }

    log.Println("[Debug Mode] Completed.")
}
