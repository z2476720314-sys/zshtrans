//go:build windows

// QQImpl IPC binding, adapted from the project's GPL-3.0 Python binding.
package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/local/zshtrans/internal/config"
	"golang.org/x/sys/windows"
)

type captureJob struct {
	ctx   context.Context
	reply chan captureReply
}
type captureReply struct {
	result CaptureResult
	err    error
}
type nativeHost struct {
	jobs     chan captureJob
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	busy     atomic.Bool
	err      error
}

var hostMu sync.Mutex
var captureHost *nativeHost

func Interactive(ctx context.Context) (CaptureResult, error) {
	hostMu.Lock()
	if captureHost == nil {
		captureHost = &nativeHost{jobs: make(chan captureJob), stop: make(chan struct{}), done: make(chan struct{})}
		go captureHost.run()
	}
	h := captureHost
	hostMu.Unlock()
	if !h.busy.CompareAndSwap(false, true) {
		return CaptureResult{}, ErrBusy
	}
	defer h.busy.Store(false)
	job := captureJob{ctx: ctx, reply: make(chan captureReply, 1)}
	select {
	case h.jobs <- job:
	case <-ctx.Done():
		return CaptureResult{}, ctx.Err()
	case <-h.done:
		return CaptureResult{}, errors.New("截图服务已关闭")
	}
	// Wait for cleanup too: callers may restore the application window after return.
	result := <-job.reply
	return result.result, result.err
}

func CloseInteractive() error {
	hostMu.Lock()
	h := captureHost
	hostMu.Unlock()
	if h == nil {
		return nil
	}
	h.stopOnce.Do(func() { close(h.stop) })
	select {
	case <-h.done:
		return h.err
	case <-time.After(5 * time.Second):
		return errors.New("截图进程尚未完成清理")
	}
}

func (h *nativeHost) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(h.done)
	var session *ipcSession
	defer func() {
		if session != nil {
			h.err = session.shutdown()
		}
	}()
	idle := time.NewTicker(time.Second)
	defer idle.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-idle.C:
			if session != nil && session.pid != 0 && !session.hasVisibleWindows() {
				h.err = session.closeChild()
			}
		case job := <-h.jobs:
			root, err := EnsureRuntime(config.BaseDir())
			if err == nil && session == nil {
				session, err = newIPCSession(root)
			}
			var result CaptureResult
			if err == nil {
				result, err = session.capture(job.ctx, h.stop)
			}
			job.reply <- captureReply{result, err}
		}
	}
}

type ipcSession struct {
	dll         windows.Handle
	parent      uintptr
	callback    uintptr
	executable  string
	events      chan nativeEvent
	overflow    atomic.Bool
	generation  atomic.Uint64
	pid         uint32
	process     windows.Handle
	patched     bool
	closeFailed bool
}

func ipcCall(instance uintptr, index int, args ...uintptr) uintptr {
	vtable := *(*uintptr)(unsafe.Pointer(instance))
	fn := *(*uintptr)(unsafe.Pointer(vtable + uintptr(index)*unsafe.Sizeof(uintptr(0))))
	result, _, _ := syscall.SyscallN(fn, append([]uintptr{instance}, args...)...)
	return result
}

func checkHRESULT(value uintptr, operation string) error {
	if uint32(value)&0x80000000 != 0 {
		return fmt.Errorf("%s: HRESULT 0x%08X", operation, uint32(value))
	}
	return nil
}

