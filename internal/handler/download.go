package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"FetchTubeWeb/internal/ytdlp"
)

// handleInfo 处理视频信息提取请求
// GET /api/info?url=...&proxy=...&cookies=...
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		writeError(w, 400, "缺少 url 参数")
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

// handleHealth 健康检查
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

// handleDownload 加入下载队列
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
		writeError(w, 400, "无效的请求体: "+err.Error())
		return
	}

	if req.URL == "" || req.FormatID == "" {
		writeError(w, 400, "url 和 format_id 为必填项")
		return
	}

	if req.SaveDir == "" {
		writeError(w, 400, "save_dir 为必填项")
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

// handleListTasks 列出所有任务
// GET /api/tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.tasks.List())
}

// handleCancelTask 取消指定任务
// POST /api/tasks/{taskID}/cancel
func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		writeError(w, 400, "缺少 taskID")
		return
	}

	if err := s.tasks.Cancel(taskID); err != nil {
		writeError(w, 404, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

// handleDeleteTask 从任务列表中删除指定任务（不影响正在进行的下载）
// DELETE /api/tasks/{taskID}
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		writeError(w, 400, "缺少 taskID")
		return
	}

	if err := s.tasks.Remove(taskID); err != nil {
		writeError(w, 404, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleBatchDeleteTasks 批量删除任务
// POST /api/tasks/batch-delete
func (s *Server) handleBatchDeleteTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskIDs []string `json:"task_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效的请求体: "+err.Error())
		return
	}
	if len(req.TaskIDs) == 0 {
		writeError(w, 400, "task_ids 不能为空")
		return
	}

	count := s.tasks.RemoveBatch(req.TaskIDs)
	writeJSON(w, 200, map[string]interface{}{
		"status":  "deleted",
		"deleted": count,
	})
}

// handleOpenDir 在文件资源管理器中打开指定目录
// POST /api/open-dir
func (s *Server) handleOpenDir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效的请求体: "+err.Error())
		return
	}
	if req.Path == "" {
		writeError(w, 400, "缺少 path 参数")
		return
	}

	// 先检查目录是否存在
	if info, err := os.Stat(req.Path); err != nil || !info.IsDir() {
		writeError(w, 400, "目录不存在: "+req.Path)
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// explorer.exe 会通过 COM 委托给已有 shell 进程然后退出，
		// 其退出码不可靠（有时返回 1 但窗口已正常打开），
		// 因此使用 Start() 启动后立即分离，不等待退出。
		cmd = exec.Command("explorer", req.Path)
		if err := cmd.Start(); err != nil {
			writeError(w, 500, "打开目录失败: "+err.Error())
			return
		}
		// 释放进程句柄，避免僵尸进程
		go cmd.Wait()
	case "darwin":
		cmd = exec.Command("open", req.Path)
		if err := cmd.Start(); err != nil {
			writeError(w, 500, "打开目录失败: "+err.Error())
			return
		}
		go cmd.Wait()
	default:
		cmd = exec.Command("xdg-open", req.Path)
		if err := cmd.Start(); err != nil {
			writeError(w, 500, "打开目录失败: "+err.Error())
			return
		}
		go cmd.Wait()
	}

	writeJSON(w, 200, map[string]string{"status": "opened"})
}

// handlePickFolder 弹出原生系统文件夹选择对话框，返回用户选中的路径
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

func pickFolderWindows() (path string, cancelled bool, err error) {
	// 用 OpenFileDialog 实现：外观就是现代文件对话框，
	// 用户进入目标文件夹后直接点"打开"即可（FileName 只是占位提示符，取目录部分）
	psScript := `
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = '选择保存目录 — 进入目标文件夹后点击"打开"'
$d.Filter = '所有文件 (*.*)|*.*'
$d.CheckFileExists = $false
$d.CheckPathExists = $true
$d.FileName = '【选择此文件夹】'
$d.Multiselect = $false
$d.RestoreDirectory = $true
if ($d.ShowDialog() -eq 'OK') { [System.IO.Path]::GetDirectoryName($d.FileName) } else { '' }
`
	cmd := exec.Command("powershell", "-sta", "-NoProfile", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("调出文件夹选择对话框失败: %w", err)
	}
	result := strings.TrimSpace(string(output))
	if result == "" {
		return "", true, nil // 用户取消了选择
	}
	return result, false, nil
}

func pickFolderMacOS() (path string, cancelled bool, err error) {
	cmd := exec.Command("osascript", "-e",
		`POSIX path of (choose folder with prompt "选择保存目录")`)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", true, nil
		}
		return "", false, fmt.Errorf("调出文件夹选择对话框失败: %w", err)
	}
	return strings.TrimSpace(string(output)), false, nil
}

func pickFolderLinux() (path string, cancelled bool, err error) {
	cmd := exec.Command("zenity", "--file-selection", "--directory",
		"--title=选择保存目录")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", true, nil
		}
		return "", false, fmt.Errorf("调出文件夹选择对话框失败（请安装 zenity）: %w", err)
	}
	return strings.TrimSpace(string(output)), false, nil
}
