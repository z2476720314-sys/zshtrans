//go:build !windows

package hotkeys

import "errors"

// Manager 非 Windows 平台的空实现
type Manager struct{}

// NewManager 创建管理器
func NewManager() *Manager { return &Manager{} }

// Bind 注册热键（不支持）
func (m *Manager) Bind(selection, screenshot, window string, handler func(name string)) error {
	return errors.New("global hotkeys only supported on windows")
}

// Rebind 热更新热键（不支持）
func (m *Manager) Rebind(selection, screenshot, window string, handler func(name string)) error {
	return m.Bind(selection, screenshot, window, handler)
}

// Start 启动消息循环
func (m *Manager) Start() error { return nil }

// Stop 停止
func (m *Manager) Stop() {}
