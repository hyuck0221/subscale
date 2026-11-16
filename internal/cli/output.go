package cli

import (
	"fmt"
	"github.com/hyuck0221/subscale/internal/daemon"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

func printStatus(out daemon.Response, only string) {
	names := []string{}
	for n := range out.Config.Accounts {
		if only == "" || n == only {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Println("No accounts. Add one: subscale work login")
		return
	}
	for _, n := range names {
		st := out.Status[n]
		fmt.Printf("%s\t%s\t%s\n", n, st.State, strings.Join(st.IPs, ", "))
		if st.Error != "" {
			fmt.Println("  error:", st.Error)
		}
		for _, r := range out.Config.Routes {
			if r.Account == n {
				fmt.Printf("  %s", r.Host)
				if r.Target != "" {
					fmt.Printf(" → %s", r.Target)
				}
				fmt.Println()
			}
		}
	}
	fmt.Printf("Proxy: http://%s\nPAC: http://%s/proxy.pac\n", out.Proxy, out.Proxy)
}

func writeList(w io.Writer, out daemon.Response) {
	names := make([]string, 0, len(out.Config.Accounts))
	for name := range out.Config.Accounts {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(w, "No logged-in accounts. Sign in: subscale work login")
		return
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tSTATUS\tIP")
	for _, name := range names {
		st := out.Status[name]
		fmt.Fprintf(table, "%s\t%s\t%s\n", name, st.State, strings.Join(st.IPs, ", "))
	}
	table.Flush()
}
