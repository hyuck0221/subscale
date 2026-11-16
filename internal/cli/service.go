package cli

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/hyuck0221/subscale/internal/daemon"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func service(dir string, args []string) error {
	if len(args) != 1 || (args[0] != "install" && args[0] != "uninstall") {
		return errors.New("usage: subscale service install|uninstall")
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	run := func(name string, args ...string) error {
		b, e := exec.Command(name, args...).CombinedOutput()
		if e != nil {
			return fmt.Errorf("%s: %s: %w", name, strings.TrimSpace(string(b)), e)
		}
		return nil
	}
	if runtime.GOOS == "darwin" {
		path := filepath.Join(home, "Library", "LaunchAgents", "io.subscale.agent.plist")
		domain := "gui/" + strconv.Itoa(os.Getuid())
		label := domain + "/io.subscale.agent"
		if args[0] == "uninstall" {
			if _, e = os.Stat(path); e != nil {
				return e
			}
			if e = run("launchctl", "bootout", label); e != nil {
				return e
			}
			if e = os.Remove(path); e != nil {
				return e
			}
			fmt.Println("Login startup removed")
			return nil
		}
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		escape := func(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
		text := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>io.subscale.agent</string>
<key>ProgramArguments</key><array><string>%s</string><string>daemon</string></array>
<key>EnvironmentVariables</key><dict><key>SUBSCALE_HOME</key><string>%s</string></dict>
<key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
<key>StandardOutPath</key><string>%s</string>
<key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, escape(exe), escape(dir), escape(filepath.Join(dir, "daemon.log")), escape(filepath.Join(dir, "daemon.log")))
		if e = os.WriteFile(path, []byte(text), 0600); e != nil {
			return e
		}
		// If this is an update, unload the old service before launching its replacement.
		if exec.Command("launchctl", "print", label).Run() == nil {
			if e = run("launchctl", "bootout", label); e != nil {
				return e
			}
		}
		stopForService(dir)
		if e = run("launchctl", "bootstrap", domain, path); e != nil {
			return e
		}
	} else {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		path := filepath.Join(base, "systemd", "user", "subscale.service")
		if args[0] == "uninstall" {
			if e = run("systemctl", "--user", "disable", "--now", "subscale.service"); e != nil {
				return e
			}
			if e = os.Remove(path); e != nil {
				return e
			}
			if e = run("systemctl", "--user", "daemon-reload"); e != nil {
				return e
			}
			fmt.Println("Login startup removed")
			return nil
		}
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		quote := func(s string) string { return strconv.Quote(strings.ReplaceAll(s, "%", "%%")) }
		text := fmt.Sprintf("[Unit]\nDescription=Subscale secondary Tailscale connections\nAfter=network-online.target\n\n[Service]\nExecStart=%s daemon\nEnvironment=%s\nRestart=on-failure\nRestartSec=3\nUMask=0077\n\n[Install]\nWantedBy=default.target\n", quote(exe), quote("SUBSCALE_HOME="+dir))
		if e = os.WriteFile(path, []byte(text), 0600); e != nil {
			return e
		}
		if e = run("systemctl", "--user", "daemon-reload"); e != nil {
			return e
		}
		if e = run("systemctl", "--user", "stop", "subscale.service"); e != nil {
			return e
		}
		stopForService(dir)
		if e = run("systemctl", "--user", "enable", "--now", "subscale.service"); e != nil {
			return e
		}
	}
	fmt.Println("Login startup installed; registered accounts reconnect automatically")
	return nil
}
func stopForService(dir string) {
	if _, e := call(dir, daemon.Request{Op: "stop"}); e == nil {
		for i := 0; i < 50; i++ {
			time.Sleep(100 * time.Millisecond)
			if _, e = call(dir, daemon.Request{Op: "ping"}); e != nil {
				break
			}
		}
	}
}
