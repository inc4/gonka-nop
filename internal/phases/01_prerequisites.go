package phases

import (
	"context"
	"fmt"
	"net"
	"os"
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
	// 1. Detect distro
	if err := p.detectDistro(state); err != nil {
		ui.Warn("Could not detect Linux distro: %v", err)
	}

	// 2. Detect sudo early — other phases (deploy, installs) need this
	if !p.mocked {
		if docker.DetectSudo(ctx) {
			state.UseSudo = true
			ui.Info("Docker requires sudo — commands will use 'sudo -E'")
		}
	}

	// 3. Check Docker → offer install if missing
	if err := p.checkDocker(ctx, state); err != nil {
		return err
	}

	// 4. Check Docker Compose (comes with docker-ce install)
	if err := p.checkDockerCompose(ctx); err != nil {
		return err
	}

	// 5-9: NVIDIA checks — skip for network-only topology (no GPU needed)
	if !state.IsNetworkOnly() {
		// 5. Check NVIDIA driver → offer install if missing
		if err := p.checkNVIDIADriver(ctx, state); err != nil {
			return err
		}

		// 6. Check driver consistency (userspace vs kernel module vs FM)
		if !p.mocked {
			p.checkDriverConsistency(ctx, state)
		}

		// 7. Check Container Toolkit → offer install if missing
		if err := p.checkContainerToolkit(ctx, state); err != nil {
			return err
		}

		// 8. Check CUDA in Docker
		if err := p.checkCUDAInDocker(ctx, state); err != nil {
			return err
		}

		// 9. Check Fabric Manager if multi-GPU
		if !p.mocked {
			p.checkFabricManager(ctx, state)
		}

		// Check auto-updates (NVIDIA driver risk)
		p.checkAutoUpdates(ctx, state)
	} else {
		ui.Info("Skipping NVIDIA checks (network-only topology — no GPU required)")
	}

	// 10-12. System checks
	p.checkStorageLayout(ctx, state)
	p.checkPorts(state)

	ui.Success("All prerequisites satisfied")
	return nil
}

func (p *Prerequisites) detectDistro(state *config.State) error {
	if p.mocked {
		state.Distro = config.Distro{ID: "ubuntu", Version: "22.04", Family: "debian"}
		return nil
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("read /etc/os-release: %w", err)
	}
	distro, err := ParseOSRelease(string(data))
	if err != nil {
		return err
	}
	state.Distro = distro
	ui.Detail("Linux distro: %s %s (%s family)", distro.ID, distro.Version, distro.Family)
	return nil
}

