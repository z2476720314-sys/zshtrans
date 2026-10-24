package server

import (
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
	mu      sync.Mutex
	rel     *update.Release
	path    string
	done    int64
	total   int64
	busy    bool
	lastErr string
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
		cfg := s.app.Config()
		cfg.UpdateSkipped = req.Skip
		if err := s.app.Apply(cfg); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": req.Skip})
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
		// install：确保已下载
		if path == "" {
			upd.mu.Lock()
			upd.busy = true
			upd.done, upd.total, upd.lastErr = 0, 0, ""
			upd.mu.Unlock()
			go downloadAsync(rel)
			writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "state": "downloading"})
			return
		}
		if _, err := os.Stat(path); err != nil {
			writeErr(w, http.StatusBadRequest, "安装包已失效，请重新下载")
			return
		}
		if err := update.Apply(path); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
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
		upd.mu.Lock()
		upd.busy = true
		upd.done, upd.total, upd.lastErr = 0, 0, ""
		upd.mu.Unlock()
		go downloadAsync(rel)
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "state": "downloading"})
		return
	case "status":
		upd.mu.Lock()
		defer upd.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"busy":  upd.busy,
			"done":  upd.done,
			"total": upd.total,
			"ready": upd.path != "",
			"error": upd.lastErr,
		})
		return
	}
	writeErr(w, http.StatusBadRequest, "未知动作")
}

func downloadAsync(rel *update.Release) {
	path := update.TempPath(rel.Version)
	err := update.Download(rel.URL, path, func(done, total int64) {
		upd.mu.Lock()
		upd.done, upd.total = done, total
		upd.mu.Unlock()
	})
	upd.mu.Lock()
	defer upd.mu.Unlock()
	upd.busy = false
	if err != nil {
		upd.lastErr = err.Error()
		upd.path = ""
		logx.Warn("update download fail: %v", err)
		return
	}
	upd.path = path
	logx.Info("update downloaded: %s -> %s", rel.Version, path)
}
