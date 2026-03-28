package main

import (
        "context"
        "encoding/base64"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "time"
)

// ============================================================
// MCP 工具处理器
// 将 MCP 工具调用映射到 GarClaw 内部工具
// ============================================================

// initMCPTools 初始化 MCP 工具处理器
func initMCPTools(server *MCPServer) {
        // Shell 命令执行
        server.RegisterTool(MCPTool{
                Name:        "execute_shell",
                Description: "Execute a shell command on the system",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "command": map[string]interface{}{
                                        "type":        "string",
                                        "description": "The shell command to execute",
                                },
                                "timeout": map[string]interface{}{
                                        "type":        "integer",
                                        "description": "Timeout in seconds (default: 60)",
                                },
                                "cwd": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Working directory",
                                },
                        },
                        "required": []string{"command"},
                },
        }, handleExecuteShell)

        // 文件读取
        server.RegisterTool(MCPTool{
                Name:        "read_file",
                Description: "Read content from a file",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "path": map[string]interface{}{
                                        "type":        "string",
                                        "description": "The file path to read",
                                },
                                "encoding": map[string]interface{}{
                                        "type":        "string",
                                        "description": "File encoding (text or base64, default: text)",
                                },
                        },
                        "required": []string{"path"},
                },
        }, handleReadFile)

        // 文件写入
        server.RegisterTool(MCPTool{
                Name:        "write_file",
                Description: "Write content to a file",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "path": map[string]interface{}{
                                        "type":        "string",
                                        "description": "The file path to write",
                                },
                                "content": map[string]interface{}{
                                        "type":        "string",
                                        "description": "The content to write",
                                },
                                "encoding": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Content encoding (text or base64, default: text)",
                                },
                                "append": map[string]interface{}{
                                        "type":        "boolean",
                                        "description": "Append to file instead of overwrite",
                                },
                        },
                        "required": []string{"path", "content"},
                },
        }, handleWriteFile)

        // 列出目录
        server.RegisterTool(MCPTool{
                Name:        "list_directory",
                Description: "List files and directories",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "path": map[string]interface{}{
                                        "type":        "string",
                                        "description": "The directory path to list",
                                },
                        },
                        "required": []string{"path"},
                },
        }, handleListDirectory)

        // 网络搜索
        server.RegisterTool(MCPTool{
                Name:        "search_web",
                Description: "Search the web for information",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "query": map[string]interface{}{
                                        "type":        "string",
                                        "description": "The search query",
                                },
                                "num": map[string]interface{}{
                                        "type":        "integer",
                                        "description": "Number of results (default: 5)",
                                },
                        },
                        "required": []string{"query"},
                },
        }, handleSearchWeb)

        // HTTP 请求
        server.RegisterTool(MCPTool{
                Name:        "http_request",
                Description: "Make an HTTP request",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "url": map[string]interface{}{
                                        "type":        "string",
                                        "description": "The URL to request",
                                },
                                "method": map[string]interface{}{
                                        "type":        "string",
                                        "description": "HTTP method (GET, POST, etc.)",
                                },
                                "headers": map[string]interface{}{
                                        "type":        "object",
                                        "description": "HTTP headers",
                                },
                                "body": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Request body",
                                },
                        },
                        "required": []string{"url"},
                },
        }, handleHTTPRequest)

        // 记忆操作
        server.RegisterTool(MCPTool{
                Name:        "memory_save",
                Description: "Save information to long-term memory",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "key": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Memory key/identifier",
                                },
                                "value": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Memory value/content",
                                },
                                "category": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Category: preference, fact, project, skill, context",
                                },
                                "tags": map[string]interface{}{
                                        "type":        "array",
                                        "description": "Tags for organization",
                                        "items": map[string]interface{}{
                                                "type": "string",
                                        },
                                },
                        },
                        "required": []string{"key", "value"},
                },
        }, mcpHandleMemorySave)

        server.RegisterTool(MCPTool{
                Name:        "memory_recall",
                Description: "Recall information from memory",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "query": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Search query",
                                },
                                "category": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Filter by category",
                                },
                                "limit": map[string]interface{}{
                                        "type":        "integer",
                                        "description": "Maximum results (default: 10)",
                                },
                        },
                        "required": []string{"query"},
                },
        }, mcpHandleMemoryRecall)

        server.RegisterTool(MCPTool{
                Name:        "memory_list",
                Description: "List all memories",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "category": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Filter by category",
                                },
                        },
                },
        }, mcpHandleMemoryList)

        server.RegisterTool(MCPTool{
                Name:        "memory_forget",
                Description: "Delete a memory",
                InputSchema: map[string]interface{}{
                        "type": "object",
                        "properties": map[string]interface{}{
                                "key": map[string]interface{}{
                                        "type":        "string",
                                        "description": "Memory key to delete",
                                },
                        },
                        "required": []string{"key"},
                },
        }, mcpHandleMemoryForget)
}

