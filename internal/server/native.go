package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/local/zshtrans/internal/capture"
	"github.com/local/zshtrans/internal/lang"
	"github.com/local/zshtrans/internal/logx"
	"github.com/local/zshtrans/internal/ocr"
)

// popupResult 划词/截图弹窗的统一返回（对应 pyc 弹窗 原文+译文+meta）
type popupResult struct {
	Source   string `json:"source"`
	Text     string `json:"text"`
	Provider string `json:"provider"`
	MS       int    `json:"ms"`
	Src      string `json:"src"`
	Tgt      string `json:"tgt"`
	Engine   string `json:"engine,omitempty"`
}

// translateForPopup 按配置语向翻译一段文本
func (s *Server) translateForPopup(text string) (popupResult, error) {
	return translatePopup(s.app, text, ""), nil
}

// translatePopup 独立函数版本，供热键 goroutine 直接调用（不持有 Server）
func translatePopup(app *App, text, engineTag string) popupResult {
	text = strings.TrimSpace(text)
	cfg := app.Config()
	src, tgt := lang.Resolve(text, cfg.SrcLang, cfg.TgtLang)
	res, err := app.Engine().Translate(text, src, tgt)
	if err != nil {
		logx.Err("popup translate fail: %v", err)
		return popupResult{Source: text, Text: "翻译失败：" + err.Error(), Src: src, Tgt: tgt, Engine: engineTag}
	}
	return popupResult{
		Source:   text,
		Text:     res.Text,
		Provider: res.Provider,
		MS:       res.MS,
		Src:      src,
		Tgt:      tgt,
		Engine:   engineTag,
	}
}

// handleSelection POST /api/selection —— 划词翻译：模拟 Ctrl+C 取选中文本后翻译
// 对应 translate.py capture_selected_text + 弹窗链路。
func (s *Server) handleSelection(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	text, err := capture.SelectedText(1200 * time.Millisecond)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		writeErr(w, http.StatusBadRequest, "no selection captured")
		return
	}
	out, err := s.translateForPopup(text)
	if err != nil {
		logx.Err("selection translate fail: %v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// captureRequest POST /api/capture 请求体：全部留空表示全屏
type captureRequest struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// handleCapture POST /api/capture —— 截图取词：截屏 → OCR → 翻译
// 对应 translate.py 截图翻译全链路（11.3）。
func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req captureRequest
	// 允许空 body
	if r.ContentLength > 0 {
		if err := readBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var raw []byte
	var err error
	if req.W > 0 && req.H > 0 {
		raw, err = capture.Region(req.X, req.Y, req.W, req.H)
	} else {
		raw, err = capture.Screenshot()
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	img, err := ocr.Decode(raw)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "bad image: "+err.Error())
		return
	}
	cfg := s.app.Config()
	res, err := ocr.Run(img, s.app.ocrKeys(cfg))
	if err != nil {
		logx.Err("capture ocr fail: %v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out, err := s.translateForPopup(strings.TrimSpace(res.Text))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out.Engine = res.Engine
	writeJSON(w, http.StatusOK, out)
}
