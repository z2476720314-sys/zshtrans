package lang

import "testing"

func TestQuickTranslationDirection(t *testing.T) {
	for _, tc := range []struct{ text, src, tgt, wantSrc, wantTgt string }{
		{"Привет", "zh", "ru", "auto", "zh"},
		{"Hello", "ru", "en", "auto", "zh"},
		{"こんにちは", "auto", "ru", "auto", "zh"},
		{"你好", "ru", "zh", "zh", "ru"},
		{"你好", "en", "zh", "zh", "en"},
		{"你好", "auto", "zh", "zh", "ru"},
		{"你好", "ru", "fr", "zh", "fr"},
	} {
		src, tgt := Decide(tc.text, tc.src, tc.tgt)
		if src != tc.wantSrc || tgt != tc.wantTgt {
			t.Errorf("Decide(%q,%s,%s) = %s/%s, want %s/%s", tc.text, tc.src, tc.tgt, src, tgt, tc.wantSrc, tc.wantTgt)
		}
	}
}
