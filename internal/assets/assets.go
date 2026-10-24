// Package assets 内嵌二进制资源（应用图标）。
package assets

import _ "embed"

// Icon 应用图标（ICO 字节，供托盘与 exe 资源共用）
//
//go:embed app.ico
var Icon []byte
