package update

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReleaseDownloadRejectsWrongDigest(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(raw) }))
	defer ts.Close()
	dest := filepath.Join(t.TempDir(), "update.exe")
	rel := &Release{URLs: []string{ts.URL}, Size: int64(len(raw)), Digest: "sha256:" + fmt.Sprintf("%064d", 0)}
	if err := DownloadRelease(context.Background(), rel, dest, nil); err == nil {
		t.Fatal("wrong digest accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("invalid update remains installable")
	}
	rel.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	if err := DownloadRelease(context.Background(), rel, dest, nil); err != nil {
		t.Fatal(err)
	}
	rel.Size++
	if err := VerifyRelease(rel, dest); err == nil {
		t.Fatal("mismatched release size accepted")
	}
}

func TestReleaseDownloadRejectsTruncatedPackage(t *testing.T) {
	fixture := make([]byte, 512)
	fixture[0], fixture[1], fixture[0x3c] = 'M', 'Z', 0x40
	copy(fixture[0x40:], "PE\x00\x00")
	path := filepath.Join(t.TempDir(), "truncated.exe")
	if err := os.WriteFile(path, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkPE(path); err == nil {
		t.Fatal("headers without an executable image accepted")
	}
}

func TestReleaseDownloadCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := DownloadRelease(ctx, &Release{URLs: []string{ts.URL}, Size: 1024, Digest: "sha256:" + fmt.Sprintf("%064d", 0)}, filepath.Join(t.TempDir(), "update.exe"), nil)
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("cancelled body read did not stop promptly: %v", err)
	}
}
