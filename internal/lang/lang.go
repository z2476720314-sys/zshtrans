package lang

import (
	"regexp"
	"strings"
)

// Languages 16 种语言（translate.py:3185 LANGUAGES）
var Languages = map[string]string{
	"auto": "自动检测", "zh": "中文", "en": "英语", "ru": "俄语",
	"ja": "日语", "ko": "韩语", "fr": "法语", "de": "德语",
	"es": "西班牙语", "pt": "葡萄牙语", "it": "意大利语", "ar": "阿拉伯语",
	"tr": "土耳其语", "vi": "越南语", "th": "泰语", "id": "印尼语",
}

// Order 下拉顺序（与 Qt 界面一致）
var Order = []string{
	"auto", "zh", "en", "ru", "ja", "ko", "fr", "de",
	"es", "pt", "it", "ar", "tr", "vi", "th", "id",
}

// Flip 语向翻转表：zh→ru 为默认业务方向。
// translate.py:4338 install_language_support 把 FLIP 扩展为「除 auto/zh 外一律→zh」，
// 因此 ja/ko/fr/... 反向都回中文，这里在 init 中按同一规则补齐。
var Flip = map[string]string{
	"ru": "zh", "zh": "ru", "en": "zh",
}

func init() {
	for id := range Languages {
		if id == "auto" || id == "zh" {
			continue
		}
		Flip[id] = "zh"
	}
}

var (
	cyr    = regexp.MustCompile(`[\x{0400}-\x{04FF}\x{0500}-\x{052F}]`)
	cjk    = regexp.MustCompile(`[\x{3400}-\x{4DBF}\x{4E00}-\x{9FFF}\x{F900}-\x{FAFF}]`)
	kana   = regexp.MustCompile(`[\x{3040}-\x{30FF}]`)
	hangul = regexp.MustCompile(`[\x{1100}-\x{11FF}\x{AC00}-\x{D7AF}]`)
	arabic = regexp.MustCompile(`[\x{0600}-\x{06FF}]`)
	thai   = regexp.MustCompile(`[\x{0E00}-\x{0E7F}]`)
)

// Detect 语言检测。
// 顺序与原实现一致：先 cyr→ru，再 cjk→zh，其余 en；
// 但 pyc detect_lang 只判 3 类，translate.py:4331 install_language_support
// 在最前面加了 ja/ko/ar/th 四个扩展，这里合并为同一条链。
func Detect(text string) string {
	// 扩展检测优先（原实现覆盖在 detect_lang 外层）
	for _, probe := range []struct {
		re *regexp.Regexp
		id string
	}{
		{kana, "ja"},
		{hangul, "ko"},
		{arabic, "ar"},
		{thai, "th"},
	} {
		if probe.re.MatchString(text) {
			return probe.id
		}
	}
	if cyr.MatchString(text) {
		return "ru"
	}
	if cjk.MatchString(text) {
		return "zh"
	}
	return "en"
}

// Resolve 语向决策（pyc resolve_direction）
func Resolve(text, src, tgt string) (string, string) {
	if src == "auto" {
		d := Detect(text)
		if d == tgt {
			return d, flipOf(d)
		}
		return "auto", tgt
	}
	if src == tgt {
		return src, flipOf(src)
	}
	return src, tgt
}

// Decide 用配置里的 src/tgt 决策（pyc decide_direction）
func Decide(text, cfgSrc, cfgTgt string) (string, string) {
	if Detect(text) != "zh" {
		return "auto", "zh"
	}
	if cfgTgt == "auto" || cfgTgt == "zh" || cfgTgt == "" {
		cfgTgt = cfgSrc
		if cfgTgt == "auto" || cfgTgt == "zh" || cfgTgt == "" {
			cfgTgt = flipOf("zh")
		}
	}
	return "zh", cfgTgt
}

func flipOf(id string) string {
	if v, ok := Flip[id]; ok && v != "" {
		return v
	}
	return "ru"
}

// FlipOf 暴露给外部（前端交换按钮用）
func FlipOf(id string) string { return flipOf(id) }

// Swap 交换语向，严格照 translate.py:2874 _swap_lang：
//
//	newTgt = src（src 不是 auto 时）否则 FLIP[tgt]
//	newSrc = 旧 tgt
//
// 若交换后 src == tgt，按 _set_lang 的规则把 src 归为 auto。
// detected 用于 src 为 auto 时替代真实语种（对应 web 版 lastDetected）。
func Swap(src, tgt, detected string) (string, string) {
	if src == "" {
		src = "auto"
	}
	if tgt == "" {
		tgt = "zh"
	}
	base := src
	if base == "auto" {
		base = strings.TrimSpace(detected)
		if base == "" || base == "auto" {
			base = "ru"
		}
	}
	newTgt := base
	if base == tgt {
		newTgt = flipOf(tgt)
	}
	newSrc := tgt
	if newSrc == newTgt {
		newSrc = "auto"
	}
	return newSrc, newTgt
}

// HasCyrillic 目标中文时校验残留西里尔
func HasCyrillic(s string) bool { return cyr.MatchString(s) }

// HasHan 目标俄语时校验残留汉字
func HasHan(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return (r >= 0x3400 && r <= 0x4DBF) ||
			(r >= 0x4E00 && r <= 0x9FFF) ||
			(r >= 0xF900 && r <= 0xFAFF) ||
			(r >= 0x20000 && r <= 0x2FA1F)
	}) >= 0
}

// Name 语言中文名
func Name(id string) string {
	if v, ok := Languages[id]; ok {
		return v
	}
	return id
}

// Options 前端下拉用 [{value,label}]
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// AllOptions 源语言下拉（含 auto）
func AllOptions() []Option {
	out := make([]Option, 0, len(Order))
	for _, id := range Order {
		out = append(out, Option{Value: id, Label: Languages[id]})
	}
	return out
}

// TargetOptions 目标语言下拉（不含 auto）
func TargetOptions() []Option {
	out := make([]Option, 0, len(Order)-1)
	for _, id := range Order {
		if id == "auto" {
			continue
		}
		out = append(out, Option{Value: id, Label: Languages[id]})
	}
	return out
}

// WebAllowed 语言白名单。
// 原 pyc WebHandler 只认 auto/ru/en/zh，因为内置网页只有 3 个语种；
// 桌面端按 translate.py LANGUAGES 支持全部 16 种，因此这里放开到全量。
var (
	WebSrcAllowed = func() map[string]bool {
		m := make(map[string]bool, len(Languages))
		for id := range Languages {
			m[id] = true
		}
		return m
	}()
	WebTgtAllowed = func() map[string]bool {
		m := make(map[string]bool, len(Languages))
		for id := range Languages {
			if id != "auto" {
				m[id] = true
			}
		}
		return m
	}()
)
