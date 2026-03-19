# GarClaw

GarClaw 是一个基于 LLM（大语言模型）的多前端智能助手，使用 Go 语言开发，支持命令行、Web 网页和邮件三种交互方式，提供文件操作、系统命令执行、网络搜索与下载等多种工具调用能力。

## 功能特性

- **多前端支持**：命令行（`readline`）、Web 网页（WebSocket）、邮件（IMAP/SMTP）三种方式均可与 AI 交互
- **多模型兼容**：支持 OpenAI、Anthropic、Ollama 等标准接口，以及 DeepSeek、GLM、MiniMax 等国内模型
- **工具调用**：内置 shell 命令执行、文件读写、百度搜索、网页访问与下载、待办事项管理等多种工具
- **流式输出**：实时显示模型响应，提供丝滑交互体验
- **思考模式**：支持展示模型的推理过程（如 DeepSeek 的 reasoning_content）
- **对话历史管理**：每个前端独立维护对话上下文，支持连续多轮对话
- **跨平台**：自动适配 Windows 与 Unix 命令差异，拦截危险操作
- **灵活配置**：TOON 格式配置文件 + 环境变量双重配置

## 支持的工具

1. **shell**：执行系统命令，自动拦截危险命令（如 `rm -rf /`）
2. **read_file_line**：读取文件指定行
3. **write_file_line**：写入文件指定行（自动扩展空行）
4. **read_all_lines**：读取文件所有行
5. **write_all_lines**：覆盖写入文件所有行
6. **search**：使用百度搜索引擎搜索关键词
7. **visit**：访问 URL 并提取可见文本内容
8. **download**：下载网页 HTML 保存为本地文件
9. **todo**：管理待办事项列表，支持状态跟踪（pending/in_progress/completed）

## 快速开始

### 前置条件

- Go 1.20 或更高版本
- 对应 AI 模型的 API Key（如 DeepSeek、GLM、OpenAI 等）
- （可选）如需邮件功能，需准备邮箱的 IMAP/SMTP 配置
- （可选）如需网页功能，浏览器可自动检测（Chrome/Chromium/Firefox）

### 安装

```bash
git clone https://github.com/naamfung/garclaw.git --depth=1
cd garclaw
go build -o garclaw .
```

### 配置

首次运行会自动生成 `config.toon` 配置文件（位于可执行文件同目录），内容示例：

```toon
APIConfig:
  APIType: openai               # 可选 anthropic, ollama, openai
  BaseURL: "https://api.deepseek.com/beta"
  APIKey: "your-api-key"
  Model: "deepseek-chat"
  Temperature: 0.0
  MaxTokens: 8192
  Stream: true
  Thinking: true

HTTPServer:
  Listen: "0.0.0.0:10086"        # Web 服务监听地址，设为空可禁用

EmailConfig:                     # 如需邮件功能，取消注释并填写
  IMAPServer: "imap.example.com"
  IMAPPort: 993
  IMAPUseTLS: true
  IMAPUser: "user@example.com"
  IMAPPassword: "your-password"
  SMTPServer: "smtp.example.com"
  SMTPPort: 587
  SMTPUseTLS: true
  SMTPUser: "user@example.com"
  SMTPPassword: "your-password"
  PollInterval: 30               # 轮询间隔（秒）
```

也可通过环境变量覆盖（优先级更高）：
- `API_TYPE`、`BASE_URL`、`API_KEY`、`MODEL_ID`、`TEMPERATURE`、`MAX_TOKENS`、`STREAM`、`THINKING`
- 针对不同平台：`OPENAI_API_KEY`、`ANTHROPIC_API_KEY`、`OPENAI_BASE_URL`、`ANTHROPIC_BASE_URL`

## 使用方法

### 命令行模式

运行程序后直接进入交互式命令行：

```bash
GarClaw /> 列出当前目录的文件
GarClaw /> 读取 main.go 的第 5 行
GarClaw /> 搜索 "最新 AI 资讯"
```

支持历史记录、行编辑（`readline`），输入 `exit` 或 `q` 退出。

### 网页模式

启动程序后，浏览器访问 `http://<配置的地址>:10086`（默认 `http://0.0.0.0:10086`），即可打开聊天页面，通过 WebSocket 与 AI 实时对话。

### 邮件模式

在配置文件中填写 `EmailConfig` 后，程序会按指定间隔轮询邮箱收件箱，将每封新邮件内容作为用户输入，自动回复处理结果。回复邮件的主题为原邮件的主题前加 "Re:"。

## 项目结构

```
garclaw/
├── main.go               # 程序入口，启动各前端
├── config.go             # 配置加载
├── channel.go            # 频道接口定义
├── cmd_channel.go        # 命令行频道
├── ws_channel.go         # WebSocket 频道
├── email_channel.go      # 邮件频道
├── email_poller.go       # 邮件轮询器
├── http_server.go        # HTTP 与 WebSocket 服务器
├── AgentLoop.go          # 核心对话循环
├── CallModel.go          # 模型调用（流式/非流式）
├── getTools.go           # 工具定义
├── todo.go               # 待办事项管理
├── shell.go              # shell 命令执行
├── file.go               # 文件读写
├── services.go           # 浏览器自动化（搜索/访问/下载）
├── helper.go             # 辅助函数
├── const.go              # 系统提示词
├── wordsmap.go           # 词映射（用于结果替换）
├── StringReplacement.go  # 词映射排序
└── StreamChunk.go        # 流式数据块定义
```

## 安全机制

- 危险命令拦截：`rm -rf /`、`mkfs`、`dd`、`sudo` 等危险命令会被阻止执行
- 命令超时：每条 shell 命令最长运行 3 分钟，防止长时间占用
- 浏览器自动化：仅用于可控的网页访问，不执行页面内脚本

## 许可证

本项目采用 Apache License Version 2.0 许可证。

---

如有任何问题或建议，欢迎提交 Issue 或 Pull Request。
