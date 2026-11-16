package daemon

import (
	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

func authenticatedAccounts(out Response) Response {
	filtered := Response{Proxy: out.Proxy, Config: config.Config{Accounts: map[string]config.Account{}, Routes: []config.Route{}}, Status: map[string]tailnet.Status{}}
	for name, account := range out.Config.Accounts {
		if !account.Enabled {
			continue
		}
		status := out.Status[name]
		switch status.State {
		case "Running", "Starting", "Stopped", "NeedsMachineAuth":
			filtered.Config.Accounts[name] = account
			filtered.Status[name] = status
		}
	}
	for _, route := range out.Config.Routes {
		if _, ok := filtered.Config.Accounts[route.Account]; ok {
			filtered.Config.Routes = append(filtered.Config.Routes, route)
		}
	}
	return filtered
}
