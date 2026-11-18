package daemon

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
)

type forwardNodes struct {
	Nodes
	server string
	calls  chan string
}

func (n *forwardNodes) Dial(ctx context.Context, name, target string) (net.Conn, error) {
	n.calls <- name + "/" + target
	return (&net.Dialer{}).DialContext(ctx, "tcp", n.server)
}

func forwardAPI(t *testing.T) (*API, *forwardNodes) {
	t.Helper()
	server, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	go func() {
		for {
			c, err := server.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(5 * time.Second))
				data, _ := io.ReadAll(c)
				fmt.Fprint(c, strings.ToUpper(string(data)))
			}()
		}
	}()
	a := newAPI(t)
	n := &forwardNodes{Nodes: a.Nodes, server: server.Addr().String(), calls: make(chan string, 32)}
	a.Nodes = n
	t.Cleanup(a.closeForwards)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	return a, n
}

func forwardExchange(t *testing.T, f config.Forward) {
	t.Helper()
	c, err := net.DialTimeout("tcp", f.LocalAddress(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	io.WriteString(c, "hello through tailnet")
	c.(*net.TCPConn).CloseWrite()
	got, err := io.ReadAll(c)
	if err != nil || string(got) != "HELLO THROUGH TAILNET" {
		t.Fatalf("%q, %v", got, err)
	}
}

func TestForwardLifecycleAndHalfClose(t *testing.T) {
	a, nodes := forwardAPI(t)
	q := Request{Op: "forward-add", Name: "work", Target: "DB.Example:5432"}
	out := mustRequest(t, a, q)
	f := out.Config.Forwards[0]
	if f.LocalPort == 0 || f.Target != "db.example:5432" || out.ForwardStatus[f.LocalPort] != "listening" {
		t.Fatalf("%+v", out)
	}
	// No HTTP proxy route is needed, and each connection uses the named account.
	forwardExchange(t, f)
	if got := <-nodes.calls; got != "work/db.example:5432" {
		t.Fatal(got)
	}
	out = mustRequest(t, a, q)
	if len(out.Config.Forwards) != 1 || out.Config.Forwards[0] != f {
		t.Fatal("add was not idempotent")
	}
	loaded, err := config.Load(a.Dir)
	if err != nil || loaded.Forwards[0] != f {
		t.Fatalf("persistence: %+v %v", loaded, err)
	}
	// Unrelated configuration changes must preserve forwards.
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "other.example"})
	mustRequest(t, a, Request{Op: "rename", Name: "work", NewName: "office"})
	forwardExchange(t, f)
	if got := <-nodes.calls; got != "office/db.example:5432" {
		t.Fatal(got)
	}
	mustRequest(t, a, Request{Op: "purge", Name: "office"})
	if c, err := net.DialTimeout("tcp", f.LocalAddress(), time.Second); err == nil {
		c.Close()
		t.Fatal("logged out listener remains open")
	}
	mustRequest(t, a, Request{Op: "login", Name: "office"})
	out = mustRequest(t, a, Request{Op: "forward-list", Name: "office"})
	if len(out.Config.Forwards) != 0 {
		t.Fatal("login restored a forward removed by logout")
	}
	q.Name = "office"
	f = mustRequest(t, a, q).Config.Forwards[0]
	forwardExchange(t, f)
	<-nodes.calls
	mustRequest(t, a, Request{Op: "forward-remove", Name: "office", Target: f.Target})
	if c, err := net.DialTimeout("tcp", f.LocalAddress(), time.Second); err == nil {
		c.Close()
		t.Fatal("removed listener remains open")
	}
}

