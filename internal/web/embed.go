// Package web 内嵌前端构建产物（vite build 输出到 dist/）
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
