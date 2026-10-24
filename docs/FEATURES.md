# ZSH TRANS 功能清单（重构验收基线）

> 来源：`translate/translate_tool_core.pyc`（v1.1 核心，反编译字节码）+ `translate/translate.py`（v2.38 外壳）。
> 每一条都标注了原始实现位置与 Go 重构后的落地位置。**一条都不能少。**

## 0. 版本与常量

| # | 功能 | 原始位置 | Go 实现 |
|---|------|----------|---------|
| 0.1 | 应用名 `ZSH TRANS` / 版本 `v2.38` | `translate.py:3152` | `internal/config` `AppName`/`AppVersion` |
| 0.2 | 16 种语言表 `LANGUAGES` | `translate.py:3185` | `internal/lang` `Languages` |
| 0.3 | `FLIP` 语向翻转表（`translate.py:4338` 扩为「除 auto/zh 外一律→zh」） | pyc + `translate.py:4338` | `internal/lang` `Flip`（`init()` 补齐 16 语言） |
| 0.4 | 输出标签清理表 `TRANSLATION_OUTPUT_LABELS`（15 项） | `translate.py:3164` | `internal/providers/prompt.go` `outputLabels` |
| 0.5 | 请求超时 8s / 缓存 300 条 | pyc `TIMEOUT`/`CACHE_MAX` | `internal/config` `Timeout`/`CacheMax` |
| 0.6 | 主题配色 `C_*`（12 色） | pyc | `frontend/src/styles.css` CSS 变量（**有意重设计**，见下） |
| 0.7 | 5 套主题方案 `THEME_SCHEMES`（laser/plasma/aurora/gold/ice） | pyc | **有意不移植**：用户 2026-10-24 明确「UI 配色等可自由发挥不必遵循原版配色」 |
| 0.8 | 4 种背景方向 `THEME_DIRECTIONS` | pyc | **有意不移植**：同上，背景改为单一极光渐变 |

## 1. 配置（`tt_config.json`）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 1.1 | 默认配置结构（hotkeys/src_lang/tgt_lang/default_provider/web/api_keys/providers/ocr_model） | pyc `DEFAULT_CONFIG` | `config.Default()` |
| 1.2 | 读配置（缺失字段用默认值补齐） | pyc `load_config` | `config.Load()` |
| 1.3 | 写配置（原子写、ensure_ascii=false、indent=2） | pyc `save_config` | `config.Save()` |
| 1.4 | 配置路径：exe 同级目录 | pyc `_config_path` | `config.Path()` |
| 1.5 | `CFG` 全局内存态 + 热更新 | pyc | `config.Store` |

## 2. 常用语（`tt_phrases.json`）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 2.1 | 结构 `{id, desc, content}` | pyc `load_phrases` | `phrases.Item` |
| 2.2 | 读取（坏文件回退空表并打印告警） | pyc `load_phrases` | `phrases.Load()` |
| 2.3 | 写入（`ensure_ascii=False, indent=2`） | pyc `save_phrases` | `phrases.Save()` |
| 2.4 | 新增 id 生成 `_new_phrase_id` | pyc | `phrases.NewID()` |
| 2.5 | 内置种子常用语 `_seed_bundled` | pyc | `phrases.Seed()` |
| 2.6 | 增 / 改 / 删 / 复制到剪贴板 | `translate.py:2909-2967` | `internal/server` + 前端 |

## 3. 语言检测与语向决策

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 3.1 | 西里尔 → ru，CJK → zh，其余 → en | pyc `detect_lang` | `lang.Detect()` |
| 3.2 | 扩展检测：日语假名→ja、韩文→ko、阿拉伯→ar、泰文→th | `translate.py:4331` | `lang.Detect()` |
| 3.3 | `resolve_direction(text, src, tgt)`：auto 时按检测结果；检测==目标则翻转 | pyc | `lang.Resolve()` |
| 3.4 | `decide_direction` 用配置中的 src/tgt | pyc | `lang.Decide()` |
| 3.5 | 中→非中文走 `FLIP["zh"]` | `translate.py:4360` | `lang.Decide()` |
| 3.6 | 交换语向 `_swap_lang`：`tgt=src`（src 非 auto）或 `FLIP[tgt]`，`src=旧tgt`，两者相同则 src 归 auto | `translate.py:2874` | `lang.Swap()` + `server` `/api/swap` |

## 4. 内置免费/直连翻译提供方（9 个）

