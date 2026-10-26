package deskwin

// Result 是热键与截图共用的独立译文弹窗内容。
type Result struct {
	Source, Text, Provider string
	Milliseconds           int64
}

type popupRect struct{ left, top, right, bottom int }

func resultWindowRect(x, y, scale, contentHeight int, work popupRect) popupRect {
	width, height := min(440*scale/100, max(1, work.right-work.left-24*scale/100)), min(contentHeight, work.bottom-work.top)
	x = max(work.left, min(x+14*scale/100, work.right-width))
	y = max(work.top, min(y+18*scale/100, work.bottom-height))
	return popupRect{x, y, x + width, y + height}
}

func outsideResult(x, y int, r popupRect) bool {
	return x < r.left || x >= r.right || y < r.top || y >= r.bottom
}
