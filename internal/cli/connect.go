package cli

import (
	"bufio"
	"fmt"
	"github.com/hyuck0221/subscale/internal/config"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

func connect(host, port string) error {
	h, e := config.Host(host, false)
	if e != nil {
		return e
	}
	p, e := strconv.Atoi(port)
	if e != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid port %q", port)
	}
	conn, e := net.DialTimeout("tcp", config.ProxyAddress, 5*time.Second)
	if e != nil {
		return e
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(35 * time.Second))
	address := net.JoinHostPort(h, port)
	if _, e = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", address, address); e != nil {
		return e
	}
	reader := bufio.NewReader(conn)
	res, e := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
	if e != nil {
		return e
	}
	if res.StatusCode != 200 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("proxy: %s: %s", res.Status, string(b))
	}
	conn.SetDeadline(time.Time{})
	go func() {
		io.Copy(conn, os.Stdin)
		if tcp, ok := conn.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	_, e = io.Copy(os.Stdout, reader)
	return e
}
