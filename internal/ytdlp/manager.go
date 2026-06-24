package ytdlp

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Merge phase timeout config
const (
	MergeTimeout   = 300 * time.Second // Merge timeout: 5 minutes (large files need more time)
	MergeStallSecs = 30 * time.Second
	MaxRetries     = 2
)

// ProgressCallback progress callback function type
type ProgressCallback func(ProgressData)

// ProgressData progress data
type ProgressData struct {
	TaskID         string  `json:"task_id"`
	Status         string  `json:"status"`
	Percent        float64 `json:"percent"`
	SpeedMBps      float64 `json:"speed_mbps"`
	DownloadedMB   float64 `json:"downloaded_mb"`
	TotalMB        float64 `json:"total_mb"`
	ETASeconds     int     `json:"eta_seconds"`
	ElapsedSeconds int     `json:"elapsed_seconds"`
	Filename       string  `json:"filename"`
	FragmentIndex  int     `json:"fragment_index"`
	FragmentCount  int     `json:"fragment_count"`
	MergeElapsed   float64 `json:"merge_elapsed,omitempty"`
	MergeRemaining int     `json:"merge_remaining,omitempty"`
	MergeDone      bool    `json:"merge_done,omitempty"`
	RetryAttempt   int     `json:"retry_attempt,omitempty"`
	RetryMax       int     `json:"retry_max,omitempty"`
	ErrorMessage   string  `json:"error_message,omitempty"`
	Title          string  `json:"title,omitempty"`
	URL            string  `json:"url,omitempty"`
	SaveDir        string  `json:"save_dir,omitempty"`
	// Completion stats (sent with finished status, persisted by frontend)
	AvgSpeedMBps      float64 `json:"avg_speed_mbps,omitempty"`
	FinalSizeMB       float64 `json:"final_size_mb,omitempty"`
	DownloadElapsed   int     `json:"download_elapsed,omitempty"`
	MergeElapsedFinal float64 `json:"merge_elapsed_final,omitempty"`
}

// DownloadManager manages a single download task lifecycle
type DownloadManager struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	cancelFn  context.CancelFunc
	cancelled atomic.Bool
	running   atomic.Bool
}

// NewDownloadManager creates a new download manager
func NewDownloadManager() *DownloadManager {
	return &DownloadManager{}
}

// IsRunning returns whether a download is in progress
func (m *DownloadManager) IsRunning() bool {
	return m.running.Load()
}

// Cancel cancels the current download (thread-safe, idempotent)
func (m *DownloadManager) Cancel() {
	if !m.cancelled.Swap(true) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.cancelFn != nil {
			m.cancelFn()
		}
		if m.cmd != nil && m.cmd.Process != nil {
			KillProcessTree(m.cmd)
		}
	}
}

// Download executes the download (blocking), reports progress via callback
func (m *DownloadManager) Download(opts DownloadOptions, callback ProgressCallback) error {
	m.cancelled.Store(false)
	m.running.Store(true)
	defer m.running.Store(false)

	// Per-task temp directory isolates parallel downloads from each other
	if opts.TempDir == "" {
		opts.TempDir = filepath.Join(opts.SaveDir, fmt.Sprintf(".ytdl_%d", time.Now().UnixNano()))
	}
	os.MkdirAll(opts.TempDir, 0755)

	cleanup := func() {
		if !opts.KeepTempFiles {
			_ = os.RemoveAll(opts.TempDir)
		}
	}

	for attempt := 1; attempt <= MaxRetries+1; attempt++ {
		if m.cancelled.Load() {
			callback(ProgressData{Status: "cancelled"})
			cleanup()
			return nil
		}

		if attempt > 1 {
			callback(ProgressData{
				Status:       "retry",
				Percent:      100.0,
				RetryAttempt: attempt - 1,
				RetryMax:     MaxRetries,
			})
		}

		result := m.runOneDownload(opts, callback)

		switch result {
		case "done":
			moveFinalFiles(opts.TempDir, opts.SaveDir)
			cleanup()
			return nil
		case "cancelled":
			callback(ProgressData{Status: "cancelled"})
			cleanup()
			return nil
		case "timeout":
			if attempt > MaxRetries {
				callback(ProgressData{
					Status:       "error",
					ErrorMessage: fmt.Sprintf("Merge timed out (retried %d times).\nTry a lower resolution or check disk space.", MaxRetries),
				})
				cleanup()
				return fmt.Errorf("merge timed out")
			}
			continue // keep temp files for retry
		case "error":
			cleanup()
			return nil
		}
	}

	cleanup()
	return nil
}

