# ZSH TRANS v3.4 · 跨境翻译台

面向中俄跨境客服的 Windows 翻译工具：Go 后端、React / TypeScript / Ant Design 前端，由 WebView2 桌面窗口承载。提供文本翻译、全局划词、QQ 原生截图、常用语和用户自主更新，无需安装 Python 或 Qt。

原版为 Python + PySide6 / Qt。原功能逐项对应关系、改版取舍和验证边界保留在 [功能清单](docs/FEATURES.md)。实现覆盖不等于全部外部平台和真实桌面场景均已验收。

## 1. 安装、迁移与数据

将发行版 `zshtrans.exe` 放在可写目录后运行。需要 Windows x64 和 Microsoft Edge WebView2 Runtime。缺少 WebView2 时显示错误，不自动改用浏览器。

优先在 exe 同级保存数据；目录不可写时使用 `%APPDATA%\ZSH TRANS`。自检页显示实际位置。

| 文件或目录 | 内容 |
|---|---|
| `tt_config.json` | 语言、快捷键、提供方、模型和接口配置 |
| `tt_phrases.json` | 常用语 |
| `zshtrans.log` | 运行日志 |
| `WebView2/` | 本应用桌面界面的 WebView2 用户数据 |
| `.updates/` | 已下载的待安装更新 |
| `native-runtime/9.9.30-48517/` | 首次截图时释放的原生组件、校验清单与许可 |

新便携数据目录尚无配置时，程序尝试从旧版 Roaming 用户数据目录复制配置和常用语。迁移校验 JSON，不覆盖新目录已有文件，不修改旧文件。已有便携配置时继续使用该配置。其他位置的旧数据可在退出程序后备份并迁入。

原生截图的 71 个文件内嵌于 exe，压缩包约 12.7 MB、释放后约 29.1 MB。使用时按内嵌 manifest 验证 SHA-256，不依赖原 Python 安装目录。组件许可见 [第三方说明](THIRD_PARTY_NOTICES.md)。

关闭窗口或 `Ctrl+Alt+Q` 只隐藏到托盘；`Ctrl+Alt+M` 显示或隐藏主窗口。真正退出用托盘「退出」。卸载前先退出，再按需备份并移除 exe 和自检页列出的本应用数据；WebView2 是共享运行时。

## 2. 文本翻译与常用语

- 原文和译文左右排列，语向选择器分别位于对应栏。列表包含「自动」及 15 种语言；自动检测使用字符特征，无法可靠区分的语言请手动选择。
- 点「翻译」或按 `Ctrl+Enter`。支持取消当前请求，取消或迟到的响应不会覆盖后续结果；切换页面保留翻译草稿。
- 主窗口手动翻译按所选源／目标语言执行。快捷划词和截图将非中文译为中文；中文译为所选外语，目标为中文时使用所选外语源语言，两端都未选外语时使用俄语。
- 常用语支持新增、编辑、删除、搜索、插入和复制，沿用 `{id, desc, content}` 结构。

引擎先尝试指定的默认提供方，再按健康度和轮转顺序尝试其他可用提供方；保留失败原因供排查，翻译缓存容量为 300。内置 Google、MyMemory、DeepL Free、百度、小牛、Lingva 及兼容模型接口；可用性、配额和语言支持由各服务决定。

## 3. 截图、划词与结果窗

`Ctrl+Alt+S`、主界面与托盘截图入口调用同一 QQ 原生交互组件，提供框选、标注、长截图、保存、复制与钉图。

- 「提取文字」只执行 OCR；「翻译」执行 OCR 后翻译，结果附实际 OCR 引擎和 OCR 加翻译的总耗时。
- 普通复制、保存和取消不提交图片给 OCR 或翻译接口。
- 截图前隐藏主窗口，完成后按之前的可见状态恢复。
- 保留原组件校验和工具栏修正：仅对本程序启动、路径与版本匹配的子进程修改已知指令，磁盘中的原文件保持不变。钉图仍可见时保留该子进程，否则在完成后回收。

在目标程序选中文字后按 `Ctrl+Alt+T`。程序等待修饰键释放，再模拟复制，并按剪贴板序列变化取词；目标程序必须允许复制。

