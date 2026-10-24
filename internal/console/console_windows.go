//go:build windows

// Package console 给 GUI 子系统的 exe 按需挂一个控制台。
//
// 发布版用 -H windowsgui 链接：双击不弹黑框，代价是进程一开始没有控制台，
// stdout/stderr 全丢。而 -console 是排障手段（README 第 7 节），必须还能用，
// 所以在这里用 AllocConsole 现挂一个，并把标准句柄重绑到 os.Stdout/os.Stderr。
package console

import (
	"os"

	"golang.org/x/sys/windows"
)

var (
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procAllocConsole   = kernel32.NewProc("AllocConsole")
	procFreeConsole    = kernel32.NewProc("FreeConsole")
	procGetConsoleWnd  = kernel32.NewProc("GetConsoleWindow")
	procSetStdHandle   = kernel32.NewProc("SetStdHandle")
	stdOutputHandleVal = uint32(0xFFFFFFF5) // STD_OUTPUT_HANDLE = -11
)

// Attach 确保当前进程有一个可见控制台，并把 os.Stdout / os.Stderr 指过去。
// 已经挂在某个控制台上（从 cmd 里启动的情况）就直接复用。
// 返回 true 表示这次新挂了一个控制台。
func Attach() bool {
	if h, _, _ := procGetConsoleWnd.Call(); h != 0 {
		// 已有控制台（例如从 cmd/powershell 里跑），标准句柄本来就是通的。
		return false
	}
	if r, _, _ := procAllocConsole.Call(); r == 0 {
		return false
	}

	// AllocConsole 之后要重新打开 CONOUT$，否则 os.Stdout 还指着无效句柄。
	out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	os.Stdout = out
	os.Stderr = out
	if in, errIn := os.OpenFile("CONIN$", os.O_RDONLY, 0); errIn == nil {
		os.Stdin = in
	}

	// 同步 Win32 层的 STD_OUTPUT_HANDLE，让子进程/原生库也能往这个控制台写。
	if f, err := out.Stat(); err == nil {
		_ = f
	}
	h, _, _ := procGetConsoleWnd.Call()
	_ = h
	_, _, _ = procSetStdHandle.Call(uintptr(stdOutputHandleVal), out.Fd())
	return true
}

// Detach 释放控制台（退出前清理用）。
func Detach() {
	_, _, _ = procFreeConsole.Call()
}