func TestClearAccountSettings(t *testing.T) {
	a, nodes := forwardAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "home"})
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "work.example"})
	mustRequest(t, a, Request{Op: "add", Name: "home", Host: "home.example"})
	work := mustRequest(t, a, Request{Op: "forward-add", Name: "work", Target: "db:5432"}).Config.Forwards[0]
	home := mustRequest(t, a, Request{Op: "forward-add", Name: "home", Target: "db:5432"}).Config.Forwards[1]
	conn, err := net.DialTimeout("tcp", work.LocalAddress(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case <-nodes.calls:
	case <-time.After(time.Second):
		t.Fatal("forward did not dial")
	}

	account := a.Config.Accounts["work"]
	mustRequest(t, a, Request{Op: "clear", Name: "work"})
	mustRequest(t, a, Request{Op: "clear", Name: "work"})
	if a.Config.Accounts["work"] != account || a.Nodes.Status(context.Background(), "work").State != "Running" {
		t.Fatal("clear changed account identity or disconnected it")
	}
	loaded, err := config.Load(a.Dir)
	if err != nil || len(loaded.Routes) != 1 || loaded.Routes[0].Account != "home" ||
		len(loaded.Forwards) != 1 || loaded.Forwards[0] != home {
		t.Fatalf("saved settings after clear: %+v, %v", loaded, err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("active forwarding connection stayed open: %v", err)
	}
	forwardExchange(t, home)
}

func TestForwardRestoreConflictAndRetry(t *testing.T) {
	a, _ := forwardAPI(t)
	q := Request{Op: "forward-add", Name: "work", Target: "db:5432"}
	out := mustRequest(t, a, q)
	f := out.Config.Forwards[0]
	a.closeForwards()
	loaded, err := config.Load(a.Dir)
	if err != nil {
		t.Fatal(err)
	}
	a.Config = loaded
	a.restoreForwards()
	forwardExchange(t, f)
	a.closeForwards()
	blocker, err := net.Listen("tcp", f.LocalAddress())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	a.restoreForwards()
	out = mustRequest(t, a, Request{Op: "forward-list", Name: "work"})
	if !strings.HasPrefix(out.ForwardStatus[f.LocalPort], "error:") {
		t.Fatalf("conflict hidden: %+v", out)
	}
	if _, code := request(t, a, q); code != 400 {
		t.Fatal("occupied port accepted")
	}
	blocker.Close()
	out = mustRequest(t, a, q)
	if out.Config.Forwards[0] != f || out.ForwardStatus[f.LocalPort] != "listening" {
		t.Fatal("retry lost saved port")
	}
	forwardExchange(t, f)
}

func TestForwardFailureKeepsConfigAndListeners(t *testing.T) {
	a, _ := forwardAPI(t)
	q := Request{Op: "forward-add", Name: "work", Target: "db:5432"}
	f := mustRequest(t, a, q).Config.Forwards[0]
	for _, invalid := range []Request{
		{Op: "forward-add", Name: "work", Target: "other:22", LocalPort: f.LocalPort},
		{Op: "forward-add", Name: "work", Target: "other:22", LocalPort: -1},
		{Op: "forward-add", Name: "work", Target: "other:22", LocalPort: 18765},
		{Op: "forward-add", Name: "work", Target: "*.example:22"},
		{Op: "forward-add", Name: "missing", Target: "other:22"},
	} {
		if _, code := request(t, a, invalid); code != 400 {
			t.Fatalf("accepted %+v", invalid)
		}
	}
	if err := os.Remove(filepath.Join(a.Dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(a.Dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, code := request(t, a, Request{Op: "forward-remove", Name: "work", Target: f.Target}); code != 400 {
		t.Fatal("save succeeded")
	}
	if _, code := request(t, a, Request{Op: "forward-add", Name: "work", Target: "other:22"}); code != 400 {
		t.Fatal("save succeeded")
	}
	if len(a.Config.Forwards) != 1 || len(a.forwards) != 1 {
		t.Fatal("failed save changed forwards or leaked listener")
	}
	forwardExchange(t, f)
}

func TestForwardRemovalClosesActiveConnections(t *testing.T) {
	a, nodes := forwardAPI(t)
	f := mustRequest(t, a, Request{Op: "forward-add", Name: "work", Target: "db:5432"}).Config.Forwards[0]
	c, err := net.Dial("tcp", f.LocalAddress())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case <-nodes.calls:
	case <-time.After(time.Second):
		t.Fatal("upstream not dialed")
	}
	mustRequest(t, a, Request{Op: "forward-remove", Name: "work", Target: f.Target})
	c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection remains open")
	} else if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("removal did not close active connection")
	}
}

func TestForwardExplicitPortAndAccountIsolation(t *testing.T) {
	a, nodes := forwardAPI(t)
	q := Request{Op: "forward-add", Name: "work", Target: "db:5432"}
	f := mustRequest(t, a, q).Config.Forwards[0]
	mustRequest(t, a, Request{Op: "login", Name: "home"})
	out := mustRequest(t, a, Request{Op: "forward-add", Name: "home", Target: q.Target})
	home := out.Config.Forwards[1]
	if home.LocalPort == f.LocalPort {
		t.Fatal("accounts share local port")
	}
	forwardExchange(t, home)
	if got := <-nodes.calls; got != "home/db:5432" {
		t.Fatal(got)
	}
	mustRequest(t, a, Request{Op: "forward-remove", Name: "work", Target: q.Target})
	q.LocalPort = f.LocalPort
	out = mustRequest(t, a, q)
	if got := out.Config.Forwards[1]; got != f {
		t.Fatalf("explicit port not honored: %+v", got)
	}
	forwardExchange(t, f)
	if got := <-nodes.calls; got != "work/db:5432" {
		t.Fatal(got)
	}
	blocker, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	if _, code := request(t, a, Request{Op: "forward-add", Name: "work", Target: "other:22", LocalPort: blocker.Addr().(*net.TCPAddr).Port}); code != 400 {
		t.Fatal("external port conflict accepted")
	}
	if len(a.Config.Forwards) != 2 {
		t.Fatal("conflict changed registrations")
	}
}
