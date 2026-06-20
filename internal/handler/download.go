package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"FetchTubeWeb/internal/ytdlp"
)

// handleInfo handles video info extraction requests
// GET /api/info?url=...&proxy=...&cookies=...
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		writeError(w, 400, "Missing url parameter")
		return
	}

	proxy := r.URL.Query().Get("proxy")
	cookies := r.URL.Query().Get("cookies")

	raw, err := ytdlp.FetchInfo(url, proxy, cookies)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	info := ytdlp.ParseVideoInfo(raw)
	writeJSON(w, 200, info)
}

// handleHealth health check
// GET /api/health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"status":   "ok",
		"ytdlp":    ytdlp.FindYtDlp() != "",
		"ffmpeg":   ytdlp.FindFFmpeg() != "",
		"node":     ytdlp.FindNode() != "",
		"browsers": ytdlp.Browsers,
	})
}

// handleDownload enqueues a download job
// POST /api/download
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL                string `json:"url"`
		FormatID           string `json:"format_id"`
		OutputExt          string `json:"output_ext"`
		SaveDir            string `json:"save_dir"`
		ConcurrentFragments int   `json:"concurrent_fragments"`
		Resume             bool   `json:"resume"`
		Proxy              string `json:"proxy,omitempty"`
		Cookies            string `json:"cookies,omitempty"`
		KeepTempFiles      bool   `json:"keep_temp_files"`
		Title              string `json:"title,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "Invalid request body: "+err.Error())
		return
	}

	if req.URL == "" || req.FormatID == "" {
		writeError(w, 400, "url and format_id are required")
		return
	}

	if req.SaveDir == "" {
		writeError(w, 400, "save_dir is required")
		return
	}

	if req.ConcurrentFragments <= 0 {
		req.ConcurrentFragments = 8
	}
	if req.OutputExt == "" {
		req.OutputExt = "mkv"
	}

	opts := ytdlp.DownloadOptions{
		URL:                req.URL,
		FormatID:           req.FormatID,
		OutputExt:          req.OutputExt,
		SaveDir:            req.SaveDir,
		ConcurrentFragments: req.ConcurrentFragments,
		Resume:             req.Resume,
		Proxy:              req.Proxy,
		Cookies:            req.Cookies,
		KeepTempFiles:      req.KeepTempFiles,
	}

	title := req.Title
	if title == "" {
		title = req.URL
	}

	taskID := s.tasks.Enqueue(opts, title, func(data ytdlp.ProgressData) {
		s.wsHub.Broadcast(data)
	}, nil)

	writeJSON(w, 200, map[string]string{
		"status":  "queued",
		"task_id": taskID,
	})
}

// handleListTasks lists all tasks
// GET /api/tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.tasks.List())
}

// handleCancelTask cancels a specified task
// POST /api/tasks/{taskID}/cancel
func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		writeError(w, 400, "Missing taskID")
		return
	}

	if err := s.tasks.Cancel(taskID); err != nil {
		writeError(w, 404, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

// handleDeleteTask removes a task from the list (does not affect active downloads)
// DELETE /api/tasks/{taskID}
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		writeError(w, 400, "Missing taskID")
		return
	}

	if err := s.tasks.Remove(taskID); err != nil {
		writeError(w, 404, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleBatchDeleteTasks batch deletes tasks
// POST /api/tasks/batch-delete
func (s *Server) handleBatchDeleteTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskIDs []string `json:"task_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "Invalid request body: "+err.Error())
		return
	}
	if len(req.TaskIDs) == 0 {
		writeError(w, 400, "task_ids cannot be empty")
		return
	}

	count := s.tasks.RemoveBatch(req.TaskIDs)
	writeJSON(w, 200, map[string]interface{}{
		"status":  "deleted",
		"deleted": count,
	})
}

