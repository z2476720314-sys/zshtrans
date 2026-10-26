package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/lang"
	"github.com/local/zshtrans/internal/logx"
	"github.com/local/zshtrans/internal/ocr"
	"github.com/local/zshtrans/internal/phrases"
	"github.com/local/zshtrans/internal/providers"
)

// Server HTTP 服务（对应 pyc WebHandler + 重构后的完整 API）
type Server struct {
	app      *App
	mux      *http.ServeMux
	asset    fs.FS
	onExit   func()                     // 进程退出回调（自更新换版后重启用），由 main 注入
	onHotkey func(config.Hotkeys) error // 热键热更新回调，由 main 注入
}

// SetExitHook 注入进程退出回调
func (s *Server) SetExitHook(fn func()) { s.onExit = fn }

// SetHotkeyHook 注入热键热更新回调（设置页保存后立刻生效，不必重启）
func (s *Server) SetHotkeyHook(fn func(config.Hotkeys) error) { s.onHotkey = fn }

// New 构建服务；asset 为前端构建产物（可为 nil）
func New(app *App, asset fs.FS) *Server {
	s := &Server{app: app, mux: http.NewServeMux(), asset: asset}
	s.routes()
	return s
}

// Handler 暴露给 http.Server
func (s *Server) Handler() http.Handler { return guardHost(withCORS(withLog(s.mux))) }

// guardHost 挡住 DNS rebinding：只接受 Host 头是本机/私有 IP 字面量或 localhost 的请求。
//
// 默认监听 0.0.0.0（原版契约，局域网要用），但没有任何鉴权。攻击者若把自己的域名
// 解析到 127.0.0.1，浏览器就会把它当同源去读 /api/config、POST /api/settings。
// 拒绝一切「域名形态」的 Host 即可断掉这条路，普通用户不受影响。
func guardHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "" && host != "localhost" {
			if ip := net.ParseIP(host); ip == nil {
				writeErr(w, http.StatusForbidden, "forbidden host")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackOnly 只允许本机发起的请求（管理类接口用，局域网页面只需 translate/status）
func loopbackOnly(w http.ResponseWriter, r *http.Request) bool {
	if !localRequest(r) {
		writeErr(w, http.StatusForbidden, "该接口只允许本机访问")
		return false
	}
	return true
}

func localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (s *Server) routes() {
	// 原 pyc 三个路由（保持兼容）
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/translate", s.handleTranslate)

	// 重构新增
	s.mux.HandleFunc("/api/config", s.adminOnly(s.handleConfig))
	// 设置/更新/原生能力属于管理面，只允许本机访问：
	// 默认监听 0.0.0.0 且无鉴权，否则同网段任何人（或恶意网页）都能改配置劫持密钥。
	s.mux.HandleFunc("/api/settings", s.adminOnly(s.handleSettings))
	s.mux.HandleFunc("/api/languages", s.handleLanguages)
	s.mux.HandleFunc("/api/presets", s.handlePresets)
	s.mux.HandleFunc("/api/providers", s.handleProviders)
	s.mux.HandleFunc("/api/models", s.adminOnly(s.handleModels))
	s.mux.HandleFunc("/api/test", s.adminOnly(s.handleTest))
	s.mux.HandleFunc("/api/phrases", s.adminOnly(s.handlePhrases))
	s.mux.HandleFunc("/api/ocr", s.adminOnly(s.handleOCR))
	s.mux.HandleFunc("/api/selfcheck", s.adminOnly(s.handleSelfCheck))
	s.mux.HandleFunc("/api/swap", s.handleSwap)
	s.mux.HandleFunc("/api/lanip", s.handleLanIP)

	// 自更新（检查 GitHub Release / 下载 / 换版重启）
	s.mux.HandleFunc("/api/update/check", s.adminOnly(s.handleUpdateCheck))
	s.mux.HandleFunc("/api/update/apply", s.adminOnly(s.handleUpdateApply))

	// 原生能力：划词翻译 / 截图取词 / 热键事件流
	s.mux.HandleFunc("/api/selection", s.adminOnly(s.handleSelection))
	s.mux.HandleFunc("/api/clipboard", s.adminOnly(s.handleClipboard))
	s.mux.HandleFunc("/api/capture", s.adminOnly(s.handleCapture))
	s.mux.HandleFunc("/api/events", s.adminOnly(s.handleEvents))

	// 静态资源与 SPA 兜底
	s.mux.HandleFunc("/", s.handleStatic)
}

// adminOnly 包装管理类接口，非本机来源直接 403
func (s *Server) adminOnly(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !loopbackOnly(w, r) {
			return
		}
		fn(w, r)
	}
}

// ---------- 中间件 ----------

// withCORS 只回显同源/本机来源的 Origin，不再无脑 `*`。
//
// 原来对任何响应都发 `Access-Control-Allow-Origin: *`，于是一个恶意网页
// （甚至 DNS rebinding 后的页面）就能跨站读 /api/config（含 masked key 与
// 全部 provider BaseURL）、POST /api/settings 把 BaseURL 改成自己的服务器劫持
// 密钥和原文、甚至 POST /api/update/apply 换掉 exe。
// 现在：非浏览器请求（无 Origin）照常；带 Origin 的只放行本机与私有网段。
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !allowedOrigin(origin, r) {
			writeErr(w, http.StatusForbidden, "该请求不是来自当前应用")
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allowedOrigin 判断 Origin 是否属于本机或私有局域网（http://127.0.0.1:8765 等）
func allowedOrigin(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
}

// withLog 静默请求日志（对应 pyc log_message 不刷屏，写文件不写控制台）
func withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t0 := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			logx.Info("%s %s %s", r.Method, r.URL.Path, time.Since(t0).Round(time.Millisecond))
		}
	})
}

