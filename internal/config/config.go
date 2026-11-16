package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const ProxyAddress = "127.0.0.1:18765"

type Account struct {
	Enabled bool `json:"enabled"`
}
type Route struct {
	Account string `json:"account"`
	Host    string `json:"host"`
	Target  string `json:"target,omitempty"`
}
type Config struct {
	Accounts map[string]Account `json:"accounts"`
	Routes   []Route            `json:"routes"`
}

func Dir() (string, error) {
	if d := os.Getenv("SUBSCALE_HOME"); d != "" {
		return filepath.Abs(d)
	}
	d, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "subscale"), nil
}
func Load(dir string) (Config, error) {
	c := Config{Accounts: map[string]Account{}, Routes: []Route{}}
	b, e := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(e, os.ErrNotExist) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, fmt.Errorf("invalid config: %w", e)
	}
	if c.Accounts == nil {
		c.Accounts = map[string]Account{}
	}
	for n := range c.Accounts {
		if e := Name(n); e != nil {
			return c, e
		}
	}
	for i, r := range c.Routes {
		if _, ok := c.Accounts[r.Account]; !ok {
			return c, fmt.Errorf("unknown route account %q", r.Account)
		}
		h, e := Host(r.Host, true)
		if e != nil || h != r.Host {
			return c, fmt.Errorf("invalid route host %q", r.Host)
		}
		if r.Target != "" {
			h, e = Host(r.Target, false)
			if e != nil || h != r.Target {
				return c, fmt.Errorf("invalid target %q", r.Target)
			}
		}
		for _, other := range c.Routes[:i] {
			if other.Host == r.Host {
				return c, fmt.Errorf("duplicate route %q", r.Host)
			}
		}
	}
	return c, nil
}
func Save(dir string, c Config) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".config-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(dir, "config.json"))
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,31}$`)

func Name(s string) error {
	if !validName.MatchString(s) {
		return fmt.Errorf("account name must be 1-32 ASCII letters, digits, _ or -, starting with a letter or digit")
	}
	switch s {
	case "help", "version", "daemon", "start", "stop", "status", "list", "proxy", "exec", "connect", "service":
		return fmt.Errorf("reserved account name %q", s)
	}
	return nil
}
func Host(s string, wildcard bool) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.Contains(s, "://") {
		u, e := url.Parse(s)
		if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return "", fmt.Errorf("invalid HTTP(S) address")
		}
		if p := u.Port(); p != "" {
			if n, e := strconv.Atoi(p); e != nil || n < 1 || n > 65535 {
				return "", fmt.Errorf("invalid port")
			}
		}
		s = u.Hostname()
	}
	if h, port, e := net.SplitHostPort(s); e == nil {
		if n, e := strconv.Atoi(port); e != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid port")
		}
		s = h
	}
	s = strings.TrimSuffix(strings.Trim(s, "[]"), ".")
	if ip, e := netip.ParseAddr(s); e == nil {
		if ip.Zone() != "" {
			return "", fmt.Errorf("IPv6 zones are unsupported")
		}
		return ip.Unmap().String(), nil
	}
	prefix := ""
	if wildcard && strings.HasPrefix(s, "*.") {
		prefix = "*."
		s = s[2:]
	}
	if len(s) == 0 || len(s) > 253 {
		return "", fmt.Errorf("invalid hostname")
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid hostname")
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return "", fmt.Errorf("invalid hostname: use ASCII or punycode, IP, or *.domain")
			}
		}
	}
	return prefix + s, nil
}
func Match(routes []Route, host string) (Route, bool) {
	h, e := Host(host, false)
	if e != nil {
		return Route{}, false
	}
	var best Route
	score := -1
	for _, r := range routes {
		if r.Host == h {
			return r, true
		}
		if strings.HasPrefix(r.Host, "*.") && strings.HasSuffix(h, r.Host[1:]) && len(r.Host) > score {
			best = r
			score = len(r.Host)
		}
	}
	return best, score >= 0
}