| # | 提供方 | 端点 | Go |
|---|--------|------|-----|
| 4.1 | `google` Google 网页翻译 | `translate.googleapis.com/translate_a/single` client=gtx | `providers/direct.go` `tGoogle` |
| 4.2 | `mymemory` MyMemory | `api.mymemory.translated.net/get` | `providers/direct.go` `tMyMemory` |
| 4.3 | `deepl` DeepL Free | `api-free.deepl.com/v2/translate` | `providers/direct.go` `tDeepL` |
| 4.4 | `baidu` 百度翻译 | `fanyi-api.baidu.com/api/trans/vip/translate`（MD5 签名） | `providers/direct.go` `tBaidu` |
| 4.5 | `niutrans` 小牛翻译 | `api.niutrans.com/NiuTransServer/translation` | `providers/direct.go` `tNiuTrans` |
| 4.6 | `lingva` Lingva（双镜像） | `translate.plausibility.cloud` / `lingva.ml` | `providers/direct.go` `tLingva` |
| 4.7 | `agnes` Agnes 2.5 Flash | OpenAI 兼容 | `providers/direct.go` `tAgnes` |
| 4.8 | `qwen-max` GLM-5.2 | 百炼兼容接口 | `providers/direct.go` `tQwenMax`/`tQwenMaxSnap` |
| 4.9 | `qwen-max-2606` Qwen3.7-Max(2026-06-08) | 百炼兼容接口 | `providers/direct.go` `tQwenMax`/`tQwenMaxSnap` |

## 5. 翻译引擎（`TranslateEngine`）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 5.1 | 文本归一化（空白折叠）后作为缓存键 `(norm, src, tgt)` | pyc:570 | `engine.Translate()` |
| 5.2 | LRU 缓存，容量 300，命中返回 `provider="cache"`, `ms=0` | pyc `_cache_get/_cache_put` | `engine.lru` |
| 5.3 | 无可用提供方 → `no provider enabled` | pyc:577 | `engine.ErrNoProvider` |
| 5.4 | 首选提供方优先，其余轮转（`idx` 自增取模） | pyc:579-592 | `engine.Translate()` |
| 5.5 | 提供方健康度 `healthy()` / `mark_ok()` / `mark_fail()` + 冷却 | pyc `Provider` | `providers.Provider.Health` |
| 5.6 | 失败自动降级到下一个，全部失败 → `all providers failed:\n…` | pyc:596-610 | `engine.ErrAllFailed` |
| 5.7 | 返回 `{text, provider, ms}` | pyc:604 | `engine.Result` |
| 5.8 | 并发安全（互斥锁保护 idx / 缓存） | pyc `lock` | `sync.Mutex` |

## 6. OCR（截图取字）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 6.1 | OCR.space（`OCREngine=2`, `scale=true`, `filetype=PNG`, base64） | pyc `ocr_space` | `ocr/space.go`（OCR.space） |
| 6.2 | 百度 OCR（access_token 缓存，过期刷新） | pyc `ocr_baidu` | `ocr/space.go` `Baidu()` + `baiduAccess()` |
| 6.3 | 视觉大模型 OCR（`ocr_model` 指定的 image 模型） | pyc `ocr_qwen` | `ocr/json.go` `Vision()` |
| 6.4 | `run_ocr` 多引擎并行 + 中位数择优 | pyc `run_ocr` | `ocr.Run()` |
| 6.5 | 图像预处理：Otsu 阈值二值化 + 白底扩展 | pyc `_otsu_threshold`/`_binary_white_bg` | `ocr/image.go` |
| 6.6 | 候选集生成 `_ocr_candidates`（原图/二值/放大） | pyc | `ocr.Candidates()` |
| 6.7 | OCR 结果缓存 `_OCR_CACHE` | pyc | `ocr` 内部 LRU |

## 7. HTTP 服务（原自带 Web 界面）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 7.1 | 监听 `host:port`（默认 `0.0.0.0:8765`，可在设置里改） | pyc `start_web` | `server.Start()` |
| 7.2 | `GET /` `GET /index.html` → 内置 HTML | pyc `do_GET` | `server` 静态资源 |
| 7.3 | `GET /api/status` → `{providers:[{name,healthy}]}` | pyc:1154 | `server` `handleStatus` |
| 7.4 | `POST /api/translate` `{text,src,tgt}` → `{text,provider,ms,src}` | pyc:1176 | `server` `handleTranslate` |
| 7.5 | 语言白名单校验，越界 400 `bad lang` | pyc:1172 | `server`。**有意扩展**：原 pyc 只认 `src∈{auto,ru,en,zh}` / `tgt∈{ru,en,zh}`（内置网页仅 3 语种）；桌面端按 `LANGUAGES` 16 种放开，否则 ja/ko/fr… 一律 400 |
| 7.6 | 空文本 → 400 `empty text` | pyc:1170 | `server` |
| 7.7 | 未知路由 → 404 `{error:"not found"}` | pyc:1158 | `server` |
| 7.8 | 局域网 IP 探测 `lan_ip` | pyc `lan_ip` | `server.LanIP()` |
| 7.9 | 静默日志 `log_message`（不刷屏） | pyc | `logx` |

