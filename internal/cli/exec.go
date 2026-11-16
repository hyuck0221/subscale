package cli

import (
	"errors"
	"github.com/hyuck0221/subscale/internal/config"
	"os"
	"os/exec"
	"strings"
)

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
