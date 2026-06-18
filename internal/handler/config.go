package handler

import (
	"encoding/json"
	"net/http"

	"FetchTubeWeb/internal/config"
)

// handleGetConfig returns config
// GET /api/config
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	writeJSON(w, 200, cfg)
}

// handlePutConfig saves config
// PUT /api/config
func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, 400, "Invalid config data: "+err.Error())
		return
	}

	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		writeError(w, 500, "Failed to save config: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "saved"})
}
