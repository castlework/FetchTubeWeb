// Package web — 内嵌前端静态资源
package web

import "embed"

// Assets 包含所有前端静态文件
//
//go:embed static/*
var Assets embed.FS
