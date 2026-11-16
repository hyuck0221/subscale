package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

func TestListOnlyAuthenticatedAccounts(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "login", Name: "home"})
	mustRequest(t, a, Request{Op: "add", Name: "home", Host: "nas.example"})
	mustRequest(t, a, Request{Op: "logout", Name: "home"})
	out := mustRequest(t, a, Request{Op: "list"})
	if len(out.Config.Accounts) != 1 || out.Status["work"].State != "Running" || len(out.Config.Routes) != 0 {
		t.Fatalf("list: %+v", out)
	}
	all := mustRequest(t, a, Request{Op: "status"})
	if len(all.Config.Accounts) != 2 {
		t.Fatal("status lost logged-out accounts")
	}
	out = authenticatedAccounts(Response{Config: config.Config{Accounts: map[string]config.Account{"expired": {Enabled: true}, "pending": {Enabled: true}}}, Status: map[string]tailnet.Status{"expired": {State: "NeedsLogin"}, "pending": {State: "NeedsMachineAuth"}}})
	if len(out.Config.Accounts) != 1 || out.Status["pending"].State != "NeedsMachineAuth" {
		t.Fatal("list included an unauthenticated account")
	}
}
func TestRenameKeepsStateAndRoutes(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "db.example"})
	storage := a.Config.Accounts["work"].StateDir
	out := mustRequest(t, a, Request{Op: "rename", Name: "work", NewName: "office"})
	if _, ok := out.Config.Accounts["work"]; ok {
		t.Fatal("old name remains")
	}
	if out.Config.Accounts["office"].StateDir != storage || !out.Config.Accounts["office"].Enabled || out.Config.Routes[0].Account != "office" {
		t.Fatal("rename changed credentials or lost rules")
	}
	if a.Nodes.Status(context.Background(), "office").State != "Running" {
		t.Fatal("renamed node disconnected")
	}
	loaded, e := config.Load(a.Dir)
	if e != nil || loaded.Accounts["office"].StateDir != storage {
		t.Fatalf("rename not persisted: %+v %v", loaded, e)
	}
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	if a.Config.Accounts["work"].StateDir == storage {
		t.Fatal("reused name inherited the renamed account's credentials")
	}
	mustRequest(t, a, Request{Op: "logout", Name: "office"})
	if a.Config.Accounts["office"].StateDir != storage {
		t.Fatal("logout discarded identity")
	}
}
func TestRenameLegacyStorageAndFailureRollback(t *testing.T) {
	a := newAPI(t)
	a.Config.Accounts["work"] = config.Account{Enabled: true}
	a.Nodes.Start("work", "work")
	mustRequest(t, a, Request{Op: "add", Name: "work", Host: "db.example"})
	mustRequest(t, a, Request{Op: "rename", Name: "work", NewName: "office"})
	if a.Config.Accounts["office"].StateDir != "work" {
		t.Fatal("legacy credential directory lost")
	}
	if e := os.Remove(filepath.Join(a.Dir, "config.json")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(a.Dir, "config.json"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, code := request(t, a, Request{Op: "rename", Name: "office", NewName: "company"}); code != 400 {
		t.Fatal("rename save unexpectedly succeeded")
	}
	if a.Config.Routes[0].Account != "office" || a.Nodes.Status(context.Background(), "office").State != "Running" || a.Nodes.Status(context.Background(), "company").State != "LoggedOut" {
		t.Fatal("failed rename did not restore live identity and rules")
	}
}
func TestRenameRejectsConflictsAndReservedNames(t *testing.T) {
	a := newAPI(t)
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	mustRequest(t, a, Request{Op: "login", Name: "home"})
	for _, name := range []string{"home", "status", "update", "upgrade", "rename", "../x", ""} {
		if _, code := request(t, a, Request{Op: "rename", Name: "work", NewName: name}); code != 400 {
			t.Errorf("accepted rename to %q", name)
		}
	}
	for _, name := range []string{"status", "list", "update", "upgrade", "login", "rename"} {
		if _, code := request(t, a, Request{Op: "login", Name: name}); code != 400 {
			t.Errorf("accepted reserved login %q", name)
		}
	}
	mustRequest(t, a, Request{Op: "rename", Name: "work", NewName: "work"})
}

func TestFailedLoginRetainsIdentityForRetry(t *testing.T) {
	a := newAPI(t)
	nodes := a.Nodes.(*fakeNodes)
	nodes.loginErr = context.DeadlineExceeded
	if _, code := request(t, a, Request{Op: "login", Name: "work"}); code != 400 {
		t.Fatal("login unexpectedly succeeded")
	}
	first := a.Config.Accounts["work"]
	if first.StateDir == "" || first.Enabled {
		t.Fatal("pending identity not saved")
	}
	loaded, e := config.Load(a.Dir)
	if e != nil || loaded.Accounts["work"].StateDir != first.StateDir {
		t.Fatal("failed login lost persistent identity")
	}
	if _, e := a.Dial(context.Background(), "work", "db.example:443"); e == nil {
		t.Fatal("failed login allowed routing")
	}
	nodes.loginErr = nil
	mustRequest(t, a, Request{Op: "login", Name: "work"})
	if a.Config.Accounts["work"].StateDir != first.StateDir || !a.Config.Accounts["work"].Enabled {
		t.Fatal("retry replaced identity")
	}
}

type failSaveAfterLogin struct {
	Nodes
	dir string
}

func (n failSaveAfterLogin) Login(ctx context.Context, name, storage string) error {
	if e := n.Nodes.Login(ctx, name, storage); e != nil {
		return e
	}
	if e := os.Remove(filepath.Join(n.dir, "config.json")); e != nil {
		return e
	}
	return os.Mkdir(filepath.Join(n.dir, "config.json"), 0700)
}
func TestFailedActivationSaveCannotEnableAccount(t *testing.T) {
	a := newAPI(t)
	a.Nodes = failSaveAfterLogin{Nodes: a.Nodes, dir: a.Dir}
	if _, code := request(t, a, Request{Op: "login", Name: "work"}); code != 400 {
		t.Fatal("activation save unexpectedly succeeded")
	}
	if a.Config.Accounts["work"].Enabled {
		t.Fatal("failed persistence enabled live routing")
	}
}
