# 第三方组件说明

## QQImpl IPC 桥接

本项目复用 [EEEEhex/QQImpl](https://github.com/EEEEhex/QQImpl) 的 QQ Mojo IPC 接口和已有桥接组件。组件依据 GPL-3.0 许可提供；已记录的 QQImpl 来源提交为 `3aac29788fc63d25665a0ae473b8b8c788ed20bb`。

完整 `QQ_CAPTURE_LICENSE` 随 `internal/capture/bundled/qq-runtime.zip` 内嵌，并在首次使用截图功能时释放到该版本的原生运行时目录。分发相关组件时应同时保留该许可及适用的来源说明。

## 腾讯原版截图组件

截图界面、标注、长图拼接和钉图由随包提供的腾讯原版 QQ 截图组件及资源承担，版本为 `9.9.30-48517`。原组件和资源保留其原权利归属；QQImpl 的开源许可不改变腾讯组件本身的权利与许可。

内嵌清单记录 71 个原始文件的 SHA-256。宿主校验资源后，仅在自己启动且验证匹配的截图子进程中应用已有工具栏修正；磁盘中的原文件保持不变。

## 其他依赖

Go 和前端依赖及其固定版本分别由 `go.mod` / `go.sum`、`frontend/package.json` / `frontend/pnpm-lock.yaml` 记录。各依赖保留自身许可和版权声明。Microsoft Edge WebView2 Runtime 使用 Microsoft 提供的运行时与适用条款。

本文件用于说明第三方组件，不替整个 Go 项目指定许可证，也不将所有内嵌组件统一重新许可。
