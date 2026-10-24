//go:build windows

package tray

import (
	"errors"
	"sync"

	"fyne.io/systray"

	"github.com/local/zshtrans/internal/logx"
)

var (
	once    sync.Once
	iconBuf []byte
	actions Actions
	exitFn  = func() {}
)

// Run 初始化托盘并进入消息循环（阻塞，直到 Exit 被调用）
func Run(icon []byte, act Actions) error {
	if len(icon) == 0 {
		return errors.New("tray icon empty")
	}
	iconBuf, actions = icon, act
	once.Do(func() {
		systray.Run(onReady, onExit)
	})
	return nil
}

func onReady() {
	systray.SetIcon(iconBuf)
	systray.SetTooltip("ZSH TRANS")
	systray.SetOnTapped(func() {
		if actions.Show != nil {
			actions.Show()
		}
	})

	mShow := systray.AddMenuItem("显示主窗口", "在浏览器中打开翻译台")
	mCapture := systray.AddMenuItem("截图翻译", "全屏截图 → OCR → 翻译")
	mSettings := systray.AddMenuItem("设置", "打开设置页")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 ZSH TRANS")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if actions.Show != nil {
					actions.Show()
				}
			case <-mCapture.ClickedCh:
				if actions.Capture != nil {
					go actions.Capture()
				}
			case <-mSettings.ClickedCh:
				if actions.Settings != nil {
					actions.Settings()
				}
			case <-mQuit.ClickedCh:
				logx.Info("tray quit requested")
				if actions.Quit != nil {
					actions.Quit()
				}
				systray.Quit()
				return
			}
		}
	}()
}

func onExit() {
	exitFn()
}

// Exit 退出托盘消息循环
func Exit() { systray.Quit() }
