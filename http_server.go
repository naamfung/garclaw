package main

import (
        "context"
        "encoding/json"
        "fmt"
        "io"
        "log"
        "net/http"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "time"

        "github.com/google/uuid"
        "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool { return true },
}

// 全局会话管理器
var globalWebSessionManager *WebSessionManager

// 全局上传目录
var globalUploadDir string

// HTTPServer 管理 HTTP 和 WebSocket 服务
type HTTPServer struct {
        addr   string
        server *http.Server
}

// NewHTTPServer 创建 HTTP 服务器实例
func NewHTTPServer(addr string) *HTTPServer {
        // 初始化会话管理器
        globalWebSessionManager = NewWebSessionManager()

        // 初始化上传目录
        execPath, err := os.Executable()
        if err != nil {
                log.Printf("Warning: cannot get executable path: %v", err)
                execPath = "."
        }
        execDir := filepath.Dir(execPath)
        globalUploadDir = filepath.Join(execDir, "uploads")

        // 确保上传目录存在
        if err := os.MkdirAll(globalUploadDir, 0755); err != nil {
                log.Printf("Warning: failed to create uploads directory: %v", err)
        } else {
                log.Printf("Upload directory: %s", globalUploadDir)
        }

        return &HTTPServer{
                addr: addr,
        }
}

// Start 启动 HTTP 服务器
func (s *HTTPServer) Start() {
        // 初始化认证管理器
        if globalAuthConfig.Enabled {
                globalAuthManager = NewAuthManager(&globalAuthConfig)
                log.Printf("Authentication enabled. Sessions will expire after %d hours.", globalAuthConfig.TokenExpiry)
        }

        mux := http.NewServeMux()

        // 登录相关路由（无需认证）
        mux.HandleFunc("/login", HandleLoginPage)
        mux.HandleFunc("/login/submit", HandleLogin)
        mux.HandleFunc("/logout", HandleLogout)
        mux.HandleFunc("/api/login", HandleAPILogin)

        // 需要认证的路由
        mux.HandleFunc("/", AuthMiddleware(s.indexHandler))
        mux.HandleFunc("/ws", AuthMiddleware(s.wsHandler))
        mux.HandleFunc("/props", AuthMiddleware(s.propsHandler))
        mux.HandleFunc("/v1/models", AuthMiddleware(s.modelsHandler))
        mux.HandleFunc("/upload", AuthMiddleware(s.uploadHandler))
        mux.HandleFunc("/file/", AuthMiddleware(s.fileHandler))

        // API 路由：配置管理
        mux.HandleFunc("/api/config", AuthMiddleware(s.configHandler))
        mux.HandleFunc("/api/models", AuthMiddleware(s.modelsAPIHandler))
        mux.HandleFunc("/api/models/", AuthMiddleware(s.modelDetailHandler))

        // API 路由：角色管理
        mux.HandleFunc("/api/roles", AuthMiddleware(s.rolesHandler))
        mux.HandleFunc("/api/roles/", AuthMiddleware(s.roleDetailHandler))

        // API 路由：技能管理
        mux.HandleFunc("/api/skills", AuthMiddleware(s.skillsHandler))
        mux.HandleFunc("/api/skills/", AuthMiddleware(s.skillDetailHandler))

        // API 路由：演员管理
        mux.HandleFunc("/api/actors", AuthMiddleware(s.actorsHandler))
        mux.HandleFunc("/api/actors/", AuthMiddleware(s.actorDetailHandler))

        // API 路由：Hooks 管理
        mux.HandleFunc("/api/hooks", AuthMiddleware(s.hooksHandler))
        mux.HandleFunc("/api/hooks/", AuthMiddleware(s.hookDetailHandler))

        // MCP 路由（如果启用）
        if globalMCPServer != nil {
                mux.HandleFunc("/mcp", AuthMiddleware(globalMCPServer.HandleHTTP))
                mux.HandleFunc("/mcp/sse", AuthMiddleware(globalMCPServer.HandleSSE))
                mux.HandleFunc("/mcp/message", AuthMiddleware(globalMCPServer.HandleSSEMessage))
                log.Println("MCP endpoints enabled: /mcp, /mcp/sse")
        }

        s.server = &http.Server{
                Addr:         s.addr,
                Handler:      mux,
                ReadTimeout:  60 * time.Second,  // 增加读取超时，支持大文件上传
                WriteTimeout: 60 * time.Second,
        }

        log.Printf("HTTP server listening on %s", s.addr)
        if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                log.Fatalf("HTTP server error: %v", err)
        }
}

// Stop 关闭服务器
func (s *HTTPServer) Stop() error {
        return s.server.Close()
}

