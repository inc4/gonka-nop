package phases

import (
	"context"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// Prerequisites checks Docker and NVIDIA requirements
type Prerequisites struct{}

func NewPrerequisites() *Prerequisites {
	return &Prerequisites{}
}

func (p *Prerequisites) Name() string {
	return "Prerequisites"
}

func (p *Prerequisites) Description() string {
	return "Checking Docker and NVIDIA driver installation"
}

func (p *Prerequisites) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *Prerequisites) Run(ctx context.Context, state *config.State) error {
	// Check Docker
	err := ui.WithSpinner("Checking Docker installation", func() error {
		time.Sleep(500 * time.Millisecond) // Simulated check
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Docker version: 24.0.7 (mocked)")

	// Check NVIDIA driver
	err = ui.WithSpinner("Checking NVIDIA driver", func() error {
		time.Sleep(500 * time.Millisecond) // Simulated check
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("NVIDIA driver: 535.154.05 (mocked)")

	// Check NVIDIA Container Toolkit
	err = ui.WithSpinner("Checking NVIDIA Container Toolkit", func() error {
		time.Sleep(500 * time.Millisecond) // Simulated check
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("nvidia-container-toolkit: 1.14.3 (mocked)")

	return nil
}
