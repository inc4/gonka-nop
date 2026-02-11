package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
)

// ComposeClient wraps Docker Compose CLI operations.
type ComposeClient struct {
	WorkDir string   // directory containing compose files
	Files   []string // compose file names (e.g., ["docker-compose.yml", "docker-compose.mlnode.yml"])
	EnvFile string   // path to config.env (for env propagation)
	UseSudo bool     // prefix commands with sudo -E
	Stdout  io.Writer
	Stderr  io.Writer
}

// NewComposeClient creates a client from state.
func NewComposeClient(state *config.State) (*ComposeClient, error) {
	if state.OutputDir == "" {
		return nil, fmt.Errorf("output directory not set in state")
	}

	files := state.ComposeFiles
	if len(files) == 0 {
		files = []string{"docker-compose.yml", "docker-compose.mlnode.yml"}
	}

	envFile := state.OutputDir + "/config.env"

	return &ComposeClient{
		WorkDir: state.OutputDir,
		Files:   files,
		EnvFile: envFile,
		UseSudo: state.UseSudo,
	}, nil
}

// baseArgs returns ["-f", "file1", "-f", "file2"].
func (c *ComposeClient) baseArgs() []string {
	args := make([]string, 0, 2*len(c.Files))
	for _, f := range c.Files {
		args = append(args, "-f", f)
	}
	return args
}

// run executes a docker compose command.
func (c *ComposeClient) run(ctx context.Context, args ...string) error {
	cmdArgs := append([]string{"compose"}, c.baseArgs()...)
	cmdArgs = append(cmdArgs, args...)

	var cmd *exec.Cmd
	if c.UseSudo {
		sudoArgs := append([]string{"-E", "docker"}, cmdArgs...)
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...) // #nosec G204 - args are constructed internally
	} else {
		cmd = exec.CommandContext(ctx, "docker", cmdArgs...) // #nosec G204 - args are constructed internally
	}

	cmd.Dir = c.WorkDir

	// Load environment from config.env if it exists
	fileEnv, err := ParseEnvFile(c.EnvFile)
	if err == nil && len(fileEnv) > 0 {
		cmd.Env = MergeEnv(fileEnv)
	}

	var stderr bytes.Buffer
	if c.Stdout != nil {
		cmd.Stdout = c.Stdout
	}
	if c.Stderr != nil {
		cmd.Stderr = io.MultiWriter(c.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("docker compose %s: %w\n%s", strings.Join(args, " "), err, errMsg)
		}
		return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Pull pulls all images.
func (c *ComposeClient) Pull(ctx context.Context) error {
	return c.run(ctx, "pull")
}

// Up starts services (docker compose up -d [services...]).
func (c *ComposeClient) Up(ctx context.Context, services ...string) error {
	args := make([]string, 0, 2+len(services))
	args = append(args, "up", "-d")
	args = append(args, services...)
	return c.run(ctx, args...)
}

// Down stops services.
func (c *ComposeClient) Down(ctx context.Context) error {
	return c.run(ctx, "down")
}

// Logs returns recent logs for a service.
func (c *ComposeClient) Logs(ctx context.Context, service string, lines int) (string, error) {
	cmdArgs := append([]string{"compose"}, c.baseArgs()...)
	cmdArgs = append(cmdArgs, "logs", "--tail", fmt.Sprintf("%d", lines), service)

	var cmd *exec.Cmd
	if c.UseSudo {
		sudoArgs := append([]string{"-E", "docker"}, cmdArgs...)
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...) // #nosec G204 - args are constructed internally
	} else {
		cmd = exec.CommandContext(ctx, "docker", cmdArgs...) // #nosec G204 - args are constructed internally
	}
	cmd.Dir = c.WorkDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose logs %s: %w", service, err)
	}
	return string(out), nil
}

// Ps lists running containers. Env is loaded to suppress missing-variable warnings.
func (c *ComposeClient) Ps(ctx context.Context) (string, error) {
	cmdArgs := append([]string{"compose"}, c.baseArgs()...)
	cmdArgs = append(cmdArgs, "ps", "--format", "table")

	var cmd *exec.Cmd
	if c.UseSudo {
		sudoArgs := append([]string{"-E", "docker"}, cmdArgs...)
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...) // #nosec G204 - args are constructed internally
	} else {
		cmd = exec.CommandContext(ctx, "docker", cmdArgs...) // #nosec G204 - args are constructed internally
	}
	cmd.Dir = c.WorkDir

	// Load env to avoid "variable is not set" warnings from compose
	fileEnv, err := ParseEnvFile(c.EnvFile)
	if err == nil && len(fileEnv) > 0 {
		cmd.Env = MergeEnv(fileEnv)
	}

	// Separate stdout (table) from stderr (warnings)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("docker compose ps: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// DetectSudo checks if docker needs sudo by running `docker info`.
func DetectSudo(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() != nil
}
