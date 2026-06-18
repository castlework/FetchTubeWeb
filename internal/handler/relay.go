package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"FetchTubeWeb/internal/relay"
)

// handleRelayTest 测试 VPS 中继连接
// POST /api/relay/test
func (s *Server) handleRelayTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效的请求体")
		return
	}

	client := relay.NewClient(req.Host, req.Port)
	ok, err := client.TestConnection()
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"ok":   ok,
		"host": req.Host,
		"port": req.Port,
	})
}

// handleRelaySubmit 下发下载任务到 VPS
// POST /api/relay/submit
func (s *Server) handleRelaySubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		URL       string `json:"url"`
		FormatID  string `json:"format_id"`
		OutputExt string `json:"output_ext"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效的请求体")
		return
	}

	client := relay.NewClient(req.Host, req.Port)
	task, err := client.SubmitDownload(req.URL, req.FormatID, req.OutputExt)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, task)
}

// handleRelayTasks 获取 VPS 任务列表
// GET /api/relay/tasks?host=...&port=...
func (s *Server) handleRelayTasks(w http.ResponseWriter, r *http.Request) {
	host, port := getHostPort(r)
	if host == "" {
		writeError(w, 400, "缺少 host/port 参数")
		return
	}

	client := relay.NewClient(host, port)
	tasks, err := client.ListTasks()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, tasks)
}

// handleRelayFiles 获取 VPS 文件列表
// GET /api/relay/files?host=...&port=...
func (s *Server) handleRelayFiles(w http.ResponseWriter, r *http.Request) {
	host, port := getHostPort(r)
	if host == "" {
		writeError(w, 400, "缺少 host/port 参数")
		return
	}

	client := relay.NewClient(host, port)
	files, err := client.ListFiles()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, files)
}

// handleRelayDownload 从 VPS 下载文件到本地
// POST /api/relay/download
func (s *Server) handleRelayDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Filename string `json:"filename"`
		SaveDir  string `json:"save_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效的请求体")
		return
	}

	client := relay.NewClient(req.Host, req.Port)
	savePath, err := client.DownloadFile(req.Filename, req.SaveDir, nil)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{
		"status": "downloaded",
		"path":   savePath,
	})
}

// handleRelayDeleteFile 删除 VPS 文件
// DELETE /api/relay/file
func (s *Server) handleRelayDeleteFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效的请求体")
		return
	}

	client := relay.NewClient(req.Host, req.Port)
	if err := client.DeleteFile(req.Filename); err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ---- 辅助 ----

func getHostPort(r *http.Request) (string, int) {
	host := r.URL.Query().Get("host")
	portStr := r.URL.Query().Get("port")
	if host == "" || portStr == "" {
		return "", 0
	}
	port := 8899
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}
