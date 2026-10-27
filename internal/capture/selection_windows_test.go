//go:build windows

package capture

import (
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

// All clipboard and input calls stay behind the Win32 boundary; no desktop keys are sent.
func selectionClipboard(t *testing.T, text string) (*uintptr, *bool) {
	t.Helper()
	withClipboardShim(t)
	input, async, foreground := procSendInput, procGetAsyncKeyState, procGetForegroundWindow
	t.Cleanup(func() { procSendInput, procGetAsyncKeyState, procGetForegroundWindow = input, async, foreground })
	procGetForegroundWindow = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 123, 0, nil })
	sequence, requested := uintptr(55), false
	procGetAsyncKeyState = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil })
	procSendInput = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) { requested = true; return args[0], 0, nil })
	procGetClipboardSequenceNo = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return sequence, 0, nil })
	procEnumClipboardFormats = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil })
	procOpenClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, nil })
	procEmptyClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return 1, 0, nil })
	var data []byte
	for _, r := range utf16.Encode([]rune(text + "\x00")) {
		data = append(data, byte(r), byte(r>>8))
	}
	handle, err := allocateClipboardData(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { procGlobalFree.Call(handle) })
	procGetClipboardData = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return handle, 0, nil })
	return &sequence, &requested
}

func TestSelectedTextRendersPendingClipboardBeforeCheckingSequence(t *testing.T) {
	sequence, requested := selectionClipboard(t, "Заказ")
	get := procGetClipboardData
	procGetClipboardData = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		if *requested {
			*sequence = 56
		}
		return get.Call(args...)
	})
	got, err := SelectedText(90*time.Millisecond, "")
	if err != nil || got != "Заказ" {
		t.Fatalf("pending selected text was never rendered: got=%q err=%v", got, err)
	}
}

func TestSelectedTextNeverUsesUnchangedClipboard(t *testing.T) {
	selectionClipboard(t, "stale clipboard")
	procEmptyClipboard = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) {
		t.Fatal("unchanged clipboard was overwritten")
		return 0, 0, nil
	})
	got, err := SelectedText(90*time.Millisecond, "")
	if got != "" || err == nil || !strings.Contains(err.Error(), "selection") {
		t.Fatalf("stale text was accepted: %q %v", got, err)
	}
}

func TestHotkeyCopyNeutralizesTriggerBeforeAltRelease(t *testing.T) {
	selectionClipboard(t, "example")
	var events []nativeKeyboardInput
	procSendInput = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		for _, input := range unsafe.Slice((*nativeInput)(unsafe.Pointer(args[1])), int(args[0])) {
			events = append(events, *(*nativeKeyboardInput)(unsafe.Pointer(&input.Data[0])))
		}
		return args[0], 0, nil
	})
	if err := sendCopyInput("alt+q"); err != nil {
		t.Fatal(err)
	}
	want := []nativeKeyboardInput{{VK: 'Q', Flags: keyeventfKeyup}, {VK: vkControl}, {VK: vkMenu, Flags: keyeventfKeyup}, {VK: 'C'}, {VK: 'C', Flags: keyeventfKeyup}, {VK: vkControl, Flags: keyeventfKeyup}}
	if len(events) != len(want) {
		t.Fatalf("copy event count=%d, want %d", len(events), len(want))
	}
	for i, event := range events {
		if event.VK != want[i].VK || event.Flags != want[i].Flags {
			t.Fatalf("copy event %d=%+v, want %+v", i, event, want[i])
		}
	}
}

func TestSelectedTextRetriesCopyOnceWithoutUsingStaleText(t *testing.T) {
	sequence, _ := selectionClipboard(t, "fresh selection")
	copies := 0
	procSendInput = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		copies++
		if copies == 2 {
			*sequence = 56
		}
		return args[0], 0, nil
	})
	got, err := SelectedText(210*time.Millisecond, "alt+q")
	if err != nil || got != "fresh selection" || copies != 2 {
		t.Fatalf("copy retry: text=%q copies=%d err=%v", got, copies, err)
	}
}

func TestSelectedTextStopsWhenSourceLosesFocus(t *testing.T) {
	sequence, _ := selectionClipboard(t, "other window text")
	foreground := uintptr(123)
	procGetForegroundWindow = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { return foreground, 0, nil })
	procSendInput = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		*sequence = 56
		foreground = 456
		return args[0], 0, nil
	})
	got, err := SelectedText(90*time.Millisecond, "alt+q")
	if got != "" || err == nil {
		t.Fatalf("accepted another foreground window's text: %q %v", got, err)
	}
}

func TestHotkeyCopyReleasesKeysAfterPartialSend(t *testing.T) {
	selectionClipboard(t, "example")
	calls := 0
	procSendInput = fakeNativeCall(func(args ...uintptr) (uintptr, uintptr, error) {
		calls++
		if calls == 1 {
			return args[0] - 1, 0, syscall.Errno(5)
		}
		released := map[uint16]bool{}
		for _, input := range unsafe.Slice((*nativeInput)(unsafe.Pointer(args[1])), int(args[0])) {
			key := *(*nativeKeyboardInput)(unsafe.Pointer(&input.Data[0]))
			if key.Flags != keyeventfKeyup {
				t.Fatal("partial-send cleanup pressed a key")
			}
			released[key.VK] = true
		}
		for _, key := range []uint16{'C', vkControl, 'Q', vkMenu} {
			if !released[key] {
				t.Fatalf("cleanup left key %d held", key)
			}
		}
		return args[0], 0, nil
	})
	if err := sendCopyInput("alt+q"); err == nil || !strings.Contains(err.Error(), "5/6") {
		t.Fatalf("partial input was reported as successful: %v", err)
	}
	if calls != 2 {
		t.Fatalf("cleanup calls=%d, want 2", calls)
	}
}

func TestHotkeyCopyRejectsInvalidBindingBeforeInput(t *testing.T) {
	selectionClipboard(t, "example")
	procSendInput = fakeNativeCall(func(...uintptr) (uintptr, uintptr, error) { t.Fatal("invalid hotkey injected input"); return 0, 0, nil })
	if err := sendCopyInput("bogus"); err == nil {
		t.Fatal("invalid hotkey was accepted")
	}
}