// ---------- 工具 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// maxBody 请求体上限。translate/ocr 共用一个预算，但 OCR 的图片另有限制（见 handleOCR）。
const maxBody = 24 << 20

func readBody(r *http.Request, out any) error {
	return readBodyLimit(r, out, maxBody)
}

// readBodyLimit 读 JSON 请求体；超限时报明确错误，而不是让 io.LimitReader
// 静默截断后抛 JSON 解析失败（用户会看到莫名其妙的 unexpected end of JSON input）。
func readBodyLimit(r *http.Request, out any, limit int64) error {
	if r.ContentLength > limit {
		return fmt.Errorf("请求体过大（%d 字节，上限 %d）", r.ContentLength, limit)
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > limit {
		return fmt.Errorf("请求体过大（上限 %d 字节）", limit)
	}
	if len(raw) == 0 {
		return errors.New("empty body")
	}
	return json.Unmarshal(raw, out)
}

func method(w http.ResponseWriter, r *http.Request, allow ...string) bool {
	for _, m := range allow {
		if r.Method == m {
			return true
		}
	}
	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// ---------- 原 pyc 路由 ----------

// handleStatus GET /api/status
// 原返回 {providers:[{name,healthy}]}；重构后额外带 label/version/lan。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	cfg := s.app.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"app":          config.AppName,
		"version":      config.AppVersion,
		"providers":    s.app.Engine().Statuses(),
		"src":          cfg.SrcLang,
		"tgt":          cfg.TgtLang,
		"default":      cfg.DefaultProvider,
		"cache":        s.app.Engine().Len(),
		"desktop":      s.app.windower() != nil,
		"update_check": cfg.UpdateCheck,
		"local":        localRequest(r),
	})
}

// translateRequest POST /api/translate 请求体
type translateRequest struct {
	Text string `json:"text"`
	Src  string `json:"src"`
	Tgt  string `json:"tgt"`
}

// handleTranslate POST /api/translate
// 原实现：空文本→400 empty text；语言白名单外→400 bad lang；
// 返回 {text,provider,ms,src}。
func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req translateRequest
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeErr(w, http.StatusBadRequest, "empty text")
		return
	}
	src := req.Src
	if src == "" {
		src = "auto"
	}
	tgt := req.Tgt
	if tgt == "" {
		tgt = "zh"
	}
	if !lang.WebSrcAllowed[src] || !lang.WebTgtAllowed[tgt] {
		writeErr(w, http.StatusBadRequest, "bad lang")
		return
	}
	src, tgt = resolveDirection(text, src, tgt)
	res, err := s.app.Engine().TranslateContext(r.Context(), text, src, tgt)
	if err != nil {
		logx.Err("translate fail: %v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"text":     res.Text,
		"provider": res.Provider,
		"ms":       res.MS,
		"src":      src,
		"tgt":      tgt,
	})
}

