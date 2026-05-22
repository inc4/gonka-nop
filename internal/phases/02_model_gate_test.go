package phases

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// TestPhase02_KimiOnNonBlackwell_FastFails covers TC-1.6 / AC-M11-2:
// when --model=moonshotai/Kimi-K2.6 is set and phase 02 detects a
// non-Blackwell GPU (mocked default = 4x RTX 4090 sm_89), the phase
// MUST return an error mentioning both the observed arch and the
// required Blackwell arch, in under 5 seconds, without writing files.
func TestPhase02_KimiOnNonBlackwell_FastFails(t *testing.T) {
	phase := NewGPUDetection(true) // mocked sm_89 GPUs

	state := &config.State{
		OutputDir:     t.TempDir(),
		SelectedModel: kimiModelID, // simulates `--model` flag
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := phase.Run(ctx, state)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected phase 02 to return an error for Kimi on sm_89")
	}
	if !strings.Contains(err.Error(), "sm_89") {
		t.Errorf("error should mention observed arch sm_89: %v", err)
	}
	if !strings.Contains(err.Error(), "sm_100") {
		t.Errorf("error should mention required Blackwell arch sm_100: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("phase 02 took %s, want <5s (TC-1.6)", elapsed)
	}

	// Check SelectedModel is preserved (Review Notes F5).
	if state.SelectedModel != kimiModelID {
		t.Errorf("state.SelectedModel mutated on error: got %q, want %q", state.SelectedModel, kimiModelID)
	}
}

// TestPhase02_HonorsPreSetSelectedModel covers Review Notes F5: when the
// caller pre-populates state.SelectedModel (e.g. from --model flag), phase
// 02's recommender result MUST NOT overwrite that value. Mocked detection
// returns Qwen3-235B as the recommended model for 4x RTX 4090, but the
// pre-set value should survive.
//
// Note: for the test to PASS the recipe gate must accept Qwen3-235B on
// sm_89 — the legacy recipe has no GPUArchAllowed restriction, so this works.
func TestPhase02_HonorsPreSetSelectedModel(t *testing.T) {
	// Non-interactive mode + prompt overrides — phase 02 emits an MLNode-image
	// Input prompt and Attention-backend Select after the recommender succeeds.
	ui.SetNonInteractive(true)
	t.Cleanup(func() { ui.SetNonInteractive(false) })
	ui.SetOverride("MLNode image", "")
	ui.SetOverride("Attention backend", "FLASHINFER (default, recommended)")

	phase := NewGPUDetection(true)
	state := &config.State{
		OutputDir:     t.TempDir(),
		SelectedModel: qwqModelID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := phase.Run(ctx, state); err != nil {
		t.Fatalf("unexpected error from phase 02 with pre-set SelectedModel: %v", err)
	}

	if state.SelectedModel != qwqModelID {
		t.Errorf("phase 02 overwrote pre-set SelectedModel: got %q, want %q",
			state.SelectedModel, qwqModelID)
	}
}

// Note: legacy "empty SelectedModel falls back to tier default" is covered by
// TestRecommendConfig (legacy table) + TestRecommendConfig_EmptyModelFallsBackToTier.
// Exercising it here would require driving the interactive MLNode-image prompt
// that phase 02 emits after the recommender succeeds.
