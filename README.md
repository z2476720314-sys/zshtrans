# ZSH TRANS v3.5.1 · 跨境翻译台

Windows 桌面翻译工具，支持手动翻译、划词、交互截图、常用语和可选更新。双击 EXE 打开桌面窗口，不启动外部浏览器，不监听 localhost 或局域网端口。Go 业务与内嵌 React / Ant Design 界面通过进程内消息通信。

需要 Windows x64 和 Microsoft Edge WebView2 Runtime，无需 Python 或 Qt。缺少 WebView2 时报告错误。翻译、OCR 和检查更新仍按对应服务配置访问互联网。

## 安装与数据

把发行版 `zshtrans.exe` 放在可写目录后运行。优先在 EXE 同级保存数据；目录不可写时使用 `%APPDATA%\ZSH TRANS`。自检页显示实际位置。

| 文件或目录 | 内容 |
|---|---|
| `tt_config.json` | 语言、三个快捷键、模型提供方和接口配置 |
| `tt_phrases.json` | 常用语 |
| `zshtrans.log` | 运行日志 |
| `UI/index.html` | 从 EXE 内嵌资源生成的自包含本地界面 |
| `WebView2/` | 桌面界面的用户数据 |
| `.updates/` | 下载后等待用户安装的更新 |
| `native-runtime/9.9.30-48517/` | 首次截图时释放并校验的原生组件 |

新便携目录尚无配置时，尝试复制旧版 Roaming 配置与常用语，不覆盖已有数据、不修改旧文件。旧配置里的 `close`、`web` 字段不再参与运行；其他设置与密钥继续读取。密钥留空或保持占位值时不会覆盖原值，自定义密钥槽可以保存和重载，读取接口不回显密钥。

点 X 隐藏主窗口；用同一个“主窗口显隐”快捷键或托盘图标恢复。真正退出使用托盘“退出”。卸载前先退出，再备份所需配置和常用语；WebView2 是系统共享运行时。

## 翻译与截图

- 原文、译文左右排列，支持自动检测和 15 种语言，手动语向、交换语向与快捷翻译沿用原有规则。自动检测无法可靠区分的语言请手动选择。
- 支持三种模型协议、自定义提供方、请求输出上限、取消、失败降级与缓存隔离。提供方缺额、不可用或返回异常会明确报告。
- 划词复制兼容延迟提供剪贴板文本的应用；Alt/Win 热键先按旧版顺序消除按键干扰，首次复制未生效时只在原窗口补试一次。切换窗口后停止，旧剪贴板内容不会被当作新选区。
- 常用语支持添加、编辑、复制与删除。翻译草稿在正常切页时保留；完整刷新界面会重置未持久化的草稿。
- QQ 原生截图支持框选、标注、长截图、复制、保存与钉图。71 个运行文件内嵌于 EXE，按 manifest 验证 SHA-256，无需依赖旧 Python 目录。许可见 [第三方说明](THIRD_PARTY_NOTICES.md)。
- 普通截图完成、复制、保存不进入 OCR 或翻译；“提取文字”只执行 OCR，“翻译”才识别并翻译。空或未知动作不会默认为翻译。
- QQ 正常完成后的迟到断连不会改成错误；无效回调仍会明确报错。回调长度按 signed 32-bit C `int` 读取。
- 独立结果弹窗沿用旧版紧凑布局：约 440 宽、随译文长度调整高度、右上“复制”、底部来源与耗时。配色与新版主界面一致：深色底、浅色译文、荧光绿操作色、粉红错误色。长文可滚动，支持外点、Esc 与 15 秒自动关闭，不主动抢焦点。
- OCR.space、百度 OCR、视觉模型及其候选择优、缓存规则保留；实际可用性取决于相应服务。

原功能逐项清单见 [功能清单](docs/FEATURES.md)。

## 快捷键

| 操作 | 新配置默认值 |
|---|---|
| 划词翻译 | `Ctrl+Alt+T` |
| 截图 | `Ctrl+Alt+S` |
| 主窗口显隐 | `Ctrl+Alt+M` |

已有自定义快捷键继续读取。三项必须互不冲突；保存等待 Windows 实际注册结果。启动遇到无效旧配置后，可在设置中修正并恢复，无需重启。独立“隐藏到托盘”热键已删除。

## 更新与启动选项

开启自动检查后，程序查询 GitHub Releases；也可手动检查。用户先下载并验证附件，再单独点击“安装并重启”。关闭更新面板或选择稍后不会安装；支持取消与跳过。

```
zshtrans.exe
  -no-hotkeys    不注册全局快捷键
  -no-tray       不显示托盘图标
  -no-single     允许多开
  -console       日志同时输出到控制台
  -wv-debug      开启 WebView2 调试工具
```

浏览器模式与 `-web`、`-host`、`-port`、`-no-open` 已删除。重复启动 EXE 只唤起已有桌面窗口。

## 源码构建

使用兼容 `go.mod` 与前端锁文件的 Go、Node.js、pnpm、go-winres。依赖与下载缓存按本机规则配置到非系统盘，依赖按锁文件安装，不通过 `@latest` 自动升级。

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
go build -buildvcs=false -trimpath -ldflags="-s -w -H windowsgui" -o dist/zshtrans.exe ./cmd/zshtrans
```

前端构建写入 `internal/web/dist`，它是 `go:embed` 的输入。原生资源压缩包不提交到 Git；`prepare-runtime.ps1` 获取固定版本并核验摘要。只有重新收集已校验组件时才使用 `capture-runtime.ps1`，不要打包用户配置。

Win32 IPC、GDI 与窗口回调使用原生指针，静态校验沿用 `-unsafeptr=false`；这不等于真实桌面交互已经验收。

## 验证边界

本次增加了进程内请求与取消、事件、跨刷新响应隔离、截图动作、回调 ABI 和弹窗尺寸回归；另有独立翻译链路审查。安装后的进程与子进程已检查为无 TCP 监听，实际配置与界面通过本地文件加载。

本轮没有新增真实模型或 OCR 线上调用。旧有有限样本不能代表完整中俄双向质量，也不能据此宣称任意截图准确率。真实截图交互、弹窗外观和混合 DPI 需结合对应版本的实际验收结果判断。
