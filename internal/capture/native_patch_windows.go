//go:build windows

package capture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func sameNativePath(left, right string) bool {
	if resolved, err := filepath.EvalSymlinks(left); err == nil {
		left = resolved
	}
	if resolved, err := filepath.EvalSymlinks(right); err == nil {
		right = resolved
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func verifyCaptureProcess(process windows.Handle, expected string) error {
	name := make([]uint16, 32768)
	size := uint32(len(name))
	if err := windows.QueryFullProcessImageName(process, 0, &name[0], &size); err != nil {
		return err
	}
	if !sameNativePath(windows.UTF16ToString(name[:size]), expected) {
		return errors.New("拒绝操作非本工具启动的截图进程")
	}
	return nil
}

func captureModule(process windows.Handle, path string) (uintptr, error) {
	modules := make([]windows.Handle, 256)
	var needed uint32
	if err := windows.EnumProcessModules(process, &modules[0], uint32(len(modules))*uint32(unsafe.Sizeof(modules[0])), &needed); err != nil {
		return 0, err
	}
	count := int(needed) / int(unsafe.Sizeof(modules[0]))
	if count > len(modules) {
		return 0, errors.New("截图进程模块列表过大")
	}
	for _, module := range modules[:count] {
		name := make([]uint16, 32768)
		if err := windows.GetModuleFileNameEx(process, module, &name[0], uint32(len(name))); err != nil {
			return 0, err
		}
		if sameNativePath(windows.UTF16ToString(name), path) {
			return uintptr(module), nil
		}
	}
	return 0, nil
}

func (s *ipcSession) patchToolbar() error {
	camera := filepath.Join(filepath.Dir(s.executable), "Camera.dll")
	raw, err := os.ReadFile(camera)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "32c72d62f8f597fba018abfb0abc60be00790723822139b78918dc1545e9cef2" {
		return errors.New("原生截图组件版本不匹配")
	}
	if err := verifyCaptureProcess(s.process, s.executable); err != nil {
		return err
	}
	var base uintptr
	deadline := time.Now().Add(2 * time.Second)
	for base == 0 {
		base, err = captureModule(s.process, camera)
		if err != nil {
			return err
		}
		if base != 0 {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("原生截图绘制模块尚未加载")
		}
		time.Sleep(40 * time.Millisecond)
	}
	address := base + 0xAAEFA
	before := make([]byte, 4)
	var transferred uintptr
	if err := windows.ReadProcessMemory(s.process, address, &before[0], 4, &transferred); err != nil {
		return err
	}
	if transferred != 4 {
		return errors.New("无法完整读取原生工具栏指令")
	}
	after, err := repairedInstruction(before)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return nil
	}
	var oldProtection uint32
	if err := windows.VirtualProtectEx(s.process, address, 4, windows.PAGE_EXECUTE_READWRITE, &oldProtection); err != nil {
		return err
	}
	writeErr := windows.WriteProcessMemory(s.process, address+3, &after[3], 1, &transferred)
	if writeErr == nil && transferred != 1 {
		writeErr = errors.New("原生工具栏修正写入不完整")
	}
	if writeErr == nil {
		if ok, _, lastErr := kernel32.NewProc("FlushInstructionCache").Call(uintptr(s.process), address, 4); ok == 0 {
			writeErr = fmt.Errorf("刷新截图指令缓存: %w", lastErr)
		}
	}
	var ignored uint32
	protectionErr := windows.VirtualProtectEx(s.process, address, 4, oldProtection, &ignored)
	if err := errors.Join(writeErr, protectionErr); err != nil {
		return err
	}
	if err := windows.ReadProcessMemory(s.process, address, &before[0], 4, &transferred); err != nil {
		return err
	}
	if transferred != 4 || !bytes.Equal(before, after) {
		return errors.New("原生工具栏修正回读不一致")
	}
	return nil
}
