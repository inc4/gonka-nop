// Package chain provides Cosmos-chain interaction helpers (tx broadcast,
// query execution via docker-exec into the api container, key-related
// helpers) and PoC-phase safety predicates shared by NOP commands.
package chain

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
)

// LoadKeyringPassword returns the keyring password from <OutputDir>/config.env,
// or "" if the file is unreadable or KEYRING_PASSWORD is not set.
//
// SECURITY: the returned value is sensitive. Callers typically assign to
// state.KeyringPassword which lives for the process lifetime. M11 accepts
// this risk; a future SecretString type with explicit zeroization may
// supersede this contract.
func LoadKeyringPassword(state *config.State) string {
	envFile := state.OutputDir + "/config.env"
	envVars, err := docker.ParseEnvFile(envFile)
	if err != nil {
		return ""
	}
	for _, kv := range envVars {
		if strings.HasPrefix(kv, "KEYRING_PASSWORD=") {
			return strings.TrimPrefix(kv, "KEYRING_PASSWORD=")
		}
	}
	return ""
}

// RunComposeExec runs a shell command inside a container via `docker compose run --rm --no-deps`.
// It loads <OutputDir>/config.env into the subprocess environment to suppress
// Compose "variable is not set" warnings, and prefixes with sudo when state.UseSudo.
// Returns combined stdout+stderr (trimmed) on success; on failure returns the
// combined output plus the wrapped error.
func RunComposeExec(ctx context.Context, state *config.State, service, shellCmd string) (string, error) {
	files := state.ComposeFiles
	if len(files) == 0 {
		files = []string{"docker-compose.yml"}
	}
	// 1 (compose) + 2*len(files) (-f file) + 7 (run --rm --no-deps service sh -c cmd)
	composeArgs := make([]string, 0, 1+2*len(files)+7)
	composeArgs = append(composeArgs, "compose")
	for _, f := range files {
		composeArgs = append(composeArgs, "-f", f)
	}
	composeArgs = append(composeArgs, "run", "--rm", "--no-deps", service, "sh", "-c", shellCmd)

	var cmd *exec.Cmd
	if state.UseSudo {
		sudoArgs := append([]string{"-E", docker.CmdDocker}, composeArgs...)
		cmd = exec.CommandContext(ctx, docker.CmdSudo, sudoArgs...) // #nosec G204
	} else {
		cmd = exec.CommandContext(ctx, docker.CmdDocker, composeArgs...) // #nosec G204
	}
	cmd.Dir = state.OutputDir

	// Load config.env to suppress Docker Compose "variable is not set" warnings
	envFile := state.OutputDir + "/config.env"
	fileEnv, envErr := docker.ParseEnvFile(envFile)
	if envErr == nil && len(fileEnv) > 0 {
		cmd.Env = docker.MergeEnv(fileEnv)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return combined, fmt.Errorf("docker compose run %s: %w\n%s", service, err, combined)
	}

	return strings.TrimSpace(stdout.String()), nil
}
