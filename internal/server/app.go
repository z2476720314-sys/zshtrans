package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/engine"
	"github.com/local/zshtrans/internal/lang"
	"github.com/local/zshtrans/internal/logx"
	"github.com/local/zshtrans/internal/ocr"
	"github.com/local/zshtrans/internal/providers"
)

// keysAdapter 把 config.APIKeys 适配成 providers.Keys
type keysAdapter struct{ keys config.APIKeys }

func (k keysAdapter) DeepL() string      { return k.keys.DeepL }
func (k keysAdapter) BaiduAppID() string { return k.keys.BaiduAppID }
func (k keysAdapter) BaiduKey() string   { return k.keys.BaiduKey }
func (k keysAdapter) NiuTrans() string   { return k.keys.NiuTrans }
func (k keysAdapter) Agnes() string      { return k.keys.Agnes }
func (k keysAdapter) Qwen() string       { return k.keys.Qwen }
func (k keysAdapter) OCRSpace() string   { return k.keys.OCRSpace }
func (k keysAdapter) BaiduOCRAK() string { return k.keys.BaiduOCRAK }
func (k keysAdapter) BaiduOCRsk() string { return k.keys.BaiduOCRsk }

// Has 对应 pyc CFG['api_keys'].get(k) 的真值判断
func (k keysAdapter) Has(key string) bool {
	switch key {
	case "deepl":
		return strings.TrimSpace(k.keys.DeepL) != ""
	case "baidu_appid":
		return strings.TrimSpace(k.keys.BaiduAppID) != ""
	case "baidu_key":
		return strings.TrimSpace(k.keys.BaiduKey) != ""
	case "niutrans":
		return strings.TrimSpace(k.keys.NiuTrans) != ""
	case "agnes":
		return strings.TrimSpace(k.keys.Agnes) != ""
	case "qwen":
		return strings.TrimSpace(k.keys.Qwen) != ""
	case "acctoken":
		return strings.TrimSpace(k.keys.Acctoken) != ""
	case "ocrspace":
		return strings.TrimSpace(k.keys.OCRSpace) != ""
	case "baidu_ocr_ak":
		return strings.TrimSpace(k.keys.BaiduOCRAK) != ""
	case "baidu_ocr_sk":
		return strings.TrimSpace(k.keys.BaiduOCRsk) != ""
	}
	return false
}

// App 运行时聚合：配置 + 引擎 + 密钥 + 桌面窗口
type App struct {
	mu       sync.RWMutex
	store    *config.Store
	eng      *engine.Engine
	bus      *bus
	win      Windower
	onResult func(Result)
	// applyMu 串行化「读配置 → 改 → 落盘 → 重建引擎」整段。
	// 原来 /api/settings 全程无锁读-改-写，两个并发保存会互相覆盖对方的字段。
	applyMu sync.Mutex
}

// setWindower 注入桌面窗口控制（main 在 WebView2 就绪后调用）
func (a *App) setWindower(w Windower) {
	a.mu.Lock()
	a.win = w
	a.mu.Unlock()
}

// windower 读当前窗口控制（可能为 nil）
func (a *App) windower() Windower {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.win
}

// NewApp 载入配置并构建引擎
func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		// 首次运行没有配置文件是正常情况，不该打成告警——否则按 README 排障时
		// 会把「第一次启动」误读成出错。只有文件存在但读/解析失败才值得 warn。
		if _, statErr := os.Stat(config.Path()); os.IsNotExist(statErr) {
			logx.Info("first run, seeding default config")
		} else {
			logx.Warn("config read fail, use defaults: %v", err)
		}
		cfg = config.Default()
		// 对应 pyc `_seed_bundled`：首次运行落盘默认配置；文件已存在（含损坏）时不覆盖
		if _, statErr := os.Stat(config.Path()); os.IsNotExist(statErr) {
			if saveErr := config.Save(cfg); saveErr != nil {
				logx.Warn("seed config fail: %v", saveErr)
			}
		}
	}
	if len(cfg.Providers) == 0 {
		cfg.Providers = providers.Seed()
	}
	cfg.Providers = providers.NormalizeProviders(cfg.Providers)
	app := &App{store: config.NewStore(cfg), bus: newBus()}
	app.rebuild()
	return app
}

// Config 当前配置快照
func (a *App) Config() config.Config { return a.store.Get() }

// Engine 引擎引用
func (a *App) Engine() *engine.Engine {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.eng
}

