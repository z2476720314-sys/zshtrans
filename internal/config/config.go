package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	AppName    = "ZSH TRANS"
	AppVersion = "v3.0"
	// 原 pyc 常量
	Timeout  = 8
	CacheMax = 300
)

// Hotkeys 三个全局热键（close = 主窗口显隐）
type Hotkeys struct {
	Selection  string `json:"selection"`
	Screenshot string `json:"screenshot"`
	Window     string `json:"window"`
	Close      string `json:"close"`
}

// Web 内置 HTTP 服务
type Web struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

// APIKeys 直连翻译接口与 OCR 的密钥
type APIKeys struct {
	DeepL      string `json:"deepl"`
	BaiduAppID string `json:"baidu_appid"`
	BaiduKey   string `json:"baidu_key"`
	NiuTrans   string `json:"niutrans"`
	OCRSpace   string `json:"ocrspace"`
	BaiduOCRAK string `json:"baidu_ocr_ak"`
	BaiduOCRsk string `json:"baidu_ocr_sk"`
	Agnes      string `json:"agnes"`
	Qwen       string `json:"qwen"`
	Acctoken   string `json:"acctoken"`
}

// Model 一个模型条目
type Model struct {
	ID             string   `json:"id"`
	Label          string   `json:"label,omitempty"`
	Image          bool     `json:"image"`
	Reasoning      string   `json:"reasoning"`
	ThinkingParam  string   `json:"thinking_param,omitempty"`
	ThinkingBudget int      `json:"thinking_budget,omitempty"`
	Temperature    *float64 `json:"temperature"`
	// 以下为模型目录（provider /models）返回的元数据，缺省表示未知
	ContextWindow   int      `json:"context_window,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	ReasoningLevels []string `json:"reasoning_levels,omitempty"`
	ThinkingTypes   []string `json:"thinking_types,omitempty"`
	MetadataSource  string   `json:"metadata_source,omitempty"`
}

// Provider 一个模型提供方
type Provider struct {
	ID            string  `json:"id"`
	PresetID      string  `json:"preset_id,omitempty"`
	Label         string  `json:"label"`
	Group         string  `json:"group,omitempty"`
	BaseURL       string  `json:"base_url"`
	Protocol      string  `json:"protocol"`
	Enabled       bool    `json:"enabled"`
	Builtin       bool    `json:"builtin,omitempty"`
	KeyRef        string  `json:"key_ref"`
	KeyEnv        string  `json:"key_env"`
	ThinkingStyle string  `json:"thinking_style"`
	Timeout       int     `json:"timeout"`
	Stream        bool    `json:"stream"`
	Models        []Model `json:"models"`
	DocsURL       string  `json:"docs_url,omitempty"`
	Notice        string  `json:"notice,omitempty"`
}

// Config 全量配置，字段与 tt_config.json 一一对应
type Config struct {
	Hotkeys         Hotkeys    `json:"hotkeys"`
	SrcLang         string     `json:"src_lang"`
	TgtLang         string     `json:"tgt_lang"`
	DefaultProvider string     `json:"default_provider"`
	OCRModel        string     `json:"ocr_model"`
	Web             Web        `json:"web"`
	APIKeys         APIKeys    `json:"api_keys"`
	Providers       []Provider `json:"providers"`
	// 自更新
	UpdateCheck   bool   `json:"update_check"`   // 启动时自动检查更新
	UpdateSkipped string `json:"update_skipped"` // 用户选择「跳过此版本」的版本号
	UpdateRepo    string `json:"update_repo"`    // 覆盖默认仓库 owner/name
	// 兼容旧配置里的初始化标记，原样保留
	AcctokenInitialized bool `json:"acctoken_deepseek_default_initialized_v2_0,omitempty"`
}

// Default 对应 pyc DEFAULT_CONFIG
func Default() Config {
	return Config{
		Hotkeys: Hotkeys{
			Selection:  "ctrl+alt+t",
			Screenshot: "ctrl+alt+s",
			Window:     "ctrl+alt+m",
			Close:      "ctrl+alt+q",
		},
		SrcLang:         "auto",
		TgtLang:         "zh",
		DefaultProvider: "",
		Web:             Web{Enabled: true, Host: "0.0.0.0", Port: 8765},
		APIKeys:         APIKeys{OCRSpace: "helloworld"},
		UpdateCheck:     true,
		UpdateRepo:      "z2476720314-sys/zshtrans",
	}
}

// Path 配置文件路径：exe 同级目录
func Path() string {
	if p := os.Getenv("ZSHSTRANS_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(BaseDir(), "tt_config.json")
}

// PhrasesPath 常用语文件路径
func PhrasesPath() string {
	return filepath.Join(BaseDir(), "tt_phrases.json")
}

// LogPath 日志路径
func LogPath() string {
	return filepath.Join(BaseDir(), "zshtrans.log")
}

// BaseDir exe 所在目录（开发时为工作目录）
func BaseDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	dir := filepath.Dir(exe)
	// go run / 测试时 exe 在临时目录，退回工作目录
	if base := filepath.Base(dir); base == "go-build" || len(base) > 0 && base[0] == '0' && filepath.Base(filepath.Dir(dir)) == "go-build" {
		wd, _ := os.Getwd()
		return wd
	}
	return dir
}

// Store 内存态配置，支持热更新（对应 pyc 全局 CFG）
type Store struct {
	mu  sync.RWMutex
	cfg Config
}

func NewStore(cfg Config) *Store { return &Store{cfg: cfg} }

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Set(cfg Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// Load 读取配置；缺失字段用默认值补齐，坏文件回退默认值
func Load() (Config, error) {
	def := Default()
	raw, err := os.ReadFile(Path())
	if err != nil {
		return def, err
	}
	// 先填默认值再反序列化，缺失字段保持默认
	cfg := def
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return def, err
	}
	cfg = normalize(cfg, def)
	return cfg, nil
}

func normalize(cfg, def Config) Config {
	if cfg.SrcLang == "" {
		cfg.SrcLang = def.SrcLang
	}
	if cfg.TgtLang == "" {
		cfg.TgtLang = def.TgtLang
	}
	if cfg.Hotkeys.Selection == "" {
		cfg.Hotkeys.Selection = def.Hotkeys.Selection
	}
	if cfg.Hotkeys.Screenshot == "" {
		cfg.Hotkeys.Screenshot = def.Hotkeys.Screenshot
	}
	if cfg.Hotkeys.Window == "" {
		cfg.Hotkeys.Window = def.Hotkeys.Window
	}
	if cfg.Hotkeys.Close == "" {
		cfg.Hotkeys.Close = def.Hotkeys.Close
	}
	if cfg.Web.Host == "" {
		cfg.Web.Host = def.Web.Host
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = def.Web.Port
	}
	if cfg.APIKeys.OCRSpace == "" {
		cfg.APIKeys.OCRSpace = def.APIKeys.OCRSpace
	}
	// 原 install_language_support：src==tgt 时把 src 拉回 auto
	if cfg.SrcLang == cfg.TgtLang {
		cfg.SrcLang = "auto"
	}
	return cfg
}

// Save 原子写配置（缩进 2，保留中文）
func Save(cfg Config) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(Path(), raw)
}

func writeAtomic(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
