package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/daemon"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

func TestReservedNamesRejectedBeforeStartingDaemon(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUBSCALE_HOME", dir)
	for _, name := range []string{"help", "version", "daemon", "start", "stop", "status", "list", "proxy", "exec", "connect", "forward", "service", "update", "upgrade", "login", "logout", "add", "remove", "rm", "rename", "install", "uninstall", "on", "off", "pac", "STATUS"} {
		e := Run([]string{name, "login"}, "dev")
		if e == nil || !strings.Contains(e.Error(), "reserved account name") {
			t.Errorf("%s login: %v", name, e)
		}
	}
	if _, e := os.Stat(filepath.Join(dir, "daemon.sock")); !os.IsNotExist(e) {
		t.Fatal("invalid login started daemon")
	}
}
func TestInvalidRenameAndUpdateArguments(t *testing.T) {
	for _, args := range [][]string{{"work", "rename"}, {"work", "rename", "status"}, {"work", "rename", "../escape"}, {"update", "extra"}, {"upgrade", "extra"}} {
		if e := Run(args, "dev"); e == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
func TestListOutput(t *testing.T) {
	out := daemon.Response{Config: config.Config{Accounts: map[string]config.Account{"work": {}, "home": {}}}, Status: map[string]tailnet.Status{"work": {State: "Running", IPs: []string{"100.64.1.2"}}, "home": {State: "Starting"}}}
	var b bytes.Buffer
	writeList(&b, out)
	s := b.String()
	if !strings.Contains(s, "100.64.1.2") || strings.Index(s, "home") > strings.Index(s, "work") {
		t.Fatal(s)
	}
	b.Reset()
	writeList(&b, daemon.Response{})
	if !strings.Contains(b.String(), "No logged-in accounts") {
		t.Fatal(b.String())
	}
}
