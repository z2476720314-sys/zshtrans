# ZSH TRANS v3.2 · 跨境翻译台

俄 ↔ 中 ↔ 英跨境客服翻译工具。Go 后端 + React/Ant Design 前端，单文件 exe，无需安装 Python/Qt/运行库。

原版是 Python 3.14 + PyQt5 的 `translate.py` + `translate_tool_core.pyc`（v2.38）。本版是完整重构，**原版每一项功能都在 `docs/FEATURES.md` 里逐条登记并标注落地位置**，一条不落。

---

## 1. 安装

### 1.1 免安装（推荐）

复制 `dist/zshtrans.exe` 到任意目录，双击运行。首次运行会在 exe 同级目录自动生成：

| 文件 | 内容 |
|------|------|
| `tt_config.json` | 全部配置（热键、语言、密钥、模型提供方） |
| `tt_phrases.json` | 常用语 |
| `zshtrans.log` | 运行日志（每行带时间戳） |

**卸载**：关掉托盘图标 → 删掉 exe 和这三个文件即可，不写注册表、不装服务、不落到 `%APPDATA%`。

### 1.2 系统要求

- Windows 10 1809+ / Windows 11，x64
- 无需 .NET、无需 VC++ 运行库（静态链接）
- 出网访问 `translate.googleapis.com` / `api.mymemory.translated.net` 等免费引擎（或你自己的模型 API）

### 1.3 首次启动做什么

1. 双击 exe → 弹出 **WebView2 桌面窗口**（原生窗口，不是浏览器标签页），托盘同时出现图标
2. 窗口关掉只是隐藏，从托盘或 `Ctrl+Alt+M` 叫回来；真正退出用托盘「退出」或 `Ctrl+Alt+Q`
3. 想用浏览器打开：`zshtrans.exe -web`，或手动访问 `http://127.0.0.1:8765/`
4. 不想要托盘图标：`zshtrans.exe -no-tray`；只要服务不要界面：`zshtrans.exe -no-open`

---

## 2. 命令行参数

```
zshtrans.exe [flags]

  -port int         HTTP 端口（默认读配置，配置缺省 8765）
  -host string      监听地址（默认读配置，缺省 0.0.0.0）
  -no-open          不自动打开界面（只起服务，界面自己开）
  -web              用默认浏览器承载界面（默认是 WebView2 桌面窗口）
  -wv-debug          WebView2 调试模式（开 DevTools）
  -no-hotkeys       不注册全局快捷键
  -no-tray          不显示系统托盘图标
  -no-single        允许多开（关闭单实例保护）
  -console          日志同时输出到控制台
```

示例：

```bash
# 换端口 + 不开浏览器（服务器/远程桌面场景）
zshtrans.exe -port 9000 -no-open

# 调试：控制台看日志、不要热键冲突
zshtrans.exe -console -no-hotkeys

# 用浏览器而不是桌面窗口（远程桌面 / 想开多个标签页时）
zshtrans.exe -web
```

---

## 3. 使用方法

### 3.1 翻译主界面

| 操作 | 说明 |
|------|------|
| 输入/粘贴文本 | 俄语、中文、英语、日语等 16 种语言自动识别 |
| 点「翻译」或 `Ctrl+Enter` | 走「自动检测 → 目标语言」，结果右侧显示 |
| 语言下拉 | 源语言可「自动检测」，目标语言 15 种可选 |
| 交换按钮 ⇄ | 互换语向；源是自动检测时按检测结果互换 |
| 复制 | 一键复制译文 |
| 清空 | 清空原文和译文 |
| 截图取词 | 全屏截图 → OCR → 自动翻译 |
| 划词取词 | 取当前外部程序里选中的文字 → 翻译 |

输入框右下显示字符数，译文区顶部显示 `提供方 · 耗时ms`。

### 3.2 常用语

预置 5 条俄语客服话术（问候、催单、物流、退换、道谢）。每条支持：

- **插入**：一键填进翻译框
- **编辑**：改备注和内容
- **删除**：二次确认

