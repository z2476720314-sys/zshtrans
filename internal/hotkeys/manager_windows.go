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
	// wmApp 是线程私有的自定义消息，用来让消息线程重新注册热键。
	// WM_APP 之后的区间留给应用自己用，不会和系统消息撞车。
	wmApp    = 0x8000
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

// spec 一组待注册的热键（id → 组合键），由 Bind/Rebind 写入，消息线程读取。
type spec struct {
	id    int
	value string
	name  string
}

// Manager 全局热键管理器（对应 pyc _HOTKEY_BINDINGS + RegisterHotKey 消息循环）
//
// 线程模型（重要）：RegisterHotKey(hWnd=0) 把热键绑到「调用它的那个线程」的消息队列，
// 只有该线程 GetMessageW(0,...) 才能收到 WM_HOTKEY。所以注册、注销、泵消息
// 三件事必须在同一个 LockOSThread 的 goroutine 里做——这也是 Start 之前
// 不能注册的原因（原来 Bind 在主 goroutine 注册、Start 在子 goroutine 泵消息，
// 热键永远不会触发）。
type Manager struct {
	mu       sync.Mutex
	desired  []spec         // 期望注册的集合（Bind/Rebind 写入）
	handlers map[int]func() // id → 回调
	threadID uint32
	running  bool
	ready    chan struct{}
	started  chan error // Start 期间消息线程把注册结果回传
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{handlers: map[int]func(){}}
}

// Bind 设置一组热键；handler 按 name 分发。
// 只记录配置，真正的 RegisterHotKey 在 Start 的消息线程里执行。
// 返回 nil 表示格式校验通过（是否被系统占用要到 Start/Rebind 才知道）。
func (m *Manager) Bind(selection, screenshot, window, close string, handler func(name string)) error {
	return m.Rebind(selection, screenshot, window, close, handler)
}

// Rebind 热更新：替换热键集合并通知消息线程重注册。
// 未 Start 时只存配置；已 Start 时通过 PostThreadMessage 唤醒消息线程执行注销+注册。
func (m *Manager) Rebind(selection, screenshot, window, close string, handler func(name string)) error {
	items := []spec{
		{1, selection, "selection"},
		{2, screenshot, "screenshot"},
		{3, window, "window"},
		{4, close, "close"},
	}
	// 先做格式校验，别把非法配置送到消息线程
	for _, it := range items {
		if it.value == "" {
			continue
		}
		if _, err := Parse(it.value); err != nil {
			return err
		}
	}

	m.mu.Lock()
	m.desired = items
	m.handlers = map[int]func(){}
	for _, it := range items {
		if it.value == "" {
			continue
		}
		name := it.name
		m.handlers[it.id] = func() { handler(name) }
	}
	threadID, running := m.threadID, m.running
	m.mu.Unlock()

	if running && threadID != 0 {
		// 让消息线程自己重注册（注册/注销有线程亲和性，不能在这里直接调）
		procPostThreadMsg.Call(uintptr(threadID), wmApp, 0, 0)
	}
	return nil
}

// Start 启动消息循环并在同一线程内注册热键。
// 返回注册结果：被占用/非法时返回错误，但消息循环仍在跑（便于之后 Rebind 换键）。
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.ready = make(chan struct{})
	m.started = make(chan error, 1)
	ready, started := m.ready, m.started
	m.mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var msg msgStruct
		// 强制创建线程消息队列，否则 PostThreadMessage 会失败
		procPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)

		tid := windows.GetCurrentThreadId()
		m.mu.Lock()
		m.threadID = tid
		m.mu.Unlock()

		// 在本线程注册（此时才有正确的消息队列归属）
		regErr := m.applyLocked()
		started <- regErr
		close(ready)

		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) <= 0 { // 0 = WM_QUIT，-1 = 出错
				break
			}
			switch msg.Message {
			case wmHotkey:
				m.mu.Lock()
				fn := m.handlers[int(msg.WParam)]
				m.mu.Unlock()
				if fn != nil {
					go fn()
				}
			case wmApp:
				// 配置变了：注销旧的、注册新的
				if err := m.applyLocked(); err != nil {
					// 注册失败不致命：保留旧键已注销的状态，界面会显示未生效
					_ = err
				}
			case wmQuit:
				m.unregisterLocked()
				return
			}
		}
		m.unregisterLocked()
	}()
	<-ready

	m.mu.Lock()
	running := m.running
	m.mu.Unlock()
	if !running {
		return nil
	}
	return <-started
}

// applyLocked 在消息线程内注销旧热键并注册当前 desired。调用方必须已在消息线程。
func (m *Manager) applyLocked() error {
	m.unregisterLocked()

	m.mu.Lock()
	items := make([]spec, len(m.desired))
	copy(items, m.desired)
	m.mu.Unlock()

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
	}
	return nil
}

// unregisterLocked 注销当前已注册的热键。调用方必须已在消息线程。
func (m *Manager) unregisterLocked() {
	m.mu.Lock()
	ids := make([]int, 0, len(m.handlers))
	for id := range m.handlers {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		procUnregisterKey.Call(0, uintptr(id))
	}
}

// Stop 停止消息循环（消息线程退出前自己注销热键）
func (m *Manager) Stop() {
	m.mu.Lock()
	threadID := m.threadID
	m.running = false
	m.mu.Unlock()
	if threadID != 0 {
		procPostThreadMsg.Call(uintptr(threadID), wmQuit, 0, 0)
	}
}
