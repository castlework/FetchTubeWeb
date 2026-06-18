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

// 合并阶段超时配置
const (
	MergeTimeout   = 300 * time.Second // 合并超时：5 分钟（大文件合并需要更长时间）
	MergeStallSecs = 30 * time.Second
	MaxRetries     = 2
)

// ProgressCallback 进度回调函数类型
type ProgressCallback func(ProgressData)

// ProgressData 进度数据
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
	// 完成时的统计信息（随 finished 状态下发，前端持久化保留显示）
	AvgSpeedMBps      float64 `json:"avg_speed_mbps,omitempty"`
	FinalSizeMB       float64 `json:"final_size_mb,omitempty"`
	DownloadElapsed   int     `json:"download_elapsed,omitempty"`
	MergeElapsedFinal float64 `json:"merge_elapsed_final,omitempty"`
}

// DownloadManager 管理单个下载任务的生命周期
type DownloadManager struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	cancelFn  context.CancelFunc
	cancelled atomic.Bool
	running   atomic.Bool
}

// NewDownloadManager 创建新的下载管理器
func NewDownloadManager() *DownloadManager {
	return &DownloadManager{}
}

// IsRunning 返回是否正在下载
func (m *DownloadManager) IsRunning() bool {
	return m.running.Load()
}

// Cancel 取消当前下载（线程安全，可多次调用）
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

// Download 执行下载（阻塞），通过回调报告进度
func (m *DownloadManager) Download(opts DownloadOptions, callback ProgressCallback) error {
	m.cancelled.Store(false)
	m.running.Store(true)
	defer m.running.Store(false)

	for attempt := 1; attempt <= MaxRetries+1; attempt++ {
		if m.cancelled.Load() {
			callback(ProgressData{Status: "cancelled"})
			cleanupTempFiles(opts.SaveDir)
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
			cleanupTempFiles(opts.SaveDir)
			// finished 回调（含统计信息）已由 runOneDownload 发送，此处不再重复发送
			return nil
		case "cancelled":
			callback(ProgressData{Status: "cancelled"})
			cleanupTempFiles(opts.SaveDir)
			return nil
		case "timeout":
			if attempt > MaxRetries {
				callback(ProgressData{
					Status:       "error",
					ErrorMessage: fmt.Sprintf("合并超时（已重试 %d 次）。\n请尝试选择更低的分辨率，或检查磁盘空间。", MaxRetries),
				})
				return fmt.Errorf("合并超时")
			}
			// 自动重试 — 已下载的流保留
			continue
		case "error":
			return nil // 错误已通过回调报告
		}
	}

	return nil
}