「添加常用语」新增一条，字段为「备注」+「内容」。

### 3.3 设置

四个子页，改动点「保存并生效」才落盘。

#### 模型提供方

- **默认翻译模型**：「自动路由（按健康度轮转）」或指定某个模型
- **添加提供方**：按 4 个分组选预设 —— 国内厂商（9）/ 国外厂商（6）/ Coding Plan（8）/ 第三方平台（3），共 27 个
- **提供方卡片**：名称、接口地址、协议、密钥、思考风格、超时、流式整包
- **模型表格**：模型 ID、显示名、是否支持图片、推理档、温度
- **识别模型**：调该提供方的 `/models` 接口拉账号可用模型列表
- **测试**：发一次最小请求验证地址 + 密钥
- **恢复默认提供方**：回到内置的百炼 + Acctoken 两张卡
- **重置**：单卡回退到上次保存值

协议支持三种：`OpenAI 兼容 (chat/completions)`、`OpenAI Responses`、`Anthropic Messages`。
思考参数支持五种风格：`enable_thinking`、`thinking_object`、`adaptive`、`reasoning_split`、`none`。

#### 截图与 OCR

- **识别模型**：从「启用且支持图片」的模型里选，或选「不使用视觉模型」
- **OCR.space key**：默认 `helloworld`（公共额度，够轻量用）
- **百度 OCR API Key / Secret**：填了就走百度高精度 OCR

OCR 三引擎（视觉模型 / OCR.space / 百度 OCR）**并行**跑，多候选图（原图 / Otsu 二值化 / 放大）逐个试，按识别长度取中位数择优。

#### 快捷键

四项，格式 `ctrl+alt+t`：

| 项 | 默认 | 作用 |
|----|------|------|
| 划词翻译 | `Ctrl+Alt+T` | 取选中文字 → 弹翻译窗 |
| 截图翻译 | `Ctrl+Alt+S` | 截全屏 → OCR → 弹翻译窗 |
| 主窗口显隐 | `Ctrl+Alt+M` | 唤起浏览器界面 |
| 隐藏到托盘 | `Ctrl+Alt+Q` | 收起界面，进程常驻 |

保存时会校验格式与冲突，非法或撞车直接报错不落盘。

#### 其他翻译接口

8 个免费/第三方引擎的密钥，全部密码框、不回显已保存值：

DeepL Free · 百度翻译 AppID · 百度翻译密钥 · 小牛翻译 apikey · Agnes 2.5 Flash · OCR.space key · 百度 OCR AK · 百度 OCR SK

> 不填也能用：Google 网页翻译、MyMemory、Lingva 三个引擎**免密钥**，装完即用。

### 3.4 自检

一屏看到：应用版本、界面工具包、语言数量（16）、提供方预设（27）、直连提供方（9）、模型卡/可用数、常用语数与 SHA256 指纹、字形完整、翻译缓存条数，以及配置文件/常用语/日志的完整路径。

### 3.5 局域网共享

默认监听 `0.0.0.0:8765`，同一局域网内手机/平板用 `http://<你的IP>:8765/` 直接打开同一个界面。界面右上角显示当前局域网地址。

不想暴露：设置里把 `web.host` 改 `127.0.0.1`。

---

## 4. 翻译引擎链路

9 个直连免费引擎 + 你配置的任意个模型提供方，共用一个池：

1. 优先用「默认翻译模型」指定的提供方
2. 否则按健康度轮转（失败过的排后面）
3. 逐个尝试，失败自动降级到下一个，全部失败才报错
4. 成功后标记健康；同一段文本+语向命中缓存直接返回（`provider=cache`，`0ms`）
5. LRU 缓存 300 条

界面底部「链路」条实时显示每个提供方的健康状态，本次使用的那个高亮并显示耗时。

---

## 5. 提示词与输出清洗

当提供方是 LLM 时，使用专门的**俄跨境客服翻译契约**提示词：规定语体、情绪、俚语处理、电商术语（брак / ПВЗ / выкупить / не подошёл）、型号保护（Redmi Note 13、Pro Max、Ultra 原样保留）、口语品牌（айфон / самсунг / сяоми）归一，并给 4 组示例。

