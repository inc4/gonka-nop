package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
)

// PoCPhase enumerates the coarse-grained chain PoC phases.
// The phase is derived from `inferenced query inference epoch-info` output
// (block_height vs latest_epoch.poc_start_block_height + epoch_params windows).
type PoCPhase string

const (
	PhaseInference       PoCPhase = "Inference"
	PhasePoCGenerate     PoCPhase = "PoCGenerate"
	PhasePoCValidate     PoCPhase = "PoCValidate"
	PhaseConfirmationPoC PoCPhase = "ConfirmationPoC"
	PhaseUnknown         PoCPhase = "Unknown"
)

// PoCCheckTimeout caps the chain query (NFR-M11-4: <2 s under normal conditions).
const PoCCheckTimeout = 2 * time.Second

// SafetyCheck is the result of a PoC-phase safety query.
type SafetyCheck struct {
	Safe             bool
	Phase            PoCPhase
	CurrentBlock     int64
	NextPhaseBlock   int64 // block height at which Phase transitions next
	SecondsUntilSafe int   // estimate based on ~5s block time; 0 when Safe==true
	Reason           string
}

// QueryRunner abstracts the chain query call so tests can inject a fake
// (httptest is not the right seam here — we shell out to `docker exec`, not HTTP).
type QueryRunner interface {
	// Query runs a query command against the chain via the api container
	// and returns raw stdout. Implementations honor the context's deadline.
	Query(ctx context.Context, args ...string) ([]byte, error)
}

// PoCSafetyChecker queries chain state and returns whether disruptive
// operations (api hotfix install, ml-node update, govmodel tx) are safe.
//
// Fail-closed contract (NFR-M11-4): any error from the underlying query
// — context timeout, command failure, JSON parse error, missing fields —
// produces SafetyCheck{Safe: false, Phase: PhaseUnknown, Reason: <err>}.
// Callers MUST honor this; --force flags should be the only bypass.
type PoCSafetyChecker struct {
	State   *config.State
	Runner  QueryRunner   // injectable for tests
	Timeout time.Duration // defaults to PoCCheckTimeout when zero
}

// NewPoCSafetyChecker constructs a checker that queries via `docker exec api
// inferenced query inference epoch-info`. The state is used for sudo and
// working-directory resolution.
func NewPoCSafetyChecker(state *config.State) *PoCSafetyChecker {
	return &PoCSafetyChecker{
		State:   state,
		Runner:  &dockerExecQueryRunner{state: state},
		Timeout: PoCCheckTimeout,
	}
}

// Check runs the safety query and returns the result.
// On any error, returns SafetyCheck with Safe=false and Phase=PhaseUnknown.
func (c *PoCSafetyChecker) Check(ctx context.Context) (*SafetyCheck, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = PoCCheckTimeout
	}
	qCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := c.Runner.Query(qCtx, "inferenced", "query", "inference", "epoch-info",
		"--node", "http://node:26657", "-o", "json")
	if err != nil {
		return &SafetyCheck{
			Safe:   false,
			Phase:  PhaseUnknown,
			Reason: fmt.Sprintf("chain query failed: %v", err),
		}, nil
	}

	check, err := derivePhase(out)
	if err != nil {
		return &SafetyCheck{
			Safe:   false,
			Phase:  PhaseUnknown,
			Reason: fmt.Sprintf("phase derivation failed: %v", err),
		}, nil
	}
	return check, nil
}

// epochInfoResponse mirrors the relevant subset of `inferenced query
// inference epoch-info -o json` output. Fields are strings on the wire
// (cosmos-sdk uint64 JSON convention).
type epochInfoResponse struct {
	BlockHeight string `json:"block_height"`
	Params      struct {
		EpochParams struct {
			EpochLength                 string `json:"epoch_length"`
			PoCStageDuration            string `json:"poc_stage_duration"`
			PoCValidationDelay          string `json:"poc_validation_delay"`
			PoCValidationDuration       string `json:"poc_validation_duration"`
			ConfirmationPoCSafetyWindow string `json:"confirmation_poc_safety_window"`
		} `json:"epoch_params"`
	} `json:"params"`
	LatestEpoch struct {
		Index               string `json:"index"`
		PoCStartBlockHeight string `json:"poc_start_block_height"`
	} `json:"latest_epoch"`
}

