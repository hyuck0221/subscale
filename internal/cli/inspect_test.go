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

func TestInvalidInspectArgumentsDoNotStartDaemon(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUBSCALE_HOME", dir)
	for _, args := range [][]string{{"work", "check"}, {"work", "check", "db"}, {"work", "check", "db:22", "extra"}, {"work", "check", "*.mds:22"}, {"work", "devices", "extra"}} {
		if err := Run(args, "dev"); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "daemon.sock")); !os.IsNotExist(err) {
		t.Fatal("invalid input started daemon")
	}
}

func TestDevicesOutput(t *testing.T) {
	var b bytes.Buffer
	writeDevices(&b, []tailnet.Device{
		{Name: "subscale.tail.ts.net", IPs: []string{"100.64.0.1"}, Online: true, Self: true},
		{Name: "db.tail.ts.net", IPs: []string{"100.64.0.2"}, OS: "linux"},
	})
	for _, want := range []string{"(self)", "online", "offline", "100.64.0.2", "db.tail.ts.net", "linux"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %s: %s", want, b.String())
		}
	}
	b.Reset()
	writeDevices(&b, nil)
	if !strings.Contains(b.String(), "No visible devices") {
		t.Fatal(b.String())
	}
}

func TestCheckOutputAndFailureExit(t *testing.T) {
	var b bytes.Buffer
	r := &daemon.CheckResult{Account: "work", State: "Running", Input: "db:22", Target: "db:22", Resolved: "100.64.0.2:22", DNS: "ok", TCP: "ok", Success: true}
	if err := writeCheck(&b, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "OS network (no Tailscale route)") || !strings.Contains(b.String(), "Application authentication was not tested") {
		t.Fatal(b.String())
	}
	r.Success, r.TCP, r.Error, r.Hint = false, "failed", "connection refused", "Check destination service"
	r.Rule = &config.Route{Account: "home", Host: "db", Target: "other"}
	b.Reset()
	if err := writeCheck(&b, r); err == nil {
		t.Fatal("failed check returned success")
	}
	for _, want := range []string{"ignored: different account", "connection refused", "Check destination service"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %s: %s", want, b.String())
		}
	}
	if err := writeCheck(&b, nil); err == nil {
		t.Fatal("accepted empty daemon response")
	}
}
