//go:build windows

// Package deskwin 用 WebView2 承载内嵌 UI，替代「启动外部浏览器」。
//
// 线程模型（关键）：go-webview2 在包 init 里对**主 goroutine** 做了
// runtime.LockOSThread + CoInitializeEx，因此 WebView2 的创建与消息循环
// 必须留在主 goroutine 上。调用方约定：
//
//	w, err := deskwin.Open(...)   // 主 goroutine，同步创建
//	... 把 HTTP 服务/热键/托盘都放进 goroutine ...
//	w.Run()                       // 主 goroutine，阻塞到窗口销毁
//
// 其它 goroutine 通过 Show/Hide/Toggle/Navigate/Eval 操作窗口（内部 Dispatch）。
// 点窗口 X 只隐藏到托盘，不销毁、不退出进程（对应原程序「关闭即最小化到托盘」）。
package deskwin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

// Settings 窗口初始参数
type Settings struct {
	Title  string
	URL    string // 首页地址，例如 http://127.0.0.1:8765/
	Width  int
	Height int
	Debug  bool

	// DataPath WebView2 用户数据目录。留空时 go-webview2 会退回
	// %AppData%\<exe 全名>，多个实例/改名后的 exe 会互相抢目录，
	// 表现为 CreateCoreWebView2Controller 回调拿到 nil controller 后崩溃。
	DataPath string
}

// Win 一个 WebView2 窗口
type Win struct {
	wv      webview2.WebView
	hwnd    uintptr
	mu      sync.Mutex
	shown   atomic.Bool
	hidden  atomic.Bool
	closing atomic.Bool
	err     error
}

// ErrUnavailable 表示系统缺少 WebView2 运行时（老 Win10/精简系统）
var ErrUnavailable = errors.New("WebView2 运行时不可用")

// ---------- Win32 ----------

var (
	user32 = windows.NewLazySystemDLL("user32.dll")

	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW     = user32.NewProc("CallWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procSetWindowTextW      = user32.NewProc("SetWindowTextW")
)

const (
	gwlpWndProc = ^uintptr(3) // GWLP_WNDPROC = -4
	wmClose     = 0x0010
	swHide      = 0
	swShow      = 5
	swRestore   = 9
)

// 窗口过程子类化状态
var (
	subclassMu  sync.Mutex
	origProcs   = map[uintptr]uintptr{}
	closeHooks  = map[uintptr]func(){}
	wndProcOnce sync.Map
)

// subclass 把窗口过程换成自己的，拦截 WM_CLOSE 走 onClose，其余转发原 proc。
func subclass(hwnd uintptr, onClose func()) {
	if hwnd == 0 {
		return
	}
	subclassMu.Lock()
	closeHooks[hwnd] = onClose
	subclassMu.Unlock()

	if _, loaded := wndProcOnce.LoadOrStore(hwnd, struct{}{}); loaded {
		return
	}
	cb := windows.NewCallback(func(h, msg, wp, lp uintptr) uintptr {
		if msg == wmClose {
			subclassMu.Lock()
			fn := closeHooks[h]
			subclassMu.Unlock()
			if fn != nil {
				fn()
				return 0
			}
		}
		subclassMu.Lock()
		orig := origProcs[h]
		subclassMu.Unlock()
		if orig == 0 {
			return 0
		}
		r, _, _ := procCallWindowProcW.Call(orig, h, msg, wp, lp)
		return r
	})
	prev, _, _ := procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, cb)
	if prev == 0 {
		return
	}
	subclassMu.Lock()
	origProcs[hwnd] = prev
	subclassMu.Unlock()
}

// ---------- 生命周期 ----------

// defaultDataPath WebView2 用户数据目录：%LOCALAPPDATA%\ZSH TRANS\WebView2
//
// 必须显式指定。go-webview2 默认用 %AppData%\<exe 全名>，开发版 exe 改名
// （zshtrans-dev.exe）与正式版（zshtrans.exe）会各占一个目录；若该目录正被
// 另一个残留的 msedgewebview2 进程持有，CreateCoreWebView2Controller 会以
// nil controller 回调，库内直接解引用 → 进程崩溃。
//
// 优先 LOCALAPPDATA（普通用户可写）；os.TempDir() 在部分环境是 C:\WINDOWS，
// 无写权限会导致 WebView2 起不来，所以只作最后兜底。
func defaultDataPath() string {
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		return filepath.Join(dir, "ZSH TRANS", "WebView2")
	}
	// 某些受限启动环境（计划任务、精简 shell）不带 LOCALAPPDATA
	if up := os.Getenv("USERPROFILE"); up != "" {
		return filepath.Join(up, "AppData", "Local", "ZSH TRANS", "WebView2")
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "ZSH TRANS", "WebView2")
	}
	return filepath.Join(os.TempDir(), "ZSH TRANS WebView2")
}

