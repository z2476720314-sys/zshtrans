package ocr

import (
	"context"
	"errors"
	"image"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancelOCRStopsCandidateFallback(t *testing.T) {
	resetTestCache(t)
	started := make(chan struct{})
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := RunContext(ctx, image.NewRGBA(image.Rect(0, 0, 20, 12)), Keys{VisionTag: "cancel", VisionContext: func(ctx context.Context, _ string) (string, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-ctx.Done()
			return "", ctx.Err()
		}})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
			t.Fatalf("OCR cancel = %v, calls=%d", err, calls.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("OCR ignored cancellation")
	}
}

func resetTestCache(t *testing.T) {
	previous := ocrCache
	ocrCache = &cache{items: map[string]string{}, cap: 60}
	t.Cleanup(func() { ocrCache = previous })
}

func TestOCRStopsAfterSuccessfulOriginalAndReusesResult(t *testing.T) {
	resetTestCache(t)
	var calls atomic.Int32
	keys := Keys{VisionTag: "model-a", Vision: func(string) (string, error) { calls.Add(1); return "Привет", nil }}
	img := image.NewRGBA(image.Rect(0, 0, 20, 12))
	for i := 0; i < 2; i++ {
		r, err := Run(img, keys)
		if err != nil || r.Text != "Привет" {
			t.Fatalf("OCR = %+v, %v", r, err)
		}
		if calls.Load() != 1 {
			t.Fatalf("OCR sent %d requests; one successful original should cover repeated capture", calls.Load())
		}
	}
}

func TestOCRTriesNextCandidateOnlyAfterFailure(t *testing.T) {
	resetTestCache(t)
	var calls atomic.Int32
	img := image.NewRGBA(image.Rect(0, 0, 20, 12))
	raw, err := EncodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Run(img, Keys{VisionTag: "model-a", Vision: func(b64 string) (string, error) {
		calls.Add(1)
		if b64 == B64(raw) {
			return "", errors.New("unreadable original")
		}
		return "Привет", nil
	}})
	if err != nil || r.Text != "Привет" || calls.Load() != 2 {
		t.Fatalf("OCR fallback = %+v, %v, calls=%d", r, err, calls.Load())
	}
}

func TestOCRCacheRespectsLanguageAndModel(t *testing.T) {
	resetTestCache(t)
	img := image.NewRGBA(image.Rect(0, 0, 20, 12))
	for _, tc := range []struct{ lang, model, text string }{{"ru", "model-a", "Первый"}, {"en", "model-a", "Second"}, {"en", "model-b", "Third"}} {
		r, err := Run(img, Keys{SrcLang: tc.lang, VisionTag: tc.model, Vision: func(string) (string, error) { return tc.text, nil }})
		if err != nil || r.Text != tc.text {
			t.Fatalf("OCR for %s/%s = %+v, %v", tc.lang, tc.model, r, err)
		}
	}
}

func TestOCRCacheIsolatesCapturedVisionConfiguration(t *testing.T) {
	resetTestCache(t)
	img := image.NewRGBA(image.Rect(0, 0, 20, 12))
	oldKeys := Keys{VisionTag: "same-model", VisionScope: "old-provider-model-credential", Vision: func(string) (string, error) { return "旧结果", nil }}
	newKeys := Keys{VisionTag: "same-model", VisionScope: "new-provider-model-credential", Vision: func(string) (string, error) { return "新结果", nil }}
	// 旧请求的keys在Reset前捕获，但编码结束进入缓存逻辑时已是新代际。
	ResetCache()
	if _, err := Run(img, oldKeys); err != nil {
		t.Fatal(err)
	}
	r, err := Run(img, newKeys)
	if err != nil || r.Text != "新结果" || r.Engine == "cache" {
		t.Fatalf("old configuration contaminated cache: %+v,%v", r, err)
	}
}

func TestOCRResetIsolatesInFlightResult(t *testing.T) {
	resetTestCache(t)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	img := image.NewRGBA(image.Rect(0, 0, 20, 12))
	go func() {
		defer close(done)
		_, _ = Run(img, Keys{VisionTag: "same-model", Vision: func(string) (string, error) { close(started); <-release; return "旧结果", nil }})
	}()
	<-started
	ResetCache()
	close(release)
	<-done
	r, err := Run(img, Keys{VisionTag: "same-model", Vision: func(string) (string, error) { return "新结果", nil }})
	if err != nil || r.Text != "新结果" || r.Engine == "cache" {
		t.Fatalf("old OCR request polluted reset cache: %+v,%v", r, err)
	}
}
