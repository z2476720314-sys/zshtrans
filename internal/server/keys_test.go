package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/zshtrans/internal/config"
	"github.com/local/zshtrans/internal/providers"
)

func TestSettingsPersistsAndMasksCustomProviderCredentials(t *testing.T) {
	// The test executable has its own Go build directory; never write a user profile.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(config.Path()) != filepath.Dir(exe) || !strings.Contains(filepath.Base(exe), ".test") {
		t.Fatal("settings test requires its isolated test executable directory")
	}
	if _, err := os.Stat(config.Path()); err == nil {
		t.Fatal("settings test will not overwrite an existing config")
	}
	t.Cleanup(func() { _ = os.Remove(config.Path()) })
	cfg := config.Default()
	app := &App{store: config.NewStore(cfg), bus: newBus()}
	s := New(app, nil)
	post := func(body string) {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/settings", strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:12345"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("settings status = %d, want 200", w.Code)
		}
	}
	post(`{"default_provider":"custom:translator","providers":[{"id":"custom","label":"Custom","enabled":true,"base_url":"https://provider.example/v1","protocol":"openai-completions","key_ref":"custom-provider","models":[{"id":"translator"}]}],"api_keys":{"custom-provider":"placeholder","openai":"example"}}`)
	for _, value := range []string{"", KeySaved, KeyMissing} {
		body, err := json.Marshal(map[string]any{"api_keys": map[string]string{"custom-provider": value}})
		if err != nil {
			t.Fatal(err)
		}
		post(string(body))
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if providers.ResolveKey(saved.Providers[0], saved.APIKeys, func(string) string { return "" }) != "placeholder" {
		t.Fatal("saved custom provider credential was not resolved after reload")
	}
	if providers.ResolveKey(config.Provider{KeyRef: "openai"}, saved.APIKeys, nil) != "example" {
		t.Fatal("a second extra credential slot was dropped")
	}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/config", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var response struct {
		APIKeys map[string]string `json:"api_keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.APIKeys["custom-provider"] != KeySaved || response.APIKeys["openai"] != KeySaved {
		t.Fatal("custom credential slots were missing or unmasked in config response")
	}
	if strings.Contains(w.Body.String(), "placeholder") || strings.Contains(w.Body.String(), `"example"`) {
		t.Fatal("config response exposed a credential")
	}
}
