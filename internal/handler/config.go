package handler

import (
	"encoding/json"
	"net/http"

	"FetchTubeWeb/internal/config"
)

// handleGetConfig 获取配置
// GET /api/config
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	writeJSON(w, 200, cfg)
}

// handlePutConfig 保存配置
// PUT /api/config
func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, 400, "无效的配置数据: "+err.Error())
		return
	}

	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		writeError(w, 500, "保存配置失败: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "saved"})
}
