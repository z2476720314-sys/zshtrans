# ZSH TRANS v3.4.1 功能清单（重构验收基线）

> 来源：`translate/translate_tool_core.pyc`（v1.1 核心，反编译字节码）+ `translate/translate.py`（v2.38 外壳）。
> 保留原功能逐项基线。下表的“已实现”指源码中已有对应入口或行为；最后一列说明本轮验证边界，不代表真实桌面、所有提供方与全部业务样本均已验收。原始行号对应 v2.38 源码，后续移动不改变功能编号。

## 0. 版本与常量

| # | 功能 | 原始位置 | Go 实现 | 本轮验证边界 |
|---|------|----------|---------|---|
| 0.1 | 应用名 `ZSH TRANS` / 版本 `v2.38` | `translate.py:3152` | 已实现：`internal/config` `AppName`/`AppVersion` | 常量 / 源码对照，配色允许改版 |
| 0.2 | 16 种语言表 `LANGUAGES` | `translate.py:3185` | 已实现：`internal/lang` `Languages` | 常量 / 源码对照，配色允许改版 |
| 0.3 | `FLIP` 语向翻转表（`translate.py:4338` 扩为「除 auto/zh 外一律→zh」） | pyc + `translate.py:4338` | 已实现：`internal/lang` `Flip`（`init()` 补齐 16 语言） | 常量 / 源码对照，配色允许改版 |
| 0.4 | 输出标签清理表 `TRANSLATION_OUTPUT_LABELS`（15 项） | `translate.py:3164` | 已实现：`internal/providers/prompt.go` `outputLabels` | 常量 / 源码对照，配色允许改版 |
| 0.5 | 请求超时 8s / 缓存 300 条 | pyc `TIMEOUT`/`CACHE_MAX` | 已实现：`internal/config` `Timeout`/`CacheMax` | 常量 / 源码对照，配色允许改版 |
| 0.6 | 主题配色 `C_*`（12 色） | pyc | 已实现：`frontend/src/styles.css` CSS 变量（**有意重设计**，见下） | 常量 / 源码对照，配色允许改版 |
| 0.7 | 5 套主题方案 `THEME_SCHEMES`（laser/plasma/aurora/gold/ice） | pyc | **有意不移植**：用户 2026-10-24 明确「UI 配色等可自由发挥不必遵循原版配色」 | 配色与方向按允许的改版取舍 |
| 0.8 | 4 种背景方向 `THEME_DIRECTIONS` | pyc | **有意不移植**：同上，背景改为单一极光渐变 | 配色与方向按允许的改版取舍 |

## 1. 配置（`tt_config.json`）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 1.1 | 默认配置结构（hotkeys/src_lang/tgt_lang/default_provider/web/api_keys/providers/ocr_model） | pyc `DEFAULT_CONFIG` | 已实现：`config.Default()` | 迁移 / 配置回归；旧文件保持不变 |
| 1.2 | 读配置（缺失字段用默认值补齐） | pyc `load_config` | 已实现：`config.Load()` | 迁移 / 配置回归；旧文件保持不变 |
| 1.3 | 写配置（原子写、ensure_ascii=false、indent=2） | pyc `save_config` | 已实现：`config.Save()` | 迁移 / 配置回归；旧文件保持不变 |
| 1.4 | 配置与常用语路径 | pyc `_config_path` | 已实现：`config.BaseDir()` / `Path()`：可写 exe 目录优先，否则使用 Roaming 用户数据目录 | 迁移 / 配置回归；旧文件保持不变 |
| 1.5 | `CFG` 全局内存态 + 热更新 | pyc | 已实现：`config.Store` | 迁移 / 配置回归；旧文件保持不变 |
| 1.6 | 首次导入旧 Roaming 配置与常用语 | 原版用户数据目录 | 已实现：`config.MigrateLegacy()`；新目录有配置时不导入，旧文件不改动 | 迁移 / 配置回归；旧文件保持不变 |