// runOneDownload executes one download attempt, returns result status
func (m *DownloadManager) runOneDownload(opts DownloadOptions, callback ProgressCallback) string {

	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancelFn = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.cancelFn = nil
		m.mu.Unlock()
		cancel()
	}()

	ytdlp := FindYtDlp()
	if ytdlp == "" {
		callback(ProgressData{Status: "error", ErrorMessage: "yt-dlp.exe not found"})
		return "error"
	}

	log.Printf("[download] save dir: %s", opts.SaveDir)
	if opts.Cookies != "" {
		log.Printf("[download] cookies: %s", opts.Cookies)
	}

	args := BuildDownloadArgs(opts)
	cmd := exec.CommandContext(ctx, ytdlp, args...)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1")
	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		callback(ProgressData{Status: "error", ErrorMessage: err.Error()})
		return "error"
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		callback(ProgressData{Status: "error", ErrorMessage: err.Error()})
		return "error"
	}

	if err := cmd.Start(); err != nil {
		callback(ProgressData{Status: "error", ErrorMessage: err.Error()})
		return "error"
	}

	// Continuously consume stderr (yt-dlp ERROR output goes to stderr),
	// accumulate in stderrBuf for error diagnosis after process exits.
	// Note: confirmed that yt-dlp [download] progress lines actually go to stdout, parsed by the main loop below.
	var stderrBuf strings.Builder
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		sc.Split(scanProgressLines)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			stderrBuf.WriteString(line + "\n")
		}
	}()

	downloadStart := time.Now()
	mergePhase := false
	mergeStart := time.Time{}
	lastMsgTime := time.Now()
	var lastFileSize int64 = 0

	// Key fix: yt-dlp progress info outputs to stdout, but defaults to \r for in-place refresh.
	// bufio.Scanner with default \n split would miss all progress lines.
	// Solution: 1) BuildDownloadArgs adds --newline to make progress lines use \n;
	//           2) Custom SplitFunc splits on both \r and \n, as a safety net.
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // large buffer
	scanner.Split(scanProgressLines)

	for scanner.Scan() {
		now := time.Now()

		// Check cancellation
		if m.cancelled.Load() {
			KillProcessTree(cmd)
			return "cancelled"
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		data := parseProgressLine(line)
		if data == nil {
			continue
		}

		lastMsgTime = now

		// Downloading — compute elapsed time
		if data.Status == "downloading" {
			data.ElapsedSeconds = int(now.Sub(downloadStart).Seconds())
		}

		if data.Status == "merging" && !mergePhase {
			mergePhase = true
			mergeStart = now
			// Entering merge phase, print newline to break the download progress bar
			fmt.Fprintln(os.Stdout)
		}

		if mergePhase {
			elapsed := now.Sub(mergeStart).Seconds()
			data.MergeElapsed = elapsed
			data.MergeRemaining = int(MergeTimeout.Seconds()) - int(elapsed)
			if data.MergeRemaining < 0 {
				data.MergeRemaining = 0
			}
		}

		// Console real-time display of parsed progress (speed/size/elapsed/remaining/progress bar)
		printProgressConsole(*data)

		callback(*data)

		if data.Status == "error" {
			fmt.Fprintln(os.Stdout)
			KillProcessTree(cmd)
			return "error"
		}

		// Merge phase timeout detection
		if mergePhase {
			elapsed := now.Sub(mergeStart)
			if elapsed > MergeTimeout {
				fmt.Fprintln(os.Stdout)
				KillProcessTree(cmd)
				return "timeout"
			}
			// Stall detection
			if now.Sub(lastMsgTime) > MergeStallSecs {
				outputFiles := findOutputFiles(opts.TempDir)
				if len(outputFiles) > 0 {
					stat, err := os.Stat(outputFiles[0])
					if err == nil {
						currentSize := stat.Size()
						if currentSize == lastFileSize && currentSize > 0 {
							// File size is stable → merge is complete
							KillProcessTree(cmd)
							data := ProgressData{
								Status:         "merging",
								Percent:        100.0,
								MergeElapsed:   elapsed.Seconds(),
								MergeRemaining: 0,
								MergeDone:      true,
							}
							callback(data)
							sendFinishedStats(callback, opts, downloadStart, mergeStart, true)
							return "done"
						}
						lastFileSize = currentSize
					}
				}
			}
		}
	}

	// Wait for process to end
	_ = cmd.Wait()

	// Check stderr for errors
	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		errMsg := stderrBuf.String()
		if errMsg != "" {
			callback(ProgressData{Status: "error", ErrorMessage: translateSimple(errMsg)})
			return "error"
		}
	}

	// Send finished callback with stats (avg speed / actual file size / download time / merge time)
	sendFinishedStats(callback, opts, downloadStart, mergeStart, mergePhase)
	return "done"
}