// indexHandler 提供静态聊天页面
func (s *HTTPServer) indexHandler(w http.ResponseWriter, r *http.Request) {
        // Use embedded index.html from llama.cpp webui
        html := GetIndexHTML()
        if html == "" {
                // Fallback to simple HTML if embed failed
                html = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>GarClaw Chat</title>
    <link rel="icon" href="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjU2IiB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIGhlaWdodD0iMjU2IiBpZD0ic2NyZWVuc2hvdC1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyOTkwMGVkYmYiIHZpZXdCb3g9IjAgMCAyNTYgMjU2IiB4bWxuczp4bGluaz0iaHR0cDovL3d3dy53My5vcmcvMTk5OS94bGluayIgZmlsbD0ibm9uZSIgdmVyc2lvbj0iMS4xIj48ZyBpZD0ic2hhcGUtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0Mjk5MDBlZGJmIiByeD0iMCIgcnk9IjAiPjxnIGlkPSJzaGFwZS1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyMTU3NTVjM2EiPjxnIGNsYXNzPSJmaWxscyIgaWQ9ImZpbGxzLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIxNTc1NWMzYSI+PHJlY3Qgcng9IjAiIHJ5PSIwIiB4PSIwIiB5PSIwIiB0cmFuc2Zvcm09Im1hdHJpeCgxLjAwMDAwMCwgMC4wMDAwMDAsIDAuMDAwMDAwLCAxLjAwMDAwMCwgMC4wMDAwMDAsIDAuMDAwMDAwKSIgd2lkdGg9IjI1NiIgaGVpZ2h0PSIyNTYiIHN0eWxlPSJmaWxsOiByZ2IoMjcsIDMxLCAzMik7IGZpbGwtb3BhY2l0eTogMTsiLz48L2c+PC9nPjxnIGlkPSJzaGFwZS1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyMjM2M2VmM2YiIHJ4PSIwIiByeT0iMCI+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MCI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQwIj48cGF0aCBkPSJNMTcxLjY2NTAwODU0NDkyMTg4LDk5LjUzMDI1MDU0OTMxNjRMMTU5Ljc5OTUzMDAyOTI5Njg4LDEyMC42MjQ2ODcxOTQ4MjQyMkMxNDQuMTU0NTEwNDk4MDQ2ODgsMTA4LjU4MzI5MDEwMDA5NzY2LDEyMC45NTA0MTY1NjQ5NDE0LDEwNi44MjU0MTY1NjQ5NDE0LDEwNS4zMDUzOTcwMzM2OTE0LDExOS43NDU3NTA0MjcyNDYxQzgwLjA3OTgxMTA5NjE5MTQsMTQwLjU3NjUyMjgyNzE0ODQ0LDgxLjgzNzYyMzU5NjE5MTQsMTg4Ljc0MjI2Mzc5Mzk0NTMsMTIxLjEyNjE5NzgxNDk0MTQsMTg5LjAwNTg3NDYzMzc4OTA2QzEzMi4xMTMwMDY1OTE3OTY4OCwxODkuMDA1ODc0NjMzNzg5MDYsMTQxLjQyOTY1Njk4MjQyMTg4LDE4My44MjAxMTQxMzU3NDIyLDE1MS40NDk2NzY1MTM2NzE4OCwxODAuMzkyMzQ5MjQzMTY0MDZMMTU2LjcyMzM1ODE1NDI5Njg4LDIwMS4zOTg4NDk0ODczMDQ3QzE0Ny44NDU5MTY3NDgwNDY4OCwyMDUuNTI5ODkxOTY3NzczNDQsMTM4Ljc5MjkzODIzMjQyMTg4LDIwOS43NDg3MzM1MjA1MDc4LDEyOS4wMzY4MzQ3MTY3OTY4OCwyMTEuMDY3MTIzNDEzMDg1OTRDNDAuMDg4MzUyMjAzMzY5MTQsMjIzLjE5NjQ1NjkwOTE3OTcsNDUuMTg2MDA4NDUzMzY5MTQsOTQuNzg0MDA0MjExNDI1NzgsMTI1LjYwODg2MzgzMDU2NjQsODguMTA0MDcyNTcwODAwNzhDMTQyLjQ4NDM0NDQ4MjQyMTg4LDg2LjY5NzgyMjU3MDgwMDc4LDE1Ny4zMzgzNDgzODg2NzE4OCw5MS4wOTI0NzU4OTExMTMyOCwxNzEuNzUzMTQzMzEwNTQ2ODgsOTkuNTMwMjUwNTQ5MzE2NFoiIGNsYXNzPSJzdDAiIHN0eWxlPSJmaWxsOiByZ2IoMjU1LCAxMzAsIDU0KTsgZmlsbC1vcGFjaXR5OiAxOyIvPjwvZz48L2c+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MSI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQxIj48cGF0aCBkPSJNMTEwLjIyNzI3MjAzMzY5MTQsNzkuMzE0NzA0ODk1MDE5NTNDOTYuNjkxODcxNjQzMDY2NCw4My4zNTc4NTY3NTA0ODgyOCw4NC4xMjMyNjgxMjc0NDE0LDkwLjgyODgzNDUzMzY5MTQsNzQuNjMwNTkyMzQ2MTkxNCwxMDEuMjg4MTI0MDg0NDcyNjZDNzIuODcyNzc5ODQ2MTkxNCw4MC4wMTc4Mjk4OTUwMTk1Myw3Ny42MTg4NzM1OTYxOTE0LDM3LjAzNzkzNzE2NDMwNjY0LDEwMS4yNjIxODQxNDMwNjY0LDI4LjYwMDEwMzM3ODI5NTlDMTA0Ljc3ODA1MzI4MzY5MTQsMjcuMzY5NjQ5ODg3MDg0OTYsMTE2LjgxOTU1NzE4OTk0MTQsMjQuMjkzMzcxMjAwNTYxNTIzLDExNi40Njc5OTQ2ODk5NDE0LDMwLjUzMzc4ODY4MTAzMDI3M0MxMTYuMTE2MTg4MDQ5MzE2NCwzNi43NzQyNjUyODkzMDY2NCwxMDcuNzY2MzM0NTMzNjkxNCw0Ny40OTcyMjY3MTUwODc4OSwxMDUuNzQ1MDk0Mjk5MzE2NCw1My4yOTgyMzY4NDY5MjM4M0MxMDIuMjI5MjI1MTU4NjkxNCw2My40OTM4Njk3ODE0OTQxNCwxMDUuNDgxMTc4MjgzNjkxNCw3MC41MjUzNTI0NzgwMjczNCwxMTAuMzE1NDA2Nzk5MzE2NCw3OS40MDI2NTY1NTUxNzU3OFoiIGNsYXNzPSJzdDAiIHN0eWxlPSJmaWxsOiByZ2IoMjU1LCAxMzAsIDU0KTsgZmlsbC1vcGFjaXR5OiAxOyIvPjwvZz48L2c+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MiI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQyIj48cGF0aCBkPSJNMTQzLjYyNjkyMjYwNzQyMTg4LDEyNy42NTYyMTE4NTMwMjczNEwxNDMuNjI2OTIyNjA3NDIxODgsMTQzLjQ3NzA2NjA0MDAzOTA2TDE1Ny42ODk5MTA4ODg2NzE4OCwxNDMuNDc3MDY2MDQwMDM5MDZMMTU3LjY4OTkxMDg4ODY3MTg4LDE1NS43ODIxODA3ODYxMzI4TDE0My42MjY5MjI2MDc0MjE4OCwxNTUuNzgyMTgwNzg2MTMyOEwxNDMuNjI2OTIyNjA3NDIxODgsMTcwLjcyNDA3NTMxNzM4MjhMMTMwLjQ0Mjg0MDU3NjE3MTg4LDE3MC43MjQwNzUzMTczODI4TDEzMC40NDI4NDA1NzYxNzE4OCwxNTUuNzgyMTgwNzg2MTMyOEwxMTUuNTAwOTUzNjc0MzE2NCwxNTUuNzgyMTgwNzg2MTMyOEwxMTUuNTAwOTUzNjc0MzE2NCwxNDMuNDc3MDY2MDQwMDM5MDZMMTI5LjEyNDQ4MTIwMTE3MTg4LDE0My40NzcwNjYwNDAwMzkwNkwxMzAuNDQyODQwNTc2MTcxODgsMTQyLjE1ODY3NjE0NzQ2MDk0TDEzMC40NDI4NDA1NzYxNzE4OCwxMjcuNjU2MjExODUzMDI3MzRMMTQzLjYyNjkyMjYwNzQyMTg4LDEyNy42NTYyMTE4NTMwMjczNFoiIGNsYXNzPSJzdDAiIHN0eWxlPSJmaWxsOiByZ2IoMjU1LCAxMzAsIDU0KTsgZmlsbC1vcGFjaXR5OiAxOyIvPjwvZz48L2c+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MyI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQzIj48cGF0aCBkPSJNMTkxLjk2ODIzMTIwMTE3MTg4LDEyNy42NTYyMTE4NTMwMjczNEwxOTEuOTY4MjMxMjAxMTcxODgsMTQyLjE1ODY3NjE0NzQ2MDk0TDE5My4yODY4MzQ3MTY3OTY4OCwxNDMuNDc3MDY2MDQwMDM5MDZMMjA2LjkxMDM2OThT73A0Njg4LDE0My40NzcwNjYwNDAwMzkwNkwyMDYuOTEwMzY5ODczMDQ2ODgsMTU1Ljc4MjE4MDc4NjEzMjhMMTkxLjk2ODIzMTIwMTE3MTg4LDE1NS43ODIxODA3ODYxMzI4TDE5MS45NjgyMzEyMDExNzE4OCwxNzAuNzI0MDc1MzE3MzgyOEwxNzguNzg0MzkzMzEwNTQ2ODgsMTcwLjcyNDA3NTMxNzM4MjhMMTc4Ljc4NDM5MzMxMDU0Njg4LDE1NS43ODIxODA3ODYxMzI4TDE2NC43MjE0MDUwMjkzOTY4OCwxNTUuNzgyMTgwNzg2MTMyOEwxNjQuNzIxNDA1MDI5Mjk2ODgsMTQzLjQ3NzA2NjA0MDAzOTA2TDE3OC43ODQzOTMzMTA1NDY4OCwxNDMuNDc3MDY2MDQwMDM5MDZMMTc4Ljc4NDM5MzMxMDU0Njg4LDEyNy42NTYyMTE4NTMwMjczNEwxOTEuOTY4MjMxMjAxMTcxODgsMTI3LjY1NjIxMTg1MzAyNzM0WiIgY2xhc3M9InN0MCIgc3R5bGU9ImZpbGw6IHJnYigyNTUsIDEzMCwgNTQpOyBmaWxsLW9wYWNpdHk6IDE7Ii8+PC9nPjwvZz48ZyBpZD0ic2hhcGUtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQ0Ij48ZyBjbGFzcz0iZmlsbHMiIGlkPSJmaWxscy1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyMjM2M2VmNDQiPjxwYXRoIGQ9Ik0xNTMuMjA3NDg5MDEzNjcxODgsMzguMDkyNjU1MTgxODg0NzY2QzE1NC45NjU1NDU2NTQyOTY4OCw0MC43Mjk0NjU0ODQ2MTkxNCwxNDUuMDMzNDE2NzQ4MDQ2ODgsNTIuMDY3NzA3MDYxNzY3NTgsMTQzLjQ1MTE0MTM1NzQyMTg4LDU0Ljk2ODE3Mzk4MDcxMjg5QzEzOC44ODA4Mjg4NTc0MjE4OCw2My41ODE3OTA5MjQwNzIyNjYsMTQxLjk1NzAwMDczMjQyMTg4LDY4LjUwMzgyMjMyNjY2MDE2LDE0NS4zODQ3MzUxMDc0MjE4OCw3Ni42Nzc5MjUxMDk4NjMyOEMxMzUuNDUyODUwMzQxNzk2ODgsNzUuMTgzNzIzNDQ5NzA3MDMsMTI2LjIyNDA5ODIwNTU2NjQsNzYuNDE0MjUzMjM0ODYzMjgsMTE2LjM3OTg1OTkyNDMxNjQsNzcuNTU2ODMxMzU5ODYzMjhDMTE4LjU3NzM2OTY4OTk0MTQsNTguNjU5NzMyODE4NjAzNTE2LDEyOS4yMTI2MTU5NjY3OTY4OCwzMS4xNDkwNTM1NzM2MDg0LDE1My4yMDc0ODkwMTM2NzE4OCwzOC4wOTI2NTUxODE4ODQ3NjZaIiBjbGFzcz0ic3QwIiBzdHlsZT0iZmlsbDogcmdiKDI1NSwgMTMwLCA1NCk7IGZpbGwtb3BhY2l0eTogMTsiLz48L2c+PC9nPjwvZz48L2c+PC9zdmc+" />
    <style>
        :root {
            --bg-primary: #1b1f20;
            --bg-secondary: #242829;
            --bg-tertiary: #2d3132;
            --text-primary: #e8eaeb;
            --text-secondary: #9ca3af;
            --text-muted: #6b7280;
            --accent: #ff8236;
            --accent-hover: #ff9a57;
            --border: #3a3f40;
            --user-bg: #2d4a6d;
            --assistant-bg: #2d3132;
            --reasoning-bg: #1e2526;
            --error: #ef4444;
            --success: #22c55e;
            --warning: #f59e0b;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        html, body { height: 100%; overflow: hidden; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background-color: var(--bg-primary);
            color: var(--text-primary);
        }
        .container {
            max-width: 900px;
            margin: 0 auto;
            width: 100%;
            height: 100%;
            display: flex;
            flex-direction: column;
            padding: 0 16px;
        }
        header {
            padding: 16px 0;
            border-bottom: 1px solid var(--border);
            flex-shrink: 0;
        }
        .header-row {
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .logo { display: flex; align-items: center; gap: 12px; }
        .logo svg { width: 40px; height: 40px; }
        .logo h1 { font-size: 1.75rem; font-weight: 600; color: var(--text-primary); letter-spacing: -0.02em; }
        .logo span { color: var(--accent); }
        .header-actions {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        .session-info {
            font-size: 0.75rem;
            color: var(--text-muted);
            background: var(--bg-tertiary);
            padding: 4px 8px;
            border-radius: 4px;
        }
        .header-btn {
            background: var(--bg-tertiary);
            border: 1px solid var(--border);
            color: var(--text-secondary);
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 0.8rem;
            cursor: pointer;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            gap: 6px;
        }
        .header-btn:hover {
            background: var(--bg-secondary);
            color: var(--text-primary);
            border-color: var(--accent);
        }
        .header-btn svg {
            width: 14px;
            height: 14px;
        }
        .status {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 8px 0;
            font-size: 0.875em;
            color: var(--text-muted);
            flex-shrink: 0;
        }
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--success);
            animation: pulse 2s infinite;
        }
        .status-dot.disconnected { background: var(--error); animation: none; }
        .status-dot.working { background: var(--warning); }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
        #messages {
            flex: 1;
            overflow-y: auto;
            padding: 16px 0;
            display: flex;
            flex-direction: column;
            gap: 16px;
            min-height: 0;
        }
        .message {
            padding: 14px 18px;
            border-radius: 16px;
            max-width: 85%;
            word-wrap: break-word;
            overflow-wrap: break-word;
            line-height: 1.6;
            animation: fadeIn 0.3s ease;
        }
        @keyframes fadeIn { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }
        .user {
            background: var(--user-bg);
            color: var(--text-primary);
            align-self: flex-end;
            border-bottom-right-radius: 4px;
        }
        .assistant {
            background: var(--assistant-bg);
            color: var(--text-primary);
            align-self: flex-start;
            border: 1px solid var(--border);
            border-bottom-left-radius: 4px;
            white-space: pre-wrap;
        }
        .error {
            background: rgba(239, 68, 68, 0.15);
            color: var(--error);
            border: 1px solid rgba(239, 68, 68, 0.3);
            align-self: flex-start;
        }
        .system {
            background: transparent;
            color: var(--text-muted);
            font-style: italic;
            font-size: 0.875em;
            align-self: center;
            padding: 8px 16px;
        }
        .reasoning {
            background: var(--reasoning-bg);
            color: var(--text-secondary);
            font-style: italic;
            align-self: flex-start;
            border-left: 3px solid var(--accent);
            border-radius: 0 12px 12px 0;
            padding-left: 16px;
            white-space: pre-wrap;
            font-size: 0.9em;
        }
        .reasoning-label {
            color: var(--accent);
            font-weight: 500;
            margin-bottom: 6px;
            display: block;
        }
        .input-container {
            padding: 16px 0 24px;
            border-top: 1px solid var(--border);
            background: var(--bg-primary);
            flex-shrink: 0;
        }
        .input-wrapper { display: flex; gap: 12px; align-items: flex-end; }
        #input {
            flex: 1;
            padding: 14px 18px;
            border: 1px solid var(--border);
            border-radius: 24px;
            background: var(--bg-secondary);
            color: var(--text-primary);
            font-size: 1rem;
            outline: none;
            transition: all 0.2s ease;
            resize: none;
            min-height: 50px;
            max-height: 150px;
        }
        #input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px rgba(255, 130, 54, 0.15); }
        #input::placeholder { color: var(--text-muted); }
        .send-btn {
            width: 50px;
            height: 50px;
            border-radius: 50%;
            background: var(--accent);
            border: none;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: all 0.2s ease;
            flex-shrink: 0;
        }
        .send-btn:hover { background: var(--accent-hover); transform: scale(1.05); }
        .send-btn:active { transform: scale(0.95); }
        .send-btn svg { width: 22px; height: 22px; color: white; }
        .helper-text { font-size: 0.8em; color: var(--text-muted); margin-top: 8px; }
        .helper-text kbd {
            background: var(--bg-tertiary);
            padding: 2px 6px;
            border-radius: 4px;
            font-family: inherit;
            border: 1px solid var(--border);
        }
        /* 确认对话框 */
        .modal-overlay {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.6);
            z-index: 1000;
            justify-content: center;
            align-items: center;
        }
        .modal-overlay.show {
            display: flex;
        }
        .modal {
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 12px;
            padding: 24px;
            max-width: 400px;
            text-align: center;
        }
        .modal h3 {
            margin-bottom: 12px;
            color: var(--text-primary);
        }
        .modal p {
            color: var(--text-secondary);
            margin-bottom: 20px;
        }
        .modal-btns {
            display: flex;
            gap: 12px;
            justify-content: center;
        }
        .modal-btn {
            padding: 10px 24px;
            border-radius: 8px;
            font-size: 0.9rem;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .modal-btn.cancel {
            background: var(--bg-tertiary);
            border: 1px solid var(--border);
            color: var(--text-secondary);
        }
        .modal-btn.cancel:hover {
            background: var(--bg-primary);
            color: var(--text-primary);
        }
        .modal-btn.confirm {
            background: var(--error);
            border: none;
            color: white;
        }
        .modal-btn.confirm:hover {
            background: #dc2626;
        }
        ::-webkit-scrollbar { width: 8px; }
        ::-webkit-scrollbar-track { background: transparent; }
        ::-webkit-scrollbar-thumb { background: var(--border); border-radius: 4px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--text-muted); }
        @media (max-width: 640px) {
            .container { padding: 0 12px; }
            .message { max-width: 90%; padding: 12px 14px; }
            .logo h1 { font-size: 1.5rem; }
            .header-btn span { display: none; }
            .header-btn { padding: 8px; }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="header-row">
                <div class="logo">
                    <h1>Gar<span>Claw</span></h1>
                </div>
                <div class="header-actions">
                    <div class="session-info" id="sessionInfo">Session: --</div>
                    <button class="header-btn" id="newChatBtn" title="新建對話">
                        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                            <polyline points="14 2 14 8 20 8"></polyline>
                            <line x1="12" y1="18" x2="12" y2="12"></line>
                            <line x1="9" y1="15" x2="15" y2="15"></line>
                        </svg>
                        <span>新建</span>
                    </button>
                    <button class="header-btn" id="clearBtn" title="清除歷史">
                        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <polyline points="3 6 5 6 21 6"></polyline>
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                        </svg>
                        <span>清除</span>
                    </button>
                </div>
            </div>
        </header>
        <div class="status">
            <div class="status-dot" id="statusDot"></div>
            <span id="statusText">已連線</span>
        </div>
        <div id="messages"></div>
        <div class="input-container">
            <div class="input-wrapper">
                <input type="text" id="input" placeholder="輸入訊息..." autocomplete="off">
                <button class="send-btn" id="sendBtn" title="發送訊息">
                    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <line x1="22" y1="2" x2="11" y2="13"></line>
                        <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
                    </svg>
                </button>
            </div>
            <p class="helper-text">按 <kbd>Enter</kbd> 發送 · 輸入 <kbd>/help</kbd> 查看命令 · 關閉瀏覽器後任務繼續執行 · 聊天記錄自動保存</p>
        </div>
    </div>
    
    <!-- 确认对话框 -->
    <div class="modal-overlay" id="modalOverlay">
        <div class="modal">
            <h3 id="modalTitle">確認操作</h3>
            <p id="modalMessage">確定要執行此操作嗎？</p>
            <div class="modal-btns">
                <button class="modal-btn cancel" id="modalCancel">取消</button>
                <button class="modal-btn confirm" id="modalConfirm">確定</button>
            </div>
        </div>
    </div>
    
    <script>
        const messagesDiv = document.getElementById('messages');
        const input = document.getElementById('input');
        const sendBtn = document.getElementById('sendBtn');
        const statusDot = document.getElementById('statusDot');
        const statusText = document.getElementById('statusText');
        const sessionInfo = document.getElementById('sessionInfo');
        const newChatBtn = document.getElementById('newChatBtn');
        const clearBtn = document.getElementById('clearBtn');
        const modalOverlay = document.getElementById('modalOverlay');
        const modalTitle = document.getElementById('modalTitle');
        const modalMessage = document.getElementById('modalMessage');
        const modalCancel = document.getElementById('modalCancel');
        const modalConfirm = document.getElementById('modalConfirm');

        let currentAssistantEl = null;
        let currentReasoningEl = null;
        let autoScroll = true;
        let sessionId = localStorage.getItem('garclaw_session_id') || '';
        let ws = null;
        
        // ===== 聊天历史持久化 =====
        const STORAGE_KEY = 'garclaw_chat_history';
        const MAX_HISTORY_SIZE = 100; // 最多保存100条消息
        
        // 保存消息到 localStorage
        function saveMessageToStorage(role, content) {
            try {
                let history = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
                history.push({
                    role: role,
                    content: content,
                    timestamp: Date.now()
                });
                // 限制历史记录大小
                if (history.length > MAX_HISTORY_SIZE) {
                    history = history.slice(-MAX_HISTORY_SIZE);
                }
                localStorage.setItem(STORAGE_KEY, JSON.stringify(history));
            } catch (e) {
                console.error('Failed to save message to localStorage:', e);
            }
        }
        
        // 从 localStorage 加载历史
        function loadHistoryFromStorage() {
            try {
                const history = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
                return history;
            } catch (e) {
                console.error('Failed to load history from localStorage:', e);
                return [];
            }
        }
        
        // 清除存储的历史
        function clearHistoryStorage() {
            localStorage.removeItem(STORAGE_KEY);
        }
        
        // 渲染历史消息到界面
        function renderHistory() {
            const history = loadHistoryFromStorage();
            messagesDiv.innerHTML = '';
            
            history.forEach(msg => {
                const div = document.createElement('div');
                div.className = 'message ' + msg.role;
                div.textContent = msg.content;
                messagesDiv.appendChild(div);
            });
            
            if (history.length > 0) {
                scrollToBottom();
            }
        }
        
        // ===== 模态对话框 =====
        let modalCallback = null;
        
        function showModal(title, message, onConfirm) {
            modalTitle.textContent = title;
            modalMessage.textContent = message;
            modalCallback = onConfirm;
            modalOverlay.classList.add('show');
        }
        
        function hideModal() {
            modalOverlay.classList.remove('show');
            modalCallback = null;
        }
        
        modalCancel.addEventListener('click', hideModal);
        modalConfirm.addEventListener('click', () => {
            if (modalCallback) {
                modalCallback();
            }
            hideModal();
        });
        
        modalOverlay.addEventListener('click', (e) => {
            if (e.target === modalOverlay) {
                hideModal();
            }
        });
        
        // ===== 按钮事件 =====
        newChatBtn.addEventListener('click', () => {
            showModal('新建對話', '確定要開始新對話嗎？當前對話記錄將被清除。', () => {
                clearHistoryStorage();
                sessionId = '';
                localStorage.removeItem('garclaw_session_id');
                messagesDiv.innerHTML = '';
                sessionInfo.textContent = 'Session: --';
                // 重新连接（不带 session 参数）
                if (ws) {
                    ws.close();
                }
            });
        });
        
        clearBtn.addEventListener('click', () => {
            showModal('清除歷史', '確定要清除所有聊天記錄嗎？此操作不可恢復。', () => {
                clearHistoryStorage();
                messagesDiv.innerHTML = '';
            });
        });
        
        // ===== 滚动控制 =====
        function isNearBottom() {
            const scrollTop = messagesDiv.scrollTop;
            const clientHeight = messagesDiv.clientHeight;
            const scrollHeight = messagesDiv.scrollHeight;
            return scrollHeight - scrollTop - clientHeight <= 100;
        }

        function scrollToBottom(smooth = false) {
            if (autoScroll) {
                if (smooth) {
                    messagesDiv.scrollTo({ top: messagesDiv.scrollHeight, behavior: 'smooth' });
                } else {
                    messagesDiv.scrollTop = messagesDiv.scrollHeight;
                }
            }
        }

        messagesDiv.addEventListener('scroll', function() {
            autoScroll = isNearBottom();
        });

        function setConnected(connected, working = false) {
            if (connected) {
                statusDot.classList.remove('disconnected');
                if (working) {
                    statusDot.classList.add('working');
                    statusText.textContent = '任務執行中...';
                } else {
                    statusDot.classList.remove('working');
                    statusText.textContent = '已連線';
                }
            } else {
                statusDot.classList.add('disconnected');
                statusDot.classList.remove('working');
                statusText.textContent = '未連線';
            }
        }

        function finishCurrentResponse() {
            currentAssistantEl = null;
            currentReasoningEl = null;
        }

        function getOrCreateAssistantEl() {
            if (!currentAssistantEl) {
                currentAssistantEl = document.createElement('div');
                currentAssistantEl.className = 'message assistant';
                messagesDiv.appendChild(currentAssistantEl);
            }
            return currentAssistantEl;
        }

        function getOrCreateReasoningEl() {
            if (!currentReasoningEl) {
                currentReasoningEl = document.createElement('div');
                currentReasoningEl.className = 'message reasoning';
                currentReasoningEl.innerHTML = '<span class="reasoning-label">推理過程</span>';
                if (currentAssistantEl) {
                    messagesDiv.insertBefore(currentReasoningEl, currentAssistantEl);
                } else {
                    messagesDiv.appendChild(currentReasoningEl);
                }
            }
            return currentReasoningEl;
        }

        function appendMessage(text, className, saveToStorage = true) {
            if (className === 'assistant' && currentAssistantEl) {
                currentAssistantEl.textContent += text;
            } else if (className === 'reasoning' && currentReasoningEl) {
                currentReasoningEl.appendChild(document.createTextNode(text));
            } else {
                const div = document.createElement('div');
                div.className = 'message ' + className;
                div.textContent = text;
                messagesDiv.appendChild(div);
            }
            
            // 保存用户消息和完成的助手消息到 localStorage
            if (saveToStorage && (className === 'user' || className === 'assistant')) {
                if (className === 'user') {
                    saveMessageToStorage(className, text);
                }
            }
            
            scrollToBottom();
        }
        
        // 保存完整的助手消息
        function saveAssistantMessage(content) {
            if (content && content.trim()) {
                saveMessageToStorage('assistant', content.trim());
            }
        }

        function connect() {
            const wsUrl = sessionId 
                ? "ws://" + location.host + "/ws?session=" + sessionId
                : "ws://" + location.host + "/ws";
            ws = new WebSocket(wsUrl);

            ws.onmessage = function(event) {
                const chunk = JSON.parse(event.data);
                
                if (chunk.session_id) {
                    sessionId = chunk.session_id;
                    localStorage.setItem('garclaw_session_id', sessionId);
                    sessionInfo.textContent = 'Session: ' + sessionId;
                }
                
                if (chunk.task_running !== undefined) {
                    setConnected(true, chunk.task_running);
                }

                // 处理历史同步（重连时发送错过的新消息）
                if (chunk.history_sync && chunk.history_sync.length > 0) {
                    appendMessage("同步 " + chunk.history_sync.length + " 條新消息", 'system', false);
                    chunk.history_sync.forEach(function(msg) {
                        // 显示消息
                        const div = document.createElement('div');
                        div.className = 'message ' + msg.role;
                        div.textContent = msg.content;
                        messagesDiv.appendChild(div);
                        // 保存到本地存储
                        saveMessageToStorage(msg.role, msg.content);
                    });
                    scrollToBottom();
                    // 注意：不要 return，继续处理可能的 task_running 状态
                }

                if (chunk.error) {
                    finishCurrentResponse();
                    appendMessage("錯誤: " + chunk.error, 'error', false);
                    return;
                }

                if (chunk.content) {
                    const el = getOrCreateAssistantEl();
                    el.textContent += chunk.content;
                    scrollToBottom(true);
                }

                if (chunk.reasoning_content) {
                    const el = getOrCreateReasoningEl();
                    el.appendChild(document.createTextNode(chunk.reasoning_content));
                    scrollToBottom(true);
                }

                if (chunk.tool_calls && chunk.tool_calls.length > 0) {
                    const el = getOrCreateAssistantEl();
                    el.textContent += "\n[工具調用: " + JSON.stringify(chunk.tool_calls) + "]\n";
                    scrollToBottom(true);
                }

                if (chunk.done) {
                    // 保存完整的助手消息
                    if (currentAssistantEl && currentAssistantEl.textContent) {
                        saveAssistantMessage(currentAssistantEl.textContent);
                    }
                    appendMessage("回應完成", 'system', false);
                    finishCurrentResponse();
                    setConnected(true, false);
                }
            };

            ws.onopen = function() {
                setConnected(true);
                // 仅在首次连接且没有历史时显示消息
                const history = loadHistoryFromStorage();
                if (history.length === 0) {
                    appendMessage("已連線，開始新對話" + (sessionId ? " (Session: " + sessionId + ")" : ""), 'system', false);
                } else {
                    appendMessage("已連線，會話已恢復" + (sessionId ? " (" + sessionId + ")" : ""), 'system', false);
                }
            };

            ws.onerror = function(error) {
                console.error("WebSocket error:", error);
                finishCurrentResponse();
                appendMessage("連線錯誤", 'error', false);
                setConnected(false);
            };

            ws.onclose = function() {
                setConnected(false);
                appendMessage("連線已斷開，任務將在後台繼續執行", 'system', false);
                // 自动重连
                setTimeout(connect, 3000);
            };
        }

        function sendMessage() {
            const msg = input.value.trim();
            if (msg !== '' && ws && ws.readyState === WebSocket.OPEN) {
                appendMessage(msg, 'user', true);
                ws.send(JSON.stringify({content: msg}));
                input.value = '';
                finishCurrentResponse();
                autoScroll = true;
                scrollToBottom();
            }
        }

        sendBtn.addEventListener('click', sendMessage);
        input.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendMessage();
            }
        });

        input.addEventListener('input', function() {
            this.style.height = 'auto';
            this.style.height = Math.min(this.scrollHeight, 150) + 'px';
        });

        // 初始化：先渲染历史，再连接
        renderHistory();
        connect();
    </script>
