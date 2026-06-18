package ytdlp

import (
	"fmt"
	"sort"
	"strings"

	"youtube-downloader/internal/models"
)

// 语言代码到可读名称的映射
var langMap = map[string]string{
	"en": "English", "zh": "Chinese", "ja": "Japanese", "ko": "Korean",
	"fr": "French", "de": "German", "es": "Spanish", "pt": "Portuguese",
	"ru": "Russian", "ar": "Arabic", "hi": "Hindi", "it": "Italian",
}

// 视频编码器兼容性权重
var codecRank = map[string]int{
	"avc1": 2000, "avc2": 2000, "avc3": 2000, "avc4": 2000, "h264": 2000,
	"vp9": 1000, "vp09": 1000,
	"av01": 0, "av1": 0,
}

// ParseVideoInfo 将 yt-dlp 原始信息转换为 VideoInfo 模型
func ParseVideoInfo(raw *RawInfo) *models.VideoInfo {
	allFormats := raw.Formats

	// 查找最佳音频流
	var bestAudio *RawFormat
	for i := range allFormats {
		f := &allFormats[i]
		vcodec := strings.ToLower(f.Vcodec)
		acodec := strings.ToLower(f.Acodec)
		if vcodec == "none" && acodec != "none" {
			abr := toInt(f.ABR)
			if bestAudio == nil || abr > toInt(bestAudio.ABR) {
				bestAudio = f
			}
		}
	}

	// 收集每个高度的最佳视频流
	dashVideo := make(map[int]*RawFormat)   // DASH 纯视频流
	videoByHeight := make(map[int]*RawFormat) // 综合最佳视频流

	for i := range allFormats {
		f := &allFormats[i]
		vcodec := strings.ToLower(f.Vcodec)
		if vcodec == "none" {
			continue
		}
		if f.Height == 0 {
			continue
		}

		tbr := toInt(f.TBR)
		cs := getCodecScore(f.Vcodec)
		currentScore := tbr + cs

		if existing, ok := videoByHeight[f.Height]; ok {
			existingScore := toInt(existing.TBR) + getCodecScore(existing.Vcodec)
			if currentScore > existingScore {
				videoByHeight[f.Height] = f
			}
		} else {
			videoByHeight[f.Height] = f
		}

		// 纯视频流（无音频）
		acodec := strings.ToLower(f.Acodec)
		if acodec == "none" {
			if existing, ok := dashVideo[f.Height]; ok {
				existingScore := toInt(existing.TBR) + getCodecScore(existing.Vcodec)
				if currentScore > existingScore {
					dashVideo[f.Height] = f
				}
			} else {
				dashVideo[f.Height] = f
			}
		}
	}

	// 构建格式列表
	formats := make([]models.FormatOption, 0)
	seenHeights := make(map[int]bool)

	// 按高度降序排序
	heights := make([]int, 0, len(videoByHeight))
	for h := range videoByHeight {
		heights = append(heights, h)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(heights)))

	for _, height := range heights {
		f := videoByHeight[height]
		dashF := dashVideo[height]

		if seenHeights[height] {
			continue
		}
		seenHeights[height] = true

		formatID := f.FormatID
		audioCodec := "none"
		totalSize := float64(toInt(f.Filesize) + toInt(f.FilesizeApprox))

		if bestAudio != nil {
			formatID = f.FormatID + "+" + bestAudio.FormatID
			audioCodec = bestAudio.Acodec
			audioSize := float64(toInt(bestAudio.Filesize) + toInt(bestAudio.FilesizeApprox))
			if totalSize > 0 && audioSize > 0 {
				totalSize = totalSize + audioSize
			}
		} else {
			audioCodec = f.Acodec
		}

		fileSizeMB := totalSize / (1024 * 1024)
		if fileSizeMB < 0.01 {
			fileSizeMB = 0
		}
		fileSizeMB = float64(int(fileSizeMB*10)) / 10 // 保留1位小数

		note := f.FormatNote
		if note == "" || note == "(default)" || note == "unknown" {
			fps := toInt(f.FPS)
			if fps >= 50 {
				note = formatNote(height, fps)
			} else {
				note = formatNote(height, 0)
			}
		}

		vidOnlyID := f.FormatID
		if dashF != nil {
			vidOnlyID = dashF.FormatID
		}

		formats = append(formats, models.FormatOption{
			FormatID:   formatID,
			VideoID:    vidOnlyID,
			Resolution: formatResolution(f.Width, height),
			FPS:        toInt(f.FPS),
			Codec:      f.Vcodec,
			AudioCodec: audioCodec,
			FileSizeMB: fileSizeMB,
			Ext:        f.Ext,
			Note:       note,
		})
	}

	// 也处理同时有视频和音频的合并流
	for i := range allFormats {
		f := &allFormats[i]
		vcodec := strings.ToLower(f.Vcodec)
		acodec := strings.ToLower(f.Acodec)
		if vcodec == "none" || acodec == "none" {
			continue
		}
		if seenHeights[f.Height] {
			continue
		}
		seenHeights[f.Height] = true

		totalSize := float64(toInt(f.Filesize) + toInt(f.FilesizeApprox))
		fileSizeMB := totalSize / (1024 * 1024)
		if fileSizeMB < 0.01 {
			fileSizeMB = 0
		}
		fileSizeMB = float64(int(fileSizeMB*10)) / 10

		note := f.FormatNote
		if note == "" || note == "(default)" || note == "unknown" {
			note = formatNote(f.Height, 0)
		}

		dashF := dashVideo[f.Height]
		vidOnlyID := f.FormatID
		if dashF != nil {
			vidOnlyID = dashF.FormatID
		}

		formats = append(formats, models.FormatOption{
			FormatID:   f.FormatID,
			VideoID:    vidOnlyID,
			Resolution: formatResolution(f.Width, f.Height),
			FPS:        toInt(f.FPS),
			Codec:      f.Vcodec,
			AudioCodec: f.Acodec,
			FileSizeMB: fileSizeMB,
			Ext:        f.Ext,
			Note:       note,
		})
	}

	// 按分辨率和文件大小排序
	sort.Slice(formats, func(i, j int) bool {
		a := formats[i]
		b := formats[j]
		aH := parseHeight(a.Resolution)
		bH := parseHeight(b.Resolution)
		if aH != bH {
			return aH > bH
		}
		return a.FileSizeMB > b.FileSizeMB
	})

	// 提取音频轨道
	audioTracks := extractAudioTracks(allFormats)

	// 缩略图
	thumbnail := raw.Thumbnail
	if thumbnail == "" {
		thumbnail = raw.URL
	}

	duration := toInt(raw.Duration)

	return &models.VideoInfo{
		Title:           raw.Title,
		URL:             raw.URL,
		ThumbnailURL:    thumbnail,
		DurationSeconds: duration,
		DurationStr:     FormatDuration(duration),
		Uploader:        raw.Uploader,
		ViewCount:       toInt(raw.ViewCount),
		LikeCount:       toInt(raw.LikeCount),
		Description:     truncate(raw.Description, 500),
		Formats:         formats,
		AudioTracks:     audioTracks,
	}
}