// ============================================================
// 工具处理器实现
// ============================================================

func handleExecuteShell(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        command, _ := args["command"].(string)
        if command == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: command is required")},
                        IsError: true,
                }, nil
        }

        timeout := 60
        if t, ok := args["timeout"].(float64); ok {
                timeout = int(t)
        }

        cwd, _ := args["cwd"].(string)

        // 使用内部 shell 执行
        result, err := executeShellCommand(ctx, command, cwd, timeout)
        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error: %v", err))},
                        IsError: true,
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(result)},
        }, nil
}

func handleReadFile(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        path, _ := args["path"].(string)
        if path == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: path is required")},
                        IsError: true,
                }, nil
        }

        // 安全检查：防止路径遍历
        path = filepath.Clean(path)
        if strings.Contains(path, "..") {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: path traversal not allowed")},
                        IsError: true,
                }, nil
        }

        data, err := os.ReadFile(path)
        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error reading file: %v", err))},
                        IsError: true,
                }, nil
        }

        encoding, _ := args["encoding"].(string)
        if encoding == "base64" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(base64.StdEncoding.EncodeToString(data))},
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(string(data))},
        }, nil
}

func handleWriteFile(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        path, _ := args["path"].(string)
        if path == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: path is required")},
                        IsError: true,
                }, nil
        }

        content, _ := args["content"].(string)
        if content == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: content is required")},
                        IsError: true,
                }, nil
        }

        // 安全检查
        path = filepath.Clean(path)

        // 解码内容
        encoding, _ := args["encoding"].(string)
        var data []byte
        if encoding == "base64" {
                decoded, err := base64.StdEncoding.DecodeString(content)
                if err != nil {
                        return CallToolResult{
                                Content: []MCPContent{NewTextContent(fmt.Sprintf("Error decoding base64: %v", err))},
                                IsError: true,
                        }, nil
                }
                data = decoded
        } else {
                data = []byte(content)
        }

        // 确保目录存在
        dir := filepath.Dir(path)
        if err := os.MkdirAll(dir, 0755); err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error creating directory: %v", err))},
                        IsError: true,
                }, nil
        }

        // 写入文件
        append := false
        if a, ok := args["append"].(bool); ok {
                append = a
        }

        var err error
        if append {
                err = os.WriteFile(path, data, 0644)
        } else {
                f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
                if err != nil {
                        return CallToolResult{
                                Content: []MCPContent{NewTextContent(fmt.Sprintf("Error opening file: %v", err))},
                                IsError: true,
                        }, nil
                }
                defer f.Close()
                _, err = f.Write(data)
        }

        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error writing file: %v", err))},
                        IsError: true,
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(fmt.Sprintf("Successfully wrote %d bytes to %s", len(data), path))},
        }, nil
}

func handleListDirectory(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        path, _ := args["path"].(string)
        if path == "" {
                path = "."
        }

        entries, err := os.ReadDir(path)
        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error listing directory: %v", err))},
                        IsError: true,
                }, nil
        }

        var result strings.Builder
        for _, entry := range entries {
                info, err := entry.Info()
                if err != nil {
                        continue
                }
                typeStr := "file"
                if entry.IsDir() {
                        typeStr = "dir"
                }
                result.WriteString(fmt.Sprintf("%s\t%s\t%d\n", entry.Name(), typeStr, info.Size()))
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(result.String())},
        }, nil
}

func handleSearchWeb(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        query, _ := args["query"].(string)
        if query == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: query is required")},
                        IsError: true,
                }, nil
        }

        num := 5
        if n, ok := args["num"].(float64); ok {
                num = int(n)
        }

        // 使用内部搜索功能
        results, err := searchWebInternal(ctx, query, num)
        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error searching: %v", err))},
                        IsError: true,
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(results)},
        }, nil
}

func handleHTTPRequest(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        url, _ := args["url"].(string)
        if url == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: url is required")},
                        IsError: true,
                }, nil
        }

        method, _ := args["method"].(string)
        if method == "" {
                method = "GET"
        }

        body, _ := args["body"].(string)
        headers := make(map[string]string)
        if h, ok := args["headers"].(map[string]interface{}); ok {
                for k, v := range h {
                        if s, ok := v.(string); ok {
                                headers[k] = s
                        }
                }
        }

        // 使用安全 HTTP 客户端
        result, err := makeHTTPRequest(ctx, method, url, body, headers)
        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error: %v", err))},
                        IsError: true,
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(result)},
        }, nil
}

