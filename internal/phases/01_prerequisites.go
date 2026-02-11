package phases

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
)

const cmdTimeout = 15 * time.Second

// Prerequisites checks Docker, NVIDIA, disk, and system requirements.
type Prerequisites struct {
	mocked bool
}

// NewPrerequisites creates a new Prerequisites phase.
// When mocked is true, all checks use hardcoded demo values.
func NewPrerequisites(mocked bool) *Prerequisites {
	return &Prerequisites{mocked: mocked}
}

func (p *Prerequisites) Name() string {
	return "Prerequisites"
}

func (p *Prerequisites) Description() string {
	return "Checking Docker, NVIDIA drivers, disk space, and system requirements"
}

func (p *Prerequisites) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *Prerequisites) Run(ctx context.Context, state *config.State) error {
	// Detect sudo early — other phases (deploy) need this
	if !p.mocked {
		if docker.DetectSudo(ctx) {
			state.UseSudo = true
			ui.Info("Docker requires sudo — commands will use 'sudo -E'")
		}
	}

	if err := p.checkDocker(ctx, state); err != nil {
		return err
	}
	if err := p.checkDockerCompose(ctx); err != nil {
		return err
	}
	if err := p.checkNVIDIADriver(ctx, state); err != nil {
		return err
	}
	if err := p.checkContainerToolkit(ctx); err != nil {
		// Non-fatal: warn only
		ui.Warn("NVIDIA Container Toolkit not detected: %v", err)
		ui.Detail("Install with: sudo apt-get install nvidia-container-toolkit")
	}
	if err := p.checkCUDAInDocker(ctx, state); err != nil {
		return err
	}
	p.checkAutoUpdates(ctx, state)
	p.checkDiskSpace(ctx, state)
	p.checkPorts(state)

	ui.Success("All prerequisites satisfied")
	return nil
}

