package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
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
		port    = flag.Int("port", 0, "HTTP 端口（默认读配置，配置缺省 8765）")
		host    = flag.String("host", "", "监听地址（默认读配置）")
		noOpen  = flag.Bool("no-open", false, "不自动打开界面（只起服务）")
		useWeb  = flag.Bool("web", false, "用默认浏览器承载界面（默认用 WebView2 桌面窗口）")
		noHot   = flag.Bool("no-hotkeys", false, "不注册全局快捷键")
		noTray  = flag.Bool("no-tray", false, "不显示系统托盘图标")
		noInst  = flag.Bool("no-single", false, "允许多开（不启用单实例保护）")
		conOut  = flag.Bool("console", false, "日志同时输出到控制台")
		wvDebug = flag.Bool("wv-debug", false, "WebView2 调试模式（开 DevTools）")
	)
	flag.Parse()

	// 发布版用 -H windowsgui 链接（双击不弹黑框），代价是进程没有控制台。
	// -console 是排障手段，这里按需现挂一个，别让文档里的用法失效。
	if *conOut {
		console.Attach()
		logx.EnableConsole()
	}

	logx.Init(config.LogPath())
	defer logx.Close()
	logx.Info("%s %s starting, base=%s", config.AppName, config.AppVersion, config.BaseDir())

	// ---------- 单实例 ----------
	var lock instance.Lock
	if !*noInst {
		l, err := instance.Acquire("Global\\ZSHTRANS_SINGLE_INSTANCE")
		if errors.Is(err, instance.ErrAlreadyRunning) {
			cfg, _ := config.Load()
			p := cfg.Web.Port
			if p == 0 {
				p = 8765
			}
			if *port > 0 {
				p = *port
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/", p)
			logx.Info("another instance running, activating")
			// 正常桌面模式只唤醒已有窗口；浏览器仅用于显式 -web 调试。
			activated := false
			if !*useWeb && !*noOpen {
				activated = deskwin.ActivateExisting(winTitle)
			}
			if !activated && *useWeb && !*noOpen {
				openBrowser(url)
			}
			if !activated && !*useWeb && !*noOpen {
				fmt.Fprintf(os.Stderr, "%s 已在运行，请从托盘打开主窗口\n", config.AppName)
				return
			}
			fmt.Printf("%s 已在运行，已唤起\n", config.AppName)
			return
		}
		if err != nil {
			logx.Warn("single instance unavailable: %v", err)
		} else {
			lock = l
			defer lock.Release()
		}
	}

	if err := config.MigrateLegacy(); err != nil {
		logx.Err("legacy profile migration failed: %v", err)
		fmt.Fprintln(os.Stderr, err)
		return
	}
	phrases.Init(config.PhrasesPath())
	phrases.Seed()

	app := server.NewApp()
	cfg := app.Config()

	listenHost := cfg.Web.Host
	if !*useWeb {
		listenHost = "127.0.0.1"
	}
	if *host != "" {
		listenHost = *host
	}
	listenPort := cfg.Web.Port
	if *port > 0 {
		listenPort = *port
	}
	if listenPort == 0 {
		listenPort = 8765
	}

	asset, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		logx.Err("embed asset fail: %v", err)
		os.Exit(1)
	}

	srv := server.New(app, asset)
	srv.SetResultHook(func(result server.Result) {
		if err := deskwin.ShowResult(deskwin.Result{Source: result.Source, Text: result.Text, Provider: result.Provider, Milliseconds: int64(result.MS)}); err != nil {
			logx.Warn("result popup unavailable: %v", err)
		}
	})
	defer deskwin.CloseResult()
	defer capture.CloseInteractive()
	addr := net.JoinHostPort(listenHost, strconv.Itoa(listenPort))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// 慢速攻击（slowloris）防护：只有 ReadHeaderTimeout 时，body 可以无限慢地传。
		// 60s 足够覆盖 OCR 大图上传（24MB 上限），也比零值安全得多。
		ReadTimeout: 60 * time.Second,
		// SSE 长连接要保持，IdleTimeout 只影响 keep-alive 之间的空转
		IdleTimeout: 120 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logx.Err("listen %s fail: %v", addr, err)
		fmt.Fprintf(os.Stderr, "端口 %d 被占用或不可用：%v\n", listenPort, err)
		os.Exit(1)
	}
	logx.Info("http listening on %s", addr)

	localURL := fmt.Sprintf("http://127.0.0.1:%d/", listenPort)
	lanURL := fmt.Sprintf("http://%s:%d/", server.LanIP(), listenPort)
	if *conOut {
		go tailLog()
	}

	// ---------- 桌面窗口（必须在主 goroutine 创建，见 deskwin 包注释） ----------
	var win *deskwin.Win
	if !*useWeb && !*noOpen {
		w, wErr := deskwin.Open(deskwin.Settings{
			DataPath: filepath.Join(config.BaseDir(), "WebView2"),
			Title:    winTitle,
			URL:      localURL,
			Width:    1180,
			Height:   820,
			Debug:    *wvDebug,
		})
		if wErr != nil {
			logx.Err("webview2 unavailable: %v", wErr)
			deskwin.ShowError("无法创建桌面窗口。请安装或修复 Microsoft Edge WebView2 Runtime 后重试。\n\n" + wErr.Error())
			_ = ln.Close()
			os.Exit(1)
		} else {
			win = w
			srv.SetWindower(win)
			logx.Info("desktop window created (hwnd=%#x)", win.HWND())
		}
	}

	fmt.Printf("%s %s\n  本机: %s\n  局域网: %s\n", config.AppName, config.AppVersion, localURL, lanURL)

	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logx.Err("http serve fail: %v", err)
		}
	}()

	// ---------- 全局快捷键 ----------
	// 注意：RegisterHotKey(hWnd=0) 把热键绑到「调用线程」的消息队列，
	// 所以注册和泵消息必须在同一个线程。Start() 内部先 LockOSThread 再注册。
	var mgr *hotkeys.Manager
	if !*noHot {
		mgr = hotkeys.NewManager()
		handler := func(name string) {
			logx.Info("hotkey fired: %s", name)
			server.FireHotkey(app, name)
		}
		if err := mgr.Bind(cfg.Hotkeys.Selection, cfg.Hotkeys.Screenshot, cfg.Hotkeys.Window, cfg.Hotkeys.Close, handler); err != nil {
			logx.Warn("hotkey format invalid: %v", err)
			fmt.Fprintf(os.Stderr, "快捷键配置无效：%v\n", err)
			mgr = nil
		} else if err := mgr.Start(); err != nil {
			// 注册失败（被占用）时消息循环仍在跑，改设置里的键位即可热更新生效
			logx.Warn("hotkeys unavailable: %v", err)
			fmt.Fprintf(os.Stderr, "快捷键未生效：%v\n", err)
		} else {
			logx.Info("hotkeys registered")
		}
		// 设置页保存后热更新热键（原来只换 cfg 不重建，改了键位要重启才生效）
		srv.SetHotkeyHook(func(h config.Hotkeys) error {
			if mgr == nil {
				return errors.New("全局快捷键已禁用（-no-hotkeys）")
			}
			return mgr.Rebind(h.Selection, h.Screenshot, h.Window, h.Close, handler)
		})
	}

	// 浏览器只作为显式要求的开发审查入口。
	if *useWeb && !*noOpen {
		openBrowser(localURL)
	}

	// 自更新换版后退出：批处理会等进程退掉再覆盖 exe
	srv.SetExitHook(func() { requestExit() })

	// ---------- 系统托盘 ----------
	showUI := func(hash string) {
		if win != nil {
			if hash != "" {
				win.Navigate(localURL + hash)
			}
			win.Show()
			return
		}
		if *useWeb {
			openBrowser(localURL + hash)
		}
	}

	if !*noTray {
		go func() {
			err := tray.Run(assets.Icon, tray.Actions{
				Show:     func() { showUI("") },
				Settings: func() { showUI("#settings") },
				Capture:  func() { server.FireHotkey(app, "screenshot") },
				Quit:     func() { requestExit() },
			})
			if err != nil {
				logx.Warn("tray unavailable: %v", err)
			}
		}()
	}

	// ---------- 信号 / 退出 ----------
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		logx.Info("shutdown signal received")
		if win != nil {
			win.Close()
			return
		}
		requestExit()
	}()

	if win != nil {
		// 主 goroutine 进消息循环；窗口销毁后返回
		win.Run()
		logx.Info("desktop window closed")
	} else {
		<-sigCh
	}

	// ---------- 收尾 ----------
	if mgr != nil {
		mgr.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
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
