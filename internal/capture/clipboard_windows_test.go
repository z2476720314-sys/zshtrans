//go:build windows

package capture

import (
	"bytes"
	"errors"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

type fakeNativeCall func(...uintptr) (uintptr, uintptr, error)

func (f fakeNativeCall) Call(args ...uintptr) (uintptr, uintptr, error) { return f(args...) }

func withClipboardShim(t *testing.T) {
	open, close, empty, set, get, enum, seq := procOpenClipboard, procCloseClipboard, procEmptyClipboard, procSetClipboardData, procGetClipboardData, procEnumClipboardFormats, procGetClipboardSequenceNo
	t.Cleanup(func() {
		procOpenClipboard, procCloseClipboard, procEmptyClipboard, procSetClipboardData, procGetClipboardData, procEnumClipboardFormats, procGetClipboardSequenceNo = open, close, empty, set, get, enum, seq
	})
	procCloseClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
}

func TestWriteClipboardSuppliesLiveOwnerAndClosesBeforeDestroy(t *testing.T) {
	withClipboardShim(t)
	var owner uintptr
	procOpenClipboard = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		owner = args[0]
		valid, _, _ := user32.NewProc("IsWindow").Call(owner)
		if owner == 0 || valid == 0 {
			t.Fatal("clipboard has no live owner")
		}
		return 1, 0, syscall.Errno(0)
	})
	procEmptyClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
	procSetClipboardData = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		if args[0] != cfUnicodeText {
			t.Fatal("wrong text format")
		}
		ptr, _, _ := procGlobalLock.Call(args[1])
		got := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), 3)
		if got[0] != '测' || got[1] != '试' || got[2] != 0 {
			t.Fatal("bad UTF16 data")
		}
		procGlobalUnlock.Call(args[1])
		procGlobalFree.Call(args[1])
		return args[1], 0, syscall.Errno(0)
	})
	procCloseClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) {
		valid, _, _ := user32.NewProc("IsWindow").Call(owner)
		if valid == 0 {
			t.Fatal("destroyed owner before CloseClipboard")
		}
		return 1, 0, syscall.Errno(0)
	})
	if err := WriteClipboard("测试"); err != nil {
		t.Fatal(err)
	}
	valid, _, _ := user32.NewProc("IsWindow").Call(owner)
	if valid != 0 {
		t.Fatal("hidden owner leaked")
	}
}

func TestClipboardEmptyFailureNeverTransfersData(t *testing.T) {
	withClipboardShim(t)
	procOpenClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
	procEmptyClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 0, 0, errors.New("blocked") })
	procSetClipboardData = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) {
		t.Fatal("SetClipboardData after EmptyClipboard failed")
		return 0, 0, syscall.Errno(0)
	})
	if err := WriteClipboard("safe"); err == nil {
		t.Fatal("ignored EmptyClipboard failure")
	}
}

func TestClipboardSnapshotKeepsRichFormatsAndRejectsOpaqueObjects(t *testing.T) {
	withClipboardShim(t)
	formats := []uint32{13, 0xC001, 0xC002}
	data := [][]byte{{'A', 0, 0, 0}, []byte("<b>A</b>"), []byte("{\\rtf1 A}")}
	handles := make([]uintptr, len(data))
	for i, b := range data {
		h, err := allocateClipboardData(b)
		if err != nil {
			t.Fatal(err)
		}
		handles[i] = h
		t.Cleanup(func() { procGlobalFree.Call(h) })
	}
	procOpenClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
	procGetClipboardSequenceNo = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 77, 0, syscall.Errno(0) })
	procEnumClipboardFormats = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		if args[0] == 0 {
			return uintptr(formats[0]), 0, syscall.Errno(0)
		}
		for i, f := range formats {
			if uintptr(f) == args[0] && i+1 < len(formats) {
				return uintptr(formats[i+1]), 0, syscall.Errno(0)
			}
		}
		return 0, 0, syscall.Errno(0)
	})
	procGetClipboardData = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		for i, f := range formats {
			if uintptr(f) == args[0] {
				return handles[i], 0, syscall.Errno(0)
			}
		}
		return 0, 0, syscall.Errno(0)
	})
	snapshot, err := snapshotClipboard()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.sequence != 77 || len(snapshot.items) != 3 {
		t.Fatalf("incomplete snapshot: %+v", snapshot)
	}
	for i, item := range snapshot.items {
		if item.format != formats[i] || !bytes.Equal(item.data[:len(data[i])], data[i]) {
			t.Fatal("lost rich format")
		}
	}
	transferred := 0
	procEmptyClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
	procSetClipboardData = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		i := transferred
		transferred++
		if i >= len(formats) || uint32(args[0]) != formats[i] {
			t.Fatal("restore changed formats or order")
		}
		ptr, _, _ := procGlobalLock.Call(args[1])
		if !bytes.Equal(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data[i])), data[i]) {
			t.Fatal("restore lost formatted content")
		}
		procGlobalUnlock.Call(args[1])
		procGlobalFree.Call(args[1])
		return args[1], 0, syscall.Errno(0)
	})
	if err := writeClipboardFormats(snapshot.items, &snapshot.sequence); err != nil {
		t.Fatal(err)
	}
	if transferred != 3 {
		t.Fatal("did not restore every format")
	}
	formats = []uint32{2}
	if _, err := snapshotClipboard(); err == nil {
		t.Fatal("accepted non-copyable bitmap handle")
	}
}

func TestClipboardRestoreDoesNotOverwriteNewCopy(t *testing.T) {
	withClipboardShim(t)
	procOpenClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
	procGetClipboardSequenceNo = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 88, 0, syscall.Errno(0) })
	procEmptyClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) {
		t.Fatal("overwrote newer user clipboard")
		return 0, 0, syscall.Errno(0)
	})
	expected := uint32(77)
	if err := writeClipboardFormats([]clipboardItem{{format: 13, data: []byte{'A', 0, 0, 0}}}, &expected); !errors.Is(err, errClipboardChanged) {
		t.Fatalf("expected changed clipboard, got %v", err)
	}
}

func TestWriteClipboardWaitsForSelectionTransaction(t *testing.T) {
	withClipboardShim(t)
	opened := make(chan struct{}, 1)
	procOpenClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { opened <- struct{}{}; return 1, 0, syscall.Errno(0) })
	procEmptyClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
	procSetClipboardData = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		procGlobalFree.Call(args[1])
		return args[1], 0, syscall.Errno(0)
	})
	selectionMu.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() { close(started); done <- WriteClipboard("previous translation") }()
	<-started
	interleaved := false
	select {
	case <-opened:
		interleaved = true
	case <-time.After(50 * time.Millisecond):
	}
	selectionMu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if interleaved {
		t.Fatal("result copy entered clipboard during selection transaction")
	}
}

func TestClipboardReadReturnsPostRenderSequence(t *testing.T) {
	withClipboardShim(t)
	handle, err := allocateClipboardData([]byte{'A', 0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	defer procGlobalFree.Call(handle)
	sequence := uintptr(55)
	procOpenClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, syscall.Errno(0) })
	procGetClipboardSequenceNo = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return sequence, 0, syscall.Errno(0) })
	procGetClipboardData = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { sequence = 56; return handle, 0, syscall.Errno(0) })
	text, observed, err := readClipboardTextWithSequence()
	if err != nil {
		t.Fatal(err)
	}
	if text != "A" || observed != 56 {
		t.Fatalf("delayed rendered result = %q, sequence=%d", text, observed)
	}
}
