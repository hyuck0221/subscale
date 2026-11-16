package router

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
)

func TestHTTPRoutingAndNoFallback(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy credential forwarded")
		}
		io.WriteString(w, r.Host)
	}))
	defer origin.Close()
	var direct, account atomic.Int32
	p := &Proxy{Routes: func() []config.Route {
		return []config.Route{{Account: "work", Host: "db.example", Target: "100.64.1.2"}}
	}, DialAccount: func(ctx context.Context, name, address string) (net.Conn, error) {
		account.Add(1)
		if name != "work" || address != "100.64.1.2:80" {
			t.Errorf("account dial %s %s", name, address)
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(origin.URL, "http://"))
	}, Direct: func(ctx context.Context, network, address string) (net.Conn, error) {
		direct.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	server := httptest.NewServer(p)
	defer server.Close()
	u, _ := url.Parse(server.URL)
	tr := &http.Transport{Proxy: http.ProxyURL(u)}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	req, _ := http.NewRequest("GET", "http://db.example/path", nil)
	req.Header.Set("Proxy-Authorization", "secret")
	res, e := c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "db.example" || account.Load() != 1 || direct.Load() != 0 {
		t.Fatalf("wrong route: %s", b)
	}
	res, e = c.Get(origin.URL)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if direct.Load() != 1 {
		t.Fatal("unmatched route did not use OS")
	}
	p.DialAccount = func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("logged out") }
	res, e = c.Get("http://db.example")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 502 || direct.Load() != 1 {
		t.Fatal("failed account leaked into direct route")
	}
}
func TestConnectTLS(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "encrypted tailnet response") }))
	defer origin.Close()
	p := &Proxy{Routes: func() []config.Route { return []config.Route{{Account: "work", Host: "secure.example.com"}} }, DialAccount: func(ctx context.Context, _, address string) (net.Conn, error) {
		if address != "secure.example.com:443" {
			t.Errorf("dial: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(origin.URL, "https://"))
	}}
	proxy := httptest.NewServer(p)
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	client := origin.Client()
	tr := client.Transport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyURL(u)
	defer tr.CloseIdleConnections()
	client = &http.Client{Transport: tr, Timeout: 5 * time.Second}
	res, e := client.Get("https://secure.example.com/")
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if string(b) != "encrypted tailnet response" {
		t.Fatalf("response: %s", b)
	}
}
func TestConnectBufferedBytes(t *testing.T) {
	origin, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer origin.Close()
	go func() {
		c, e := origin.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		b := make([]byte, 4)
		if _, e = io.ReadFull(c, b); e == nil {
			c.Write(b)
		}
	}()
	p := &Proxy{Routes: func() []config.Route { return nil }}
	server := httptest.NewServer(p)
	defer server.Close()
	c, e := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	io.WriteString(c, "CONNECT "+origin.Addr().String()+" HTTP/1.1\r\nHost: "+origin.Addr().String()+"\r\n\r\nPING")
	reader := bufio.NewReader(c)
	res, e := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
	if e != nil || res.StatusCode != 200 {
		t.Fatalf("CONNECT = %v %v", res, e)
	}
	b := make([]byte, 4)
	if _, e = io.ReadFull(reader, b); e != nil || string(b) != "PING" {
		t.Fatalf("buffered bytes = %s %v", b, e)
	}
}
func TestPACAndValidation(t *testing.T) {
	pac := PAC([]config.Route{{Account: "work", Host: "*.example.com"}, {Account: "home", Host: "100.64.1.2"}})
	if strings.Contains(pac, "DIRECT;") || !strings.Contains(pac, "return 'DIRECT'") || !strings.Contains(pac, "host.slice(-12)") {
		t.Fatalf("bad PAC: %s", pac)
	}
	p := &Proxy{Routes: func() []config.Route { return nil }}
	for _, target := range []string{"bad", "host:0", "host:65536", "host:abc"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("CONNECT", "http://x", nil)
		r.Host = target
		p.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Errorf("accepted CONNECT %q", target)
		}
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("GET", "/proxy.pac", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "FindProxyForURL") {
		t.Fatal("PAC endpoint failed")
	}
}
