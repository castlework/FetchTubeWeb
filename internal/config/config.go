// Package config — JSON 配置文件持久化
package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// AppConfig 应用配置
type AppConfig struct {
	Local LocalConfig `json:"local"`
}

// LocalConfig 本地下载配置
type LocalConfig struct {
	LastURL             string `json:"last_url"`
	ProxyMode           string `json:"proxy_mode"`            // "无" | "HTTP" | "SOCKS5"
	ProxyHost           string `json:"proxy_host"`
	ProxyPort           string `json:"proxy_port"`
	OutputFormat        string `json:"output_format"`         // "mp4" | "webm" | "mkv"
	ConcurrentFragments int    `json:"concurrent_fragments"`  // 1-32
	Cookies             string `json:"cookies"`               // "无" | "Chrome" | "Firefox" | ...
	CookiesPath         string `json:"cookies_path"`
	SaveDir             string `json:"save_dir"`
	KeepTempFiles       bool   `json:"keep_temp_files"`
}


// DefaultConfig 返回默认配置
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

// configPath 返回配置文件路径
func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".FetchTubeWeb_config.json"
	}
	return filepath.Join(home, ".FetchTubeWeb_config.json")
}

// Load 加载配置，文件不存在则返回默认值
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

	// 合并默认值
	merge(&cfg, &saved)
	return cfg
}

// Save 保存配置到磁盘
func Save(cfg AppConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := configPath()
	log.Printf("[config] saved: %s", path)
	return os.WriteFile(path, data, 0644)
}

// merge 将 saved 中的非零值合并到 dst 中
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
	// bool 字段特殊处理：仅当 saved 中为 true 才覆盖
	if saved.Local.KeepTempFiles {
		dst.Local.KeepTempFiles = true
	}
}