func newIPCSession(root string) (*ipcSession, error) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return nil, errors.New("原生截图需要 Windows x64")
	}
	dllPath := filepath.Join(root, "native", "parent-ipc-core-x64.dll")
	dll, err := windows.LoadLibraryEx(dllPath, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, fmt.Errorf("加载截图 IPC: %w", err)
	}
	factoryProc, err := windows.GetProcAddress(dll, "DllGetClassObject")
	if err != nil {
		windows.FreeLibrary(dll)
		return nil, err
	}
	clsid, _ := windows.GUIDFromString("{14D5685E-0574-4F1F-B6A4-9BE9C3BE0C0D}")
	factoryIID, _ := windows.GUIDFromString("{00000001-0000-0000-C000-000000000046}")
	parentIID, _ := windows.GUIDFromString("{8504298A-A491-4B40-AAC4-B2608DAC4045}")
	var factory uintptr
	hr, _, _ := syscall.SyscallN(factoryProc, uintptr(unsafe.Pointer(&clsid)), uintptr(unsafe.Pointer(&factoryIID)), uintptr(unsafe.Pointer(&factory)))
	if err := checkHRESULT(hr, "QQImpl DllGetClassObject"); err != nil || factory == 0 {
		windows.FreeLibrary(dll)
		return nil, errors.Join(err, errors.New("截图 class factory 无效"))
	}
	s := &ipcSession{dll: dll, events: make(chan nativeEvent, 64), executable: filepath.Join(root, "runtime", "QQScreenShot-"+qqVersion, "BinRelease-x64", "QQScreenshot.exe")}
	hr = ipcCall(factory, 3, 0, uintptr(unsafe.Pointer(&parentIID)), uintptr(unsafe.Pointer(&s.parent)))
	ipcCall(factory, 2)
	if err := checkHRESULT(hr, "QQImpl CreateInstance"); err != nil || s.parent == 0 {
		windows.FreeLibrary(dll)
		return nil, errors.Join(err, errors.New("截图 IPC interface 无效"))
	}
	s.callback = syscall.NewCallbackCDecl(s.onMessage)
	ipcCall(s.parent, 4, 0, 0)
	ipcCall(s.parent, 3)
	return s, nil
}

func (s *ipcSession) onMessage(generation uintptr, command uintptr, _ uintptr, payload uintptr, size uintptr) uintptr {
	if uint64(generation) != s.generation.Load() {
		return 0
	}
	name := ""
	if command != 0 {
		// Commands are native NUL-terminated strings; do not retain their memory.
		buf := make([]byte, 0, 64)
		for i := uintptr(0); i < 256; i++ {
			b := *(*byte)(unsafe.Pointer(command + i))
			if b == 0 {
				break
			}
			buf = append(buf, b)
		}
		name = string(buf)
	}
	event := nativeEvent{Name: name}
	// QQ's callback length is a signed C int, not a pointer-sized value.
	length := int32(size)
	if length < 0 || length > 32768 || (length != 0 && payload == 0) {
		event.Name = "IPC_PAYLOAD_ERROR"
	} else if length > 0 {
		event.Payload = append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(payload)), int(length))...)
	}
	select {
	case s.events <- event:
	default:
		s.overflow.Store(true)
	}
	return 0
}

func (s *ipcSession) start() error {
	if s.closeFailed {
		return errors.New("截图进程清理失败，请重新启动程序")
	}
	if s.pid != 0 {
		return nil
	}
	exe, err := windows.UTF16PtrFromString(s.executable)
	if err != nil {
		return err
	}
	cache := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(s.executable))), "cache")
	if err := os.MkdirAll(cache, 0700); err != nil {
		return err
	}
	// The IPC launcher inherits the environment and exposes no child-env argument.
	temp, hasTemp := os.LookupEnv("TEMP")
	tmp, hasTmp := os.LookupEnv("TMP")
	os.Setenv("TEMP", cache)
	os.Setenv("TMP", cache)
	generation := s.generation.Add(1)
	pid := ipcCall(s.parent, 5, uintptr(unsafe.Pointer(exe)), 0, 0, s.callback, uintptr(generation))
	if hasTemp {
		os.Setenv("TEMP", temp)
	} else {
		os.Unsetenv("TEMP")
	}
	if hasTmp {
		os.Setenv("TMP", tmp)
	} else {
		os.Unsetenv("TMP")
	}
	if int32(pid) <= 0 {
		return errors.New("QQ 原生截图进程启动失败")
	}
	s.pid = uint32(pid)
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ|windows.PROCESS_VM_WRITE|windows.PROCESS_VM_OPERATION|windows.SYNCHRONIZE, false, s.pid)
	if err != nil {
		return errors.Join(err, s.closeChild())
	}
	s.process = process
	if err := verifyCaptureProcess(process, s.executable); err != nil {
		return errors.Join(err, s.closeChild())
	}
	if err := checkHRESULT(ipcCall(s.parent, 6, uintptr(s.pid)), "QQImpl connect"); err != nil {
		return errors.Join(err, s.closeChild())
	}
	return nil
}

