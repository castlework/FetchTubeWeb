// Package config — JSON config file persistence
package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// AppConfig application configuration
type AppConfig struct {
	Local LocalConfig `json:"local"`
}

// LocalConfig local download configuration
type LocalConfig struct {
	LastURL             string `json:"last_url"`
	ProxyMode           string `json:"proxy_mode"`            // "None" | "HTTP" | "SOCKS5"
	ProxyHost           string `json:"proxy_host"`
	ProxyPort           string `json:"proxy_port"`
	OutputFormat        string `json:"output_format"`         // "mp4" | "webm" | "mkv"
	ConcurrentFragments int    `json:"concurrent_fragments"`  // 1-32
	Cookies             string `json:"cookies"`               // "None" | "Chrome" | "Firefox" | ...
	CookiesPath         string `json:"cookies_path"`
	SaveDir             string `json:"save_dir"`
	KeepTempFiles       bool   `json:"keep_temp_files"`
}


// DefaultConfig returns default configuration
func DefaultConfig() AppConfig {
	return AppConfig{
		Local: LocalConfig{
			ProxyMode:           "无",
			ProxyHost:           "127.0.0.1",
			ProxyPort:           "1080",
			OutputFormat:        "mkv",
			ConcurrentFragments: 8,
			Cookies:             "无",
			CookiesPath:         "",
			SaveDir:             "",
			KeepTempFiles:       false,
		},
	}
}

// configPath returns the config file path
func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".FetchTubeWeb_config.json"
	}
	return filepath.Join(home, ".FetchTubeWeb_config.json")
}

// Load loads config, returns defaults if file doesn't exist
// Path returns the config file path
func Path() string {
	return configPath()
}

func Load() AppConfig {
	cfg := DefaultConfig()
	path := configPath()

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[config] config not found, using defaults (%s)", path)
		return cfg
	}
	log.Printf("[config] loaded: %s", path)

	var saved AppConfig
	if err := json.Unmarshal(data, &saved); err != nil {
		return cfg
	}

	// Merge defaults
	merge(&cfg, &saved)
	return cfg
}

// Save writes config to disk
func Save(cfg AppConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := configPath()
	log.Printf("[config] saved: %s", path)
	return os.WriteFile(path, data, 0644)
}

// merge copies non-zero values from saved into dst
func merge(dst *AppConfig, saved *AppConfig) {
	if saved.Local.LastURL != "" {
		dst.Local.LastURL = saved.Local.LastURL
	}
	if saved.Local.ProxyMode != "" {
		dst.Local.ProxyMode = saved.Local.ProxyMode
	}
	if saved.Local.ProxyHost != "" {
		dst.Local.ProxyHost = saved.Local.ProxyHost
	}
	if saved.Local.ProxyPort != "" {
		dst.Local.ProxyPort = saved.Local.ProxyPort
	}
	if saved.Local.OutputFormat != "" {
		dst.Local.OutputFormat = saved.Local.OutputFormat
	}
	if saved.Local.ConcurrentFragments > 0 {
		dst.Local.ConcurrentFragments = saved.Local.ConcurrentFragments
	}
	if saved.Local.Cookies != "" {
		dst.Local.Cookies = saved.Local.Cookies
	}
	if saved.Local.CookiesPath != "" {
		dst.Local.CookiesPath = saved.Local.CookiesPath
	}
	if saved.Local.SaveDir != "" {
		dst.Local.SaveDir = saved.Local.SaveDir
	}
	// bool field: only override when saved is true
	if saved.Local.KeepTempFiles {
		dst.Local.KeepTempFiles = true
	}
}