结果使用独立、置顶、不主动激活的原生窗口，主窗口隐藏时也可展示。支持复制、滚动、外部点击、Esc 和 15 秒自动关闭。浏览器审查模式保留页内结果展示。

划词前尝试保存全部可安全快照的 HGLOBAL 格式，包括纯文本、HTML、RTF 等，完成后按原格式恢复。bitmap、元文件或私有对象等不能安全快照时，在发送复制按键前停止并提示改用截图取词。恢复前核对序列，避免覆盖用户后来复制的内容；不承诺任意剪贴板格式都能完整恢复。

## 4. 设置与 OCR

设置保存后生效。支持提供方卡片、手动添加模型、模型目录识别、连通性测试，以及默认翻译模型和 OCR 模型选择。协议包括 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages。

模型目录的上下文窗口、最大输出是能力元数据；`output_limit` 是用户选择的本次输出上限，两者分别保存和校验。目录信息缺失时保持未知，不凭模型名推定。已保存密钥不回显，空白输入保持原值；连通性测试会实际请求对应服务。

| 快捷键 | 默认值 |
|---|---|
| 划词翻译 | `Ctrl+Alt+T` |
| 截图翻译 | `Ctrl+Alt+S` |
| 主窗口显隐 | `Ctrl+Alt+M` |
| 隐藏到托盘 | `Ctrl+Alt+Q` |

保存时校验快捷键格式与冲突，重新注册失败会显示原因。

OCR 可配置视觉模型、OCR.space 或百度 OCR。同一候选图内可用引擎并行识别，没有有效文字才继续下一候选。候选包括原图、二值图及适用的小图放大，缓存隔离模型和配置变更。OCR.space 使用 Engine 3 和自动语言识别；缺少可用引擎、额度耗尽或服务失败均明确报错。

俄语客服提示词包括口语、电商术语、型号保护和输出约束；返回结果进行标签及目标语种检查。规则检查不能代替语义质量验证。

## 5. 桌面与浏览器审查

默认桌面模式仅监听 `127.0.0.1`，不自动开放局域网，也不在 WebView2 失败时回退浏览器。

```text
zshtrans.exe [flags]
  -port int       HTTP 端口，默认读配置，缺省 8765
  -host string    明确指定监听地址，覆盖默认回环地址
  -web            显式使用浏览器模式，用于审查或受控共享
  -no-open        不自动打开界面
  -no-hotkeys     不注册全局快捷键
  -no-tray        不显示托盘图标
  -no-single      允许测试实例并存
  -console        输出控制台日志
  -wv-debug       开启 WebView2 调试工具
```

浏览器审查示例：

```powershell
.\dist\zshtrans.exe -web -no-open -no-hotkeys -no-tray -port 9000
```

随后访问 `http://127.0.0.1:9000/`。显式使用 `-web -host 0.0.0.0` 时，局域网页面仅提供允许的翻译界面，隐藏配置、常用语、自检、更新和本机捕获管理入口。服务检查 Host、请求来源和同源关系；本机 SSE 事件也受本机访问限制。局域网模式不是带账号认证的公网服务。

## 6. 更新与排障

开启自动检查时，启动后查询 GitHub Releases，也可手动检查。用户先选择「下载更新」，下载与校验完成后，再单独点击「安装并重启」。关闭弹窗或选择「稍后」不会安装；支持取消下载和跳过版本。中断、错误或无法确认的状态不显示为安装成功。

| 现象 | 处理 |
|---|---|
| 桌面窗口创建失败 | 安装或修复 WebView2 Runtime 后重试 |
| 端口被占用 | 检查现有实例，或选其他端口 |
| 翻译失败或较慢 | 查看完整提供方错误和超时配置，检查服务及默认模型 |
| 截图组件校验失败 | 使用完整发行 exe，检查资源是否被修改 |
| OCR 没有文字 | 检查图像清晰度、OCR 配置和具体引擎错误 |
| 划词拒绝操作剪贴板 | 旧内容含不能安全快照的对象；先复制目标文字或使用截图 |
| 热键注册失败 | 更换与其他软件冲突的组合键后保存 |

自检页提供版本、路径、提供方和缓存等结构信息，不代表全部权限、语言字形或业务场景已验收。

## 7. 从源码验证与构建

