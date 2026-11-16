package cli

import (
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/daemon"
)

func parseForward(args []string) (daemon.Request, error) {
	q := daemon.Request{}
	usage := fmt.Errorf("usage: subscale NAME forward add HOST:PORT [--local PORT] | list | remove HOST:PORT")
	if len(args) < 3 {
		return q, usage
	}
	q.Name = args[0]
	switch args[2] {
	case "list":
		if len(args) != 3 {
			return q, usage
		}
		q.Op = "forward-list"
		return q, nil
	case "add":
		if len(args) != 4 && (len(args) != 6 || args[4] != "--local") {
			return q, usage
		}
		q.Op = "forward-add"
		if len(args) == 6 {
			port, err := strconv.Atoi(args[5])
			if err != nil || port < 1 || port > 65535 || port == 18765 {
				return q, fmt.Errorf("--local must be a port from 1 to 65535, excluding the proxy port 18765")
			}
			q.LocalPort = port
		}
	case "remove", "rm":
		if len(args) != 4 {
			return q, usage
		}
		q.Op = "forward-remove"
	default:
		return q, usage
	}
	var err error
	q.Target, err = config.Endpoint(args[3])
	return q, err
}

func writeForwards(w io.Writer, out daemon.Response, name, target string) {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	found := false
	for _, f := range out.Config.Forwards {
		if (name != "" && f.Account != name) || (target != "" && f.Target != target) {
			continue
		}
		if !found {
			fmt.Fprintln(table, "ACCOUNT\tLOCAL\tTARGET\tSTATUS")
			found = true
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", f.Account, f.LocalAddress(), f.Target, out.ForwardStatus[f.LocalPort])
	}
	table.Flush()
	if !found {
		fmt.Fprintln(w, "No TCP forwards registered.")
	}
}
