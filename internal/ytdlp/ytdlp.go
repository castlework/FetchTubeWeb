// Package ytdlp — yt-dlp process invocation wrapper
package ytdlp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Browser list (supports reading cookies from browsers)
var Browsers = []string{"chrome", "firefox", "edge", "brave", "opera"}

// FindYtDlp looks for yt-dlp.exe location
// Priority: same directory > PATH
func FindYtDlp() string {
	exeDir := exeDirectory()
	if exeDir != "" {
		candidate := filepath.Join(exeDir, "yt-dlp.exe")
		if fileExists(candidate) {
			return candidate
		}
	}
	p, _ := exec.LookPath("yt-dlp.exe")
	if p != "" {
		return p
	}
	p, _ = exec.LookPath("yt-dlp")
	return p
}

// FindFFmpeg looks for ffmpeg.exe location
func FindFFmpeg() string {
	exeDir := exeDirectory()
	if exeDir != "" {
		candidate := filepath.Join(exeDir, "ffmpeg.exe")
		if fileExists(candidate) {
			return candidate
		}
	}
	p, _ := exec.LookPath("ffmpeg.exe")
	if p != "" {
		return p
	}
	p, _ = exec.LookPath("ffmpeg")
	return p
}

// FindNode looks for node.exe location
func FindNode() string {
	exeDir := exeDirectory()
	if exeDir != "" {
		candidate := filepath.Join(exeDir, "node.exe")
		if fileExists(candidate) {
			return candidate
		}
	}
	p, _ := exec.LookPath("node.exe")
	if p != "" {
		return p
	}
	p, _ = exec.LookPath("node")
	return p
}

// RawFormat raw format entry from yt-dlp --dump-json
type RawFormat struct {
	FormatID     string      `json:"format_id"`
	Vcodec       string      `json:"vcodec"`
	Acodec       string      `json:"acodec"`
	Width        int         `json:"width"`
	Height       int         `json:"height"`
	FPS          interface{} `json:"fps"`
	ABR          interface{} `json:"abr"`
	TBR          interface{} `json:"tbr"`
	Filesize     interface{} `json:"filesize"`
	FilesizeApprox interface{} `json:"filesize_approx"`
	Ext          string      `json:"ext"`
	FormatNote   string      `json:"format_note"`
	Language     string      `json:"language"`
}

// RawInfo complete video info from yt-dlp --dump-json
type RawInfo struct {
	Title       string      `json:"title"`
	URL         string      `json:"webpage_url"`
	Thumbnail   string      `json:"thumbnail"`
	Duration    interface{} `json:"duration"`
	Uploader    string      `json:"uploader"`
	ViewCount   interface{} `json:"view_count"`
	LikeCount   interface{} `json:"like_count"`
	Description string      `json:"description"`
	Formats     []RawFormat `json:"formats"`
}

// FetchInfo calls yt-dlp to extract video info
func FetchInfo(url, proxy, cookies string) (*RawInfo, error) {
	ytdlp := FindYtDlp()
	if ytdlp == "" {
		return nil, fmt.Errorf("yt-dlp.exe not found — place it in the same directory as the program")
	}
	return fetchInfoOnce(ytdlp, url, proxy, cookies)
}

