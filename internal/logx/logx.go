package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu   sync.Mutex
	file *os.File
	path string
)

// Init 打开日志文件（追加模式）；失败时退化为仅控制台
func Init(p string) {
	mu.Lock()
	defer mu.Unlock()
	path = p
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	file = f
}

// Path 当前日志路径
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// Close 关闭日志文件
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
}

// Write 写一行日志（同时输出到 stderr）
func Write(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := time.Now().Format("2006-01-02 15:04:05") + " " + msg + "\n"
	mu.Lock()
	if file != nil {
		_, _ = file.WriteString(line)
	}
	mu.Unlock()
}

// Info 普通信息
func Info(format string, args ...any) { Write("[*] "+format, args...) }

// Warn 警告（对应原脚本 '[!] ...' 前缀）
func Warn(format string, args ...any) { Write("[!] "+format, args...) }

// Err 错误
func Err(format string, args ...any) { Write("[x] "+format, args...) }
