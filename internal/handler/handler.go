package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"FetchTubeWeb/internal/config"
	"FetchTubeWeb/internal/ytdlp"
)

// Server 封装所有 HTTP 处理器和共享状态
type Server struct {
	mu        sync.Mutex
	cfg       config.AppConfig
	tasks     *TaskManager
	wsHub     *WebSocketHub
}

// NewServer 创建新的服务器实例
func NewServer() *Server {
	s := &Server{
		cfg:   config.Load(),
		tasks: NewTaskManager(),
		wsHub: newWebSocketHub(),
	}
	go s.wsHub.run()

	log.Printf("[tools] yt-dlp: %s", ytdlp.FindYtDlp())
	log.Printf("[tools] ffmpeg: %s", ytdlp.FindFFmpeg())
	log.Printf("[tools] node:   %s", ytdlp.FindNode())

	return s
}

// SetupRoutes 注册所有路由到 ServeMux
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	// API 路由
	mux.HandleFunc("GET /api/info", s.handleInfo)
	mux.HandleFunc("POST /api/download", s.handleDownload)
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks/{taskID}/cancel", s.handleCancelTask)
	mux.HandleFunc("DELETE /api/tasks/{taskID}", s.handleDeleteTask)
	mux.HandleFunc("POST /api/tasks/batch-delete", s.handleBatchDeleteTasks)
	mux.HandleFunc("POST /api/open-dir", s.handleOpenDir)
	mux.HandleFunc("POST /api/pick-folder", s.handlePickFolder)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/browse", s.handleBrowse)
	mux.HandleFunc("GET /api/drives", s.handleDrives)

	// WebSocket
	mux.HandleFunc("GET /ws/progress", s.handleWebSocket)

	// 远程中继
	mux.HandleFunc("POST /api/relay/test", s.handleRelayTest)
	mux.HandleFunc("POST /api/relay/submit", s.handleRelaySubmit)
	mux.HandleFunc("GET /api/relay/tasks", s.handleRelayTasks)
	mux.HandleFunc("GET /api/relay/files", s.handleRelayFiles)
	mux.HandleFunc("POST /api/relay/download", s.handleRelayDownload)
	mux.HandleFunc("DELETE /api/relay/file", s.handleRelayDeleteFile)

	log.Println("[server] 路由已注册")
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError 写入错误响应
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// corsMiddleware 处理 CORS 预检请求
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}

		next(w, r)
	}
}
