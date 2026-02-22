package phases

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	aptTimeout     = 5 * time.Minute
	nvidiaDriver   = "nvidia-driver-570"
	nvidiaRepoBase = "https://developer.download.nvidia.com/compute/cuda/repos"
	nctRepoBase    = "https://nvidia.github.io/libnvidia-container"
)

// runSudoCmd executes a command, optionally prepending sudo.
func runSudoCmd(ctx context.Context, useSudo bool, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, aptTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if useSudo {
		sudoArgs := append([]string{"-E", name}, args...)
		cmd = exec.CommandContext(cmdCtx, "sudo", sudoArgs...) // #nosec G204
	} else {
		cmd = exec.CommandContext(cmdCtx, name, args...) // #nosec G204
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runSudoShell executes a shell command string, optionally via sudo.
func runSudoShell(ctx context.Context, useSudo bool, shellCmd string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, aptTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.CommandContext(cmdCtx, "sudo", "sh", "-c", shellCmd) // #nosec G204
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", shellCmd) // #nosec G204
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// checkSecureBoot returns true if Secure Boot is enabled.
func checkSecureBoot(ctx context.Context) bool {
	out, err := runCmd(ctx, "mokutil", "--sb-state")
	if err != nil {
		// mokutil not installed or not EFI — assume Secure Boot is off
		return false
	}
	return strings.Contains(strings.ToLower(out), "secureboot enabled")
}

// checkKernelHeaders returns true if kernel headers are installed for the running kernel.
func checkKernelHeaders(ctx context.Context) bool {
	out, err := runCmd(ctx, "uname", "-r")
	if err != nil {
		return false
	}
	kernelVersion := strings.TrimSpace(out)
	_, err = runCmd(ctx, "dpkg", "-l", "linux-headers-"+kernelVersion)
	return err == nil
}

// installKernelHeaders installs kernel headers for the running kernel.
func installKernelHeaders(ctx context.Context, useSudo bool) error {
	out, err := runCmd(ctx, "uname", "-r")
	if err != nil {
		return fmt.Errorf("could not detect kernel version: %w", err)
	}
	kernelVersion := strings.TrimSpace(out)
	pkg := "linux-headers-" + kernelVersion

	var installErr error
	err = ui.WithSpinner("Installing kernel headers ("+pkg+")", func() error {
		_, installErr = runSudoCmd(ctx, useSudo, "apt-get", "install", "-y", pkg)
		return installErr
	})
	if err != nil {
		return fmt.Errorf("failed to install %s: %w", pkg, err)
	}
	return nil
}

// installDocker installs Docker Engine on Debian/Ubuntu systems.
func installDocker(ctx context.Context, distro config.Distro, useSudo bool) error {
	if distro.Family != "debian" {
		return fmt.Errorf("Docker auto-install only supported on Debian/Ubuntu (detected: %s)", distro.ID)
	}

	steps := []struct {
		desc string
		fn   func() error
	}{
		{"Installing prerequisites (ca-certificates, curl, gnupg)", func() error {
			_, err := runSudoCmd(ctx, useSudo, "apt-get", "update")
			if err != nil {
				return err
			}
			_, err = runSudoCmd(ctx, useSudo, "apt-get", "install", "-y",
				"ca-certificates", "curl", "gnupg")
			return err
		}},
		{"Adding Docker GPG key and repository", func() error {
			// Create keyrings dir
			_, _ = runSudoCmd(ctx, useSudo, "install", "-m", "0755", "-d", "/etc/apt/keyrings")

			// Download GPG key
			gpgCmd := fmt.Sprintf(
				"curl -fsSL https://download.docker.com/linux/%s/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg",
				distro.ID)
			_, err := runSudoShell(ctx, useSudo, gpgCmd)
			if err != nil {
				return fmt.Errorf("failed to add Docker GPG key: %w", err)
			}
			_, _ = runSudoCmd(ctx, useSudo, "chmod", "a+r", "/etc/apt/keyrings/docker.gpg")

			// Add repo
			repoCmd := fmt.Sprintf(
				`echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/%s $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list`,
				distro.ID)
			_, err = runSudoShell(ctx, useSudo, repoCmd)
			return err
		}},
		{"Installing Docker Engine", func() error {
			_, err := runSudoCmd(ctx, useSudo, "apt-get", "update")
			if err != nil {
				return err
			}
			_, err = runSudoCmd(ctx, useSudo, "apt-get", "install", "-y",
				"docker-ce", "docker-ce-cli", "containerd.io",
				"docker-buildx-plugin", "docker-compose-plugin")
			return err
		}},
	}

	for _, step := range steps {
		var stepErr error
		err := ui.WithSpinner(step.desc, func() error {
			stepErr = step.fn()
			return stepErr
		})
		if err != nil {
			return fmt.Errorf("%s: %w", step.desc, err)
		}
	}

	// Verify
	out, err := runCmd(ctx, "docker", "--version")
	if err != nil {
		return fmt.Errorf("Docker install completed but verification failed: %w", err)
	}
	ver, _ := ParseDockerVersion(out)
	ui.Success("Docker %s installed", ver)
	return nil
}

// installNVIDIADriver installs the NVIDIA driver on Debian/Ubuntu systems.
func installNVIDIADriver(ctx context.Context, distro config.Distro, useSudo bool) error {
	if distro.Family != "debian" {
		return fmt.Errorf("NVIDIA driver auto-install only supported on Debian/Ubuntu (detected: %s)", distro.ID)
	}

	// Pre-flight: Secure Boot
	if checkSecureBoot(ctx) {
		ui.Warn("Secure Boot is enabled — unsigned NVIDIA kernel modules may not load")
		ui.Detail("Disable Secure Boot in BIOS/UEFI or enroll MOK keys before proceeding")
		install, _ := ui.Confirm("Continue anyway?", false)
		if !install {
			return fmt.Errorf("NVIDIA driver installation aborted (Secure Boot enabled)")
		}
	}

	// Pre-flight: kernel headers
	if !checkKernelHeaders(ctx) {
		ui.Info("Kernel headers not found — installing before driver")
		if err := installKernelHeaders(ctx, useSudo); err != nil {
			return fmt.Errorf("kernel headers required for driver install: %w", err)
		}
	}

	// Determine CUDA repo URL for this distro
	repoDistro := distro.ID + distro.Version
	// Ubuntu uses e.g. "ubuntu2204", Debian uses "debian12"
	repoDistro = strings.ReplaceAll(repoDistro, ".", "")

	steps := []struct {
		desc string
		fn   func() error
	}{
		{"Adding NVIDIA CUDA repository", func() error {
			keyURL := fmt.Sprintf("%s/%s/x86_64/cuda-keyring_1.1-1_all.deb", nvidiaRepoBase, repoDistro)
			_, err := runSudoShell(ctx, useSudo,
				fmt.Sprintf("curl -fsSL -o /tmp/cuda-keyring.deb %s && dpkg -i /tmp/cuda-keyring.deb && rm -f /tmp/cuda-keyring.deb", keyURL))
			return err
		}},
		{"Installing " + nvidiaDriver, func() error {
			_, err := runSudoCmd(ctx, useSudo, "apt-get", "update")
			if err != nil {
				return err
			}
			_, err = runSudoCmd(ctx, useSudo, "apt-get", "install", "-y", nvidiaDriver)
			return err
		}},
	}

	for _, step := range steps {
		var stepErr error
		err := ui.WithSpinner(step.desc, func() error {
			stepErr = step.fn()
			return stepErr
		})
		if err != nil {
			return fmt.Errorf("%s: %w", step.desc, err)
		}
	}

	// Verify
	out, err := runCmd(ctx, "nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
	if err != nil {
		ui.Warn("nvidia-smi not available after install — a reboot may be required")
		ui.Detail("Run: sudo reboot")
		return fmt.Errorf("NVIDIA driver installed but nvidia-smi failed (reboot required): %w", err)
	}
	ver := strings.TrimSpace(strings.Split(strings.TrimSpace(out), "\n")[0])
	ui.Success("NVIDIA driver %s installed", ver)
	return nil
}

// installContainerToolkit installs the NVIDIA Container Toolkit and configures Docker.
func installContainerToolkit(ctx context.Context, distro config.Distro, useSudo bool) error {
	if distro.Family != "debian" {
		return fmt.Errorf("Container Toolkit auto-install only supported on Debian/Ubuntu (detected: %s)", distro.ID)
	}

	steps := []struct {
		desc string
		fn   func() error
	}{
		{"Adding NVIDIA Container Toolkit repository", func() error {
			gpgCmd := fmt.Sprintf(
				"curl -fsSL %s/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg",
				nctRepoBase)
			_, err := runSudoShell(ctx, useSudo, gpgCmd)
			if err != nil {
				return err
			}
			repoCmd := fmt.Sprintf(
				`curl -s -L %s/stable/deb/nvidia-container-toolkit.list | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | tee /etc/apt/sources.list.d/nvidia-container-toolkit.list`,
				nctRepoBase)
			_, err = runSudoShell(ctx, useSudo, repoCmd)
			return err
		}},
		{"Installing nvidia-container-toolkit", func() error {
			_, err := runSudoCmd(ctx, useSudo, "apt-get", "update")
			if err != nil {
				return err
			}
			_, err = runSudoCmd(ctx, useSudo, "apt-get", "install", "-y", "nvidia-container-toolkit")
			return err
		}},
		{"Configuring Docker runtime for NVIDIA", func() error {
			_, err := runSudoCmd(ctx, useSudo, "nvidia-ctk", "runtime", "configure", "--runtime=docker")
			if err != nil {
				return err
			}
			_, err = runSudoCmd(ctx, useSudo, "systemctl", "restart", "docker")
			return err
		}},
	}

	for _, step := range steps {
		var stepErr error
		err := ui.WithSpinner(step.desc, func() error {
			stepErr = step.fn()
			return stepErr
		})
		if err != nil {
			return fmt.Errorf("%s: %w", step.desc, err)
		}
	}

	// Verify
	out, err := runCmd(ctx, "nvidia-ctk", "--version")
	if err != nil {
		return fmt.Errorf("Container Toolkit installed but verification failed: %w", err)
	}
	ui.Success("NVIDIA Container Toolkit installed (%s)", strings.TrimSpace(out))
	return nil
}

// installFabricManager installs nvidia-fabricmanager for multi-GPU NVLink setups.
func installFabricManager(ctx context.Context, driverVersion string, useSudo bool) error {
	major := DriverMajorVersion(driverVersion)
	if major == "" {
		return fmt.Errorf("could not determine driver major version from %q", driverVersion)
	}
	pkg := "nvidia-fabricmanager-" + major

	var installErr error
	err := ui.WithSpinner("Installing "+pkg, func() error {
		_, installErr = runSudoCmd(ctx, useSudo, "apt-get", "install", "-y", pkg)
		return installErr
	})
	if err != nil {
		return fmt.Errorf("failed to install %s: %w", pkg, err)
	}

	err = ui.WithSpinner("Enabling Fabric Manager service", func() error {
		_, installErr = runSudoCmd(ctx, useSudo, "systemctl", "enable", "--now", "nvidia-fabricmanager")
		return installErr
	})
	if err != nil {
		return fmt.Errorf("failed to enable nvidia-fabricmanager: %w", err)
	}

	ui.Success("Fabric Manager (%s) installed and running", pkg)
	return nil
}
