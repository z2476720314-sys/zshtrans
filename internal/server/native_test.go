package server

import (
	"context"
	"errors"
	"testing"

	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/engine"
	"github.com/local/zshtrans/internal/providers"
)

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