// ---------- 配置 ----------

// maskedKeys 密钥掩码：已设置显示「已保存（不显示）」，未设置显示「未设置」
func maskedKeys(k config.APIKeys) map[string]string {
	values := k.Values()
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			values[name] = ""
		} else {
			values[name] = KeySaved
		}
	}
	return values
}

// KeySaved 已保存密钥占位（translate.py KEY_SAVED）
const KeySaved = "已保存（不显示）"

// KeyMissing 未设置占位（translate.py KEY_MISSING）
const KeyMissing = "未设置"

// handleConfig GET /api/config —— 返回配置（密钥掩码）
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	cfg := s.app.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"app":              config.AppName,
		"version":          config.AppVersion,
		"src_lang":         cfg.SrcLang,
		"tgt_lang":         cfg.TgtLang,
		"default_provider": cfg.DefaultProvider,
		"ocr_model":        cfg.OCRModel,
		"hotkeys":          cfg.Hotkeys,
		"providers":        cfg.Providers,
		"api_keys":         maskedKeys(cfg.APIKeys),
		"key_saved":        KeySaved,
		"key_missing":      KeyMissing,
		"ocr_unset":        ocrUnset,
		"update_check":     cfg.UpdateCheck,
		"update_repo":      cfg.UpdateRepo,
		"update_skipped":   cfg.UpdateSkipped,
	})
}

// settingsRequest POST /api/settings 请求体（密钥留空表示保持原值）
type settingsRequest struct {
	SrcLang string `json:"src_lang"`
	TgtLang string `json:"tgt_lang"`
	// DefaultProvider / OCRModel 用指针：nil = 本次不涉及（自检页局部保存不能把它们清空），
	// 显式传空串才是用户真的要「自动路由 / 不使用视觉模型」。
	DefaultProvider *string           `json:"default_provider"`
	OCRModel        *string           `json:"ocr_model"`
	Hotkeys         *config.Hotkeys   `json:"hotkeys"`
	Providers       []config.Provider `json:"providers"`
	APIKeys         map[string]string `json:"api_keys"`
	UpdateCheck     *bool             `json:"update_check"`
	UpdateRepo      *string           `json:"update_repo"`
	UpdateSkipped   *string           `json:"update_skipped"`
}

