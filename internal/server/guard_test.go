package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGuardHostRejectsForeignHost 非 IP、非 localhost 的 Host 必须 403（DNS rebinding 防护，H3 回归）
func TestGuardHostRejectsForeignHost(t *testing.T) {
	h := guardHost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		host string
		want int
	}{
		{"127.0.0.1:8788", http.StatusOK},
		{"localhost:8788", http.StatusOK},
		{"192.168.0.129:8788", http.StatusOK},
		{"[::1]:8788", http.StatusOK},
		{"evil.example.com", http.StatusForbidden},
		{"attacker.test:8788", http.StatusForbidden},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://"+c.host+"/api/status", nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("Host %q → %d, want %d", c.host, w.Code, c.want)
		}
	}
}

// TestLoopbackOnly 非回环来源必须被拒（adminOnly 的内核，H3 回归）
func TestLoopbackOnly(t *testing.T) {
	cases := []struct {
		remote string
		want   bool
	}{
		{"127.0.0.1:12345", true},
		{"[::1]:12345", true},
		{"192.168.0.50:12345", false},
		{"8.8.8.8:12345", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://127.0.0.1/", nil)
		r.RemoteAddr = c.remote
		w := httptest.NewRecorder()
		if got := loopbackOnly(w, r); got != c.want {
			t.Errorf("loopbackOnly(%q) = %v, want %v", c.remote, got, c.want)
		}
	}
}

// TestAllowedOrigin 只有本机/私网来源才允许 CORS 回显，公网来源一律拒绝（H3 回归）
func TestAllowedOrigin(t *testing.T) {
	allowed := []string{
		"http://localhost:5173",
		"http://127.0.0.1:8788",
		"http://192.168.0.10:8080",
		"https://10.0.0.5",
	}
	for _, o := range allowed {
		if !allowedOrigin(o) {
			t.Errorf("origin %q should be allowed", o)
		}
	}
	denied := []string{
		"http://evil.example.com",
		"https://attacker.test:443",
		"file://local",
		"null",
		"",
	}
	for _, o := range denied {
		if allowedOrigin(o) {
			t.Errorf("origin %q should be denied", o)
		}
	}
}

// TestWithCORSNoWildcard 响应头不得出现 Access-Control-Allow-Origin: *（H3 回归）
func TestWithCORSNoWildcard(t *testing.T) {
	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 本机来源：回显该来源
	r := httptest.NewRequest("GET", "http://127.0.0.1:8788/api/status", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("allow-origin = %q, want echoed origin", got)
	}

	// 公网来源：不回显、不发 *
	r2 := httptest.NewRequest("GET", "http://127.0.0.1:8788/api/status", nil)
	r2.Header.Set("Origin", "http://evil.example.com")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r2)
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("allow-origin for foreign origin = %q, want empty", got)
	}
}

// TestReadBodyLimit 超限请求体必须明确报错，不得静默截断（M5 回归）
func TestReadBodyLimit(t *testing.T) {
	body := `{"text":"` + strings.Repeat("a", 200) + `"}`
	r := httptest.NewRequest("POST", "/api/translate", strings.NewReader(body))
	var out map[string]any
	if err := readBodyLimit(r, &out, 64); err == nil {
		t.Error("oversized body should be rejected")
	} else if !strings.Contains(err.Error(), "过大") {
		t.Errorf("error = %v, want oversize message", err)
	}

	// 正常体积应当解析成功
	r2 := httptest.NewRequest("POST", "/api/translate", strings.NewReader(`{"text":"hi"}`))
	var out2 map[string]any
	if err := readBodyLimit(r2, &out2, 1024); err != nil {
		t.Errorf("valid body rejected: %v", err)
	}
}

// TestReadBodyDeclaredLength 声明的 Content-Length 超限时要在读之前就拒绝
func TestReadBodyDeclaredLength(t *testing.T) {
	big := strings.Repeat("x", 1000)
	r := httptest.NewRequest("POST", "/api/translate", strings.NewReader(big))
	r.ContentLength = 1000
	var out map[string]any
	err := readBodyLimit(r, &out, 100)
	if err == nil {
		t.Fatal("declared oversize body should be rejected before reading")
	}
	if !strings.Contains(err.Error(), "过大") {
		t.Errorf("error = %v, want oversize message", err)
	}
}
