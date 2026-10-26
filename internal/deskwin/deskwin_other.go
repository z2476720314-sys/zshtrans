//go:build !windows

// Package deskwin 非 Windows 平台的占位实现：始终返回 ErrUnavailable，
// 不支持的平台不创建浏览器替代窗口。真实实现见 deskwin_windows.go。
package deskwin

import "errors"

// Settings 窗口初始参数
type Settings struct {
	DataPath string
	Title    string
	URL      string
	Width    int
	Height   int
	Debug    bool
}

// ErrUnavailable 本平台不支持 WebView2 桌面窗口
var ErrUnavailable = errors.New("desktop window unsupported on this platform")

// Win 占位窗口
type Win struct{}

// Open 占位实现
func Open(Settings) (*Win, error) { return nil, ErrUnavailable }

// Run 占位实现
func (*Win) Run() {}

// ActivateExisting 占位实现
func ActivateExisting(string) bool { return false }

// Ready 占位实现
func (*Win) Ready() bool { return false }

// Shown 占位实现
func (*Win) Shown() bool { return false }

// Hidden 占位实现
func (*Win) Hidden() bool { return false }

// Navigate 占位实现
func (*Win) Navigate(string) {}

// Eval 占位实现
func (*Win) Eval(string) {}

func (*Win) Bind(string, any) error { return ErrUnavailable }
func (*Win) Init(string)            {}

// Show 占位实现
func (*Win) Show() {}

// Hide 占位实现
func (*Win) Hide() {}

// Close 占位实现
func (*Win) Close() {}

// Wait 占位实现
func (*Win) Wait() error { return ErrUnavailable }

// HWND 占位实现
func (*Win) HWND() uintptr { return 0 }

// Toggle 占位实现
func (*Win) Toggle() {}

// Available 占位实现
func (*Win) Available() bool        { return false }
func (*Win) Visible() bool          { return false }
func (*Win) PrepareSelection() bool { return false }
func ShowError(string)              {}
