package ocr

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SpaceURL OCR.space 端点（pyc ocr_space）
const SpaceURL = "https://api.ocr.space/parse/image"

// spacePass 视为成功的 OCRExitCode
var spacePass = map[string]bool{"1": true, "2": true, "3": true}

// Space OCR.space 识别
func Space(b64 string, apiKey string, srcLang string) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errors.New("ocr.space key empty")
	}
	lang := "auto"
	if srcLang == "ru" {
		lang = "rus"
	}
	form := url.Values{}
	form.Set("apikey", apiKey)
	form.Set("language", lang)
	form.Set("OCREngine", "2")
	form.Set("scale", "true")
	form.Set("isOverlayRequired", "false")
	form.Set("filetype", "PNG")
	form.Set("base64Image", "data:image/png;base64,"+b64)

	req, err := http.NewRequest(http.MethodPost, SpaceURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 40 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		ParsedResults []struct {
			ParsedText string `json:"ParsedText"`
		} `json:"ParsedResults"`
		IsErroredOnProcessing bool `json:"IsErroredOnProcessing"`
		OCRExitCode           any  `json:"OCRExitCode"`
		ErrorDetails          any  `json:"ErrorDetails"`
	}
	if err := decodeJSON(resp, &payload); err != nil {
		return "", err
	}
	code := fmt.Sprint(payload.OCRExitCode)
	if payload.IsErroredOnProcessing || (code != "<nil>" && code != "" && !spacePass[code]) {
		detail := fmt.Sprint(payload.ErrorDetails)
		if detail == "<nil>" {
			detail = ""
		}
		return "", fmt.Errorf("ocr error: %s", detail)
	}
	var sb strings.Builder
	for _, r := range payload.ParsedResults {
		sb.WriteString(r.ParsedText)
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", errors.New("no text recognized")
	}
	return out, nil
}

// BaiduToken 百度 OCR access_token 缓存（对应 pyc 模块级 _BAIDU_OCR_TOKEN）
type BaiduToken struct {
	value string
	exp   time.Time
}

var baiduToken BaiduToken

// ResetBaiduToken 清空缓存（设置变更后调用）
func ResetBaiduToken() { baiduToken = BaiduToken{} }

// Baidu 百度 OCR 通用文字识别
func Baidu(b64 string, ak, sk string) (string, error) {
	if strings.TrimSpace(ak) == "" || strings.TrimSpace(sk) == "" {
		return "", errors.New("baidu ocr key not set")
	}
	token, err := baiduAccess(ak, sk)
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("image", b64)
	form.Set("language_type", "auto_detect")

	endpoint := "https://aip.baidubce.com/rest/2.0/ocr/v1/general_basic?access_token=" + url.QueryEscape(token)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 40 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		WordsResult []struct {
			Words string `json:"words"`
		} `json:"words_result"`
		ErrorCode any    `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	if err := decodeJSON(resp, &payload); err != nil {
		return "", err
	}
	if code := fmt.Sprint(payload.ErrorCode); code != "<nil>" && code != "" && code != "0" {
		return "", fmt.Errorf("baidu ocr %s: %s", code, payload.ErrorMsg)
	}
	var sb strings.Builder
	for _, w := range payload.WordsResult {
		sb.WriteString(w.Words)
		sb.WriteString("\n")
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", errors.New("no text recognized")
	}
	return out, nil
}

// baiduAccess 取 token，命中缓存直接用
func baiduAccess(ak, sk string) (string, error) {
	if baiduToken.value != "" && time.Now().Before(baiduToken.exp) {
		return baiduToken.value, nil
	}
	q := url.Values{}
	q.Set("grant_type", "client_credentials")
	q.Set("client_id", ak)
	q.Set("client_secret", sk)
	endpoint := "https://aip.baidubce.com/oauth/2.0/token?" + q.Encode()
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := decodeJSON(resp, &payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", errors.New("baidu ocr token fail")
	}
	ttl := payload.ExpiresIn
	if ttl <= 0 {
		ttl = 2592000
	}
	// 提前 60 秒过期，避免边界失效
	baiduToken = BaiduToken{value: payload.AccessToken, exp: time.Now().Add(time.Duration(ttl-60) * time.Second)}
	return payload.AccessToken, nil
}

// B64 图片字节 → base64 字符串
func B64(raw []byte) string { return base64.StdEncoding.EncodeToString(raw) }
