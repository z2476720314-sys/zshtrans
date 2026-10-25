package update

import (
	"os"
	"testing"
)

// writeFile 测试用落盘助手
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// TestTrimVersion 去 v/V 前缀
func TestTrimVersion(t *testing.T) {
	cases := map[string]string{
		"v3.0":    "3.0",
		"V3.1.2":  "3.1.2",
		"3.0":     "3.0",
		" v2.38 ": "2.38",
		"":        "",
	}
	for in, want := range cases {
		if got := TrimVersion(in); got != want {
			t.Errorf("TrimVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCompareVersions 只比数字段，缺失按 0
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"3.0", "3.0", 0},
		{"3.1", "3.0", 1},
		{"3.0", "3.1", -1},
		{"3.0.1", "3.0", 1},
		{"3.0", "3.0.0", 0},
		{"v3.10", "v3.9", 1},
		{"4", "3.99.99", 1},
		{"", "", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestCheckPEFixture checkPE 对合法 PE 头放行、对垃圾数据拒绝。
// 用最小合成字节而不是真 exe，保证测试不依赖外部文件。
func TestCheckPEFixture(t *testing.T) {
	dir := t.TempDir()

	// 真实测试二进制也是 x64 PE；只有签名的合成文件不能作为合法更新包。
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	off := 0x40
	goodPath := dir + "/good.exe"
	if err := writeFile(goodPath, good); err != nil {
		t.Fatal(err)
	}
	if err := checkPE(goodPath); err != nil {
		t.Errorf("valid PE rejected: %v", err)
	}

	// 非法 1：不是 MZ
	badPath := dir + "/bad1.exe"
	if err := writeFile(badPath, make([]byte, 0x200)); err != nil {
		t.Fatal(err)
	}
	if err := checkPE(badPath); err == nil {
		t.Error("non-MZ file should be rejected")
	}

	// 非法 2：MZ 但没有 PE 签名
	noSig := make([]byte, 0x200)
	noSig[0], noSig[1] = 'M', 'Z'
	noSig[0x3C] = byte(off)
	noSigPath := dir + "/bad2.exe"
	if err := writeFile(noSigPath, noSig); err != nil {
		t.Fatal(err)
	}
	if err := checkPE(noSigPath); err == nil {
		t.Error("MZ without PE signature should be rejected")
	}

	// 非法 3：文件太短
	shortPath := dir + "/bad3.exe"
	if err := writeFile(shortPath, []byte("MZ")); err != nil {
		t.Fatal(err)
	}
	if err := checkPE(shortPath); err == nil {
		t.Error("truncated file should be rejected")
	}
}

// TestPickLatest 发布列表按版本号取最大，跳过 draft / prerelease。
// 这条对应真实缺陷：GitHub 的 /releases/latest 按 created_at 排序，同一天连发
// 两个 release 时会返回较旧的那个（实测 v3.2 建好后 latest 仍指向 v3.1），
// 导致「检查更新」永远说已是最新。改用列表 + 版本号比较。
func TestPickLatest(t *testing.T) {
	list := []Release{
		{Tag: "v3.1", Draft: false, Prerelease: false},
		{Tag: "v3.2", Draft: false, Prerelease: false},
		{Tag: "v9.9", Draft: true, Prerelease: false},   // draft 必须跳过
		{Tag: "v8.0", Draft: false, Prerelease: true},   // prerelease 必须跳过
		{Tag: "v3.10", Draft: false, Prerelease: false}, // 数字比较，不能按字符串
	}
	best := pickLatest(list)
	if best == nil {
		t.Fatal("pickLatest returned nil")
	}
	if best.Tag != "v3.10" {
		t.Errorf("pickLatest = %q, want v3.10", best.Tag)
	}

	// 全是 draft/prerelease → nil
	if got := pickLatest([]Release{{Tag: "v1.0", Draft: true}}); got != nil {
		t.Errorf("all-draft list should return nil, got %q", got.Tag)
	}
	// 空列表 → nil
	if got := pickLatest(nil); got != nil {
		t.Errorf("empty list should return nil, got %q", got.Tag)
	}
}

// TestReleaseVersionComputed 列表项算出的 Version 已去掉 v 前缀
func TestReleaseVersionComputed(t *testing.T) {
	list := []Release{{Tag: "v3.2"}}
	best := pickLatest(list)
	if best == nil || best.Version != "3.2" {
		t.Fatalf("best.Version = %q, want 3.2", best.Version)
	}
}
