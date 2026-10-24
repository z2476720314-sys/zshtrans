//go:build !windows

package main

import (
	"os/exec"

	"github.com/local/zshtrans/internal/logx"
)

func openBrowser(url string) {
	if err := exec.Command("xdg-open", url).Start(); err != nil {
		logx.Warn("open browser fail: %v", err)
	}
}
