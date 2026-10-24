package providers

import (
	"github.com/local/zshtrans/internal/config"
)

// Seed 初始提供方卡片（translate.py:188 seed_providers）
func Seed() []config.Provider {
	temp := func(v float64) *float64 { return &v }
	return []config.Provider{
		{
			ID: "bailian", Label: "百炼",
			BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Protocol: "openai-completions", Enabled: true, Builtin: true,
			KeyRef: "qwen", ThinkingStyle: "enable_thinking",
			Timeout: DefaultTimeout,
			Models: []config.Model{
				{ID: "glm-5.2", Label: "GLM-5.2", Reasoning: "low", ThinkingParam: "reasoning_effort", ThinkingBudget: 256, Temperature: temp(0.3)},
				{ID: "qwen3.7-max-2026-06-08", Label: "Qwen3.7-Max(2026-06-08)", Reasoning: "medium", ThinkingParam: "thinking_budget", ThinkingBudget: 4096, Temperature: temp(0.3)},
			},
		},
		{
			ID: "acctoken", Label: "Acctoken",
			BaseURL:  "https://www.acctoken.com/v1",
			Protocol: "openai-completions", Enabled: true, Builtin: true,
			KeyRef: "acctoken", KeyEnv: "ACCTOKEN_API_KEY", ThinkingStyle: "thinking_object",
			Timeout: DefaultTimeout,
			Models: []config.Model{
				{ID: "deepseek-v4.1-flash", Label: "DeepSeek V4.1 Flash", Reasoning: "off", ThinkingParam: "none", Temperature: temp(0.3)},
			},
		},
	}
}

type presetRow struct {
	id, label, group, baseURL, protocol, style, docs, notice string
	models                                                   [][3]string // {id, image(bool as "1"/"0"), effort}
}

