package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/daemon"
)

func TestForwardArguments(t *testing.T) {
	q, err := parseForward([]string{"mds", "forward", "add", "100.89.207.112:5900", "--local", "15900"})
	if err != nil || q.Op != "forward-add" || q.Name != "mds" || q.LocalPort != 15900 || q.Target != "100.89.207.112:5900" {
		t.Fatalf("%+v %v", q, err)
	}
	q, err = parseForward([]string{"mds", "forward", "add", "[::1]:22"})
	if err != nil || q.LocalPort != 0 || q.Target != "[::1]:22" {
		t.Fatalf("%+v %v", q, err)
	}
	for _, args := range [][]string{
		{"mds", "forward"}, {"mds", "forward", "add"}, {"mds", "forward", "add", "host"},
		{"mds", "forward", "add", "host:22", "--local", "0"},
		{"mds", "forward", "add", "host:22", "--local", "65536"},
		{"mds", "forward", "add", "host:22", "--local", "18765"},
		{"mds", "forward", "add", "host:22", "--local", "no"},
		{"mds", "forward", "add", "host:22", "--bind", "15900"},
		{"mds", "forward", "remove"}, {"mds", "forward", "list", "extra"},
	} {
		if err := Run(args, "dev"); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestForwardOutput(t *testing.T) {
	out := daemon.Response{Config: config.Config{Forwards: []config.Forward{
		{Account: "mds", Target: "100.89.207.112:5900", LocalPort: 15900},
		{Account: "other", Target: "db:5432", LocalPort: 15432},
	}}, ForwardStatus: map[int]string{15900: "listening"}}
	var b bytes.Buffer
	writeForwards(&b, out, "mds", "")
	if s := b.String(); !strings.Contains(s, "127.0.0.1:15900") || !strings.Contains(s, "100.89.207.112:5900") || !strings.Contains(s, "listening") || strings.Contains(s, "db:5432") {
		t.Fatal(s)
	}
}
