//go:build windows

package instance

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CreateMutex 在互斥量已存在时仍返回有效句柄且 err==nil，
// 因此必须直接读 GetLastError 才能区分"新建"与"已存在"。
var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procCreateMutex  = kernel32.NewProc("CreateMutexW")
	procReleaseMutex = kernel32.NewProc("ReleaseMutex")
	procCloseHandle  = kernel32.NewProc("CloseHandle")

	errAlreadyExists = windows.Errno(183) // ERROR_ALREADY_EXISTS

	mu   sync.Mutex
	held []uintptr
)

type winLock struct{ h uintptr }

func acquire(name string) (Lock, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	r, _, lastErr := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if r == 0 {
		return nil, lastErr
	}
	if lastErr == errAlreadyExists {
		_, _, _ = procCloseHandle.Call(r)
		return nil, ErrAlreadyRunning
	}
	mu.Lock()
	held = append(held, r)
	mu.Unlock()
	return &winLock{h: r}, nil
}

func (l *winLock) Release() {
	if l.h == 0 {
		return
	}
	mu.Lock()
	for i, h := range held {
		if h == l.h {
			held = append(held[:i], held[i+1:]...)
			break
		}
	}
	mu.Unlock()
	_, _, _ = procReleaseMutex.Call(l.h)
	_, _, _ = procCloseHandle.Call(l.h)
	l.h = 0
}
