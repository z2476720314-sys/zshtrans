//go:build windows

package capture

import (
	"errors"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/local/zshtrans/internal/logx"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procGetClipboardSequenceNo = user32.NewProc("GetClipboardSequenceNumber")
	procGetClipboardData       = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvail = user32.NewProc("IsClipboardFormatAvailable")
	procEmptyClipboard         = user32.NewProc("EmptyClipboard")
	procSetClipboardData       = user32.NewProc("SetClipboardData")
	procGetAsyncKeyState       = user32.NewProc("GetAsyncKeyState")
	procKeybdEvent             = user32.NewProc("keybd_event")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
)

const (
	cfUnicodeText  = 13
	gmemMoveable   = 0x0002
	srcCopy        = 0x00CC0020
	smCxScreen     = 0
	smCyScreen     = 1
	vkControl      = 0x11
	vkMenu         = 0x12
	vkShift        = 0x10
	keyeventfKeyup = 0x0002
)

// ClipboardSeq 剪贴板序列号（对应 _win32_clipboard_sequence）
func ClipboardSeq() uint32 {
	n, _, _ := procGetClipboardSequenceNo.Call()
	return uint32(n)
}

// ModifiersDown 修饰键是否按下（对应 _win32_modifiers_pressed）
func ModifiersDown() (ctrl, alt, shift bool) {
	down := func(vk uintptr) bool {
		v, _, _ := procGetAsyncKeyState.Call(vk)
		return int16(v) < 0
	}
	return down(vkControl), down(vkMenu), down(vkShift)
}

// SendCtrlC 模拟 Ctrl+C（对应 send_ctrl_c）
func SendCtrlC() {
	procKeybdEvent.Call(vkControl, 0, 0, 0)
	procKeybdEvent.Call('C', 0, 0, 0)
	procKeybdEvent.Call('C', 0, keyeventfKeyup, 0)
	procKeybdEvent.Call(vkControl, 0, keyeventfKeyup, 0)
}

// ReadClipboard 读取剪贴板文本
func ReadClipboard() (string, error) {
	ok, _, err := procOpenClipboard.Call(0)
	if ok == 0 {
		return "", errors.New("open clipboard fail: " + err.Error())
	}
	defer procCloseClipboard.Call()

	avail, _, _ := procIsClipboardFormatAvail.Call(cfUnicodeText)
	if avail == 0 {
		return "", errors.New("clipboard has no text")
	}
	handle, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", errors.New("get clipboard data fail")
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return "", errors.New("lock clipboard fail")
	}
	defer procGlobalUnlock.Call(handle)
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(ptr))), nil
}

// WriteClipboard 写入剪贴板文本
func WriteClipboard(text string) error {
	utf16, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := uintptr(len(utf16) * 2)
	handle, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return errors.New("global alloc fail")
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		return errors.New("global lock fail")
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16)), utf16)
	procGlobalUnlock.Call(handle)

	ok, _, err := procOpenClipboard.Call(0)
	if ok == 0 {
		procGlobalFree.Call(handle)
		return errors.New("open clipboard fail: " + err.Error())
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	if r, _, _ := procSetClipboardData.Call(cfUnicodeText, handle); r == 0 {
		procGlobalFree.Call(handle)
		return errors.New("set clipboard data fail")
	}
	return nil
}

// SelectedText 取当前选中文本（对应 capture_selected_text）
// 流程：记序列号 → 发 Ctrl+C → 轮询剪贴板变化 → 读出文本 → 还原原剪贴板
//
// 剪贴板还原是原版承诺的行为（注释里写了"还原原剪贴板"），但原实现只把 before
// 用来判空、从不写回，用户每划一次词就永久丢掉自己剪贴板里的东西。这里真的还原。
func SelectedText(wait time.Duration) (string, error) {
	before, _ := ReadClipboard()
	seq := ClipboardSeq()
	SendCtrlC()

	grabbed := ""
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
		if ClipboardSeq() == seq {
			continue
		}
		text, err := ReadClipboard()
		if err != nil {
			continue
		}
		if text != "" {
			grabbed = text
			break
		}
	}
	if grabbed == "" {
		// 超时后尝试直接读一次
		if text, err := ReadClipboard(); err == nil && text != "" && text != before {
			grabbed = text
		}
	}
	// 无论成败都把用户原来的剪贴板内容写回去
	if before != "" {
		if err := WriteClipboard(before); err != nil {
			logx.Warn("clipboard restore fail: %v", err)
		}
	}
	if grabbed == "" {
		return "", errors.New("no selection captured")
	}
	return grabbed, nil
}

// Screenshot 全屏截图，返回 PNG 字节
func Screenshot() ([]byte, error) {
	w, _, _ := procGetSystemMetrics.Call(smCxScreen)
	h, _, _ := procGetSystemMetrics.Call(smCyScreen)
	if w == 0 || h == 0 {
		return nil, errors.New("bad screen metrics")
	}
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("get dc fail")
	}
	defer procReleaseDC.Call(0, screenDC)

	dib, err := newDIBSection(screenDC, int(w), int(h))
	if err != nil {
		return nil, err
	}
	defer dib.close()

	if r, _, _ := procBitBlt.Call(dib.dc, 0, 0, w, h, screenDC, 0, 0, srcCopy); r == 0 {
		return nil, errors.New("bitblt fail")
	}
	return dib.png()
}

// Region 区域截图（x,y,w,h）
func Region(x, y, w, h int) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("bad region")
	}
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("get dc fail")
	}
	defer procReleaseDC.Call(0, screenDC)

	dib, err := newDIBSection(screenDC, w, h)
	if err != nil {
		return nil, err
	}
	defer dib.close()

	if r, _, _ := procBitBlt.Call(dib.dc, 0, 0, uintptr(w), uintptr(h), screenDC, uintptr(x), uintptr(y), srcCopy); r == 0 {
		return nil, errors.New("bitblt fail")
	}
	return dib.png()
}

var _ = syscall.NewLazyDLL
