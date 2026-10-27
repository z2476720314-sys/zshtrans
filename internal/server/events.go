package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/local/zshtrans/internal/capture"
	"github.com/local/zshtrans/internal/logx"
)

// Event 服务端推送事件（供前端 EventSource 订阅）
type Event struct {
	Kind string `json:"kind"` // ready | popup | toggle | log
	Data any    `json:"data,omitempty"`
}

// bus 极简广播总线：每个订阅者一个带缓冲通道
type bus struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func newBus() *bus { return &bus{subs: map[chan Event]struct{}{}} }

func (b *bus) subscribe() chan Event {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *bus) unsubscribe(ch chan Event) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *bus) publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // 订阅者太慢就丢弃，避免阻塞热键线程
		}
	}
}

// Publish 对外广播（热键回调等外部来源使用）
func (a *App) Publish(ev Event) { a.bus.publish(ev) }

// handleEvents GET /api/events —— SSE 推送（热键触发的弹窗/窗口动作）
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := s.app.bus.subscribe()
	defer s.app.bus.unsubscribe(ch)

	// 立即发一条 hello，让前端确认通道就绪
	// 注意：前端把它们当普通事件处理，所以必须是前端认识的 kind，
	// 否则每建立一次连接就弹一条「热键执行失败」的错误提示。
	writeSSE(w, Event{Kind: "ready"})
	flusher.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, ev)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, ev Event) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", raw)
}

// Windower 桌面窗口控制（由 main 注入 WebView2 窗口；浏览器模式下为 nil）。
// 用接口而非具体类型，避免 server 包反向依赖 deskwin。
type Windower interface {
	Show()
	Hide()
	Toggle()
	Navigate(url string)
	Available() bool
	Visible() bool
	PrepareSelection() bool
}

// SetWindower 注入桌面窗口控制器（可传 nil）
func (s *Server) SetWindower(w Windower) { s.app.setWindower(w) }

func (s *Server) SetResultHook(fn func(Result)) {
	s.app.mu.Lock()
	s.app.onResult = fn
	s.app.mu.Unlock()
}

func (a *App) showResult(result Result) {
	a.mu.RLock()
	fn := a.onResult
	a.mu.RUnlock()
	if fn != nil {
		fn(result)
	}
	a.Publish(Event{Kind: "popup", Data: result})
}

// toggleWindow 显隐主窗口：有桌面窗口就操作真窗口，否则退回 SSE 通知前端
func toggleWindow(app *App) {
	if w := app.windower(); w != nil && w.Available() {
		w.Toggle()
		return
	}
	app.Publish(Event{Kind: "toggle"})
}

// FireHotkey 全局热键回调：把动作转成 SSE 事件（无前端订阅时也照常执行翻译）
func FireHotkey(app *App, name string) {
	switch name {
	case "selection":
		go func() {
			text, err := capture.SelectedText(1200*time.Millisecond, app.Config().Hotkeys.Selection)
			if err != nil {
				app.showResult(Result{Text: "取词失败：" + err.Error(), Provider: "错误"})
				return
			}
			out, err := translatePopup(context.Background(), app, text, "")
			if err != nil {
				app.showResult(Result{Source: text, Text: "翻译失败：" + err.Error(), Provider: "错误"})
				return
			}
			app.showResult(out)
		}()
	case "screenshot":
		go func() {
			out, err := capturePopup(context.Background(), app, captureRequest{})
			if err != nil {
				logx.Warn("screenshot failed: %v", err)
				app.showResult(Result{Text: "截图取词失败：" + err.Error(), Provider: "错误"})
				return
			}
			if out.Action != "cancel" && out.Action != "complete" {
				app.showResult(out)
			}
		}()
	case "window":
		toggleWindow(app)
	}
}
