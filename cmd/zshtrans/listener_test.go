package main

import (
	"net"
	"testing"
)

func occupiedPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}

func TestDesktopStartsWhenConfiguredPortIsOccupied(t *testing.T) {
	port := occupiedPort(t)
	listener, err := openAppListener("127.0.0.1", port, 0, false)
	if err != nil {
		t.Fatalf("desktop startup must not depend on the old configured port: %v", err)
	}
	defer listener.Close()
	actual := listener.Addr().(*net.TCPAddr)
	if actual.Port == 0 || actual.Port == port || !actual.IP.IsLoopback() {
		t.Fatalf("unexpected desktop listener: %v", actual)
	}
}

func TestExplicitAndBrowserPortsAreNotSilentlyChanged(t *testing.T) {
	port := occupiedPort(t)
	for _, mode := range []struct {
		name     string
		explicit int
		web      bool
	}{
		{"explicit desktop port", port, false},
		{"configured browser port", 0, true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			listener, err := openAppListener("127.0.0.1", port, mode.explicit, mode.web)
			if err == nil {
				listener.Close()
				t.Fatal("an explicitly fixed occupied port must fail")
			}
		})
	}
}
