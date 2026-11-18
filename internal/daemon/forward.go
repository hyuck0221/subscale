package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/hyuck0221/subscale/internal/config"
)

// Each listener owns its connections, so removal and shutdown also end sessions.
type tcpForward struct {
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	conns    map[net.Conn]bool
}

func (f *tcpForward) track(c net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ctx.Err() != nil {
		c.Close()
		return false
	}
	f.conns[c] = true
	return true
}

func (f *tcpForward) release(c net.Conn) {
	c.Close()
	f.mu.Lock()
	delete(f.conns, c)
	f.mu.Unlock()
}

func (f *tcpForward) close() {
	f.cancel()
	f.listener.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	for c := range f.conns {
		c.Close()
	}
}

func (f *tcpForward) serve(dial func(context.Context) (net.Conn, error)) {
	for {
		client, err := f.listener.Accept()
		if err != nil {
			return
		}
		if !f.track(client) {
			return
		}
		go func() {
			defer f.release(client)
			ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
			upstream, err := dial(ctx)
			cancel()
			if err != nil {
				log.Printf("forward %s: %v", f.listener.Addr(), err)
				return
			}
			if !f.track(upstream) {
				return
			}
			defer f.release(upstream)
			done := make(chan struct{})
			go func() {
				defer close(done)
				io.Copy(upstream, client)
				closeForwardWrite(upstream)
			}()
			io.Copy(client, upstream)
			closeForwardWrite(client)
			// The peer may still be finishing its write after receiving EOF.
			<-done
		}()
	}
}

func closeForwardWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	}
}

// Called with a.mu held. The dial callback resolves the current account on each
// connection, allowing rename to take effect without replacing listeners.
func (a *API) listenForward(port int) (*tcpForward, int, error) {
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, 0, err
	}
	port = listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	f := &tcpForward{listener: listener, ctx: ctx, cancel: cancel, conns: map[net.Conn]bool{}}
	go f.serve(func(ctx context.Context) (net.Conn, error) {
		a.mu.Lock()
		var target config.Forward
		for _, current := range a.Config.Forwards {
			if current.LocalPort == port {
				target = current
				break
			}
		}
		a.mu.Unlock()
		if target.Account == "" {
			return nil, fmt.Errorf("forward was removed")
		}
		// This always uses the named tailnet; it never falls back to the OS.
		return a.Dial(ctx, target.Account, target.Target)
	})
	return f, port, nil
}

func (a *API) applyForward(q Request, c config.Config) error {
	target, err := config.Endpoint(q.Target)
	if err != nil {
		return err
	}
	if q.LocalPort < 0 || q.LocalPort > 65535 || q.LocalPort == 18765 {
		return fmt.Errorf("invalid or reserved local port %d", q.LocalPort)
	}
	index := -1
	for i, f := range c.Forwards {
		if f.Account == q.Name && f.Target == target {
			index = i
			break
		}
	}
	if q.Op == "forward-remove" {
		if index < 0 {
			return fmt.Errorf("forward %q not found in account %s", target, q.Name)
		}
		port := c.Forwards[index].LocalPort
		c.Forwards = append(c.Forwards[:index], c.Forwards[index+1:]...)
		if err := config.Save(a.Dir, c); err != nil {
			return err
		}
		a.Config = c
		a.closeForward(port)
		return nil
	}
	port := q.LocalPort
	if index >= 0 {
		previous := c.Forwards[index]
		if port != 0 && port != previous.LocalPort {
			return fmt.Errorf("forward already uses local port %d; remove it before changing the port", previous.LocalPort)
		}
		port = previous.LocalPort
		if a.forwards[port] != nil {
			return nil
		}
	}
	for _, f := range c.Forwards {
		if f.LocalPort == port && (f.Account != q.Name || f.Target != target) {
			return fmt.Errorf("local port %d is already registered", port)
		}
	}
	f, port, err := a.listenForward(port)
	if err != nil {
		return fmt.Errorf("listen on local port %d: %w", port, err)
	}
	// An inactive saved forward can own a port not currently bound by us.
	for i, previous := range c.Forwards {
		if i != index && previous.LocalPort == port {
			f.close()
			return fmt.Errorf("automatically selected port %d belongs to another saved forward; retry", port)
		}
	}
	if index < 0 {
		c.Forwards = append(c.Forwards, config.Forward{Account: q.Name, Target: target, LocalPort: port})
	}
	if err := config.Save(a.Dir, c); err != nil {
		f.close()
		return err
	}
	a.Config = c
	if a.forwards == nil {
		a.forwards = map[int]*tcpForward{}
	}
	a.forwards[port] = f
	delete(a.forwardErrors, port)
	return nil
}

func (a *API) restoreForwards() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.forwards == nil {
		a.forwards = map[int]*tcpForward{}
	}
	a.forwardErrors = map[int]string{}
	for _, saved := range a.Config.Forwards {
		if a.forwards[saved.LocalPort] != nil {
			continue
		}
		f, _, err := a.listenForward(saved.LocalPort)
		if err != nil {
			a.forwardErrors[saved.LocalPort] = err.Error()
			log.Printf("restore forward %s -> %s: %v", saved.LocalAddress(), saved.Target, err)
			continue
		}
		a.forwards[saved.LocalPort] = f
	}
}

// Called with a.mu held.
func (a *API) closeForward(port int) {
	if f := a.forwards[port]; f != nil {
		f.close()
		delete(a.forwards, port)
	}
	delete(a.forwardErrors, port)
}

func (a *API) closeForwards() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for port, f := range a.forwards {
		f.close()
		delete(a.forwards, port)
	}
}

// Called with a.mu held. "listening" describes the local listener, not remote reachability.
func (a *API) forwardStatus() map[int]string {
	status := map[int]string{}
	for _, f := range a.Config.Forwards {
		switch {
		case a.forwardErrors[f.LocalPort] != "":
			status[f.LocalPort] = "error: " + a.forwardErrors[f.LocalPort]
		case a.forwards[f.LocalPort] == nil:
			status[f.LocalPort] = "inactive"
		case !a.Config.Accounts[f.Account].Enabled:
			status[f.LocalPort] = "logged out"
		default:
			status[f.LocalPort] = "listening"
		}
	}
	return status
}
