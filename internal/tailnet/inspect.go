package tailnet

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"tailscale.com/ipn/ipnstate"
)

type Device struct {
	Name   string   `json:"name"`
	OS     string   `json:"os,omitempty"`
	IPs    []string `json:"ips"`
	Online bool     `json:"online"`
	Self   bool     `json:"self,omitempty"`
}

type Resolution struct {
	Address      string `json:"address"`
	ViaTailscale bool   `json:"via_tailscale"`
}

func (m *Manager) Devices(ctx context.Context, name string) ([]Device, error) {
	s, err := m.node(name)
	if err != nil {
		return nil, err
	}
	lc, err := s.LocalClient()
	if err != nil {
		return nil, err
	}
	status, err := lc.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status.BackendState != "Running" {
		return nil, fmt.Errorf("account %q is %s; run subscale %s login", name, status.BackendState, name)
	}
	return devicesFromStatus(status), nil
}

func devicesFromStatus(status *ipnstate.Status) []Device {
	devices := []Device{}
	add := func(peer *ipnstate.PeerStatus, self bool) {
		if peer == nil {
			return
		}
		name := strings.TrimSuffix(peer.DNSName, ".")
		if name == "" {
			name = peer.HostName
		}
		device := Device{Name: name, OS: peer.OS, Online: peer.Online, Self: self, IPs: []string{}}
		for _, ip := range peer.TailscaleIPs {
			device.IPs = append(device.IPs, ip.String())
		}
		devices = append(devices, device)
	}
	add(status.Self, true)
	for _, peer := range status.Peer {
		add(peer, false)
	}
	slices.SortFunc(devices, func(a, b Device) int {
		if a.Self != b.Self {
			if a.Self {
				return -1
			}
			return 1
		}
		if order := strings.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(strings.Join(a.IPs, ","), strings.Join(b.IPs, ","))
	})
	return devices
}

// Resolve uses the same dial planner as tsnet.Dial, including its MagicDNS
// handling and network fallback. Keep this unstable subsystem API isolated here.
func (m *Manager) Resolve(ctx context.Context, name, address string) (Resolution, error) {
	s, err := m.node(name)
	if err != nil {
		return Resolution{}, err
	}
	if _, err := s.LocalClient(); err != nil {
		return Resolution{}, err
	}
	dialer, ok := s.Sys().Dialer.GetOK()
	if !ok {
		return Resolution{}, fmt.Errorf("account %q dialer is not ready", name)
	}
	ip, viaTailscale, err := dialer.UserDialPlan(ctx, "tcp", address)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Address: ip.String(), ViaTailscale: viaTailscale}, nil
}
