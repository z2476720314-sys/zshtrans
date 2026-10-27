package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/local/zshtrans/internal/capture"
	"github.com/local/zshtrans/internal/lang"
	"github.com/local/zshtrans/internal/ocr"
)

// Result is shared by the desktop popup and the translation page.
type Result struct {
	Source   string `json:"source"`
	Text     string `json:"text"`
	Provider string `json:"provider"`
	MS       int    `json:"ms"`
	Src      string `json:"src"`
	Tgt      string `json:"tgt"`
	Engine   string `json:"engine,omitempty"`
	Action   string `json:"action,omitempty"`
}

func translatePopup(ctx context.Context, app *App, text, engineTag string) (Result, error) {
	text = strings.TrimSpace(text)
	cfg := app.Config()
	src, tgt := lang.Decide(text, cfg.SrcLang, cfg.TgtLang)
	res, err := app.Engine().TranslateContext(ctx, text, src, tgt)
	if err != nil {
		return Result{}, err
	}
	if src == "auto" {
		src = lang.Detect(text)
	}
	return Result{Source: text, Text: res.Text, Provider: res.Provider, MS: res.MS,
		Src: src, Tgt: tgt, Engine: engineTag, Action: "translate"}, nil
}

func (s *Server) handleSelection(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if win := s.app.windower(); win != nil && win.Available() && win.Visible() {
		if !win.PrepareSelection() {
			writeErr(w, http.StatusConflict, "请在目标程序中选中文字，再按划词翻译快捷键")
			return
		}
		defer win.Show()
	}
	text, err := capture.SelectedText(1200*time.Millisecond, "")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if strings.TrimSpace(text) == "" {
		writeErr(w, http.StatusBadRequest, "未获取到选中文字，请使用划词翻译快捷键")
		return
	}
	out, err := translatePopup(r.Context(), s.app, text, "")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type captureRequest struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func capturePopup(ctx context.Context, app *App, req captureRequest) (Result, error) {
	if win := app.windower(); win != nil && win.Available() {
		visible := win.Visible()
		win.Hide()
		if visible {
			defer win.Show()
		}
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	var captured capture.CaptureResult
	var err error
	if req.W > 0 && req.H > 0 {
		captured.PNG, err = capture.Region(req.X, req.Y, req.W, req.H)
		captured.Action = "translate"
	} else {
		captured, err = capture.Interactive(ctx)
	}
	if errors.Is(err, capture.ErrCanceled) {
		return Result{Action: "cancel"}, nil
	}
	if err != nil {
		return Result{}, err
	}
	return processCapture(ctx, app, captured)
}

func processCapture(ctx context.Context, app *App, captured capture.CaptureResult) (Result, error) {
	switch captured.Action {
	case "ocr", "translate":
	case "cancel":
		return Result{Action: "cancel"}, nil
	default:
		return Result{Action: "complete"}, nil
	}
	start := time.Now()
	img, err := ocr.Decode(captured.PNG)
	if err != nil {
		return Result{}, err
	}
	res, err := ocr.RunContext(ctx, img, app.ocrKeys(app.Config()))
	if err != nil {
		return Result{}, err
	}
	if captured.Action == "ocr" {
		return Result{Source: res.Text, Text: res.Text, Provider: "OCR",
			Engine: res.Engine, MS: int(time.Since(start).Milliseconds()), Action: "ocr"}, nil
	}
	out, err := translatePopup(ctx, app, res.Text, res.Engine)
	if err != nil {
		return Result{}, err
	}
	out.MS = int(time.Since(start).Milliseconds())
	return out, nil
}

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req captureRequest
	if r.ContentLength != 0 {
		if err := readBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.W < 0 || req.H < 0 || (req.W == 0) != (req.H == 0) {
		writeErr(w, http.StatusBadRequest, "截图区域宽高必须同时为正数")
		return
	}
	out, err := capturePopup(r.Context(), s.app, req)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, capture.ErrBusy) {
			code = http.StatusConflict
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
