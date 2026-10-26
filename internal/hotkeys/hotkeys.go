package hotkeys

import (
	"fmt"
	"sort"
	"strings"
)

// Modifier 位掩码（对应 Win32 MOD_* 常量）
const (
	ModAlt      = 0x0001
	ModControl  = 0x0002
	ModShift    = 0x0004
	ModWin      = 0x0008
	ModNoRepeat = 0x4000
)

// keyCodes 键名 → 虚拟键码（translate.py parse_native_hotkey 支持集）
var keyCodes = map[string]uint32{
	"backspace": 0x08, "tab": 0x09, "enter": 0x0D, "return": 0x0D,
	"esc": 0x1B, "escape": 0x1B, "space": 0x20,
	"pageup": 0x21, "pagedown": 0x22, "end": 0x23, "home": 0x24,
	"left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28,
	"insert": 0x2D, "delete": 0x2E, "del": 0x2E,
	"`": 0xC0, "-": 0xBD, "=": 0xBB, "[": 0xDB, "]": 0xDD,
	"\\": 0xDC, ";": 0xBA, "'": 0xDE, ",": 0xBC, ".": 0xBE, "/": 0xBF,
	"num0": 0x60, "num1": 0x61, "num2": 0x62, "num3": 0x63, "num4": 0x64,
	"num5": 0x65, "num6": 0x66, "num7": 0x67, "num8": 0x68, "num9": 0x69,
	"multiply": 0x6A, "add": 0x6B, "subtract": 0x6D, "decimal": 0x6E, "divide": 0x6F,
}

// aliasModifiers 修饰键别名
var aliasModifiers = map[string]uint32{
	"ctrl": ModControl, "control": ModControl,
	"alt": ModAlt, "shift": ModShift,
	"win": ModWin, "meta": ModWin, "super": ModWin, "cmd": ModWin,
}

func init() {
	for i := 0; i < 26; i++ {
		keyCodes[string(rune('a'+i))] = uint32(0x41 + i)
	}
	for i := 0; i < 10; i++ {
		keyCodes[fmt.Sprintf("%d", i)] = uint32(0x30 + i)
	}
	for i := 1; i <= 24; i++ {
		keyCodes[fmt.Sprintf("f%d", i)] = uint32(0x6F + i)
	}
}

// Combo 解析后的热键
type Combo struct {
	Mods uint32
	Key  uint32
	Raw  string
}

// Parse 解析 "ctrl+alt+t" → Combo（对应 parse_native_hotkey）
func Parse(text string) (Combo, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return Combo{}, fmt.Errorf("hotkey %q 无效", text)
	}
	parts := strings.Split(strings.ToLower(raw), "+")
	parts = compact(parts)
	if len(parts) == 0 {
		return Combo{}, fmt.Errorf("hotkey %q 无效", text)
	}
	var mods uint32
	var key uint32
	for i, part := range parts {
		if mod, ok := aliasModifiers[part]; ok {
			// 修饰键只能出现在前面
			if i == len(parts)-1 {
				return Combo{}, fmt.Errorf("hotkey %q 无效", text)
			}
			mods |= mod
			continue
		}
		if key != 0 {
			return Combo{}, fmt.Errorf("hotkey %q 无效", text)
		}
		code, ok := keyCodes[part]
		if !ok {
			return Combo{}, fmt.Errorf("hotkey %q 无效", text)
		}
		key = code
	}
	if key == 0 || mods == 0 {
		return Combo{}, fmt.Errorf("hotkey %q 无效", text)
	}
	return Combo{Mods: mods, Key: key, Raw: raw}, nil
}

// compact 去掉空段
func compact(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Binding 一个命名热键
type Binding struct {
	Name  string
	Label string
	Value string
}

// Fields 三个可编辑热键
var Fields = []Binding{
	{Name: "selection", Label: "划词翻译", Value: ""},
	{Name: "screenshot", Label: "截图翻译", Value: ""},
	{Name: "window", Label: "主窗口显隐", Value: ""},
}

// Validate 校验一组热键：非法格式 / 相互冲突
func Validate(selection, screenshot, window string) error {
	seen := map[uint64]string{}
	for _, b := range []Binding{
		{"selection", "划词翻译", selection},
		{"screenshot", "截图翻译", screenshot},
		{"window", "主窗口显隐", window},
	} {
		combo, err := Parse(b.Value)
		if err != nil {
			return fmt.Errorf("%s热键 %s 无效", b.Label, orEmpty(b.Value))
		}
		id := uint64(combo.Mods)<<32 | uint64(combo.Key)
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%s热键与其他操作冲突", b.Label)
		}
		seen[id] = b.Name
	}
	return nil
}

func orEmpty(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(空)"
	}
	return v
}

// Normalize 归一化热键字符串（小写、去空格、修饰键排序）
func Normalize(text string) string {
	combo, err := Parse(text)
	if err != nil {
		return strings.TrimSpace(text)
	}
	names := make([]string, 0, 4)
	for name, bit := range map[string]uint32{
		"ctrl": ModControl, "alt": ModAlt, "shift": ModShift, "win": ModWin,
	} {
		if combo.Mods&bit != 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for key, code := range keyCodes {
		if code == combo.Key && key != "" {
			names = append(names, key)
			break
		}
	}
	return strings.Join(names, "+")
}