// parseProgressLine parses yt-dlp progress output line
// yt-dlp uses --print progress or parses default output in newer versions
// We extract progress via ANSI progress lines in stderr
func parseProgressLine(line string) *ProgressData {
	// yt-dlp progress line format examples:
	// [download]   5.0% of ~100.00MiB at  2.50MiB/s ETA 00:38 (frag 3/8)
	// [download] 100% of 100.00MiB in 00:00:40
	// [ExtractAudio] ...
	// [Merger] Merging formats into "file.mkv"

	line = strings.TrimSpace(line)

	// Download progress
	if strings.Contains(line, "[download]") && strings.Contains(line, "%") {
		data := &ProgressData{Status: "downloading"}

		// Percentage
		if re := regexp.MustCompile(`(\d+\.?\d*)%`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 2 {
				fmt.Sscanf(m[1], "%f", &data.Percent)
			}
		}

		// Download size
		if re := regexp.MustCompile(`of\s+~?(\d+\.?\d*)([KMG])iB`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 3 {
				size, _ := parseSize(m[1], m[2])
				data.TotalMB = size
			}
		}

		// Downloaded amount = percentage × total size
		if data.Percent > 0 && data.TotalMB > 0 {
			data.DownloadedMB = data.Percent / 100.0 * data.TotalMB
		}

		// Speed
		if re := regexp.MustCompile(`at\s+(\d+\.?\d*)([KMG])iB/s`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 3 {
				speed, _ := parseSize(m[1], m[2])
				data.SpeedMBps = speed
			}
		}

		// ETA
		if re := regexp.MustCompile(`ETA\s+(\d+):?(\d*)`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 2 {
				minutes := 0
				seconds := 0
				fmt.Sscanf(m[1], "%d", &minutes)
				if len(m) >= 3 && m[2] != "" {
					fmt.Sscanf(m[2], "%d", &seconds)
				}
				data.ETASeconds = minutes*60 + seconds
			}
		}

		// Fragment info
		if re := regexp.MustCompile(`frag\s+(\d+)/(\d+)`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 3 {
				fmt.Sscanf(m[1], "%d", &data.FragmentIndex)
				fmt.Sscanf(m[2], "%d", &data.FragmentCount)
			}
		}

		return data
	}

	// Download complete (single stream)
	if strings.Contains(line, "[download]") && strings.Contains(line, "100%") {
		return &ProgressData{Status: "downloading", Percent: 100.0}
	}

	// Merge phase
	if strings.Contains(line, "[Merger]") || strings.Contains(line, "[VideoConvertor]") {
		return &ProgressData{Status: "merging", Percent: 100.0}
	}

	// ffmpeg merge
	if strings.Contains(line, "[ffmpeg]") {
		return &ProgressData{Status: "merging", Percent: 100.0}
	}

	return nil
}

// parseSize parses size with unit into MB
func parseSize(value, unit string) (float64, error) {
	var num float64
	fmt.Sscanf(value, "%f", &num)
	switch unit {
	case "K":
		return num / 1024, nil
	case "M":
		return num, nil
	case "G":
		return num * 1024, nil
	}
	return num, nil
}

// translateSimple simple error translation
func translateSimple(msg string) string {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "sign in") || strings.Contains(lower, "bot") {
		return "YouTube requires identity verification. Please use Cookies login."
	}
	if strings.Contains(lower, "429") {
		return "Too many requests (429). Please try again later."
	}
	if strings.Contains(lower, "unavailable") || strings.Contains(lower, "private") {
		return "This video is unavailable (deleted or private)."
	}
	return msg
}

// moveFinalFiles moves the final merged video from the per-task temp directory
// to the save directory, skipping intermediate format files (kept by --keep-video,
// named like "Title.f137.mp4") and temp/fragment files — those stay in TempDir
// and get cleaned up by os.RemoveAll.
func moveFinalFiles(tempDir, saveDir string) {
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return
	}
	formatFileRe := regexp.MustCompile(`\.f\d+\.`)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if formatFileRe.MatchString(name) {
			continue
		}
		if strings.Contains(name, ".ytdl") || strings.Contains(name, ".part") || strings.Contains(name, ".temp.") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".mp4" || ext == ".webm" || ext == ".mkv" {
			src := filepath.Join(tempDir, name)
			dst := filepath.Join(saveDir, name)
			// On Windows, os.Rename fails if dst exists — remove old file first
			_ = os.Remove(dst)
			if err := os.Rename(src, dst); err != nil {
				log.Printf("[move] failed to move final output %s: %v", name, err)
			} else {
				log.Printf("[move] final output: %s -> %s", name, saveDir)
			}
		}
	}
}