## 8. 多模型提供方体系（`translate.py`）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 8.1 | `normalize_provider` / `normalize_model` 归一化 | :139/:104 | `providers.Normalize*` |
| 8.2 | `seed_providers` 初始提供方（含 `ACCTOKEN_API_KEY` 环境变量） | :188 | `providers.Seed()` |
| 8.3 | `provider_presets` 24 个预设（国内厂商/国外厂商/Coding Plan/第三方平台） | :251 | `providers.Presets()` |
| 8.4 | `engine_name` = `provider:model` | :325 | `providers.EngineName()` |
| 8.5 | `resolve_key`：配置 key → 环境变量 → 空 | :333 | `providers.ResolveKey()` |
| 8.6 | 三种协议：`openai-completions` / `openai-responses` / `anthropic-messages` | :98 | `providers.Protocol*` |
| 8.7 | `chat_url` / `models_url` 拼接 | :357/:362 | `providers.ChatURL/ModelsURL` |
| 8.8 | `auth_headers`（Bearer / x-api-key + anthropic-version） | :366 | `providers.AuthHeaders()` |
| 8.9 | 5 种思考参数风格 `_thinking_fields` | :381 | `providers.ThinkingFields()` |
| 8.10 | `generation_limit` 输出上限 | :417 | `providers.GenerationLimit()` |
| 8.11 | `build_chat_payload` 组包 | :425 | `providers.BuildChatPayload()` |
| 8.12 | `extract_completion_text` 三种协议解包 | :534 | `providers.ExtractText()` |
| 8.13 | 流式增量 `_stream_delta` / `collect_stream_text` | :559/:587 | `providers.CollectStream()` |
| 8.14 | `make_translator` 生成翻译函数 | :625 | `providers.TranslateWith()` |
| 8.15 | `make_vision_ocr` 生成视觉 OCR 函数 | :674 | `providers.VisionOCR()` |
| 8.16 | `fetch_models` 拉取账号模型列表 | :874 | `providers.FetchModels()` |
| 8.17 | `test_connection` 连通性测试 | :879 | `providers.TestConnection()` |
| 8.18 | `vision_models` 可用视觉模型 | :906 | `providers.VisionModels()` |
| 8.19 | `usable_providers` 可用提供方 | :937 | `providers.UsableProviders()` |
| 8.20 | `model_config_from_catalogue` 从目录生成模型配置 | :793 | `providers.ModelFromCatalogue()` |

## 9. 翻译提示词与输出清洗

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 9.1 | 俄跨境客服翻译契约（语体/情绪/俚语/电商术语/型号保护/示例） | :3410 | `providers/prompt.go` `BuildPrompt()` |
| 9.2 | 输出标签剥离（15 个标签，含 `标签：` 前缀与整行标题） | :3476 | `providers.CleanOutput()` |
| 9.3 | 目标俄语仍含汉字 → 报错 | :3476 | `providers.CleanOutput()` |
| 9.4 | 目标中文仍含西里尔 → 报错 | :3476 | `providers.CleanOutput()` |
| 9.5 | `extract_chat_completion` 非空校验 | :3529 | `providers.ExtractText()` |

## 10. 全局快捷键（Win32）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 10.1 | 三个热键：划词翻译 / 截图翻译 / 主窗口显隐 | pyc `_HOTKEY_BINDINGS` | `hotkeys.Bindings` |
| 10.2 | 组合键解析 `parse_native_hotkey`（ctrl/alt/shift/win + 键名） | :3744 | `hotkeys.Parse()` |
| 10.3 | `RegisterHotKey` 注册 + 消息循环 | :3790 | `hotkeys.Manager` |
| 10.4 | 冲突检测（同一组合重复） | :2446 | `hotkeys.Validate()` |
| 10.5 | 修饰键状态查询 `_win32_modifiers_pressed` | :3240 | `hotkeys.ModifiersDown()` |
| 10.6 | 剪贴板序列号 `_win32_clipboard_sequence` | :3245 | `capture.ClipboardSeq()` |
| 10.7 | `send_ctrl_c` 模拟复制 | :3269 | `capture.SendCtrlC()` |
| 10.8 | `capture_selected_text` 取选中文本（剪贴板轮询） | :3350 | `capture.SelectedText()` |
| 10.9 | 弹窗点击外部关闭 `PopupClickMonitor` | :4140 | `frontend` 失焦关闭 |

