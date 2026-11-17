package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/hyuck0221/subscale/internal/daemon"
	"github.com/hyuck0221/subscale/internal/tailnet"
)

func writeDevices(w io.Writer, devices []tailnet.Device) {
	if len(devices) == 0 {
		fmt.Fprintln(w, "No visible devices.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tIP\tSTATUS\tOS")
	for _, device := range devices {
		name := device.Name
		if device.Self {
			name += " (self)"
		}
		status := "offline"
		if device.Online {
			status = "online"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", name, strings.Join(device.IPs, ", "), status, device.OS)
	}
	table.Flush()
}

func writeCheck(w io.Writer, result *daemon.CheckResult) error {
	if result == nil {
		return errors.New("daemon returned no check result; update and restart the daemon")
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(table, "Account\t%s (%s)\n", result.Account, result.State)
	fmt.Fprintf(table, "Input\t%s\n", result.Input)
	if result.Rule == nil {
		fmt.Fprintln(table, "Rule\texplicit account; no matching rule")
	} else {
		rule := result.Rule.Host
		if result.Rule.Target != "" {
			rule += " -> " + result.Rule.Target
		}
		rule += " (" + result.Rule.Account + ")"
		if result.Rule.Account != result.Account {
			rule += "; ignored: different account"
		}
		fmt.Fprintf(table, "Rule\t%s\n", rule)
	}
	fmt.Fprintf(table, "Target\t%s\n", result.Target)
	dns := result.DNS
	if result.LiteralIP && result.DNS == "ok" {
		dns = "not needed (IP address)"
	}
	if result.Resolved != "" {
		dns += " -> " + result.Resolved
	}
	fmt.Fprintf(table, "DNS\t%s\n", dns)
	if result.Resolved != "" {
		path := "OS network (no Tailscale route)"
		if result.ViaTailscale {
			path = "Tailscale"
		}
		fmt.Fprintf(table, "Network\t%s\n", path)
	}
	fmt.Fprintf(table, "TCP\t%s\n", result.TCP)
	table.Flush()
	if result.Error != "" {
		fmt.Fprintln(w, "Error:", result.Error)
	}
	if result.Hint != "" {
		fmt.Fprintln(w, "Hint:", result.Hint)
	}
	if !result.Success {
		return errors.New("connection check failed")
	}
	fmt.Fprintln(w, "TCP connection succeeded. Application authentication was not tested.")
	return nil
}
