package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateCancellationStopsDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upd = updateState{busy: true, cancel: cancel}
	t.Cleanup(func() { cancel(); upd = updateState{} })
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleUpdateApply(w, httptest.NewRequest(http.MethodPost, "/api/update/apply", strings.NewReader(`{"action":"cancel"}`)))
	if w.Code != http.StatusOK || ctx.Err() == nil {
		t.Fatalf("cancel failed: %d %s", w.Code, w.Body.String())
	}
}

func TestDownloadingPreviousFileIsNotReady(t *testing.T) {
	upd = updateState{busy: true, path: "previous-download.exe"}
	t.Cleanup(func() { upd = updateState{} })
	w := httptest.NewRecorder()
	(&Server{}).handleUpdateApply(w, httptest.NewRequest(http.MethodPost, "/api/update/apply", strings.NewReader(`{"action":"status"}`)))
	var state struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Ready {
		t.Fatal("old download advertised as ready during new download")
	}
}