// fetchInfoOnce executes yt-dlp info extraction
// Does not specify -f, lets yt-dlp use its default selector (bv*+ba/b), matching Python original extract_info behavior
func fetchInfoOnce(ytdlp, url, proxy, cookies string) (*RawInfo, error) {
	args := []string{"--dump-json", "--no-warnings", "--no-playlist"}

	if proxy != "" {
		args = append(args, "--proxy", proxy)
	}

	// Node.js runtime + EJS remote component (solves YouTube n challenge, matching Python version)
	if node := FindNode(); node != "" {
		args = append(args, "--js-runtimes", "node:"+node)
	}
	// Matches Python opts["remote_components"] = ["ejs:github"] exactly
	args = append(args, "--remote-components", "ejs:github")

	if cookies != "" {
		if isBrowser(cookies) {
			args = append(args, "--cookies-from-browser", strings.ToLower(cookies))
		} else {
			args = append(args, "--cookies", cookies)
		}
	}

	args = append(args, url)

	var stderr bytes.Buffer
	cmd := exec.Command(ytdlp, args...)
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		// Merge stdout and stderr
		combined := strings.TrimSpace(string(output) + "\n" + stderr.String())
		if combined == "" {
			combined = err.Error()
		}
		return nil, translateError(combined, nil)
	}

	// Check stderr for proxy/cookie warnings
	stderrStr := stderr.String()
	if stderrStr != "" {
		lower := strings.ToLower(stderrStr)
		if strings.Contains(lower, "unable to connect to proxy") ||
			strings.Contains(lower, "proxyerror") ||
			strings.Contains(lower, "connection refused") {
			return nil, translateError(stderrStr, nil)
		}
	}

	var info RawInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse video info: %w", err)
	}

	return &info, nil
}

// BuildDownloadArgs builds the argument list for the download command (excludes yt-dlp path)
// Format fallback chain exactly matches the Python version
func BuildDownloadArgs(opts DownloadOptions) []string {
	// Format fallback chain (matches Python downloader._download_worker exactly)
	formatStr := opts.FormatID
	if ffmpeg := FindFFmpeg(); ffmpeg != "" {
		// With ffmpeg: prefer specified format, fallback to bestvideo+bestaudio
		formatStr = fmt.Sprintf("%s/bestvideo+bestaudio/best", opts.FormatID)
	} else if strings.Contains(opts.FormatID, "+") {
		// No ffmpeg and DASH split stream selected → fallback to best mp4
		formatStr = "best[ext=mp4]/best"
	} else {
		formatStr = fmt.Sprintf("%s/best[ext=mp4]/best", opts.FormatID)
	}

	args := []string{
		"--no-warnings",
		"--no-playlist",
		// --newline: force yt-dlp to output each progress line on its own line (using \n instead of \r).
		// This allows the backend bufio.Scanner to parse progress line by line, otherwise \r-refreshed
		// progress lines cannot be read by ScanLines. Only affects output format, not download behavior.
		"--newline",
		"--format", formatStr,
		"--output", filepath.Join(opts.SaveDir, "%(title)s.%(ext)s"),
		"--concurrent-fragments", fmt.Sprintf("%d", opts.ConcurrentFragments),
		"--retries", "10",
		"--fragment-retries", "10",
		"--file-access-retries", "3",
		"--extractor-retries", "3",
	}

	// Multi-audio: yt-dlp defaults to merging only the first audio stream; --audio-multistreams keeps all
	if strings.Count(opts.FormatID, "+") > 1 {
		args = append(args, "--audio-multistreams")
	}

	if opts.Resume {
		args = append(args, "--continue")
	}
	// Use --part and --keep-video, matching Python original behavior
	// --part is yt-dlp default (uses .part files), --keep-video retains intermediate video streams for retries
	args = append(args, "--keep-video")

	// Output format
	if opts.OutputExt != "" {
		args = append(args, "--merge-output-format", opts.OutputExt)
	}

	// ffmpeg location
	if ffmpeg := FindFFmpeg(); ffmpeg != "" {
		args = append(args, "--ffmpeg-location", ffmpeg)
	}

	// Node.js runtime + EJS remote component (solves YouTube n challenge, matching Python)
	if node := FindNode(); node != "" {
		args = append(args, "--js-runtimes", "node:"+node)
	}
	args = append(args, "--remote-components", "ejs:github")

	// Proxy
	if opts.Proxy != "" {
		args = append(args, "--proxy", opts.Proxy)
	}

	// Cookies
	if opts.Cookies != "" {
		if isBrowser(opts.Cookies) {
			args = append(args, "--cookies-from-browser", strings.ToLower(opts.Cookies))
		} else {
			args = append(args, "--cookies", opts.Cookies)
		}
	}

	// ffmpeg post-processing args (optimize compatibility)
	if opts.OutputExt == "mp4" {
		args = append(args, "--postprocessor-args", "ffmpeg:-movflags +faststart")
	} else {
		args = append(args, "--postprocessor-args", "ffmpeg:-c copy")
	}

	args = append(args, opts.URL)
	return args
}