使用已安装且兼容 `go.mod`、前端锁文件的 Go、Node.js、pnpm 和 go-winres。依赖按锁文件安装，不通过 `@latest` 自动换版本。从项目根目录执行以下命令；先完成浏览器审查，再生成交付 exe。

```powershell
.\scripts\prepare-runtime.ps1

Push-Location frontend
pnpm install --frozen-lockfile
pnpm test
pnpm build
Pop-Location

go test -buildvcs=false ./...
go vet -unsafeptr=false ./...

go-winres make --in winres/winres.json --arch amd64 --out cmd/zshtrans/rsrc
go build -buildvcs=false -ldflags="-s -w -H windowsgui" -o dist/zshtrans.exe ./cmd/zshtrans
```

`pnpm build` 包含 TypeScript 检查，写入 `internal/web/dist`，它是 `go:embed` 的编译输入。`-H windowsgui` 避免正式 exe 启动时出现控制台。Win32 GDI 与 IPC 桥接使用 native 指针，vet 沿用 `-unsafeptr=false`，不能因此省略原生运行验证。

`internal/capture/bundled/qq-runtime.zip` 是必要编译输入，作为 Release 附件分发，不提交原生二进制到源码仓库。`prepare-runtime.ps1` 获取固定版本并核验 SHA-256。只有重新打包已核验组件时才调用 `scripts/capture-runtime.ps1` 并传入 `-SourceDirectory` 组件根目录；脚本按清单验证和收集 71 个文件，并附清单与许可，不收集用户配置。

## 8. 本轮验证范围

源码全量检查：开启 DLL smoke 的 Go 检查中，98 个顶层测试、10 个测试包通过；Go vet 通过，前端 7 项 Node 测试和 TypeScript 检查通过。官方 registry 的 pnpm audit 未报告已知漏洞。浏览器在 1440 / 768 / 390 宽度检查了翻译、设置五个子页、常用语、自检，并实际保存设置。

- 受控验证包括协议、资源 SHA-256、回调生命周期和独立子进程中的 DLL 初始化。剪贴板使用 Win32 shim 验证，仅实际创建隐藏 owner 和分配内存，没有操作用户剪贴板。
- 本轮未执行真实桌面框选、标注、滚轮长截图、钉图或物理热键操作。组件已接入，真实交互仍需实际使用确认。
- OCR.space 两张合成图取得读回：俄文样本精确一致，约 2.169 秒；中文样本按空白和冒号规范化后一致，约 1.096 秒。结果不代表任意截图的准确率或固定速度。
- 俄中模型仅完成有限样本：6 次翻译调用（含连通性探测）和 1 次评审取得结果，中断请求不计作完成。未完成完整中译俄测试集，也未进行本轮新提示词调优。实际 token 用量未可靠核对，不报告数值。
- 旧记录中的跨模型、跨样本均分不可直接比较，不能作为 v3.4 的质量提升成绩。后续需以固定业务样本持续观察自然度、型号保留、OCR 准确性和耗时。

最终全量检查及发行包验证以对应版本发行验收结果为准，不由上述有限样本代替。

## 9. 代码与接口

`cmd/zshtrans` 管理入口与生命周期；`internal/capture` 提供剪贴板、GDI 与 QQ 截图桥接；`internal/deskwin` 提供桌面和结果窗口；`internal/server` 提供 HTTP / SSE；`engine`、`providers`、`ocr` 处理翻译和识别；`config`、`phrases`、`update` 管理数据和更新。前端位于 `frontend/`。

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/status`、`/api/languages`、`/api/providers` | 状态、语言、提供方摘要 |
| POST | `/api/translate` | 文本翻译 |
| GET | `/api/config`、`/api/selfcheck` | 本机配置、自检 |
| POST | `/api/settings`、`/api/models`、`/api/test` | 本机设置、模型目录、连通性测试 |
| GET / POST | `/api/phrases` | 本机常用语 |
| POST | `/api/ocr`、`/api/selection`、`/api/capture` | 本机 OCR、划词、截图 |
| GET | `/api/events` | 受限的本机事件流 |
| GET | `/api/update/check` | 本机检查更新 |
| POST | `/api/update/apply` | 下载、取消、查询状态或显式安装 |

详细基线见 [FEATURES.md](docs/FEATURES.md)，版本变化见 [CHANGELOG.md](CHANGELOG.md)。