## 2. 常用语（`tt_phrases.json`）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 2.1 | 结构 `{id, desc, content}` | pyc `load_phrases` | 已实现：`phrases.Item` | CRUD 实现对照；未操作真实常用语数据 |
| 2.2 | 读取（坏文件回退空表并打印告警） | pyc `load_phrases` | 已实现：`phrases.Load()` | CRUD 实现对照；未操作真实常用语数据 |
| 2.3 | 写入（`ensure_ascii=False, indent=2`） | pyc `save_phrases` | 已实现：`phrases.Save()` | CRUD 实现对照；未操作真实常用语数据 |
| 2.4 | 新增 id 生成 `_new_phrase_id` | pyc | 已实现：`phrases.NewID()` | CRUD 实现对照；未操作真实常用语数据 |
| 2.5 | 内置种子常用语 `_seed_bundled` | pyc | 已实现：`phrases.Seed()` | CRUD 实现对照；未操作真实常用语数据 |
| 2.6 | 增 / 改 / 删 / 复制到剪贴板 | `translate.py:2909-2967` | 已实现：`internal/server` + 前端 | CRUD 实现对照；未操作真实常用语数据 |

## 3. 语言检测与语向决策

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 3.1 | 西里尔 → ru，CJK → zh，其余 → en | pyc `detect_lang` | 已实现：`lang.Detect()` | 语向回归；语义质量限于有限样本 |
| 3.2 | 扩展检测：日语假名→ja、韩文→ko、阿拉伯→ar、泰文→th | `translate.py:4331` | 已实现：`lang.Detect()` | 语向回归；语义质量限于有限样本 |
| 3.3 | `resolve_direction(text, src, tgt)`：auto 时按检测结果；检测==目标则翻转 | pyc | 已实现：`lang.Resolve()` | 语向回归；语义质量限于有限样本 |
| 3.4 | 快捷翻译：非中文统一译为中文 | `translate.py:4350` | 已实现：`lang.Decide()`；手动翻译仍用 `Resolve()` | 语向回归；语义质量限于有限样本 |
| 3.5 | 中文快捷翻译取所选外语，无外语选择才回退俄语 | `translate.py:4353-4358` | 已实现：`lang.Decide()` | 语向回归；语义质量限于有限样本 |
| 3.6 | 交换语向 `_swap_lang`：`tgt=src`（src 非 auto）或 `FLIP[tgt]`，`src=旧tgt`，两者相同则 src 归 auto | `translate.py:2874` | 已实现：`lang.Swap()` + `server` `/api/swap` | 语向回归；语义质量限于有限样本 |

## 4. 内置免费/直连翻译提供方（9 个）

| # | 提供方 | 端点 | Go | 本轮验证边界 |
|---|--------|------|-----|---|
| 4.1 | `google` Google 网页翻译 | `translate.googleapis.com/translate_a/single` client=gtx | 已实现：`providers/direct.go` `tGoogle` | 接口实现；未逐平台线上验收 |
| 4.2 | `mymemory` MyMemory | `api.mymemory.translated.net/get` | 已实现：`providers/direct.go` `tMyMemory` | 接口实现；未逐平台线上验收 |
| 4.3 | `deepl` DeepL Free | `api-free.deepl.com/v2/translate` | 已实现：`providers/direct.go` `tDeepL` | 接口实现；未逐平台线上验收 |
| 4.4 | `baidu` 百度翻译 | `fanyi-api.baidu.com/api/trans/vip/translate`（MD5 签名） | 已实现：`providers/direct.go` `tBaidu` | 接口实现；未逐平台线上验收 |
| 4.5 | `niutrans` 小牛翻译 | `api.niutrans.com/NiuTransServer/translation` | 已实现：`providers/direct.go` `tNiuTrans` | 接口实现；未逐平台线上验收 |
| 4.6 | `lingva` Lingva（双镜像） | `translate.plausibility.cloud` / `lingva.ml` | 已实现：`providers/direct.go` `tLingva` | 接口实现；未逐平台线上验收 |
| 4.7 | `agnes` Agnes 2.5 Flash | OpenAI 兼容 | 已实现：`providers/direct.go` `tAgnes` | 接口实现；未逐平台线上验收 |
| 4.8 | `qwen-max` GLM-5.2 | 百炼兼容接口 | 已实现：`providers/direct.go` `tQwenMax`/`tQwenMaxSnap` | 接口实现；未逐平台线上验收 |
| 4.9 | `qwen-max-2606` Qwen3.7-Max(2026-06-08) | 百炼兼容接口 | 已实现：`providers/direct.go` `tQwenMax`/`tQwenMaxSnap` | 接口实现；未逐平台线上验收 |

