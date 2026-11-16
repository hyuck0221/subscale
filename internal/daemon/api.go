package daemon

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

type Nodes interface {
	Start(string, string) error
	Login(context.Context, string, string) error
	Rename(string, string) error
	Logout(context.Context, string) error
	Status(context.Context, string) tailnet.Status
	Dial(context.Context, string, string) (net.Conn, error)
	Close()
}
type Request struct {
	Op        string `json:"op"`
	Name      string `json:"name,omitempty"`
	NewName   string `json:"new_name,omitempty"`
	Host      string `json:"host,omitempty"`
	Target    string `json:"target,omitempty"`
	LocalPort int    `json:"local_port,omitempty"`
}
type Response struct {
	Config        config.Config             `json:"config"`
	Status        map[string]tailnet.Status `json:"status,omitempty"`
	Error         string                    `json:"error,omitempty"`
	Proxy         string                    `json:"proxy"`
	ForwardStatus map[int]string            `json:"forward_status,omitempty"`
}
type API struct {
	mu            sync.Mutex
	Dir           string
	Config        config.Config
	Nodes         Nodes
	Stop          func()
	forwards      map[int]*tcpForward
	forwardErrors map[int]string
}

func (a *API) Routes() []config.Route {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]config.Route(nil), a.Config.Routes...)
}
func (a *API) Dial(ctx context.Context, name, address string) (net.Conn, error) {
	a.mu.Lock()
	account, ok := a.Config.Accounts[name]
	a.mu.Unlock()
	if !ok || !account.Enabled {
		return nil, fmt.Errorf("account %q is logged out; run subscale %s login", name, name)
	}
	return a.Nodes.Dial(ctx, name, address)
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
	out := Response{Config: a.Config, Proxy: config.ProxyAddress, Status: map[string]tailnet.Status{}, ForwardStatus: a.forwardStatus()}
	a.mu.Unlock()
	if q.Op == "status" || q.Op == "login" || q.Op == "list" {
		for n := range out.Config.Accounts {
			if q.Name == "" || n == q.Name {
				out.Status[n] = a.Nodes.Status(ctx, n)
			}
		}
	}
	if q.Op == "list" {
		out = authenticatedAccounts(out)
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
	case "status", "list", "forward-list":
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
	c := config.Config{Accounts: map[string]config.Account{}, Routes: append([]config.Route(nil), a.Config.Routes...), Forwards: append([]config.Forward(nil), a.Config.Forwards...)}
	for n, v := range a.Config.Accounts {
		c.Accounts[n] = v
	}
	_, exists := c.Accounts[q.Name]
	if q.Op != "login" && !exists {
		return fmt.Errorf("unknown account %q; run subscale %s login", q.Name, q.Name)
	}
	if q.Op == "forward-add" || q.Op == "forward-remove" {
		return a.applyForward(q, c)
	}
	switch q.Op {
	case "login":
		account := c.Accounts[q.Name]
		if !exists {
			account.StateDir = "acct-" + rand.Text()[:20]
			// Persist the identity before starting OAuth, so a failed attempt or
			// daemon restart retries the same credentials rather than a new node.
			c.Accounts[q.Name] = account
			if e := config.Save(a.Dir, c); e != nil {
				return e
			}
			a.Config = c
			c.Accounts = maps.Clone(c.Accounts)
		}
		if e := a.Nodes.Login(ctx, q.Name, account.Storage(q.Name)); e != nil {
			return e
		}
		account.Enabled = true
		c.Accounts[q.Name] = account
	case "logout":
		if !c.Accounts[q.Name].Enabled {
			return nil
		}
		if e := a.Nodes.Logout(ctx, q.Name); e != nil {
			return e
		}
		account := c.Accounts[q.Name]
		account.Enabled = false
		c.Accounts[q.Name] = account
	case "rename":
		if e := config.Name(q.NewName); e != nil {
			return e
		}
		if q.NewName == q.Name {
			return nil
		}
		if _, ok := c.Accounts[q.NewName]; ok {
			return fmt.Errorf("account %q already exists", q.NewName)
		}
		account := c.Accounts[q.Name]
		account.StateDir = account.Storage(q.Name)
		delete(c.Accounts, q.Name)
		c.Accounts[q.NewName] = account
		for i := range c.Routes {
			if c.Routes[i].Account == q.Name {
				c.Routes[i].Account = q.NewName
			}
		}
		for i := range c.Forwards {
			if c.Forwards[i].Account == q.Name {
				c.Forwards[i].Account = q.NewName
			}
		}
		if e := a.Nodes.Rename(q.Name, q.NewName); e != nil {
			return e
		}
		if e := config.Save(a.Dir, c); e != nil {
			return errors.Join(e, a.Nodes.Rename(q.NewName, q.Name))
		}
		a.Config = c
		return nil
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
