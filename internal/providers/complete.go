package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/local/zshtrans/internal/config"
)

// ExtractText 三种协议解包（translate.py:534 extract_completion_text）
func ExtractText(raw []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return CollectStream(strings.Split(string(raw), "\n"))
	}
	if err := CheckOutputLimit(payload); err != nil {
		return "", err
	}
	var out string
	if s, ok := payload["choices"]; ok {
		out = fromChoices(s)
	} else if s, ok := payload["output"]; ok {
		out = fromResponses(s)
	} else if s, ok := payload["content"]; ok {
		out = fromAnthropic(s)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", errors.New("empty result")
	}
	return out, nil
}

func fromChoices(v any) string {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return ""
	}
	first, ok := list[0].(map[string]any)
	if !ok {
		return ""
	}
	msg, ok := first["message"].(map[string]any)
	if !ok {
		return ""
	}
	return typedText(msg["content"], "text")
}

func fromResponses(v any) string {
	list, ok := v.([]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, item := range list {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		sb.WriteString(typedText(node["content"], "text", "output_text", "input_text"))
	}
	return sb.String()
}

func fromAnthropic(v any) string {
	return typedText(v, "text")
}

// typedText 从 string / []block 里抽取指定类型的文本
func typedText(v any, kinds ...string) string {
	switch node := v.(type) {
	case string:
		return node
	case []any:
		var sb strings.Builder
		for _, item := range node {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := block["type"].(string)
			if kind != "" && !contains(kinds, kind) {
				continue
			}
			if text, ok := block["text"].(string); ok {
				sb.WriteString(text)
			}
		}
		return sb.String()
	}
	return ""
}

// CheckOutputLimit 输出被截断时给出明确错误（translate.py:522）
func CheckOutputLimit(payload map[string]any) error {
	if payload["error"] != nil || payload["status"] == "failed" {
		return errors.New("model request failed")
	}
	if payload["status"] == "incomplete" {
		return errors.New("模型输出未完成，请提高最大输出或缩短原文")
	}
	for _, key := range []string{"finish_reason", "stop_reason"} {
		switch payload[key] {
		case "length", "max_tokens", "model_context_window_exceeded", "content_filter":
			return errors.New("模型输出被截断，请提高最大输出或缩短原文")
		}
	}
	for _, key := range []string{"response", "delta", "incomplete_details"} {
		if nested, ok := payload[key].(map[string]any); ok {
			if nested["reason"] == "max_output_tokens" {
				return errors.New("模型输出被截断，请提高最大输出或缩短原文")
			}
			if err := CheckOutputLimit(nested); err != nil {
				return err
			}
		}
	}
	if choices, ok := payload["choices"].([]any); ok {
		for _, choice := range choices {
			if item, ok := choice.(map[string]any); ok {
				if err := CheckOutputLimit(item); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// StreamDelta 流式增量（translate.py:559）
func StreamDelta(chunk map[string]any) string {
	if chunk["type"] == "response.output_text.delta" {
		text, _ := chunk["delta"].(string)
		return text
	}
	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if first, ok := choices[0].(map[string]any); ok {
			if delta, ok := first["delta"].(map[string]any); ok {
				if s, ok := delta["content"].(string); ok {
					return s
				}
			}
			if msg, ok := first["message"].(map[string]any); ok {
				return typedText(msg["content"], "text")
			}
		}
	}
	if delta, ok := chunk["delta"].(map[string]any); ok {
		if s, ok := delta["text"].(string); ok {
			return s
		}
	}
	if t, ok := chunk["type"].(string); ok && t == "content_block_delta" {
		if d, ok := chunk["delta"].(map[string]any); ok {
			if s, ok := d["text"].(string); ok {
				return s
			}
		}
	}
	return ""
}

// CollectStream 解析 SSE 行并拼接（translate.py:587）
func CollectStream(lines []string) (string, error) {
	var sb strings.Builder
	var data []string
	complete := false
	for _, line := range append(lines, "") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if line != "" || len(data) == 0 {
			continue
		}
		payload := strings.Join(data, "\n")
		data = nil
		if payload == "[DONE]" {
			complete = true
			break
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return "", errors.New("invalid model stream")
		}
		if err := CheckOutputLimit(chunk); err != nil {
			return "", err
		}
		sb.WriteString(StreamDelta(chunk))
		if chunk["type"] == "message_stop" || chunk["type"] == "response.completed" {
			complete = true
		}
		if choices, ok := chunk["choices"].([]any); ok {
			for _, choice := range choices {
				if item, ok := choice.(map[string]any); ok && item["finish_reason"] == "stop" {
					complete = true
				}
			}
		}
	}
	if !complete {
		return "", errors.New("模型流式响应中途结束，译文不完整")
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return "", errors.New("empty result")
	}
	return text, nil
}

// TranslateWith 用某个提供方+模型翻译一次（translate.py:625 make_translator）
func TranslateWith(p config.Provider, m config.Model, key, text, src, tgt string) (string, error) {
	return TranslateWithContext(context.Background(), p, m, key, text, src, tgt)
}

func TranslateWithContext(ctx context.Context, p config.Provider, m config.Model, key, text, src, tgt string) (string, error) {
	prompt := BuildPrompt(src, tgt)
	body, err := BuildChatPayload(p, m, prompt, text)
	if err != nil {
		return "", err
	}
	raw, err := PostChatContext(ctx, p, key, body)
	if err != nil {
		return "", err
	}
	out, err := ExtractText(raw)
	if err != nil {
		return "", err
	}
	return CleanOutput(out, tgt)
}

// PostChat 发请求返回原始响应
func PostChat(p config.Provider, key string, body map[string]any) ([]byte, error) {
	return PostChatContext(context.Background(), p, key, body)
}

func PostChatContext(ctx context.Context, p config.Provider, key string, body map[string]any) ([]byte, error) {
	body["stream"] = p.Stream
	url := ChatURL(p)
	hdr := AuthHeaders(p, key)
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return postRaw(ctx, url, body, hdr, timeout)
}

// VisionOCR 用视觉模型识别图片（translate.py:674 make_vision_ocr）
func VisionOCR(p config.Provider, m config.Model, key, b64 string) (string, error) {
	return VisionOCRContext(context.Background(), p, m, key, b64)
}

func VisionOCRContext(ctx context.Context, p config.Provider, m config.Model, key, b64 string) (string, error) {
	limit, err := GenerationLimit(m)
	if err != nil {
		return "", err
	}
	var body map[string]any
	switch ProtocolOf(p) {
	case "anthropic-messages":
		body = map[string]any{
			"model":      m.ID,
			"max_tokens": limit,
			"messages": []map[string]any{{
				"role": "user",
				"content": []map[string]any{
					{"type": "image", "source": map[string]any{
						"type": "base64", "media_type": "image/png", "data": b64,
					}},
					{"type": "text", "text": OCRPrompt},
				},
			}},
		}
	case "openai-responses":
		body = map[string]any{
			"model":             m.ID,
			"max_output_tokens": limit,
			"input": []map[string]any{{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": OCRPrompt},
					{"type": "input_image", "image_url": "data:image/png;base64," + b64},
				},
			}},
		}
	default:
		body = map[string]any{
			"model":      m.ID,
			"max_tokens": limit,
			"messages": []map[string]any{{
				"role": "user",
				"content": []map[string]any{
					{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + b64}},
					{"type": "text", "text": OCRPrompt},
				},
			}},
		}
		if m.Temperature != nil {
			body["temperature"] = *m.Temperature
		}
		for k, v := range ThinkingFields(p, m) {
			body[k] = v
		}
	}
	raw, err := PostChatContext(ctx, p, key, body)
	if err != nil {
		return "", err
	}
	out, err := ExtractText(raw)
	if err != nil {
		return "", fmt.Errorf("vision ocr: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// FetchModels 拉取账号可用模型列表（translate.py:874）
func FetchModels(p config.Provider, key string) ([]string, error) {
	list, err := FetchModelConfigs(p, key)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, m.ID)
	}
	return out, nil
}

// googleBase 谷歌 OpenAI 兼容端点的特例前缀（目录在 /openai 之外）
const googleBase = "https://generativelanguage.googleapis.com/v1beta/openai"

// FetchModelConfigs 读取模型目录全部页并解析元数据（translate.py:829 fetch_model_configs）
// 分页按 nextPageToken（Google）/ last_id + has_more（其他）推进；分页不完整时报错不落地。
func FetchModelConfigs(p config.Provider, key string) ([]config.Model, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("%s 未配置 API 密钥", ProviderLabel(p, config.Model{}))
	}
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	isGoogle := base == googleBase
	url := ModelsURL(p)
	hdr := AuthHeaders(p, key)
	if isGoogle {
		url = strings.TrimSuffix(base, "/openai") + "/models"
		hdr = map[string]string{"x-goog-api-key": key}
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	seen := map[string]config.Model{}
	cursors := map[string]bool{}
	params := map[string]string{}
	for {
		raw, err := getJSONRaw(url, hdr, params, timeout)
		if err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, errors.New("模型目录格式无效")
		}
		key := "data"
		if isGoogle {
			key = "models"
		}
		items, ok := payload[key].([]any)
		if !ok {
			return nil, errors.New("接口没有返回模型目录；可按官方文档手动添加模型")
		}
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if isGoogle && !hasMethod(row, "generateContent") {
				continue
			}
			m := ModelFromCatalogue(row)
			if m.ID != "" {
				seen[m.ID] = m
			}
		}
		cursor := ""
		more := false
		if isGoogle {
			cursor, _ = payload["nextPageToken"].(string)
			more = cursor != ""
		} else {
			cursor, _ = payload["last_id"].(string)
			flag, _ := payload["has_more"].(bool)
			more = flag
		}
		if !more {
			break
		}
		if cursor == "" || cursors[cursor] {
			return nil, errors.New("模型目录分页不完整，未应用本次结果")
		}
		cursors[cursor] = true
		if isGoogle {
			params = map[string]string{"pageToken": cursor}
		} else {
			params = map[string]string{"after_id": cursor}
		}
	}
	out := make([]config.Model, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	return out, nil
}

// hasMethod Google 目录里 supportedGenerationMethods 是否含指定方法
func hasMethod(row map[string]any, name string) bool {
	raw, ok := row["supportedGenerationMethods"].([]any)
	if !ok {
		return false
	}
	for _, v := range raw {
		if s, ok := v.(string); ok && s == name {
			return true
		}
	}
	return false
}

// ModelFromCatalogue 从目录条目生成模型配置（translate.py:793 model_config_from_catalogue）
// 缺 limits / effort 枚举时保持未知，不臆造。
func ModelFromCatalogue(raw map[string]any) config.Model {
	caps, _ := raw["capabilities"].(map[string]any)
	effort, _ := caps["effort"].(map[string]any)
	thinking, _ := caps["thinking"].(map[string]any)
	types, _ := thinking["types"].(map[string]any)
	top, _ := raw["top_provider"].(map[string]any)
	arch, _ := raw["architecture"].(map[string]any)
	imageInput, _ := caps["image_input"].(map[string]any)

	// 推理档：按 REASONING_LEVELS 顺序取 supported=true 的档位；disabled 支持时最前插入 off
	levels := make([]string, 0, len(ReasoningLevels))
	for _, level := range ReasoningLevels {
		row, ok := effort[level].(map[string]any)
		if !ok {
			continue
		}
		if supported, _ := row["supported"].(bool); supported {
			levels = append(levels, level)
		}
	}
	if disabled, ok := types["disabled"].(map[string]any); ok {
		if supported, _ := disabled["supported"].(bool); supported {
			levels = append([]string{"off"}, levels...)
		}
	}
	if len(levels) == 0 {
		for _, field := range []string{"reasoning_levels", "supported_reasoning_efforts"} {
			if list, ok := raw[field].([]any); ok {
				for _, v := range list {
					if s, ok := v.(string); ok {
						levels = append(levels, s)
					}
				}
			}
		}
	}

	// 思考参数种类：types 中 supported=true 的键
	thinkingTypes := make([]string, 0, len(types))
	for kind, detail := range types {
		row, ok := detail.(map[string]any)
		if !ok {
			continue
		}
		if supported, _ := row["supported"].(bool); supported {
			thinkingTypes = append(thinkingTypes, kind)
		}
	}

	id := strField(raw, "id")
	if id == "" {
		id = strField(raw, "model")
	}
	if id == "" {
		id = strings.TrimPrefix(strField(raw, "name"), "models/")
	}
	label := strField(raw, "display_name")
	if label == "" {
		label = strField(raw, "displayName")
	}
	if label == "" {
		label = id
	}

	image := false
	if supported, ok := imageInput["supported"].(bool); ok && supported {
		image = true
	}
	if mods, ok := arch["input_modalities"].([]any); ok {
		for _, v := range mods {
			if s, ok := v.(string); ok && s == "image" {
				image = true
			}
		}
	}

	contextWindow := firstInt(raw, "max_input_tokens", "inputTokenLimit", "context_window", "context_length")
	if contextWindow == 0 {
		contextWindow = intField(top, "context_length")
	}
	maxOutput := firstInt(raw, "max_tokens", "outputTokenLimit", "max_output_tokens")
	if maxOutput == 0 {
		maxOutput = intField(top, "max_completion_tokens")
	}

	return NormalizeModel(config.Model{
		ID:              id,
		Label:           label,
		Image:           image,
		Reasoning:       "off",
		ThinkingParam:   "none",
		ContextWindow:   contextWindow,
		MaxOutputTokens: maxOutput,
		ReasoningLevels: levels,
		ThinkingTypes:   thinkingTypes,
		MetadataSource:  "provider",
	})
}

func strField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return 0
}

