package main

import (
	"net"
	"strconv"
)

func openAppListener(host string, configuredPort, explicitPort int, browserMode bool) (net.Listener, error) {
	port := configuredPort
	if explicitPort > 0 {
		port = explicitPort
	} else if !browserMode {
		port = 0
	} else if port == 0 {
		port = 8765
	}
	return net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
}
