package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestModelPreservesLegacyOutputLimit(t *testing.T) {
	var model Model
	if err := json.Unmarshal([]byte(`{"id":"translator","output_limit":4096,"max_output_tokens":65536}`), &model); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["output_limit"] != float64(4096) || saved["max_output_tokens"] != float64(65536) {
		t.Fatalf("user output limit must survive separately from model capacity: %s", raw)
	}
}

// TestDefaultSane 默认配置的关键字段必须与 pyc DEFAULT_CONFIG 一致
func TestDefaultSane(t *testing.T) {
	c := Default()
	if c.SrcLang != "auto" || c.TgtLang != "zh" {
		t.Errorf("default langs = %q→%q, want auto→zh", c.SrcLang, c.TgtLang)
	}
	if c.APIKeys.OCRSpace != "helloworld" {
		t.Errorf("default ocrspace = %q, want helloworld", c.APIKeys.OCRSpace)
	}
	if !c.Web.Enabled || c.Web.Host != "127.0.0.1" || c.Web.Port != 8765 {
		t.Errorf("default web = %+v", c.Web)
	}
	if c.Hotkeys.Selection != "ctrl+alt+t" || c.Hotkeys.Screenshot != "ctrl+alt+s" ||
		c.Hotkeys.Window != "ctrl+alt+m" || c.Hotkeys.Close != "ctrl+alt+q" {
		t.Errorf("default hotkeys = %+v", c.Hotkeys)
	}
	if !c.UpdateCheck {
		t.Error("update check should default to true")
	}
}

// TestNormalizeSameLang src==tgt 时 src 归为 auto（契约行为）
func TestNormalizeSameLang(t *testing.T) {
	c := Default()
	c.SrcLang, c.TgtLang = "ru", "ru"
	n := normalize(c, Default())
	if n.SrcLang != "auto" || n.TgtLang != "ru" {
		t.Errorf("normalize ru→ru = %q→%q, want auto→ru", n.SrcLang, n.TgtLang)
	}
}

// TestNormalizeClearsOCRSpace 用户主动清空的 ocrspace 不得被复活（M9 回归）
func TestNormalizeClearsOCRSpace(t *testing.T) {
	c := Default()
	c.APIKeys.OCRSpace = ""
	n := normalize(c, Default())
	if n.APIKeys.OCRSpace != "" {
		t.Errorf("user-cleared ocrspace was resurrected to %q", n.APIKeys.OCRSpace)
	}
}

// TestLoadCorruptBacksUp 坏配置必须改名为 .bad 留档，不得被默认值覆盖（M8 回归）。
// Load 走包级 Path()（= BaseDir()/tt_config.json），因此这里临时把工作目录切到
// t.TempDir()，让 Path() 指到临时目录里，测完再切回。
func TestLoadCorruptBacksUp(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	// BaseDir 在测试里可能因为 go-build 临时目录回退到工作目录，先确认目标路径
	target := Path()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("corrupt config should return an error")
	}
	if _, err := os.Stat(target + ".bad"); err != nil {
		t.Fatalf("backup %q not created: %v", target+".bad", err)
	}
}

// TestStoreGetIsolated Store.Get 返回深拷贝，改它不影响内部状态（L1 回归）
func TestStoreGetIsolated(t *testing.T) {
	c := Default()
	c.Providers = []Provider{{
		ID:     "p1",
		Label:  "P1",
		Models: []Model{{ID: "m1", Label: "M1"}},
	}}
	s := NewStore(c)

	got := s.Get()
	got.Providers[0].Label = "MUTATED"
	got.Providers[0].Models[0].Label = "MUTATED-MODEL"

	again := s.Get()
	if again.Providers[0].Label == "MUTATED" {
		t.Error("Store.Get leaked Providers slice")
	}
	if again.Providers[0].Models[0].Label == "MUTATED-MODEL" {
		t.Error("Store.Get leaked nested Models slice")
	}
}

// TestSaveLoadRoundTrip 保存后读回必须一致（含密钥与热键）
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tt_config.json")

	c := Default()
	c.SrcLang = "ru"
	c.TgtLang = "zh"
	c.APIKeys.DeepL = "deepl-key"
	c.Hotkeys.Selection = "ctrl+shift+f1"

	raw0, err0 := json.MarshalIndent(c, "", "  ")
	if err0 != nil {
		t.Fatal(err0)
	}
	if err := writeAtomic(path, raw0); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	got = normalize(got, Default())
	if got.SrcLang != "ru" || got.TgtLang != "zh" {
		t.Errorf("langs round-trip = %q→%q", got.SrcLang, got.TgtLang)
	}
	if got.APIKeys.DeepL != "deepl-key" {
		t.Errorf("deepl key lost: %q", got.APIKeys.DeepL)
	}
	if got.Hotkeys.Selection != "ctrl+shift+f1" {
		t.Errorf("hotkey lost: %q", got.Hotkeys.Selection)
	}
}
