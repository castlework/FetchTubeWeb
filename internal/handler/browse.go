package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirEntry directory entry
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	IsDir bool  `json:"is_dir"`
}

// handleDrives returns Windows drive list
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

// handleBrowse browses directories
// GET /api/browse?path=C:\Users
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		// Return drive list
		s.handleDrives(w, r)
		return
	}

	// Security check: prevent path traversal
	absPath, err := filepath.Abs(path)
	if err != nil {
		writeError(w, 400, "Invalid path")
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		// Try returning parent directory contents
		parent := filepath.Dir(absPath)
		entries, err = os.ReadDir(parent)
		if err != nil {
			writeError(w, 400, "Cannot access directory: "+err.Error())
			return
		}
		absPath = parent
	}

	var result []DirEntry
	for _, e := range entries {
		// Skip hidden and system files
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

	// Sort by name
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	// Add parent directory
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