## 11. 截图取词（原生捕获）

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 11.1 | 全屏/区域截图 | pyc `CaptureSession` | `capture.Region()` |
| 11.2 | 原生运行时校验 `validate_runtime` / `validate_native_files` | :1194/:1388 | 不再依赖 QQ 运行时，直接 GDI 截图 |
| 11.3 | 截图结果 → OCR → 翻译 → 弹窗 | :1412 | `server` `/api/capture` 全链路 |
| 11.4 | 工具栏动画修复 / 相机文件校验 | :1095/:1090 | 不适用（已脱离 QQ 运行时） |

## 12. 主界面（Qt → Web）

| # | 功能 | 原始 | Go/前端 |
|---|------|------|---------|
| 12.1 | 标题 `ZSH TRANS v2.38`、660×800 窗口 | :2760 | 前端布局 |
| 12.2 | 源/目标语言下拉 + 交换按钮 | :2788 | 前端 `LangBar` |
| 12.3 | 原文输入框（占位「输入文字，或粘贴客户消息…」） | :2813 | 前端 |
| 12.4 | `Ctrl+Enter` 翻译 | :2827 | 前端快捷键 |
| 12.5 | 「翻译」主按钮 / 「截图翻译」按钮 | :2818 | 前端 |
| 12.6 | 译文只读区 + 「复制译文」 | :2841 | 前端 |
| 12.7 | 常用语列表 + 「添加」+ 计数 | :2852 | 前端 `Phrases` |
| 12.8 | 状态栏（链路显示 `_paint_chain`：提供方名 + 健康点 + 本次使用高亮 + ms） | :2876 | 前端 `ChainBar` |
| 12.9 | 系统托盘：显示主窗口 / 截图翻译 / 设置 / 退出 | :2848 | `internal/tray`（fyne.io/systray 原生托盘，点击图标显示主窗口） |
| 12.10 | 划词翻译弹窗（原文+译文+meta，点击外部/Esc 关闭） | :2624 | 前端 `App.tsx` `Popup`（`mousedown` 外部 + `Escape` 关闭，对应 `PopupClickMonitor`）；主界面另有「划词取词」按钮手动触发 `/api/selection` |
| 12.11 | 关闭窗口最小化到托盘 | :3081 | `Ctrl+Alt+Q` 热键推 `close` 事件，浏览器端回到托盘态；进程常驻托盘 |
| 12.12 | 翻译失败提示（原版 `QMessageBox` 弹窗展示 `all providers failed:\n…` 全文） | pyc `translate` 抛 `RuntimeError` | `frontend/src/errors.ts` `briefError()`：toast 只显示一行摘要（`翻译失败（N 项）：<首条短因>`），完整逐条原因（含内联 HTML 折叠、URL 查询串折叠）折叠进右栏 `.errbox` 内部滚动，避免撑破两栏布局 |

## 13. 设置窗口（4 页）

| # | 功能 | 原始 | Go/前端 |
|---|------|------|---------|
| 13.1 | 侧栏导航：模型提供方 / 截图与 OCR / 快捷键 / 其他翻译接口 | :2147 | 前端 `Settings` |
| 13.2 | 「默认翻译模型」下拉（含「自动路由」） | :2188 | 前端 |
| 13.3 | 提供方卡片：id / 名称 / 地址 / 协议 / 启用 / 密钥 / 思考风格 / 超时 / 流式 | :1806 | 前端 `ProviderCard` |
| 13.4 | 模型表格：模型 ID / 显示名 / 图片 / 推理档 / 温度 | :1806 | 前端 |
| 13.5 | 「添加提供方」按 4 分组菜单（含第三方/自定义） | :2317 | 前端 |
| 13.6 | 「恢复默认提供方」 | :2346 | 前端 + `providers.Seed()` |
| 13.7 | 单卡「重置」回已保存值 | :2352 | 前端 |
| 13.8 | 「识别模型」下拉（仅启用且支持图片的模型） | :2198 | 前端 |
| 13.9 | 快捷键 3 项编辑 + 冲突校验 | :2204 | 前端 + `hotkeys.Validate` |
| 13.10 | 其他接口密钥 8 项（密码框，占位显示「已保存（不显示）」/「未设置」） | :2213 | 前端 |
| 13.11 | 保存校验：ID 重复 / 默认模型不可用 / OCR 模型非视觉 / 热键非法或冲突 | :2394 | `server` `/api/settings` |
| 13.12 | 保存后热重载（重建引擎 + 重注册热键） | :2462 | `config.Store` + `engine.Rebuild()` |

