package deskwin

// Controller 主窗口控制接口：热键与托盘通过它操作真实窗口，
// 在没有桌面窗口（浏览器模式 / 非 Windows）时由调用方传 nil，
// 走原来的 SSE 事件通知前端。
type Controller interface {
	Show()           // 显示并置前
	Hide()           // 隐藏到托盘
	Toggle()         // 显隐切换（对应「主窗口显隐」热键）
	Navigate(string) // 切页（托盘「设置」深链）
	Available() bool
}
