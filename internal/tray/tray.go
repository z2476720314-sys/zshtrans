// Package tray 系统托盘（对应 translate.py _build_tray：
// 显示主窗口 / 截图翻译 / 设置 / 退出，左键点击唤起主窗口）。
//
// 托盘直接控制桌面窗口；截图触发既有截图链路，退出结束进程。
package tray

// Actions 托盘菜单动作
type Actions struct {
	Show     func() // 显示主窗口
	Capture  func() // 截图翻译
	Settings func() // 打开设置
	Quit     func() // 退出
}
