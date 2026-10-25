package ocr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type redirectedTransport struct {
	target *url.URL
	next   http.RoundTripper
}

func (r redirectedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme = r.target.Scheme
	copy.URL.Host = r.target.Host
	return r.next.RoundTrip(copy)
}

func TestSpaceUsesHighAccuracyMixedLanguageRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("OCREngine") != "3" || r.Form.Get("language") != "auto" {
			fmt.Fprint(w, `{"OCRExitCode":1,"ParsedResults":[{"ParsedText":"XIE: Galaxy S24 Ultra"}]}`)
			return
		}
		fmt.Fprint(w, `{"OCRExitCode":1,"ParsedResults":[{"ParsedText":"请核对型号: Galaxy S24 Ultra"}]}`)
	}))
	defer srv.Close()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = redirectedTransport{target, previous}
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, language := range []string{"zh", "ru", "auto"} {
		out, err := SpaceContext(context.Background(), "cG5n", "synthetic-test-key", language)
		if err != nil || out != "请核对型号: Galaxy S24 Ultra" {
			t.Fatalf("OCR %s = %q,%v", language, out, err)
		}
	}
}

func TestSpaceReportsProviderErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"OCRExitCode":3,"IsErroredOnProcessing":true,"ErrorMessage":["Engine 3 quota exceeded"],"ErrorDetails":""}`)
	}))
	defer srv.Close()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = redirectedTransport{target, previous}
	t.Cleanup(func() { http.DefaultTransport = previous })
	_, err = SpaceContext(context.Background(), "cG5n", "synthetic-test-key", "auto")
	if err == nil || !strings.Contains(err.Error(), "Engine 3 quota exceeded") {
		t.Fatalf("missing diagnostic: %v", err)
	}
}
