package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/local/zshtrans/internal/config"
)

func TestDesktopBridgeUsesLocalHandlerWithoutSocket(t *testing.T) {
	cfg := config.Default()
	cfg.APIKeys.Set("private-slot", "private-placeholder")
	app := &App{store: config.NewStore(cfg), bus: newBus()}
	responses := make(chan DesktopResponse, 1)
	bridge := NewDesktopBridge(New(app, nil).Handler(), func(r DesktopResponse) { responses <- r })
	defer bridge.Close()
	if err := bridge.Request("wrong", "1", "GET", "/api/config", ""); err == nil {
		t.Fatal("unauthorized native request accepted")
	}
	if err := bridge.Request(bridge.Token(), "1", "GET", "/api/config", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-responses:
		if response.Status != 200 || strings.Contains(response.Body, "private-placeholder") {
			t.Fatal("native config response failed or exposed a credential")
		}
		if strings.Contains(response.Body, `"close"`) || strings.Contains(response.Body, `"web"`) {
			t.Fatal("removed tray-hide or web configuration is still exposed")
		}
	case <-time.After(time.Second):
		t.Fatal("native handler did not return")
	}
	for _, path := range []string{"https://example.com/api/config", "//example.com/api/config", "/api/events", "/index.html"} {
		if err := bridge.Request(bridge.Token(), "2", "GET", path, ""); err == nil {
			t.Fatalf("accepted unsupported native path %q", path)
		}
	}
}

func TestDesktopBridgeCancellationReachesHandler(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	bridge := NewDesktopBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
		w.WriteHeader(http.StatusRequestTimeout)
	}), func(DesktopResponse) {})
	defer bridge.Close()
	if err := bridge.Request(bridge.Token(), "request", "POST", "/api/translate", "{}"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	if err := bridge.Cancel(bridge.Token(), "request"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("native cancellation did not reach handler")
	}
}

func TestDesktopEventsUseInProcessBus(t *testing.T) {
	app := &App{bus: newBus()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 2)
	go New(app, nil).DesktopEvents(ctx, func(ev Event) { events <- ev })
	select {
	case ev := <-events:
		if ev.Kind != "ready" {
			t.Fatal("native event bridge not ready")
		}
	case <-time.After(time.Second):
		t.Fatal("event bridge did not start")
	}
	app.Publish(Event{Kind: "popup", Data: "example"})
	select {
	case ev := <-events:
		if ev.Kind != "popup" || ev.Data != "example" {
			t.Fatal("native event lost")
		}
	case <-time.After(time.Second):
		t.Fatal("event did not arrive")
	}
}