// handleSettings POST /api/settings —— 校验 + 保存 + 热重载（对应 SettingsWindow.commit）
//
// 读-改-写整段放在 app.Mutate 里（applyMu 串行化）：原来无锁读 cfg、改完再整份写回，
// 两个并发保存会把对方刚写的字段覆盖掉。
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req settingsRequest
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// 重复 ID 必须在归一化之前判定：NormalizeProviders 会去重，
	// 若放到 validateSettings 里检查将永远不会触发（对应 pyc candidate() 的"提供方 ID 重复"）。
	if req.Providers != nil {
		if dup := duplicateProviderID(req.Providers); dup != "" {
			writeErr(w, http.StatusBadRequest, "提供方 ID 重复："+dup)
			return
		}
	}

	var saved config.Config
	err := s.app.Mutate(func(c *config.Config) error {
		if req.SrcLang != "" {
			c.SrcLang = req.SrcLang
		}
		if req.TgtLang != "" {
			c.TgtLang = req.TgtLang
		}
		if c.SrcLang == c.TgtLang {
			c.SrcLang = "auto"
		}
		if req.DefaultProvider != nil {
			c.DefaultProvider = strings.TrimSpace(*req.DefaultProvider)
		}
		if req.OCRModel != nil {
			c.OCRModel = strings.TrimSpace(*req.OCRModel)
		}
		if req.Hotkeys != nil {
			c.Hotkeys = *req.Hotkeys
		}
		if req.UpdateCheck != nil {
			c.UpdateCheck = *req.UpdateCheck
		}
		if req.UpdateRepo != nil {
			c.UpdateRepo = strings.TrimSpace(*req.UpdateRepo)
		}
		if req.UpdateSkipped != nil {
			c.UpdateSkipped = strings.TrimSpace(*req.UpdateSkipped)
		}
		if req.Providers != nil {
			c.Providers = providers.NormalizeProviders(req.Providers)
		}
		// 密钥：留空或占位字符串保留原值
		for name, incoming := range req.APIKeys {
			v := strings.TrimSpace(incoming)
			if v == "" || v == KeySaved || v == KeyMissing {
				continue
			}
			c.APIKeys.Set(name, v)
		}
		if msg := validateSettings(*c); msg != "" {
			return errors.New(msg)
		}
		saved = *c
		return nil
	})
	if err != nil {
		// 校验失败 → 400；写盘失败 → 500
		code := http.StatusBadRequest
		msg := err.Error()
		if strings.HasPrefix(msg, "保存设置失败") {
			code = http.StatusInternalServerError
		}
		writeErr(w, code, msg)
		return
	}

	// 热键热更新（改了键位不必重启）
	if req.Hotkeys != nil && s.onHotkey != nil {
		if hErr := s.onHotkey(saved.Hotkeys); hErr != nil {
			logx.Warn("hotkey rebind fail: %v", hErr)
			usable := len(providers.UsableProviders(saved.Providers, saved.APIKeys, envOrNil()))
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":           true,
				"message":      "设置已保存，但热键未生效：" + hErr.Error(),
				"usable":       usable,
				"hotkey_error": hErr.Error(),
			})
			return
		}
	}

	usable := len(providers.UsableProviders(saved.Providers, saved.APIKeys, envOrNil()))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "设置已保存并生效（" + itoa(usable) + " 个模型可用）",
		"usable":  usable,
	})
}

// duplicateProviderID 返回第一个重复的提供方 ID，无重复返回空串
func duplicateProviderID(list []config.Provider) string {
	seen := map[string]bool{}
	for _, p := range list {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		if seen[id] {
			return id
		}
		seen[id] = true
	}
	return ""
}

// validateSettings 保存前校验（对应 translate.py:2394 candidate()）
func validateSettings(cfg config.Config) string {
	if len(cfg.Providers) == 0 {
		return "至少需要一个模型提供方"
	}
	// 默认模型必须可用
	if cfg.DefaultProvider != "" {
		if !engineUsable(cfg, cfg.DefaultProvider, false) {
			return "默认模型 " + cfg.DefaultProvider + " 当前不可用（模型缺失、提供方被禁用或密钥缺失）"
		}
	}
	// OCR 模型必须是启用且支持图片的模型
	if cfg.OCRModel != "" && cfg.OCRModel != ocrUnset {
		if !engineUsable(cfg, cfg.OCRModel, true) {
			return "所选截图 OCR 模型当前不可用（未支持图片、提供方被禁用或密钥缺失）"
		}
	}
	// 热键校验
	if msg := validateHotkeys(cfg.Hotkeys); msg != "" {
		return msg
	}
	return ""
}

// engineUsable 指定 provider:model 是否可用。
// name 不含冒号时按「直连提供方名」（google / mymemory / deepl …）判定：
// 原 pyc 的 default_provider 同样可以填直连名，这条不能被误判为不可用。
func engineUsable(cfg config.Config, name string, requireImage bool) bool {
	providerID, modelID, ok := splitEngine(name)
	if !ok {
		if requireImage {
			return false // 直连提供方不支持图片输入
		}
		// providers.Labels 的键才是提供方名（值是中文显示名）
		if _, isDirect := providers.Labels[name]; !isDirect {
			return false
		}
		// 直连提供方也要查密钥：Enabled() 缺密钥就把它过滤掉，
		// 不查的话能存下「默认=qwen-max 但没配 qwen key」这种必然失败的配置。
		keys := keysAdapter{keys: cfg.APIKeys}
		for _, k := range providers.DirectNeedKeys(name) {
			if !keys.Has(k) {
				return false
			}
		}
		return true
	}
	for _, p := range cfg.Providers {
		if p.ID != providerID || !p.Enabled {
			continue
		}
		if providers.ResolveKey(p, cfg.APIKeys, envOrNil()) == "" {
			return false
		}
		for _, m := range p.Models {
			if m.ID != modelID {
				continue
			}
			if requireImage && !m.Image {
				return false
			}
			return true
		}
	}
	return false
}

