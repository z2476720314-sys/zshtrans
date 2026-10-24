package providers

import (
	"regexp"
	"strings"

	"github.com/local/zshtrans/internal/config"
)

// 协议与档位常量（translate.py:42-61）
var (
	Protocols = []string{"openai-completions", "openai-responses", "anthropic-messages"}
	// ProtocolPaths 各协议追加路径
	ProtocolPaths = map[string]string{
		"openai-completions": "/chat/completions",
		"openai-responses":   "/responses",
		"anthropic-messages": "/messages",
	}
	// ProtocolLabels 设置页显示名
	ProtocolLabels = map[string]string{
		"openai-completions": "OpenAI 兼容（chat/completions）",
		"openai-responses":   "OpenAI Responses",
		"anthropic-messages": "Anthropic Messages",
	}
	// ReasoningLevels 推理档位
	ReasoningLevels = []string{"off", "none", "minimal", "low", "medium", "high", "xhigh", "max"}
	// ThinkingParams 思考参数种类
	ThinkingParams = []string{"none", "reasoning_effort", "thinking_budget"}
	// ThinkingStyles 思考参数风格
	ThinkingStyles = []string{"enable_thinking", "thinking_object", "adaptive", "reasoning_split", "none"}
	// ReasoningLabels 档位中文名
	ReasoningLabels = map[string]string{
		"off": "默认 / 关闭", "none": "无推理", "minimal": "极低",
		"low": "低", "medium": "中", "high": "高", "xhigh": "极高", "max": "最高",
	}
	// StyleLabels 风格中文名
	StyleLabels = map[string]string{
		"enable_thinking": "enable_thinking（百炼系）",
		"thinking_object": "thinking.object（Acctoken 系）",
		"adaptive":        "自适应思考（Claude）",
		"reasoning_split": "分离思考输出（MiniMax）",
		"none":            "不发送思考参数",
	}
)

const (
	DefaultTimeout          = 20
	DefaultTemperature      = 0.3
	DefaultMaxTokens        = 8192
	AnthropicVersion        = "2023-06-01"
	AnthropicMinThinkBudget = 1024
)

// OCRPrompt 视觉模型识别提示词
const OCRPrompt = "请识别图片中的所有文字，按原样输出识别到的文字内容；不要解释、不要翻译、不要添加额外内容；多行文字保持原有换行。如果文字较小或图片模糊，请仔细辨认每个字符，不要遗漏、不要臆造，保持原文拼写正确。"

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug 显示名 → 稳定 ascii id
func Slug(text, fallback string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(text)), "-"), "-")
	if s == "" {
		return fallback
	}
	return s
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// ProtocolOf 协议收敛到支持集
func ProtocolOf(p config.Provider) string {
	v := strings.ToLower(strings.TrimSpace(p.Protocol))
	if contains(Protocols, v) {
		return v
	}
	return Protocols[0]
}

// NormalizeModel 模型条目归一化
func NormalizeModel(m config.Model) config.Model {
	id := strings.TrimSpace(m.ID)
	reasoning := strings.ToLower(strings.TrimSpace(m.Reasoning))
	if !contains(ReasoningLevels, reasoning) {
		reasoning = "off"
	}
	tp := strings.ToLower(strings.TrimSpace(m.ThinkingParam))
	if !contains(ThinkingParams, tp) {
		if reasoning == "off" {
			tp = "none"
		} else {
			tp = "reasoning_effort"
		}
	}
	label := strings.TrimSpace(m.Label)
	if label == "" {
		label = id
	}
	temp := m.Temperature
	// Kimi K3 固定采样参数，不下发 temperature
	if id == "kimi-k3" {
		temp = nil
	}
	return config.Model{
		ID:              id,
		Label:           label,
		Image:           m.Image,
		Reasoning:       reasoning,
		ThinkingParam:   tp,
		ThinkingBudget:  m.ThinkingBudget,
		Temperature:     temp,
		ContextWindow:   m.ContextWindow,
		MaxOutputTokens: m.MaxOutputTokens,
		ReasoningLevels: m.ReasoningLevels,
		ThinkingTypes:   m.ThinkingTypes,
		MetadataSource:  m.MetadataSource,
	}
}

// NormalizeProvider 提供方归一化；无模型列表时返回 false
func NormalizeProvider(p config.Provider, requireModels bool) (config.Provider, bool) {
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	models := make([]config.Model, 0, len(p.Models))
	for _, m := range p.Models {
		nm := NormalizeModel(m)
		if nm.ID == "" {
			continue
		}
		models = append(models, nm)
	}
	id := Slug(p.ID, "")
	if id == "" {
		id = Slug(p.Label, "")
	}
	if id == "" || baseURL == "" || (requireModels && len(models) == 0) {
		return config.Provider{}, false
	}
	style := strings.ToLower(strings.TrimSpace(p.ThinkingStyle))
	if !contains(ThinkingStyles, style) {
		style = "none"
	}
	timeout := p.Timeout
	if timeout < 1 {
		timeout = DefaultTimeout
	}
	label := strings.TrimSpace(p.Label)
	if label == "" {
		label = id
	}
	keyRef := strings.TrimSpace(p.KeyRef)
	keyEnv := strings.ToUpper(strings.TrimSpace(p.KeyEnv))
	// 内置提供方缺 key_ref 时按种子补齐，避免用户填的密钥被丢弃
	if p.Builtin && keyRef == "" {
		for _, seed := range Seed() {
			if seed.ID == id {
				keyRef = seed.KeyRef
				break
			}
		}
	}
	return config.Provider{
		ID:            id,
		PresetID:      strings.TrimSpace(p.PresetID),
		Label:         label,
		Group:         p.Group,
		BaseURL:       baseURL,
		Protocol:      ProtocolOf(p),
		Enabled:       p.Enabled,
		Builtin:       p.Builtin,
		KeyRef:        keyRef,
		KeyEnv:        keyEnv,
		ThinkingStyle: style,
		Timeout:       timeout,
		Stream:        p.Stream,
		Models:        models,
		DocsURL:       p.DocsURL,
		Notice:        p.Notice,
	}, true
}