func (p *Prerequisites) checkDocker(ctx context.Context, state *config.State) error {
	if p.mocked {
		return p.mockedCheck("Checking Docker installation", "Docker version: 27.4.1 (mocked)")
	}

	var version string
	err := ui.WithSpinner("Checking Docker installation", func() error {
		out, cmdErr := runCmd(ctx, "docker", "--version")
		if cmdErr != nil {
			return cmdErr
		}
		ver, parseErr := ParseDockerVersion(out)
		if parseErr != nil {
			return parseErr
		}
		version = ver
		return nil
	})

	if err != nil {
		// Docker not found — offer to install
		ui.Warn("Docker is not installed")
		install, _ := ui.Confirm("Install Docker Engine?", true)
		if !install {
			return fmt.Errorf("docker is required but not installed")
		}
		if installErr := installDocker(ctx, state.Distro, state.UseSudo); installErr != nil {
			return fmt.Errorf("docker installation failed: %w", installErr)
		}
		// Re-detect sudo after Docker install
		if docker.DetectSudo(ctx) {
			state.UseSudo = true
		}
		return nil
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
		return nil
	}

	var driverVer string
	err := ui.WithSpinner("Checking NVIDIA driver", func() error {
		out, cmdErr := runCmd(ctx, "nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
		if cmdErr != nil {
			return cmdErr
		}
		driverVer = strings.TrimSpace(strings.Split(strings.TrimSpace(out), "\n")[0])
		return nil
	})

	if err != nil {
		// NVIDIA driver not found — offer to install
		ui.Warn("NVIDIA driver not detected (nvidia-smi not found)")
		install, _ := ui.Confirm("Install "+nvidiaDriver+"?", true)
		if !install {
			return fmt.Errorf("nvidia driver is required but not installed")
		}
		if installErr := installNVIDIADriver(ctx, state.Distro, state.UseSudo); installErr != nil {
			return installErr
		}
		// Re-read driver version after install
		out, retryErr := runCmd(ctx, "nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
		if retryErr != nil {
			return fmt.Errorf("nvidia-smi still not available after install (reboot may be required): %w", retryErr)
		}
		driverVer = strings.TrimSpace(strings.Split(strings.TrimSpace(out), "\n")[0])
	}

	state.DriverInfo = config.DriverInfo{
		UserVersion: driverVer,
		Consistent:  true,
	}
	ui.Detail("NVIDIA driver: %s", driverVer)
	return nil
}

func (p *Prerequisites) checkDriverConsistency(ctx context.Context, state *config.State) {
	if state.DriverInfo.UserVersion == "" {
		return
	}

	// Check kernel module version
	out, err := runCmd(ctx, "modinfo", "nvidia")
	if err == nil {
		state.DriverInfo.KernelVersion = ParseModinfoVersion(out)
	}

	// Check Fabric Manager version (only relevant if installed)
	out, _ = runCmd(ctx, "dpkg", "-l", "nvidia-fabricmanager-*")
	fmVer := ParseFabricManagerVersion(out)
	if fmVer != "" {
		state.DriverInfo.FMVersion = fmVer
	}

	// Compare versions
	userMajor := DriverMajorVersion(state.DriverInfo.UserVersion)
	consistent := true

	if state.DriverInfo.KernelVersion != "" && state.DriverInfo.KernelVersion != state.DriverInfo.UserVersion {
		ui.Warn("Driver version mismatch: userspace=%s, kernel module=%s",
			state.DriverInfo.UserVersion, state.DriverInfo.KernelVersion)
		ui.Detail("This can cause GPU errors. Fix: sudo apt-get install --reinstall %s", nvidiaDriver)
		consistent = false
	}

	if state.DriverInfo.FMVersion != "" {
		fmMajor := DriverMajorVersion(state.DriverInfo.FMVersion)
		if fmMajor != userMajor {
			ui.Warn("Fabric Manager version mismatch: driver=%s, FM=%s",
				state.DriverInfo.UserVersion, state.DriverInfo.FMVersion)
			ui.Detail("Fix: sudo apt-get install nvidia-fabricmanager-%s", userMajor)
			consistent = false
		}
	}

	state.DriverInfo.Consistent = consistent
	if consistent {
		if state.DriverInfo.KernelVersion != "" {
			ui.Success("Driver versions consistent (user: %s, kernel: %s)",
				state.DriverInfo.UserVersion, state.DriverInfo.KernelVersion)
		}
	}
}

func (p *Prerequisites) checkContainerToolkit(ctx context.Context, state *config.State) error {
	if p.mocked {
		return p.mockedCheck("Checking NVIDIA Container Toolkit", "nvidia-container-toolkit: 1.17.4 (mocked)")
	}

	var version string
	err := ui.WithSpinner("Checking NVIDIA Container Toolkit", func() error {
		out, cmdErr := runCmd(ctx, "nvidia-ctk", "--version")
		if cmdErr != nil {
			return cmdErr
		}
		version = strings.TrimSpace(out)
		return nil
	})

	if err != nil {
		// Container Toolkit not found — offer to install
		ui.Warn("NVIDIA Container Toolkit not detected")
		install, _ := ui.Confirm("Install NVIDIA Container Toolkit?", true)
		if !install {
			ui.Warn("Without Container Toolkit, GPUs won't be available inside Docker containers")
			return nil
		}
		if installErr := installContainerToolkit(ctx, state.Distro, state.UseSudo); installErr != nil {
			return fmt.Errorf("container toolkit installation failed: %w", installErr)
		}
		return nil
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
			return fmt.Errorf("cuda not available in Docker container: %w\n%s", cmdErr, string(out))
		}
		return nil
	})
	if err != nil {
		return err
	}
	ui.Success("CUDA available inside Docker (nvidia-smi works in container)")
	return nil
}