func (p *Prerequisites) checkDocker(ctx context.Context, _ *config.State) error {
	if p.mocked {
		return p.mockedCheck("Checking Docker installation", "Docker version: 27.4.1 (mocked)")
	}
	var version string
	err := ui.WithSpinner("Checking Docker installation", func() error {
		out, cmdErr := runCmd(ctx, "docker", "--version")
		if cmdErr != nil {
			return fmt.Errorf("docker not found: %w", cmdErr)
		}
		ver, parseErr := ParseDockerVersion(out)
		if parseErr != nil {
			return parseErr
		}
		version = ver
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Docker version: %s", version)
	return nil
}

func (p *Prerequisites) checkDockerCompose(ctx context.Context) error {
	if p.mocked {
		return p.mockedCheck("Checking Docker Compose", "Docker Compose version: v2.32.4 (mocked)")
	}
	var version string
	err := ui.WithSpinner("Checking Docker Compose", func() error {
		out, cmdErr := runCmd(ctx, "docker", "compose", "version")
		if cmdErr != nil {
			return fmt.Errorf("docker compose not found: %w", cmdErr)
		}
		ver, parseErr := ParseDockerComposeVersion(out)
		if parseErr != nil {
			return parseErr
		}
		version = ver
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Docker Compose version: %s", version)
	return nil
}

func (p *Prerequisites) checkNVIDIADriver(ctx context.Context, state *config.State) error {
	if p.mocked {
		state.DriverInfo = config.DriverInfo{
			UserVersion:   "570.133.20",
			KernelVersion: "570.133.20",
			FMVersion:     "570.133.20",
			Consistent:    true,
		}
		_ = p.mockedCheck("Checking NVIDIA driver", "NVIDIA driver: 570.133.20 (mocked)")
		ui.Success("Driver versions consistent (user: %s, kernel: %s, FM: %s)",
			state.DriverInfo.UserVersion, state.DriverInfo.KernelVersion, state.DriverInfo.FMVersion)
		return nil
	}

	var driverVer string
	err := ui.WithSpinner("Checking NVIDIA driver", func() error {
		out, cmdErr := runCmd(ctx, "nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
		if cmdErr != nil {
			return fmt.Errorf("nvidia-smi not found — NVIDIA driver required: %w", cmdErr)
		}
		// Take first line (all GPUs report same driver)
		driverVer = strings.TrimSpace(strings.Split(strings.TrimSpace(out), "\n")[0])
		return nil
	})
	if err != nil {
		return err
	}
	state.DriverInfo = config.DriverInfo{
		UserVersion: driverVer,
		Consistent:  true, // simplified — full consistency check needs modinfo + FM version
	}
	ui.Detail("NVIDIA driver: %s", driverVer)
	return nil
}

func (p *Prerequisites) checkContainerToolkit(ctx context.Context) error {
	if p.mocked {
		return p.mockedCheck("Checking NVIDIA Container Toolkit", "nvidia-container-toolkit: 1.17.4 (mocked)")
	}
	var version string
	err := ui.WithSpinner("Checking NVIDIA Container Toolkit", func() error {
		out, cmdErr := runCmd(ctx, "nvidia-ctk", "--version")
		if cmdErr != nil {
			return fmt.Errorf("nvidia-ctk not found: %w", cmdErr)
		}
		version = strings.TrimSpace(out)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Container Toolkit: %s", version)
	return nil
}

func (p *Prerequisites) checkCUDAInDocker(ctx context.Context, state *config.State) error {
	if p.mocked {
		return p.mockedCheck("Verifying CUDA inside Docker container",
			"CUDA available inside Docker (nvidia-smi works in container)")
	}
	err := ui.WithSpinner("Verifying CUDA inside Docker container", func() error {
		args := []string{"run", "--rm", "--gpus", "all",
			"nvidia/cuda:12.6.0-base-ubuntu22.04", "nvidia-smi"}
		var cmd *exec.Cmd
		if state.UseSudo {
			sudoArgs := append([]string{"-E", "docker"}, args...)
			cmd = exec.CommandContext(ctx, "sudo", sudoArgs...) // #nosec G204
		} else {
			cmd = exec.CommandContext(ctx, "docker", args...) // #nosec G204
		}
		out, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			return fmt.Errorf("CUDA not available in Docker container: %w\n%s", cmdErr, string(out))
		}
		return nil
	})
	if err != nil {
		return err
	}
	ui.Success("CUDA available inside Docker (nvidia-smi works in container)")
	return nil
}

func (p *Prerequisites) checkAutoUpdates(ctx context.Context, state *config.State) {
	if p.mocked {
		state.AutoUpdateOff = true
		ui.Success("No auto-update packages detected that could break NVIDIA drivers")
		return
	}
	err := ui.WithSpinner("Checking for auto-update packages", func() error {
		_, cmdErr := runCmd(ctx, "dpkg", "-l", "unattended-upgrades")
		if cmdErr != nil {
			// Not installed — good
			return nil
		}
		return fmt.Errorf("unattended-upgrades detected")
	})
	if err != nil {
		state.AutoUpdateOff = false
		ui.Warn("unattended-upgrades is installed — this can break NVIDIA drivers during auto-update")
		ui.Detail("Consider: sudo apt-mark hold nvidia-driver-*")
	} else {
		state.AutoUpdateOff = true
		ui.Success("No auto-update packages detected that could break NVIDIA drivers")
	}
}

func (p *Prerequisites) checkDiskSpace(ctx context.Context, state *config.State) {
	if p.mocked {
		state.DiskFreeGB = 512
		ui.Success("Disk space: %d GB free (250 GB minimum for Cosmovisor upgrades)", state.DiskFreeGB)
		return
	}
	var freeGB int
	err := ui.WithSpinner("Checking available disk space", func() error {
		out, cmdErr := runCmd(ctx, "df", "--output=avail", "-BG", state.OutputDir)
		if cmdErr != nil {
			return cmdErr
		}
		gb, parseErr := ParseDiskFreeGB(out)
		if parseErr != nil {
			return parseErr
		}
		freeGB = gb
		return nil
	})
	if err != nil {
		ui.Warn("Could not check disk space: %v", err)
		return
	}
	state.DiskFreeGB = freeGB

	minDisk := 250
	if state.IsTestNet {
		minDisk = 133
	}
	if freeGB >= minDisk {
		ui.Success("Disk space: %d GB free (%d GB minimum)", freeGB, minDisk)
	} else {
		ui.Warn("Disk space: %d GB free — recommended minimum is %d GB", freeGB, minDisk)
	}
}

func (p *Prerequisites) checkPorts(state *config.State) {
	if p.mocked {
		ports := []struct {
			port int
			name string
		}{
			{5000, "P2P"}, {8000, "API"}, {26657, "RPC"},
			{5050, "ML Inference"}, {8080, "PoC"},
			{9100, "API ML Callback"}, {9200, "Admin API"},
		}
		for _, pt := range ports {
			ui.Detail("Port %d (%s): available (mocked)", pt.port, pt.name)
		}
		return
	}

	ports := []struct {
		port int
		name string
	}{
		{state.P2PPort, "P2P"},
		{state.APIPort, "API"},
		{26657, "RPC"},
		{5050, "ML Inference"},
		{8080, "PoC"},
		{9100, "API ML Callback"},
		{9200, "Admin API"},
	}
	for _, pt := range ports {
		addr := fmt.Sprintf(":%d", pt.port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			ui.Warn("Port %d (%s): in use — may conflict during deployment", pt.port, pt.name)
		} else {
			_ = ln.Close()
			ui.Detail("Port %d (%s): available", pt.port, pt.name)
		}
	}
}

// mockedCheck simulates a check with a brief sleep and detail message.
func (p *Prerequisites) mockedCheck(spinnerMsg, detailMsg string) error {
	err := ui.WithSpinner(spinnerMsg, func() error {
		time.Sleep(300 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail(detailMsg)
	return nil
}

// runCmd executes a command with timeout and returns stdout.
func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...) // #nosec G204 - args are constructed internally
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