func mcpHandleMemorySave(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        key, _ := args["key"].(string)
        value, _ := args["value"].(string)
        
        if key == "" || value == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: key and value are required")},
                        IsError: true,
                }, nil
        }

        category := MemoryCategoryFact
        if c, ok := args["category"].(string); ok {
                category = MemoryCategory(c)
        }

        var tags []string
        if t, ok := args["tags"].([]interface{}); ok {
                for _, tag := range t {
                        if s, ok := tag.(string); ok {
                                tags = append(tags, s)
                        }
                }
        }

        if globalMemoryManager == nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: memory manager not initialized")},
                        IsError: true,
                }, nil
        }

        _, err := globalMemoryManager.Save(key, value, category, MemoryScopeUser, tags)
        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error saving memory: %v", err))},
                        IsError: true,
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(fmt.Sprintf("Memory saved: %s", key))},
        }, nil
}

func mcpHandleMemoryRecall(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        query, _ := args["query"].(string)
        
        limit := 10
        if l, ok := args["limit"].(float64); ok {
                limit = int(l)
        }

        var category MemoryCategory
        if c, ok := args["category"].(string); ok {
                category = MemoryCategory(c)
        }

        if globalMemoryManager == nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: memory manager not initialized")},
                        IsError: true,
                }, nil
        }

        memories := globalMemoryManager.Recall(query, category, limit)

        var result strings.Builder
        for _, m := range memories {
                result.WriteString(fmt.Sprintf("- %s: %s\n", m.Key, m.Value))
        }

        if result.Len() == 0 {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("No memories found")},
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(result.String())},
        }, nil
}

func mcpHandleMemoryList(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        var category MemoryCategory
        if c, ok := args["category"].(string); ok {
                category = MemoryCategory(c)
        }

        if globalMemoryManager == nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: memory manager not initialized")},
                        IsError: true,
                }, nil
        }

        memories := globalMemoryManager.List(category, "")

        var result strings.Builder
        for _, m := range memories {
                result.WriteString(fmt.Sprintf("- %s [%s]: %s\n", m.Key, m.Category, m.Value))
        }

        if result.Len() == 0 {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("No memories")},
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(result.String())},
        }, nil
}

func mcpHandleMemoryForget(ctx context.Context, args map[string]interface{}) (CallToolResult, error) {
        key, _ := args["key"].(string)
        if key == "" {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: key is required")},
                        IsError: true,
                }, nil
        }

        if globalMemoryManager == nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent("Error: memory manager not initialized")},
                        IsError: true,
                }, nil
        }

        err := globalMemoryManager.Forget(key)
        if err != nil {
                return CallToolResult{
                        Content: []MCPContent{NewTextContent(fmt.Sprintf("Error: %v", err))},
                        IsError: true,
                }, nil
        }

        return CallToolResult{
                Content: []MCPContent{NewTextContent(fmt.Sprintf("Memory forgotten: %s", key))},
        }, nil
}

// ============================================================
// 辅助函数
// ============================================================

func executeShellCommand(ctx context.Context, command, cwd string, timeout int) (string, error) {
        // 创建超时上下文
        ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
        defer cancel()

        // 使用现有的 shell 执行逻辑
        result := runShell(ctx, command)
        if result.ExitCode != 0 {
                return result.Stdout + result.Stderr, fmt.Errorf("command exited with code %d", result.ExitCode)
        }
        return result.Stdout + result.Stderr, nil
}

func searchWebInternal(ctx context.Context, query string, num int) (string, error) {
        // 简化实现：返回提示信息
        // 实际使用时可以集成搜索 API
        return fmt.Sprintf("Web search for: %s\n(Implement with search API)", query), nil
}

func makeHTTPRequest(ctx context.Context, method, url, body string, headers map[string]string) (string, error) {
        // SSRF 检查
        if err := ValidateURLForFetch(url); err != nil {
                return "", err
        }

        // 使用安全 HTTP 客户端
        if method == "GET" {
                resp, err := SafeHTTPGet(ctx, url)
                if err != nil {
                        return "", err
                }
                defer resp.Body.Close()
                
                // 读取响应
                data := make([]byte, 1024*1024) // 限制 1MB
                n, _ := resp.Body.Read(data)
                return string(data[:n]), nil
        }

        // POST 请求
        resp, err := SafeHTTPPost(ctx, url, strings.NewReader(body), "application/json")
        if err != nil {
                return "", err
        }
        defer resp.Body.Close()

        data := make([]byte, 1024*1024)
        n, _ := resp.Body.Read(data)
        return string(data[:n]), nil
}