func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if n := intField(m, k); n != 0 {
			return n
		}
	}
	return 0
}

// TestConnection 连通性测试（translate.py:879）
func TestConnection(p config.Provider, m config.Model, key string) error {
	_, err := TranslateWith(p, m, key, "ping", "auto", "zh")
	return err
}

// UsableProviders 有密钥且启用的提供方（translate.py:937）
func UsableProviders(list []config.Provider, keys config.APIKeys, environ func(string) string) []config.Provider {
	out := make([]config.Provider, 0, len(list))
	for _, p := range list {
		if !p.Enabled || len(p.Models) == 0 {
			continue
		}
		if ResolveKey(p, keys, environ) == "" && p.KeyEnv == "" && !p.Builtin {
			continue
		}
		out = append(out, p)
	}
	return out
}

// VisionModels 可用视觉模型（translate.py:906）
func VisionModels(list []config.Provider, keys config.APIKeys, environ func(string) string, requireKey bool) [][2]string {
	var out [][2]string
	for _, p := range list {
		if !p.Enabled {
			continue
		}
		if requireKey && ResolveKey(p, keys, environ) == "" {
			continue
		}
		for _, m := range p.Models {
			if m.Image {
				out = append(out, [2]string{EngineName(p, m), ProviderLabel(p, m)})
			}
		}
	}
	return out
}
