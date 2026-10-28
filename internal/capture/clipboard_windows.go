//go:build windows

package capture

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeProcedure interface {
	Call(...uintptr) (uintptr, uintptr, error)
}

var procEnumClipboardFormats nativeProcedure = user32.NewProc("EnumClipboardFormats")
var procGlobalSize = kernel32.NewProc("GlobalSize")
var errClipboardChanged = errors.New("剪贴板已有新内容，保留用户的新复制")

const (
	clipboardMaxBytes = 64 << 20
	cfBitmap          = 2
	cfDIB             = 8
	cfPalette         = 9
	cfDIBV5           = 17
)

type clipboardItem struct {
	format uint32
	data   []byte
}
type clipboardSnapshot struct {
	items    []clipboardItem
	sequence uint32
}

func ReadClipboard() (string, error) {
	text, _, err := readClipboardTextWithSequence()
	return text, err
}

func readClipboardTextWithSequence() (string, uint32, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ok, _, err := procOpenClipboard.Call(0); ok == 0 {
		return "", 0, fmt.Errorf("打开剪贴板: %w", err)
	}
	defer procCloseClipboard.Call()
	data, err := readClipboardBlock(cfUnicodeText)
	sequence := ClipboardSeq()
	if err != nil {
		return "", sequence, err
	}
	if len(data)%2 != 0 {
		return "", sequence, errors.New("剪贴板 Unicode 格式无效")
	}
	text := make([]uint16, len(data)/2)
	for i := range text {
		text[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	return windows.UTF16ToString(text), sequence, nil
}

func readClipboardBlock(format uint32) ([]byte, error) {
	// These formats contain object handles rather than transferable HGLOBAL bytes.
	if format == 2 || format == 3 || format == 9 || format == 14 || format == 0x80 || format == 0x82 || format == 0x83 || format == 0x8e || (format >= 0x200 && format <= 0x3ff) {
		return nil, errors.New("剪贴板含不能安全保留的图片或对象，请先复制选中文字或使用截图取词")
	}
	handle, _, _ := procGetClipboardData.Call(uintptr(format))
	if handle == 0 {
		return nil, fmt.Errorf("无法保留剪贴板格式 %d", format)
	}
	size, _, _ := procGlobalSize.Call(handle)
	if size == 0 || size > clipboardMaxBytes {
		return nil, errors.New("剪贴板格式无法安全快照或超过 64 MiB")
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return nil, errors.New("锁定剪贴板数据失败")
	}
	defer procGlobalUnlock.Call(handle)
	return append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size))...), nil
}

func snapshotClipboard() (clipboardSnapshot, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var snapshot clipboardSnapshot
	if ok, _, err := procOpenClipboard.Call(0); ok == 0 {
		return snapshot, fmt.Errorf("打开剪贴板: %w", err)
	}
	defer procCloseClipboard.Call()
	var format uintptr
	total := 0
	hasImageHandle, hasDIB := false, false
	for {
		kernel32.NewProc("SetLastError").Call(0)
		next, _, err := procEnumClipboardFormats.Call(format)
		if next == 0 {
			if err != nil && err != syscall.Errno(0) {
				return snapshot, fmt.Errorf("枚举剪贴板格式: %w", err)
			}
			break
		}
		format = next
		// Windows reconstructs bitmap/palette handles from the preserved DIB bytes.
		if format == cfBitmap || format == cfPalette {
			hasImageHandle = true
			continue
		}
		if len(snapshot.items) >= 256 {
			return snapshot, errors.New("剪贴板格式过多，已停止划词")
		}
		data, err := readClipboardBlock(uint32(format))
		if err != nil {
			return snapshot, err
		}
		total += len(data)
		if total > clipboardMaxBytes {
			return snapshot, errors.New("剪贴板快照超过 64 MiB，已停止划词")
		}
		snapshot.items = append(snapshot.items, clipboardItem{uint32(format), data})
		hasDIB = hasDIB || format == cfDIB || format == cfDIBV5
	}
	if hasImageHandle && !hasDIB {
		imageFormat := uint32(cfDIBV5)
		data, err := readClipboardBlock(imageFormat)
		if err != nil {
			imageFormat = cfDIB
			data, err = readClipboardBlock(imageFormat)
		}
		if err != nil {
			return snapshot, err
		}
		if len(snapshot.items) >= 256 || total+len(data) > clipboardMaxBytes {
			return snapshot, errors.New("剪贴板图片快照超过安全限制，已停止划词")
		}
		snapshot.items = append(snapshot.items, clipboardItem{imageFormat, data})
	}
	snapshot.sequence = ClipboardSeq()
	if snapshot.sequence == 0 {
		return snapshot, errors.New("无法确认剪贴板序列，已停止划词")
	}
	return snapshot, nil
}

func allocateClipboardData(data []byte) (uintptr, error) {
	if len(data) == 0 {
		return 0, errors.New("剪贴板数据为空")
	}
	handle, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if handle == 0 {
		return 0, errors.New("剪贴板内存分配失败")
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		return 0, errors.New("剪贴板内存锁定失败")
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
	procGlobalUnlock.Call(handle)
	return handle, nil
}

func WriteClipboard(text string) error {
	selectionMu.Lock()
	defer selectionMu.Unlock()
	utf16, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	data := make([]byte, len(utf16)*2)
	for i, value := range utf16 {
		binary.LittleEndian.PutUint16(data[2*i:], value)
	}
	return writeClipboardFormats([]clipboardItem{{format: cfUnicodeText, data: data}}, nil)
}

func writeClipboardFormats(items []clipboardItem, expectedSequence *uint32) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	handles := make([]uintptr, len(items))
	defer func() {
		for _, handle := range handles {
			if handle != 0 {
				procGlobalFree.Call(handle)
			}
		}
	}()
	for i, item := range items {
		handle, err := allocateClipboardData(item.data)
		if err != nil {
			return err
		}
		handles[i] = handle
	}
	class, _ := windows.UTF16PtrFromString("STATIC")
	owner, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0)
	if owner == 0 {
		return fmt.Errorf("创建剪贴板 owner: %w", err)
	}
	defer user32.NewProc("DestroyWindow").Call(owner)
	if ok, _, err := procOpenClipboard.Call(owner); ok == 0 {
		return fmt.Errorf("打开剪贴板: %w", err)
	}
	defer procCloseClipboard.Call()
	if expectedSequence != nil && ClipboardSeq() != *expectedSequence {
		return errClipboardChanged
	}
	if ok, _, err := procEmptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("清空剪贴板: %w", err)
	}
	for i, item := range items {
		if ok, _, err := procSetClipboardData.Call(uintptr(item.format), handles[i]); ok == 0 {
			return fmt.Errorf("写入剪贴板格式 %d: %w", item.format, err)
		}
		handles[i] = 0 // Windows owns each successfully transferred memory block.
	}
	return nil
}
