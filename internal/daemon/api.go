package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

type Nodes interface {
	Start(string) error
	Login(context.Context, string) error
	Logout(context.Context, string) error
	Status(context.Context, string) tailnet.Status
	Dial(context.Context, string, string) (net.Conn, error)
	Close()
}
type Request struct {
	Op     string `json:"op"`
	Name   string `json:"name,omitempty"`
	Host   string `json:"host,omitempty"`
	Target string `json:"target,omitempty"`
}
type Response struct {
	Config config.Config             `json:"config"`
	Status map[string]tailnet.Status `json:"status,omitempty"`
	Error  string                    `json:"error,omitempty"`
	Proxy  string                    `json:"proxy"`
}
type API struct {
	mu     sync.Mutex
	Dir    string
	Config config.Config
	Nodes  Nodes
	Stop   func()
}

func (a *API) Routes() []config.Route {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]config.Route(nil), a.Config.Routes...)
}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" || r.URL.Path != "/api" {
		http.Error(w, "not found", 404)
		return
	}
	var q Request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&q); e != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(Response{Error: e.Error()})
		return
	}
	a.mu.Lock()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if e := a.apply(ctx, q); e != nil {
		a.mu.Unlock()
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(Response{Error: e.Error()})
		return
	}
	out := Response{Config: a.Config, Proxy: config.ProxyAddress, Status: map[string]tailnet.Status{}}
	a.mu.Unlock()
	if q.Op == "status" || q.Op == "login" {
		for n := range out.Config.Accounts {
			if q.Name == "" || n == q.Name {
				out.Status[n] = a.Nodes.Status(ctx, n)
			}
		}
	}
	json.NewEncoder(w).Encode(out)
	if q.Op == "stop" && a.Stop != nil {
		go a.Stop()
	}
}
func (a *API) apply(ctx context.Context, q Request) error {
	switch q.Op {
	case "ping", "stop":
		return nil
	case "status":
		if q.Name != "" {
			if _, ok := a.Config.Accounts[q.Name]; !ok {
				return fmt.Errorf("unknown account %q", q.Name)
			}
		}
		return nil
	}
	if e := config.Name(q.Name); e != nil {
		return e
	}
	// Clone before editing, so a failed save cannot change live routing.
	c := config.Config{Accounts: map[string]config.Account{}, Routes: append([]config.Route(nil), a.Config.Routes...)}
	for n, v := range a.Config.Accounts {
		c.Accounts[n] = v
	}
	_, exists := c.Accounts[q.Name]
	if q.Op != "login" && !exists {
		return fmt.Errorf("unknown account %q; run subscale %s login", q.Name, q.Name)
	}
	switch q.Op {
	case "login":
		if e := a.Nodes.Login(ctx, q.Name); e != nil {
			return e
		}
		c.Accounts[q.Name] = config.Account{Enabled: true}
	case "logout":
		if !c.Accounts[q.Name].Enabled {
			return nil
		}
		if e := a.Nodes.Logout(ctx, q.Name); e != nil {
			return e
		}
		c.Accounts[q.Name] = config.Account{Enabled: false}
	case "add", "remove":
		h, e := config.Host(q.Host, true)
		if e != nil {
			return e
		}
		target := ""
		if q.Target != "" {
			target, e = config.Host(q.Target, false)
			if e != nil {
				return e
			}
		}
		index := -1
		for i, r := range c.Routes {
			if r.Host == h {
				if r.Account != q.Name {
					return fmt.Errorf("%s belongs to account %s; remove its rule first", h, r.Account)
				}
				index = i
			}
		}
		if q.Op == "add" {
			r := config.Route{Account: q.Name, Host: h, Target: target}
			if index >= 0 {
				c.Routes[index] = r
			} else {
				c.Routes = append(c.Routes, r)
			}
		} else {
			if index < 0 {
				return fmt.Errorf("route %q not found", h)
			}
			c.Routes = append(c.Routes[:index], c.Routes[index+1:]...)
		}
	default:
		return fmt.Errorf("unknown operation %q", q.Op)
	}
	if e := config.Save(a.Dir, c); e != nil {
		return e
	}
	a.Config = c
	return nil
}
func Client(dir string) *http.Client {
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "daemon.sock"))
	}}}
}
