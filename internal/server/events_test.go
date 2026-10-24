package server

import (
	"testing"
	"time"
)

// TestBusPublishToggle 验证热键 → SSE 事件总线的链路（窗口显隐 / 关闭）
// 对应 pyc 主窗口显隐与托盘隐藏，这两条不依赖屏幕与剪贴板，可离线验证。
func TestBusPublishToggle(t *testing.T) {
	b := newBus()
	ch := b.subscribe()
	defer b.unsubscribe(ch)

	app := &App{bus: b}
	FireHotkey(app, "window")
	FireHotkey(app, "close")

	want := []string{"toggle", "close"}
	for _, kind := range want {
		select {
		case ev := <-ch:
			if ev.Kind != kind {
				t.Fatalf("期望事件 %q，实际 %q", kind, ev.Kind)
			}
		case <-time.After(time.Second):
			t.Fatalf("等待事件 %q 超时", kind)
		}
	}
}

// TestBusSlowSubscriberDropped 慢订阅者不阻塞发布方（SSE 必须非阻塞）
func TestBusSlowSubscriberDropped(t *testing.T) {
	b := newBus()
	ch0 := b.subscribe()
	defer b.unsubscribe(ch0)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			b.publish(Event{Kind: "log", Data: i})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("发布被慢订阅者阻塞")
	}
}
