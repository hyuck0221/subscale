package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHost(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"HTTPS://Grafana.Example.ts.net:8443/dashboard", "grafana.example.ts.net"},
		{"*.CORP.example.", "*.corp.example"}, {"100.64.0.1:22", "100.64.0.1"}, {"[fd7a:115c:a1e0::1]:443", "fd7a:115c:a1e0::1"}, {"::ffff:100.64.0.1", "100.64.0.1"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, e := Host(tt.input, true)
			if e != nil || got != tt.want {
				t.Fatalf("Host = %q, %v; want %q", got, e, tt.want)
			}
		})
	}
	for _, bad := range []string{"", "../../etc/passwd", "example.com;alert(1)", "a..com", "foo:bad", "https://user:pass@host", "ftp://host", "*.bad*", "-bad.example", "100.64.0.0/10", "fe80::1%en0"} {
		if h, e := Host(bad, true); e == nil {
			t.Errorf("accepted %q as %q", bad, h)
		}
	}
}
func TestMatch(t *testing.T) {
	routes := []Route{{Account: "work", Host: "*.example.com"}, {Account: "lab", Host: "*.dev.example.com"}, {Account: "home", Host: "app.dev.example.com"}}
	for _, tt := range []struct{ host, account string }{{"www.example.com", "work"}, {"api.dev.example.com", "lab"}, {"APP.dev.example.com.", "home"}, {"example.com", ""}, {"evil-example.com", ""}, {"api.example.com.evil", ""}} {
		got, ok := Match(routes, tt.host)
		if ok != (tt.account != "") || got.Account != tt.account {
			t.Errorf("%s = %v, %v", tt.host, got, ok)
		}
	}
}
func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	c := Config{Accounts: map[string]Account{"work": {Enabled: true}}, Routes: []Route{{Account: "work", Host: "db.example"}}}
	if e := Save(dir, c); e != nil {
		t.Fatal(e)
	}
	loaded, e := Load(dir)
	if e != nil || len(loaded.Routes) != 1 || !loaded.Accounts["work"].Enabled {
		t.Fatalf("load = %+v %v", loaded, e)
	}
	info, e := os.Stat(filepath.Join(dir, "config.json"))
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions: %v %v", info, e)
	}
	if e := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"accounts":{"../escape":{"enabled":true}}}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Load(dir); e == nil {
		t.Fatal("accepted unsafe account path")
	}
}
func TestNames(t *testing.T) {
	for _, name := range []string{"../x", "work/home", "", "status", "daemon"} {
		if Name(name) == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if e := Name("work-2"); e != nil {
		t.Fatal(e)
	}
}

func TestStorageValidation(t *testing.T) {
	for _, accounts := range []map[string]Account{
		{"work": {StateDir: "../escape"}},
		{"work": {StateDir: "same"}, "home": {StateDir: "same"}},
	} {
		dir := t.TempDir()
		if e := Save(dir, Config{Accounts: accounts}); e != nil {
			t.Fatal(e)
		}
		if _, e := Load(dir); e == nil {
			t.Fatal("accepted unsafe or duplicate storage path")
		}
	}
	account := Account{}
	if account.Storage("work") != "work" {
		t.Fatal("legacy storage changed")
	}
	account.StateDir = "identity"
	if account.Storage("office") != "identity" {
		t.Fatal("rename changed stable storage")
	}
}
