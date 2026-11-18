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
	"strings"
	"sync"
	"testing"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

type fakeNodes struct {
	mu        sync.Mutex
	online    map[string]bool
	loginErr  error
	logoutErr error
}

func (f *fakeNodes) Start(n, storage string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online[n] = true
	return nil
}
func (f *fakeNodes) Login(_ context.Context, n, storage string, ephemeral bool) error {
	if e := f.Start(n, storage, ephemeral); e != nil {
		return e
	}
	return f.loginErr
}
func (f *fakeNodes) Rename(old, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value, ok := f.online[old]; ok {
		f.online[name] = value
		delete(f.online, old)
	}
	return nil
}
func (f *fakeNodes) LogoutEphemeral(_ context.Context, n, _ string) error {
	if f.logoutErr != nil {
		return f.logoutErr
	}
	return f.Disconnect(n)
}
func (f *fakeNodes) Disconnect(n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online[n] = false
	return nil
}
func (f *fakeNodes) Status(_ context.Context, n string) tailnet.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.online[n] {
		return tailnet.Status{State: "Running"}
	}
	return tailnet.Status{State: "LoggedOut"}
}
func (f *fakeNodes) Dial(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("unused")
}
func (f *fakeNodes) Close() {}
func (f *fakeNodes) Devices(context.Context, string) ([]tailnet.Device, error) {
	return nil, errors.New("unused")
}
func (f *fakeNodes) Resolve(context.Context, string, string) (tailnet.Resolution, error) {
	return tailnet.Resolution{}, errors.New("unused")
}
func newAPI(t *testing.T) *API {
	t.Helper()
	return &API{
		Dir: t.TempDir(), Config: config.Config{Accounts: map[string]config.Account{}},
		Nodes: &fakeNodes{online: map[string]bool{}},
	}
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
	storage := filepath.Join(a.Dir, "accounts", a.Config.Accounts["work"].StateDir)
	if err := os.MkdirAll(storage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage, "credentials"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	out = mustRequest(t, a, Request{Op: "purge", Name: "work"})
	if _, exists := out.Config.Accounts["work"]; exists || len(out.Config.Routes) != 0 {
		t.Fatal("logout kept rules or the account")
	}
	if _, err := os.Stat(storage); !os.IsNotExist(err) {
		t.Fatalf("credential directory survived logout: %v", err)
	}
	loaded, e := config.Load(a.Dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, exists := loaded.Accounts["work"]; exists || len(loaded.Routes) != 0 {
		t.Fatal("logout not persisted")
	}
	out = mustRequest(t, a, Request{Op: "status"})
	if out.Status["home"].State != "Running" || len(out.Status) != 1 {
		t.Fatal("logout affected another account")
	}
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "db.example"})
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
	if _, code := request(t, a, Request{Op: "purge", Name: "work"}); code != 400 {
		t.Fatal("logout continued after clear failed")
	}
	if len(a.Routes()) != 1 || a.Nodes.Status(context.Background(), "work").State != "Running" {
		t.Fatal("failed clear changed live routes or logged out the node")
	}
}
func TestPurgeRetriesAfterRemoteFailure(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "db.example"})
	nodes := a.Nodes.(*fakeNodes)
	nodes.logoutErr = errors.New("offline")
	q := Request{Op: "purge", Name: "work"}
	if _, code := request(t, a, q); code != 400 {
		t.Fatal("remote failure ignored")
	}
	loaded, err := config.Load(a.Dir)
	if err != nil {
		t.Fatal(err)
	}
	pending := loaded.Accounts["work"]
	if pending.Enabled || pending.Removal == nil || pending.Removal.MachineRemoved || len(loaded.Routes) != 0 {
		t.Fatalf("unexpected removal state: %+v", loaded)
	}
	a.Config = loaded
	if _, code := request(t, a, Request{Op: "login", Name: "work"}); code != 400 {
		t.Fatal("login continued during pending removal")
	}
	nodes.logoutErr = nil
	out := mustRequest(t, a, q)
	if _, exists := out.Config.Accounts["work"]; exists {
		t.Fatal("retry kept deleted account")
	}
}
func TestPurgeLegacyAccountRequiresMachinesRemoval(t *testing.T) {
	a := newAPI(t)
	a.Config.Accounts["legacy"] = config.Account{}
	mustRequest(t, a, Request{Op: "add", Name: "legacy", Host: "db.example"})

	q := Request{Op: "purge", Name: "legacy"}
	if out, code := request(t, a, q); code != 400 || !strings.Contains(out.Error, "--machine-removed") {
		t.Fatalf("expected manual Machines removal instructions: %d %s", code, out.Error)
	}
	if len(a.Routes()) != 1 || a.Config.Accounts["legacy"].Removal != nil {
		t.Fatal("legacy account changed before Machines removal")
	}
	q.MachineRemoved = true
	out := mustRequest(t, a, q)
	if len(out.Config.Accounts) != 0 || len(out.Config.Routes) != 0 {
		t.Fatal("local legacy account survived removal")
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
