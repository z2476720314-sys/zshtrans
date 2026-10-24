// Package instance 单实例互斥（对应 translate.py 单实例保护：二次启动不重复起服务，
// 而是唤起已运行实例的窗口后自行退出）。
package instance

import "errors"

// ErrAlreadyRunning 已存在运行中的实例
var ErrAlreadyRunning = errors.New("another instance is already running")

// Lock 获取单实例锁；成功返回释放函数。
// 已存在实例时返回 ErrAlreadyRunning，调用方应唤起旧窗口后退出。
type Lock interface {
	Release()
}

// Acquire 获取单实例锁（name 为互斥量名）
func Acquire(name string) (Lock, error) {
	return acquire(name)
}
