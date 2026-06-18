package ytdlp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"FetchTubeWeb/internal/models"
)

// Language code to readable name mapping
var langMap = map[string]string{
	"en": "English", "zh": "Chinese", "ja": "Japanese", "ko": "Korean",
	"fr": "French", "de": "German", "es": "Spanish", "pt": "Portuguese",
	"ru": "Russian", "ar": "Arabic", "hi": "Hindi", "it": "Italian",
}

// Video codec compatibility weights
var codecRank = map[string]int{
	"avc1": 2000, "avc2": 2000, "avc3": 2000, "avc4": 2000, "h264": 2000,
	"vp9": 1000, "vp09": 1000,
	"av01": 0, "av1": 0,
}

// ParseVideoInfo converts yt-dlp raw info to VideoInfo model
func ParseVideoInfo(raw *RawInfo) *models.VideoInfo {
	allFormats := raw.Formats

	// Find best audio stream (skip DRC variants)
	var bestAudio *RawFormat
	for i := range allFormats {
		f := &allFormats[i]
		vcodec := strings.ToLower(f.Vcodec)
		acodec := strings.ToLower(f.Acodec)
		if vcodec == "none" && acodec != "none" {
			if strings.Contains(f.FormatID, "-drc") {
				continue
			}
			abr := toInt(f.ABR)
			if bestAudio == nil || abr > toInt(bestAudio.ABR) {
				bestAudio = f
			}
		}
	}

	// Collect best video stream per height
	dashVideo := make(map[int]*RawFormat)   // DASH video-only streams
	videoByHeight := make(map[int]*RawFormat) // Overall best video streams

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
			if betterFormat(f, existing, currentScore, existingScore) {
				videoByHeight[f.Height] = f
			}
		} else {
			videoByHeight[f.Height] = f
		}

		// Video-only stream (no audio)
		acodec := strings.ToLower(f.Acodec)
		if acodec == "none" {
			if existing, ok := dashVideo[f.Height]; ok {
				existingScore := toInt(existing.TBR) + getCodecScore(existing.Vcodec)
				if betterFormat(f, existing, currentScore, existingScore) {
					dashVideo[f.Height] = f
				}
			} else {
				dashVideo[f.Height] = f
			}
		}
	}

	// Build format list
	durationSeconds := toInt(raw.Duration)

	formats := make([]models.FormatOption, 0)
	seenHeights := make(map[int]bool)

	// Sort by height descending
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
		fileSizeMB := calcFileSizeMB(f.Filesize, f.FilesizeApprox, f.TBR, durationSeconds)

		if bestAudio != nil && strings.ToLower(f.Acodec) == "none" {
			formatID = f.FormatID + "+" + bestAudio.FormatID
			audioCodec = bestAudio.Acodec
			audioMB := calcFileSizeMB(bestAudio.Filesize, bestAudio.FilesizeApprox, bestAudio.TBR, durationSeconds)
			if fileSizeMB > 0 && audioMB > 0 {
				fileSizeMB = fileSizeMB + audioMB
			}
		} else {
			audioCodec = f.Acodec
		}

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

	// Also handle merged streams with both video and audio
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

		fileSizeMB := calcFileSizeMB(f.Filesize, f.FilesizeApprox, f.TBR, durationSeconds)

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

	// Sort by resolution and file size
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

	// Extract audio tracks
	audioTracks := extractAudioTracks(allFormats)

	// Thumbnail: prefer yt-dlp returned field, otherwise construct i.ytimg.com URL
	thumbnail := raw.Thumbnail
	if thumbnail == "" {
		thumbnail = buildThumbnailURL(raw.URL)
	}

	return &models.VideoInfo{
		Title:           raw.Title,
		URL:             raw.URL,
		ThumbnailURL:    thumbnail,
		DurationSeconds: durationSeconds,
		DurationStr:     FormatDuration(durationSeconds),
		Uploader:        raw.Uploader,
		ViewCount:       toInt(raw.ViewCount),
		LikeCount:       toInt(raw.LikeCount),
		Description:     truncate(raw.Description, 500),
		Formats:         formats,
		AudioTracks:     audioTracks,
	}
}

// extractAudioTracks extracts audio tracks
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
		// Skip DRC variants
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

	// Default language first
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

// ---- Helper functions ----

// hasSize checks if the format has actual file size data
func hasSize(f *RawFormat) bool {
	return toFloat(f.Filesize) > 0 || toFloat(f.FilesizeApprox) > 0
}

// betterFormat compares two formats, preferring ones with filesize data (avoids TBR estimation bias)
func betterFormat(newF, existing *RawFormat, newScore, existingScore int) bool {
	newHas := hasSize(newF)
	existingHas := hasSize(existing)
	if newHas != existingHas {
		return newHas
	}
	return newScore > existingScore
}

func getCodecScore(vcodec string) int {
	vc := strings.ToLower(vcodec)
	for prefix, score := range codecRank {
		if strings.HasPrefix(vc, prefix) {
			return score
		}
	}
	return 0
}

func calcFileSizeMB(fs, fsApprox, tbr interface{}, durationSeconds int) float64 {
	sz := toFloat(fs)
	if sz == 0 {
		sz = toFloat(fsApprox)
	}
	if sz == 0 {
		br := toFloat(tbr)
		if br > 0 && durationSeconds > 0 {
			sz = br * 1000 / 8 * float64(durationSeconds)
		}
	}
	mb := sz / (1024 * 1024)
	if mb < 0.01 {
		return 0
	}
	return float64(int(mb*10)) / 10
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

// buildThumbnailURL extracts video ID from YouTube URL to construct thumbnail URL
func buildThumbnailURL(webpageURL string) string {
	patterns := []string{
		`(?:youtube\.com/watch\?v=|youtu\.be/|youtube\.com/shorts/|youtube\.com/embed/|youtube\.com/v/)([a-zA-Z0-9_-]{11})`,
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		if m := re.FindStringSubmatch(webpageURL); len(m) >= 2 {
			return "https://i.ytimg.com/vi/" + m[1] + "/maxresdefault.jpg"
		}
	}
	return ""
}