返回后清洗：

- 剥掉 15 种常见标题前缀（「译文：」「Translation:」「Перевод:」…）
- 目标俄语却含汉字 → 报错 `俄语译文仍含汉字`
- 目标中文却含西里尔字母 → 报错 `中文译文仍含西里尔字母`

---

## 6. 单实例与退出

- 双击第二次：不起第二个进程，直接把已在跑的实例的桌面窗口拉到前台
- 托盘菜单：显示主窗口 / 截图翻译 / 设置 / 退出
- 退出：托盘「退出」或 `Ctrl+C`（控制台模式）；3 秒内优雅关停 HTTP 服务
- 关窗口 ≠ 退出：窗口只是隐藏（`Ctrl+Alt+M` 叫回来），进程仍在跑热键和托盘

---

## 7. 排障

| 现象 | 原因与处理 |
|------|-----------|
| 双击没反应 | 看 exe 同级 `zshtrans.log`；端口被占会打印「端口 N 被占用或不可用」 |
| 浏览器打不开 | 手动访问 `http://127.0.0.1:8765/`；被改过端口就看日志里 `http listening on` |
| 桌面窗口不出现 | 系统缺 WebView2 运行时（Win11 自带、Win10 需装 Evergreen 运行时），程序会自动回退到默认浏览器并记日志；也可 `-web` 强制用浏览器 |
| 桌面窗口白屏 | 加 `-wv-debug` 开 DevTools 看报错；或删掉 `%LOCALAPPDATA%\ZSH TRANS\WebView2` 让它重建 |
| 翻译全部失败 | 右栏「翻译失败」面板逐条列出每个引擎的原因（鼠标可滚动）；日志里有同样内容。免费引擎偶发超时/限流，重试或配一个 LLM key |
| 翻译要等十几秒 | 免费链路会逐个尝试 google → mymemory → lingva，每家超时 8s 才换下一家。配一个 LLM key 或把默认模型设为可用模型可秒回 |
| OCR 识别不出 | 日志给每个引擎每个候选图的失败原因；换更清晰的图，或配百度 OCR |
| 热键不生效 | 撞了别的软件的热键，设置里换一个；或 `-no-hotkeys` 用界面按钮 |
| 划词取词拿不到 | 需要目标程序允许 `Ctrl+C` 复制；某些游戏/终端不行，改用截图取词 |

日志位置：exe 同级 `zshtrans.log`，每行 `YYYY-MM-DD HH:MM:SS ` 前缀。

---

## 8. 从源码构建

```bash
# 1. 前端
cd frontend
npm install          # 或 pnpm install
npx tsc --noEmit
npx vite build       # 产物输出到 ../internal/web/dist

# 2. 资源（图标 + manifest -> .syso，由 go build 自动链接）
go install github.com/akavel/rsrc@latest
rsrc -manifest rsrc.manifest -ico assets/app.ico -o cmd/zshtrans/rsrc_windows_amd64.syso
# 版本信息（FileVersion/ProductName 等）由 versioninfo.json 提供，发布前用
#   go-winres simply --in versioninfo.json --arch amd64 --out cmd/zshtrans/rsrc
# 不需要版本信息可省略这一步，只留图标 + manifest 也能正常跑。

# 3. 后端
go build -buildvcs=false -ldflags "-s -w" -o zshtrans.exe ./cmd/zshtrans

# 4. 验证
go vet -unsafeptr=false ./...
go test -buildvcs=false ./...
```

> `-buildvcs=false` 必须加：本目录不是干净 git 仓库时 `go build` 会因 `error obtaining VCS status` 失败。
> `-unsafeptr=false` 是给 vet 用的：`internal/capture` 里 Win32 GDI 的 `unsafe.Pointer` 用法是故意的。

---

## 9. 目录结构

