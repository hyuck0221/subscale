//go:build darwin || linux

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/daemon"
)

const help = `subscale — named secondary Tailscale connections

  subscale NAME login                 Register a name and sign in with Tailscale
  subscale NAME logout                Sign out (keep the name and routing rules)
  subscale NAME add HOST [--to HOST]   Route a hostname, IP or *.domain via NAME
  subscale NAME remove HOST           Remove a routing rule (alias: rm)
  subscale NAME list                  Show this account and its rules
  subscale status                     Show all accounts and routing rules
  subscale start | stop               Start/stop the background daemon
  subscale exec COMMAND [ARG...]      Run a program with HTTP proxy variables
  subscale connect HOST PORT          TCP stdio tunnel, e.g. SSH ProxyCommand
  subscale proxy pac                  Print the browser PAC URL
  subscale proxy on SERVICE           Enable macOS automatic proxy, e.g. Wi-Fi
  subscale proxy off SERVICE          Restore that service's previous PAC setting
  subscale service install|uninstall  Manage login startup (launchd/systemd user)
  subscale version

Examples:
  subscale work login
  subscale work add grafana.example.ts.net
  subscale work add '*.corp.example'
  subscale work add dev.example --to 100.80.1.2
  subscale exec curl https://grafana.example.ts.net
  ssh -o 'ProxyCommand=subscale connect %h %p' user@server

Only proxy-aware traffic is routed. Unmatched hosts use the existing OS connection.
State: SUBSCALE_HOME or the OS user config directory / subscale.
`

func Run(args []string, version string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		io.WriteString(os.Stdout, help)
		return nil
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Println("subscale", version)
		return nil
	}
	dir, e := config.Dir()
	if e != nil {
		return e
	}
	if args[0] == "daemon" {
		if len(args) != 1 {
			return errors.New("usage: subscale daemon")
		}
		return daemon.Run(dir)
	}
	if args[0] == "service" {
		return service(dir, args[1:])
	}
	if args[0] == "proxy" {
		return proxySettings(dir, args[1:])
	}
	if args[0] == "stop" {
		if len(args) != 1 {
			return errors.New("usage: subscale stop")
		}
		_, e = call(dir, daemon.Request{Op: "stop"})
		if e != nil {
			return fmt.Errorf("cannot stop daemon: %w", e)
		}
		fmt.Println("subscale stopped")
		return nil
	}
	q := daemon.Request{}
	switch args[0] {
	case "start", "status", "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: subscale %s", args[0])
		}
		q.Op = "status"
	case "exec":
		if len(args) < 2 {
			return errors.New("usage: subscale exec COMMAND [ARG...]")
		}
	case "connect":
		if len(args) != 3 {
			return errors.New("usage: subscale connect HOST PORT")
		}
	default:
		if e = config.Name(args[0]); e != nil {
			return e
		}
		if len(args) < 2 {
			return errors.New("expected NAME login|logout|add|remove|list")
		}
		q.Name = args[0]
		q.Op = args[1]
		switch q.Op {
		case "login", "logout", "list", "status":
			if len(args) != 2 {
				return errors.New("unexpected arguments")
			}
			if q.Op == "list" {
				q.Op = "status"
			}
		case "add":
			if len(args) != 3 && (len(args) != 5 || args[3] != "--to") {
				return errors.New("usage: subscale NAME add HOST [--to HOST]")
			}
			q.Host = args[2]
			if len(args) == 5 {
				q.Target = args[4]
			}
			if _, e = config.Host(q.Host, true); e != nil {
				return e
			}
			if q.Target != "" {
				if _, e = config.Host(q.Target, false); e != nil {
					return e
				}
			}
		case "remove", "rm":
			if len(args) != 3 {
				return errors.New("usage: subscale NAME remove HOST")
			}
			q.Op = "remove"
			q.Host = args[2]
		default:
			return fmt.Errorf("unknown account command %q", q.Op)
		}
	}
	if e = ensure(dir); e != nil {
		return e
	}
	if args[0] == "exec" {
		return execute(args[1:])
	}
	if args[0] == "connect" {
		return connect(args[1], args[2])
	}
	out, e := call(dir, q)
	if e != nil {
		return e
	}
	if q.Op == "login" {
		return waitLogin(dir, q.Name)
	}
	if q.Op == "status" {
		printStatus(out, q.Name)
	} else {
		fmt.Printf("%s: %s complete\n", q.Name, q.Op)
	}
	return nil
}
func call(dir string, q daemon.Request) (daemon.Response, error) {
	var out daemon.Response
	b, e := json.Marshal(q)
	if e != nil {
		return out, e
	}
	c := daemon.Client(dir)
	defer c.CloseIdleConnections()
	res, e := c.Post("http://subscale/api", "application/json", bytes.NewReader(b))
	if e != nil {
		return out, e
	}
	defer res.Body.Close()
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); e != nil {
		return out, e
	}
	if out.Error != "" {
		return out, errors.New(out.Error)
	}
	if res.StatusCode != http.StatusOK {
		return out, fmt.Errorf("daemon HTTP %d", res.StatusCode)
	}
	return out, nil
}
func ensure(dir string) error {
	if _, e := call(dir, daemon.Request{Op: "ping"}); e == nil {
		return nil
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(dir, "daemon.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	cmd := exec.Command(exe, "daemon")
	cmd.Env = append(os.Environ(), "SUBSCALE_HOME="+dir)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return e
	}
	go cmd.Wait()
	for i := 0; i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, e = call(dir, daemon.Request{Op: "ping"}); e == nil {
			return nil
		}
	}
	return fmt.Errorf("daemon failed to start; check %s", filepath.Join(dir, "daemon.log"))
}
func waitLogin(dir, name string) error {
	lastURL := ""
	fmt.Printf("Connecting %s…\n", name)
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		out, e := call(dir, daemon.Request{Op: "status", Name: name})
		if e != nil {
			return e
		}
		st := out.Status[name]
		if st.State == "Running" {
			fmt.Printf("%s connected (%s). Authentication saved.\n", name, strings.Join(st.IPs, ", "))
			return nil
		}
		if st.Error != "" {
			return errors.New(st.Error)
		}
		if st.LoginURL != "" && st.LoginURL != lastURL {
			lastURL = st.LoginURL
			fmt.Printf("Sign in to the Tailscale account for %s:\n%s\n", name, lastURL)
			openBrowser(lastURL)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("login timed out; run subscale %s login again (the daemon remains running)", name)
}
func openBrowser(url string) {
	if os.Getenv("SUBSCALE_NO_BROWSER") == "1" {
		return
	}
	program := "xdg-open"
	if runtime.GOOS == "darwin" {
		program = "open"
	}
	cmd := exec.Command(program, url)
	if e := cmd.Start(); e == nil {
		go cmd.Wait()
	}
}
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
func execute(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	blocked := map[string]bool{}
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		blocked[k] = true
	}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if !blocked[k] {
			cmd.Env = append(cmd.Env, v)
		}
	}
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		cmd.Env = append(cmd.Env, k+"=http://"+config.ProxyAddress)
	}
	cmd.Env = append(cmd.Env, "NO_PROXY=", "no_proxy=")
	if e := cmd.Run(); e != nil {
		var exit *exec.ExitError
		if errors.As(e, &exit) {
			os.Exit(exit.ExitCode())
		}
		return e
	}
	return nil
}
