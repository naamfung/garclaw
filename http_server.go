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

// indexHandler 提供静态聊天页面，修复流式显示问题
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
        .user { color: blue; margin: 8px 0; }
        .assistant { color: green; margin: 8px 0; white-space: pre-wrap; }
        .error { color: red; margin: 8px 0; }
        .tool { color: orange; margin: 8px 0; }
        .system { color: gray; font-style: italic; margin: 8px 0; }
        .reasoning { color: #666; font-style: italic; background-color: #f5f5f5; padding: 5px 8px; border-radius: 3px; margin: 8px 0; white-space: pre-wrap; }
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

        // 当前正在累积的元素
        let currentAssistantEl = null;
        let currentReasoningEl = null;

        // 完成当前响应，重置累积状态
        function finishCurrentResponse() {
            currentAssistantEl = null;
            currentReasoningEl = null;
        }

        // 获取或创建 assistant 元素
        function getOrCreateAssistantEl() {
            if (!currentAssistantEl) {
                currentAssistantEl = document.createElement('p');
                currentAssistantEl.className = 'assistant';
                messagesDiv.appendChild(currentAssistantEl);
            }
            return currentAssistantEl;
        }

        // 获取或创建 reasoning 元素
        function getOrCreateReasoningEl() {
            if (!currentReasoningEl) {
                currentReasoningEl = document.createElement('p');
                currentReasoningEl.className = 'reasoning';
                currentReasoningEl.textContent = '推理过程: ';
                messagesDiv.appendChild(currentReasoningEl);
            }
            return currentReasoningEl;
        }

        // 添加新消息（用户消息或系统消息）
        function appendMessage(text, className) {
            const p = document.createElement('p');
            p.className = className;
            p.textContent = text;
            messagesDiv.appendChild(p);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }

        ws.onmessage = function(event) {
            const chunk = JSON.parse(event.data);

            // 处理错误
            if (chunk.error) {
                finishCurrentResponse();
                appendMessage("Error: " + chunk.error, "error");
                return;
            }

            // 处理 reasoning_content（累积到同一个元素）
            if (chunk.reasoning_content) {
                const el = getOrCreateReasoningEl();
                el.textContent += chunk.reasoning_content;
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }

            // 处理普通内容（累积到同一个元素）
            if (chunk.content) {
                const el = getOrCreateAssistantEl();
                el.textContent += chunk.content;
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }

            // 处理工具调用
            if (chunk.tool_calls && chunk.tool_calls.length > 0) {
                const el = getOrCreateAssistantEl();
                el.textContent += "\n[Tool calls: " + JSON.stringify(chunk.tool_calls) + "]\n";
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }

            // 响应结束
            if (chunk.done) {
                appendMessage("--- Response complete ---", "system");
                finishCurrentResponse();
            }
        };

        ws.onerror = function(error) {
            console.error("WebSocket error:", error);
            finishCurrentResponse();
            appendMessage("WebSocket connection error", "error");
        };

        ws.onclose = function() {
            finishCurrentResponse();
            appendMessage("Connection closed", "system");
        };

        input.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && input.value.trim() !== '') {
                const msg = input.value.trim();
                appendMessage(msg, "user");
                ws.send(JSON.stringify({content: msg}));
                input.value = '';
                // 发送新消息时重置累积状态
                finishCurrentResponse();
            }
        });
    </script>
</body>
</html>`
    w.Header().Set("Content-Type", "text/html")
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