</body>
</html>`
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write([]byte(html))
}

// wsHandler 处理 WebSocket 连接
func (s *HTTPServer) wsHandler(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                log.Printf("WebSocket upgrade error: %v", err)
                return
        }

        // 从 URL 参数获取 session ID
        sessionID := r.URL.Query().Get("session")
        session := globalWebSessionManager.GetOrCreate(sessionID)

        // 为此连接生成唯一标识（便于日志追踪）
        connID := uuid.New().String()[:8]

        // 创建 WebSocket 通道
        wsChannel := NewWSChannel(conn)

        // 为此连接创建独立的 context（不再共享会话级别的 WsCtx）
        // 这样旧连接断开时不会影响新连接
        wsCtx, wsCancel := context.WithCancel(context.Background())

        // 标记会话已连接
        session.SetConnected(true)
        log.Printf("[WS] Connection %s established for session %s", connID, session.ID)
        defer func() {
                session.SetConnected(false)
                // 只取消此连接的 context，不影响其他连接
                wsCancel()
                conn.Close()
                log.Printf("[WS] Connection %s disconnected for session %s", connID, session.ID)
        }()

        // 发送会话信息
        wsChannel.WriteChunk(StreamChunk{
                Content:     "",
                SessionID:   session.ID,
                TaskRunning: session.IsTaskRunning(),
        })

        // 检查是否有新消息需要同步（重连时发送错过的新消息）
        newMessages := session.GetNewMessages()
        if len(newMessages) > 0 {
                log.Printf("Session %s: syncing %d new messages on reconnect, task_running=%v", session.ID, len(newMessages), session.IsTaskRunning())
                wsChannel.WriteChunk(StreamChunk{
                        SessionID:   session.ID,
                        HistorySync: newMessages,
                        TaskRunning: session.IsTaskRunning(), // 确保同步最新的任务状态
                })
                // 标记所有消息已发送
                session.MarkAllSent()
        }

        // 启动输出协程：从会话输出队列读取，发送到 WebSocket
        // 使用此连接独立的 wsCtx，这样旧连接断开不会影响新连接
        outputDone := make(chan struct{})
        go func() {
                defer close(outputDone)
                log.Printf("[WS] Connection %s output goroutine started", connID)
                for {
                        select {
                        case chunk, ok := <-session.OutputQueue:
                                if !ok {
                                        log.Printf("[WS] Connection %s output queue closed", connID)
                                        return
                                }
                                // 添加 session_id 和 task_running 状态
                                chunk.SessionID = session.ID
                                if err := wsChannel.WriteChunk(chunk); err != nil {
                                        log.Printf("[WS] Connection %s write error: %v", connID, err)
                                        return
                                }
                        case <-wsCtx.Done():
                                log.Printf("[WS] Connection %s output goroutine stopped (context done)", connID)
                                return
                        }
                }
        }()

        // 主循环：读取用户输入
        var mu sync.Mutex

        for {
                var msg struct {
                        Content string `json:"content"`
                }
                err := conn.ReadJSON(&msg)
                if err != nil {
                        log.Printf("[WS] Connection %s read error (session %s): %v", connID, session.ID, err)
                        break
                }

                trimmed := strings.TrimSpace(msg.Content)
                if trimmed == "" {
                        continue
                }

                // 统一处理斜杠命令
                if strings.HasPrefix(trimmed, "/") {
                        if globalRoleManager != nil && globalActorManager != nil && globalStage != nil {
                                result := ProcessSlashCommand(trimmed, globalRoleManager, globalActorManager, globalStage)
                                if result.Handled {
                                        if result.IsExit {
                                                session.EnqueueOutput(StreamChunk{Content: result.Response + "\n", Done: true})
                                                break
                                        }
                                        if result.IsStop {
                                                mu.Lock()
                                                if session.IsTaskRunning() {
                                                        session.CancelTask()
                                                        // 发送取消响应和任务状态更新
                                                        session.EnqueueOutput(StreamChunk{Content: result.Response + "\n", TaskRunning: false, Done: true})
                                                } else {
                                                        session.EnqueueOutput(StreamChunk{Content: "No active task to cancel.\n", Done: true})
                                                }
                                                mu.Unlock()
                                                continue
                                        }
                                        session.EnqueueOutput(StreamChunk{Content: result.Response + "\n", Done: true})
                                        continue
                                }
                        }
                }

                // 将输入加入队列，启动后台任务处理
                session.AddToHistory("user", trimmed)
                go processUserInput(session, trimmed)
        }

        // 等待输出协程结束
        <-outputDone
}

// processUserInput 处理用户输入（后台任务）
func processUserInput(session *WebSession, input string) {
        // 原子操作：检查并设置任务状态，获取任务ID
        ok, taskID := session.TryStartTask()
        if !ok {
                session.EnqueueOutput(StreamChunk{
                        Error: "已有任务在执行中，请使用 /stop 取消后再试",
                })
                return
        }

        // 保存当前任务的 context，用于判断是否被取消
        taskCtx := session.GetTaskCtx()

        session.EnqueueOutput(StreamChunk{TaskRunning: true})
        defer func() {
                // 检查任务是否被取消
                select {
                case <-taskCtx.Done():
                        // 任务被取消，CancelTask() 已经处理了状态
                        log.Printf("[Session %s] processUserInput: task was cancelled", session.ID)
                default:
                        // 任务正常完成，重置状态（仅当 taskID 匹配时）
                        session.SetTaskRunning(false, taskID)
                        session.EnqueueOutput(StreamChunk{TaskRunning: false})
                }
        }()

        // 创建输出通道
        outputChannel := NewSessionChannel(session)

        // 获取当前历史
        history := session.GetHistory()

        // 执行 AgentLoop
        newHistory, err := AgentLoop(
                session.TaskCtx,
                outputChannel,
                history,
                apiType, baseURL, apiKey, modelID,
                temperature, maxTokens, stream, thinking,
        )

        if err != nil {
                // 即使发生错误，也要保存已生成的消息历史（防止消息丢失）
                if len(newHistory) > len(history) {
                        session.SetHistory(newHistory)
                        log.Printf("[Session %s] Saved partial history after error (old: %d, new: %d)", session.ID, len(history), len(newHistory))
                }
                if err == context.Canceled {
                        session.EnqueueOutput(StreamChunk{Content: "\n[任务已取消]\n", Done: true})
                } else {
                        session.EnqueueOutput(StreamChunk{Error: err.Error(), Done: true})
                }
                return
        }

        // 更新历史
        session.SetHistory(newHistory)
}

// TriggerDelayedTaskWake 触发延迟任务唤醒后的模型调用
func TriggerDelayedTaskWake(session *WebSession, wakeMessage string) {
        log.Printf("[TaskManager] TriggerDelayedTaskWake started for session %s", session.ID)

        // 原子操作：检查并设置任务状态，获取任务ID
        ok, taskID := session.TryStartTask()
        if !ok {
                log.Printf("[TaskManager] Session %s has running task, skipping wake trigger", session.ID)
                return
        }

        // 保存当前任务的 context，用于判断是否被取消
        taskCtx := session.GetTaskCtx()

        session.EnqueueOutput(StreamChunk{TaskRunning: true})
        defer func() {
                select {
                case <-taskCtx.Done():
                        // 任务被取消，CancelTask() 已经处理了状态
                        log.Printf("[TaskManager] TriggerDelayedTaskWake: task was cancelled for session %s", session.ID)
                default:
                        // 任务正常完成，重置状态（仅当 taskID 匹配时）
                        session.SetTaskRunning(false, taskID)
                        session.EnqueueOutput(StreamChunk{TaskRunning: false})
                        log.Printf("[TaskManager] TriggerDelayedTaskWake completed for session %s", session.ID)
                }
        }()

        // 创建输出通道
        outputChannel := NewSessionChannel(session)

        // 获取当前历史
        history := session.GetHistory()
        log.Printf("[TaskManager] Session %s history length: %d", session.ID, len(history))

        // 执行 AgentLoop
        log.Printf("[TaskManager] Calling AgentLoop for session %s", session.ID)
        newHistory, err := AgentLoop(
                session.TaskCtx,
                outputChannel,
                history,
                apiType, baseURL, apiKey, modelID,
                temperature, maxTokens, stream, thinking,
        )

        if err != nil {
                log.Printf("[TaskManager] AgentLoop error for session %s: %v", session.ID, err)
                if len(newHistory) > len(history) {
                        session.SetHistory(newHistory)
                        log.Printf("[TaskManager] Saved partial history after error for session %s (old: %d, new: %d)", session.ID, len(history), len(newHistory))
                }
                if err == context.Canceled {
                        session.EnqueueOutput(StreamChunk{Content: "\n[任务已取消]\n", Done: true})
                } else {
                        session.EnqueueOutput(StreamChunk{Error: err.Error(), Done: true})
                }
                return
        }

        log.Printf("[TaskManager] AgentLoop completed for session %s, new history length: %d", session.ID, len(newHistory))

        // 更新历史
        session.SetHistory(newHistory)
}

// propsHandler 返回服务器属性（真实的机器属性和配置）
func (s *HTTPServer) propsHandler(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Access-Control-Allow-Origin", "*")

        // 检查配置是否完整（API Key 为空则需要配置）
        needsSetup := apiKey == ""

        // 构建真实的 props 数据
        props := map[string]interface{}{
                "default_generation_settings": map[string]interface{}{
                        "id":             0,
                        "id_task":        0,
                        "n_ctx":          4096, // 默认上下文大小
                        "speculative":    false,
                        "is_processing":  false,
                        "params": map[string]interface{}{
                                "n_predict":              -1,
                                "seed":                   -1,
                                "temperature":            temperature,
                                "dynatemp_range":         0.0,
                                "dynatemp_exponent":      1.0,
                                "top_k":                  40,
                                "top_p":                  0.9,
                                "min_p":                  0.05,
                                "top_n_sigma":            -1,
                                "xtc_probability":        0.0,
                                "xtc_threshold":          0.1,
                                "typ_p":                  1.0,
                                "repeat_last_n":          64,
                                "repeat_penalty":         1.0,
                                "presence_penalty":       0.0,
                                "frequency_penalty":      0.0,
                                "dry_multiplier":         0.0,
                                "dry_base":               1.75,
                                "dry_allowed_length":     2,
                                "dry_penalty_last_n":     -1,
                                "dry_sequence_breakers":  []string{},
                                "mirostat":               0,
                                "mirostat_tau":           5.0,
                                "mirostat_eta":           0.1,
                                "stop":                   []string{},
                                "max_tokens":             maxTokens,
                                "n_keep":                 0,
                                "n_discard":              0,
                                "ignore_eos":             false,
                                "stream":                 stream,
                                "logit_bias":             []interface{}{},
                                "n_probs":                0,
                                "min_keep":               0,
                                "grammar":                "",
                                "grammar_lazy":           false,
                                "grammar_triggers":       []string{},
                                "preserved_tokens":       []int{},
                                "chat_format":            "chatml",
                                "reasoning_format":       "auto",
                                "reasoning_in_content":   false,
                                "generation_prompt":      "",
                                "samplers":               []string{"top_k", "top_p", "min_p"},
                                "backend_sampling":       true,
                                "speculative.n_max":      16,
                                "speculative.n_min":      1,
                                "speculative.p_min":      0.9,
                                "timings_per_token":      false,
                                "post_sampling_probs":    false,
                                "lora":                   []interface{}{},
                        },
                        "prompt": "",
                        "next_token": map[string]interface{}{
                                "has_next_token": false,
                                "has_new_line":   false,
                                "n_remain":       0,
                                "n_decoded":      0,
                                "stopping_word":  "",
                        },
                },
                "total_slots":   1,
                "model_path":    modelID, // 使用配置中的模型 ID
                "role":          "model",  // garclaw 以单模型模式运行
                "modalities": map[string]interface{}{
                        "vision": false,
                        "audio":  false,
                },
                "chat_template": "chatml",
                "bos_token":     "<|begin_of_text|>",
                "eos_token":     "<|end_of_text|>",
                "build_info":    "garclaw v1.0.0",
                "needs_setup":   needsSetup, // 标识是否需要配置
                "webui_settings": map[string]interface{}{
                        "show_thinking": thinking,
                        "api_type":      apiType,
                        "base_url":      baseURL,
                },
        }

        jsonData, err := json.Marshal(props)
        if err != nil {
                log.Printf("Error marshaling props: %v", err)
                w.WriteHeader(http.StatusInternalServerError)
                return
        }
        w.Write(jsonData)
}

// modelsHandler 返回模型列表（配置文件中的真实模型）
func (s *HTTPServer) modelsHandler(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Access-Control-Allow-Origin", "*")

        // 从配置中获取当前模型
        currentModel := modelID
        if currentModel == "" {
                currentModel = "default"
        }

        // 构建模型列表 - 返回配置中的模型
        models := map[string]interface{}{
                "object": "list",
                "data": []map[string]interface{}{
                        {
                                "id":       currentModel,
                                "object":   "model",
                                "created":  time.Now().Unix(),
                                "owned_by": "garclaw",
                                "in_cache": true,
                                "path":     currentModel,
                                "status": map[string]interface{}{
                                        "value": "loaded",
                                },
                                "tags": []string{
                                        apiType,
                                },
                        },
                },
        }

        jsonData, err := json.Marshal(models)
        if err != nil {
                log.Printf("Error marshaling models: %v", err)
                w.WriteHeader(http.StatusInternalServerError)
                return
        }
        w.Write(jsonData)
}

// uploadHandler 处理文件上传
func (s *HTTPServer) uploadHandler(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

        // 处理 CORS 预检请求
        if r.Method == "OPTIONS" {
                w.WriteHeader(http.StatusOK)
                return
        }

        if r.Method != "POST" {
                http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
                return
        }

        // 解析 multipart form，最大 100MB
        err := r.ParseMultipartForm(100 << 20)
        if err != nil {
                http.Error(w, fmt.Sprintf(`{"error": "Failed to parse form: %s"}`, err.Error()), http.StatusBadRequest)
                return
        }

        file, header, err := r.FormFile("file")
        if err != nil {
                http.Error(w, fmt.Sprintf(`{"error": "Failed to get file: %s"}`, err.Error()), http.StatusBadRequest)
                return
        }
        defer file.Close()

        // 生成唯一文件名，保留原始扩展名
        ext := filepath.Ext(header.Filename)
        uniqueID := uuid.New().String()
        newFilename := uniqueID + ext
        savePath := filepath.Join(globalUploadDir, newFilename)

        // 创建目标文件
        dst, err := os.Create(savePath)
        if err != nil {
                http.Error(w, fmt.Sprintf(`{"error": "Failed to create file: %s"}`, err.Error()), http.StatusInternalServerError)
                return
        }
        defer dst.Close()

        // 复制文件内容
        written, err := io.Copy(dst, file)
        if err != nil {
                http.Error(w, fmt.Sprintf(`{"error": "Failed to save file: %s"}`, err.Error()), http.StatusInternalServerError)
                return
        }

        log.Printf("File uploaded: %s -> %s (%d bytes)", header.Filename, savePath, written)

        // 返回文件信息
        response := map[string]interface{}{
                "success":  true,
                "filename": header.Filename,
                "size":     written,
                "path":     savePath,
                "url":      "/file/" + newFilename,
                "message":  fmt.Sprintf("文件已上传到: %s\n你可以告诉模型去读取这个文件: /path %s", savePath, savePath),
        }

        jsonData, _ := json.Marshal(response)
        w.Write(jsonData)
}

// fileHandler 提供已上传文件的访问
func (s *HTTPServer) fileHandler(w http.ResponseWriter, r *http.Request) {
        // 提取文件名
        filename := strings.TrimPrefix(r.URL.Path, "/file/")
        if filename == "" {
                http.Error(w, "Filename required", http.StatusBadRequest)
                return
        }

        // 安全检查：防止路径遍历
        if strings.Contains(filename, "..") || strings.ContainsAny(filename, "/\\") {
                http.Error(w, "Invalid filename", http.StatusBadRequest)
                return
        }

        // 构建文件路径
        filePath := filepath.Join(globalUploadDir, filename)

        // 检查文件是否存在
        info, err := os.Stat(filePath)
        if err != nil {
                http.Error(w, "File not found", http.StatusNotFound)
                return
        }

        // 禁止访问目录
        if info.IsDir() {
                http.Error(w, "Cannot access directory", http.StatusForbidden)
                return
        }

        // 设置 CORS 头
        w.Header().Set("Access-Control-Allow-Origin", "*")

        // 提供文件
        http.ServeFile(w, r, filePath)
}
