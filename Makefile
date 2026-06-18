# YouTube Downloader Go WebUI — Build System

APP_NAME = youtube-downloader
VERSION = 2.0.0
OUTPUT_DIR = dist

# 默认目标
.PHONY: all
all: build

# 开发构建（快速）
.PHONY: build
build:
	go build -ldflags="-s -w" -o $(APP_NAME).exe .

# 生产构建（压缩 + 版本信息）
.PHONY: release
release:
	@if not exist $(OUTPUT_DIR) mkdir $(OUTPUT_DIR)
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(OUTPUT_DIR)/$(APP_NAME).exe .
	@echo "Release built: $(OUTPUT_DIR)/$(APP_NAME).exe"

# 运行开发服务器
.PHONY: run
run:
	go run . 8899

# 清理构建产物
.PHONY: clean
clean:
	del /q $(APP_NAME).exe 2>nul
	rmdir /s /q $(OUTPUT_DIR) 2>nul

# 下载依赖
.PHONY: deps
deps:
	go mod tidy
	go mod download

# 格式化代码
.PHONY: fmt
fmt:
	go fmt ./...

# 静态检查
.PHONY: vet
vet:
	go vet ./...

# 构建分发包（EXE + 外部二进制说明）
.PHONY: dist
dist: release
	@echo ============================================
	@echo  分发说明：
	@echo  将以下文件放在同一目录：
	@echo    - $(APP_NAME).exe
	@echo    - yt-dlp.exe      (下载: https://github.com/yt-dlp/yt-dlp/releases)
	@echo    - ffmpeg.exe      (下载: https://www.gyan.dev/ffmpeg/builds/)
	@echo    - node.exe        (可选，下载: https://nodejs.org)
	@echo ============================================

# 帮助
.PHONY: help
help:
	@echo 可用目标:
	@echo   build     - 开发构建
	@echo   release   - 生产构建（压缩）
	@echo   run       - 运行开发服务器
	@echo   clean     - 清理
	@echo   deps      - 下载依赖
	@echo   fmt       - 格式化代码
	@echo   vet       - 静态检查
	@echo   dist      - 构建分发包 + 显示说明
