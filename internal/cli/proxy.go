package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hyuck0221/subscale/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type pacState struct {
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

func proxySettings(dir string, args []string) error {
	url := "http://" + config.ProxyAddress + "/proxy.pac"
	if len(args) == 1 && args[0] == "pac" {
		if e := ensure(dir); e != nil {
			return e
		}
		fmt.Println(url)
		return nil
	}
	if len(args) != 2 || (args[0] != "on" && args[0] != "off") {
		return errors.New("usage: subscale proxy pac | subscale proxy on|off SERVICE (e.g. Wi-Fi)")
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("set your browser's automatic proxy URL to %s (native proxy settings currently support macOS)", url)
	}
	if args[0] == "on" {
		if e := ensure(dir); e != nil {
			return e
		}
	}
	stateFile := filepath.Join(dir, "proxy-backup.json")
	saved := map[string]pacState{}
	b, e := os.ReadFile(stateFile)
	if e == nil {
		if e = json.Unmarshal(b, &saved); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	service := args[1]
	run := func(a ...string) (string, error) {
		b, e := exec.Command("/usr/sbin/networksetup", a...).CombinedOutput()
		if e != nil {
			return "", fmt.Errorf("networksetup: %s: %w", strings.TrimSpace(string(b)), e)
		}
		text := string(b)
		if strings.Contains(text, "Error:") || strings.Contains(text, "not a recognized") {
			return "", errors.New(strings.TrimSpace(text))
		}
		return text, nil
	}
	currentText, e := run("-getautoproxyurl", service)
	if e != nil {
		return e
	}
	current := pacState{}
	for _, line := range strings.Split(currentText, "\n") {
		if v, ok := strings.CutPrefix(line, "URL: "); ok {
			current.URL = v
		}
		if line == "Enabled: Yes" {
			current.Enabled = true
		}
	}
	save := func() error {
		if e := os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		b, e := json.MarshalIndent(saved, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(stateFile, b, 0600)
	}
	if args[0] == "on" {
		if _, ok := saved[service]; !ok {
			saved[service] = current
			if e = save(); e != nil {
				return e
			}
		}
		if _, e = run("-setautoproxyurl", service, url); e != nil {
			return e
		}
		if _, e = run("-setautoproxystate", service, "on"); e != nil {
			return e
		}
		fmt.Printf("%s: automatic proxy enabled (%s)\n", service, url)
		return nil
	}
	previous, ok := saved[service]
	if !ok {
		return fmt.Errorf("no saved proxy settings for %q", service)
	}
	if current.URL != url {
		return fmt.Errorf("proxy URL was changed externally; refusing to overwrite it (backup: %s)", stateFile)
	}
	if previous.URL != "" && previous.URL != "(null)" {
		if _, e = run("-setautoproxyurl", service, previous.URL); e != nil {
			return e
		}
	}
	enabled := "off"
	if previous.Enabled {
		enabled = "on"
	}
	if _, e = run("-setautoproxystate", service, enabled); e != nil {
		return e
	}
	delete(saved, service)
	if e = save(); e != nil {
		return e
	}
	fmt.Printf("%s: previous automatic proxy setting restored\n", service)
	return nil
}