// rebuild 依据配置重建引擎（对应 translate.py SettingsWindow.commit → rebuild）
func (a *App) rebuild() {
	cfg := a.store.Get()
	list := buildPool(cfg)
	timeout := config.Timeout
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.eng == nil {
		a.eng = engine.New(list, cfg.DefaultProvider, timeout)
		return
	}
	a.eng.Rebuild(list, cfg.DefaultProvider, timeout)
	logx.Info("engine rebuilt: %d provider(s), default=%q", len(list), cfg.DefaultProvider)
}

// Apply 保存配置 → 落盘 → 重建引擎（对应 commit）
func (a *App) Apply(cfg config.Config) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	return a.applyLocked(cfg)
}

// Mutate 在锁内完成「读当前配置 → 由 fn 修改 → 校验 → 落盘 → 重建引擎」。
// fn 返回错误时什么都不写，调用方拿到原始错误（用于把校验失败转成 400）。
func (a *App) Mutate(fn func(cfg *config.Config) error) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	cfg := a.store.Get()
	if err := fn(&cfg); err != nil {
		return err
	}
	return a.applyLocked(cfg)
}

func (a *App) applyLocked(cfg config.Config) error {
	if err := config.Save(cfg); err != nil {
		return err
	}
	a.store.Set(cfg)
	ocr.ResetBaiduToken()
	ocr.ResetCache()
	a.rebuild()
	return nil
}

// buildPool 构建引擎池：9 个直连提供方 + 配置卡片里的模型提供方
// 对应 pyc：ENGINE 初始化时就含 9 个直连节点，install_model_providers 再向后追加模型节点，
// 二者共存，由 enabled()/健康度共同决定实际参与顺序。
func buildPool(cfg config.Config) []*providers.Provider {
	keys := keysAdapter{keys: cfg.APIKeys}
	environ := os.Getenv
	pool := providers.BuildDirect(keys)
	for _, p := range cfg.Providers {
		if !p.Enabled || len(p.Models) == 0 {
			continue
		}
		key := providers.ResolveKey(p, cfg.APIKeys, environ)
		if key == "" {
			continue
		}
		for _, m := range p.Models {
			p, m := p, m
			name := providers.EngineName(p, m)
			credential := key
			pool = append(pool, &providers.Provider{
				Name: name,
				Fn: func(ctx context.Context, text, src, tgt string) (string, error) {
					return providers.TranslateWithContext(ctx, p, m, credential, text, src, tgt)
				},
			})
		}
	}
	return pool
}

// ocrKeys 组装 OCR 密钥（对应 run_ocr 的引擎集合）
func (a *App) ocrKeys(cfg config.Config) ocr.Keys {
	out := ocr.Keys{
		Space:   cfg.APIKeys.OCRSpace,
		BaiduAK: cfg.APIKeys.BaiduOCRAK,
		BaiduSK: cfg.APIKeys.BaiduOCRsk,
		SrcLang: cfg.SrcLang,
	}
	// 视觉模型：ocr_model = "provider:model"
	tag := strings.TrimSpace(cfg.OCRModel)
	if tag == "" || tag == ocrUnset {
		return out
	}
	providerID, modelID, ok := splitEngine(tag)
	if !ok {
		return out
	}
	for _, p := range cfg.Providers {
		if p.ID != providerID || !p.Enabled {
			continue
		}
		key := providers.ResolveKey(p, cfg.APIKeys, os.Getenv)
		if key == "" {
			continue
		}
		for _, m := range p.Models {
			if m.ID != modelID || !m.Image {
				continue
			}
			p, m, credential := p, m, key
			out.VisionContext = func(ctx context.Context, b64 string) (string, error) {
				return providers.VisionOCRContext(ctx, p, m, credential, b64)
			}
			out.VisionTag = tag
			scope, _ := json.Marshal(struct {
				Provider config.Provider
				Model    config.Model
				Key      string
			}{p, m, credential})
			digest := sha256.Sum256(scope)
			out.VisionScope = hex.EncodeToString(digest[:])
			return out
		}
	}
	return out
}

// ocrUnset 「不使用视觉模型」哨兵值（translate.py OCR_UNSET）
const ocrUnset = "不使用视觉模型"

func splitEngine(name string) (string, string, bool) {
	idx := strings.LastIndex(name, ":")
	if idx <= 0 || idx == len(name)-1 {
		return "", "", false
	}
	return name[:idx], name[idx+1:], true
}

// resolveDirection 语向决策（对应 decide_direction）
func resolveDirection(text, src, tgt string) (string, string) {
	return lang.Resolve(text, src, tgt)
}