func (p *Prerequisites) checkFabricManager(ctx context.Context, state *config.State) {
	// Only relevant for multi-GPU setups
	out, err := runCmd(ctx, "nvidia-smi", "-L")
	if err != nil {
		return
	}
	gpuCount := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) != "" {
			gpuCount++
		}
	}
	if gpuCount <= 1 {
		return
	}

	// Check if FM is already running
	_, err = runCmd(ctx, "systemctl", "is-active", "nvidia-fabricmanager")
	if err == nil {
		ui.Detail("Fabric Manager: running (%d GPUs detected)", gpuCount)
		return
	}

	// Check if FM is installed but not running
	fmOut, _ := runCmd(ctx, "dpkg", "-l", "nvidia-fabricmanager-*")
	fmVer := ParseFabricManagerVersion(fmOut)
	if fmVer != "" {
		// Installed but not running — start it
		ui.Warn("Fabric Manager installed but not running (%d GPUs detected)", gpuCount)
		_, _ = runSudoCmd(ctx, state.UseSudo, "systemctl", "enable", "--now", "nvidia-fabricmanager")
		return
	}

	// Not installed — offer to install
	ui.Warn("Multiple GPUs detected (%d) but Fabric Manager is not installed", gpuCount)
	ui.Detail("Fabric Manager is required for NVLink multi-GPU communication")
	install, _ := ui.Confirm("Install Fabric Manager?", true)
	if !install {
		return
	}
	if installErr := installFabricManager(ctx, state.DriverInfo.UserVersion, state.UseSudo); installErr != nil {
		ui.Warn("Fabric Manager installation failed: %v", installErr)
	}
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

const minUnmountedDriveGB = 500

func (p *Prerequisites) checkStorageLayout(ctx context.Context, state *config.State) {
	if p.mocked {
		state.DiskFreeGB = 3500
		ui.Success("Storage: /dev/nvme1n1 (3.5 TB) mounted at %s", state.OutputDir)
		return
	}

	ui.Header("Storage Validation")

	// 1. Get disk free space on deploy dir
	freeGB := p.getDiskFreeGB(ctx, state.OutputDir)
	state.DiskFreeGB = freeGB

	// 2. Check which device backs the deploy dir and root
	deploySource := p.getDeviceForPath(ctx, state.OutputDir)
	rootSource := p.getDeviceForPath(ctx, "/")
	onRoot := deploySource != "" && rootSource != "" && deploySource == rootSource

	// 3. Get block device layout
	devices, lsblkErr := p.getLsblkDevices(ctx)
	if lsblkErr != nil {
		// Can't detect storage layout — fall back to simple disk space check
		p.reportDiskSpace(state, freeGB)
		return
	}

	// 4. Find unmounted drives
	unmounted := FindUnmountedDrives(devices, minUnmountedDriveGB)

	// 5. Show storage summary
	if deploySource != "" {
		ui.Detail("Deploy directory: %s (on %s, %d GB free)", state.OutputDir, deploySource, freeGB)
	}

	// 6. Decision logic
	if onRoot && len(unmounted) > 0 {
		ui.Warn("Deploy directory is on root filesystem (%d GB free). Blockchain data can grow to 500GB+.", freeGB)
		ui.Info("Found %d unmounted drive(s) that could be used:", len(unmounted))
		for i, d := range unmounted {
			ui.Detail("  [%d] /dev/%s (%s, unmounted)", i+1, d.Name, FormatDriveSize(d.Size))
		}

		// In non-interactive mode, warn but don't offer to format
		if ui.IsNonInteractive() {
			ui.Warn("Deploy directory is on root filesystem. Use --output-dir on a dedicated mount for production.")
			return
		}

		// Offer to mount
		if err := p.offerMountDrive(ctx, state, unmounted); err != nil {
			ui.Warn("Drive mount failed: %v", err)
		} else {
			// Re-check disk space after mount
			freeGB = p.getDiskFreeGB(ctx, state.OutputDir)
			state.DiskFreeGB = freeGB
		}
	} else if onRoot {
		// On root, but no unmounted drives — just warn if space is low
		p.reportDiskSpace(state, freeGB)
	} else {
		// Deploy dir is on a separate mount — good
		p.reportDiskSpace(state, freeGB)
	}
}

