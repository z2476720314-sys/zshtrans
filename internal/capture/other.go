//go:build !windows

package capture

import "errors"

var errUnsupported = errors.New("capture only supported on windows")

// ClipboardSeq 剪贴板序列号
func ClipboardSeq() uint32 { return 0 }

// ModifiersDown 修饰键状态
func ModifiersDown() (bool, bool, bool) { return false, false, false }

// SendCtrlC 模拟复制
func SendCtrlC() {}

// ReadClipboard 读剪贴板
func ReadClipboard() (string, error) { return "", errUnsupported }

// WriteClipboard 写剪贴板
func WriteClipboard(string) error { return errUnsupported }

// SelectedText 取选中文本
func SelectedText(any) (string, error) { return "", errUnsupported }

// Screenshot 全屏截图
func Screenshot() ([]byte, error) { return nil, errUnsupported }

// Region 区域截图
func Region(int, int, int, int) ([]byte, error) { return nil, errUnsupported }