// derivePhase parses the epoch-info JSON and computes the current phase.
// Mainnet schema verified 2026-05-16 against node1 (chain-id gonka-mainnet,
// epoch 265, block 4102021). Schema is stable per the v0.2.x line; any
// new top-level fields are ignored (forward-compat).
func derivePhase(jsonBytes []byte) (*SafetyCheck, error) {
	var resp epochInfoResponse
	if err := json.Unmarshal(jsonBytes, &resp); err != nil {
		return nil, fmt.Errorf("parse epoch-info: %w", err)
	}

	blockHeight, err := strconv.ParseInt(resp.BlockHeight, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("block_height %q: %w", resp.BlockHeight, err)
	}
	pocStart, err := strconv.ParseInt(resp.LatestEpoch.PoCStartBlockHeight, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("latest_epoch.poc_start_block_height %q: %w",
			resp.LatestEpoch.PoCStartBlockHeight, err)
	}
	epochLength, err := strconv.ParseInt(resp.Params.EpochParams.EpochLength, 10, 64)
	if err != nil || epochLength <= 0 {
		return nil, fmt.Errorf("epoch_length %q: %w", resp.Params.EpochParams.EpochLength, err)
	}
	pocStage, _ := strconv.ParseInt(resp.Params.EpochParams.PoCStageDuration, 10, 64)
	pocDelay, _ := strconv.ParseInt(resp.Params.EpochParams.PoCValidationDelay, 10, 64)
	pocValDur, _ := strconv.ParseInt(resp.Params.EpochParams.PoCValidationDuration, 10, 64)
	cpocWindow, _ := strconv.ParseInt(resp.Params.EpochParams.ConfirmationPoCSafetyWindow, 10, 64)

	offset := blockHeight - pocStart
	if offset < 0 || offset >= epochLength {
		return &SafetyCheck{
			Safe:         false,
			Phase:        PhaseUnknown,
			CurrentBlock: blockHeight,
			Reason: fmt.Sprintf("block %d is outside expected epoch window [%d, %d); cannot derive phase",
				blockHeight, pocStart, pocStart+epochLength),
		}, nil
	}

	var phase PoCPhase
	var nextPhase int64
	switch {
	case offset < pocStage:
		phase = PhasePoCGenerate
		nextPhase = pocStart + pocStage
	case offset < pocStage+pocDelay:
		// Brief inter-stage gap; still treat as unsafe (no PoC traffic yet,
		// but operators should not touch ml-nodes / api binary across
		// the boundary).
		phase = PhasePoCGenerate
		nextPhase = pocStart + pocStage + pocDelay
	case offset < pocStage+pocDelay+pocValDur:
		phase = PhasePoCValidate
		nextPhase = pocStart + pocStage + pocDelay + pocValDur
	case offset >= epochLength-cpocWindow:
		phase = PhaseConfirmationPoC
		nextPhase = pocStart + epochLength // = next epoch's PoC start
	default:
		phase = PhaseInference
		nextPhase = pocStart + epochLength - cpocWindow
	}

	safe := phase == PhaseInference
	blocksUntilSafe := int64(0)
	if !safe {
		// Time until end of current unsafe window.
		blocksUntilSafe = nextPhase - blockHeight
		if phase != PhaseInference {
			// We're unsafe; nextPhase is the END of unsafe window, hence safety arrives there.
		}
	}
	// ~5s per block on mainnet
	secondsUntilSafe := int(blocksUntilSafe * 5)
	if secondsUntilSafe < 0 {
		secondsUntilSafe = 0
	}

	return &SafetyCheck{
		Safe:             safe,
		Phase:            phase,
		CurrentBlock:     blockHeight,
		NextPhaseBlock:   nextPhase,
		SecondsUntilSafe: secondsUntilSafe,
		Reason:           reasonFor(phase, blockHeight, nextPhase),
	}, nil
}

func reasonFor(phase PoCPhase, current, next int64) string {
	switch phase {
	case PhaseInference:
		return fmt.Sprintf("Inference phase, safe until block %d (~CPoC window)", next)
	case PhasePoCGenerate:
		return fmt.Sprintf("PoC Generate in progress (block %d → ends ~%d)", current, next)
	case PhasePoCValidate:
		return fmt.Sprintf("PoC Validate in progress (block %d → ends ~%d)", current, next)
	case PhaseConfirmationPoC:
		return fmt.Sprintf("Confirmation PoC safety window (block %d → next PoC at %d)", current, next)
	default:
		return "phase unknown — fail-closed"
	}
}

// dockerExecQueryRunner is the production QueryRunner; runs commands via
// `docker exec api ...` (optionally with sudo) and returns combined stdout.
type dockerExecQueryRunner struct {
	state *config.State
}

func (r *dockerExecQueryRunner) Query(ctx context.Context, args ...string) ([]byte, error) {
	if r.state == nil {
		return nil, fmt.Errorf("dockerExecQueryRunner: nil state")
	}
	// Build: [sudo -E] docker exec api <args...>
	execArgs := append([]string{"exec", "api"}, args...)

	var name string
	var cmdArgs []string
	if r.state.UseSudo {
		name = docker.CmdSudo
		cmdArgs = append([]string{"-E", docker.CmdDocker}, execArgs...)
	} else {
		name = docker.CmdDocker
		cmdArgs = execArgs
	}

	cmd := exec.CommandContext(ctx, name, cmdArgs...) // #nosec G204
	cmd.Dir = r.state.OutputDir
	return cmd.Output()
}
