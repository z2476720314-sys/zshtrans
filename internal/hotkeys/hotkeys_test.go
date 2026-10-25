package hotkeys

import "testing"

// TestParseValid 覆盖原 pyc parse_native_hotkey 支持的写法
func TestParseValid(t *testing.T) {
	cases := []struct {
		in   string
		mods uint32
		key  uint32
	}{
		{"ctrl+alt+t", ModControl | ModAlt, 0x54},
		{"CTRL+ALT+T", ModControl | ModAlt, 0x54},
		{"ctrl+alt+s", ModControl | ModAlt, 0x53},
		{"ctrl+alt+m", ModControl | ModAlt, 0x4D},
		{"ctrl+alt+q", ModControl | ModAlt, 0x51},
		{"ctrl+shift+f5", ModControl | ModShift, 0x74},
		{" alt + ctrl + a ", ModControl | ModAlt, 0x41},
		{"win+space", ModWin, 0x20},
		{"ctrl+enter", ModControl, 0x0D},
	}
	for _, c := range cases {
		combo, err := Parse(c.in)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", c.in, err)
		}
		if combo.Mods != c.mods || combo.Key != c.key {
			t.Errorf("%q: got mods=%#x key=%#x, want mods=%#x key=%#x",
				c.in, combo.Mods, combo.Key, c.mods, c.key)
		}
	}
}

// TestParseInvalid 修饰键结尾、缺修饰键、未知键、重复键都应报错
func TestParseInvalid(t *testing.T) {
	for _, in := range []string{
		"", "   ", "ctrl", "alt", "t", "ctrl+alt", "ctrl+alt+", "shift+nope", "ctrl+a+b",
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("%q: expected error, got nil", in)
		}
	}
}

// TestValidateConflict 两个操作绑定同一组合键必须被拒（对应 candidate() 的热键冲突）
func TestValidateConflict(t *testing.T) {
	err := Validate("ctrl+alt+t", "ctrl+alt+t", "ctrl+alt+m", "ctrl+alt+q")
	if err == nil {
		t.Fatal("expected conflict error for duplicated hotkey")
	}
	if err := Validate("ctrl+alt+t", "ctrl+alt+s", "ctrl+alt+m", "ctrl+alt+q"); err != nil {
		t.Fatalf("distinct hotkeys should pass, got %v", err)
	}
	// 大小写与空格不同但语义相同的组合也算冲突
	if err := Validate("ctrl+alt+t", "ALT+CTRL+T", "ctrl+alt+m", "ctrl+alt+q"); err == nil {
		t.Fatal("expected conflict for case/order-insensitive duplicate")
	}
}

// TestValidateInvalid 非法格式的报错文案要带操作名和原值（对应 pyc 的 %s热键 %s 无效）
func TestValidateInvalid(t *testing.T) {
	err := Validate("bogus", "ctrl+alt+s", "ctrl+alt+m", "ctrl+alt+q")
	if err == nil {
		t.Fatal("expected error for invalid hotkey")
	}
	if got := err.Error(); got != "划词翻译热键 bogus 无效" {
		t.Errorf("unexpected message: %s", got)
	}
}

// TestNormalize 归一化后应当稳定、可再次解析
func TestNormalize(t *testing.T) {
	for _, in := range []string{"CTRL+ALT+T", "alt+ctrl+t", " ctrl + alt + t "} {
		if got := Normalize(in); got != "alt+ctrl+t" {
			t.Errorf("%q: got %q, want alt+ctrl+t", in, got)
		}
	}
	// 非法输入原样返回，不吞掉用户的输入
	if got := Normalize("bogus"); got != "bogus" {
		t.Errorf("invalid input should be returned as-is, got %q", got)
	}
}
