package tailnet

import (
	"context"
	"net/netip"
	"testing"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

func TestDevicesFromStatus(t *testing.T) {
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{HostName: "subscale-work", DNSName: "subscale-work.tail.ts.net.", Online: true, TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")}},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			{}:                     {HostName: "z-machine", DNSName: "z.tail.ts.net.", OS: "macOS", Online: false, TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.2"), netip.MustParseAddr("fd7a:115c:a1e0::2")}},
			key.NewNode().Public(): {HostName: "a-machine", Online: true},
			key.NewNode().Public(): nil,
		},
	}
	devices := devicesFromStatus(status)
	if len(devices) != 3 || !devices[0].Self || devices[0].Name != "subscale-work.tail.ts.net" {
		t.Fatalf("%+v", devices)
	}
	if devices[1].Name != "a-machine" || !devices[1].Online {
		t.Fatalf("fallback name or sorting: %+v", devices)
	}
	if devices[2].Name != "z.tail.ts.net" || devices[2].Online || len(devices[2].IPs) != 2 || devices[2].OS != "macOS" {
		t.Fatalf("%+v", devices[2])
	}
	if len(devicesFromStatus(&ipnstate.Status{})) != 0 {
		t.Fatal("unexpected devices")
	}
}

func TestInspectLoggedOutNode(t *testing.T) {
	m := New(t.TempDir())
	if _, err := m.Devices(context.Background(), "work"); err == nil {
		t.Fatal("listed a logged-out node")
	}
	if _, err := m.Resolve(context.Background(), "work", "db:22"); err == nil {
		t.Fatal("resolved on a logged-out node")
	}
}
