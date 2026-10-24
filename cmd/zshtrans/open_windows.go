//go:build windows

package main

import (
	"os/exec"
	"syscall"

	"github.com/local/zshtrans/internal/logx"
)

// openBrowser 用默认浏览器打开本机地址
func openBrowser(url string) {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		logx.Warn("open browser fail: %v", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}