// ---------- 语言 / 预设 / 提供方 ----------

func (s *Server) handleLanguages(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"all":    lang.AllOptions(),
		"target": lang.TargetOptions(),
		"flip":   lang.Flip,
	})
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"groups":  providers.Groups,
		"presets": providers.Presets(),
		"seed":    providers.Seed(),
		"protocols": map[string]any{
			"values": providers.Protocols,
			"labels": providers.ProtocolLabels,
		},
		"reasoning": map[string]any{
			"values": providers.ReasoningLevels,
			"labels": providers.ReasoningLabels,
		},
		"thinking": map[string]any{
			"params": providers.ThinkingParams,
			"styles": providers.ThinkingStyles,
			"labels": providers.StyleLabels,
		},
	})
}

// handleProviders GET /api/providers —— 9 个直连提供方 + 卡片状态
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	direct := make([]map[string]any, 0, len(providers.Labels))
	for _, name := range []string{"qwen-max", "qwen-max-2606", "google", "mymemory", "deepl", "baidu", "niutrans", "lingva", "agnes"} {
		direct = append(direct, map[string]any{"name": name, "label": providers.Label(name)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"direct": direct})
}

// modelsRequest POST /api/models —— 拉取某提供方账号可用模型
type modelsRequest struct {
	ProviderID string `json:"provider_id"`
	Key        string `json:"key"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req modelsRequest
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := s.app.Config()
	for _, p := range cfg.Providers {
		if p.ID != req.ProviderID {
			continue
		}
		key := strings.TrimSpace(req.Key)
		if key == "" || key == KeySaved {
			key = providers.ResolveKey(p, cfg.APIKeys, envOrNil())
		}
		if key == "" {
			writeErr(w, http.StatusBadRequest, "密钥未设置")
			return
		}
		list, err := providers.FetchModelConfigs(p, key)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": list})
		return
	}
	writeErr(w, http.StatusNotFound, "provider not found")
}

// testRequest POST /api/test —— 连通性测试
type testRequest struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	Key        string `json:"key"`
}

func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req testRequest
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := s.app.Config()
	for _, p := range cfg.Providers {
		if p.ID != req.ProviderID {
			continue
		}
		key := strings.TrimSpace(req.Key)
		if key == "" || key == KeySaved {
			key = providers.ResolveKey(p, cfg.APIKeys, envOrNil())
		}
		for _, m := range p.Models {
			if req.ModelID != "" && m.ID != req.ModelID {
				continue
			}
			t0 := time.Now()
			if err := providers.TestConnection(p, m, key); err != nil {
				writeJSON(w, http.StatusOK, map[string]any{
					"ok":    false,
					"error": err.Error(),
					"ms":    time.Since(t0).Milliseconds(),
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true,
				"ms": time.Since(t0).Milliseconds(),
			})
			return
		}
	}
	writeErr(w, http.StatusNotFound, "model not found")
}

// ---------- 常用语 ----------

// phraseRequest 常用语写操作
type phraseRequest struct {
	Action  string `json:"action"` // list | add | update | delete
	ID      string `json:"id"`
	Desc    string `json:"desc"`
	Content string `json:"content"`
}

func (s *Server) handlePhrases(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"items": phrases.List()})
		return
	}
	var req phraseRequest
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch req.Action {
	case "", "list":
		writeJSON(w, http.StatusOK, map[string]any{"items": phrases.List()})
	case "add":
		it, err := phrases.Add(req.Desc, req.Content)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "item": it, "items": phrases.List()})
	case "update":
		ok, err := phrases.Update(req.ID, req.Desc, req.Content)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "items": phrases.List()})
	case "delete":
		ok, err := phrases.Delete(req.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "items": phrases.List()})
	default:
		writeErr(w, http.StatusBadRequest, "unknown action")
	}
}

// ---------- OCR ----------

// maxOCRPixels 单次 OCR 允许的最大像素数（约 4000×4000）
const maxOCRPixels = 16_000_000

// ocrRequest POST /api/ocr —— 图片 base64 → 文字
type ocrRequest struct {
	Image string `json:"image"`
}

func (s *Server) handleOCR(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req ocrRequest
	if err := readBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	b64 := strings.TrimSpace(req.Image)
	if idx := strings.Index(b64, "base64,"); idx >= 0 {
		b64 = b64[idx+len("base64,"):]
	}
	if b64 == "" {
		writeErr(w, http.StatusBadRequest, "empty image")
		return
	}
	raw, err := decodeB64(b64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad image: "+err.Error())
		return
	}
	img, err := ocr.Decode(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad image: "+err.Error())
		return
	}
	// 像素上限：一张 10000×10000 的 PNG 可以压到几百 KB，解码后却是 400MB RGBA，
	// 后面还要再乘候选数（+Gray +Otsu），不拦住就是内存放大炸弹。
	if wpx, hpx := img.Bounds().Dx(), img.Bounds().Dy(); wpx*hpx > maxOCRPixels {
		writeErr(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("图片过大（%d×%d，上限 %d 万像素）", wpx, hpx, maxOCRPixels/10000))
		return
	}
	cfg := s.app.Config()
	res, err := ocr.RunContext(r.Context(), img, s.app.ocrKeys(cfg))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": res.Text, "engine": res.Engine})
}

// ---------- 自检 / 交换 / 局域网 ----------

func (s *Server) handleSelfCheck(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	cfg := s.app.Config()
	list := phrases.List()
	// phrases_sha256：与原 pyc self_check 一致，对常用语内容做稳定性指纹
	h := sha256.New()
	for _, it := range list {
		io.WriteString(h, it.ID)
		io.WriteString(h, "\x00")
		io.WriteString(h, it.Content)
		io.WriteString(h, "\x00")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app":                   config.AppName,
		"version":               config.AppVersion,
		"language_count":        len(lang.Order),
		"provider_preset_count": len(providers.Presets()),
		"direct_provider_count": len(providers.Labels),
		"ui_toolkit":            "React + Ant Design",
		"phrases_count":         len(list),
		"phrases_sha256":        hex.EncodeToString(h.Sum(nil)),
		"font_glyphs_ok":        nil,
		"providers":             len(cfg.Providers),
		"usable":                len(providers.UsableProviders(cfg.Providers, cfg.APIKeys, envOrNil())),
		"cache":                 s.app.Engine().Len(),
		"config_path":           config.Path(),
		"phrases_path":          config.PhrasesPath(),
		"log_path":              config.LogPath(),
		"ok":                    true,
	})
}

// handleSwap GET /api/swap?from=zh&to=ru&detected=ru —— 语向交换（对应 translate.py _swap_lang）
func (s *Server) handleSwap(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	from := q.Get("from")
	if from == "" {
		from = "auto"
	}
	to := q.Get("to")
	if to == "" {
		to = "zh"
	}
	src, tgt := lang.Swap(from, to, q.Get("detected"))
	writeJSON(w, http.StatusOK, map[string]any{"from": src, "to": tgt})
}

func (s *Server) handleLanIP(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ip": LanIP()})
}

// ---------- 静态资源 ----------

// handleStatic 前端静态资源 + SPA 兜底（替代原 EMBEDDED_HTML）
// 未命中的 /api/ 路径仍返回 404 JSON（对齐 pyc do_GET 的 {"error":"not found"}）。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.asset == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(s.asset, path); err != nil {
		// SPA 路由回退到 index.html
		path = "index.html"
		if _, err := fs.Stat(s.asset, path); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
	}
	if path == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(path, "assets/") {
		// vite 产物文件名带内容 hash，可长缓存；不打这个头的话每次刷新都重新拉 1MB JS
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeFileFS(w, r, s.asset, path)
}

// LanIP 局域网 IP 探测（对应 pyc lan_ip）
func LanIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return "127.0.0.1"
	}
	return addr.IP.String()
}
