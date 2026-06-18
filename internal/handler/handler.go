package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"FetchTubeWeb/internal/config"
	"FetchTubeWeb/internal/ytdlp"
)

// Server encapsulates all HTTP handlers and shared state
type Server struct {
	mu        sync.Mutex
	cfg       config.AppConfig
	tasks     *TaskManager
	wsHub     *WebSocketHub
}

// NewServer creates a new server instance
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

// SetupRoutes registers all routes on the ServeMux
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	// API routes
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
	mux.HandleFunc("GET /api/thumbnail", s.handleThumbnail)

	// WebSocket
	mux.HandleFunc("GET /ws/progress", s.handleWebSocket)

	log.Println("[server] routes registered")
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// corsMiddleware handles CORS preflight requests
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