## 5. 翻译引擎（`TranslateEngine`）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 5.1 | 文本归一化（空白折叠）后作为缓存键 `(norm, src, tgt)` | pyc:570 | 已实现：`engine.Translate()` | 引擎回归；不保证上游可用性 |
| 5.2 | LRU 缓存，容量 300，命中返回 `provider="cache"`, `ms=0` | pyc `_cache_get/_cache_put` | 已实现：`engine.lru` | 引擎回归；不保证上游可用性 |
| 5.3 | 无可用提供方 → `no provider enabled` | pyc:577 | 已实现：`engine.ErrNoProvider` | 引擎回归；不保证上游可用性 |
| 5.4 | 首选提供方优先，其余轮转（`idx` 自增取模） | pyc:579-592 | 已实现：`engine.Translate()` | 引擎回归；不保证上游可用性 |
| 5.5 | 提供方健康度 `healthy()` / `mark_ok()` / `mark_fail()` + 冷却 | pyc `Provider` | 已实现：`providers.Provider.Health` | 引擎回归；不保证上游可用性 |
| 5.6 | 失败降级，全部失败保留逐条原因 | pyc:596-610 | 已实现：`engine.TranslateContext()` 返回聚合错误 | 引擎回归；不保证上游可用性 |
| 5.7 | 返回 `{text, provider, ms}` | pyc:604 | 已实现：`engine.Result` | 引擎回归；不保证上游可用性 |
| 5.8 | 并发安全（互斥锁保护 idx / 缓存） | pyc `lock` | 已实现：`sync.Mutex` | 引擎回归；不保证上游可用性 |

## 6. OCR（截图取字）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 6.1 | OCR.space 识别与图像上传 | pyc `ocr_space` | 已实现：`ocr/space.go`：Engine 3、自动语言，支持中俄识别 | 候选 / 缓存回归；线上仅两张合成图 |
| 6.2 | 百度 OCR（access_token 缓存，过期刷新） | pyc `ocr_baidu` | 已实现：`ocr/space.go` `Baidu()` + `baiduAccess()` | 候选 / 缓存回归；线上仅两张合成图 |
| 6.3 | 视觉大模型 OCR（`ocr_model` 指定的 image 模型） | pyc `ocr_qwen` | 已实现：`ocr/json.go` `Vision()` | 候选 / 缓存回归；线上仅两张合成图 |
| 6.4 | 多引擎并行与候选择优 | pyc `run_ocr` | 已实现：`ocr.RunContext()`：同一候选并行；无有效文字再试下一候选，支持取消 | 候选 / 缓存回归；线上仅两张合成图 |
| 6.5 | 图像预处理：Otsu 阈值二值化 + 白底扩展 | pyc `_otsu_threshold`/`_binary_white_bg` | 已实现：`ocr/image.go` | 候选 / 缓存回归；线上仅两张合成图 |
| 6.6 | 候选集生成 `_ocr_candidates`（原图/二值/放大） | pyc | 已实现：`ocr.Candidates()` | 候选 / 缓存回归；线上仅两张合成图 |
| 6.7 | OCR 结果缓存 | pyc `_OCR_CACHE` | 已实现：`ocr` 缓存按模型、配置和代次隔离；设置变更清理 | 候选 / 缓存回归；线上仅两张合成图 |

## 7. HTTP 服务（原自带 Web 界面）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 7.1 | HTTP 界面服务与可配置端口 | pyc `start_web` | 已实现：`internal/server`；桌面固定回环，显式 `-web` 才采用 Web 监听配置 | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.2 | `GET /` `GET /index.html` → 内置 HTML | pyc `do_GET` | 已实现：`server` 静态资源 | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.3 | `GET /api/status` → `{providers:[{name,healthy}]}` | pyc:1154 | 已实现：`server` `handleStatus` | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.4 | `POST /api/translate` `{text,src,tgt}` → `{text,provider,ms,src}` | pyc:1176 | 已实现：`server` `handleTranslate` | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.5 | 语言白名单校验，越界 400 `bad lang` | pyc:1172 | 已实现：`server`。**有意扩展**：原 pyc 只认 `src∈{auto,ru,en,zh}` / `tgt∈{ru,en,zh}`（内置网页仅 3 语种）；桌面端按 `LANGUAGES` 16 种放开，否则 ja/ko/fr… 一律 400 | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.6 | 空文本 → 400 `empty text` | pyc:1170 | 已实现：`server` | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.7 | 未知路由 → 404 `{error:"not found"}` | pyc:1158 | 已实现：`server` | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.8 | 局域网 IP 探测 `lan_ip` | pyc `lan_ip` | 已实现：`server.LanIP()` | 接口 / 来源校验回归；未做 LAN 设备实测 |
| 7.9 | 静默日志 `log_message`（不刷屏） | pyc | 已实现：`logx` | 接口 / 来源校验回归；未做 LAN 设备实测 |

