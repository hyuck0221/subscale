package daemon

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/hyuck0221/subscale/internal/config"
)

// Called with a.mu held. Persist removal progress so a restart cannot reconnect
// a partially deleted account, and a retry can use the same credentials.
func (a *API) purgeAccount(ctx context.Context, q Request, c config.Config) error {
	account := c.Accounts[q.Name]
	removal := account.Removal
	if removal == nil || (q.MachineRemoved && !removal.MachineRemoved) {
		removal = &config.DeviceRemoval{MachineRemoved: q.MachineRemoved}
	}
	if !account.Ephemeral && !removal.MachineRemoved {
		return fmt.Errorf("%q is registered as a regular Tailscale device; remove it at https://login.tailscale.com/admin/machines, then run subscale %s logout --machine-removed", q.Name, q.Name)
	}

	account.Enabled = false
	account.Removal = removal
	c.Accounts[q.Name] = account
	if err := a.clearAccountSettings(q.Name, c); err != nil {
		return err
	}
	c = a.Config
	c.Accounts = maps.Clone(c.Accounts)
	if !removal.MachineRemoved {
		if err := a.Nodes.LogoutEphemeral(ctx, q.Name, account.Storage(q.Name)); err != nil {
			// Reopen the saved profile on retry. The failed logout may have
			// discarded the control client's in-memory credentials.
			err = errors.Join(err, a.Nodes.Disconnect(q.Name))
			return fmt.Errorf("settings cleared, but Tailscale logout is pending: %w", err)
		}
		account.Removal = &config.DeviceRemoval{MachineRemoved: true}
		c.Accounts[q.Name] = account
		if err := config.Save(a.Dir, c); err != nil {
			return fmt.Errorf("device removed from Machines, but removal progress could not be saved; retry with --machine-removed: %w", err)
		}
		a.Config = c
		c.Accounts = maps.Clone(c.Accounts)
	}
	if err := a.Nodes.Disconnect(q.Name); err != nil {
		return fmt.Errorf("disconnect account %q: %w", q.Name, err)
	}
	if err := os.RemoveAll(filepath.Join(a.Dir, "accounts", account.Storage(q.Name))); err != nil {
		return fmt.Errorf("delete local authentication state: %w", err)
	}
	delete(c.Accounts, q.Name)
	if err := config.Save(a.Dir, c); err != nil {
		return fmt.Errorf("delete local account registration: %w", err)
	}
	a.Config = c
	return nil
}
