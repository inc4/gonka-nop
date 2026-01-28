package phases

import (
	"context"
	"fmt"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// Phase defines the interface for setup phases
type Phase interface {
	// Name returns the phase name (for display and state tracking)
	Name() string

	// Description returns a short description of what the phase does
	Description() string

	// Run executes the phase
	Run(ctx context.Context, state *config.State) error

	// ShouldRun returns true if this phase should run given current state
	ShouldRun(state *config.State) bool
}

// Runner executes phases in sequence
type Runner struct {
	phases []Phase
	state  *config.State
}

// NewRunner creates a new phase runner
func NewRunner(phases []Phase, state *config.State) *Runner {
	return &Runner{
		phases: phases,
		state:  state,
	}
}

// Run executes all phases in order
func (r *Runner) Run(ctx context.Context) error {
	total := len(r.phases)

	for i, phase := range r.phases {
		// Check if phase should run
		if !phase.ShouldRun(r.state) {
			ui.Detail("Skipping %s (already complete)", phase.Name())
			continue
		}

		// Display phase header
		ui.PhaseStart(i+1, phase.Name())
		ui.Detail(phase.Description())

		// Update state
		r.state.CurrentPhase = phase.Name()
		if err := r.state.Save(); err != nil {
			ui.Warn("Failed to save state: %v", err)
		}

		// Run the phase
		if err := phase.Run(ctx, r.state); err != nil {
			ui.PhaseFailed(phase.Name(), err)
			return fmt.Errorf("phase %s failed: %w", phase.Name(), err)
		}

		// Mark complete
		r.state.MarkPhaseComplete(phase.Name())
		if err := r.state.Save(); err != nil {
			ui.Warn("Failed to save state: %v", err)
		}

		ui.PhaseComplete(phase.Name())

		// Show progress
		ui.Detail("Progress: %d/%d phases complete", i+1, total)
	}

	return nil
}

// GetState returns the current state
func (r *Runner) GetState() *config.State {
	return r.state
}