## 14. 其它

| # | 功能 | 原始 | Go |
|---|------|------|-----|
| 14.1 | 自检 `self_check()`（语言数/预设数/UI 工具包/字形/常用语数与哈希） | :4460 | `server` `/api/selfcheck` |
| 14.2 | 应用图标嵌入 | :4410 | `internal/assets/app.ico`（`go:embed`，托盘与 exe 资源共用；exe 图标由 `rsrc_windows_amd64.syso` 嵌入） |
| 14.3 | `resource_path` 打包资源定位 | :4455 | `embed.FS` |
| 14.4 | 单实例 / 退出清理 | :3088 | `internal/instance`（命名互斥体 `Global\ZSHTRANS_SINGLE_INSTANCE`，二开时唤起已有实例并退出）+ 托盘退出 |
| 14.5 | 日志文件 | — | `logx` → `zshtrans.log` |

## 15. v3.0 相对原版的新增（重构带来的能力，非原版功能）

原版没有这些；列出是为了说明「功能一项没落下」之外还多了什么，以及为什么。

| # | 新增 | 说明 | 落地 |
|---|------|------|------|
| 15.1 | WebView2 桌面窗口 | 原版是 PyQt5 原生窗口；网页版只能开浏览器标签页。现在 exe 默认弹**原生桌面窗口**（WebView2 内核），关窗只隐藏、`Ctrl+Alt+M` 叫回，托盘可控。`-web` 仍可退回浏览器 | `internal/deskwin`、`cmd/zshtrans/main.go` |
| 15.2 | 自动检测更新 | 启动 2.5s 后查 GitHub Releases，有新版弹窗展示更新说明，用户可选「立即更新 / 跳过此版本 / 稍后」；下载走 `%TEMP%`，替换由分离的批处理完成（本进程先退出，避免自我覆盖失败） | `internal/update`、`/api/update/{check,apply}`、`frontend/src/components/UpdatePanel.tsx` |
| 15.3 | 翻译提示词 7 轮实测调优 | 原版提示词是手写一次性的。本版搭了 28 条用例 + 规则 lint + LLM 盲评的调参台，实测 judge 均分 7.81→8.9，客观 lint 硬错误 0 条。俄语口语化、去机翻腔、去性别括注、型号保护、电商术语（брак/ПВЗ/выкупить）都是逐条修出来的 | `internal/providers/prompt.go`、`.tmp/tune/`（调参台，不随发布） |
| 15.4 | OCR 提速 | 原版对每张图无条件生成「原图 / 二值化 / 放大 2 倍」三份候选并全部送 OCR。本版只在原图 < 720×520 时才加放大候选（大图放大后 OCR 侧也会压回去，收益≈0），并在日志里给每个引擎每个候选的失败原因 | `internal/ocr/ocr.go` `Candidates()` |
| 15.5 | OCR 失败原因可诊断 | 原版只报 `all OCR failed`；本版逐引擎逐候选列出（`引擎 [候选]: 原因`），并报真实 Otsu 阈值（原版写死 255） | `internal/ocr/ocr.go` `Run()` |
| 15.6 | 设置写入的字段级语义 | 前端只改一项时（如自检页关掉自动更新），不会把没提交的字段清空。`default_provider`/`ocr_model` 用指针区分「不涉及」与「清空」 | `internal/server/server.go` `settingsRequest` |
| 15.7 | 直连引擎可设为默认 | 原版默认模型只能选「多模型提供方」里的模型；本版 `default_provider` 也可填 `google`/`mymemory` 等直连引擎名 | `internal/server/server.go` `engineUsable()` |
| 15.8 | 局域网 IP 显示 | 顶栏直接显示 `局域网 192.168.x.x`，方便手机/另一台机器访问网页版 | `/api/lanip`、`frontend/src/App.tsx` |
| 15.9 | 界面语向选择器左右分置 | 源语言下拉贴在左栏表头、目标语言贴在右栏表头，中间只留交换按钮——一眼看清方向，且控件比原版紧凑 | `frontend/src/pages/Translate.tsx` |
| 15.10 | 日志带时间戳 | 每行 `YYYY-MM-DD HH:MM:SS ` 前缀，原版是裸 print | `internal/logx` |
