package deskwin

import "testing"

func TestResultPositionStaysInsideCurrentMonitor(t *testing.T) {
	for _, c := range []struct {
		x, y, scale int
		work        popupRect
	}{
		{1910, 1030, 100, popupRect{0, 0, 1920, 1040}},
		{-5, 800, 150, popupRect{-1920, 0, 0, 1040}},
		{50, 50, 200, popupRect{0, 0, 360, 600}},
	} {
		r := resultWindowRect(c.x, c.y, c.scale, c.work)
		if r.left < c.work.left || r.top < c.work.top || r.right > c.work.right || r.bottom > c.work.bottom || r.right <= r.left || r.bottom <= r.top {
			t.Fatalf("off-monitor popup: %+v in %+v", r, c.work)
		}
	}
}

func TestResultDismissIgnoresClicksInside(t *testing.T) {
	r := popupRect{100, 100, 540, 470}
	if outsideResult(110, 120, r) {
		t.Fatal("click inside popup dismissed it")
	}
	if !outsideResult(540, 120, r) || !outsideResult(90, 120, r) {
		t.Fatal("outside click did not dismiss popup")
	}
}
