//go:build !windows

// Package console 在非 Windows 平台是空实现：进程本来就有终端。
package console

// Attach 在非 Windows 上无事可做，恒返回 false。
func Attach() bool { return false }

// Detach 在非 Windows 上无事可做。
func Detach() {}
