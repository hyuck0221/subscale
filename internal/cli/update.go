//go:build darwin || linux

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/hyuck0221/subscale/internal/daemon"
	"github.com/hyuck0221/subscale/internal/update"
)

func updateBinary(dir, current string) error {
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return e
	}
	fmt.Println("Checking the latest subscale release…")
	result, e := update.Default().Apply(context.Background(), executable, current)
	if e != nil {
		return e
	}
	if !result.Changed {
		fmt.Printf("subscale %s is already up to date\n", result.Version)
		return nil
	}
	fmt.Printf("Updated subscale to %s\n", result.Version)
	if _, e = call(dir, daemon.Request{Op: "ping"}); e != nil {
		return nil
	}
	// Remember the service owner before asking the old daemon to exit cleanly.
	var restart *exec.Cmd
	if runtime.GOOS == "darwin" {
		label := "gui/" + strconv.Itoa(os.Getuid()) + "/io.subscale.agent"
		if exec.Command("launchctl", "print", label).Run() == nil {
			restart = exec.Command("launchctl", "kickstart", label)
		}
	} else if exec.Command("systemctl", "--user", "is-active", "--quiet", "subscale.service").Run() == nil {
		restart = exec.Command("systemctl", "--user", "restart", "subscale.service")
	}
	if _, e = call(dir, daemon.Request{Op: "stop"}); e != nil {
		return fmt.Errorf("updated binary, but could not stop old daemon: %w", e)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, e = os.Stat(filepath.Join(dir, "daemon.sock")); os.IsNotExist(e) {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("updated binary, but old daemon did not stop; restart subscale manually")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if restart == nil {
		restart = exec.Command(executable, "start")
		restart.Env = append(os.Environ(), "SUBSCALE_HOME="+dir)
	}
	if output, e := restart.CombinedOutput(); e != nil {
		return fmt.Errorf("updated binary, but daemon restart failed: %s: %w", output, e)
	}
	fmt.Println("Background daemon restarted; saved accounts and rules retained")
	return nil
}