// handleOpenDir opens the specified directory in file explorer
// POST /api/open-dir
func (s *Server) handleOpenDir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "Invalid request body: "+err.Error())
		return
	}
	if req.Path == "" {
		writeError(w, 400, "Missing path parameter")
		return
	}

	// Check if directory exists first
	if info, err := os.Stat(req.Path); err != nil || !info.IsDir() {
		writeError(w, 400, "Directory does not exist: "+req.Path)
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// explorer.exe delegates to existing shell process via COM and then exits.
		// Its exit code is unreliable (sometimes returns 1 even when window is open),
		// so we use Start() to launch and detach immediately, without waiting.
		cmd = exec.Command("explorer", req.Path)
		if err := cmd.Start(); err != nil {
			writeError(w, 500, "Failed to open directory: "+err.Error())
			return
		}
		// Release process handle to avoid zombie processes
		go cmd.Wait()
	case "darwin":
		cmd = exec.Command("open", req.Path)
		if err := cmd.Start(); err != nil {
			writeError(w, 500, "Failed to open directory: "+err.Error())
			return
		}
		go cmd.Wait()
	default:
		cmd = exec.Command("xdg-open", req.Path)
		if err := cmd.Start(); err != nil {
			writeError(w, 500, "Failed to open directory: "+err.Error())
			return
		}
		go cmd.Wait()
	}

	writeJSON(w, 200, map[string]string{"status": "opened"})
}

// handlePickFolder opens a native system folder picker dialog, returns the selected path
// POST /api/pick-folder
func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	var path string
	var cancelled bool
	var err error

	switch runtime.GOOS {
	case "windows":
		path, cancelled, err = pickFolderWindows()
	case "darwin":
		path, cancelled, err = pickFolderMacOS()
	default:
		path, cancelled, err = pickFolderLinux()
	}

	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if cancelled {
		writeJSON(w, 200, map[string]interface{}{
			"cancelled": true,
			"path":      "",
		})
		return
	}

	writeJSON(w, 200, map[string]string{"path": path})
}

// handleThumbnail proxies thumbnail fetching (solves browser being unable to reach i.ytimg.com directly)
// GET /api/thumbnail?url=...&proxy=...
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	imgURL := r.URL.Query().Get("url")
	if imgURL == "" {
		writeError(w, 400, "Missing url parameter")
		return
	}

	proxyStr := r.URL.Query().Get("proxy")

	var client *http.Client
	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			writeError(w, 400, "Invalid proxy address: "+err.Error())
			return
		}
		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		client = &http.Client{Transport: transport, Timeout: 30 * time.Second}
	} else {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Get(imgURL)
	if err != nil {
		writeError(w, 502, "Failed to fetch thumbnail: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, 502, fmt.Sprintf("Failed to fetch thumbnail: HTTP %d", resp.StatusCode))
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, resp.Body)
}

func pickFolderWindows() (path string, cancelled bool, err error) {
	// Uses OpenFileDialog: displays a modern file dialog,
	// user navigates into the target folder and clicks "Open" (FileName is just a placeholder, we take the directory part).
	// PowerShell outputs UTF-8 so that Chinese/Unicode paths are preserved through Go's cmd.Output().
	psScript := `
$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = 'Select save directory — Enter the target folder then click "Open"'
$d.Filter = 'All files (*.*)|*.*'
$d.CheckFileExists = $false
$d.CheckPathExists = $true
$d.FileName = '[Select This Folder]'
$d.Multiselect = $false
$d.RestoreDirectory = $true
if ($d.ShowDialog() -eq 'OK') { [System.IO.Path]::GetDirectoryName($d.FileName) } else { '' }
`
	cmd := exec.Command("powershell", "-sta", "-NoProfile", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("Failed to open folder picker dialog: %w", err)
	}
	result := strings.TrimSpace(string(output))
	// Safety net: if PowerShell somehow still produced garbled text (Unicode replacement chars),
	// retry with chcp 65001 to force UTF-8 code page before invoking powershell.
	if strings.ContainsRune(result, '�') || strings.ContainsRune(result, '?') {
		cmd2 := exec.Command("cmd", "/c", "chcp 65001 > nul && powershell -sta -NoProfile -Command "+psScript)
		output2, err2 := cmd2.Output()
		if err2 == nil {
			result = strings.TrimSpace(string(output2))
		}
	}
	if result == "" {
		return "", true, nil // user cancelled selection
	}
	return result, false, nil
}

func pickFolderMacOS() (path string, cancelled bool, err error) {
	cmd := exec.Command("osascript", "-e",
		`POSIX path of (choose folder with prompt "Select save directory")`)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", true, nil
		}
		return "", false, fmt.Errorf("Failed to open folder picker dialog: %w", err)
	}
	return strings.TrimSpace(string(output)), false, nil
}

func pickFolderLinux() (path string, cancelled bool, err error) {
	cmd := exec.Command("zenity", "--file-selection", "--directory",
		"--title=Select save directory")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", true, nil
		}
		return "", false, fmt.Errorf("Failed to open folder picker dialog (install zenity): %w", err)
	}
	return strings.TrimSpace(string(output)), false, nil
}
