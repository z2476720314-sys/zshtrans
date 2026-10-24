# 更新记录

## v3.2 — 2026-10-25

联调自动更新时发现的三处真实缺陷。三条都只在「真的去 GitHub 拉一次新版本」时才会暴露，
单测和本地起服务都测不出来。

### 修复
- **新版本永远检测不到**：`CheckRepo` 走 GitHub 的 `/releases/latest`，该接口按 `created_at`
  取「最新」。同一天连续发两个 release 时排序不稳定，实测 v3.2 建好后 `latest` 仍指向 v3.1，
  于是 v3.1 的用户永远看到「已是最新」。改为拉 `/releases?per_page=30` 列表，自己按版本号取
  最大，并跳过 draft / prerelease。
- **下载 10MB 就超时中断**：检查更新与下载共用同一个 20 秒总超时的 client，实测在国内网络
  只下到 1.2MB 就 `Client.Timeout`。下载改用独立 client（只限制连接/握手/首字节，不限制
  传输总时长），并对握手超时、中途 `wsarecv` 断流做 3 次带退避的重试。
- **进度轮询被自己挡掉**：`/api/update/apply` 的 `busy` 检查放在最前面，前端每 700ms 轮询
  `{action:"status"}` 全部收到 409，进度条不动、下载完也不会自动安装。`status` 改为在锁外
  优先处理，任何时刻都能读。

### 其它
- 新增 `.gitattributes` 钉死 `eol=lf`（`*.bat` 保留 CRLF），并把 `core.autocrlf` 设为 false。
  此前检出时变 CRLF，`gofmt -l` 会把这些文件一直报成未格式化。

---

## v3.1 — 2026-10-25

代码审查（独立只读评审 7091 行 / 19 包）发现的 20 条缺陷全部修复，并补齐单元测试。
本轮无新增功能，只做正确性与安全性加固。

### 修复 · 高危
- **全局热键全部失效**：`RegisterHotKey(hWnd=0)` 把热键绑到调用线程的消息队列，但注册在主 goroutine、
  泵消息在另一个加了 `LockOSThread` 的 goroutine，主 goroutine 随即进入 WebView2 消息循环并把
  `hwnd=NULL` 的 `WM_HOTKEY` 丢弃。重写为「注册与泵消息都在同一个锁定的 OS 线程」，并支持改设置时
  热更新注册（新增 `Rebind`）。
- **更新包不校验完整性**：原来从不比较 `done == total`，也不校验文件头，`Content-Length` 缺失或提前
  EOF 时截断的 exe 会被装上并重启且无法回滚。现在校验长度、非空、MZ/PE 签名；更新脚本先备份旧 exe，
  `copy` 失败走 `:rollback` 分支还原。
- **默认监听 `0.0.0.0` + 零鉴权 + `Access-Control-Allow-Origin: *`**：任意局域网主机或恶意网页可读走
  全部密钥、改写提供方地址、触发更新。现在：Host 头必须是 IP 或 `localhost`（防 DNS rebinding）；
  写操作类接口（设置 / 模型 / 测试 / 常用语 / OCR / 自检 / 更新 / 划词 / 截图）只允许回环来源；
  CORS 只回显本机与私网来源，不再发 `*`。

### 修复 · 中危
- 更新下载的 `busy` 标志改为先原子置位再发起网络请求，消除并发重复下载；失败路径统一释放占位。
- `/api/settings` 的读-改-写改为在 `App.Mutate` 里串行执行，并发保存不再互相覆盖；热键保存失败时
  返回 200 并带 `hotkey_error`，不再静默失败。
- `engineUsable` 判定直连引擎时会校验必需密钥，不再允许保存「默认引擎缺密钥」的必然失败配置。
- `/api/ocr` 加 1600 万像素上限，挡住 PNG 解压炸弹（10000×10000 解码即 400MB RGBA）。
- `readBody` 不再静默截断：`Content-Length` 超限在读之前就拒绝，读后再复查实际长度。
- 百度 OCR 的 access_token 加双重互斥锁（原为包级变量并发读写，存在 data race）。
- OCR 缓存键改用实际胜出的候选图哈希，不再恒用原图，避免缓存污染导致下次跳过更优候选。
- 坏配置文件改名为 `.bad` 留档后再回落默认值，不再在用户下次保存时用空配置覆盖掉密钥。
- 删除「`ocrspace` 为空时回填 `helloworld`」的逻辑，用户主动清空后不再被复活。
- 配置目录改为「exe 目录可写就用，否则用 `%APPDATA%\ZSH TRANS`」，装到 `C:\Program Files` 后保存
  不再静默失败。
- 划词取词真正还原用户原剪贴板（原实现只把原内容用于判空，从不写回，每划一次词就永久覆盖剪贴板）。
- 更新脚本名带 PID、按 PID 杀进程（不再 `taskkill /im` 误杀同名实例）。

### 修复 · 低危
- `Store.Get` 返回深拷贝，调用方修改不再污染内部状态。
- `keyByRef` 的 `key_ref` 与密钥值都做 `TrimSpace`。
- `/api/test` 返回真实耗时（原恒为 0）；OCR 失败信息里的放大倍数不再恒为 `1.0`。
- 桌面窗口子类化失败时撤销占位允许重试，不再让窗口永久关不掉。
- HTTP 服务补 `ReadTimeout`/`IdleTimeout`；`-console` 现在真的会同时输出到 stderr。

### 新增 · 单元测试
45 个测试，覆盖 `config` / `engine` / `hotkeys` / `lang` / `ocr` / `server` / `update` 七个包
（此前 19 个包里只有 `server` 有两个测试）。测试顺带抓出一个真实缺陷：引擎的「健康优先」排序原来
只会把不健康的节点往右推一格，无法把排在队首的不健康节点挪到后面，与 `order.sort(key=not healthy)`
不等价，已改为 `sort.SliceStable`。

---

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
