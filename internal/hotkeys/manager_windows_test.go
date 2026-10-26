//go:build windows

package hotkeys

import (
	"testing"
	"time"
)

func TestManagerRejectsDuplicateWindowBinding(t *testing.T) {
	m := NewManager()
	if err := m.Bind("ctrl+alt+shift+f17", "ctrl+alt+shift+f18", "SHIFT+ALT+CTRL+F17", func(string) {}); err == nil {
		t.Fatal("duplicate window/selection binding was accepted")
	}
}

func TestManagerRecoversAfterInvalidInitialBinding(t *testing.T) {
	m := NewManager()
	if err := m.Bind("ctrl+alt+shift+f17", "ctrl+alt+shift+f18", "-", func(string) {}); err == nil {
		t.Fatal("invalid initial binding was accepted")
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	fired := make(chan string, 1)
	if err := m.Rebind("ctrl+alt+shift+f17", "ctrl+alt+shift+f18", "ctrl+alt+shift+f19", func(name string) { fired <- name }); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	tid := m.threadID
	m.mu.Unlock()
	if ok, _, err := procPostThreadMsg.Call(uintptr(tid), wmHotkey, 3, 0); ok == 0 {
		t.Fatal(err)
	}
	select {
	case name := <-fired:
		if name != "window" {
			t.Fatalf("hotkey dispatched %q, want window", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("repaired binding did not reach the window action")
	}
}

func TestRebindReportsWindowsRegistrationFailure(t *testing.T) {
	owner := NewManager()
	if err := owner.Bind("ctrl+alt+shift+f17", "ctrl+alt+shift+f18", "ctrl+alt+shift+f19", func(string) {}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Stop)
	m := NewManager()
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if err := m.Rebind("ctrl+alt+shift+f17", "ctrl+alt+shift+f18", "ctrl+alt+shift+f19", func(string) {}); err == nil {
		t.Fatal("rebind reported success although another Windows thread owns the hotkeys")
	}
}
