package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/zshtrans/internal/config"
)

func TestSettingsRejectsInvalidOrConflictingWindowHotkey(t *testing.T) {
	for _, window := range []string{"-", "alt+1", "ALT+Q", "ctrl+alt+s"} {
		t.Run(window, func(t *testing.T) {
			t.Setenv("ZSHSTRANS_CONFIG", filepath.Join(t.TempDir(), "config.json"))
			cfg := config.Default()
			cfg.Providers = []config.Provider{{ID: "offline"}}
			app := &App{store: config.NewStore(cfg), bus: newBus()}
			s := New(app, nil)
			body := `{"hotkeys":{"selection":"alt+q","screenshot":"ctrl+alt+s","window":"` + window + `","close":"alt+1"}}`
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/settings", strings.NewReader(body))
			r.RemoteAddr = "127.0.0.1:12345"
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("settings accepted invalid window hotkey: status %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), "热键") {
				t.Fatalf("settings rejected for an unrelated reason: %s", w.Body.String())
			}
			if got := app.store.Get().Hotkeys; got != cfg.Hotkeys {
				t.Fatalf("rejected settings changed hotkeys: %+v", got)
			}
		})
	}
}
