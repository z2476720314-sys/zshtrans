//go:build windows

package capture

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"unsafe"
)

func TestNativeCallbackCopiesBorrowedMemory(t *testing.T) {
	s := &ipcSession{events: make(chan nativeEvent, 4)}
	command := append([]byte("onScreenShotRequestOCR"), 0)
	payload := []byte("image.png")
	s.onMessage(0, uintptr(unsafe.Pointer(&command[0])), 0, uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)))
	payload[0] = 'x'
	event := <-s.events
	if event.Name != "onScreenShotRequestOCR" || string(event.Payload) != "image.png" {
		t.Fatalf("borrowed memory escaped: %+v", event)
	}
	s.onMessage(0, uintptr(unsafe.Pointer(&command[0])), 0, 0, 1)
	if (<-s.events).Name != "IPC_PAYLOAD_ERROR" {
		t.Fatal("accepted null payload")
	}
	s.generation.Store(1)
	s.onMessage(0, uintptr(unsafe.Pointer(&command[0])), 0, 0, 0)
	if len(s.events) != 0 {
		t.Fatal("accepted callback from previous capture process")
	}
}

func TestNativeCallbackReadsSigned32BitPayloadSize(t *testing.T) {
	s := &ipcSession{events: make(chan nativeEvent, 2)}
	command := append([]byte("onScreenShotRequestTranslate"), 0)
	payload := []byte("image.png")
	// The native ABI defines size as int; the upper half of its x64 slot is unspecified.
	size := uint64(0xa5a5a5a500000000) | uint64(len(payload))
	s.onMessage(0, uintptr(unsafe.Pointer(&command[0])), 0, uintptr(unsafe.Pointer(&payload[0])), uintptr(size))
	event := <-s.events
	if event.Name != "onScreenShotRequestTranslate" || string(event.Payload) != "image.png" {
		t.Fatalf("valid native int size was rejected: %+v", event)
	}
	s.onMessage(0, uintptr(unsafe.Pointer(&command[0])), 0, 0, uintptr(uint32(0xffffffff)))
	if (<-s.events).Name != "IPC_PAYLOAD_ERROR" {
		t.Fatal("negative native int size was accepted")
	}
}

func TestSendInputUsesWindowsABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(nativeInput{}) != 40 || unsafe.Offsetof(nativeInput{}.Data) != 8) {
		t.Fatal("SendInput requires a 40-byte aligned INPUT structure on x64")
	}
}

func TestNativeDLLSmoke(t *testing.T) {
	if os.Getenv("ZSHTRANS_NATIVE_SMOKE") != "1" {
		t.Skip("explicit DLL-only smoke")
	}
	root := os.Getenv("ZSHTRANS_NATIVE_SMOKE_ROOT")
	if root == "" {
		var err error
		root, err = EnsureRuntime(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestNativeDLLSmoke$", "-test.v")
		child.Env = append(os.Environ(), "ZSHTRANS_NATIVE_SMOKE_ROOT="+root)
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("DLL smoke subprocess: %v\n%s", err, output)
		}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Log("runtime extracted and verified; creating IPC host")
	s, err := newIPCSession(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.pid != 0 || s.parent == 0 {
		t.Fatal("invalid idle IPC host")
	}
	t.Log("IPC host created without launching a child; releasing host")
	if err := s.shutdown(); err != nil {
		t.Fatal(err)
	}
	t.Log("IPC host released")
}
