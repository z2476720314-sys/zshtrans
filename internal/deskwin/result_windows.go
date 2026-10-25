//go:build windows

package deskwin

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/local/zshtrans/internal/capture"
	"golang.org/x/sys/windows"
)

var (
	resultCreateWindow     = user32.NewProc("CreateWindowExW")
	resultDestroyWindow    = user32.NewProc("DestroyWindow")
	resultDefWindowProc    = user32.NewProc("DefWindowProcW")
	resultRegisterClass    = user32.NewProc("RegisterClassExW")
	resultGetMessage       = user32.NewProc("GetMessageW")
	resultTranslateMessage = user32.NewProc("TranslateMessage")
	resultDispatchMessage  = user32.NewProc("DispatchMessageW")
	resultPostQuit         = user32.NewProc("PostQuitMessage")
	resultSetTimer         = user32.NewProc("SetTimer")
	resultSendMessage      = user32.NewProc("SendMessageW")
	resultCursor           = user32.NewProc("GetCursorPos")
	resultMonitor          = user32.NewProc("MonitorFromPoint")
	resultMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	resultWindowRectProc   = user32.NewProc("GetWindowRect")
	resultHook             = user32.NewProc("SetWindowsHookExW")
	resultUnhook           = user32.NewProc("UnhookWindowsHookEx")
	resultNextHook         = user32.NewProc("CallNextHookEx")
	resultAsyncKey         = user32.NewProc("GetAsyncKeyState")
	resultDPI              = user32.NewProc("GetDpiForSystem")
	resultMonitorDPI       = windows.NewLazySystemDLL("shcore.dll").NewProc("GetDpiForMonitor")
	resultGetModule        = kernel32.NewProc("GetModuleHandleW")
	resultGDI              = windows.NewLazySystemDLL("gdi32.dll")
	resultCreateFont       = resultGDI.NewProc("CreateFontW")
	resultDeleteObject     = resultGDI.NewProc("DeleteObject")
	resultOpenMu           sync.Mutex
	resultActive           atomic.Uintptr
	resultTexts            sync.Map
	resultClassOnce        sync.Once
	resultClassError       error
	resultMouseCallback    = windows.NewCallback(resultMouseHook)
	resultKeyCallback      = windows.NewCallback(resultKeyboardHook)
)

type resultPoint struct{ X, Y int32 }
type resultRect struct{ Left, Top, Right, Bottom int32 }
type resultMonitorData struct {
	Size          uint32
	Monitor, Work resultRect
	Flags         uint32
}
type resultMessage struct {
	HWnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          resultPoint
	Private        uint32
}
type resultWindowClass struct {
	Size, Style                                                        uint32
	Proc                                                               uintptr
	ClassExtra, WindowExtra                                            int32
	Instance, Icon, Cursor, Background, MenuName, ClassName, SmallIcon uintptr
}

// ShowResult 使用独立消息线程，主 WebView 隐藏时也能显示，且不切换前台焦点。
func ShowResult(result Result) error {
	resultOpenMu.Lock()
	defer resultOpenMu.Unlock()
	CloseResult()
	ready := make(chan error, 1)
	go runResult(result, ready)
	return <-ready
}

