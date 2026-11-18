//go:build darwin || linux

package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
	"github.com/hyuck0221/subscale/internal/router"
	"github.com/hyuck0221/subscale/internal/tailnet"
	"golang.org/x/sys/unix"
)

func Run(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	if e := os.Chmod(dir, 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(filepath.Join(dir, "daemon.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		return fmt.Errorf("subscale daemon already running: %w", e)
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	c, e := config.Load(dir)
	if e != nil {
		return e
	}
	// Named interactive accounts must not inherit an unrelated deployment identity.
	for _, k := range []string{"TS_AUTHKEY", "TS_AUTH_KEY", "TS_CLIENT_SECRET", "TSNET_FORCE_LOGIN"} {
		os.Unsetenv(k)
	}
	sock := filepath.Join(dir, "daemon.sock")
	if e = os.Remove(sock); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	management, e := net.Listen("unix", sock)
	if e != nil {
		return e
	}
	defer management.Close()
	defer os.Remove(sock)
	if e = os.Chmod(sock, 0600); e != nil {
		return e
	}
	proxyListener, e := net.Listen("tcp", config.ProxyAddress)
	if e != nil {
		return e
	}
	defer proxyListener.Close()
	nodes := tailnet.New(dir)
	defer nodes.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	api := &API{Dir: dir, Config: c, Nodes: nodes, Stop: cancel}
	defer api.closeForwards()
	proxy := &router.Proxy{Routes: api.Routes, DialAccount: api.Dial}
	ps := &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 1 << 20}
	ms := &http.Server{Handler: api, ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 2)
	go func() { errs <- ms.Serve(management) }()
	go func() { errs <- ps.Serve(proxyListener) }()
	log.Printf("subscale listening on %s", config.ProxyAddress)
	for n := range c.Accounts {
		if ctx.Err() != nil {
			break
		}
		api.mu.Lock()
		if account := api.Config.Accounts[n]; account.Enabled {
			if startErr := nodes.Start(n, account.Storage(n), account.Ephemeral); startErr != nil {
				log.Printf("start %s: %v", n, startErr)
			}
		}
		api.mu.Unlock()
	}
	api.restoreForwards()
	select {
	case <-ctx.Done():
	case e = <-errs:
	}
	// Close the TCP listeners first. Account shutdown also closes active tunnels.
	ms.Close()
	ps.Close()
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
