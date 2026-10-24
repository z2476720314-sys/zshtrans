//go:build !windows

package tray

import "errors"

// Run 非 Windows 平台不支持托盘
func Run(icon []byte, act Actions) error { return errors.New("tray only supported on windows") }

// Exit 退出托盘
func Exit() {}
