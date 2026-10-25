package providers

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strings"

	"github.com/local/zshtrans/internal/lang"
)

// Func 翻译函数签名（对应 pyc 各 t_* 函数）
type Func func(ctx context.Context, text, src, tgt string) (string, error)

// Keys 密钥读取器，隔离 config 依赖
type Keys interface {
	DeepL() string
	BaiduAppID() string
	BaiduKey() string
	NiuTrans() string
	Agnes() string
	Qwen() string
	OCRSpace() string
	BaiduOCRAK() string
	BaiduOCRsk() string
	// Has 判断 api_keys 中某个键是否有非空值（对应 pyc CFG['api_keys'].get(k) 真值）
	Has(key string) bool
}

// Client 统一 HTTP 客户端（带超时）
type Client struct {
	Timeout int
}

var httpClient = newHTTPClient(8)

// ---------- Google ----------

const googleURL = "https://translate.googleapis.com/translate_a/single"

func tGoogle(ctx context.Context, text, src, tgt string) (string, error) {
	q := url.Values{}
	q.Set("client", "gtx")
	q.Set("sl", src)
	q.Set("tl", tgt)
	q.Set("dt", "t")
	q.Set("q", text)
	var resp [][][]any
	if err := getJSON(ctx, googleURL+"?"+q.Encode(), nil, &resp); err != nil {
		return "", err
	}
	var sb strings.Builder
	if len(resp) > 0 {
		for _, seg := range resp[0] {
			if len(seg) > 0 {
				if s, ok := seg[0].(string); ok {
					sb.WriteString(s)
				}
			}
		}
	}
	out := sb.String()
	if strings.TrimSpace(out) == "" {
		return "", errors.New("empty result")
	}
	return out, nil
}

// ---------- Lingva（双镜像） ----------

var lingvaMirrors = []string{
	"https://translate.plausibility.cloud",
	"https://lingva.ml",
}

func tLingva(ctx context.Context, text, src, tgt string) (string, error) {
	var errs []string
	for _, mirror := range lingvaMirrors {
		u := fmt.Sprintf("%s/api/v1/%s/%s/%s?safe=0", mirror, src, tgt, url.PathEscape(text))
		var resp struct {
			Translation string `json:"translation"`
		}
		if err := getJSON(ctx, u, nil, &resp); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if strings.TrimSpace(resp.Translation) != "" {
			return resp.Translation, nil
		}
		errs = append(errs, "empty result")
	}
	return "", fmt.Errorf("all lingva mirrors failed: %s", strings.Join(errs, "; "))
}

// ---------- MyMemory（按句切分并发） ----------

const myMemoryURL = "https://api.mymemory.translated.net/get"

// sentenceBreaks 句末标点集合（Go RE2 不支持 lookbehind，改为手工扫描）
const sentenceBreaks = ".!?。！？\n"

func tMyMemory(ctx context.Context, text, src, tgt string) (string, error) {
	parts := splitSentences(text)
	if len(parts) == 0 {
		parts = []string{text}
	}
	type res struct {
		idx int
		out string
		err error
	}
	ch := make(chan res, len(parts))
	sem := make(chan struct{}, 4)
	for i, part := range parts {
		if strings.TrimSpace(part) == "" {
			ch <- res{idx: i, out: part}
			continue
		}
		go func(i int, part string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			out, err := myMemoryOne(ctx, part, src, tgt)
			ch <- res{idx: i, out: out, err: err}
		}(i, part)
	}
	outs := make([]string, len(parts))
	var firstErr error
	for range parts {
		r := <-ch
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		outs[r.idx] = r.out
	}
	if firstErr != nil && strings.TrimSpace(strings.Join(outs, "")) == "" {
		return "", firstErr
	}
	return strings.Join(outs, ""), nil
}

func myMemoryOne(ctx context.Context, text, src, tgt string) (string, error) {
	// MyMemory 不接受 langpair 里的 auto（会返回 "'AUTO' IS AN INVALID SOURCE
	// LANGUAGE"），必须先按文字内容判出具体源语言，否则 auto 场景必然失败。
	if src == "auto" {
		src = lang.Detect(text)
	}
	q := url.Values{}
	q.Set("q", text)
	q.Set("langpair", src+"|"+tgt)
	var resp struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
		ResponseStatus any `json:"responseStatus"`
	}
	if err := getJSON(ctx, myMemoryURL+"?"+q.Encode(), nil, &resp); err != nil {
		return "", err
	}
	status := fmt.Sprint(resp.ResponseStatus)
	if status != "200" {
		return "", errors.New("mymemory error")
	}
	out := resp.ResponseData.TranslatedText
	if strings.TrimSpace(out) == "" {
		return "", errors.New("empty result")
	}
	return out, nil
}

