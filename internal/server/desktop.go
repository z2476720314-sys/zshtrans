package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"github.com/local/zshtrans/internal/capture"
)

type DesktopResponse struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// DesktopBridge runs the existing API handlers in-process, without a listener.
type DesktopBridge struct {
	handler http.Handler
	respond func(DesktopResponse)
	token   string
	ctx     context.Context
	close   context.CancelFunc
	mu      sync.Mutex
	pending map[string]context.CancelFunc
}

func NewDesktopBridge(handler http.Handler, respond func(DesktopResponse)) *DesktopBridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &DesktopBridge{handler: handler, respond: respond, token: rand.Text(), ctx: ctx, close: cancel, pending: make(map[string]context.CancelFunc)}
}

func (b *DesktopBridge) Token() string { return b.token }
func (b *DesktopBridge) Close()        { b.close() }

func (b *DesktopBridge) Request(token, id, method, path, body string) error {
	if subtle.ConstantTimeCompare([]byte(token), []byte(b.token)) != 1 {
		return errors.New("无效的桌面调用")
	}
	u, err := url.ParseRequestURI(path)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/api/") || u.Path == "/api/events" || len(body) > maxBody || id == "" || len(id) > 64 || (method != "GET" && method != "POST") {
		return errors.New("无效的桌面请求")
	}
	ctx, cancel := context.WithCancel(b.ctx)
	b.mu.Lock()
	if _, exists := b.pending[id]; exists || b.ctx.Err() != nil {
		b.mu.Unlock()
		cancel()
		return errors.New("桌面请求已关闭或重复")
	}
	b.pending[id] = cancel
	b.mu.Unlock()
	// Bind callbacks run on the window thread; network and capture work must leave it.
	go func() {
		defer func() {
			cancel()
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
		}()
		r, _ := http.NewRequestWithContext(ctx, method, "http://localhost"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:0"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		b.handler.ServeHTTP(w, r)
		if b.ctx.Err() == nil {
			b.respond(DesktopResponse{ID: id, Status: w.Code, Body: w.Body.String()})
		}
	}()
	return nil
}

func (b *DesktopBridge) Cancel(token, id string) error {
	if subtle.ConstantTimeCompare([]byte(token), []byte(b.token)) != 1 {
		return errors.New("无效的桌面调用")
	}
	b.mu.Lock()
	cancel := b.pending[id]
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *Server) DesktopEvents(ctx context.Context, emit func(Event)) {
	ch := s.app.bus.subscribe()
	defer s.app.bus.unsubscribe(ch)
	emit(Event{Kind: "ready"})
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			emit(ev)
		}
	}
}

func (s *Server) handleClipboard(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := capture.WriteClipboard(req.Text); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