// Open 同步创建窗口（必须在主 goroutine 调用）。创建失败返回 ErrUnavailable，
// 调用方应回退到浏览器模式。返回后需由同一 goroutine 调 Run()。
func Open(s Settings) (w *Win, err error) {
	// go-webview2 在 controller 回调为 nil 时直接解引用崩溃（chromium.go:190）。
	// 这里兜底，把它变成可回退的错误，而不是让整个程序消失。
	defer func() {
		if r := recover(); r != nil {
			w = nil
			err = fmt.Errorf("%w: %v", ErrUnavailable, r)
		}
	}()

	if s.Title == "" {
		s.Title = "ZSH TRANS"
	}
	if s.Width <= 0 {
		s.Width = 1180
	}
	if s.Height <= 0 {
		s.Height = 820
	}
	if s.DataPath == "" {
		s.DataPath = defaultDataPath()
		fmt.Fprintf(os.Stderr, "[deskwin] data path = %s\n", s.DataPath)
		_ = os.MkdirAll(s.DataPath, 0o755)
	}

	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     s.Debug,
		AutoFocus: true,
		DataPath:  s.DataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  s.Title,
			Width:  uint(s.Width),
			Height: uint(s.Height),
			Center: true,
		},
	})
	if wv == nil {
		return nil, ErrUnavailable
	}

	w = &Win{wv: wv}
	if p := wv.Window(); p != nil {
		w.hwnd = uintptr(p)
	}

	// 点 X = 隐藏到托盘（不销毁窗口，页面状态保留）
	subclass(w.hwnd, func() { w.Hide() })

	if s.URL != "" {
		wv.Navigate(s.URL)
	}
	w.shown.Store(true)
	return w, nil
}

// Run 进入消息循环，阻塞直到窗口销毁（主 goroutine 调用）
func (w *Win) Run() {
	if w.wv == nil {
		return
	}
	w.wv.Run()
	w.wv.Destroy()
	w.shown.Store(false)
}

// Shown 窗口是否已创建成功
func (w *Win) Shown() bool { return w.shown.Load() }

// Hidden 窗口当前是否处于「已隐藏到托盘」状态
func (w *Win) Hidden() bool { return w.hidden.Load() }

// Available 窗口是否可用
func (w *Win) Available() bool { return w.wv != nil && w.shown.Load() }

// HWND 返回原生窗口句柄（0 表示未创建）
func (w *Win) HWND() uintptr { return w.hwnd }

// Navigate 切换页面（用于托盘菜单里的 #settings 深链）
func (w *Win) Navigate(url string) {
	if w.wv == nil || url == "" {
		return
	}
	w.wv.Dispatch(func() { w.wv.Navigate(url) })
}

// Eval 在页面里执行脚本（用于托盘菜单跳转到指定页签）
func (w *Win) Eval(js string) {
	if w.wv == nil || js == "" {
		return
	}
	w.wv.Dispatch(func() { w.wv.Eval(js) })
}

// Show 把窗口显示出来并提到前面（最小化则先还原）
func (w *Win) Show() {
	if w.hwnd == 0 {
		return
	}
	if ok, _, _ := procIsIconic.Call(w.hwnd); ok != 0 {
		procShowWindow.Call(w.hwnd, swRestore)
	} else {
		procShowWindow.Call(w.hwnd, swShow)
	}
	procSetForegroundWindow.Call(w.hwnd)
	w.hidden.Store(false)
}

// Hide 隐藏窗口到托盘（不销毁）
func (w *Win) Hide() {
	if w.hwnd == 0 {
		return
	}
	procShowWindow.Call(w.hwnd, swHide)
	w.hidden.Store(true)
}

// Toggle 显隐切换（对应「主窗口显隐」热键）
func (w *Win) Toggle() {
	if w.visible() {
		w.Hide()
		return
	}
	w.Show()
}

// visible 窗口当前是否在屏幕上可见
func (w *Win) visible() bool {
	if w.hwnd == 0 {
		return false
	}
	ok, _, _ := procIsWindowVisible.Call(w.hwnd)
	return ok != 0
}

// Close 真正销毁窗口，Run 随之返回。用于退出程序。
func (w *Win) Close() {
	w.closing.Store(true)
	if w.hwnd != 0 {
		// 走系统默认关闭路径：临时摘掉 hook，再投递 WM_CLOSE
		subclassMu.Lock()
		closeHooks[w.hwnd] = nil
		subclassMu.Unlock()
		procPostMessageW.Call(w.hwnd, wmClose, 0, 0)
		return
	}
	if w.wv != nil {
		w.wv.Dispatch(func() { w.wv.Destroy() })
	}
}

// Title 读取窗口标题（当前值）。用于自检与自动化断言：页面可用 JS 把
// 渲染结果写进 document.title，这里再读出来，即可在不截屏的前提下验证
// WebView2 里真的渲染了内容。
func (w *Win) Title() string {
	if w.hwnd == 0 {
		return ""
	}
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// SetTitle 设置窗口标题（单实例唤醒也按标题查找，退出前需还原）。
func (w *Win) SetTitle(title string) {
	if w.hwnd == 0 || title == "" {
		return
	}
	p, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	procSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(p)))
}

// ActivateExisting 二次启动时唤醒已运行实例的窗口（按标题找窗口）。
// 返回 false 表示没找到（可能已被隐藏到托盘或标题不同），调用方回退到浏览器。
func ActivateExisting(title string) bool {
	p, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return false
	}
	h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(p)))
	if h == 0 {
		return false
	}
	procShowWindow.Call(h, swRestore)
	procShowWindow.Call(h, swShow)
	procSetForegroundWindow.Call(h)
	return true
}
