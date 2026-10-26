package server

import (
	"context"
	"errors"
	"testing"

	"github.com/local/zshtrans/internal/capture"
	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/engine"
	"github.com/local/zshtrans/internal/providers"
)

func TestProcessCaptureRequiresExplicitImageRequest(t *testing.T) {
	for _, action := range []string{"", "unknown", "copy", "save", "long-complete", "complete", "cancel"} {
		t.Run(action, func(t *testing.T) {
			out, err := processCapture(context.Background(), nil, capture.CaptureResult{Action: action, PNG: []byte("not an image")})
			if err != nil {
				t.Fatalf("ordinary capture entered image processing: %v", err)
			}
			want := "complete"
			if action == "cancel" {
				want = "cancel"
			}
			if out.Action != want || out.Source != "" || out.Text != "" || out.Provider != "" {
				t.Fatalf("ordinary capture produced a popup: %+v", out)
			}
		})
	}
	for _, action := range []string{"ocr", "translate"} {
		if _, err := processCapture(context.Background(), nil, capture.CaptureResult{Action: action, PNG: []byte("not an image")}); err == nil {
			t.Fatalf("explicit %s request skipped image validation", action)
		}
	}
}

func TestPopupTranslationFailureIsAnError(t *testing.T) {
	p := providers.NewProvider("offline", func(context.Context, string, string, string) (string, error) {
		return "", errors.New("upstream unavailable")
	})
	a := &App{store: config.NewStore(config.Default()), eng: engine.New([]*providers.Provider{p}, "", 8), bus: newBus()}
	if _, err := translatePopup(context.Background(), a, "Заказ отправлен", ""); err == nil {
		t.Fatal("failed translation was returned as a successful popup")
	}
}

func TestResultReachesDesktopWithoutBrowserSubscribers(t *testing.T) {
	a := &App{bus: newBus()}
	s := New(a, nil)
	var received Result
	s.SetResultHook(func(r Result) { received = r })
	a.showResult(Result{Source: "Заказ", Text: "订单"})
	if received.Text != "订单" || received.Source != "Заказ" {
		t.Fatalf("desktop result missing without SSE subscribers: %+v", received)
	}
}
