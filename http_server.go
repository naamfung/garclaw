package main

import (
        "context"
        "fmt"
        "log"
        "net/http"
        "strings"
        "sync"
        "time"

        "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool { return true },
}

// HTTPServer 管理 HTTP 和 WebSocket 服务
type HTTPServer struct {
        addr   string
        server *http.Server
}

// NewHTTPServer 创建 HTTP 服务器实例
func NewHTTPServer(addr string) *HTTPServer {
        return &HTTPServer{
                addr: addr,
        }
}

// Start 启动 HTTP 服务器
func (s *HTTPServer) Start() {
        mux := http.NewServeMux()
        mux.HandleFunc("/", s.indexHandler)
        mux.HandleFunc("/ws", s.wsHandler)

        s.server = &http.Server{
                Addr:         s.addr,
                Handler:      mux,
                ReadTimeout:  10 * time.Second,
                WriteTimeout: 10 * time.Second,
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

// indexHandler 提供静态聊天页面，现代化暗色主题界面
func (s *HTTPServer) indexHandler(w http.ResponseWriter, r *http.Request) {
    html := `<!DOCTYPE html>
<html lang="zh-TW">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>GarClaw Chat</title>
    <link rel="icon" href="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjU2IiB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIGhlaWdodD0iMjU2IiBpZD0ic2NyZWVuc2hvdC1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyOTkwMGVkYmYiIHZpZXdCb3g9IjAgMCAyNTYgMjU2IiB4bWxuczp4bGluaz0iaHR0cDovL3d3dy53My5vcmcvMTk5OS94bGluayIgZmlsbD0ibm9uZSIgdmVyc2lvbj0iMS4xIj48ZyBpZD0ic2hhcGUtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0Mjk5MDBlZGJmIiByeD0iMCIgcnk9IjAiPjxnIGlkPSJzaGFwZS1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyMTU3NTVjM2EiPjxnIGNsYXNzPSJmaWxscyIgaWQ9ImZpbGxzLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIxNTc1NWMzYSI+PHJlY3Qgcng9IjAiIHJ5PSIwIiB4PSIwIiB5PSIwIiB0cmFuc2Zvcm09Im1hdHJpeCgxLjAwMDAwMCwgMC4wMDAwMDAsIDAuMDAwMDAwLCAxLjAwMDAwMCwgMC4wMDAwMDAsIDAuMDAwMDAwKSIgd2lkdGg9IjI1NiIgaGVpZ2h0PSIyNTYiIHN0eWxlPSJmaWxsOiByZ2IoMjcsIDMxLCAzMik7IGZpbGwtb3BhY2l0eTogMTsiLz48L2c+PC9nPjxnIGlkPSJzaGFwZS1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyMjM2M2VmM2YiIHJ4PSIwIiByeT0iMCI+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MCI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQwIj48cGF0aCBkPSJNMTcxLjY2NTAwODU0NDkyMTg4LDk5LjUzMDI1MDU0OTMxNjRMMTU5Ljc5OTUzMDAyOTI5Njg4LDEyMC42MjQ2ODcxOTQ4MjQyMkMxNDQuMTU0NTEwNDk4MDQ2ODgsMTA4LjU4MzI5MDEwMDA5NzY2LDEyMC45NTA0MTY1NjQ5NDE0LDEwNi44MjU0MTY1NjQ5NDE0LDEwNS4zMDUzOTcwMzM2OTE0LDExOS43NDU3NTA0MjcyNDYxQzgwLjA3OTgxMTA5NjE5MTQsMTQwLjU3NjUyMjgyNzE0ODQ0LDgxLjgzNzYyMzU5NjE5MTQsMTg4Ljc0MjI2Mzc5Mzk0NTMsMTIxLjEyNjE5NzgxNDk0MTQsMTg5LjAwNTg3NDYzMzc4OTA2QzEzMi4xMTMwMDY1OTE3OTY4OCwxODkuMDA1ODc0NjMzNzg5MDYsMTQxLjQyOTY1Njk4MjQyMTg4LDE4My44MjAxMTQxMzU3NDIyLDE1MS40NDk2NzY1MTM2NzE4OCwxODAuMzkyMzQ5MjQzMTY0MDZMMTU2LjcyMzM1ODE1NDI5Njg4LDIwMS4zOTg4NDk0ODczMDQ3QzE0Ny44NDU5MTY3NDgwNDY4OCwyMDUuNTI5ODkxOTY3NzczNDQsMTM4Ljc5MjkzODIzMjQyMTg4LDIwOS43NDg3MzM1MjA1MDc4LDEyOS4wMzY4MzQ3MTY3OTY4OCwyMTEuMDY3MTIzNDEzMDg1OTRDNDAuMDg4MzUyMjAzMzY5MTQsMjIzLjE5NjQ1NjkwOTE3OTcsNDUuMTg2MDA4NDUzMzY5MTQsOTQuNzg0MDA0MjExNDI1NzgsMTI1LjYwODg2MzgzMDU2NjQsODguMTA0MDcyNTcwODAwNzhDMTQyLjQ4NDM0NDQ4MjQyMTg4LDg2LjY5NzgyMjU3MDgwMDc4LDE1Ny4zMzgzNDgzODg2NzE4OCw5MS4wOTI0NzU4OTExMTMyOCwxNzEuNzUzMTQzMzEwNTQ2ODgsOTkuNTMwMjUwNTQ5MzE2NFoiIGNsYXNzPSJzdDAiIHN0eWxlPSJmaWxsOiByZ2IoMjU1LCAxMzAsIDU0KTsgZmlsbC1vcGFjaXR5OiAxOyIvPjwvZz48L2c+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MSI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQxIj48cGF0aCBkPSJNMTEwLjIyNzI3MjAzMzY5MTQsNzkuMzE0NzA0ODk1MDE5NTNDOTYuNjkxODcxNjQzMDY2NCw4My4zNTc4NTY3NTA0ODgyOCw4NC4xMjMyNjgxMjc0NDE0LDkwLjgyODgzNDUzMzY5MTQsNzQuNjMwNTkyMzQ2MTkxNCwxMDEuMjg4MTI0MDg0NDcyNjZDNzIuODcyNzc5ODQ2MTkxNCw4MC4wMTc4Mjk4OTUwMTk1Myw3Ny42MTg4NzM1OTYxOTE0LDM3LjAzNzkzNzE2NDMwNjY0LDEwMS4yNjIxODQxNDMwNjY0LDI4LjYwMDEwMzM3ODI5NTlDMTA0Ljc3ODA1MzI4MzY5MTQsMjcuMzY5NjQ5ODg3MDg0OTYsMTE2LjgxOTU1NzE4OTk0MTQsMjQuMjkzMzcxMjAwNTYxNTIzLDExNi40Njc5OTQ2ODk5NDE0LDMwLjUzMzc4ODY4MTAzMDI3M0MxMTYuMTE2MTg4MDQ5MzE2NCwzNi43NzQyNjUyODkzMDY2NCwxMDcuNzY2MzM0NTMzNjkxNCw0Ny40OTcyMjY3MTUwODc4OSwxMDUuNzQ1MDk0Mjk5MzE2NCw1My4yOTgyMzY4NDY5MjM4M0MxMDIuMjI5MjI1MTU4NjkxNCw2My40OTM4Njk3ODE0OTQxNCwxMDUuNDgxMTc4MjgzNjkxNCw3MC41MjUzNTI0NzgwMjczNCwxMTAuMzE1NDA2Nzk5MzE2NCw3OS40MDI2NTY1NTUxNzU3OFoiIGNsYXNzPSJzdDAiIHN0eWxlPSJmaWxsOiByZ2IoMjU1LCAxMzAsIDU0KTsgZmlsbC1vcGFjaXR5OiAxOyIvPjwvZz48L2c+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MiI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQyIj48cGF0aCBkPSJNMTQzLjYyNjkyMjYwNzQyMTg4LDEyNy42NTYyMTE4NTMwMjczNEwxNDMuNjI2OTIyNjA3NDIxODgsMTQzLjQ3NzA2NjA0MDAzOTA2TDE1Ny42ODk5MTA4ODg2NzE4OCwxNDMuNDc3MDY2MDQwMDM5MDZMMTU3LjY4OTkxMDg4ODY3MTg4LDE1NS43ODIxODA3ODYxMzI4TDE0My42MjY5MjI2MDc0MjE4OCwxNTUuNzgyMTgwNzg2MTMyOEwxNDMuNjI2OTIyNjA3NDIxODgsMTcwLjcyNDA3NTMxNzM4MjhMMTMwLjQ0Mjg0MDU3NjE3MTg4LDE3MC43MjQwNzUzMTczODI4TDEzMC40NDI4NDA1NzYxNzE4OCwxNTUuNzgyMTgwNzg2MTMyOEwxMTUuNTAwOTUzNjc0MzE2NCwxNTUuNzgyMTgwNzg2MTMyOEwxMTUuNTAwOTUzNjc0MzE2NCwxNDMuNDc3MDY2MDQwMDM5MDZMMTI5LjEyNDQ4MTIwMTE3MTg4LDE0My40NzcwNjYwNDAwMzkwNkwxMzAuNDQyODQwNTc2MTcxODgsMTQyLjE1ODY3NjE0NzQ2MDk0TDEzMC40NDI4NDA1NzYxNzE4OCwxMjcuNjU2MjExODUzMDI3MzRMMTQzLjYyNjkyMjYwNzQyMTg4LDEyNy42NTYyMTE4NTMwMjczNFoiIGNsYXNzPSJzdDAiIHN0eWxlPSJmaWxsOiByZ2IoMjU1LCAxMzAsIDU0KTsgZmlsbC1vcGFjaXR5OiAxOyIvPjwvZz48L2c+PGcgaWQ9InNoYXBlLWVmOTRmYmIwLWRiYWItODBlZC04MDA2LTg5NDIyMzYzZWY0MyI+PGcgY2xhc3M9ImZpbGxzIiBpZD0iZmlsbHMtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQzIj48cGF0aCBkPSJNMTkxLjk2ODIzMTIwMTE3MTg4LDEyNy42NTYyMTE4NTMwMjczNEwxOTEuOTY4MjMxMjAxMTcxODgsMTQyLjE1ODY3NjE0NzQ2MDk0TDE5My4yODY4MzQ3MTY3OTY4OCwxNDMuNDc3MDY2MDQwMDM5MDZMMjA2LjkxMDM2OTg3MzA0Njg4LDE0My40NzcwNjYwNDAwMzkwNkwyMDYuOTEwMzY5ODczMDQ2ODgsMTU1Ljc4MjE4MDc4NjEzMjhMMTkxLjk2ODIzMTIwMTE3MTg4LDE1NS43ODIxODA3ODYxMzI4TDE5MS45NjgyMzEyMDExNzE4OCwxNzAuNzI0MDc1MzE3MzgyOEwxNzguNzg0MzkzMzEwNTQ2ODgsMTcwLjcyNDA3NTMxNzM4MjhMMTc4Ljc4NDM5MzMxMDU0Njg4LDE1NS43ODIxODA3ODYxMzI4TDE2NC43MjE0MDUwMjkyOTY4OCwxNTUuNzgyMTgwNzg2MTMyOEwxNjQuNzIxNDA1MDI5Mjk2ODgsMTQzLjQ3NzA2NjA0MDAzOTA2TDE3OC43ODQzOTMzMTA1NDY4OCwxNDMuNDc3MDY2MDQwMDM5MDZMMTc4Ljc4NDM5MzMxMDU0Njg4LDEyNy42NTYyMTE4NTMwMjczNEwxOTEuOTY4MjMxMjAxMTcxODgsMTI3LjY1NjIxMTg1MzAyNzM0WiIgY2xhc3M9InN0MCIgc3R5bGU9ImZpbGw6IHJnYigyNTUsIDEzMCwgNTQpOyBmaWxsLW9wYWNpdHk6IDE7Ii8+PC9nPjwvZz48ZyBpZD0ic2hhcGUtZWY5NGZiYjAtZGJhYi04MGVkLTgwMDYtODk0MjIzNjNlZjQ0Ij48ZyBjbGFzcz0iZmlsbHMiIGlkPSJmaWxscy1lZjk0ZmJiMC1kYmFiLTgwZWQtODAwNi04OTQyMjM2M2VmNDQiPjxwYXRoIGQ9Ik0xNTMuMjA3NDg5MDEzNjcxODgsMzguMDkyNjU1MTgxODg0NzY2QzE1NC45NjU1NDU2NTQyOTY4OCw0MC43Mjk0NjU0ODQ2MTkxNCwxNDUuMDMzNDE2NzQ4MDQ2ODgsNTIuMDY3NzA3MDYxNzY3NTgsMTQzLjQ1MTE0MTM1NzQyMTg4LDU0Ljk2ODE3Mzk4MDcxMjg5QzEzOC44ODA4Mjg4NTc0MjE4OCw2My41ODE3OTA5MjQwNzIyNjYsMTQxLjk1NzAwMDczMjQyMTg4LDY4LjUwMzgyMjMyNjY2MDE2LDE0NS4zODQ3MzUxMDc0MjE4OCw3Ni42Nzc5MjUxMDk4NjMyOEMxMzUuNDUyODUwMzQxNzk2ODgsNzUuMTgzNzIzNDQ5NzA3MDMsMTI2LjIyNDA5ODIwNTU2NjQsNzYuNDE0MjUzMjM0ODYzMjgsMTE2LjM3OTg1OTkyNDMxNjQsNzcuNTU2ODMxMzU5ODYzMjhDMTE4LjU3NzM2OTY4OTk0MTQsNTguNjU5NzMyODE4NjAzNTE2LDEyOS4yMTI2MTU5NjY3OTY4OCwzMS4xNDkwNTM1NzM2MDg0LDE1My4yMDc0ODkwMTM2NzE4OCwzOC4wOTI2NTUxODE4ODQ3NjZaIiBjbGFzcz0ic3QwIiBzdHlsZT0iZmlsbDogcmdiKDI1NSwgMTMwLCA1NCk7IGZpbGwtb3BhY2l0eTogMTsiLz48L2c+PC9nPjwvZz48L2c+PC9zdmc+" />
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
        }
        
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background-color: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
        }
        
        .container {
            max-width: 900px;
            margin: 0 auto;
            width: 100%;
            flex: 1;
            display: flex;
            flex-direction: column;
            padding: 0 16px;
        }
        
        header {
            padding: 20px 0;
            border-bottom: 1px solid var(--border);
            margin-bottom: 16px;
        }
        
        .logo {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        
        .logo svg {
            width: 40px;
            height: 40px;
        }
        
        .logo h1 {
            font-size: 1.75rem;
            font-weight: 600;
            color: var(--text-primary);
            letter-spacing: -0.02em;
        }
        
        .logo span {
            color: var(--accent);
        }
        
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
        
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(8px); }
            to { opacity: 1; transform: translateY(0); }
        }
        
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
        
        .tool {
            background: rgba(255, 130, 54, 0.1);
            color: var(--accent);
            border: 1px solid rgba(255, 130, 54, 0.2);
            align-self: flex-start;
            font-family: 'SF Mono', 'Fira Code', monospace;
            font-size: 0.9em;
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
            background: linear-gradient(to top, var(--bg-primary) 80%, transparent);
        }
        
        .input-wrapper {
            display: flex;
            gap: 12px;
            align-items: flex-end;
        }
        
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
        
        #input:focus {
            border-color: var(--accent);
            box-shadow: 0 0 0 3px rgba(255, 130, 54, 0.15);
        }
        
        #input::placeholder {
            color: var(--text-muted);
        }
        
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
        
        .send-btn:hover {
            background: var(--accent-hover);
            transform: scale(1.05);
        }
        
        .send-btn:active {
            transform: scale(0.95);
        }
        
        .send-btn svg {
            width: 22px;
            height: 22px;
            color: white;
        }
        
        .status {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 8px 0;
            font-size: 0.875em;
            color: var(--text-muted);
        }
        
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--success);
            animation: pulse 2s infinite;
        }
        
        .status-dot.disconnected {
            background: var(--error);
            animation: none;
        }
        
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        
        .helper-text {
            font-size: 0.8em;
            color: var(--text-muted);
            margin-top: 8px;
        }
        
        .helper-text kbd {
            background: var(--bg-tertiary);
            padding: 2px 6px;
            border-radius: 4px;
            font-family: inherit;
            border: 1px solid var(--border);
        }
        
        ::-webkit-scrollbar {
            width: 8px;
        }
        
        ::-webkit-scrollbar-track {
            background: transparent;
        }
        
        ::-webkit-scrollbar-thumb {
            background: var(--border);
            border-radius: 4px;
        }
        
        ::-webkit-scrollbar-thumb:hover {
            background: var(--text-muted);
        }
        
        @media (max-width: 640px) {
            .container {
                padding: 0 12px;
            }
            
            .message {
                max-width: 90%;
                padding: 12px 14px;
            }
            
            .logo h1 {
                font-size: 1.5rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo">
                <svg viewBox="0 0 256 256" xmlns="http://www.w3.org/2000/svg">
                    <path d="M171.66500854492188,99.5302505493164L159.79953002929688,120.62468719482422C144.15451049804688,108.58329010009766,120.9504165649414,106.8254165649414,105.3053970336914,119.7457504272461C80.0798110961914,140.57652282714844,81.8376235961914,188.7422637939453,121.1261978149414,189.00587463378906C132.11300659179688,189.00587463378906,141.42965698242188,183.8201141357422,151.44967651367188,180.39234924316406L156.72335815429688,201.3988494873047C147.84591674804688,205.52989196777344,138.79293823242188,209.7487335205078,129.03683471679688,211.06712341308594C40.08835220336914,223.1964569091797,45.18600845336914,94.78400421142578,125.6088638305664,88.10407257080078C142.48434448242188,86.69782257080078,157.33834838867188,91.09247589111328,171.75314331054688,99.5302505493164Z" fill="#ff8236"/>
                    <path d="M110.2272720336914,79.31470489501953C96.6918716430664,83.35785675048828,84.1232681274414,90.8288345336914,74.6305923461914,101.28812408447266C72.8727798461914,80.01782989501953,77.6188735961914,37.03793716430664,101.2621841430664,28.6001033782959C104.7780532836914,27.36964988708496,116.8195571899414,24.293371200561523,116.4679946899414,30.533788681030273C116.1161880493164,36.77426528930664,107.7663345336914,47.49722671508789,105.7450942993164,53.29823684692383C102.2292251586914,63.49386978149414,105.4811782836914,70.52535247802734,110.3154067993164,79.40265655517578Z" fill="#ff8236"/>
                    <path d="M143.62692260742188,127.65621185302734L143.62692260742188,143.47706604003906L157.68991088867188,143.47706604003906L157.68991088867188,155.7821807861328L143.62692260742188,155.7821807861328L143.62692260742188,170.7240753173828L130.44284057617188,170.7240753173828L130.44284057617188,155.7821807861328L115.5009536743164,155.7821807861328L115.5009536743164,143.47706604003906L129.12448120117188,143.47706604003906L130.44284057617188,142.15867614746094L130.44284057617188,127.65621185302734L143.62692260742188,127.65621185302734Z" fill="#ff8236"/>
                    <path d="M191.96823120117188,127.65621185302734L191.96823120117188,142.15867614746094L193.28683471679688,143.47706604003906L206.91036987304688,143.47706604003906L206.91036987304688,155.7821807861328L191.96823120117188,155.7821807861328L191.96823120117188,170.7240753173828L178.78439331054688,170.7240753173828L178.78439331054688,155.7821807861328L164.72140502929688,155.7821807861328L164.72140502929688,143.47706604003906L178.78439331054688,143.47706604003906L178.78439331054688,127.65621185302734L191.96823120117188,127.65621185302734Z" fill="#ff8236"/>
                    <path d="M153.20748901367188,38.092655181884766C154.96554565429688,40.72946548461914,145.03341674804688,52.06770706176758,143.45114135742188,54.96817398376465C138.88082885742188,63.581790924072266,141.95700073242188,68.50382232666016,145.38473510742188,76.67792510986328C135.45285034179688,75.18372344970703,126.2240982055664,76.41425323486328,116.3798599243164,77.55683135986328C118.5773696899414,58.659732818603516,129.21261596679688,31.1490535736084,153.20748901367188,38.092655181884766Z" fill="#ff8236"/>
                </svg>
                <h1>Gar<span>Claw</span></h1>
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
            <p class="helper-text">按 <kbd>Enter</kbd> 發送 · 輸入 <kbd>/stop</kbd> 取消當前任務</p>
        </div>
    </div>
    
    <script>
        const ws = new WebSocket("ws://" + location.host + "/ws");
        const messagesDiv = document.getElementById('messages');
        const input = document.getElementById('input');
        const sendBtn = document.getElementById('sendBtn');
        const statusDot = document.getElementById('statusDot');
        const statusText = document.getElementById('statusText');
        
        let currentAssistantEl = null;
        let currentReasoningEl = null;
        
        function setConnected(connected) {
            if (connected) {
                statusDot.classList.remove('disconnected');
                statusText.textContent = '已連線';
            } else {
                statusDot.classList.add('disconnected');
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
                messagesDiv.appendChild(currentReasoningEl);
            }
            return currentReasoningEl;
        }
        
        function appendMessage(text, className) {
            const div = document.createElement('div');
            div.className = 'message ' + className;
            div.textContent = text;
            messagesDiv.appendChild(div);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }
        
        function sendMessage() {
            const msg = input.value.trim();
            if (msg !== '') {
                appendMessage(msg, 'user');
                ws.send(JSON.stringify({content: msg}));
                input.value = '';
                finishCurrentResponse();
            }
        }
        
        ws.onmessage = function(event) {
            const chunk = JSON.parse(event.data);
            
            if (chunk.error) {
                finishCurrentResponse();
                appendMessage("錯誤: " + chunk.error, 'error');
                return;
            }
            
            if (chunk.reasoning_content) {
                const el = getOrCreateReasoningEl();
                const span = el.querySelector('.reasoning-label');
                if (span) {
                    span.insertAdjacentText('afterend', chunk.reasoning_content);
                } else {
                    el.textContent += chunk.reasoning_content;
                }
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }
            
            if (chunk.content) {
                const el = getOrCreateAssistantEl();
                el.textContent += chunk.content;
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }
            
            if (chunk.tool_calls && chunk.tool_calls.length > 0) {
                const el = getOrCreateAssistantEl();
                el.textContent += "\n[工具調用: " + JSON.stringify(chunk.tool_calls) + "]\n";
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }
            
            if (chunk.done) {
                appendMessage("回應完成", 'system');
                finishCurrentResponse();
            }
        };
        
        ws.onopen = function() {
            setConnected(true);
        };
        
        ws.onerror = function(error) {
            console.error("WebSocket error:", error);
            finishCurrentResponse();
            appendMessage("連線錯誤", 'error');
            setConnected(false);
        };
        
        ws.onclose = function() {
            finishCurrentResponse();
            appendMessage("連線已關閉", 'system');
            setConnected(false);
        };
        
        input.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendMessage();
            }
        });
        
        sendBtn.addEventListener('click', sendMessage);
    </script>
</body>
</html>`
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write([]byte(html))
}

// wsHandler 处理 WebSocket 连接（与之前相同，无需修改）
func (s *HTTPServer) wsHandler(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                log.Printf("WebSocket upgrade error: %v", err)
                return
        }
        defer func() {
                conn.Close()
                log.Println("WebSocket connection closed")
        }()

        ch := NewWSChannel(conn)
        var history []Message

        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()

        var mu sync.Mutex
        taskActive := false

        for {
                var msg struct {
                        Content string `json:"content"`
                }
                err := conn.ReadJSON(&msg)
                if err != nil {
                        log.Printf("WebSocket read error: %v", err)
                        cancel()
                        break
                }
                trimmed := strings.TrimSpace(msg.Content)
                if trimmed == "" {
                        continue
                }

                if strings.ToLower(trimmed) == "exit" {
                        log.Println("Client requested exit")
                        cancel()
                        break
                }

                if strings.ToLower(trimmed) == "/stop" {
                        mu.Lock()
                        if taskActive {
                                cancel()
                                _ = ch.WriteChunk(StreamChunk{Content: "Task cancelled.\n"})
                                ctx, cancel = context.WithCancel(context.Background())
                                taskActive = false
                        } else {
                                _ = ch.WriteChunk(StreamChunk{Content: "No active task to cancel.\n"})
                        }
                        mu.Unlock()
                        continue
                }

                history = append(history, Message{Role: "user", Content: trimmed})

                mu.Lock()
                taskActive = true
                mu.Unlock()

                go func() {
                        defer func() {
                                if r := recover(); r != nil {
                                        log.Printf("AgentLoop panic recovered: %v", r)
                                        _ = ch.WriteChunk(StreamChunk{Error: fmt.Errorf("internal server error: %v", r)})
                                }
                                mu.Lock()
                                taskActive = false
                                mu.Unlock()
                        }()

                        newHistory, err := AgentLoop(ctx, ch, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)
                        if err != nil {
                                log.Printf("AgentLoop error: %v", err)
                                if err != context.Canceled {
                                        _ = ch.WriteChunk(StreamChunk{Error: err})
                                }
                        } else {
                                history = newHistory
                        }
                }()
        }
}