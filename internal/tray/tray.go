// Package tray 系统托盘（对应 translate.py _build_tray：
// 显示主窗口 / 截图翻译 / 设置 / 退出，左键点击唤起主窗口）。
//
// Web 版没有原生窗口，"显示主窗口"与"设置"映射为在默认浏览器打开应用地址
// （设置页用 #settings 直达），"截图翻译"直接触发热键链路，"退出"结束进程。
package tray

// Actions 托盘菜单动作
type Actions struct {
	Show     func() // 显示主窗口
	Capture  func() // 截图翻译
	Settings func() // 打开设置
	Quit     func() // 退出
}
