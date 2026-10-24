package ocr

import (
	"image"
	"image/color"
	"testing"
)

// TestOtsuTwoTone 双峰图像（黑底 + 白块）的阈值必须把所有亮像素分到同一侧。
// 注意：像素只有 0 与 255 时，Otsu 的最优解就是 0（阈值以下全黑、以上全白），
// 所以这里断言的是「分隔效果」而不是「阈值必须大于 0」。
func TestOtsuTwoTone(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			c := color.RGBA{0, 0, 0, 255}
			if x >= 10 {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	g := Gray(img)
	th := Otsu(g)
	if th < 0 || th >= 255 {
		t.Fatalf("otsu threshold %d outside [0,255)", th)
	}
	bright := 0
	for y := 0; y < g.Rect.Dy(); y++ {
		for x := 0; x < g.Rect.Dx(); x++ {
			if int(g.GrayAt(x, y).Y) > th {
				bright++
			}
		}
	}
	if bright != 100 {
		t.Errorf("threshold %d splits %d bright pixels, want 100 (half of 20x10)", th, bright)
	}
}

// TestDarkBackground 深色界面截图（暗底 + 少量亮字）判为暗底；浅底深字判为非暗底
func TestDarkBackground(t *testing.T) {
	// 暗底 + 左上角一小块亮字区域（模拟夜间模式截图）
	dark := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			c := color.RGBA{20, 20, 24, 255}
			if x < 30 && y < 30 {
				c = color.RGBA{240, 240, 240, 255}
			}
			dark.Set(x, y, c)
		}
	}
	if !DarkBackground(Gray(dark)) {
		t.Error("dark screenshot with light text should be reported as dark background")
	}

	// 白底 + 左上角一小块深字区域（常规文档截图）
	light := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			c := color.RGBA{250, 250, 250, 255}
			if x < 30 && y < 30 {
				c = color.RGBA{20, 20, 20, 255}
			}
			light.Set(x, y, c)
		}
	}
	if DarkBackground(Gray(light)) {
		t.Error("light screenshot with dark text should not be reported as dark background")
	}
}

// TestCandidatesSmallImage 小图（<720x520）应产出 raw + binary + scaled 三个候选
func TestCandidatesSmallImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 40))
	cands := Candidates(img)
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates for small image, got %d", len(cands))
	}
	names := map[string]bool{}
	for _, c := range cands {
		names[c.Name] = true
		if len(c.PNG) == 0 {
			t.Errorf("candidate %q has empty PNG", c.Name)
		}
	}
	for _, want := range []string{"raw", "binary", "scaled"} {
		if !names[want] {
			t.Errorf("missing candidate %q", want)
		}
	}
}

// TestCandidatesLargeImageSkipsScale 大图不放大（相对 pyc 的有意提速，见 FEATURES 15.4）
func TestCandidatesLargeImageSkipsScale(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	cands := Candidates(img)
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates for large image, got %d", len(cands))
	}
	for _, c := range cands {
		if c.Name == "scaled" {
			t.Error("large image should not be upscaled")
		}
	}
}

// TestScaleDoubles Scale(x,2) 尺寸翻倍
func TestScaleDoubles(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	got := Scale(img, 2)
	b := got.Bounds()
	if b.Dx() != 14 || b.Dy() != 10 {
		t.Errorf("scaled bounds = %dx%d, want 14x10", b.Dx(), b.Dy())
	}
}

// TestDecodeRejectsGarbage 非图片字节必须报错而不是返回空图
func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := Decode([]byte("not an image")); err == nil {
		t.Error("garbage bytes should fail to decode")
	}
}

// TestRoundTripPNG 编码后再解码，尺寸保持
func TestRoundTripPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 13, 9))
	raw, err := EncodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	b := back.Bounds()
	if b.Dx() != 13 || b.Dy() != 9 {
		t.Errorf("round-trip bounds = %dx%d, want 13x9", b.Dx(), b.Dy())
	}
}
