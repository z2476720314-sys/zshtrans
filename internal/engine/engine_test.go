package engine

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/local/zshtrans/internal/providers"
)

func mkProvider(name string, fn providers.Func) *providers.Provider {
	return providers.NewProvider(name, fn)
}

func ok(text string) providers.Func {
	return func(src, s, t string) (string, error) { return text, nil }
}

func bad(msg string) providers.Func {
	return func(src, s, t string) (string, error) { return "", errors.New(msg) }
}

// TestTranslateCachesByNormalizedText 空白折叠后的相同文本应命中缓存（provider=cache, ms=0）
func TestTranslateCachesByNormalizedText(t *testing.T) {
	var calls int
	var mu sync.Mutex
	e := New([]*providers.Provider{
		mkProvider("p1", func(src, s, t string) (string, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return "out", nil
		}),
	}, "", 8)

	r1, err := e.Translate("hello   world", "en", "zh")
	if err != nil {
		t.Fatalf("first translate: %v", err)
	}
	if r1.Provider != "p1" || r1.Text != "out" {
		t.Fatalf("unexpected result: %+v", r1)
	}

	// 只差空白 → 归一化后同 key，必须命中缓存
	r2, err := e.Translate("hello world", "en", "zh")
	if err != nil {
		t.Fatalf("second translate: %v", err)
	}
	if r2.Provider != "cache" {
		t.Fatalf("expected cache hit, got provider=%q", r2.Provider)
	}
	if r2.MS != 0 {
		t.Fatalf("cache hit should report 0ms, got %d", r2.MS)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("provider should be called once, got %d", calls)
	}
}

// TestTranslateFailover 第一个失败要自动换下一个，并记录 provider 名
func TestTranslateFailover(t *testing.T) {
	e := New([]*providers.Provider{
		mkProvider("broken", bad("boom")),
		mkProvider("good", ok("ok-text")),
	}, "", 8)
	r, err := e.Translate("x", "en", "zh")
	if err != nil {
		t.Fatalf("expected failover success, got %v", err)
	}
	if r.Provider != "good" || r.Text != "ok-text" {
		t.Fatalf("unexpected result: %+v", r)
	}
}

// TestTranslateAllFail 全失败时错误要列出每个提供方（对应 pyc 'all providers failed:\n'）
func TestTranslateAllFail(t *testing.T) {
	e := New([]*providers.Provider{
		mkProvider("a", bad("err-a")),
		mkProvider("b", bad("err-b")),
	}, "", 8)
	_, err := e.Translate("x", "en", "zh")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "all providers failed:") {
		t.Fatalf("unexpected prefix: %s", msg)
	}
	for _, want := range []string{"a: err-a", "b: err-b"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got %s", want, msg)
		}
	}
}

// TestTranslateNoProvider 空池要报 no provider enabled
func TestTranslateNoProvider(t *testing.T) {
	e := New(nil, "", 8)
	if _, err := e.Translate("x", "en", "zh"); err == nil || !strings.Contains(err.Error(), "no provider enabled") {
		t.Fatalf("expected no provider error, got %v", err)
	}
}

// TestTranslatePrefersDefault 配置了 default_provider 时优先用它
func TestTranslatePrefersDefault(t *testing.T) {
	e := New([]*providers.Provider{
		mkProvider("first", ok("from-first")),
		mkProvider("pref", ok("from-pref")),
	}, "pref", 8)
	r, err := e.Translate("y", "en", "zh")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if r.Provider != "pref" {
		t.Fatalf("expected default provider to win, got %q", r.Provider)
	}
}

// TestTranslateRotates 无默认提供方时按 idx 轮转，连续三次应覆盖全部节点
func TestTranslateRotates(t *testing.T) {
	e := New([]*providers.Provider{
		mkProvider("a", ok("A")),
		mkProvider("b", ok("B")),
	}, "", 8)
	seen := map[string]bool{}
	for _, text := range []string{"1", "2", "3"} {
		r, err := e.Translate(text, "en", "zh")
		if err != nil {
			t.Fatalf("translate %q: %v", text, err)
		}
		seen[r.Provider] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("expected rotation across both providers, got %v", seen)
	}
}

// TestTranslateUnhealthyLast 有失败记录的节点要排到健康节点之后
func TestTranslateUnhealthyLast(t *testing.T) {
	e := New([]*providers.Provider{
		mkProvider("sick", ok("sick-out")),
		mkProvider("well", ok("well-out")),
	}, "", 8)

	// 让 sick 连续失败两次，进入冷却（Healthy()=false）才会被排到后面
	sick := e.Pool()[0]
	if sick.Name != "sick" {
		t.Fatalf("unexpected pool order: %s", sick.Name)
	}
	sick.MarkFail()
	sick.MarkFail()
	if sick.Healthy() {
		t.Fatal("provider should be unhealthy after two consecutive failures")
	}

	// 换一个从没查过的文本，避免命中缓存
	r, err := e.Translate("fresh text", "en", "zh")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if r.Provider != "well" {
		t.Fatalf("healthy provider should be tried first, got %q", r.Provider)
	}
}

// TestCacheEviction LRU 超过容量后最旧的条目应被淘汰
func TestCacheEviction(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	e := New([]*providers.Provider{
		mkProvider("p", func(src, s, t string) (string, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return "v", nil
		}),
	}, "", 8)
	// 直接换小容量缓存（容量是构造时固定的，测试里注入一个 2 条的 LRU）
	e.mu.Lock()
	e.cache = newLRU(2)
	e.mu.Unlock()

	for _, text := range []string{"a", "b", "c"} {
		if _, err := e.Translate(text, "en", "zh"); err != nil {
			t.Fatalf("translate %q: %v", text, err)
		}
	}
	// "a" 应已被淘汰 → 再查会重新调提供方
	if _, err := e.Translate("a", "en", "zh"); err != nil {
		t.Fatalf("translate a again: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 4 {
		t.Fatalf("expected 4 provider calls (a,b,c,a), got %d", calls)
	}
}
