package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（本地应用）
	},
}

// WebSocketHub 管理所有 WebSocket 连接并广播进度消息
type WebSocketHub struct {
	clients    map[*websocket.Conn]bool
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	broadcast  chan interface{}
	mu         sync.RWMutex
}

func newWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*websocket.Conn]bool),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		broadcast:  make(chan interface{}, 64),
	}
}

// Broadcast 发送消息到所有已连接的客户端
func (h *WebSocketHub) Broadcast(msg interface{}) {
	h.broadcast <- msg
}

func (h *WebSocketHub) run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()
			log.Printf("[ws] 客户端已连接 (当前 %d)", len(h.clients))

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mu.Unlock()
			log.Printf("[ws] 客户端已断开 (当前 %d)", len(h.clients))

		case msg := <-h.broadcast:
			h.mu.RLock()
			for conn := range h.clients {
				data, _ := json.Marshal(msg)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("[ws] 发送失败: %v", err)
					go func(c *websocket.Conn) {
						h.unregister <- c
					}(conn)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// handleWebSocket 处理 WebSocket 升级
// GET /ws/progress
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] 升级失败: %v", err)
		return
	}

	s.wsHub.register <- conn

	// 保持连接活跃（读取循环，检测断开）
	go func() {
		defer func() {
			s.wsHub.unregister <- conn
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
