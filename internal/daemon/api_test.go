package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

type fakeNodes struct {
	mu        sync.Mutex
	online    map[string]bool
	logoutErr error
}

func (f *fakeNodes) Start(n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online[n] = true
	return nil
}
func (f *fakeNodes) Login(_ context.Context, n string) error { return f.Start(n) }
func (f *fakeNodes) Logout(_ context.Context, n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logoutErr != nil {
		return f.logoutErr
	}
	f.online[n] = false
	return nil
}
func (f *fakeNodes) Status(_ context.Context, n string) tailnet.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := "LoggedOut"
	if f.online[n] {
		s = "Running"
	}
	return tailnet.Status{State: s}
}
func (f *fakeNodes) Dial(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("unused")
}
func (f *fakeNodes) Close() {}
func newAPI(t *testing.T) *API {
	t.Helper()
	return &API{Dir: t.TempDir(), Config: config.Config{Accounts: map[string]config.Account{}}, Nodes: &fakeNodes{online: map[string]bool{}}}
}
func request(t *testing.T, a *API, q Request) (Response, int) {
	t.Helper()
	b, e := json.Marshal(q)
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest("POST", "/api", bytes.NewReader(b)))
	var out Response
	if e = json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	return out, w.Code
}
func mustRequest(t *testing.T, a *API, q Request) Response {
	t.Helper()
	out, code := request(t, a, q)
	if code != 200 {
		t.Fatalf("%+v = %d %s", q, code, out.Error)
	}
	return out
}
func TestAccountAndRouteLifecycle(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "login", Name: "home"})
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "https://DB.Example:8443/path"})
	out := mustRequest(t, a, Request{Op: "add", Name: "work", Host: "db.example", Target: "100.64.1.2"})
	if len(out.Config.Routes) != 1 || out.Config.Routes[0].Target != "100.64.1.2" {
		t.Fatal("upsert failed")
	}
	if out, code := request(t, a, Request{Op: "add", Name: "home", Host: "db.example"}); code != 400 || out.Error == "" {
		t.Fatal("conflicting account silently stole route")
	}
	out = mustRequest(t, a, Request{Op: "logout", Name: "work"})
	if out.Config.Accounts["work"].Enabled || len(out.Config.Routes) != 1 {
		t.Fatal("logout lost rules or kept account enabled")
	}
	loaded, e := config.Load(a.Dir)
	if e != nil || loaded.Accounts["work"].Enabled {
		t.Fatal("logout not persisted")
	}
	out = mustRequest(t, a, Request{Op: "status"})
	if out.Status["home"].State != "Running" || out.Status["work"].State != "LoggedOut" {
		t.Fatal("logout affected another account")
	}
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "remove", Name: "work", Host: "db.example"})
	if len(a.Routes()) != 0 {
		t.Fatal("route not removed")
	}
}
func TestFailedSaveKeepsLiveRoutes(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "db.example"})
	// Replace the config destination with a directory to force atomic rename to fail.
	if e := os.Remove(filepath.Join(a.Dir, "config.json")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(a.Dir, "config.json"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, code := request(t, a, Request{Op: "remove", Name: "work", Host: "db.example"}); code != 400 {
		t.Fatal("save unexpectedly succeeded")
	}
	if len(a.Routes()) != 1 {
		t.Fatal("failed save changed live routes")
	}
}
func TestLogoutFailureIsReported(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	a.Nodes.(*fakeNodes).logoutErr = errors.New("offline")
	if _, code := request(t, a, Request{Op: "logout", Name: "work"}); code != 400 {
		t.Fatal("logout failure ignored")
	}
	if !a.Config.Accounts["work"].Enabled {
		t.Fatal("claimed successful logout")
	}
}
func TestConcurrentReadsAndEdits(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mustRequest(t, a, Request{Op: "add", Name: "work", Host: "db.example"})
			mustRequest(t, a, Request{Op: "status"})
			a.Routes()
		}()
	}
	wg.Wait()
}
func TestInvalidRequests(t *testing.T) {
	a := newAPI(t)
	for _, q := range []Request{{Op: "login", Name: "../x"}, {Op: "add", Name: "missing", Host: "db.example"}, {Op: "unknown", Name: "work"}, {Op: "status", Name: "unknown"}} {
		if _, code := request(t, a, q); code != 400 {
			t.Errorf("accepted %+v", q)
		}
	}
}
