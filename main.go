package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"youtube-downloader/internal/handler"
	"youtube-downloader/web"
)

var version = "dev"

func main() {
	port := flag.Int("port", 8899, "HTTP 监听端口")
	flag.Parse()

	if *port < 1 || *port > 65535 {
		log.Fatalf("无效端口号: %d（范围: 1-65535）", *port)
	}

	srv := handler.NewServer()

	mux := http.NewServeMux()

	// 静态文件（内嵌前端资源）
	mux.Handle("GET /static/", http.FileServer(http.FS(web.Assets)))
	// 主页
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data, _ := web.Assets.ReadFile("static/index.html")
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	})

	// API 路由
	srv.SetupRoutes(mux)

	// CORS + 日志中间件
	wrapped := withMiddleware(mux)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	server := &http.Server{
		Addr:         addr,
		Handler:      wrapped,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // WebSocket 需要无超时
		IdleTimeout:  120 * time.Second,
	}

	printBanner(*port)

	// 自动打开浏览器
	go openBrowser(fmt.Sprintf("http://localhost:%d", *port))

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

func printBanner(port int) {
	fmt.Println(strings.Repeat("=", 56))
	fmt.Printf("  🎬 YouTube 视频下载器 (Go WebUI)  v%s\n", version)
	fmt.Println(strings.Repeat("=", 56))
	fmt.Printf("  WebUI 地址:  http://localhost:%d\n", port)
	fmt.Printf("  按 Ctrl+C 停止服务\n")
	fmt.Println(strings.Repeat("=", 56))
}

func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}

		// 日志
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[http] %s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	_ = exec.Command(cmd, args...).Start()
}
