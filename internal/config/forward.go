package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Forward struct {
	Account   string `json:"account"`
	Target    string `json:"target"`
	LocalPort int    `json:"local_port"`
}

func (f Forward) LocalAddress() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(f.LocalPort))
}

// Endpoint accepts a concrete TCP host:port, including bracketed IPv6.
func Endpoint(s string) (string, error) {
	host, port, err := net.SplitHostPort(s)
	if err != nil || strings.Contains(s, "://") {
		return "", fmt.Errorf("expected HOST:PORT (IPv6: [ADDRESS]:PORT)")
	}
	host, err = Host(host, false)
	if err != nil {
		return "", err
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("invalid TCP port %q", port)
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid TCP port %q", port)
	}
	return net.JoinHostPort(host, strconv.Itoa(n)), nil
}
