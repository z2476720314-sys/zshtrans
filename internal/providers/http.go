package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// sharedClient 直连提供方共用客户端。
// 超时对齐 pyc 的 TIMEOUT=8（config.Timeout），否则单个不可达站点会把整条
// 提供方链拖到 20s+，前端表现为"点了翻译一直转圈"。
var sharedClient = &http.Client{Timeout: 8 * time.Second}

func newHTTPClient(timeoutSec int) *http.Client {
	if timeoutSec <= 0 {
		timeoutSec = 8
	}
	return &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
}

func getJSON(ctx context.Context, u string, hdr map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return doJSON(req, out)
}

func postJSON(ctx context.Context, u string, body any, hdr map[string]string, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return doJSON(req, out)
}

func postFormJSON(ctx context.Context, u string, form map[string][]string, hdr map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewBufferString(encodeForm(form)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return doJSON(req, out)
}

func encodeForm(form map[string][]string) string {
	var buf bytes.Buffer
	first := true
	for k, vals := range form {
		for _, v := range vals {
			if !first {
				buf.WriteByte('&')
			}
			first = false
			buf.WriteString(urlEncode(k))
			buf.WriteByte('=')
			buf.WriteString(urlEncode(v))
		}
	}
	return buf.String()
}

// getJSONRaw 带查询参数与独立超时的 GET，返回原始响应体（分页目录用）
func getJSONRaw(u string, hdr map[string]string, params map[string]string, timeoutSec int) ([]byte, error) {
	if len(params) > 0 {
		sep := "?"
		if bytes.ContainsRune([]byte(u), '?') {
			sep = "&"
		}
		var buf bytes.Buffer
		buf.WriteString(u)
		buf.WriteString(sep)
		first := true
		for k, v := range params {
			if !first {
				buf.WriteByte('&')
			}
			first = false
			buf.WriteString(urlEncode(k))
			buf.WriteByte('=')
			buf.WriteString(urlEncode(v))
		}
		u = buf.String()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	client := sharedClient
	if timeoutSec > 0 {
		client = newHTTPClient(timeoutSec)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{Status: resp.StatusCode, Body: truncate(string(raw), 240)}
	}
	return raw, nil
}

func doJSON(req *http.Request, out any) error {
	resp, err := sharedClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return &HTTPError{Status: resp.StatusCode, Body: truncate(string(raw), 240)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.New("bad response: " + truncate(string(raw), 160))
	}
	return nil
}

// HTTPError 保留状态码，便于 mymemory 限流等分支判断
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return "http " + itoa(e.Status) + ": " + e.Body
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
