package tailnet

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"tailscale.com/tsnet"
)

type Status struct {
	State    string   `json:"state"`
	LoginURL string   `json:"login_url,omitempty"`
	IPs      []string `json:"ips,omitempty"`
	Error    string   `json:"error,omitempty"`
}
type Manager struct {
	mu    sync.Mutex
	dir   string
	nodes map[string]*tsnet.Server
}

func New(dir string) *Manager { return &Manager{dir: dir, nodes: map[string]*tsnet.Server{}} }
func (m *Manager) Start(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nodes[name] != nil {
		return nil
	}
	dir := filepath.Join(m.dir, "accounts", name)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	s := &tsnet.Server{Dir: dir, Hostname: "subscale-" + name, Logf: func(string, ...any) {}, UserLogf: func(f string, a ...any) { log.Printf("[%s] "+f, append([]any{name}, a...)...) }}
	if e := s.Start(); e != nil {
		s.Close()
		return e
	}
	m.nodes[name] = s
	return nil
}
func (m *Manager) node(name string) (*tsnet.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.nodes[name]
	if s == nil {
		return nil, fmt.Errorf("account %q is logged out; run subscale %s login", name, name)
	}
	return s, nil
}
func (m *Manager) Login(ctx context.Context, name string) error {
	if e := m.Start(name); e != nil {
		return e
	}
	s, e := m.node(name)
	if e != nil {
		return e
	}
	lc, e := s.LocalClient()
	if e != nil {
		return e
	}
	st, e := lc.StatusWithoutPeers(ctx)
	if e != nil {
		return e
	}
	if st.BackendState == "Running" {
		return nil
	}
	return lc.StartLoginInteractive(ctx)
}
func (m *Manager) Status(ctx context.Context, name string) Status {
	s, e := m.node(name)
	if e != nil {
		return Status{State: "LoggedOut"}
	}
	lc, e := s.LocalClient()
	if e != nil {
		return Status{State: "Error", Error: e.Error()}
	}
	st, e := lc.StatusWithoutPeers(ctx)
	if e != nil {
		return Status{State: "Error", Error: e.Error()}
	}
	out := Status{State: st.BackendState, LoginURL: st.AuthURL}
	for _, ip := range st.TailscaleIPs {
		out.IPs = append(out.IPs, ip.String())
	}
	return out
}
func (m *Manager) Logout(ctx context.Context, name string) error {
	s, e := m.node(name)
	if e != nil {
		return nil
	}
	lc, e := s.LocalClient()
	if e != nil {
		return e
	}
	if e = lc.Logout(ctx); e != nil {
		return e
	}
	m.mu.Lock()
	delete(m.nodes, name)
	m.mu.Unlock()
	return s.Close()
}
func (m *Manager) Dial(ctx context.Context, name, address string) (net.Conn, error) {
	s, e := m.node(name)
	if e != nil {
		return nil, e
	}
	return s.Dial(ctx, "tcp", address)
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for n, s := range m.nodes {
		if e := s.Close(); e != nil {
			log.Printf("close %s: %v", n, e)
		}
	}
	m.nodes = map[string]*tsnet.Server{}
}
