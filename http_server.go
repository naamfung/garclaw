package main

import (
	"context"
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

// indexHandler 提供静态聊天页面
func (s *HTTPServer) indexHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>GarClaw Chat</title>
    <style>
        body { font-family: Arial; max-width: 800px; margin: 0 auto; padding: 20px; }
        #messages { height: 500px; overflow-y: scroll; border: 1px solid #ccc; padding: 10px; margin-bottom: 10px; }
        #input { width: 100%; box-sizing: border-box; padding: 10px; }
        .user { color: blue; }
        .assistant { color: green; }
        .error { color: red; }
        .tool { color: orange; }
        .system { color: gray; font-style: italic; }
    </style>
</head>
<body>
    <h1>GarClaw Chat</h1>
    <div id="messages"></div>
    <input type="text" id="input" placeholder="Type your message and press Enter. Use /stop to cancel current task.">
    <script>
        const ws = new WebSocket("ws://" + location.host + "/ws");
        const messagesDiv = document.getElementById('messages');
        const input = document.getElementById('input');

        ws.onmessage = function(event) {
            const chunk = JSON.parse(event.data);
            if (chunk.error) {
                appendMessage("Error: " + chunk.error, "error");
            }
            if (chunk.content) {
                appendMessage(chunk.content, "assistant");
            }
            if (chunk.reasoning_content) {
                appendMessage("[reasoning] " + chunk.reasoning_content, "assistant");
            }
            if (chunk.done) {
                appendMessage("--- Response complete ---", "system");
            }
        };

        input.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && input.value.trim() !== '') {
                const msg = input.value.trim();
                appendMessage(msg, "user");
                ws.send(JSON.stringify({content: msg}));
                input.value = '';
            }
        });

        function appendMessage(text, className) {
            const p = document.createElement('p');
            p.className = className;
            p.textContent = text;
            messagesDiv.appendChild(p);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// wsHandler 处理 WebSocket 连接
func (s *HTTPServer) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ch := NewWSChannel(conn)
	var history []Message

	// 为每个连接创建一个可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 标记是否正在处理任务
	var mu sync.Mutex
	taskActive := false

	for {
		var msg struct {
			Content string `json:"content"`
		}
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}
		// 忽略空消息
		trimmed := strings.TrimSpace(msg.Content)
		if trimmed == "" {
			continue
		}

		// 检查是否为退出命令
		if strings.ToLower(trimmed) == "exit" {
			break
		}

		// 检查是否为取消命令
		if strings.ToLower(trimmed) == "/stop" {
			mu.Lock()
			if taskActive {
				cancel() // 取消当前任务
				ch.WriteChunk(StreamChunk{Content: "Task cancelled.\n"})
				// 重置 context，以便后续新任务
				ctx, cancel = context.WithCancel(context.Background())
				taskActive = false
			} else {
				ch.WriteChunk(StreamChunk{Content: "No active task to cancel.\n"})
			}
			mu.Unlock()
			continue
		}

		// 普通用户输入
		history = append(history, Message{Role: "user", Content: trimmed})

		mu.Lock()
		taskActive = true
		mu.Unlock()

		// 启动 AgentLoop，传入可取消的 context
		newHistory, err := AgentLoop(ctx, ch, history, apiType, baseURL, apiKey, modelID, temperature, maxTokens, stream, thinking)

		mu.Lock()
		taskActive = false
		mu.Unlock()

		if err != nil {
			if err == context.Canceled {
				ch.WriteChunk(StreamChunk{Content: "Task was cancelled.\n"})
			} else {
				ch.WriteChunk(StreamChunk{Error: err})
			}
		} else {
			history = newHistory
		}
	}
}