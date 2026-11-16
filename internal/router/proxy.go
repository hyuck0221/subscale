package router

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
)

type Dialer func(context.Context, string, string) (net.Conn, error)
type Proxy struct {
	Routes      func() []config.Route
	DialAccount func(context.Context, string, string) (net.Conn, error)
	Direct      Dialer
}

func (p *Proxy) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	if r, ok := config.Match(p.Routes(), host); ok {
		if r.Target != "" {
			host = r.Target
		}
		return p.DialAccount(ctx, r.Account, net.JoinHostPort(host, port))
	}
	if p.Direct != nil {
		return p.Direct(ctx, network, address)
	}
	return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, address)
}
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/proxy.pac" && !r.URL.IsAbs() {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, PAC(p.Routes()))
		return
	}
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	if !r.URL.IsAbs() || r.URL.Scheme != "http" || r.URL.Host == "" {
		http.Error(w, "HTTP proxy requires an absolute http:// URL or CONNECT", 400)
		return
	}
	// A transport per request prevents reusing a tunnel after a rule/account changes.
	tr := &http.Transport{DialContext: p.dial, ForceAttemptHTTP2: false, ResponseHeaderTimeout: 30 * time.Second, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	proxy := &httputil.ReverseProxy{Director: func(req *http.Request) {
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Proxy-Connection")
		req.Header.Del("Forwarded")
		req.Header.Del("X-Forwarded-For")
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Forwarded-Proto")
	}, Transport: tr, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) { http.Error(w, "subscale: "+e.Error(), 502) }}
	proxy.ServeHTTP(w, r)
}
func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	host, port, e := net.SplitHostPort(r.Host)
	n, pe := strconv.Atoi(port)
	if e != nil || host == "" || pe != nil || n < 1 || n > 65535 {
		http.Error(w, "CONNECT requires host:port", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	upstream, e := p.dial(ctx, "tcp", r.Host)
	if e != nil {
		http.Error(w, "subscale: "+e.Error(), 502)
		return
	}
	defer upstream.Close()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unavailable", 500)
		return
	}
	client, rw, e := hj.Hijack()
	if e != nil {
		return
	}
	defer client.Close()
	if _, e = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); e != nil {
		return
	}
	if e = rw.Flush(); e != nil {
		return
	}
	done := make(chan struct{})
	go func() { defer close(done); io.Copy(upstream, rw); closeWrite(upstream) }()
	io.Copy(client, upstream)
	closeWrite(client)
	client.Close()
	upstream.Close()
	<-done
}
func closeWrite(c net.Conn) {
	if v, ok := c.(interface{ CloseWrite() error }); ok {
		v.CloseWrite()
	}
}
func PAC(routes []config.Route) string {
	var b strings.Builder
	b.WriteString("function FindProxyForURL(url, host) {\n  host = host.toLowerCase().replace(/\\.$/, '').replace(/^\\[|\\]$/g, '');\n")
	for _, r := range routes {
		h := strconv.Quote(r.Host)
		condition := "host === " + h
		if strings.HasPrefix(r.Host, "*.") {
			suffix := strconv.Quote(r.Host[1:])
			condition = "host.length > " + strconv.Itoa(len(r.Host)-1) + " && host.slice(-" + strconv.Itoa(len(r.Host)-1) + ") === " + suffix
		}
		fmt.Fprintf(&b, "  if (%s) return %q;\n", condition, "PROXY "+config.ProxyAddress)
	}
	b.WriteString("  return 'DIRECT';\n}\n")
	return b.String()
}
