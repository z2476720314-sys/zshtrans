# 原生截图构建资源

发行 EXE 内嵌完整截图资源。源码仓库不存放原生 EXE、DLL 或资源压缩包。
首次从源码构建前，在仓库根目录执行：

```powershell
.\scripts\prepare-runtime.ps1
```

脚本从本项目 `v3.4` Release 取得 `qq-runtime-9.9.30-48517.zip`，验证固定
SHA-256 后写入本目录的 `qq-runtime.zip`，供 `go:embed` 使用。
已有正确文件时仅校验，不重复联网；已有文件摘要不符时停止，不覆盖。

如需要从已核验的原始组件重建压缩包，使用 `scripts/capture-runtime.ps1`。
文件清单、组件许可和运行时逐文件校验均保留，详见 `THIRD_PARTY_NOTICES.md`。
