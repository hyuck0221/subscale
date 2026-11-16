package tailnet

import (
	"tailscale.com/tsnet"
	"testing"
)

func TestRenamePreservesLiveIdentity(t *testing.T) {
	m := New(t.TempDir())
	node := &tsnet.Server{Dir: "saved-credentials", Hostname: "subscale-work"}
	m.nodes["work"] = node
	if e := m.Rename("work", "office"); e != nil {
		t.Fatal(e)
	}
	if m.nodes["work"] != nil || m.nodes["office"] != node || node.Dir != "saved-credentials" {
		t.Fatal("rename replaced the node or credential path")
	}
	if _, e := m.node("work"); e == nil {
		t.Fatal("old name still connects")
	}
	m.nodes["home"] = &tsnet.Server{}
	if e := m.Rename("office", "home"); e == nil {
		t.Fatal("rename overwrote an existing node")
	}
	if m.nodes["office"] != node {
		t.Fatal("failed rename lost node")
	}
}
func TestRenameLoggedOutAccount(t *testing.T) {
	m := New(t.TempDir())
	if e := m.Rename("work", "office"); e != nil {
		t.Fatal(e)
	}
	if len(m.nodes) != 0 {
		t.Fatal("rename started a logged-out node")
	}
}