func CloseResult() {
	if hwnd := resultActive.Load(); hwnd != 0 {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

func resultUTF16(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }

func runResult(result Result, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := resultGetModule.Call(0)
	className := resultUTF16("ZSHTRANS_RESULT_POPUP")
	resultClassOnce.Do(func() {
		class := resultWindowClass{Proc: windows.NewCallback(resultWindowProc), Instance: instance, Background: 6, ClassName: uintptr(unsafe.Pointer(className))}
		class.Size = uint32(unsafe.Sizeof(class))
		if atom, _, err := resultRegisterClass.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			resultClassError = fmt.Errorf("注册译文窗口失败：%v", err)
		}
	})
	if resultClassError != nil {
		ready <- resultClassError
		return
	}
	point := resultPoint{}
	resultCursor.Call(uintptr(unsafe.Pointer(&point)))
	packed := uintptr(uint64(uint32(point.X)) | uint64(uint32(point.Y))<<32)
	monitor, _, _ := resultMonitor.Call(packed, 2)
	info := resultMonitorData{}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, err := resultMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		ready <- fmt.Errorf("读取显示器范围失败：%v", err)
		return
	}
	dpi := uintptr(96)
	if resultDPI.Find() == nil {
		if value, _, _ := resultDPI.Call(); value > 0 {
			dpi = value
		}
	}
	if resultMonitorDPI.Find() == nil {
		var x, y uint32
		if code, _, _ := resultMonitorDPI.Call(monitor, 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); code == 0 && x > 0 {
			dpi = uintptr(x)
		}
	}
	scale := int(dpi) * 100 / 96
	r := resultWindowRect(int(point.X), int(point.Y), scale, popupRect{int(info.Work.Left), int(info.Work.Top), int(info.Work.Right), int(info.Work.Bottom)})
	hwnd, _, err := resultCreateWindow.Call(0x08000088, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(resultUTF16("翻译结果"))), 0x80800000, uintptr(r.left), uintptr(r.top), uintptr(r.right-r.left), uintptr(r.bottom-r.top), 0, 0, instance, 0)
	if hwnd == 0 {
		ready <- fmt.Errorf("创建译文窗口失败：%v", err)
		return
	}
	defer resultDestroyWindow.Call(hwnd)
	defer func() { resultTexts.Delete(hwnd); resultActive.CompareAndSwap(hwnd, 0) }()
	resultTexts.Store(hwnd, result.Text)
	resultActive.Store(hwnd)
	unit := func(n int) int { return n * scale / 100 }
	width, height := r.right-r.left, r.bottom-r.top
	font, _, _ := resultCreateFont.Call(uintptr(int32(-unit(15))), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(resultUTF16("Segoe UI"))))
	defer resultDeleteObject.Call(font)
	child := func(class, text string, style uintptr, x, y, w, h int, id uintptr) {
		control, _, _ := resultCreateWindow.Call(0, uintptr(unsafe.Pointer(resultUTF16(class))), uintptr(unsafe.Pointer(resultUTF16(text))), 0x50000000|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, id, instance, 0)
		resultSendMessage.Call(control, 0x0030, font, 1)
	}
	pad := unit(14)
	child("STATIC", fmt.Sprintf("翻译结果 · %s · %d ms", result.Provider, result.Milliseconds), 0, pad, pad, width-2*pad, unit(24), 0)
	top := unit(45)
	textHeight := (height - unit(110)) / 2
	child("EDIT", result.Source, 0x200000|0x4|0x40|0x800, pad, top, width-2*pad, textHeight, 0)
	child("EDIT", result.Text, 0x200000|0x4|0x40|0x800, pad, top+textHeight+unit(10), width-2*pad, textHeight, 0)
	child("BUTTON", "复制译文", 0, width-unit(184), height-unit(44), unit(88), unit(30), 1)
	child("BUTTON", "关闭", 0, width-unit(88), height-unit(44), unit(74), unit(30), 2)
	mouseHook, _, _ := resultHook.Call(14, resultMouseCallback, instance, 0)
	keyHook, _, _ := resultHook.Call(13, resultKeyCallback, instance, 0)
	if mouseHook != 0 {
		defer resultUnhook.Call(mouseHook)
	}
	if keyHook != 0 {
		defer resultUnhook.Call(keyHook)
	}
	if mouseHook == 0 || keyHook == 0 {
		resultSetTimer.Call(hwnd, 2, 30, 0)
	}
	resultSetTimer.Call(hwnd, 1, 15000, 0)
	procShowWindow.Call(hwnd, 4)
	ready <- nil
	var msg resultMessage
	for {
		ok, _, _ := resultGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ok) <= 0 {
			break
		}
		resultTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		resultDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func resultWindowProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmClose:
		resultDestroyWindow.Call(hwnd)
		return 0
	case 0x0002:
		resultPostQuit.Call(0)
		return 0
	case 0x0021:
		return 3 // MA_NOACTIVATE：点击复制按钮也不抢走原程序焦点。
	case 0x0111:
		switch wp & 0xffff {
		case 1:
			if value, ok := resultTexts.Load(hwnd); ok {
				if capture.WriteClipboard(value.(string)) == nil {
					resultDestroyWindow.Call(hwnd)
				}
			}
		case 2:
			resultDestroyWindow.Call(hwnd)
		}
		return 0
	case 0x0113:
		if wp == 1 {
			resultDestroyWindow.Call(hwnd)
		} else {
			resultPollDismiss(hwnd)
		}
		return 0
	}
	r, _, _ := resultDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

func dismissOutside(hwnd uintptr, x, y int) {
	var rect resultRect
	if ok, _, _ := resultWindowRectProc.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok != 0 && outsideResult(x, y, popupRect{int(rect.Left), int(rect.Top), int(rect.Right), int(rect.Bottom)}) {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

func resultMouseHook(code, wp, lp uintptr) uintptr {
	if int32(code) >= 0 && (wp == 0x0201 || wp == 0x0204 || wp == 0x0207 || wp == 0x020b) {
		point := (*resultPoint)(unsafe.Pointer(lp))
		dismissOutside(resultActive.Load(), int(point.X), int(point.Y))
	}
	r, _, _ := resultNextHook.Call(0, code, wp, lp)
	return r
}

func resultKeyboardHook(code, wp, lp uintptr) uintptr {
	if int32(code) >= 0 && (wp == 0x0100 || wp == 0x0104) && *(*uint32)(unsafe.Pointer(lp)) == 0x1b {
		CloseResult()
	}
	r, _, _ := resultNextHook.Call(0, code, wp, lp)
	return r
}

func resultPollDismiss(hwnd uintptr) {
	if esc, _, _ := resultAsyncKey.Call(0x1b); esc&1 != 0 {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
	clicked := false
	for _, key := range []uintptr{1, 2, 4, 5, 6} {
		state, _, _ := resultAsyncKey.Call(key)
		clicked = clicked || state&1 != 0
	}
	if clicked {
		point := resultPoint{}
		resultCursor.Call(uintptr(unsafe.Pointer(&point)))
		dismissOutside(hwnd, int(point.X), int(point.Y))
	}
}