// extractAudioTracks 提取音频轨道
func extractAudioTracks(formats []RawFormat) []models.AudioTrack {
	type key struct {
		lang  string
		codec string
	}
	seen := make(map[key]int)
	tracks := make([]models.AudioTrack, 0)

	for _, f := range formats {
		vcodec := strings.ToLower(f.Vcodec)
		acodec := strings.ToLower(f.Acodec)
		if vcodec != "none" || acodec == "none" {
			continue
		}
		// 跳过 DRC 变体
		if strings.Contains(f.FormatID, "-drc") {
			continue
		}

		abr := toInt(f.ABR)
		lang := f.Language
		if lang == "" {
			lang = "unknown"
		}
		codec := f.Acodec

		k := key{lang, codec}
		if idx, ok := seen[k]; ok {
			if abr > tracks[idx].Bitrate {
				tracks[idx].Bitrate = abr
				tracks[idx].FormatID = f.FormatID
				tracks[idx].Note = f.FormatNote
			}
		} else {
			seen[k] = len(tracks)
			langDisplay := langMap[lang]
			if langDisplay == "" {
				if len(lang) >= 2 {
					langDisplay = langMap[lang[:2]]
				}
				if langDisplay == "" {
					langDisplay = lang
				}
			}
			tracks = append(tracks, models.AudioTrack{
				FormatID: f.FormatID,
				Language: langDisplay,
				Bitrate:  abr,
				Codec:    codec,
				Note:     f.FormatNote,
			})
		}
	}

	// 默认语言排前面
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].Language == "English" {
			return true
		}
		if tracks[j].Language == "English" {
			return false
		}
		return tracks[i].Bitrate > tracks[j].Bitrate
	})

	return tracks
}

// ---- 辅助函数 ----

func getCodecScore(vcodec string) int {
	vc := strings.ToLower(vcodec)
	for prefix, score := range codecRank {
		if strings.HasPrefix(vc, prefix) {
			return score
		}
	}
	return 0
}

func formatNote(height, fps int) string {
	if fps >= 50 {
		return formatInt(height) + "p60"
	}
	return formatInt(height) + "p"
}

func formatResolution(width, height int) string {
	if width == 0 {
		width = height * 16 / 9
	}
	return formatInt(width) + "x" + formatInt(height)
}

func formatInt(n int) string {
	return fmt.Sprintf("%d", n)
}

func parseHeight(resolution string) int {
	parts := strings.Split(resolution, "x")
	if len(parts) >= 2 {
		h := 0
		fmt.Sscanf(parts[1], "%d", &h)
		return h
	}
	return 0
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}
