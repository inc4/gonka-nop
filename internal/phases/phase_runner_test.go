package phases

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

// mockPhase implements the Phase interface for testing
type mockPhase struct {
	name      string
	desc      string
	shouldRun bool
	runErr    error
	ran       bool
}

func (m *mockPhase) Name() string        { return m.name }
func (m *mockPhase) Description() string { return m.desc }
func (m *mockPhase) ShouldRun(_ *config.State) bool {
	return m.shouldRun
}
func (m *mockPhase) Run(_ context.Context, _ *config.State) error {
	m.ran = true
	return m.runErr
}

func newMock(name string, shouldRun bool, runErr error) *mockPhase {
	return &mockPhase{
		name:      name,
		desc:      "Test phase: " + name,
		shouldRun: shouldRun,
		runErr:    runErr,
	}
}

func TestRunnerAllPhasesRun(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	p1 := newMock("Phase1", true, nil)
	p2 := newMock("Phase2", true, nil)
	p3 := newMock("Phase3", true, nil)

	state := config.NewState(tmpDir)
	runner := NewRunner([]Phase{p1, p2, p3}, state)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run() error: %v", err)
	}

	if !p1.ran {
		t.Error("Phase1 was not executed")
	}
	if !p2.ran {
		t.Error("Phase2 was not executed")
	}
	if !p3.ran {
		t.Error("Phase3 was not executed")
	}
}

func TestRunnerSkipsCompleted(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	p1 := newMock("Phase1", true, nil)
	p2 := newMock("Phase2", false, nil) // ShouldRun=false
	p3 := newMock("Phase3", true, nil)

	state := config.NewState(tmpDir)
	runner := NewRunner([]Phase{p1, p2, p3}, state)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run() error: %v", err)
	}

	if !p1.ran {
		t.Error("Phase1 should have been executed")
	}
	if p2.ran {
		t.Error("Phase2 should have been skipped (ShouldRun=false)")
	}
	if !p3.ran {
		t.Error("Phase3 should have been executed")
	}
}

func TestRunnerStopsOnError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	testErr := errors.New("phase2 failed")
	p1 := newMock("Phase1", true, nil)
	p2 := newMock("Phase2", true, testErr)
	p3 := newMock("Phase3", true, nil)

	state := config.NewState(tmpDir)
	runner := NewRunner([]Phase{p1, p2, p3}, state)

	err = runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from runner.Run(), got nil")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("expected wrapped testErr, got: %v", err)
	}

	if !p1.ran {
		t.Error("Phase1 should have been executed")
	}
	if !p2.ran {
		t.Error("Phase2 should have been executed (and failed)")
	}
	if p3.ran {
		t.Error("Phase3 should NOT have been executed after Phase2 error")
	}
}

func TestRunnerMarksComplete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	p1 := newMock("Phase1", true, nil)
	p2 := newMock("Phase2", true, nil)

	state := config.NewState(tmpDir)
	runner := NewRunner([]Phase{p1, p2}, state)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run() error: %v", err)
	}

	if !state.IsPhaseComplete("Phase1") {
		t.Error("Phase1 should be marked complete in state")
	}
	if !state.IsPhaseComplete("Phase2") {
		t.Error("Phase2 should be marked complete in state")
	}
	if len(state.CompletedPhases) != 2 {
		t.Errorf("expected 2 completed phases, got %d", len(state.CompletedPhases))
	}
}

func TestRunnerSavesState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	p1 := newMock("Phase1", true, nil)

	state := config.NewState(tmpDir)
	runner := NewRunner([]Phase{p1}, state)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run() error: %v", err)
	}

	// State file should exist after run
	statePath := tmpDir + "/state.json"
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("state.json was not created after runner.Run()")
	}

	// Load and verify
	loaded, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if !loaded.IsPhaseComplete("Phase1") {
		t.Error("loaded state does not have Phase1 as complete")
	}
}

func TestRunnerGetState(t *testing.T) {
	state := config.NewState("/tmp/test")
	runner := NewRunner(nil, state)

	got := runner.GetState()
	if got != state {
		t.Error("GetState() should return the same state pointer")
	}
}

func TestRunnerEmptyPhases(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	runner := NewRunner([]Phase{}, state)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run() with empty phases should succeed, got: %v", err)
	}
}

func TestRunnerErrorNotMarkedComplete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	testErr := errors.New("boom")
	p1 := newMock("FailPhase", true, testErr)

	state := config.NewState(tmpDir)
	runner := NewRunner([]Phase{p1}, state)

	_ = runner.Run(context.Background())

	if state.IsPhaseComplete("FailPhase") {
		t.Error("failed phase should NOT be marked as complete")
	}
}