// NormalizeProviders 整表归一化，去重去无效
func NormalizeProviders(list []config.Provider) []config.Provider {
	seen := map[string]bool{}
	out := make([]config.Provider, 0, len(list))
	for _, item := range list {
		p, ok := NormalizeProvider(item, true)
		if !ok || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	return out
}

// EngineName 引擎名 = provider:model
func EngineName(p config.Provider, m config.Model) string {
	return p.ID + ":" + m.ID
}

// ProviderLabel 卡片标签
func ProviderLabel(p config.Provider, m config.Model) string {
	label := m.Label
	if label == "" {
		label = m.ID
	}
	return p.Label + " · " + label
}

// ResolveKey 配置 key → 环境变量 → 空
// ResolveKey 解析提供方密钥。顺序严格对齐 translate.py:334 resolve_key：
// 声明了 key_env 时环境变量优先，其次才是设置里保存的 key_ref；
// 没有 key_env 的 acctoken 系提供方回落到 ACCTOKEN_API_KEY。
func ResolveKey(p config.Provider, keys config.APIKeys, environ func(string) string) string {
	ref := strings.TrimSpace(p.KeyRef)
	env := strings.ToUpper(strings.TrimSpace(p.KeyEnv))
	if env != "" && environ != nil {
		if v := strings.TrimSpace(environ(env)); v != "" {
			return v
		}
	}
	if ref != "" {
		if v := keyByRef(keys, ref); v != "" {
			return v
		}
	}
	if env == "" && ref == "acctoken" && environ != nil {
		return strings.TrimSpace(environ("ACCTOKEN_API_KEY"))
	}
	return ""
}

func keyByRef(keys config.APIKeys, ref string) string {
	switch ref {
	case "deepl":
		return keys.DeepL
	case "baidu_appid":
		return keys.BaiduAppID
	case "baidu_key":
		return keys.BaiduKey
	case "niutrans":
		return keys.NiuTrans
	case "ocrspace":
		return keys.OCRSpace
	case "baidu_ocr_ak":
		return keys.BaiduOCRAK
	case "baidu_ocr_sk":
		return keys.BaiduOCRsk
	case "agnes":
		return keys.Agnes
	case "qwen":
		return keys.Qwen
	case "acctoken":
		return keys.Acctoken
	}
	return ""
}

// ChatURL 拼接对话端点
func ChatURL(p config.Provider) string {
	return strings.TrimRight(p.BaseURL, "/") + ProtocolPaths[ProtocolOf(p)]
}

// ModelsURL 拼接模型列表端点
func ModelsURL(p config.Provider) string {
	return strings.TrimRight(p.BaseURL, "/") + "/models"
}

// AuthHeaders 认证头
func AuthHeaders(p config.Provider, credential string) map[string]string {
	h := map[string]string{}
	switch ProtocolOf(p) {
	case "anthropic-messages":
		if credential != "" {
			h["x-api-key"] = credential
		}
		h["anthropic-version"] = AnthropicVersion
	default:
		if credential != "" {
			h["Authorization"] = "Bearer " + credential
		}
	}
	h["Content-Type"] = "application/json"
	return h
}

// ThinkingFields 按风格生成思考参数（translate.py:381）
func ThinkingFields(p config.Provider, m config.Model) map[string]any {
	if m.Reasoning == "off" || m.ThinkingParam == "none" {
		return nil
	}
	effort := m.Reasoning
	budget := m.ThinkingBudget
	if budget <= 0 {
		budget = 4096
	}
	switch p.ThinkingStyle {
	case "enable_thinking":
		return map[string]any{"enable_thinking": true}
	case "thinking_object":
		return map[string]any{"thinking": map[string]any{"type": "enabled"}}
	case "reasoning_split":
		return map[string]any{"reasoning_split": true}
	case "adaptive":
		return map[string]any{"thinking": map[string]any{"type": "adaptive"}}
	}
	switch m.ThinkingParam {
	case "reasoning_effort":
		return map[string]any{"reasoning_effort": effort}
	case "thinking_budget":
		if budget < AnthropicMinThinkBudget {
			budget = AnthropicMinThinkBudget
		}
		return map[string]any{"thinking_budget": budget}
	}
	return nil
}

// GenerationLimit 输出上限
func GenerationLimit(m config.Model) int {
	if m.ThinkingBudget > 0 {
		return m.ThinkingBudget
	}
	return DefaultMaxTokens
}

// BuildChatPayload 组请求体（translate.py:425）
func BuildChatPayload(p config.Provider, m config.Model, system, user string) map[string]any {
	limit := GenerationLimit(m)
	body := map[string]any{"model": m.ID}
	switch ProtocolOf(p) {
	case "anthropic-messages":
		body["system"] = system
		body["messages"] = []map[string]any{
			{"role": "user", "content": []map[string]any{{"type": "text", "text": user}}},
		}
		body["max_tokens"] = limit
	case "openai-responses":
		body["instructions"] = system
		body["input"] = []map[string]any{
			{"role": "user", "content": []map[string]any{{"type": "input_text", "text": user}}},
		}
		body["max_output_tokens"] = limit
	default:
		body["messages"] = []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		}
		body["max_tokens"] = limit
	}
	if m.Temperature != nil {
		body["temperature"] = *m.Temperature
	}
	for k, v := range ThinkingFields(p, m) {
		body[k] = v
	}
	return body
}
