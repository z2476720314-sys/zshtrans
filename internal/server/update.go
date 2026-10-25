package server

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/logx"
	"github.com/local/zshtrans/internal/update"
)

// updateState 更新流程的内存状态（下载进度 / 已下载路径）
type updateState struct {
	mu         sync.Mutex
	rel        *update.Release
	path       string
	done       int64
	total      int64
	busy       bool
	lastErr    string
	cancel     context.CancelFunc
	installing bool
}

var upd updateState

// handleUpdateCheck GET /api/update/check —— 查最新版本
// 查询参数 force=1 时忽略「跳过此版本」，供用户手动点「检查更新」。
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	cfg := s.app.Config()
	repo := cfg.UpdateRepo
	if repo == "" {
		repo = update.Repo
	}
	rel, err := update.CheckRepo(repo, config.AppVersion)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"current": config.AppVersion,
		})
		return
	}
	if rel == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"current": config.AppVersion,
			"latest":  "",
			"has_new": false,
		})
		return
	}
	force := r.URL.Query().Get("force") == "1"
	skipped := cfg.UpdateSkipped == rel.Version
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"current": config.AppVersion,
		"latest":  rel.Version,
		"tag":     rel.Tag,
		"name":    rel.Name,
		"notes":   rel.Body,
		"page":    rel.HTMLURL,
		"size":    rel.Size,
		"has_new": true,
		"skipped": skipped && !force,
	})
}

// handleUpdateApply POST /api/update/apply —— 下载并换版重启
// body: {"action":"download"} 只下载；{"action":"install"} 下载完成后覆盖并重启。
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Action string `json:"action"`
		Skip   string `json:"skip"` // 非空时把该版本写入「跳过」
	}
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// 记录「跳过此版本」
	if req.Skip != "" {
		if err := s.app.Mutate(func(cfg *config.Config) error { cfg.UpdateSkipped = req.Skip; return nil }); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": req.Skip})
		return
	}

	// status 是只读轮询，必须能在下载进行中调用；否则前端 700ms 轮询每次都被 409
	// 挡掉，进度条不动、下载完也不会自动 install。
	if req.Action == "status" {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"busy":       upd.busy,
			"done":       upd.done,
			"total":      upd.total,
			"ready":      upd.path != "" && !upd.busy,
			"installing": upd.installing,
			"error":      upd.lastErr,
		})
		return
	}
	if req.Action == "cancel" {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		if upd.installing {
			writeErr(w, http.StatusConflict, "安装已开始，请等待重启")
			return
		}
		if upd.cancel != nil {
			upd.cancel()
		}
		upd.path = ""
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": "cancelled"})
		return
	}
	if req.Action != "" && req.Action != "download" && req.Action != "install" {
		writeErr(w, http.StatusBadRequest, "未知动作")
		return
	}
	cfg := s.app.Config()
	repo := cfg.UpdateRepo
	if repo == "" {
		repo = update.Repo
	}

	upd.mu.Lock()
	if upd.busy {
		upd.mu.Unlock()
		writeErr(w, http.StatusConflict, "已有更新任务在进行")
		return
	}
	rel, path := upd.rel, upd.path
	upd.mu.Unlock()

	// 需要下载的动作先原子占位再走网络：原来「查 busy → 释放锁 → 请求网络 →
	// 置 busy」中间有窗口，并发两次点击会各下一份、互相覆盖进度。
	wantDownload := req.Action == "download" || (req.Action == "" || req.Action == "install") && path == ""
	if wantDownload {
		upd.mu.Lock()
		if upd.busy {
			upd.mu.Unlock()
			writeErr(w, http.StatusConflict, "已有更新任务在进行")
			return
		}
		upd.busy = true
		ctx, cancel := context.WithCancel(context.Background())
		upd.cancel = cancel
		upd.path = ""
		upd.done, upd.total, upd.lastErr = 0, 0, ""
		upd.mu.Unlock()
		release := func() {
			upd.mu.Lock()
			upd.busy = false
			upd.cancel = nil
			upd.mu.Unlock()
			cancel()
		}
		// 下面任何提前 return 都要释放占位
		fail := func(code int, msg string) {
			release()
			writeErr(w, code, msg)
		}
		{
			got, err := update.CheckRepo(repo, config.AppVersion)
			if err != nil {
				fail(http.StatusBadGateway, err.Error())
				return
			}
			if got == nil {
				fail(http.StatusBadRequest, "已是最新版本")
				return
			}
			rel = got
			upd.mu.Lock()
			upd.rel = got
			upd.mu.Unlock()
		}
		if ctx.Err() != nil {
			fail(http.StatusConflict, "下载已取消")
			return
		}
		go downloadAsync(ctx, rel)
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "state": "downloading"})
		return
	}

	// 没有缓存版本信息就先查一次
	if rel == nil {
		got, err := update.CheckRepo(repo, config.AppVersion)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		if got == nil {
			writeErr(w, http.StatusBadRequest, "已是最新版本")
			return
		}
		rel = got
		upd.mu.Lock()
		upd.rel = got
		upd.mu.Unlock()
	}

	switch req.Action {
	case "", "install":
		if _, err := os.Stat(path); err != nil {
			writeErr(w, http.StatusBadRequest, "安装包已失效，请重新下载")
			return
		}
		upd.mu.Lock()
		if upd.busy || upd.path != path {
			upd.mu.Unlock()
			writeErr(w, http.StatusConflict, "已有更新任务在进行")
			return
		}
		upd.busy, upd.installing = true, true
		upd.mu.Unlock()
		failInstall := func(err error) {
			upd.mu.Lock()
			upd.busy, upd.installing, upd.lastErr = false, false, err.Error()
			upd.mu.Unlock()
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		if err := update.VerifyRelease(rel, path); err != nil {
			failInstall(err)
			return
		}
		if err := update.Apply(path); err != nil {
			failInstall(err)
			return
		}
		logx.Info("update applied: %s -> %s, restarting", config.AppVersion, rel.Version)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": "restarting"})
		// 给 HTTP 响应一点时间写出去，然后让进程退出，批处理接手换 exe
		go func() {
			// 用 server 侧的统一退出通道
			if s.onExit != nil {
				time.Sleep(400 * time.Millisecond)
				s.onExit()
			}
		}()
		return
	case "download":
		// 已在上面统一处理（含原子占位），走不到这里
		writeErr(w, http.StatusInternalServerError, "unreachable")
		return
	case "status":
		// 已在函数开头统一处理（下载进行中也要能读进度）
		writeErr(w, http.StatusInternalServerError, "unreachable")
		return
	}
	writeErr(w, http.StatusBadRequest, "未知动作")
}

func downloadAsync(ctx context.Context, rel *update.Release) {
	path := update.TempPath(rel.Version)
	// 优先 api.github.com 的资源端点（国内可达），失败再退回 github.com 直链
	err := update.DownloadRelease(ctx, rel, path, func(done, total int64) {
		upd.mu.Lock()
		upd.done, upd.total = done, total
		upd.mu.Unlock()
	})
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
		_ = os.Remove(path)
	}
	upd.busy = false
	if upd.cancel != nil {
		upd.cancel()
		upd.cancel = nil
	}
	if err != nil {
		upd.lastErr = err.Error()
		upd.path = ""
		logx.Warn("update download fail: %v", err)
		return
	}
	upd.path = path
	logx.Info("update downloaded: %s -> %s", rel.Version, path)
}