```
zshtrans/
├── cmd/zshtrans/          # main：flag 解析、单实例、HTTP 起停、托盘、热键
├── internal/
│   ├── assets/            # 图标（go:embed）
│   ├── capture/           # Win32 截图 / 剪贴板取词
│   ├── config/            # tt_config.json 读写 + 默认值
│   ├── deskwin/           # WebView2 桌面窗口（Win32 子类化 + 显隐控制）
│   ├── engine/            # 翻译引擎：LRU 缓存 + 健康度轮转 + 降级
│   ├── hotkeys/           # RegisterHotKey + 消息循环
│   ├── instance/          # 命名互斥体单实例
│   ├── lang/              # 16 语言表、检测、语向决策、FLIP
│   ├── logx/              # 带时间戳的文件日志
│   ├── ocr/               # 三引擎并行 OCR + 图像预处理
│   ├── phrases/           # 常用语读写
│   ├── providers/         # 9 个直连引擎 + 多模型提供方体系 + 提示词
│   ├── server/            # 全部 HTTP API + SSE 热键事件
│   ├── tray/              # 系统托盘
│   ├── update/            # GitHub Releases 检查 / 下载 / 自替换
│   └── web/               # 前端产物（go:embed）
├── frontend/              # React 19 + TypeScript + Vite + Ant Design
├── assets/app.ico         # 程序图标
├── rsrc.manifest          # DPI / longPath / supportedOS 清单
├── versioninfo.json       # 版本信息资源
├── docs/FEATURES.md       # 原版功能逐条对照表（验收基线）
├── tools/promptdump/      # 提示词导出小工具
└── dist/                  # 发布产物
```

---

## 10. HTTP API（给二次开发/手机端）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/status` | 应用信息 + 提供方健康 + 缓存数 + 局域网 IP |
| POST | `/api/translate` | `{text,src,tgt}` → `{text,provider,ms,src,tgt}` |
| GET | `/api/languages` | `{all:[...],target:[...]}` 16/15 语言 |
| GET | `/api/config` | 当前配置 |
| POST | `/api/settings` | 保存配置（含校验） |
| GET | `/api/presets` | 27 个提供方预设 |
| POST | `/api/models` | `{provider_id,...}` 拉模型列表 |
| POST | `/api/test` | 测试提供方连通性 |
| GET/POST | `/api/phrases` | 常用语；POST 体 `{action:list\|add\|update\|delete}` |
| POST | `/api/ocr` | `{image:"data:image/png;base64,..."}` → OCR 文本 |
| POST | `/api/capture` | 截屏（可选 `{x,y,w,h}` 区域）→ OCR → 翻译 |
| POST | `/api/selection` | 取选中文字 → 翻译 |
| GET | `/api/swap` | `?from=&to=&detected=` 交换语向 |
| GET | `/api/lanip` | 本机局域网 IP |
| GET | `/api/selfcheck` | 自检数据 |
| GET | `/api/events` | SSE：`popup` / `toggle` / `close` / `log` |
| GET | `/api/update/check` | 查 GitHub 最新版；`?force=1` 忽略「已跳过」 |
| POST | `/api/update/apply` | `{action:"download"\|"install"\|"status", skip?}` |

---

## 11. 从 Python 版迁移

配置字段与语义和原版一致，可直接使用：

- 语言编码（`ru`/`zh`/`en`/…）相同
- 热键字符串格式相同（`ctrl+alt+t`）
- `api_keys` 9 个键名相同（`deepl`/`baidu_appid`/`baidu_key`/`niutrans`/`ocrspace`/`baidu_ocr_ak`/`baidu_ocr_sk`/`agnes`/`qwen`）
- `tt_phrases.json` 结构相同（`{id,desc,content}`），直接拷过来即可

**注意**：原版 `target_mode`（`ru2zh`/`zh2ru`）字段已废弃，读取时会自动迁移成 `src_lang`/`tgt_lang`。
主题配色不再沿用原版的 5 套方案（`laser`/`plasma`/`aurora`/`gold`/`ice`）——按你的要求改成了单一炭黑+酸柠+珊瑚粉配色。