// Presets 24 个官方预设（translate.py:251 provider_presets）
func Presets() []config.Provider {
	codingNote := "套餐 Key 与按量 API Key 分开保存；仅在官方允许的工具/场景使用，默认不启用。"
	rows := []presetRow{
		{"bailian", "阿里云百炼", "国内厂商", "https://dashscope.aliyuncs.com/compatible-mode/v1", "openai-completions", "none",
			"https://help.aliyun.com/en/model-studio/what-is-model-studio", "地域或工作空间不同，请使用控制台对应的地址。",
			[][3]string{{"qwen3.8-max", "0", "off"}}},
		{"deepseek", "DeepSeek", "国内厂商", "https://api.deepseek.com/v1", "openai-completions", "thinking_object",
			"https://api-docs.deepseek.com/quick_start/pricing/", "",
			[][3]string{{"deepseek-flash", "1", "off"}, {"deepseek-v4-pro", "0", "off"}}},
		{"zhipu", "智谱 GLM", "国内厂商", "https://open.bigmodel.cn/api/paas/v4", "openai-completions", "thinking_object",
			"https://docs.bigmodel.cn/cn/guide/models/text/glm-5.3", "GLM-5.3 必须开启思考；轻量档使用 low。",
			[][3]string{{"glm-5.3", "0", "low"}}},
		{"moonshot", "月之暗面 Kimi", "国内厂商", "https://api.moonshot.cn/v1", "openai-completions", "none",
			"https://www.kimi.com/ai-models/kimi-k3", "开放平台按量 API，与 Kimi Code 订阅分开。",
			[][3]string{{"kimi-k3", "1", "off"}}},
		{"volcengine", "火山方舟 / 豆包", "国内厂商", "https://ark.cn-beijing.volces.com/api/v3", "openai-responses", "none",
			"https://docs.volcengine.com/docs/ark/quick-start", "也可将模型 ID 替换为控制台已开通的接入点 ID。",
			[][3]string{{"doubao-seed-2-1-pro-260915", "0", "off"}}},
		{"hunyuan", "腾讯混元", "国内厂商", "https://api.hunyuan.cloud.tencent.com/v1", "openai-completions", "none",
			"https://cloud.tencent.com/document/product/1729/111007", "",
			[][3]string{{"hunyuan-turbos-latest", "0", "off"}, {"hunyuan-vision", "1", "off"}}},
		{"qianfan", "百度千帆 / 文心", "国内厂商", "https://qianfan.baidubce.com/v2", "openai-completions", "none",
			"https://cloud.baidu.com/doc/qianfan-api/s/Dmba8k71y", "",
			[][3]string{{"ernie-5.0", "0", "off"}}},
		{"minimax", "MiniMax", "国内厂商", "https://api.minimax.cn/v1", "openai-completions", "reasoning_split",
			"https://platform.minimax.cn/docs/api-reference/text-openai-api", "思考内容与译文分开接收。",
			[][3]string{{"MiniMax-M3", "1", "off"}}},
		{"stepfun", "阶跃星辰 StepFun", "国内厂商", "https://api.stepfun.com/v1", "openai-completions", "none",
			"https://platform.stepfun.com/", "",
			[][3]string{{"step-5-preview", "0", "off"}, {"step-3.7-flash", "0", "off"}}},
		{"openai", "OpenAI", "国外厂商", "https://api.openai.com/v1", "openai-responses", "none",
			"https://developers.openai.com/api/docs/models", "使用 API Key；ChatGPT/Codex 登录订阅不能直接填写为 API Key。",
			[][3]string{{"gpt-6.1-sol", "1", "low"}, {"gpt-6-luna", "1", "low"}, {"gpt-6-astra", "1", "low"}}},
		{"anthropic", "Anthropic / Claude", "国外厂商", "https://api.anthropic.com/v1", "anthropic-messages", "adaptive",
			"https://platform.claude.com/docs/en/models/overview", "当前模型采用自适应思考；使用 Claude Console API Key。",
			[][3]string{{"claude-sonnet-5-5", "1", "medium"}, {"claude-haiku-5-5", "1", "medium"}, {"claude-opus-5-5", "1", "medium"}}},
		{"gemini", "Google Gemini", "国外厂商", "https://generativelanguage.googleapis.com/v1beta/openai", "openai-completions", "none",
			"https://ai.google.dev/gemini-api/docs/openai", "使用 AI Studio API Key 与 OpenAI 兼容接口。",
			[][3]string{{"gemini-3.8-flash", "1", "low"}}},
		{"xai", "xAI / Grok", "国外厂商", "https://api.x.ai/v1", "openai-responses", "none",
			"https://docs.x.ai/overview", "仅提取实际文本；不展示 encrypted reasoning。",
			[][3]string{{"grok-4.7", "0", "off"}}},
		{"mistral", "Mistral AI", "国外厂商", "https://api.mistral.ai/v1", "openai-completions", "none",
			"https://docs.mistral.ai/api", "",
			[][3]string{{"mistral-small-latest", "1", "off"}, {"mistral-large-latest", "0", "off"}}},
		{"groq", "Groq", "国外厂商", "https://api.groq.com/openai/v1", "openai-completions", "none",
			"https://console.groq.com/docs/openai", "填入 Key 后获取账号当前可用模型。", nil},
		{"siliconflow", "硅基流动 SiliconFlow", "第三方平台", "https://api.siliconflow.cn/v1", "openai-completions", "none",
			"https://docs.siliconflow.cn/docs/api/chat-completions-post", "",
			[][3]string{{"deepseek-ai/DeepSeek-V4-Flash", "0", "off"}}},
		{"openrouter", "OpenRouter", "第三方平台", "https://openrouter.ai/api/v1", "openai-completions", "none",
			"https://openrouter.ai/docs/quickstart", "模型会有不同路由和价格；从账号目录勾选。", nil},
		{"together", "Together AI", "第三方平台", "https://api.together.xyz/v1", "openai-completions", "none",
			"https://docs.together.ai/docs/quickstart", "填入 Key 后获取账号当前可用模型。", nil},
		{"bailian-coding", "阿里云 Coding Plan（国内）", "Coding Plan", "https://coding.dashscope.aliyuncs.com/v1", "openai-completions", "none",
			"https://help.aliyun.com/zh/model-studio/coding-plan", codingNote,
			[][3]string{{"qwen3.7-plus", "1", "off"}}},
		{"bailian-coding-intl", "阿里云 Coding Plan（国际）", "Coding Plan", "https://coding-intl.dashscope.aliyuncs.com/v1", "openai-completions", "none",
			"https://qwenlm.github.io/qwen-code-docs/zh/users/configuration/auth/", codingNote,
			[][3]string{{"qwen3.7-plus", "1", "off"}}},
		{"glm-coding", "智谱 GLM Coding Plan", "Coding Plan", "https://open.bigmodel.cn/api/coding/paas/v4", "openai-completions", "thinking_object",
			"https://docs.bigmodel.cn/cn/coding-plan/tool/others", "官方仅允许指定工具与产品环境；当前翻译工具未在支持名单。",
			[][3]string{{"glm-5.3", "0", "low"}, {"glm-5.3-flash", "0", "low"}}},
		{"kimi-coding", "Kimi Code 订阅", "Coding Plan", "https://api.kimi.com/coding/v1", "anthropic-messages", "none",
			"https://www.kimi.com/code/docs/en/third-party-tools/hermes.html", codingNote,
			[][3]string{{"k3-256k", "0", "off"}}},
		{"minimax-plan", "MiniMax M Plan（原 Coding / Token Plan）", "Coding Plan", "https://api.minimax.cn/v1", "openai-completions", "reasoning_split",
			"https://platform.minimax.cn/docs/m-plan/other-tools", "与按量 API 使用同一地址，但必须填写 M Plan 订阅 Key。",
			[][3]string{{"MiniMax-M3.1-Flash-Preview", "1", "low"}}},
		{"volcengine-coding", "火山方舟 Coding Plan", "Coding Plan", "https://ark.cn-beijing.volces.com/api/coding/v3", "openai-completions", "none",
			"https://www.volcengine.com/docs/82379/1925114", codingNote,
			[][3]string{{"ark-code-latest", "0", "off"}}},
		{"tencent-coding", "腾讯云 TokenHub Coding Plan", "Coding Plan", "https://api.lkeap.cloud.tencent.com/coding/v3", "openai-completions", "none",
			"https://cloud.tencent.com/document/product/1823/130092", "官方限制编程工具，禁止用于自定义应用后端与非交互批量 API 调用。",
			[][3]string{{"tc-code-latest", "0", "off"}}},
		{"qianfan-coding", "百度千帆 Coding Plan", "Coding Plan", "https://qianfan.baidubce.com/v2/coding", "openai-completions", "none",
			"https://cloud.baidu.com/doc/qianfan/s/ymmyn5kc2", codingNote, nil},
		{"step-plan", "阶跃星辰 Step Plan", "Coding Plan", "https://api.stepfun.com/step_plan/v1", "openai-completions", "none",
			"https://platform.stepfun.com/docs/zh/step-plan/integrations/reasoning-api", codingNote,
			[][3]string{{"step-5-preview", "0", "off"}, {"step-router-v1", "0", "off"}}},
	}
	out := make([]config.Provider, 0, len(rows))
	for _, r := range rows {
		models := make([]config.Model, 0, len(r.models))
		for _, m := range r.models {
			models = append(models, config.Model{
				ID: m[0], Label: m[0], Image: m[1] == "1",
				Reasoning: m[2], ThinkingParam: pickParam(m[2]),
			})
		}
		keyRef := "preset_" + r.id
		if r.id == "bailian" {
			keyRef = "qwen"
		}
		out = append(out, config.Provider{
			ID: r.id, PresetID: r.id, Label: r.label, Group: r.group,
			BaseURL: r.baseURL, Protocol: r.protocol,
			Enabled: r.group != "Coding Plan", Builtin: true,
			KeyRef: keyRef, ThinkingStyle: r.style,
			Timeout: 60, Stream: false,
			Models: models, DocsURL: r.docs, Notice: r.notice,
		})
	}
	return out
}

func pickParam(effort string) string {
	if effort == "off" {
		return "none"
	}
	return "reasoning_effort"
}

// Groups 预设分组顺序
var Groups = []string{"国内厂商", "国外厂商", "Coding Plan", "第三方平台"}
