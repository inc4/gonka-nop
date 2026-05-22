package chain

import (
	"context"
	"errors"
	"testing"
	"time"
)

// realisticMainnetEpochInfo is the actual `inferenced query inference
// epoch-info -o json` response captured from mainnet-1 (gonka-mainnet,
// epoch 265, block 4102021) on 2026-05-16. Used as the table-driven
// reference; only fields the derivation cares about are populated.
const realisticMainnetEpochInfo = `{
  "block_height": "4102021",
  "params": {
    "epoch_params": {
      "epoch_length": "15391",
      "poc_stage_duration": "35",
      "poc_validation_delay": "5",
      "poc_validation_duration": "240",
      "confirmation_poc_safety_window": "500"
    }
  },
  "latest_epoch": {
    "index": "265",
    "poc_start_block_height": "4089970"
  }
}`

func TestDerivePhase_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		blockHeight string
		pocStart    string
		wantPhase   PoCPhase
		wantSafe    bool
	}{
		{
			name:        "Inference (mainnet snapshot 4102021)",
			blockHeight: "4102021",
			pocStart:    "4089970",
			wantPhase:   PhaseInference,
			wantSafe:    true,
		},
		{
			name:        "PoCGenerate (offset 10 of 35)",
			blockHeight: "4089980",
			pocStart:    "4089970",
			wantPhase:   PhasePoCGenerate,
			wantSafe:    false,
		},
		{
			name:        "PoCGenerate boundary (offset 34)",
			blockHeight: "4090004",
			pocStart:    "4089970",
			wantPhase:   PhasePoCGenerate,
			wantSafe:    false,
		},
		{
			name:        "PoC delay (offset 37 of 35+5)",
			blockHeight: "4090007",
			pocStart:    "4089970",
			wantPhase:   PhasePoCGenerate, // delay window treated as unsafe under PoCGenerate
			wantSafe:    false,
		},
		{
			name:        "PoCValidate (offset 100 of 35+5+240)",
			blockHeight: "4090070",
			pocStart:    "4089970",
			wantPhase:   PhasePoCValidate,
			wantSafe:    false,
		},
		{
			name:        "Inference start (offset 280, just past validation)",
			blockHeight: "4090250",
			pocStart:    "4089970",
			wantPhase:   PhaseInference,
			wantSafe:    true,
		},
		{
			name:        "CPoC window (last 500 blocks before next epoch)",
			blockHeight: "4105100", // pocStart + epochLength - 261 = within CPoC window
			pocStart:    "4089970",
			wantPhase:   PhaseConfirmationPoC,
			wantSafe:    false,
		},
		{
			name:        "Out-of-window (block before pocStart)",
			blockHeight: "4089969",
			pocStart:    "4089970",
			wantPhase:   PhaseUnknown,
			wantSafe:    false,
		},
		{
			name:        "Out-of-window (block past epoch end)",
			blockHeight: "4200000",
			pocStart:    "4089970",
			wantPhase:   PhaseUnknown,
			wantSafe:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Synthesize JSON with the test's block + pocStart, keep mainnet params.
			j := `{
				"block_height": "` + tt.blockHeight + `",
				"params": {"epoch_params": {
					"epoch_length": "15391",
					"poc_stage_duration": "35",
					"poc_validation_delay": "5",
					"poc_validation_duration": "240",
					"confirmation_poc_safety_window": "500"
				}},
				"latest_epoch": {"index": "265", "poc_start_block_height": "` + tt.pocStart + `"}
			}`
			c, err := derivePhase([]byte(j))
			if err != nil {
				t.Fatalf("derivePhase: %v", err)
			}
			if c.Phase != tt.wantPhase {
				t.Errorf("Phase = %s, want %s", c.Phase, tt.wantPhase)
			}
			if c.Safe != tt.wantSafe {
				t.Errorf("Safe = %v, want %v", c.Safe, tt.wantSafe)
			}
		})
	}
}