## 8. 多模型提供方体系（`translate.py`）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 8.1 | `normalize_provider` / `normalize_model` 归一化 | :139/:104 | 已实现：`providers.Normalize*` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.2 | `seed_providers` 初始提供方（含 `ACCTOKEN_API_KEY` 环境变量） | :188 | 已实现：`providers.Seed()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.3 | `provider_presets` 24 个预设（国内厂商/国外厂商/Coding Plan/第三方平台） | :251 | 已实现：`providers.Presets()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.4 | `engine_name` = `provider:model` | :325 | 已实现：`providers.EngineName()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.5 | `resolve_key`：配置 key → 环境变量 → 空 | :333 | 已实现：`providers.ResolveKey()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.6 | 三种协议：`openai-completions` / `openai-responses` / `anthropic-messages` | :98 | 已实现：`providers.Protocol*` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.7 | `chat_url` / `models_url` 拼接 | :357/:362 | 已实现：`providers.ChatURL/ModelsURL` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.8 | `auth_headers`（Bearer / x-api-key + anthropic-version） | :366 | 已实现：`providers.AuthHeaders()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.9 | 5 种思考参数风格 `_thinking_fields` | :381 | 已实现：`providers.ThinkingFields()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.10 | 模型能力上限与本次请求输出上限 | :132/:417-422 | 已实现：`config.Model.OutputLimit` / `providers.GenerationLimit()` 分别保存、校验 | 协议 / 模型回归；非全部提供方线上验收 |
| 8.11 | `build_chat_payload` 组包 | :425 | 已实现：`providers.BuildChatPayload()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.12 | `extract_completion_text` 三种协议解包 | :534 | 已实现：`providers.ExtractText()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.13 | 流式增量 `_stream_delta` / `collect_stream_text` | :559/:587 | 已实现：`providers.CollectStream()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.14 | `make_translator` 生成翻译函数 | :625 | 已实现：`providers.TranslateWith()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.15 | `make_vision_ocr` 生成视觉 OCR 函数 | :674 | 已实现：`providers.VisionOCR()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.16 | `fetch_models` 拉取账号模型列表 | :874 | 已实现：`providers.FetchModels()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.17 | `test_connection` 连通性测试 | :879 | 已实现：`providers.TestConnection()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.18 | `vision_models` 可用视觉模型 | :906 | 已实现：`providers.VisionModels()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.19 | `usable_providers` 可用提供方 | :937 | 已实现：`providers.UsableProviders()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.20 | `model_config_from_catalogue` 从目录生成模型配置 | :793 | 已实现：`providers.ModelFromCatalogue()` | 协议 / 模型回归；非全部提供方线上验收 |
| 8.21 | 手动添加模型与请求输出上限编辑 | 原 Qt 模型卡片 | 已实现：`frontend/src/pages/Settings.tsx`，保留模型能力元数据 | 协议 / 模型回归；非全部提供方线上验收 |

## 9. 翻译提示词与输出清洗

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 9.1 | 俄跨境客服翻译契约（语体/情绪/俚语/电商术语/型号保护/示例） | :3410 | 已实现：`providers/prompt.go` `BuildPrompt()` | 清洗回归；完整双向语义集未完成 |
| 9.2 | 输出标签剥离（15 个标签，含 `标签：` 前缀与整行标题） | :3476 | 已实现：`providers.CleanOutput()` | 清洗回归；完整双向语义集未完成 |
| 9.3 | 目标俄语仍含汉字 → 报错 | :3476 | 已实现：`providers.CleanOutput()` | 清洗回归；完整双向语义集未完成 |
| 9.4 | 目标中文仍含西里尔 → 报错 | :3476 | 已实现：`providers.CleanOutput()` | 清洗回归；完整双向语义集未完成 |
| 9.5 | `extract_chat_completion` 非空校验 | :3529 | 已实现：`providers.ExtractText()` | 清洗回归；完整双向语义集未完成 |

## 10. 全局快捷键（Win32）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 10.1 | 划词、截图、主窗显隐、隐藏到托盘四个热键 | 原 Qt 运行时与配置 | 已实现：`hotkeys.Manager.Bind()` / `Rebind()` | 受控 Win32 测试；真实按键未复验 |
| 10.2 | 组合键解析 `parse_native_hotkey`（ctrl/alt/shift/win + 键名） | :3744 | 已实现：`hotkeys.Parse()` | 受控 Win32 测试；真实按键未复验 |
| 10.3 | `RegisterHotKey` 注册 + 消息循环 | :3790 | 已实现：`hotkeys.Manager` | 受控 Win32 测试；真实按键未复验 |
| 10.4 | 冲突检测（同一组合重复） | :2446 | 已实现：`hotkeys.Validate()` | 受控 Win32 测试；真实按键未复验 |
| 10.5 | 修饰键状态查询 | :3240 | 已实现：`capture.ModifiersDown()`；划词等待修饰键释放 | 受控 Win32 测试；真实按键未复验 |
| 10.6 | 剪贴板序列号 `_win32_clipboard_sequence` | :3245 | 已实现：`capture.ClipboardSeq()` | 受控 Win32 测试；真实按键未复验 |
| 10.7 | 模拟复制与发送结果检查 | :3269 | 已实现：`capture` 使用 `SendInput` 并验证事件数量 | 受控 Win32 测试；真实按键未复验 |
| 10.8 | 按剪贴板序列取选中文本 | :3350 | 已实现：`capture.SelectedText()`；保存可安全快照的 HGLOBAL 全部格式，不支持的对象在复制前拒绝 | 受控 Win32 测试；真实按键未复验 |
| 10.9 | 结果窗全局外部点击关闭 | 原 `PopupClickMonitor` | 已实现：`internal/deskwin/result_windows.go`，不是仅监听网页内点击 | 受控 Win32 测试；真实按键未复验 |

## 11. 截图取词（原生捕获）

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 11.1 | 原生框选、全屏与区域截图 | 原 `CaptureSession` / `NativeCapture` | 已实现：`capture.Interactive()`；`Screenshot()` / `Region()` 保留为基础 API | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.2 | 原生运行时与文件校验 | :1194/:1388 | 已实现：`capture.EnsureRuntime()`：内嵌 archive，按 manifest 验证 71 文件 SHA-256 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.3 | 截图 → OCR → 翻译 → 独立结果窗 | :1412 | 已实现：`server.capturePopup()` / `processCapture()`；显示实际 OCR 引擎和处理总耗时 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.4 | 工具栏修正与 Camera 文件校验 | :1095/:1090 | 已实现：`capture` 校验 SHA、进程路径和原始指令后，仅修正自己的子进程 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.5 | 标注、保存与复制图片 | 原 QQ 截图工具栏 | 已实现：由内嵌 QQ 原生组件执行，普通完成不触发 OCR | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.6 | 长截图和滚动拼接 | 原 QQ 长截图工具栏 | 已实现：由 QQ 原生组件执行，不以单张 GDI 截图替代 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.7 | 钉图及其生命周期 | 原 QQ 钉图窗口 | 已实现：保留仍可见的钉图子进程，其他完成场景回收自有进程 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.8 | 仅提取文字，不调用翻译 | 原 `onScreenShotRequestOCR` | 已实现：`CaptureResult.Action=ocr` 与 `server.processCapture()` | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.9 | 取消、完成与延迟回调隔离 | 原 `NativeCapture._poll()` | 已实现：`captureState` 保留取消后延迟翻译宽限，丢弃旧进程代次回调 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.10 | 截图前后保持主窗口可见状态 | 原 `NativeCapture.start()` | 已实现：`server.capturePopup()` 隐藏并按原状态恢复 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |
| 11.11 | 内嵌资源与首用释放 | 原打包原生运行资源 | 已实现：archive 12,708,090 字节，包含 71 原始文件、manifest 与许可；无 Python 运行依赖 | 资源 / 协议 / DLL 检查；真实捕获交互未复验 |

## 12. 主界面（Qt → Web）

| # | 功能 | 原始 | Go/前端 | 本轮验证边界 |
|---|------|------|---------|---|
| 12.1 | 标题 `ZSH TRANS v2.38`、660×800 窗口 | :2760 | 已实现：前端布局 | 浏览器界面检查；原生手工操作未验收 |
| 12.2 | 源/目标语言下拉 + 交换按钮 | :2788 | 已实现：前端 `LangBar` | 浏览器界面检查；原生手工操作未验收 |
| 12.3 | 原文输入框（占位「输入文字，或粘贴客户消息…」） | :2813 | 已实现：前端 | 浏览器界面检查；原生手工操作未验收 |
| 12.4 | `Ctrl+Enter` 翻译 | :2827 | 已实现：前端快捷键 | 浏览器界面检查；原生手工操作未验收 |
| 12.5 | 「翻译」主按钮 / 「截图翻译」按钮 | :2818 | 已实现：前端 | 浏览器界面检查；原生手工操作未验收 |
| 12.6 | 译文只读区 + 「复制译文」 | :2841 | 已实现：前端 | 浏览器界面检查；原生手工操作未验收 |
| 12.7 | 常用语列表 + 「添加」+ 计数 | :2852 | 已实现：前端 `Phrases` | 浏览器界面检查；原生手工操作未验收 |
| 12.8 | 状态栏（链路显示 `_paint_chain`：提供方名 + 健康点 + 本次使用高亮 + ms） | :2876 | 已实现：前端 `ChainBar` | 浏览器界面检查；原生手工操作未验收 |
| 12.9 | 系统托盘：显示主窗口 / 截图翻译 / 设置 / 退出 | :2848 | 已实现：`internal/tray`（fyne.io/systray 原生托盘，点击图标显示主窗口） | 浏览器界面检查；原生手工操作未验收 |
| 12.10 | 独立译文结果窗、复制与元信息 | 原 `ResultPopup` | 已实现：`internal/deskwin`：置顶、不主动激活、滚动、外点 / Esc / 15 秒关闭；Web 审查保留页内结果 | 浏览器界面检查；原生手工操作未验收 |
| 12.11 | 关闭窗口隐藏到托盘 | :3081 | 已实现：`internal/deskwin` 处理原生关闭；热键和托盘共用显隐控制 | 浏览器界面检查；原生手工操作未验收 |
| 12.12 | 翻译失败提示（原版 `QMessageBox` 弹窗展示 `all providers failed:\n…` 全文） | pyc `translate` 抛 `RuntimeError` | 已实现：`frontend/src/errors.ts` `briefError()`：toast 只显示一行摘要（`翻译失败（N 项）：<首条短因>`），完整逐条原因（含内联 HTML 折叠、URL 查询串折叠）折叠进右栏 `.errbox` 内部滚动，避免撑破两栏布局 | 浏览器界面检查；原生手工操作未验收 |
| 12.13 | 翻译草稿、页面切换与取消 | 本轮交互补齐 | 已实现：`Translate` / `App` 保留草稿并隔离取消、迟到响应 | 浏览器界面检查；原生手工操作未验收 |

## 13. 设置窗口（4 页）

| # | 功能 | 原始 | Go/前端 | 本轮验证边界 |
|---|------|------|---------|---|
| 13.1 | 侧栏导航：模型提供方 / 截图与 OCR / 快捷键 / 其他翻译接口 | :2147 | 已实现：前端 `Settings` | 浏览器设置页与保存检查 |
| 13.2 | 「默认翻译模型」下拉（含「自动路由」） | :2188 | 已实现：前端 | 浏览器设置页与保存检查 |
| 13.3 | 提供方卡片：id / 名称 / 地址 / 协议 / 启用 / 密钥 / 思考风格 / 超时 / 流式 | :1806 | 已实现：前端 `ProviderCard` | 浏览器设置页与保存检查 |
| 13.4 | 模型表格：模型 ID / 显示名 / 图片 / 推理档 / 温度 | :1806 | 已实现：前端 | 浏览器设置页与保存检查 |
| 13.5 | 「添加提供方」按 4 分组菜单（含第三方/自定义） | :2317 | 已实现：前端 | 浏览器设置页与保存检查 |
| 13.6 | 「恢复默认提供方」 | :2346 | 已实现：前端 + `providers.Seed()` | 浏览器设置页与保存检查 |
| 13.7 | 单卡「重置」回已保存值 | :2352 | 已实现：前端 | 浏览器设置页与保存检查 |
| 13.8 | 「识别模型」下拉（仅启用且支持图片的模型） | :2198 | 已实现：前端 | 浏览器设置页与保存检查 |
| 13.9 | 四项快捷键编辑和冲突校验 | 原配置与 Qt 设置 | 已实现：`Settings` + `hotkeys.Validate()`，注册失败明确回报 | 浏览器设置页与保存检查 |
| 13.10 | 其他接口密钥 8 项（密码框，占位显示「已保存（不显示）」/「未设置」） | :2213 | 已实现：前端 | 浏览器设置页与保存检查 |
| 13.11 | 保存校验：ID 重复 / 默认模型不可用 / OCR 模型非视觉 / 热键非法或冲突 | :2394 | 已实现：`server` `/api/settings` | 浏览器设置页与保存检查 |
| 13.12 | 保存后热重载（重建引擎 + 重注册热键） | :2462 | 已实现：`config.Store` + `engine.Rebuild()` | 浏览器设置页与保存检查 |

## 14. 其它

| # | 功能 | 原始 | Go | 本轮验证边界 |
|---|------|------|-----|---|
| 14.1 | 自检 `self_check()`（语言数/预设数/UI 工具包/字形/常用语数与哈希） | :4460 | 已实现：`server` `/api/selfcheck` | 结构信息核对；非完整业务验收 |
| 14.2 | 应用图标嵌入 | :4410 | 已实现：`internal/assets/app.ico`（`go:embed`，托盘与 exe 资源共用；exe 图标由 `rsrc_windows_amd64.syso` 嵌入） | 结构信息核对；非完整业务验收 |
| 14.3 | `resource_path` 打包资源定位 | :4455 | 已实现：`embed.FS` | 结构信息核对；非完整业务验收 |
| 14.4 | 单实例 / 退出清理 | :3088 | 已实现：`internal/instance`（命名互斥体 `Global\ZSHTRANS_SINGLE_INSTANCE`，二开时唤起已有实例并退出）+ 托盘退出 | 结构信息核对；非完整业务验收 |
| 14.5 | 日志文件 | — | 已实现：`logx` → `zshtrans.log` | 结构信息核对；非完整业务验收 |

## 15. 重构中的附加能力与交互调整

本节保留重构过程中新增或调整的能力记录；它们不替代前面的原功能基线。

| # | 新增 | 说明 | 落地 | 本轮验证边界 |
|---|------|------|------|---|
| 15.1 | WebView2 桌面窗口 | 原 Qt 窗口改为 WebView2；缺失运行时明确报错，不自动回退浏览器，`-web` 是显式选择 | `internal/deskwin`、`cmd/zshtrans/main.go` | 当前实现记录；完整边界见第 20 节 |
| 15.2 | 自主更新 | 可自动检查；显式下载后仍需另点安装并重启，支持取消 / 跳过 / 稍后，关闭弹窗不安装 | `internal/update`、`server/update.go`、`UpdatePanel.tsx` | 当前实现记录；完整边界见第 20 节 |
| 15.3 | 俄语领域提示词与有限评测 | 沿用领域提示词；本轮未做新调优，旧跨模型 / 跨样本均分不作为提升成绩 | `providers/prompt.go`；评测边界见第 20 节 | 有限样本；本轮未做新提示词调优 |
| 15.4 | OCR 候选处理 | 原图、二值图与适用的小图放大；已有有效结果时不继续遍历候选 | `internal/ocr` | 当前实现记录；完整边界见第 20 节 |
| 15.5 | OCR 失败原因可诊断 | 原版只报 `all OCR failed`；本版逐引擎逐候选列出（`引擎 [候选]: 原因`），并报真实 Otsu 阈值（原版写死 255） | `internal/ocr/ocr.go` `Run()` | 当前实现记录；完整边界见第 20 节 |
| 15.6 | 设置写入的字段级语义 | 前端只改一项时（如自检页关掉自动更新），不会把没提交的字段清空。`default_provider`/`ocr_model` 用指针区分「不涉及」与「清空」 | `internal/server/server.go` `settingsRequest` | 当前实现记录；完整边界见第 20 节 |
| 15.7 | 直连引擎可设为默认 | 原版默认模型只能选「多模型提供方」里的模型；本版 `default_provider` 也可填 `google`/`mymemory` 等直连引擎名 | `internal/server/server.go` `engineUsable()` | 当前实现记录；完整边界见第 20 节 |
| 15.8 | 显式 Web 共享模式 | 桌面默认只监听回环；显式共享时隐藏 LAN 端管理入口，本机事件禁止远端订阅 | `cmd/zshtrans`、`internal/server`、`frontend/src/App.tsx` | 当前实现记录；完整边界见第 20 节 |
| 15.9 | 界面语向选择器左右分置 | 源语言下拉贴在左栏表头、目标语言贴在右栏表头，中间只留交换按钮——一眼看清方向，且控件比原版紧凑 | `frontend/src/pages/Translate.tsx` | 当前实现记录；完整边界见第 20 节 |
| 15.10 | 日志带时间戳 | 每行 `YYYY-MM-DD HH:MM:SS ` 前缀，原版是裸 print | `internal/logx` | 当前实现记录；完整边界见第 20 节 |


## 16. v3.1 历史加固记录

该版本修正了全局热键注册与消息泵的线程归属、更新包长度和 PE 校验、配置深拷贝、请求体和图像限制、引擎健康度排序等问题。历史记录中的热键或更新联调，不代表 v3.4 已完成原生手工验收。

原先“剪贴板完全还原”的表述不准确：v3.4 才补齐可安全快照的 HGLOBAL 多格式恢复，并在不能安全快照时于复制前拒绝；恢复前检查序列，保留用户的新复制。有效 message-only owner 修正了 NULL owner 导致的写入失败。

## 17. v3.2 历史更新链路修复

保留按版本号选择 release、独立下载超时与断流重试、下载中允许读取进度的修复。旧版下载后自动安装的交互已被 v3.4 替换：下载完成后，必须由用户另点“安装并重启”。旧版本联调记录不等于本轮发行包安装验收。

## 18. v3.3 历史下载源修复

保留 API asset 端点优先、浏览器下载地址备用的双源机制，以及 `Accept: application/octet-stream`。历史网络可达性与下载结果仅对应当时环境，不承诺所有网络下的固定速度或可达性。

## 19. v3.4 补齐与验证入口

- 原生截图：`internal/capture/native*`、`bundled/qq-runtime.zip`、`scripts/capture-runtime.ps1`。核心回归涵盖路径和内容校验、回调复制、延迟取消顺序、操作类型、修饰键释放与 DLL 初始化。
- 剪贴板：`clipboard_windows.go` / `clipboard_windows_test.go`，验证有效隐藏 owner、清空失败、富文本多格式和后续复制保护；受控 shim 不接触用户剪贴板。
- 原生结果窗：`internal/deskwin/result*`；主窗口隐藏时仍可展示，置顶但不主动激活，支持复制、滚动、外点 / Esc / 15 秒关闭。真实桌面操作仍待使用验收。
- 配置迁移：`config.MigrateLegacy()` 仅向尚无配置的新目录导入旧 Roaming 数据，不改旧文件。本版没有承诺 `target_mode` 到新语向字段的自动转换。
- 前端：保留翻译草稿、取消请求、手动模型与输出上限；默认回环桌面模式和显式 Web 模式分开，LAN 页面隐藏管理功能。
- 同源边界：本机管理与事件流受限；Vite 开发代理保留原 Host / Origin 关系，支持正常设置保存。
- 更新：检查、下载、安装分开；关闭弹窗或稍后不安装，提供取消和跳过。
- 构建：使用现有工具与锁文件，先浏览器审查再打包，命令见 README。组件权利与许可见 `THIRD_PARTY_NOTICES.md`。

## 20. 本轮实际验证范围（2026-10-25）

| 层次 | 已取得证据 | 边界 |
|---|---|---|
| 源码 | 开启 DLL smoke 的 Go 全量检查：102 个顶层测试、10 个测试包通过；Go vet 通过；前端 7 项 Node 测试和 TypeScript 检查通过 | 不等于真实桌面和外部业务全部通过 |
| 依赖 | 官方 registry 的 pnpm audit 未报告已知漏洞 | 仅对应本轮锁文件与检查时间 |
| 浏览器 | 1440 / 768 / 390 宽度下检查翻译、设置五个子页、常用语与自检；实际设置保存成功 | 不替代原生窗口、热键和截图手工验收 |
| 原生桥接 | manifest / SHA、IPC 状态、内存回调和独立子进程 DLL 初始化；隐藏 clipboard owner 的受控检查 | 未执行真实框选、标注、滚轮长截图、钉图或物理热键 |
| OCR | OCR.space 合成俄文图精确一致，约 2.169 秒；合成中文图按空白与冒号规范化后一致，约 1.096 秒 | 仅两张图，不推导通用准确率或固定速度 |
| 翻译语义 | 6 次翻译调用（含连通性探测）及 1 次评审取得结果；中断请求不计为完成 | 未完成完整中译俄集，未做本轮新提示词调优；实际 token 未可靠核对 |
| 打包与发布 | 使用本版源码与内嵌资源生成交付物 | 以最终发行包验证记录为准，本表不提前宣称安装通过 |

旧“多轮均分提升”记录混用了模型与样本，不能作为本轮质量提升证据。后续需用固定业务样本持续观察俄语自然度、中文可读性、型号保护和截图识别，不能把有限样本写成所有业务场景通过。