// runOneDownload 执行一次下载尝试，返回结果状态
func (m *DownloadManager) runOneDownload(opts DownloadOptions, callback ProgressCallback) string {
	// 下载前清理碎片
	preCleanup(opts.SaveDir)

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
		callback(ProgressData{Status: "error", ErrorMessage: "未找到 yt-dlp.exe"})
		return "error"
	}

	log.Printf("[download] save dir: %s", opts.SaveDir)
	if opts.Cookies != "" {
		log.Printf("[download] cookies: %s", opts.Cookies)
	}

	args := BuildDownloadArgs(opts)
	cmd := exec.CommandContext(ctx, ytdlp, args...)
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

	// 持续消费 stderr（yt-dlp 的 ERROR 等输出在 stderr），
	// 累积到 stderrBuf 供进程结束后做错误诊断。
	// 注：实测确认 yt-dlp 的 [download] 进度行实际输出在 stdout，由下方主循环解析。
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

	// 关键修复：yt-dlp 的进度信息输出到 stdout，但默认用 \r 原地刷新，
	// bufio.Scanner 默认按 \n 分割会导致一行进度都读不到。
	// 解决：1) BuildDownloadArgs 已加 --newline 让进度行用 \n；
	//       2) 自定义 SplitFunc 同时按 \r 与 \n 分割，双重保险。
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 大缓冲区
	scanner.Split(scanProgressLines)

	for scanner.Scan() {
		now := time.Now()

		// 检查取消
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

		// 下载中 → 计算已用时间
		if data.Status == "downloading" {
			data.ElapsedSeconds = int(now.Sub(downloadStart).Seconds())
		}

		if data.Status == "merging" && !mergePhase {
			mergePhase = true
			mergeStart = now
			// 进入合并阶段，命令行换行，结束下载进度条原地刷新
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

		// 命令行实时刷新显示解析后的进度（速度/大小/耗时/剩余/进度条）
		printProgressConsole(*data)

		callback(*data)

		if data.Status == "error" {
			fmt.Fprintln(os.Stdout)
			KillProcessTree(cmd)
			return "error"
		}

		// 合并阶段超时检测
		if mergePhase {
			elapsed := now.Sub(mergeStart)
			if elapsed > MergeTimeout {
				fmt.Fprintln(os.Stdout)
				KillProcessTree(cmd)
				return "timeout"
			}
			// 僵死检测
			if now.Sub(lastMsgTime) > MergeStallSecs {
				outputFiles := findOutputFiles(opts.SaveDir)
				if len(outputFiles) > 0 {
					stat, err := os.Stat(outputFiles[0])
					if err == nil {
						currentSize := stat.Size()
						if currentSize == lastFileSize && currentSize > 0 {
							// 文件大小稳定 → 合并已完成
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

	// 等待进程结束
	_ = cmd.Wait()

	// 检查 stderr 中是否有错误
	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		errMsg := stderrBuf.String()
		if errMsg != "" {
			callback(ProgressData{Status: "error", ErrorMessage: translateSimple(errMsg)})
			return "error"
		}
	}

	// 发送带统计信息的完成回调（平均速度/实际文件大小/下载耗时/合并耗时）
	sendFinishedStats(callback, opts, downloadStart, mergeStart, mergePhase)
	return "done"
}

// parseProgressLine 解析 yt-dlp 的进度输出行
// yt-dlp 在新版本中使用 --print progress 或解析默认输出
// 我们通过 stderr 中的 ANSI 进度行来提取信息
func parseProgressLine(line string) *ProgressData {
	// yt-dlp 进度行格式示例:
	// [download]   5.0% of ~100.00MiB at  2.50MiB/s ETA 00:38 (frag 3/8)
	// [download] 100% of 100.00MiB in 00:00:40
	// [ExtractAudio] ...
	// [Merger] Merging formats into "file.mkv"

	line = strings.TrimSpace(line)

	// 下载进度
	if strings.Contains(line, "[download]") && strings.Contains(line, "%") {
		data := &ProgressData{Status: "downloading"}

		// 百分比
		if re := regexp.MustCompile(`(\d+\.?\d*)%`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 2 {
				fmt.Sscanf(m[1], "%f", &data.Percent)
			}
		}

		// 下载大小
		if re := regexp.MustCompile(`of\s+~?(\d+\.?\d*)([KMG])iB`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 3 {
				size, _ := parseSize(m[1], m[2])
				data.TotalMB = size
			}
		}

		// 已下载量 = 百分比 × 总大小
		if data.Percent > 0 && data.TotalMB > 0 {
			data.DownloadedMB = data.Percent / 100.0 * data.TotalMB
		}

		// 速度
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

		// 分片信息
		if re := regexp.MustCompile(`frag\s+(\d+)/(\d+)`); re != nil {
			if m := re.FindStringSubmatch(line); len(m) >= 3 {
				fmt.Sscanf(m[1], "%d", &data.FragmentIndex)
				fmt.Sscanf(m[2], "%d", &data.FragmentCount)
			}
		}

		return data
	}

	// 下载完成（单个流）
	if strings.Contains(line, "[download]") && strings.Contains(line, "100%") {
		return &ProgressData{Status: "downloading", Percent: 100.0}
	}

	// 合并阶段
	if strings.Contains(line, "[Merger]") || strings.Contains(line, "[VideoConvertor]") {
		return &ProgressData{Status: "merging", Percent: 100.0}
	}

	// ffmpeg 合并
	if strings.Contains(line, "[ffmpeg]") {
		return &ProgressData{Status: "merging", Percent: 100.0}
	}

	return nil
}

// parseSize 解析带单位的大小为 MB
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

// translateSimple 简单错误翻译
func translateSimple(msg string) string {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "sign in") || strings.Contains(lower, "bot") {
		return "YouTube 要求验证身份。请使用 Cookies 登录。"
	}
	if strings.Contains(lower, "429") {
		return "请求过于频繁（429），请稍后再试。"
	}
	if strings.Contains(lower, "unavailable") || strings.Contains(lower, "private") {
		return "该视频不可用（已删除或私密）。"
	}
	return msg
}

// 文件清理函数

func preCleanup(saveDir string) {
	var cleaned int
	patterns := []string{"*.ytdl", "*.part", "*.part-*", "*.temp.*"}
	for _, pat := range patterns {
		files, _ := filepath.Glob(filepath.Join(saveDir, pat))
		for _, f := range files {
			_ = os.Remove(f)
			cleaned++
		}
	}
	if cleaned > 0 {
		log.Printf("[cleanup] pre-clean: removed %d temp files from %s", cleaned, saveDir)
	}
}

func cleanupTempFiles(saveDir string) {
	var renamed, removed int

	files, _ := filepath.Glob(filepath.Join(saveDir, "*.temp.*"))
	for _, f := range files {
		final := strings.Replace(f, ".temp.", ".", 1)
		_ = os.Rename(f, final)
		renamed++
	}

	patterns := []string{"*.ytdl", "*.part", "*.part-*"}
	for _, pat := range patterns {
		files, _ := filepath.Glob(filepath.Join(saveDir, pat))
		for _, f := range files {
			_ = os.Remove(f)
			removed++
		}
	}

	entries, _ := os.ReadDir(saveDir)
	for _, e := range entries {
		if matched, _ := regexp.MatchString(`\.f\d+`, e.Name()); matched {
			_ = os.Remove(filepath.Join(saveDir, e.Name()))
			removed++
		}
	}

	if renamed > 0 || removed > 0 {
		log.Printf("[cleanup] post-download: renamed %d, removed %d temp files from %s", renamed, removed, saveDir)
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

// FormatDuration 格式化秒数为可读字符串
func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "未知"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatCount 格式化大数字
func FormatCount(n int) string {
	if n >= 10000 {
		return fmt.Sprintf("%.1f万", float64(n)/10000)
	}
	return fmt.Sprintf("%d", n)
}

// ---- 命令行诊断输出 ----

// scanProgressLines 是 bufio.Scanner 的自定义 SplitFunc，
// 同时按回车符 \r 和换行符 \n 分割。
// yt-dlp 默认进度行用 \r 原地刷新；加了 --newline 后用 \n。
// 这样两种情况都能被逐行解析。
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

// printProgressConsole 在命令行实时刷新显示解析后的进度信息。
// 用 \r 原地刷新，让用户直观看到「速度/大小/耗时/剩余/进度条」是否被正确解析。
func printProgressConsole(data ProgressData) {
	switch data.Status {
	case "downloading":
		// 文本进度条（20 格）
		const barLen = 20
		filled := int(data.Percent / 100.0 * float64(barLen))
		if filled < 0 {
			filled = 0
		}
		if filled > barLen {
			filled = barLen
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)

		speed := "--"
		if data.SpeedMBps > 0 {
			speed = fmt.Sprintf("%.2f MB/s", data.SpeedMBps)
		}
		size := "--"
		if data.TotalMB > 0 {
			size = fmt.Sprintf("%.1f / %.1f MB", data.DownloadedMB, data.TotalMB)
		} else if data.DownloadedMB > 0 {
			size = fmt.Sprintf("%.1f MB", data.DownloadedMB)
		}
		elapsed := FormatDuration(data.ElapsedSeconds)
		eta := FormatDuration(data.ETASeconds)
		frag := ""
		if data.FragmentCount > 0 {
			frag = fmt.Sprintf(" | 分片 %d/%d", data.FragmentIndex, data.FragmentCount)
		}
		fmt.Fprintf(os.Stdout, "\r[下载] |%s| %5.1f%% | 速度 %-12s | 大小 %-22s | 耗时 %-8s | 剩余 %-8s%s   ", bar, data.Percent, speed, size, elapsed, eta, frag)
	case "merging":
		rem := FormatDuration(data.MergeRemaining)
		fmt.Fprintf(os.Stdout, "\r[合并] 已用 %.0fs | 剩余 %-8s%s   ",
			data.MergeElapsed, rem, func() string {
				if data.MergeDone {
					return " | ✅ 完成"
				}
				return ""
			}())
	case "finished":
		fmt.Fprintf(os.Stdout, "\r[完成] ✅ 100%%                                                                       \n")
	case "error":
		fmt.Fprintf(os.Stdout, "\n[失败] %s\n", data.ErrorMessage)
	case "cancelled":
		fmt.Fprintf(os.Stdout, "\n[已取消]\n")
	case "retry":
		fmt.Fprintf(os.Stdout, "\n[重试] 第 %d/%d 次...\n", data.RetryAttempt, data.RetryMax)
	}
}

// sendFinishedStats 计算并下发任务完成时的统计信息：
// 平均速度、实际文件大小、下载耗时、合并耗时。
// 这些信息随 finished 状态发送，前端会持久化保留显示。
func sendFinishedStats(callback ProgressCallback, opts DownloadOptions, downloadStart, mergeStart time.Time, mergePhase bool) {
	now := time.Now()
	downloadDuration := now.Sub(downloadStart)

	// 获取最终输出文件的实际大小
	var finalSizeMB float64
	if files := findOutputFiles(opts.SaveDir); len(files) > 0 {
		if stat, err := os.Stat(files[0]); err == nil {
			finalSizeMB = float64(stat.Size()) / (1024.0 * 1024.0)
		}
	}

	// 平均下载速度 = 文件大小 / 下载耗时
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

// ---- 测试辅助 ----

// TestData 用于测试的固定数据
var TestData = `{"title":"Test Video","webpage_url":"https://youtube.com/watch?v=test"}`
