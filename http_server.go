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

// indexHandler 提供静态聊天页面，包含优化后的 JavaScript
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
        .reasoning { color: #666; font-style: italic; background-color: #f5f5f5; padding: 2px 5px; border-radius: 3px; margin: 2px 0; }
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

        // 用于累积 reasoning 片段的变量
        let pendingReasoning = '';

        // 刷新累积的 reasoning 内容
        function flushReasoning() {
            if (pendingReasoning) {
                const p = document.createElement('p');
                p.className = 'reasoning';
                p.textContent = '推理过程: ' + pendingReasoning;
                messagesDiv.appendChild(p);
                pendingReasoning = '';
            }
        }

        ws.onmessage = function(event) {
            const chunk = JSON.parse(event.data);

            if (chunk.error) {
                flushReasoning(); // 遇到错误先输出累积的 reasoning
                appendMessage("Error: " + chunk.error, "error");
                return;
            }

            // 处理 reasoning_content 累积
            if (chunk.reasoning_content) {
                pendingReasoning += chunk.reasoning_content;
            }

            // 如果有普通内容，先输出累积的 reasoning，再输出内容
            if (chunk.content) {
                flushReasoning();
                appendMessage(chunk.content, "assistant");
            }

            // 如果有工具调用，先输出累积的 reasoning，再输出工具调用
            if (chunk.tool_calls && chunk.tool_calls.length > 0) {
                flushReasoning();
                appendMessage("[Tool calls: " + JSON.stringify(chunk.tool_calls) + "]", "tool");
            }

            // 响应结束时，输出剩余的 reasoning 并显示完成标记
            if (chunk.done) {
                flushReasoning();
                appendMessage("--- Response complete ---", "system");
            }
        };

        ws.onerror = function(error) {
            console.error("WebSocket error:", error);
            appendMessage("WebSocket connection error", "error");
        };

        ws.onclose = function() {
            flushReasoning();
            appendMessage("Connection closed", "system");
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