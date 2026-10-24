package ocr

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// Candidate 一个预处理候选图（对应 pyc _ocr_candidates）
type Candidate struct {
	Name  string
	PNG   []byte
	Scale float64 // 相对原图的放大倍数（用于失败诊断，原图/二值图为 1）
}

// Decode 解码 PNG/JPEG 为 RGBA
func Decode(raw []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// EncodePNG 编码为 PNG
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Gray 转灰度矩阵（0-255）
func Gray(img image.Image) *image.Gray {
	b := img.Bounds()
	g := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(g, g.Bounds(), img, b.Min, draw.Src)
	return g
}

// Otsu 计算最佳二值化阈值（对应 pyc _otsu_threshold）
func Otsu(g *image.Gray) int {
	var hist [256]int
	total := 0
	for y := 0; y < g.Rect.Dy(); y++ {
		for x := 0; x < g.Rect.Dx(); x++ {
			hist[g.GrayAt(x, y).Y]++
			total++
		}
	}
	if total == 0 {
		return 128
	}
	sum := 0.0
	for i := 0; i < 256; i++ {
		sum += float64(i) * float64(hist[i])
	}
	sumB, wB, wF := 0.0, 0, 0
	maxVar, threshold := -1.0, 128
	for t := 0; t < 256; t++ {
		wB += hist[t]
		if wB == 0 {
			continue
		}
		wF = total - wB
		if wF == 0 {
			break
		}
		sumB += float64(t) * float64(hist[t])
		mB := sumB / float64(wB)
		mF := (sum - sumB) / float64(wF)
		v := float64(wB) * float64(wF) * (mB - mF) * (mB - mF)
		if v > maxVar {
			maxVar = v
			threshold = t
		}
	}
	return threshold
}

// DarkBackground 判断是否为暗底亮字（深色界面截图 / 夜间模式）
// 依据：Otsu 阈值两侧的像素占比，若超过阈值的一侧明显更少，说明亮像素是前景文字，背景是暗的。
func DarkBackground(g *image.Gray) bool {
	t := Otsu(g)
	b := g.Bounds()
	total, bright := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			total++
			if int(g.GrayAt(x, y).Y) > t {
				bright++
			}
		}
	}
	if total == 0 {
		return false
	}
	// 文字像素通常占 2%~45%；亮像素少于这个区间说明亮色是前景（暗底）
	ratio := float64(bright) / float64(total)
	return ratio > 0 && ratio < 0.5
}

// BinaryWhiteBg 二值化并铺白底（对应 pyc _binary_white_bg）
// 与原版差异：原版固定「亮→白、暗→黑」，遇到深色界面截图会得到黑底白字，
// 引擎对大块暗底的识别率明显更差。这里按极性自适应，始终输出白底黑字。
func BinaryWhiteBg(img image.Image) image.Image {
	g := Gray(img)
	t := Otsu(g)
	invert := DarkBackground(g)
	b := g.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			lit := int(g.GrayAt(x, y).Y) > t
			if invert {
				lit = !lit
			}
			if lit {
				out.Set(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				out.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	return out
}

// Scale 双线性放大（对应 pyc 放大候选）。用双线性而非最近邻：
// 小字放大后边缘更平滑，OCR 引擎的字形匹配明显更准。
func Scale(img image.Image, factor int) image.Image {
	if factor <= 1 {
		return img
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	w, h := sw*factor, sh*factor
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	if src, ok := img.(*image.RGBA); ok && b.Min.X == 0 && b.Min.Y == 0 {
		scaleRGBA(src, out, factor)
		return out
	}
	for y := 0; y < h; y++ {
		sy := float64(y)/float64(factor) + float64(b.Min.Y)
		y0 := int(sy)
		y1 := y0 + 1
		if y1 >= b.Max.Y {
			y1 = b.Max.Y - 1
		}
		fy := sy - float64(y0)
		for x := 0; x < w; x++ {
			sx := float64(x)/float64(factor) + float64(b.Min.X)
			x0 := int(sx)
			x1 := x0 + 1
			if x1 >= b.Max.X {
				x1 = b.Max.X - 1
			}
			fx := sx - float64(x0)
			r00, g00, b00, _ := img.At(x0, y0).RGBA()
			r10, g10, b10, _ := img.At(x1, y0).RGBA()
			r01, g01, b01, _ := img.At(x0, y1).RGBA()
			r11, g11, b11, _ := img.At(x1, y1).RGBA()
			mix := func(v00, v10, v01, v11 uint32) uint8 {
				top := float64(v00)*(1-fx) + float64(v10)*fx
				bot := float64(v01)*(1-fx) + float64(v11)*fx
				v := top*(1-fy) + bot*fy
				return uint8(v / 257)
			}
			out.Set(x, y, color.RGBA{mix(r00, r10, r01, r11), mix(g00, g10, g01, g11), mix(b00, b10, b01, b11), 255})
		}
	}
	return out
}

// Candidates 生成候选集：原图 / 二值化白底 / 放大
func Candidates(img image.Image) []Candidate {
	out := make([]Candidate, 0, 3)
	if raw, err := EncodePNG(img); err == nil {
		out = append(out, Candidate{Name: "raw", PNG: raw, Scale: 1})
	}
	if raw, err := EncodePNG(BinaryWhiteBg(img)); err == nil {
		out = append(out, Candidate{Name: "binary", PNG: raw, Scale: 1})
	}
	// 只在原图偏小时才加放大候选：大图放大 4 倍像素既慢、OCR 侧也会被压缩回去，
	// 收益接近零（原 pyc 无脑加，是速度上的主要开销）。
	b := img.Bounds()
	if b.Dx() < 720 && b.Dy() < 520 {
		if raw, err := EncodePNG(Scale(img, 2)); err == nil {
			out = append(out, Candidate{Name: "scaled", PNG: raw, Scale: 2})
		}
	}
	if len(out) == 0 {
		// 极端情况下退化为原字节
		if raw, err := EncodePNG(img); err == nil {
			out = append(out, Candidate{Name: "raw", PNG: raw, Scale: 1})
		}
	}
	return out
}

// scaleRGBA *image.RGBA 的双线性放大快路径（直接读 Pix，比 At() 快一个量级）
func scaleRGBA(src *image.RGBA, dst *image.RGBA, factor int) {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	inv := 1.0 / float64(factor)
	for y := 0; y < dh; y++ {
		sy := float64(y) * inv
		y0 := int(sy)
		if y0 > sh-1 {
			y0 = sh - 1
		}
		y1 := y0 + 1
		if y1 > sh-1 {
			y1 = sh - 1
		}
		fy := sy - float64(y0)
		r0 := src.Pix[y0*src.Stride:]
		r1 := src.Pix[y1*src.Stride:]
		drow := dst.Pix[y*dst.Stride:]
		for x := 0; x < dw; x++ {
			sx := float64(x) * inv
			x0 := int(sx)
			if x0 > sw-1 {
				x0 = sw - 1
			}
			x1 := x0 + 1
			if x1 > sw-1 {
				x1 = sw - 1
			}
			fx := sx - float64(x0)
			o0, o1 := x0*4, x1*4
			p := dst.Pix[x*4 : x*4+4]
			for c := 0; c < 3; c++ {
				top := float64(r0[o0+c])*(1-fx) + float64(r0[o1+c])*fx
				bot := float64(r1[o0+c])*(1-fx) + float64(r1[o1+c])*fx
				p[c] = uint8(top*(1-fy) + bot*fy)
			}
			p[3] = 255
			_ = drow
		}
	}
}
