// Package web — embedded frontend static assets
package web

import "embed"

// Assets contains all frontend static files
//
//go:embed static/*
var Assets embed.FS
