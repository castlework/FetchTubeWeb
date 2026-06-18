// Package ytdlp — yt-dlp 进程调用封装
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

// 浏览器列表（支持从浏览器读取 cookies）
var Browsers = []string{"chrome", "firefox", "edge", "brave", "opera"}

// FindYtDlp 查找 yt-dlp.exe 的位置
// 优先级：同目录 > PATH
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

// FindFFmpeg 查找 ffmpeg.exe 的位置
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

// FindNode 查找 node.exe 的位置
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

// RawFormat yt-dlp --dump-json 返回的原始格式条目
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

// RawInfo yt-dlp --dump-json 返回的完整视频信息
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

// FetchInfo 调用 yt-dlp 提取视频信息
func FetchInfo(url, proxy, cookies string) (*RawInfo, error) {
	ytdlp := FindYtDlp()
	if ytdlp == "" {
		return nil, fmt.Errorf("未找到 yt-dlp.exe，请将其放在程序同目录下")
	}
	return fetchInfoOnce(ytdlp, url, proxy, cookies)
}

// fetchInfoOnce 执行 yt-dlp 信息提取
// 不指定 -f，让 yt-dlp 使用默认选择器（bv*+ba/b），和 Python 原版 extract_info 行为一致
func fetchInfoOnce(ytdlp, url, proxy, cookies string) (*RawInfo, error) {
	args := []string{"--dump-json", "--no-warnings", "--no-playlist"}

	if proxy != "" {
		args = append(args, "--proxy", proxy)
	}

	// Node.js 运行时 + EJS 远程组件（解决 YouTube n challenge，和 Python 版本一致）
	if node := FindNode(); node != "" {
		args = append(args, "--js-runtimes", "node:"+node)
	}
	// 和 Python opts["remote_components"] = ["ejs:github"] 完全一致
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
		// 合并 stdout 和 stderr
		combined := strings.TrimSpace(string(output) + "\n" + stderr.String())
		if combined == "" {
			combined = err.Error()
		}
		return nil, translateError(combined, nil)
	}

	// 检查 stderr 中是否有代理/Cookie 警告
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
		return nil, fmt.Errorf("解析视频信息失败: %w", err)
	}

	return &info, nil
}

// BuildDownloadArgs 构建下载命令的参数列表（不包含 yt-dlp 路径）
// 格式回退链和 Python 版本完全一致
func BuildDownloadArgs(opts DownloadOptions) []string {
	// 格式回退链（和 Python downloader._download_worker 完全一致）
	formatStr := opts.FormatID
	if ffmpeg := FindFFmpeg(); ffmpeg != "" {
		// 有 ffmpeg：优先指定格式，回退到 bestvideo+bestaudio
		formatStr = fmt.Sprintf("%s/bestvideo+bestaudio/best", opts.FormatID)
	} else if strings.Contains(opts.FormatID, "+") {
		// 无 ffmpeg 且选了 DASH 分离流 → 回退到 best mp4
		formatStr = "best[ext=mp4]/best"
	} else {
		formatStr = fmt.Sprintf("%s/best[ext=mp4]/best", opts.FormatID)
	}

	args := []string{
		"--no-warnings",
		"--no-playlist",
		// --newline：强制 yt-dlp 每条进度输出单独成行（用 \n 而非 \r 原地刷新）。
		// 这样后端 bufio.Scanner 才能按行解析进度，否则进度行用 \r 刷新无法被 ScanLines 读取。
		// 仅影响输出格式，不影响下载行为。
		"--newline",
		"--format", formatStr,
		"--output", filepath.Join(opts.SaveDir, "%(title)s.%(ext)s"),
		"--concurrent-fragments", fmt.Sprintf("%d", opts.ConcurrentFragments),
		"--retries", "10",
		"--fragment-retries", "10",
		"--file-access-retries", "3",
		"--extractor-retries", "3",
	}

	// 多音轨：yt-dlp 默认 + 只合并第一个音频流，需 --audio-multistreams 才保留全部
	if strings.Count(opts.FormatID, "+") > 1 {
		args = append(args, "--audio-multistreams")
	}

	if opts.Resume {
		args = append(args, "--continue")
	}
	// 使用 --part 和 --keep-video，和 Python 原版行为一致
	// --part 是 yt-dlp 默认行为（使用 .part 文件），--keep-video 保留中间视频流以便重试
	args = append(args, "--keep-video")

	// 输出格式
	if opts.OutputExt != "" {
		args = append(args, "--merge-output-format", opts.OutputExt)
	}

	// ffmpeg 位置
	if ffmpeg := FindFFmpeg(); ffmpeg != "" {
		args = append(args, "--ffmpeg-location", ffmpeg)
	}

	// Node.js 运行时 + EJS 远程组件（解决 YouTube n challenge，和 Python 一致）
	if node := FindNode(); node != "" {
		args = append(args, "--js-runtimes", "node:"+node)
	}
	args = append(args, "--remote-components", "ejs:github")

	// 代理
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

	// ffmpeg 后处理参数（优化兼容性）
	if opts.OutputExt == "mp4" {
		args = append(args, "--postprocessor-args", "ffmpeg:-movflags +faststart")
	} else {
		args = append(args, "--postprocessor-args", "ffmpeg:-c copy")
	}

	args = append(args, opts.URL)
	return args
}

// DownloadOptions 下载选项
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

// KillProcessTree Windows: 强制终止进程树
func KillProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	if runtime.GOOS == "windows" {
		// 使用 taskkill /F /T 终止整个进程树（含 ffmpeg 子进程）
		killCmd := exec.Command("taskkill", "/F", "/T", "/PID",
			fmt.Sprintf("%d", cmd.Process.Pid))
		killCmd.Stdout = nil
		killCmd.Stderr = nil
		_ = killCmd.Run()
	} else {
		_ = cmd.Process.Kill()
	}
}

// ---- 辅助函数 ----

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

// translateError 翻译常见错误为用户友好的中文提示
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

	// 默认：返回原始错误（去除冗长的技术细节）
	if output != "" {
		// 截取第一行或前300字符
		firstLine := strings.SplitN(output, "\n", 2)[0]
		if len(firstLine) > 300 {
			firstLine = firstLine[:300] + "..."
		}
		return fmt.Errorf("下载失败：%s", firstLine)
	}
	return fmt.Errorf("下载失败：%s", errStr)
}
