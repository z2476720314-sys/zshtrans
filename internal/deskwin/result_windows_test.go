//go:build windows

package deskwin

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

func TestResultMeasurementWrapsLongIdentifiers(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	height := int32(-14)
	font, _, _ := resultCreateFont.Call(uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(resultUTF16("Segoe UI"))))
	defer resultDeleteObject.Call(font)
	one := resultMeasureText("example", 120, font)
	long := resultMeasureText(strings.Repeat("ABCDEFGHIJ", 40), 120, font)
	if one <= 0 || long <= one*4 {
		t.Fatal("long unbroken model/product text must wrap instead of being clipped")
	}
}

func TestResultDrawItemMatchesWindowsABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(resultDrawItem{}) != 64 || unsafe.Offsetof(resultDrawItem{}.DC) != 32 || unsafe.Offsetof(resultDrawItem{}.Rect) != 40) {
		t.Fatal("DRAWITEMSTRUCT layout is incompatible with Windows x64")
	}
}
