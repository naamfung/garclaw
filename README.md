# GarClaw

GarClaw 是一个基于 LLM（大语言模型）的命令行智能助手，使用 Go 语言开发，支持多种 AI 模型接口，提供文件操作与系统命令执行等功能。

## 功能特性

- **多模型支持**：兼容 DeepSeek、GLM、MiniMax、Ollama、OpenAI、Anthropic 等多种模型接口
- **工具调用**：支持执行 shell 命令、文件读写操作、搜索、网页访问与下载
- **流式输出**：实时显示模型响应，提供更好的交互体验
- **思考模式**：支持模型思考过程的展示
- **跨平台兼容**：支持 Windows 与 Unix 系统，自动转换命令格式
- **灵活配置**：支持配置文件与环境变量双重配置方式

## 支持的工具

1. **shell**：执行系统命令，如列出文件、创建目录等
2. **read_file_line**：读取文件指定行
3. **write_file_line**：写入文件指定行
4. **read_all_lines**：读取文件所有行
5. **write_all_lines**：写入文件所有行
6. **search**：使用百度搜索引擎搜索关键词
7. **visit**：访问 URL 并获取其内容
8. **download**：下载网页文件或网页文本
9. **todo**：管理待办事项，跟踪多步骤任务的进度

## 安装与配置

### 前置条件

- Go 1.20+ 环境
- 对应 AI 模型的 API Key（如 DeepSeek、GLM、MiniMax、Ollama、OpenAI、Anthropic）

### 安装

```bash
go build -o garclaw .
```

### 配置

程序会自动生成默认配置文件 `config.toon`，你可以根据需要修改：

```toon
APIConfig:
  APIType: openai  # 可选值: anthropic, ollama, openai
  BaseURL: "https://api.deepseek.com/beta"
  APIKey: "your-api-key"
  Model: "deepseek-chat"
  Temperature: 0.0
  MaxTokens: 8192
  Stream: true  # 启用流式输出
  Thinking: true  # 启用思考模式
```

也可以通过环境变量配置：
- `API_TYPE`：API 类型
- `BASE_URL`：API 基础 URL
- `API_KEY`：通用 API 密钥
- `OPENAI_API_KEY`/`ANTHROPIC_API_KEY`：对应 API 的密钥
- `OPENAI_BASE_URL`/`ANTHROPIC_BASE_URL`：对应 API 的基础 URL
- `MODEL_ID`：模型 ID
- `TEMPERATURE`：温度参数
- `MAX_TOKENS`：最大令牌数量
- `STREAM`：是否启用流式输出
- `THINKING`：是否启用思考模式

## 使用方法

运行程序后，在命令行中输入问题或指令：

```bash
GarClaw />
```

### 示例

1. **执行系统命令**：
   ```
   GarClaw /> 列出当前目录的文件
   ```

2. **读取文件内容**：
   ```
   GarClaw /> 读取 README.md 文件的所有内容
   ```

3. **修改文件**：
   ```
   GarClaw /> 在 main.go 文件的第 10 行添加注释 "// This is a comment"
   ```

4. **搜索关键词**：
   ```
   GarClaw /> 搜索 "人工智能发展趋势"
   ```

5. **访问网页**：
   ```
   GarClaw /> 访问 "https://example.com"
   ```

6. **下载网页**：
   ```
   GarClaw /> 下载 "https://example.com"
   ```

7. **查看待办事项**：
   ```
   GarClaw /> 查看待办事项
   ```

8. **添加待办事项**：
   ```
   GarClaw /> 添加待办事项 "完成项目文档"
   ```

## 安全注意事项

- 程序会拦截模型的危险命令，如 `rm -rf /`、`sudo` 等
- 执行命令时会设置 3 分钟超时，防止长时间运行的命令

## 许可证

本项目使用 Apache License Version 2.0 许可证。