// splitSentences 按句末标点切句，标点留在前一句末尾（对齐 pyc 的 `(?<=[.!?。！？\n])`）
func splitSentences(text string) []string {
	var out []string
	start := 0
	for i, r := range text {
		if !strings.ContainsRune(sentenceBreaks, r) {
			continue
		}
		end := i + len(string(r))
		if end > start {
			out = append(out, text[start:end])
			start = end
		}
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

// ---------- DeepL ----------

const deepLURL = "https://api-free.deepl.com/v2/translate"

// deeplTarget 目标语言映射（zh→ZH, en→EN-US, 其余大写）
func deeplTarget(tgt string) string {
	switch tgt {
	case "en":
		return "EN-US"
	case "zh":
		return "ZH"
	default:
		return strings.ToUpper(tgt)
	}
}

func tDeepL(ctx context.Context, text, src, tgt string, keys Keys) (string, error) {
	form := url.Values{}
	form.Set("text", text)
	form.Set("target_lang", deeplTarget(tgt))
	if src != "auto" {
		form.Set("source_lang", strings.ToUpper(src))
	}
	var resp struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	hdr := map[string]string{}
	hdr["Authorization"] = "DeepL-Auth-Key " + keys.DeepL()
	if err := postFormJSON(ctx, deepLURL, form, hdr, &resp); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, t := range resp.Translations {
		sb.WriteString(t.Text)
	}
	out := sb.String()
	if strings.TrimSpace(out) == "" {
		return "", errors.New("empty result")
	}
	return out, nil
}

// ---------- 百度翻译（MD5 签名） ----------

const baiduURL = "https://fanyi-api.baidu.com/api/trans/vip/translate"

// baiduCodes 语言代码映射（translate.py:4371）
var baiduCodes = map[string]string{
	"ja": "jp", "ko": "kor", "fr": "fra", "es": "spa",
	"ar": "ara", "vi": "vie", "id": "id",
}

func baiduCode(id string) string {
	if v, ok := baiduCodes[id]; ok {
		return v
	}
	return id
}

func tBaidu(ctx context.Context, text, src, tgt string, keys Keys) (string, error) {
	appID, key := keys.BaiduAppID(), keys.BaiduKey()
	if appID == "" || key == "" {
		return "", errors.New("baidu key not set")
	}
	salt := fmt.Sprintf("%d", 10000+rand.Intn(90000))
	sum := md5.Sum([]byte(appID + text + salt + key))
	q := url.Values{}
	q.Set("q", text)
	q.Set("from", baiduCode(src))
	q.Set("to", baiduCode(tgt))
	q.Set("appid", appID)
	q.Set("salt", salt)
	q.Set("sign", hex.EncodeToString(sum[:]))
	var resp struct {
		TransResult []struct {
			Dst string `json:"dst"`
		} `json:"trans_result"`
		ErrorCode any    `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	if err := getJSON(ctx, baiduURL+"?"+q.Encode(), nil, &resp); err != nil {
		return "", err
	}
	if code := fmt.Sprint(resp.ErrorCode); code != "" && code != "<nil>" && code != "0" {
		return "", fmt.Errorf("baidu %s: %s", code, resp.ErrorMsg)
	}
	var sb strings.Builder
	for _, item := range resp.TransResult {
		sb.WriteString(item.Dst)
	}
	out := sb.String()
	if strings.TrimSpace(out) == "" {
		return "", errors.New("empty result")
	}
	return out, nil
}

// ---------- 小牛翻译 ----------

const niuTransURL = "https://api.niutrans.com/NiuTransServer/translation"

func tNiuTrans(ctx context.Context, text, src, tgt string, keys Keys) (string, error) {
	form := url.Values{}
	form.Set("from", src)
	form.Set("to", tgt)
	form.Set("src_text", text)
	form.Set("apikey", keys.NiuTrans())
	var resp struct {
		TgtText  string `json:"tgt_text"`
		ErrorMsg string `json:"error_msg"`
	}
	if err := postFormJSON(ctx, niuTransURL, form, nil, &resp); err != nil {
		return "", err
	}
	if resp.ErrorMsg != "" {
		return "", fmt.Errorf("niutrans %s", resp.ErrorMsg)
	}
	if strings.TrimSpace(resp.TgtText) == "" {
		return "", errors.New("empty result")
	}
	return resp.TgtText, nil
}

// ---------- Agnes 2.5 Flash ----------

const agnesURL = "https://apihub.agnes-ai.com/v1/chat/completions"

func tAgnes(ctx context.Context, text, src, tgt string, keys Keys) (string, error) {
	key := keys.Agnes()
	if key == "" {
		return "", errors.New("Agnes API key empty (set it in settings)")
	}
	return chatTranslate(ctx, agnesURL, "agnes-2.5-flash", key, text, src, tgt, nil)
}

// ---------- 百炼 GLM-5.2 / Qwen3.7-Max ----------

const qwenURL = "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"

func tQwenMax(ctx context.Context, text, src, tgt string, keys Keys) (string, error) {
	key := keys.Qwen()
	if key == "" {
		return "", errors.New("Qwen API key empty (set it in settings)")
	}
	return chatTranslate(ctx, qwenURL, "glm-5.2", key, text, src, tgt, map[string]any{"enable_thinking": false})
}

func tQwenMaxSnap(ctx context.Context, text, src, tgt string, keys Keys) (string, error) {
	key := keys.Qwen()
	if key == "" {
		return "", errors.New("Qwen API key empty (set it in settings)")
	}
	return chatTranslate(ctx, qwenURL, "qwen3.7-max-2026-06-08", key, text, src, tgt, map[string]any{"enable_thinking": false})
}

// chatTranslate 通用 OpenAI 兼容翻译调用
func chatTranslate(ctx context.Context, endpoint, model, key, text, src, tgt string, extra map[string]any) (string, error) {
	sys := BuildPrompt(src, tgt)

	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": text},
		},
		"temperature": 0.3,
	}
	for k, v := range extra {
		body[k] = v
	}
	hdr := map[string]string{"Authorization": "Bearer " + key}
	raw, err := postRaw(ctx, endpoint, body, hdr, 8)
	if err != nil {
		return "", err
	}
	out, err := ExtractText(raw)
	if err != nil {
		return "", err
	}
	return CleanOutput(out, tgt)
}