func (p *Prerequisites) offerMountDrive(ctx context.Context, state *config.State, drives []BlockDevice) error {
	mount, err := ui.Confirm("Mount a drive to the deploy directory?", true)
	if err != nil || !mount {
		ui.Info("Continuing with current storage layout")
		return nil
	}

	// Select drive
	var selectedIdx int
	if len(drives) == 1 {
		selectedIdx = 0
	} else {
		options := make([]string, len(drives))
		for i, d := range drives {
			options[i] = fmt.Sprintf("/dev/%s (%s)", d.Name, FormatDriveSize(d.Size))
		}
		selected, selErr := ui.Select("Select drive to mount:", options)
		if selErr != nil {
			return selErr
		}
		for i, opt := range options {
			if opt == selected {
				selectedIdx = i
				break
			}
		}
	}

	drive := drives[selectedIdx]
	devPath := "/dev/" + drive.Name
	hasFstype := drive.Fstype != nil && *drive.Fstype != ""

	// Extra warning if drive has existing filesystem
	if hasFstype {
		ui.Warn("Drive %s has existing filesystem (%s)", devPath, *drive.Fstype)
	}

	// Double confirmation — must type 'yes'
	ui.Warn("This will FORMAT %s as ext4 — ALL DATA ON THIS DRIVE WILL BE LOST", devPath)
	typed, inputErr := ui.Input("Type 'yes' to confirm:", "")
	if inputErr != nil {
		return inputErr
	}
	if strings.ToLower(strings.TrimSpace(typed)) != "yes" {
		ui.Info("Skipping drive format")
		return nil
	}

	// Format
	sp := ui.NewSpinner(fmt.Sprintf("Formatting %s as ext4...", devPath))
	sp.Start()
	_, fmtErr := runSudoCmd(ctx, state.UseSudo, "mkfs.ext4", "-L", "gonka-data", devPath)
	if fmtErr != nil {
		sp.StopWithError("Format failed")
		return fmt.Errorf("mkfs.ext4 %s: %w", devPath, fmtErr)
	}
	sp.StopWithSuccess(fmt.Sprintf("Formatted %s as ext4", devPath))

	// Create mount point and mount
	_, _ = runSudoCmd(ctx, state.UseSudo, "mkdir", "-p", state.OutputDir)

	sp = ui.NewSpinner(fmt.Sprintf("Mounting %s at %s...", devPath, state.OutputDir))
	sp.Start()
	_, mountErr := runSudoCmd(ctx, state.UseSudo, "mount", devPath, state.OutputDir)
	if mountErr != nil {
		sp.StopWithError("Mount failed")
		return fmt.Errorf("mount %s: %w", devPath, mountErr)
	}
	sp.StopWithSuccess(fmt.Sprintf("Mounted %s at %s", devPath, state.OutputDir))

	// Add to fstab for persistence
	fstabLine := fmt.Sprintf("%s %s ext4 defaults 0 2\n", devPath, state.OutputDir)
	_, fstabErr := runSudoCmd(ctx, state.UseSudo, "sh", "-c",
		fmt.Sprintf("echo '%s' >> /etc/fstab", fstabLine))
	if fstabErr != nil {
		ui.Warn("Could not update /etc/fstab: %v — mount will not persist after reboot", fstabErr)
	} else {
		ui.Success("Added %s to /etc/fstab for persistence", devPath)
	}

	return nil
}

func (p *Prerequisites) getDiskFreeGB(ctx context.Context, path string) int {
	out, err := runCmd(ctx, "df", "--output=avail", "-BG", path)
	if err != nil {
		return 0
	}
	gb, parseErr := ParseDiskFreeGB(out)
	if parseErr != nil {
		return 0
	}
	return gb
}

func (p *Prerequisites) getDeviceForPath(ctx context.Context, path string) string {
	out, err := runCmd(ctx, "df", "--output=source", path)
	if err != nil {
		return ""
	}
	source, parseErr := ParseDfSource(out)
	if parseErr != nil {
		return ""
	}
	return source
}

func (p *Prerequisites) getLsblkDevices(ctx context.Context) ([]BlockDevice, error) {
	out, err := runCmd(ctx, "lsblk", "-J", "-b", "-o", "NAME,SIZE,TYPE,MOUNTPOINT,FSTYPE")
	if err != nil {
		return nil, fmt.Errorf("lsblk: %w", err)
	}
	return ParseLsblkJSON(out)
}

func (p *Prerequisites) reportDiskSpace(state *config.State, freeGB int) {
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
