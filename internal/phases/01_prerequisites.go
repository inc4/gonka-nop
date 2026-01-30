package phases

import (
	"context"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// Prerequisites checks Docker, NVIDIA, disk, and system requirements
type Prerequisites struct{}

func NewPrerequisites() *Prerequisites {
	return &Prerequisites{}
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

func (p *Prerequisites) Run(_ context.Context, state *config.State) error {
	// Check Docker
	err := ui.WithSpinner("Checking Docker installation", func() error {
		time.Sleep(500 * time.Millisecond) // Simulated check
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Docker version: 27.4.1 (mocked)")

	// Check Docker Compose v2
	err = ui.WithSpinner("Checking Docker Compose", func() error {
		time.Sleep(300 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Docker Compose version: v2.32.4 (mocked)")

	// Check NVIDIA driver
	err = ui.WithSpinner("Checking NVIDIA driver", func() error {
		time.Sleep(500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("NVIDIA driver: 570.133.20 (mocked)")

	// Check NVIDIA driver version consistency
	err = ui.WithSpinner("Verifying NVIDIA driver consistency", func() error {
		time.Sleep(400 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	state.DriverInfo = config.DriverInfo{
		UserVersion:   "570.133.20",
		KernelVersion: "570.133.20",
		FMVersion:     "570.133.20",
		Consistent:    true,
	}
	if state.DriverInfo.Consistent {
		ui.Success("Driver versions consistent (user: %s, kernel: %s, FM: %s)",
			state.DriverInfo.UserVersion, state.DriverInfo.KernelVersion, state.DriverInfo.FMVersion)
	} else {
		ui.Error("Driver version MISMATCH — user: %s, kernel: %s, FM: %s",
			state.DriverInfo.UserVersion, state.DriverInfo.KernelVersion, state.DriverInfo.FMVersion)
		ui.Warn("Mismatched NVIDIA driver versions cause GPU failures. Fix before proceeding.")
	}

	// Check NVIDIA Container Toolkit
	err = ui.WithSpinner("Checking NVIDIA Container Toolkit", func() error {
		time.Sleep(500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("nvidia-container-toolkit: 1.17.4 (mocked)")

	// Check CUDA inside Docker container
	err = ui.WithSpinner("Verifying CUDA inside Docker container", func() error {
		time.Sleep(600 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Success("CUDA available inside Docker (nvidia-smi works in container)")

	// Check for unattended-upgrades
	err = ui.WithSpinner("Checking for auto-update packages", func() error {
		time.Sleep(300 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	// Mocked: unattended-upgrades is NOT installed (good)
	state.AutoUpdateOff = true
	ui.Success("No auto-update packages detected that could break NVIDIA drivers")

	// Check disk space
	err = ui.WithSpinner("Checking available disk space", func() error {
		time.Sleep(300 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	state.DiskFreeGB = 512 // Mocked
	if state.DiskFreeGB >= 250 {
		ui.Success("Disk space: %d GB free (250 GB minimum for Cosmovisor upgrades)", state.DiskFreeGB)
	} else {
		ui.Warn("Disk space: %d GB free — recommended minimum is 250 GB for Cosmovisor backups", state.DiskFreeGB)
	}

	// Check port availability
	err = ui.WithSpinner("Checking port availability", func() error {
		time.Sleep(400 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}

	ports := []struct {
		port int
		name string
	}{
		{5000, "P2P"},
		{8000, "API"},
		{26657, "RPC"},
		{5050, "ML Inference"},
		{8080, "PoC"},
		{9100, "API ML Callback"},
		{9200, "Admin API"},
	}
	for _, p := range ports {
		ui.Detail("Port %d (%s): available (mocked)", p.port, p.name)
	}

	ui.Success("All prerequisites satisfied")
	return nil
}