// DownloadOptions download options
type DownloadOptions struct {
	URL                string
	FormatID           string
	OutputExt          string
	SaveDir            string
	ConcurrentFragments int
	Resume             bool
	Proxy              string
	Cookies            string
	KeepTempFiles      bool
}

// KillProcessTree Windows: force terminate process tree
func KillProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	if runtime.GOOS == "windows" {
		// Use taskkill /F /T to terminate the entire process tree (including ffmpeg child processes)
		killCmd := exec.Command("taskkill", "/F", "/T", "/PID",
			fmt.Sprintf("%d", cmd.Process.Pid))
		killCmd.Stdout = nil
		killCmd.Stderr = nil
		_ = killCmd.Run()
	} else {
		_ = cmd.Process.Kill()
	}
}

// ---- Helper functions ----

func exeDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isBrowser(name string) bool {
	n := strings.ToLower(name)
	for _, b := range Browsers {
		if n == b {
			return true
		}
	}
	return false
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case nil:
		return 0
	}
	return 0
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case nil:
		return 0
	}
	return 0
}

// translateError translates common errors to user-friendly Chinese prompts
func translateError(output string, err error) error {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	msg := strings.ToLower(output + errStr)

	if strings.Contains(msg, "sign in to confirm") || strings.Contains(msg, "bot") {
		return fmt.Errorf(
			"YouTube 要求验证身份（反爬虫检测）。\n\n" +
				"解决方法：\n" +
				"  1. 推荐：使用 Firefox 浏览器登录 YouTube，然后在 Cookies 中选择「Firefox」\n" +
				"     （Firefox 支持运行时读取，无需关闭浏览器）\n" +
				"  2. 或：安装浏览器扩展 「Get cookies.txt LOCALLY」导出 cookies.txt 文件\n" +
				"  3. 也可尝试切换代理或更换 IP")
	}
	if strings.Contains(msg, "could not copy") && strings.Contains(msg, "cookie") {
		return fmt.Errorf(
			"无法读取浏览器 Cookie 数据库（Chrome 在运行时会锁定此文件）。\n\n" +
				"解决方法：\n" +
				"  A. 推荐：改用 Firefox 浏览器登录 YouTube，然后选择「Firefox」\n" +
				"  B. 关闭 Chrome 浏览器后重试\n" +
				"  C. 安装扩展导出 cookies.txt 文件")
	}
	if strings.Contains(msg, "http error 429") {
		return fmt.Errorf("YouTube 暂时限制了请求频率（HTTP 429）。\n请等待几分钟后再试，或切换代理 IP。")
	}
	if strings.Contains(msg, "video unavailable") || strings.Contains(msg, "private") {
		return fmt.Errorf("该视频不可用（可能已被删除或设为私密）。")
	}
	if strings.Contains(msg, "requested format is not available") {
		return fmt.Errorf("所请求的视频格式不可用。\n建议：重新搜索刷新格式列表，或选择其他分辨率。")
	}
	if strings.Contains(msg, "unable to connect to proxy") || strings.Contains(msg, "proxyerror") {
		return fmt.Errorf("无法连接到代理服务器。\n请检查代理地址和端口是否正确，或尝试关闭代理。")
	}
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "connectionerror") {
		return fmt.Errorf("网络连接失败。\n请检查：\n  1. 代理设置是否正确\n  2. 网络是否正常\n  3. 防火墙是否拦截")
	}

	// Default: return original error (strip verbose technical details)
	if output != "" {
		// Take first line or first 300 chars
		firstLine := strings.SplitN(output, "\n", 2)[0]
		if len(firstLine) > 300 {
			firstLine = firstLine[:300] + "..."
		}
		return fmt.Errorf("下载失败：%s", firstLine)
	}
	return fmt.Errorf("下载失败：%s", errStr)
}
