package daemon

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hyuck0221/subscale/internal/tailnet"
)

type inspectNodes struct {
	Nodes
	resolve func(context.Context, string, string) (tailnet.Resolution, error)
	dial    func(context.Context, string, string) (net.Conn, error)
	devices func(context.Context, string) ([]tailnet.Device, error)
}

func (n *inspectNodes) Resolve(ctx context.Context, name, address string) (tailnet.Resolution, error) {
	return n.resolve(ctx, name, address)
}
func (n *inspectNodes) Dial(ctx context.Context, name, address string) (net.Conn, error) {
	return n.dial(ctx, name, address)
}
func (n *inspectNodes) Devices(ctx context.Context, name string) ([]tailnet.Device, error) {
	return n.devices(ctx, name)
}

func inspectAPI(t *testing.T) (*API, *inspectNodes) {
	t.Helper()
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	n := &inspectNodes{Nodes: a.Nodes,
		resolve: func(context.Context, string, string) (tailnet.Resolution, error) {
			t.Error("unexpected resolution")
			return tailnet.Resolution{}, errors.New("unexpected")
		},
		dial: func(context.Context, string, string) (net.Conn, error) {
			t.Error("unexpected dial")
			return nil, errors.New("unexpected")
		},
		devices: func(context.Context, string) ([]tailnet.Device, error) {
			t.Error("unexpected device lookup")
			return nil, errors.New("unexpected")
		},
	}
	a.Nodes = n
	return a, n
}

func TestCheckAliasResolutionAndPinnedDial(t *testing.T) {
	a, nodes := inspectAPI(t)
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "*.mds", Target: "mds"})
	original, err := os.ReadFile(filepath.Join(a.Dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	nodes.resolve = func(ctx context.Context, name, address string) (tailnet.Resolution, error) {
		if name != "work" || address != "mds:5900" {
			t.Fatalf("%s %s", name, address)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("no DNS timeout")
		}
		return tailnet.Resolution{Address: "100.64.0.2:5900", ViaTailscale: true}, nil
	}
	client, server := net.Pipe()
	defer server.Close()
	nodes.dial = func(ctx context.Context, name, address string) (net.Conn, error) {
		if name != "work" || address != "100.64.0.2:5900" {
			t.Fatalf("unpinned dial: %s %s", name, address)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("no TCP timeout")
		}
		return client, nil
	}
	r := mustRequest(t, a, Request{Op: "check", Name: "work", Target: "vis.mds:5900"}).Check
	if r == nil || !r.Success || r.DNS != "ok" || r.TCP != "ok" || r.Rule.Host != "*.mds" || !r.ViaTailscale {
		t.Fatalf("%+v", r)
	}
	server.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := server.Read(make([]byte, 1)); err == nil {
		t.Fatal("probe connection leaked")
	}
	current, err := os.ReadFile(filepath.Join(a.Dir, "config.json"))
	if err != nil || !bytes.Equal(original, current) {
		t.Fatal("check changed configuration")
	}
}

func TestCheckDoesNotApplyOtherAccountsAlias(t *testing.T) {
	a, nodes := inspectAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "home"})
	mustRequest(t, a, Request{Op: "add", Name: "home", Host: "db", Target: "other-db"})
	nodes.resolve = func(_ context.Context, name, address string) (tailnet.Resolution, error) {
		if name != "work" || address != "db:5432" {
			t.Fatalf("cross-account routing: %s %s", name, address)
		}
		return tailnet.Resolution{}, &net.DNSError{Err: "no such host", Name: "db", IsNotFound: true}
	}
	r := mustRequest(t, a, Request{Op: "check", Name: "work", Target: "db:5432"}).Check
	if r.Success || r.DNS != "failed" || r.TCP != "skipped" || r.Target != "db:5432" || r.Rule.Account != "home" {
		t.Fatalf("%+v", r)
	}
}

func TestCheckTCPFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		hint string
	}{
		{"refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, "Connection refused"},
		{"timeout", context.DeadlineExceeded, "Connection timed out"},
		{"other", errors.New("network unreachable"), "subnet routes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, nodes := inspectAPI(t)
			nodes.resolve = func(context.Context, string, string) (tailnet.Resolution, error) {
				return tailnet.Resolution{Address: "100.64.0.2:22", ViaTailscale: true}, nil
			}
			nodes.dial = func(context.Context, string, string) (net.Conn, error) { return nil, tc.err }
			r := mustRequest(t, a, Request{Op: "check", Name: "work", Target: "100.64.0.2:22"}).Check
			if r.Success || !r.LiteralIP || r.DNS != "ok" || r.TCP != "failed" || !strings.Contains(r.Hint, tc.hint) {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestInspectDisabledAndInvalidAccounts(t *testing.T) {
	a, _ := inspectAPI(t)
	for _, q := range []Request{
		{Op: "check", Name: "work", Target: "bad"},
		{Op: "check", Name: "missing", Target: "host:22"},
		{Op: "devices", Name: "missing"}, {Op: "devices"},
	} {
		if _, code := request(t, a, q); code != 400 {
			t.Fatalf("accepted %+v", q)
		}
	}
	account := a.Config.Accounts["work"]
	account.Enabled = false
	a.Config.Accounts["work"] = account
	r := mustRequest(t, a, Request{Op: "check", Name: "work", Target: "host:22"}).Check
	if r.Success || r.State != "LoggedOut" || r.DNS != "skipped" || r.TCP != "skipped" {
		t.Fatalf("%+v", r)
	}
	if _, code := request(t, a, Request{Op: "devices", Name: "work"}); code != 400 {
		t.Fatal("listed disabled account")
	}
}

func TestDevicesAreScopedAndReadOnly(t *testing.T) {
	a, nodes := inspectAPI(t)
	nodes.devices = func(_ context.Context, name string) ([]tailnet.Device, error) {
		if name != "work" {
			t.Fatal("wrong account")
		}
		return []tailnet.Device{{Name: "db.tail.ts.net", IPs: []string{"100.64.0.2"}, Online: true}}, nil
	}
	out := mustRequest(t, a, Request{Op: "devices", Name: "work"})
	if len(out.Devices) != 1 || out.Devices[0].Name != "db.tail.ts.net" {
		t.Fatalf("%+v", out)
	}
	if len(a.Config.Routes) != 0 {
		t.Fatal("devices added routes")
	}
	nodes.devices = func(context.Context, string) ([]tailnet.Device, error) { return nil, errors.New("node not ready") }
	out, code := request(t, a, Request{Op: "devices", Name: "work"})
	if code != 400 || out.Error != "node not ready" {
		t.Fatalf("%d %+v", code, out)
	}
}

func TestCheckCancellationAndManagementLock(t *testing.T) {
	a, nodes := inspectAPI(t)
	entered := make(chan struct{})
	nodes.resolve = func(ctx context.Context, _, _ string) (tailnet.Resolution, error) {
		close(entered)
		<-ctx.Done()
		return tailnet.Resolution{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Response, 1)
	go func() {
		out, _ := a.inspect(ctx, Request{Op: "check", Name: "work", Target: "db:22"})
		done <- out
	}()
	<-entered
	pinged := make(chan struct{})
	go func() { mustRequest(t, a, Request{Op: "ping"}); close(pinged) }()
	select {
	case <-pinged:
	case <-time.After(time.Second):
		t.Fatal("DNS blocked management API")
	}
	cancel()
	select {
	case out := <-done:
		if out.Check.DNS != "failed" || out.Check.TCP != "skipped" {
			t.Fatalf("%+v", out.Check)
		}
	case <-time.After(time.Second):
		t.Fatal("check ignored cancellation")
	}
}