func TestDerivePhase_MalformedJSON(t *testing.T) {
	_, err := derivePhase([]byte("not json"))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDerivePhase_NonNumericFields(t *testing.T) {
	j := `{"block_height": "not-a-number", "params": {"epoch_params": {"epoch_length": "1"}}, "latest_epoch": {}}`
	_, err := derivePhase([]byte(j))
	if err == nil {
		t.Fatal("expected error for non-numeric block_height")
	}
}

func TestDerivePhase_RealisticMainnetSnapshot(t *testing.T) {
	c, err := derivePhase([]byte(realisticMainnetEpochInfo))
	if err != nil {
		t.Fatalf("derivePhase: %v", err)
	}
	if c.Phase != PhaseInference {
		t.Errorf("Phase = %s, want Inference (mainnet was at offset 12051 of 15391 → safe)", c.Phase)
	}
	if !c.Safe {
		t.Errorf("Safe = false, want true at the captured mainnet snapshot")
	}
	if c.CurrentBlock != 4102021 {
		t.Errorf("CurrentBlock = %d, want 4102021", c.CurrentBlock)
	}
}

// fakeRunner is a QueryRunner stub for Check() tests.
type fakeRunner struct {
	out  []byte
	err  error
	wait time.Duration
}

func (f *fakeRunner) Query(ctx context.Context, _ ...string) ([]byte, error) {
	if f.wait > 0 {
		select {
		case <-time.After(f.wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.out, f.err
}

func TestPoCSafetyChecker_HappyPath(t *testing.T) {
	c := &PoCSafetyChecker{
		Runner:  &fakeRunner{out: []byte(realisticMainnetEpochInfo)},
		Timeout: PoCCheckTimeout,
	}
	check, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !check.Safe || check.Phase != PhaseInference {
		t.Errorf("got Phase=%s Safe=%v, want Inference + safe", check.Phase, check.Safe)
	}
}

func TestPoCSafetyChecker_RunnerError_FailsClosed(t *testing.T) {
	c := &PoCSafetyChecker{
		Runner:  &fakeRunner{err: errors.New("docker exec failed")},
		Timeout: PoCCheckTimeout,
	}
	check, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check should not return an error on fail-closed; got %v", err)
	}
	if check.Safe {
		t.Error("expected Safe=false on runner error (fail-closed)")
	}
	if check.Phase != PhaseUnknown {
		t.Errorf("Phase = %s, want Unknown", check.Phase)
	}
}

func TestPoCSafetyChecker_TimeoutFailsClosed(t *testing.T) {
	c := &PoCSafetyChecker{
		Runner:  &fakeRunner{wait: 3 * time.Second, out: []byte(realisticMainnetEpochInfo)},
		Timeout: 500 * time.Millisecond, // shorter than fake's wait
	}
	start := time.Now()
	check, err := c.Check(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Check should not return error on fail-closed timeout; got %v", err)
	}
	if check.Safe {
		t.Error("expected Safe=false on timeout (fail-closed)")
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("Check took %s, want <1.5s (timeout was 500ms)", elapsed)
	}
}

func TestPoCSafetyChecker_MalformedQueryOutput_FailsClosed(t *testing.T) {
	c := &PoCSafetyChecker{
		Runner:  &fakeRunner{out: []byte("not json")},
		Timeout: PoCCheckTimeout,
	}
	check, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check should not return error on fail-closed; got %v", err)
	}
	if check.Safe {
		t.Error("expected Safe=false on parse error")
	}
}

func TestPoCSafetyChecker_DefaultTimeoutAppliedWhenZero(t *testing.T) {
	c := &PoCSafetyChecker{
		Runner:  &fakeRunner{out: []byte(realisticMainnetEpochInfo)},
		Timeout: 0,
	}
	check, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !check.Safe {
		t.Errorf("happy-path should be Safe; got Phase=%s", check.Phase)
	}
}
