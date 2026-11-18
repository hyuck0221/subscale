//go:build darwin || linux

package cli

import (
	"bufio"
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
	"strings"
	"syscall"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/daemon"
)

const help = `subscale — named secondary Tailscale connections

  subscale NAME login                 Register a name and sign in with Tailscale
  subscale NAME clear                 Confirm and clear local rules and TCP forwards
  subscale NAME logout [--machine-removed]
                                      Confirm and delete the account and its Tailscale device
                                      For older accounts, remove the device in Machines first,
                                      then use --machine-removed
  subscale NAME add HOST [--to HOST]   Route a hostname, IP or *.domain via NAME
  subscale NAME remove HOST           Remove a routing rule (alias: rm)
  subscale NAME list                  Show this account and its rules
  subscale NAME rename NEW_NAME       Rename an account, keeping login and rules
  subscale NAME devices               List visible tailnet devices
  subscale NAME check HOST:PORT       Diagnose account, DNS and TCP connectivity
  subscale NAME forward add HOST:PORT [--local PORT]
                                      Save a TCP forward (automatic local port)
  subscale NAME forward list          Show saved forwards and local addresses
  subscale NAME forward remove HOST:PORT
                                      Remove a forward and close its connections
  subscale list                       List authenticated accounts
  subscale update | upgrade           Install the latest GitHub release
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

Proxy-aware traffic and registered local TCP forwards are routed.
Unmatched proxy hosts use the existing OS connection.
State: SUBSCALE_HOME or the OS user config directory / subscale.
`

func Run(args []string, version string) error {
	if len(args) >= 2 && args[1] == "login" {
		if e := config.Name(args[0]); e != nil {
			return e
		}
	}
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
	if args[0] == "update" || args[0] == "upgrade" {
		if len(args) != 1 {
			return fmt.Errorf("usage: subscale %s", args[0])
		}
		return updateBinary(dir, version)
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
		if args[0] == "list" {
			q.Op = "list"
		}
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
			return errors.New("expected NAME login|clear|logout|add|remove|list|rename|forward|devices|check")
		}
		q.Name = args[0]
		q.Op = args[1]
		switch q.Op {
		case "forward":
			q, e = parseForward(args)
			if e != nil {
				return e
			}
		case "check":
			if len(args) != 3 {
				return errors.New("usage: subscale NAME check HOST:PORT")
			}
			q.Target, e = config.Endpoint(args[2])
			if e != nil {
				return e
			}
		case "logout":
			if len(args) == 3 && args[2] == "--machine-removed" {
				q.MachineRemoved = true
			} else if len(args) != 2 {
				return errors.New("usage: subscale NAME logout [--machine-removed]")
			}
			q.Op = "purge"
		case "login", "clear", "list", "status", "devices":
			if len(args) != 2 {
				return errors.New("unexpected arguments")
			}
			if q.Op == "list" {
				q.Op = "status"
			}
		case "rename":
			if len(args) != 3 {
				return errors.New("usage: subscale NAME rename NEW_NAME")
			}
			q.NewName = args[2]
			if e = config.Name(q.NewName); e != nil {
				return e
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
	if q.Op == "clear" || q.Op == "purge" {
		switch {
		case q.Op == "clear":
			fmt.Fprintf(os.Stderr, "Delete all local routing rules and TCP forwards for %q? [y/N] ", q.Name)
		case q.MachineRemoved:
			fmt.Fprintf(os.Stderr, "Confirm that the Tailscale device for %q is already removed and permanently delete the local account? [y/N] ", q.Name)
		default:
			fmt.Fprintf(os.Stderr, "Remove the Tailscale device for %q from Machines and permanently delete the local account and its settings? [y/N] ", q.Name)
		}
		answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return nil
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
	if q.Op == "devices" {
		writeDevices(os.Stdout, out.Devices)
	} else if q.Op == "check" {
		return writeCheck(os.Stdout, out.Check)
	} else if q.Op == "forward-add" || q.Op == "forward-list" {
		writeForwards(os.Stdout, out, q.Name, q.Target)
	} else if q.Op == "forward-remove" {
		fmt.Printf("%s: removed forward %s\n", q.Name, q.Target)
	} else if q.Op == "list" {
		writeList(os.Stdout, out)
	} else if q.Op == "rename" {
		fmt.Printf("%s renamed to %s\n", q.Name, q.NewName)
	} else if q.Op == "purge" {
		fmt.Printf("%s: account removed\n", q.Name)
	} else if q.Op == "status" {
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
