// Package models — data model definitions
package models

// FormatOption a single downloadable video format (video stream + default audio stream)
type FormatOption struct {
	FormatID    string  `json:"format_id"`    // yt-dlp format ID, e.g. "299+140"
	VideoID     string  `json:"video_id"`     // video-only stream ID, e.g. "299" (for custom audio tracks)
	Resolution  string  `json:"resolution"`   // "1920x1080"
	FPS         int     `json:"fps"`          // frame rate
	Codec       string  `json:"codec"`        // video codec
	AudioCodec  string  `json:"audio_codec"`  // default audio codec
	FileSizeMB  float64 `json:"file_size_mb"` // file size (MB)
	Ext         string  `json:"ext"`          // file extension
	Note        string  `json:"note"`         // note (e.g. "1080p60")
}

// AudioTrack a single audio track
type AudioTrack struct {
	FormatID string `json:"format_id"` // yt-dlp format ID, e.g. "251-0"
	Language string `json:"language"`  // language label, e.g. "English"
	Bitrate  int    `json:"abr"`       // audio bitrate (kbps)
	Codec    string `json:"codec"`     // audio codec
	Note     string `json:"note"`      // note
}

// VideoInfo complete video metadata
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

// ProgressMsg download progress message (pushed via WebSocket)
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

// DownloadRequest download request parameters
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
