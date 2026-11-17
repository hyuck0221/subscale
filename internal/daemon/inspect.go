package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
)

type CheckResult struct {
	Account      string        `json:"account"`
	State        string        `json:"state"`
	Input        string        `json:"input"`
	Target       string        `json:"target"`
	Rule         *config.Route `json:"rule,omitempty"`
	Resolved     string        `json:"resolved,omitempty"`
	ViaTailscale bool          `json:"via_tailscale"`
	LiteralIP    bool          `json:"literal_ip"`
	DNS          string        `json:"dns"`
	TCP          string        `json:"tcp"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
	Hint         string        `json:"hint,omitempty"`
}

func (a *API) inspect(ctx context.Context, q Request) (Response, error) {
	out := Response{}
	if err := config.Name(q.Name); err != nil {
		return out, err
	}
	if q.Op == "check" {
		target, err := config.Endpoint(q.Target)
		if err != nil {
			return out, err
		}
		q.Target = target
	}
	a.mu.Lock()
	account, exists := a.Config.Accounts[q.Name]
	routes := append([]config.Route(nil), a.Config.Routes...)
	a.mu.Unlock()
	if !exists {
		return out, fmt.Errorf("unknown account %q; run subscale %s login", q.Name, q.Name)
	}
	if q.Op == "devices" {
		if !account.Enabled {
			return out, fmt.Errorf("account %q is logged out; run subscale %s login", q.Name, q.Name)
		}
		var err error
		out.Devices, err = a.Nodes.Devices(ctx, q.Name)
		return out, err
	}
	result := &CheckResult{Account: q.Name, Input: q.Target, Target: q.Target, DNS: "skipped", TCP: "skipped"}
	out.Check = result
	host, port, _ := net.SplitHostPort(q.Target)
	if route, ok := config.Match(routes, host); ok {
		result.Rule = &route
		if route.Account == q.Name && route.Target != "" {
			host = route.Target
			result.Target = net.JoinHostPort(host, port)
		}
	}
	_, ipErr := netip.ParseAddr(host)
	result.LiteralIP = ipErr == nil
	if !account.Enabled {
		result.State = "LoggedOut"
		result.Error = "account is logged out"
		result.Hint = "Run: subscale " + q.Name + " login"
		return out, nil
	}
	status := a.Nodes.Status(ctx, q.Name)
	result.State = status.State
	if status.State != "Running" {
		result.Error = status.Error
		if result.Error == "" {
			result.Error = "account is " + status.State
		}
		result.Hint = "Check account status; run subscale " + q.Name + " login if authentication is required."
		return out, nil
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	resolved, err := a.Nodes.Resolve(resolveCtx, q.Name, result.Target)
	cancel()
	if err != nil {
		result.DNS = "failed"
		result.Error = err.Error()
		result.Hint = "Check the hostname and DNS configuration. Routing rules do not create DNS records; use --to for aliases."
		return out, nil
	}
	result.Resolved, result.ViaTailscale = resolved.Address, resolved.ViaTailscale
	result.DNS = "ok"
	// Pin the resolved address so DNS cannot change between these two stages.
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := a.Dial(dialCtx, q.Name, resolved.Address)
	cancel()
	if err != nil {
		result.TCP = "failed"
		result.Error = err.Error()
		result.Hint = "Check the destination service, firewall, tailnet access rules and subnet routes."
		var netErr net.Error
		if errors.Is(err, syscall.ECONNREFUSED) {
			result.Hint = "Connection refused. Check that the destination service is listening on this port."
		} else if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			result.Hint = "Connection timed out. Check device connectivity, firewalls, tailnet access rules and subnet routes."
		}
		return out, nil
	}
	conn.Close()
	result.TCP, result.Success = "ok", true
	return out, nil
}
