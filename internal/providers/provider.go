package providers

import (
	"context"
	"math"
	"sync"
	"time"
)

// Provider 引擎中的一个可用翻译节点（严格对应 pyc class Provider）
type Provider struct {
	Name string
	Fn   Func
	// needKeys 需要哪些密钥才启用（pyc __init__(name, fn, need_keys)）
	needKeys []string
	// mu 保护 fails / cooldownUntil（HTTP 请求并发，健康度必须线程安全）
	mu sync.RWMutex
	// fails 连续失败次数
	fails int
	// cooldownUntil 冷却截止时间戳（秒），pyc cooldown_until
	cooldownUntil float64
	// hasKey 密钥存在性探测（由引擎注入；nil 视为全部齐备）
	hasKey func(string) bool
}

// NewProvider 构建提供方节点
func NewProvider(name string, fn Func, needKeys ...string) *Provider {
	return &Provider{Name: name, Fn: fn, needKeys: needKeys}
}

// BindKeys 注入密钥存在性探测（pyc enabled() 读取 CFG['api_keys'].get(k) 真值）
func (p *Provider) BindKeys(has func(string) bool) { p.hasKey = has }

// Enabled 全部必需密钥齐备才启用（pyc enabled()）
func (p *Provider) Enabled() bool {
	for _, k := range p.needKeys {
		if p.hasKey == nil {
			continue
		}
		if !p.hasKey(k) {
			return false
		}
	}
	return true
}

// Healthy 冷却期结束即健康（pyc healthy(): time.time() >= cooldown_until）
func (p *Provider) Healthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return now() >= p.cooldownUntil
}

// MarkOK 清零失败与冷却（pyc mark_ok）
func (p *Provider) MarkOK() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fails = 0
	p.cooldownUntil = 0
}

// MarkFail 失败计数；连续 2 次起进入冷却，时长 min(300, 60*fails) 秒（pyc mark_fail）
func (p *Provider) MarkFail() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fails++
	if p.fails >= 2 {
		p.cooldownUntil = now() + math.Min(300, 60*float64(p.fails))
	}
}

// NeedKeys 只读暴露必需密钥名
func (p *Provider) NeedKeys() []string { return p.needKeys }

// Fails 只读暴露连续失败次数（自检与状态展示）
func (p *Provider) Fails() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.fails
}

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// directNeedKeys 直连提供方名 → 必需密钥（google/mymemory/lingva 无密钥要求）
var directNeedKeys = map[string][]string{
	"qwen-max":      {"qwen"},
	"qwen-max-2606": {"qwen"},
	"deepl":         {"deepl"},
	"baidu":         {"baidu_appid", "baidu_key"},
	"niutrans":      {"niutrans"},
	"agnes":         {"agnes"},
}

// DirectNeedKeys 返回直连提供方所需密钥名；空切片表示无需密钥（对应 pyc need_keys）
func DirectNeedKeys(name string) []string {
	if keys, ok := directNeedKeys[name]; ok {
		return keys
	}
	return nil
}

// BuildDirect 按 pyc TranslateEngine.__init__ 顺序与 need_keys 构建 9 个直连提供方
func BuildDirect(k Keys) []*Provider {
	rows := []*Provider{
		NewProvider("qwen-max", func(ctx context.Context, t, s, g string) (string, error) { return tQwenMax(ctx, t, s, g, k) }, "qwen"),
		NewProvider("qwen-max-2606", func(ctx context.Context, t, s, g string) (string, error) { return tQwenMaxSnap(ctx, t, s, g, k) }, "qwen"),
		NewProvider("google", tGoogle),
		NewProvider("mymemory", tMyMemory),
		NewProvider("deepl", func(ctx context.Context, t, s, g string) (string, error) { return tDeepL(ctx, t, s, g, k) }, "deepl"),
		NewProvider("baidu", func(ctx context.Context, t, s, g string) (string, error) { return tBaidu(ctx, t, s, g, k) }, "baidu_appid", "baidu_key"),
		NewProvider("niutrans", func(ctx context.Context, t, s, g string) (string, error) { return tNiuTrans(ctx, t, s, g, k) }, "niutrans"),
		NewProvider("lingva", tLingva),
		NewProvider("agnes", func(ctx context.Context, t, s, g string) (string, error) { return tAgnes(ctx, t, s, g, k) }, "agnes"),
	}
	for _, p := range rows {
		p.BindKeys(k.Has)
	}
	return rows
}

// Labels 9 个提供方的显示名（pyc PROVIDER_LABELS）
var Labels = map[string]string{
	"google":        "Google 网页翻译",
	"mymemory":      "MyMemory",
	"deepl":         "DeepL Free",
	"baidu":         "百度翻译",
	"niutrans":      "小牛翻译",
	"lingva":        "Lingva",
	"agnes":         "Agnes 2.5 Flash",
	"qwen-max":      "GLM-5.2",
	"qwen-max-2606": "Qwen3.7-Max(2026-06-08)",
}

// Label 提供方显示名
func Label(name string) string {
	if v, ok := Labels[name]; ok {
		return v
	}
	return name
}