func (s *ipcSession) capture(ctx context.Context, stop <-chan struct{}) (result CaptureResult, err error) {
	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case <-stop:
		return result, context.Canceled
	default:
	}
	for len(s.events) > 0 {
		<-s.events
	}
	s.overflow.Store(false)
	reused := s.pid != 0
	if err = s.start(); err != nil {
		return result, err
	}
	defer func() {
		if (err != nil && !errors.Is(err, ErrCanceled)) || !s.hasVisibleWindows() {
			err = errors.Join(err, s.closeChild())
		}
	}()
	connected := reused
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for !connected {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-stop:
			return result, context.Canceled
		case <-deadline.C:
			return result, errors.New("QQ 原生截图连接超时")
		case event := <-s.events:
			if event.Name == "IPC_CONNECTED" {
				connected = true
			} else if event.Name == "IPC_DISCONNECTED" || event.Name == "IPC_PAYLOAD_ERROR" {
				return result, errors.New("QQ 原生截图连接失败")
			}
		}
	}
	if !s.patched {
		if err = s.patchToolbar(); err != nil {
			return result, err
		}
		s.patched = true
	}
	command := append([]byte("screenShot"), 0)
	if err = checkHRESULT(ipcCall(s.parent, 7, uintptr(s.pid), uintptr(unsafe.Pointer(&command[0])), 1, 0, 0), "QQImpl screenShot"); err != nil {
		return result, err
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	var state captureState
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-stop:
			return result, context.Canceled
		case event := <-s.events:
			state.accept(event, time.Now())
		case now := <-ticker.C:
			if s.overflow.Load() {
				return result, errors.New("原生截图回调队列已满")
			}
			if state.Err != nil {
				return result, state.Err
			}
			if state.ready(now) && len(s.events) == 0 {
				if state.Action == "cancel" {
					return result, ErrCanceled
				}
				result.Action = state.Action
				if result.Action == "translate" || result.Action == "ocr" {
					result.PNG, err = readCaptureImage(state.Payload)
				}
				return result, err
			}
		}
	}
}

func (s *ipcSession) closeChild() error {
	if s.pid == 0 {
		return nil
	}
	s.generation.Add(1)
	err := checkHRESULT(ipcCall(s.parent, 8, uintptr(s.pid), 0, 0), "QQImpl terminate")
	if s.process != 0 {
		status, waitErr := windows.WaitForSingleObject(s.process, 2000)
		if waitErr != nil || status != windows.WAIT_OBJECT_0 {
			s.closeFailed = true
			return errors.Join(err, waitErr, errors.New("截图子进程未退出"))
		}
		windows.CloseHandle(s.process)
	} else if err != nil {
		s.closeFailed = true
		return err
	}
	s.process = 0
	s.pid = 0
	s.patched = false
	return nil
}

func (s *ipcSession) shutdown() error {
	if err := s.closeChild(); err != nil {
		return err
	}
	if s.parent != 0 {
		ipcCall(s.parent, 2)
		s.parent = 0
	}
	// QQ's Mojo threads outlive Release; unloading their DLL terminates the host.
	// Keep this single library loaded until the application process exits.
	return nil
}

var enumCaptureWindows = user32.NewProc("EnumWindows")
var isCaptureWindowVisible = user32.NewProc("IsWindowVisible")
var captureWindowPID = user32.NewProc("GetWindowThreadProcessId")
var captureWindowRect = user32.NewProc("GetWindowRect")

type visibleCapture struct {
	pid          uint32
	found        bool
	screenWidth  int32
	screenHeight int32
}

var captureWindowVisitor = syscall.NewCallback(func(hwnd, param uintptr) uintptr {
	state := (*visibleCapture)(unsafe.Pointer(param))
	visible, _, _ := isCaptureWindowVisible.Call(hwnd)
	if visible == 0 {
		return 1
	}
	var pid uint32
	captureWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != state.pid {
		return 1
	}
	var rect struct{ Left, Top, Right, Bottom int32 }
	if ok, _, _ := captureWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok != 0 && rect.Right > rect.Left && rect.Bottom > rect.Top && (rect.Right-rect.Left < state.screenWidth || rect.Bottom-rect.Top < state.screenHeight) {
		state.found = true
		return 0
	}
	return 1
})

func (s *ipcSession) hasVisibleWindows() bool {
	if s.pid == 0 {
		return false
	}
	width, _, _ := procGetSystemMetrics.Call(smCxScreen)
	height, _, _ := procGetSystemMetrics.Call(smCyScreen)
	state := visibleCapture{pid: s.pid, screenWidth: int32(width), screenHeight: int32(height)}
	enumCaptureWindows.Call(captureWindowVisitor, uintptr(unsafe.Pointer(&state)))
	return state.found
}
