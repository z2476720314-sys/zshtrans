package capture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestManifestRejectsTraversalAndTampering(t *testing.T) {
	base := t.TempDir()
	data := []byte("runtime bytes")
	digest := sha256.Sum256(data)
	manifest := nativeManifest{Version: qqVersion, Files: map[string]string{"native/test.dll": hex.EncodeToString(digest[:])}}
	if err := os.MkdirAll(filepath.Join(base, "native"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "native", "test.dll")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateManifestFiles(base, manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateManifestFiles(base, manifest); err == nil {
		t.Fatal("accepted changed runtime")
	}
	for _, bad := range []string{"../outside", "native/../../outside", "C:/outside", "\\\\server\\share", "/outside"} {
		manifest.Files = map[string]string{bad: hex.EncodeToString(digest[:])}
		if err := validateManifestFiles(base, manifest); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestCaptureImageRequiresLocalPathAndNormalizesPNG(t *testing.T) {
	for _, path := range []string{"", "relative.png", "https://host/image.png", "bad\x00path"} {
		if _, err := readCaptureImage([]byte(path)); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	path := filepath.Join(t.TempDir(), "截图.png")
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 13, 7))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := readCaptureImage(append([]byte(path), 0))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil || decoded.Bounds().Dx() != 13 {
		t.Fatalf("bad normalized image: %v", err)
	}
}

func TestCancelBeforeTranslateKeepsDelayedImage(t *testing.T) {
	start := time.Unix(100, 0)
	var state captureState
	state.accept(nativeEvent{Name: "onScreenShotCancel"}, start)
	if state.ready(start.Add(time.Second)) {
		t.Fatal("closed before delayed translation callback")
	}
	state.accept(nativeEvent{Name: "onScreenShotRequestTranslate", Payload: []byte("image.png")}, start.Add(time.Second))
	state.accept(nativeEvent{Name: "onScreenShotComplete"}, start.Add(1100*time.Millisecond))
	if state.Action != "translate" || string(state.Payload) != "image.png" {
		t.Fatalf("lost image: %+v", state)
	}
	if !state.ready(start.Add(2500 * time.Millisecond)) {
		t.Fatal("did not finish after idle grace")
	}
}

func TestNativeCompleteNeverBecomesOCR(t *testing.T) {
	for _, name := range []string{"onScreenShotComplete", "onScreenShotLongCaptureOk", "onScreenShotLongCaptureSave", "onScreenShotLongCaptureCollect"} {
		var state captureState
		state.accept(nativeEvent{Name: name, Payload: []byte("image.png")}, time.Unix(100, 0))
		if state.Action != "complete" || len(state.Payload) != 0 {
			t.Fatalf("%s submitted image: %+v", name, state)
		}
	}
	var state captureState
	state.accept(nativeEvent{Name: "onScreenShotRequestOCR", Payload: []byte("image.png")}, time.Unix(100, 0))
	if state.Action != "ocr" {
		t.Fatalf("OCR became %q", state.Action)
	}
}

func TestToolbarPatchRejectsUnknownInstruction(t *testing.T) {
	for _, b := range [][]byte{{0x48, 0x8d, 0x4d, 0xe0}, {0x48, 0x8d, 0x4d, 0xe8}} {
		got, err := repairedInstruction(b)
		if err != nil || !bytes.Equal(got, []byte{0x48, 0x8d, 0x4d, 0xe8}) {
			t.Fatalf("patch: %x %v", got, err)
		}
	}
	if _, err := repairedInstruction([]byte{0x48, 0x8d, 0x4d, 0xe4}); err == nil {
		t.Fatal("accepted unknown instruction")
	}
}

func TestSelectionCopyWaitsForHotkeyRelease(t *testing.T) {
	var pressed atomic.Bool
	pressed.Store(true)
	var copied atomic.Bool
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { done <- copyWhenReleased(ctx, pressed.Load, func() error { copied.Store(true); return nil }) }()
	time.Sleep(30 * time.Millisecond)
	if copied.Load() {
		t.Fatal("copied while Alt/Control still pressed")
	}
	pressed.Store(false)
	if err := <-done; err != nil || !copied.Load() {
		t.Fatalf("copy after release: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := copyWhenReleased(ctx, func() bool { return true }, func() error { t.Fatal("copied after timeout"); return nil }); err == nil {
		t.Fatal("missing release timeout")
	}
}
