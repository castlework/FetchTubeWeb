package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirEntry 目录条目
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	IsDir bool  `json:"is_dir"`
}

// handleDrives 返回 Windows 驱动器列表
// GET /api/drives
func (s *Server) handleDrives(w http.ResponseWriter, r *http.Request) {
	var drives []DirEntry
	for letter := 'A'; letter <= 'Z'; letter++ {
		path := string(letter) + ":\\"
		if _, err := os.Stat(path); err == nil {
			drives = append(drives, DirEntry{
				Name:  string(letter) + ":",
				Path:  path,
				IsDir: true,
			})
		}
	}
	writeJSON(w, 200, drives)
}

// handleBrowse 浏览目录
// GET /api/browse?path=C:\Users
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		// 返回驱动器列表
		s.handleDrives(w, r)
		return
	}

	// 安全检查：防止路径穿越
	absPath, err := filepath.Abs(path)
	if err != nil {
		writeError(w, 400, "无效的路径")
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		// 尝试返回父目录的内容
		parent := filepath.Dir(absPath)
		entries, err = os.ReadDir(parent)
		if err != nil {
			writeError(w, 400, "无法访问该目录: "+err.Error())
			return
		}
		absPath = parent
	}

	var result []DirEntry
	for _, e := range entries {
		// 跳过隐藏文件和系统文件
		if strings.HasPrefix(e.Name(), "$") || strings.HasPrefix(e.Name(), "System") {
			continue
		}
		if e.IsDir() {
			result = append(result, DirEntry{
				Name:  e.Name(),
				Path:  filepath.Join(absPath, e.Name()),
				IsDir: true,
			})
		}
	}

	// 按名称排序
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	// 添加父目录
	parent := filepath.Dir(absPath)
	if parent != absPath {
		result = append([]DirEntry{{
			Name:  "..",
			Path:  parent,
			IsDir: true,
		}}, result...)
	}

	writeJSON(w, 200, map[string]interface{}{
		"current": absPath,
		"entries": result,
	})
}
