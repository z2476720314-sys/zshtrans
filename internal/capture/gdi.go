//go:build windows

package capture

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"unsafe"
)

// bitmapInfoHeader 对应 Win32 BITMAPINFOHEADER
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// dibSection 一块可直接读写的 32bpp 自顶向下 DIB
type dibSection struct {
	dc     uintptr
	bitmap uintptr
	old    uintptr
	bits   unsafe.Pointer
	w, h   int
}

// newDIBSection 创建 DIB 段并选入兼容 DC（比 CreateCompatibleBitmap + GetDIBits 稳）
func newDIBSection(screenDC uintptr, w, h int) (*dibSection, error) {
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, errors.New("create dc fail")
	}
	header := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(w),
		Height:      -int32(h), // 负值 = 自上而下
		Planes:      1,
		BitCount:    32,
		Compression: 0, // BI_RGB
	}
	var bits unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&header)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if bitmap == 0 || bits == nil {
		procDeleteDC.Call(memDC)
		return nil, errors.New("create dib section fail")
	}
	old, _, _ := procSelectObject.Call(memDC, bitmap)
	return &dibSection{dc: memDC, bitmap: bitmap, old: old, bits: bits, w: w, h: h}, nil
}

// close 还原并释放
func (d *dibSection) close() {
	if d.dc != 0 {
		procSelectObject.Call(d.dc, d.old)
	}
	if d.bitmap != 0 {
		procDeleteObject.Call(d.bitmap)
	}
	if d.dc != 0 {
		procDeleteDC.Call(d.dc)
	}
}

// png 把已 BitBlt 的像素编码成 PNG（BGRA → RGBA）
func (d *dibSection) png() ([]byte, error) {
	n := d.w * d.h * 4
	if n == 0 {
		return nil, errors.New("empty bitmap")
	}
	src := unsafe.Slice((*byte)(d.bits), n)
	img := image.NewRGBA(image.Rect(0, 0, d.w, d.h))
	for i := 0; i < n; i += 4 {
		img.Pix[i+0] = src[i+2]
		img.Pix[i+1] = src[i+1]
		img.Pix[i+2] = src[i+0]
		img.Pix[i+3] = 0xFF
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
