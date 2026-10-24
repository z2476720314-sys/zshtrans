//go:build windows

package hotkeys

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procRegisterKey   = user32.NewProc("RegisterHotKey")
	procUnregisterKey = user32.NewProc("UnregisterHotKey")
	procGetMessage    = user32.NewProc("GetMessageW")
	procPeekMessage   = user32.NewProc("PeekMessageW")
	procPostThreadMsg = user32.NewProc("PostThreadMessageW")
)

const (
	wmHotkey = 0x0312
	wmQuit   = 0x0012
	pmRemove = 0x0001
)

// msgStruct Win32 MSG
type msgStruct struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// Manager 全局热键管理器（对应 pyc _HOTKEY_BINDINGS + RegisterHotKey 消息循环）
type Manager struct {
	mu       sync.Mutex
	handlers map[int]func()
	threadID uint32
	running  bool
	ready    chan struct{}
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{handlers: map[int]func(){}}
}

// Bind 注册一组热键；handler 按 name 分发。
// 任一热键非法或被占用时，整组回滚并返回错误（对应 SettingsWindow 冲突提示）。
func (m *Manager) Bind(selection, screenshot, window, close string, handler func(name string)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	type item struct {
		name  string
		value string
		id    int
	}
	items := []item{
		{"selection", selection, 1},
		{"screenshot", screenshot, 2},
		{"window", window, 3},
		{"close", close, 4},
	}
	m.unregisterLocked()
	m.handlers = map[int]func(){}

	for _, it := range items {
		if it.value == "" {
			continue
		}
		combo, err := Parse(it.value)
		if err != nil {
			m.unregisterLocked()
			return err
		}
		r, _, e := procRegisterKey.Call(0, uintptr(it.id), uintptr(combo.Mods|ModNoRepeat), uintptr(combo.Key))
		if r == 0 {
			m.unregisterLocked()
			return errors.New("热键 " + it.value + " 注册失败（可能被系统或其他程序占用）: " + e.Error())
		}
		name := it.name
		m.handlers[it.id] = func() { handler(name) }
	}
	return nil
}

func (m *Manager) unregisterLocked() {
	for id := range m.handlers {
		procUnregisterKey.Call(0, uintptr(id))
	}
	m.handlers = map[int]func(){}
}

// Start 启动消息循环（必须锁定 OS 线程）
func (m *Manager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.ready = make(chan struct{})
	ready := m.ready
	m.mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var msg msgStruct
		// 强制创建线程消息队列
		procPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)

		tid := windows.GetCurrentThreadId()
		m.mu.Lock()
		m.threadID = tid
		m.mu.Unlock()
		close(ready)

		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			if msg.Message == wmHotkey {
				m.mu.Lock()
				fn := m.handlers[int(msg.WParam)]
				m.mu.Unlock()
				if fn != nil {
					go fn()
				}
			}
		}
	}()
	<-ready
}

// Stop 停止消息循环并注销热键
func (m *Manager) Stop() {
	m.mu.Lock()
	threadID := m.threadID
	m.running = false
	m.unregisterLocked()
	m.mu.Unlock()
	if threadID != 0 {
		procPostThreadMsg.Call(uintptr(threadID), wmQuit, 0, 0)
	}
}
