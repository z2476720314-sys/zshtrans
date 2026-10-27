//go:build windows

package capture

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/local/zshtrans/internal/hotkeys"
	"github.com/local/zshtrans/internal/logx"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procOpenClipboard          nativeProcedure = user32.NewProc("OpenClipboard")
	procCloseClipboard         nativeProcedure = user32.NewProc("CloseClipboard")
	procGetClipboardSequenceNo nativeProcedure = user32.NewProc("GetClipboardSequenceNumber")
	procGetClipboardData       nativeProcedure = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvail                 = user32.NewProc("IsClipboardFormatAvailable")
	procEmptyClipboard         nativeProcedure = user32.NewProc("EmptyClipboard")
	procSetClipboardData       nativeProcedure = user32.NewProc("SetClipboardData")
	procGetAsyncKeyState       nativeProcedure = user32.NewProc("GetAsyncKeyState")
	procKeybdEvent                             = user32.NewProc("keybd_event")
	procSendInput              nativeProcedure = user32.NewProc("SendInput")
	procGetForegroundWindow    nativeProcedure = user32.NewProc("GetForegroundWindow")
	procGetSystemMetrics                       = user32.NewProc("GetSystemMetrics")
	procGetDC                                  = user32.NewProc("GetDC")
	procReleaseDC                              = user32.NewProc("ReleaseDC")

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
func SendCtrlC() { _ = sendCopyInput("") }

type nativeMouseInput struct {
	DX, DY            int32
	Data, Flags, Time uint32
	ExtraInfo         uintptr
}
type nativeKeyboardInput struct {
	VK, Scan    uint16
	Flags, Time uint32
	ExtraInfo   uintptr
}
type nativeInput struct {
	Type uint32
	_    [0]uintptr
	Data [unsafe.Sizeof(nativeMouseInput{})]byte
}

func sendCopyInput(hotkey string) error {
	var releaseKeys []uint16
	if hotkey != "" {
		combo, err := hotkeys.Parse(hotkey)
		if err != nil {
			return err
		}
		releaseKeys = append(releaseKeys, uint16(combo.Key))
		for _, mod := range []struct {
			flag uint32
			key  uint16
		}{
			{hotkeys.ModControl, vkControl}, {hotkeys.ModShift, vkShift}, {hotkeys.ModAlt, vkMenu},
			{hotkeys.ModWin, 0x5B}, {hotkeys.ModWin, 0x5C},
		} {
			if combo.Mods&mod.flag != 0 {
				releaseKeys = append(releaseKeys, mod.key)
			}
		}
	}
	var events []nativeKeyboardInput
	if len(releaseKeys) > 0 {
		events = append(events, nativeKeyboardInput{VK: releaseKeys[0], Flags: keyeventfKeyup})
	}
	events = append(events, nativeKeyboardInput{VK: vkControl})
	// Ctrl must be held before releasing Alt/Win, or the browser may focus its menu.
	for i, key := range releaseKeys {
		if i > 0 && key != vkControl {
			events = append(events, nativeKeyboardInput{VK: key, Flags: keyeventfKeyup})
		}
	}
	events = append(events, nativeKeyboardInput{VK: 'C'}, nativeKeyboardInput{VK: 'C', Flags: keyeventfKeyup}, nativeKeyboardInput{VK: vkControl, Flags: keyeventfKeyup})
	send := func(keys []nativeKeyboardInput) (uintptr, error) {
		inputs := make([]nativeInput, len(keys))
		for i, event := range keys {
			inputs[i].Type = 1
			*(*nativeKeyboardInput)(unsafe.Pointer(&inputs[i].Data[0])) = event
		}
		written, _, err := procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
		runtime.KeepAlive(inputs)
		return written, err
	}
	written, err := send(events)
	if written != uintptr(len(events)) {
		releases := []nativeKeyboardInput{{VK: 'C', Flags: keyeventfKeyup}, {VK: vkControl, Flags: keyeventfKeyup}}
		for _, key := range releaseKeys {
			releases = append(releases, nativeKeyboardInput{VK: key, Flags: keyeventfKeyup})
		}
		_, _ = send(releases)
		return fmt.Errorf("SendInput copied %d/%d events: %v", written, len(events), err)
	}
	return nil
}

var selectionMu sync.Mutex

// SelectedText 取当前选中文本（对应 capture_selected_text）
// 不能完整快照的格式在模拟复制前拒绝；恢复不会覆盖后续用户复制。
func SelectedText(wait time.Duration, hotkey string) (string, error) {
	if !selectionMu.TryLock() {
		return "", errors.New("划词取词正在进行中")
	}
	defer selectionMu.Unlock()
	source, _, _ := procGetForegroundWindow.Call()
	if source == 0 {
		return "", errors.New("无法确认取词窗口，请保持文字选区后重试")
	}
	var trigger uint32
	if hotkey != "" {
		combo, err := hotkeys.Parse(hotkey)
		if err != nil {
			return "", err
		}
		trigger = combo.Key
	}
	pressed := func() bool {
		ctrl, alt, shift := ModifiersDown()
		left, _, _ := procGetAsyncKeyState.Call(0x5B)
		right, _, _ := procGetAsyncKeyState.Call(0x5C)
		if trigger != 0 {
			key, _, _ := procGetAsyncKeyState.Call(uintptr(trigger))
			if int16(key) < 0 {
				return true
			}
		}
		return ctrl || alt || shift || int16(left) < 0 || int16(right) < 0
	}
	sameSource := func() bool { current, _, _ := procGetForegroundWindow.Call(); return current == source }
	var before clipboardSnapshot
	copySelection := func() error {
		if !sameSource() {
			return errors.New("取词窗口已切换，请重新选中文字")
		}
		var err error
		before, err = snapshotClipboard()
		if err != nil {
			return err
		}
		if !sameSource() {
			return errors.New("取词窗口已切换，请重新选中文字")
		}
		return sendCopyInput(hotkey)
	}
	if hotkey != "" {
		if err := copySelection(); err != nil {
			return "", err
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
		defer cancel()
		if err := copyWhenReleased(ctx, pressed, copySelection); err != nil {
			return "", err
		}
	}
	var observedSequence uint32
	defer func() {
		if observedSequence != 0 {
			if err := writeClipboardFormats(before.items, &observedSequence); err != nil && !errors.Is(err, errClipboardChanged) {
				logx.Warn("clipboard restore fail: %v", err)
			}
		}
	}()
	start := time.Now()
	deadline := start.Add(wait)
	retryAt := start.Add(min(250*time.Millisecond, wait/2))
	attempts := 1
	for time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
		if !sameSource() {
			return "", errors.New("取词窗口已切换，请重新选中文字")
		}
		// GetClipboardData renders delayed text before its sequence can advance.
		text, sequence, err := readClipboardTextWithSequence()
		if !sameSource() {
			return "", errors.New("取词窗口已切换，请重新选中文字")
		}
		if sequence != 0 && sequence != before.sequence {
			observedSequence = sequence
			if err == nil && text != "" {
				logx.Info("selection captured: attempts=%d", attempts)
				return text, nil
			}
		}
		if attempts == 1 && !time.Now().Before(retryAt) && !pressed() {
			if !sameSource() {
				return "", errors.New("取词窗口已切换，请重新选中文字")
			}
			if err := sendCopyInput(""); err != nil {
				return "", err
			}
			attempts++
		}
	}
	logx.Warn("selection not captured: attempts=%d", attempts)
	return "", errors.New("no selection captured")
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
