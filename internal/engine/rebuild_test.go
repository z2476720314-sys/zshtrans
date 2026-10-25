package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/local/zshtrans/internal/providers"
)

func TestCanceledTranslationDoesNotFailoverOrMarkUnhealthy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := providers.NewProvider("first", func(ctx context.Context, _, _, _ string) (string, error) { cancel(); return "", ctx.Err() })
	second := mkProvider("second", func(_, _, _ string) (string, error) {
		t.Error("started fallback after cancellation")
		return "wrong", nil
	})
	e := New([]*providers.Provider{first, second}, "first", 8)
	if _, err := e.TranslateContext(ctx, "Привет", "ru", "zh"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	if first.Fails() != 0 || e.Len() != 0 {
		t.Fatal("caller cancellation changed provider health or cached a result")
	}
}

func TestRebuildDoesNotReusePriorModelTranslation(t *testing.T) {
	e := New([]*providers.Provider{mkProvider("old", ok("旧译文"))}, "old", 8)
	if _, err := e.Translate("Привет", "ru", "zh"); err != nil {
		t.Fatal(err)
	}
	e.Rebuild([]*providers.Provider{mkProvider("new", ok("新译文"))}, "new", 8)
	r, err := e.Translate("Привет", "ru", "zh")
	if err != nil || r.Text != "新译文" || r.Provider != "new" {
		t.Fatalf("translation after rebuild = %+v, %v", r, err)
	}
}

func TestRebuildIsolatesInFlightResult(t *testing.T) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	e := New([]*providers.Provider{mkProvider("old", func(_, _, _ string) (string, error) {
		close(started)
		<-release
		return "旧译文", nil
	})}, "old", 8)
	go func() { defer close(done); _, _ = e.Translate("Привет", "ru", "zh") }()
	<-started
	e.Rebuild([]*providers.Provider{mkProvider("new", ok("新译文"))}, "new", 8)
	close(release)
	<-done
	r, err := e.Translate("Привет", "ru", "zh")
	if err != nil || r.Text != "新译文" || r.Provider != "new" {
		t.Fatalf("old request contaminated new cache: %+v, %v", r, err)
	}
}
