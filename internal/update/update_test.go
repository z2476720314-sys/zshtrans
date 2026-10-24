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

	// 合法：MZ 头 + 0x3C 处指向 PE 签名
	good := make([]byte, 0x200)
	good[0], good[1] = 'M', 'Z'
	off := 0x40
	good[0x3C] = byte(off)
	good[0x3D] = 0
	good[0x3E] = 0
	good[0x3F] = 0
	copy(good[off:], []byte{'P', 'E', 0, 0})
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
