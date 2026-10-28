//go:build windows

package capture

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
	"unsafe"
)

func TestSelectedTextRestoresPreviousImage(t *testing.T) {
	dib := make([]byte, 44)
	binary.LittleEndian.PutUint32(dib, 40)
	binary.LittleEndian.PutUint32(dib[4:], 1)
	binary.LittleEndian.PutUint32(dib[8:], 1)
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	copy(dib[40:], []byte{0x31, 0x62, 0x93, 0x80})
	v5 := make([]byte, 128)
	copy(v5, dib[:40])
	binary.LittleEndian.PutUint32(v5, 124)
	copy(v5[124:], dib[40:])
	for _, sample := range []struct {
		name    string
		formats []uint32
		images  map[uint32][]byte
	}{
		{"bitmap with image formats", []uint32{2, 17, 8, 9, 0xC001}, map[uint32][]byte{17: v5, 8: dib, 0xC001: []byte("registered PNG content")}},
		{"bitmap converted to DIBV5", []uint32{2}, map[uint32][]byte{17: v5}},
		{"bitmap converted to DIB", []uint32{2}, map[uint32][]byte{8: dib}},
		{"DIB with synthesized palette", []uint32{8, 9}, map[uint32][]byte{8: dib}},
		{"DIB alone", []uint32{8}, map[uint32][]byte{8: dib}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			sequence, requested := selectionClipboard(t, "Выбранный текст")
			readText := procGetClipboardData
			handles := map[uint32]uintptr{}
			for format, data := range sample.images {
				handle, err := allocateClipboardData(data)
				if err != nil {
					t.Fatal(err)
				}
				handles[format] = handle
				t.Cleanup(func() { procGlobalFree.Call(handle) })
			}
			procEnumClipboardFormats = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
				if args[0] == 0 {
					return uintptr(sample.formats[0]), 0, nil
				}
				for i, format := range sample.formats {
					if uintptr(format) == args[0] && i+1 < len(sample.formats) {
						return uintptr(sample.formats[i+1]), 0, nil
					}
				}
				return 0, 0, nil
			})
			procGetClipboardData = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
				if args[0] == cfUnicodeText && *requested {
					*sequence = 56
					return readText.Call(args...)
				}
				return handles[uint32(args[0])], 0, nil
			})
			restored := map[uint32][]byte{}
			procSetClipboardData = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
				format := uint32(args[0])
				if format == 2 || format == 9 {
					t.Fatal("restoring an object format using an HGLOBAL handle")
				}
				data, ok := sample.images[format]
				if !ok {
					t.Fatalf("unexpected restored format %d", format)
				}
				ptr, _, _ := procGlobalLock.Call(args[1])
				restored[format] = append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data))...)
				procGlobalUnlock.Call(args[1])
				procGlobalFree.Call(args[1])
				return args[1], 0, nil
			})
			text, err := SelectedText(90*time.Millisecond, "alt+q")
			if err != nil || text != "Выбранный текст" {
				t.Fatalf("image blocked selection: text=%q err=%v", text, err)
			}
			for format, data := range sample.images {
				if !bytes.Equal(restored[format], data) {
					t.Fatalf("image format %d was not restored intact", format)
				}
			}
		})
	}
}
