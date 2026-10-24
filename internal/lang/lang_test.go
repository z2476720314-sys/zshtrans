package lang

import "testing"

// TestDetect 语言探测：ja/ko/ar/th 先判，再西里尔→ru，再汉字→zh，其余 en
func TestDetect(t *testing.T) {
	cases := []struct{ text, want string }{
		{"こんにちは", "ja"},
		{"カタカナ", "ja"},
		{"안녕하세요", "ko"},
		{"مرحبا", "ar"},
		{"สวัสดี", "th"},
		{"привет мир", "ru"},
		{"你好世界", "zh"},
		{"hello world", "en"},
	}
	for _, c := range cases {
		if got := Detect(c.text); got != c.want {
			t.Errorf("Detect(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestResolveAuto 源语言 auto：检测结果与目标相同则翻转，否则保持 auto
func TestResolveAuto(t *testing.T) {
	// 俄文识别为 ru，目标也是 ru → 翻成 zh（同语言不能翻译）
	src, tgt := Resolve("привет", "auto", "ru")
	if src != "ru" || tgt != "zh" {
		t.Errorf("auto→ru with Russian text: got (%q,%q), want (ru,zh)", src, tgt)
	}
	// 俄文识别为 ru，目标是 zh → 保持 auto，交给提供方自己判断
	src, tgt = Resolve("привет", "auto", "zh")
	if src != "auto" || tgt != "zh" {
		t.Errorf("auto→zh with Russian text: got (%q,%q), want (auto,zh)", src, tgt)
	}
}

// TestResolveSameLang src==tgt 时必须翻转成 FLIP 里的对向语言
func TestResolveSameLang(t *testing.T) {
	src, tgt := Resolve("hello", "ru", "ru")
	if src != "ru" || tgt != "zh" {
		t.Errorf("ru→ru: got (%q,%q), want (ru,zh)", src, tgt)
	}
	src, tgt = Resolve("hello", "zh", "zh")
	if src != "zh" || tgt != "ru" {
		t.Errorf("zh→zh: got (%q,%q), want (zh,ru)", src, tgt)
	}
}

// TestResolveExplicit 显式语向原样返回
func TestResolveExplicit(t *testing.T) {
	src, tgt := Resolve("hello", "en", "zh")
	if src != "en" || tgt != "zh" {
		t.Errorf("en→zh: got (%q,%q)", src, tgt)
	}
}

// TestFlipOf 16 语言全部有对向：ru/zh/en 各自映射，其余非中文语言一律翻回 zh。
// 这是相对原 pyc（只有 ru/zh/en 三项）的有意扩展，见 docs/FEATURES.md 7.5。
func TestFlipOf(t *testing.T) {
	if FlipOf("ru") != "zh" || FlipOf("zh") != "ru" || FlipOf("en") != "zh" {
		t.Errorf("unexpected flip for ru/zh/en: ru=%q zh=%q en=%q", FlipOf("ru"), FlipOf("zh"), FlipOf("en"))
	}
	for _, id := range []string{"ja", "ko", "fr", "de", "es", "pt", "it", "ar", "tr", "vi", "th", "id"} {
		if got := FlipOf(id); got != "zh" {
			t.Errorf("FlipOf(%q) = %q, want zh", id, got)
		}
	}
	// auto 没有对向语言，兜底返回 ru（Swap 里 auto 走的是 detected/tgt 分支，不会用到）
	if got := FlipOf("auto"); got != "ru" {
		t.Errorf("FlipOf(\"auto\") = %q, want ru fallback", got)
	}
}

// TestScriptChecks 汉字/西里尔检测（译文残留检查依赖这两个函数）
func TestScriptChecks(t *testing.T) {
	if !HasHan("你好") || HasHan("привет") {
		t.Error("HasHan misbehaves")
	}
	if !HasCyrillic("привет") || HasCyrillic("你好") {
		t.Error("HasCyrillic misbehaves")
	}
	// 日文汉字也在汉字区间内（U+4E00-9FFF），应被判为含汉字
	if !HasHan("日本語") {
		t.Error("Japanese kanji should count as Han")
	}
}

// TestAllOptions 16 种语言全列出，auto 只在源语言侧
func TestAllOptions(t *testing.T) {
	if got := len(AllOptions()); got != 16 {
		t.Errorf("expected 16 language options, got %d", got)
	}
	targets := TargetOptions()
	for _, o := range targets {
		if o.Value == "auto" {
			t.Error("auto must not be a target language")
		}
	}
	if len(targets) != 15 {
		t.Errorf("expected 15 target options, got %d", len(targets))
	}
}

// TestWebLangWhitelist 原 pyc 的 web 接口白名单（有意扩展后仍需保留 ru/en/zh）
func TestWebLangWhitelist(t *testing.T) {
	for _, code := range []string{"auto"} {
		if !WebSrcAllowed[code] {
			t.Errorf("%q should be allowed as web src", code)
		}
	}
	for _, code := range []string{"ru", "en", "zh"} {
		if !WebSrcAllowed[code] {
			t.Errorf("%q should be allowed as web src", code)
		}
		if !WebTgtAllowed[code] {
			t.Errorf("%q should be allowed as web tgt", code)
		}
	}
	if WebTgtAllowed["auto"] {
		t.Error("auto must not be allowed as web tgt")
	}
}