func findOutputFiles(saveDir string) []string {
	var files []string
	entries, _ := os.ReadDir(saveDir)
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && !strings.Contains(name, ".temp.") &&
			(strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".webm") || strings.HasSuffix(name, ".mkv")) {
			files = append(files, filepath.Join(saveDir, name))
		}
	}
	return files
}

// FormatDuration formats seconds to a readable string
func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "Unknown"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatCount formats large numbers
func FormatCount(n int) string {
	if n >= 10000 {
		return fmt.Sprintf("%.1fw", float64(n)/10000)
	}
	return fmt.Sprintf("%d", n)
}

// ---- Console diagnostic output ----

// scanProgressLines is a custom SplitFunc for bufio.Scanner,
// splitting on both carriage return \r and line feed \n.
// yt-dlp default progress lines use \r for in-place refresh; with --newline they use \n.
// This handles both cases for line-by-line parsing.
func scanProgressLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// printProgressConsole displays download progress in the terminal in real time.
// Download phase uses \r for single-line in-place refresh (compact format, avoids terminal clutter),
// merge/complete phases output newlines.
func printProgressConsole(data ProgressData) {
	switch data.Status {
	case "downloading":
		// Compact single line: percentage speed (size) remaining fragments
		line := fmt.Sprintf("\r  %.1f%%", data.Percent)
		if data.SpeedMBps > 0 {
			line += fmt.Sprintf("  %.1f MB/s", data.SpeedMBps)
		}
		if data.TotalMB > 0 {
			line += fmt.Sprintf("  (%.0f/%.0f MB)", data.DownloadedMB, data.TotalMB)
		} else if data.DownloadedMB > 0 {
			line += fmt.Sprintf("  (%.0f MB)", data.DownloadedMB)
		}
		if data.ETASeconds > 0 {
			line += fmt.Sprintf("  ETA %s", FormatDuration(data.ETASeconds))
		}
		if data.FragmentCount > 1 {
			line += fmt.Sprintf("  frag %d/%d", data.FragmentIndex, data.FragmentCount)
		}
		fmt.Fprint(os.Stdout, line, "  ")
	case "merging":
		suffix := ""
		if data.MergeDone {
			suffix = "  done"
		}
		fmt.Fprintf(os.Stdout, "\r  Merging  elapsed %.0fs  remaining %s%s  ",
			data.MergeElapsed, FormatDuration(data.MergeRemaining), suffix)
	case "finished":
		fmt.Fprintf(os.Stdout, "\r  Done\n")
	case "error":
		fmt.Fprintf(os.Stdout, "\n  Failed: %s\n", data.ErrorMessage)
	case "cancelled":
		fmt.Fprintf(os.Stdout, "\n  Cancelled\n")
	case "retry":
		fmt.Fprintf(os.Stdout, "\n  Retry %d/%d...\n", data.RetryAttempt, data.RetryMax)
	}
}

// sendFinishedStats computes and sends completion stats:
// average speed, actual file size, download time, merge time.
// These are sent with the finished status, and the frontend persists them.
func sendFinishedStats(callback ProgressCallback, opts DownloadOptions, downloadStart, mergeStart time.Time, mergePhase bool) {
	now := time.Now()
	downloadDuration := now.Sub(downloadStart)

	// Get actual size of the final output file
	var finalSizeMB float64
	if files := findOutputFiles(opts.TempDir); len(files) > 0 {
		if stat, err := os.Stat(files[0]); err == nil {
			finalSizeMB = float64(stat.Size()) / (1024.0 * 1024.0)
		}
	}

	// Average download speed = file size / download duration
	avgSpeed := 0.0
	if downloadDuration.Seconds() > 0 && finalSizeMB > 0 {
		avgSpeed = finalSizeMB / downloadDuration.Seconds()
	}

	data := ProgressData{
		Status:          "finished",
		Percent:         100.0,
		AvgSpeedMBps:    avgSpeed,
		FinalSizeMB:     finalSizeMB,
		DownloadElapsed: int(downloadDuration.Seconds()),
	}

	if mergePhase {
		data.MergeElapsedFinal = now.Sub(mergeStart).Seconds()
	}

	callback(data)
}

// ---- Test helpers ----

// TestData fixed data for testing
var TestData = `{"title":"Test Video","webpage_url":"https://youtube.com/watch?v=test"}`
