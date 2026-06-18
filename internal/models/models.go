// Package models — 数据模型定义
package models

// FormatOption 单个可下载的视频格式（视频流 + 默认音频流）
type FormatOption struct {
	FormatID    string  `json:"format_id"`    // yt-dlp 格式 ID，如 "299+140"
	VideoID     string  `json:"video_id"`     // 纯视频流 ID，如 "299"（搭配自定义音轨）
	Resolution  string  `json:"resolution"`   // "1920x1080"
	FPS         int     `json:"fps"`          // 帧率
	Codec       string  `json:"codec"`        // 视频编码
	AudioCodec  string  `json:"audio_codec"`  // 默认音频编码
	FileSizeMB  float64 `json:"file_size_mb"` // 文件大小（MB）
	Ext         string  `json:"ext"`          // 文件扩展名
	Note        string  `json:"note"`         // 备注（如 "1080p60"）
}

// AudioTrack 单个音频轨道
type AudioTrack struct {
	FormatID string `json:"format_id"` // yt-dlp 格式 ID，如 "251-0"
	Language string `json:"language"`  // 语言标签，如 "English"
	Bitrate  int    `json:"abr"`       // 音频码率 (kbps)
	Codec    string `json:"codec"`     // 音频编码
	Note     string `json:"note"`      // 备注
}

// VideoInfo 视频完整元信息
type VideoInfo struct {
	Title           string        `json:"title"`
	URL             string        `json:"url"`
	ThumbnailURL    string        `json:"thumbnail_url"`
	DurationSeconds int           `json:"duration_seconds"`
	DurationStr     string        `json:"duration_str"`
	Uploader        string        `json:"uploader"`
	ViewCount       int           `json:"view_count"`
	LikeCount       int           `json:"like_count"`
	Description     string        `json:"description"`
	Formats         []FormatOption `json:"formats"`
	AudioTracks     []AudioTrack  `json:"audio_tracks"`
}

// ProgressMsg 下载进度消息（通过 WebSocket 推送）
type ProgressMsg struct {
	Status          string  `json:"status"`           // downloading | merging | finished | error | cancelled | retry
	Percent         float64 `json:"percent"`
	SpeedMBps       float64 `json:"speed_mbps"`
	DownloadedMB    float64 `json:"downloaded_mb"`
	TotalMB         float64 `json:"total_mb"`
	ETASeconds      int     `json:"eta_seconds"`
	ElapsedSeconds  int     `json:"elapsed_seconds"`
	Filename        string  `json:"filename"`
	FragmentIndex   int     `json:"fragment_index"`
	FragmentCount   int     `json:"fragment_count"`
	MergeElapsed    float64 `json:"merge_elapsed,omitempty"`
	MergeRemaining  int     `json:"merge_remaining,omitempty"`
	MergeDone       bool    `json:"merge_done,omitempty"`
	RetryAttempt    int     `json:"retry_attempt,omitempty"`
	RetryMax        int     `json:"retry_max,omitempty"`
	ErrorMessage    string  `json:"error_message,omitempty"`
}

// DownloadRequest 下载请求参数
type DownloadRequest struct {
	URL                string `json:"url"`
	FormatID           string `json:"format_id"`
	OutputExt          string `json:"output_ext"`
	SaveDir            string `json:"save_dir"`
	ConcurrentFragments int   `json:"concurrent_fragments"`
	Resume             bool   `json:"resume"`
	Proxy              string `json:"proxy,omitempty"`
	Cookies            string `json:"cookies,omitempty"`
	KeepTempFiles      bool   `json:"keep_temp_files"`
}

// RemoteTask VPS 上的下载任务
type RemoteTask struct {
	TaskID          string  `json:"task_id"`
	URL             string  `json:"url"`
	Title           string  `json:"title"`
	FormatNote      string  `json:"format_note"`
	OutputExt       string  `json:"output_ext"`
	Status          string  `json:"status"`
	Percent         float64 `json:"percent"`
	SpeedMBps       float64 `json:"speed_mbps"`
	DownloadedMB    float64 `json:"downloaded_mb"`
	TotalMB         float64 `json:"total_mb"`
	ETASeconds      int     `json:"eta_seconds"`
	ElapsedSeconds  int     `json:"elapsed_seconds"`
	ErrorMessage    string  `json:"error_message"`
	Filename        string  `json:"filename"`
	CreatedAt       string  `json:"created_at"`
}

// RemoteFile VPS 暂存文件
type RemoteFile struct {
	Filename  string  `json:"filename"`
	SizeMB    float64 `json:"size_mb"`
	CreatedAt string  `json:"created_at"`
}
