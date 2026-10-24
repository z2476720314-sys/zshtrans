# 更新记录

## v3.0 — 2026-10-24

首个 Go 重构版。原版是 Python 3.14 + PyQt5 的 `translate.py` + `translate_tool_core.pyc`（v2.38）。

### 重构
- 后端 Python → **Go 1.27**（静态链接，无运行库依赖，单文件 exe）
- 界面 PyQt5 → **React 19 + TypeScript + Vite + Ant Design 5**，由 **WebView2** 原生窗口承载
- 原版功能逐条登记在 `docs/FEATURES.md`（15 章 100+ 条），每条标注原版行号与本版落地位置

### 功能（与原版一致）
- 16 种语言互译，自动识别语向；`auto`/`ru`/`en`/`zh` 网页端白名单
- 9 个直连翻译引擎：Google 网页翻译 / MyMemory / DeepL Free / 百度翻译 / 小牛翻译 / Lingva / Agnes 2.5 Flash / GLM-5.2 / Qwen3.7-Max
- 多模型提供方体系：3 种协议（openai-completions / openai-responses / anthropic-messages）、8 档推理等级、5 种思考风格、27 个预设、模型目录识别
- 翻译引擎 LRU 缓存（300 条）+ 健康度轮转 + 失败降级 + 首选模型优先
- OCR 三引擎并行（视觉模型 / OCR.space / 百度 OCR）+ 中位数择优 + 图像预处理（原图 / Otsu 二值化 / 小图放大）
- 全局快捷键：划词翻译、截图翻译、主窗口显隐、隐藏到托盘
- 常用语管理、系统托盘、单实例保护、局域网共享、自检页

### 新增（原版没有）
- **WebView2 桌面窗口**：exe 默认弹原生窗口，关窗只隐藏；`-web` 可退回浏览器
- **自动检测更新**：启动后查 GitHub Releases，弹窗展示更新说明，可选「立即更新 / 跳过此版本 / 稍后」
- **提示词 7 轮实测调优**：28 条用例 + 规则 lint + LLM 盲评，judge 均分 7.81 → 8.9，客观硬错误 0 条
- **OCR 提速**：小图才生成放大候选，大图不再白跑一遍
- **OCR 失败可诊断**：逐引擎逐候选列出原因，报真实 Otsu 阈值
- 界面语向选择器左右分置、控件收紧
- 日志带时间戳

### 修复（相对原版）
- 设置只改一项时不再清空其他字段（字段级指针语义）
- `default_provider` 可填直连引擎名（原版只能填多模型提供方里的模型）
- 首次运行不再误报 `config read fail` 告警
- `-console` 在 GUI 子系统 exe 下仍可用（`AllocConsole` 兜底）
