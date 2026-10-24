package ocr

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func decodeJSON(resp *http.Response, out any) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		body := string(raw)
		if len(body) > 240 {
			body = body[:240] + "…"
		}
		return errors.New("http " + itoa(resp.StatusCode) + ": " + body)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		body := string(raw)
		if len(body) > 160 {
			body = body[:160] + "…"
		}
		return errors.New("bad response: " + body)
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ---------- 视觉大模型 OCR（对应 pyc ocr_qwen） ----------

// VisionFn 由外部注入的视觉识别函数，避免 ocr 包反向依赖 providers
type VisionFn func(b64 string) (string, error)

// Vision 视觉模型识别包装
func Vision(b64 string, fn VisionFn) (string, error) {
	if fn == nil {
		return "", errors.New("vision ocr not configured")
	}
	out, err := fn(b64)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", errors.New("Qwen OCR: no text recognized")
	}
	return out, nil
}
