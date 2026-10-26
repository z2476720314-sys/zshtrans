package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/local/zshtrans/internal/assets"
	"github.com/local/zshtrans/internal/capture"
	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/console"
	"github.com/local/zshtrans/internal/deskwin"
	"github.com/local/zshtrans/internal/hotkeys"
	"github.com/local/zshtrans/internal/instance"
	"github.com/local/zshtrans/internal/logx"
	"github.com/local/zshtrans/internal/phrases"
	"github.com/local/zshtrans/internal/server"
	"github.com/local/zshtrans/internal/tray"
	"github.com/local/zshtrans/internal/web"
)

// 主窗口标题（单实例二次启动时按标题找窗口唤醒）
const winTitle = "ZSH TRANS · 跨境翻译台"

func main() {
	var (
		noHot   = flag.Bool("no-hotkeys", false, "不注册全局快捷键")
		noTray  = flag.Bool("no-tray", false, "不显示系统托盘图标")
		noInst  = flag.Bool("no-single", false, "允许多开")
		conOut  = flag.Bool("console", false, "日志同时输出到控制台")
		wvDebug = flag.Bool("wv-debug", false, "WebView2 调试模式")
	)
	flag.Parse()
	if *conOut {
		console.Attach()
		logx.EnableConsole()
	}
	logx.Init(config.LogPath())
	defer logx.Close()
	logx.Info("%s %s starting, base=%s", config.AppName, config.AppVersion, config.BaseDir())

	if !*noInst {
		lock, err := instance.Acquire("Global\\ZSHTRANS_SINGLE_INSTANCE")
		if errors.Is(err, instance.ErrAlreadyRunning) {
			if !deskwin.ActivateExisting(winTitle) {
				deskwin.ShowError("程序已在运行，请从托盘打开主窗口。")
			}
			return
		}
		if err != nil {
			logx.Warn("single instance unavailable: %v", err)
		} else {
			defer lock.Release()
		}
	}
	if err := config.MigrateLegacy(); err != nil {
		logx.Err("legacy profile migration failed: %v", err)
		deskwin.ShowError("无法导入旧版配置：" + err.Error())
		return
	}
	phrases.Init(config.PhrasesPath())
	phrases.Seed()
	app := server.NewApp()
	cfg := app.Config()
	asset, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		deskwin.ShowError("界面资源不可用：" + err.Error())
		return
	}
	pagePath, err := web.WriteDesktopPage(asset, filepath.Join(config.BaseDir(), "UI"))
	if err != nil {
		deskwin.ShowError("无法准备桌面界面：" + err.Error())
		return
	}
	pageURL := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(pagePath)}).String()
	win, err := deskwin.Open(deskwin.Settings{
		DataPath: filepath.Join(config.BaseDir(), "WebView2"), Title: winTitle,
		Width: 1180, Height: 820, Debug: *wvDebug,
	})
	if err != nil {
		logx.Err("webview2 unavailable: %v", err)
		deskwin.ShowError("无法创建桌面窗口。请安装或修复 Microsoft Edge WebView2 Runtime 后重试。\n\n" + err.Error())
		return
	}
	logx.Info("desktop window created (hwnd=%#x), in-process transport, no TCP listener", win.HWND())
	srv := server.New(app, asset)
	srv.SetWindower(win)
	srv.SetResultHook(func(result server.Result) {
		if err := deskwin.ShowResult(deskwin.Result{Source: result.Source, Text: result.Text, Provider: result.Provider, Milliseconds: int64(result.MS)}); err != nil {
			logx.Warn("result popup unavailable: %v", err)
		}
	})
	defer deskwin.CloseResult()
	defer capture.CloseInteractive()
	emit := func(name string, value any) {
		if !win.Available() {
			return
		}
		raw, err := json.Marshal(value)
		if err == nil {
			win.Eval("window.dispatchEvent(new CustomEvent('" + name + "',{detail:" + string(raw) + "}))")
		}
	}
	bridge := server.NewDesktopBridge(srv.Handler(), func(response server.DesktopResponse) { emit("zsh-response", response) })
	defer bridge.Close()
	if err := win.Bind("_zshRequest", bridge.Request); err != nil {
		deskwin.ShowError("桌面通信初始化失败：" + err.Error())
		return
	}
	if err := win.Bind("_zshCancel", bridge.Cancel); err != nil {
		deskwin.ShowError("桌面通信初始化失败：" + err.Error())
		return
	}
	win.Init(web.DesktopScript(pageURL, bridge.Token()))
	win.Navigate(pageURL)
	ctx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	go srv.DesktopEvents(ctx, func(event server.Event) { emit("zsh-event", event) })
	if *conOut {
		go tailLog()
	}

	var mgr *hotkeys.Manager
	if !*noHot {
		mgr = hotkeys.NewManager()
		handler := func(name string) { logx.Info("hotkey fired: %s", name); server.FireHotkey(app, name) }
		bindErr := mgr.Bind(cfg.Hotkeys.Selection, cfg.Hotkeys.Screenshot, cfg.Hotkeys.Window, handler)
		if bindErr != nil {
			logx.Warn("hotkey format invalid: %v", bindErr)
		}
		if err := mgr.Start(); err != nil {
			logx.Warn("hotkeys unavailable: %v", err)
		} else if bindErr == nil {
			logx.Info("hotkeys registered")
		}
		srv.SetHotkeyHook(func(h config.Hotkeys) error {
			return mgr.Rebind(h.Selection, h.Screenshot, h.Window, handler)
		})
	} else {
		srv.SetHotkeyHook(func(config.Hotkeys) error { return errors.New("全局快捷键已禁用（-no-hotkeys）") })
	}
	srv.SetExitHook(requestExit)
	if !*noTray {
		go func() {
			if err := tray.Run(assets.Icon, tray.Actions{
				Show:     win.Show,
				Settings: func() { win.Eval("location.hash = '#settings'"); win.Show() },
				Capture:  func() { server.FireHotkey(app, "screenshot") },
				Quit:     requestExit,
			}); err != nil {
				logx.Warn("tray unavailable: %v", err)
			}
		}()
	}
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() { <-sigCh; cancelRuntime(); bridge.Close(); win.Close() }()
	win.Run()
	cancelRuntime()
	bridge.Close()
	if mgr != nil {
		mgr.Stop()
	}
	logx.Info("bye")
}

var sigCh = make(chan os.Signal, 1)

// requestExit 托盘「退出」/ 无窗口模式下的信号 → 走统一收尾路径
func requestExit() {
	select {
	case sigCh <- syscall.SIGTERM:
	default:
	}
}

// tailLog 把日志文件增量打印到控制台（-console）
func tailLog() {
	var offset int64
	for {
		time.Sleep(400 * time.Millisecond)
		raw, err := os.ReadFile(config.LogPath())
		if err != nil {
			continue
		}
		if int64(len(raw)) <= offset {
			continue
		}
		os.Stdout.Write(raw[offset:])
		offset = int64(len(raw))
	}
}
