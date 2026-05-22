package cosmovisor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RunHostCmd executes a shell command on the host, optionally with sudo.
// Used by FixSymlink and other helpers that must modify Cosmovisor state
// in directories whose ownership may require elevated privileges.
func RunHostCmd(ctx context.Context, useSudo bool, dir, shellCmd string) error {
	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.CommandContext(ctx, "sudo", "sh", "-c", shellCmd) // #nosec G204
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", shellCmd) // #nosec G204
	}
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", shellCmd, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ShellQuote wraps a string in single quotes for safe shell interpolation.
// Embedded single quotes are escaped via the standard `'\”` pattern.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
