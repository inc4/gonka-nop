package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
)

// gonkaAddrRe is the shape check for Gonka bech32 addresses. The chain
// rejects malformed addresses on tx submit anyway; client-side validation
// is purely UX (catch operator typos early). Charset matches bech32
// (lowercase a-z + digits 0-9, no 1 b i o).
var gonkaAddrRe = regexp.MustCompile(`^gonka1[023456789acdefghjklmnpqrstuvwxyz]{38,58}$`)

// ValidateGonkaAddress returns nil for a bech32-shaped Gonka address.
// Does NOT perform full bech32 checksum validation (would require a heavy
// cosmos-sdk dependency just for client UX); chain enforces correctness.
func ValidateGonkaAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("address is empty")
	}
	if !strings.HasPrefix(addr, "gonka1") {
		return fmt.Errorf("address %q must start with 'gonka1'", addr)
	}
	if !gonkaAddrRe.MatchString(addr) {
		return fmt.Errorf("address %q does not match expected bech32 shape (gonka1 + 38-58 bech32 chars)", addr)
	}
	return nil
}

// TxBroadcaster broadcasts an `inferenced tx ...` command. The production
// impl shells out via `docker exec -i api` with password piped over stdin.
// Tests inject a fake to assert constructed argv + that the password is
// in stdin, never in argv (NFR-M11-6 canary).
type TxBroadcaster interface {
	// Broadcast runs `inferenced tx inference <action> <args...>` inside
	// the api container with --from <signerKey> and --keyring-backend file,
	// piping password over stdin. Returns combined stdout+stderr.
	Broadcast(ctx context.Context, signerKey, password, action string, args ...string) (string, error)
}

// QueryRunnerFromState returns a QueryRunner wired to docker exec via state.
// Reuses the unexported dockerExecQueryRunner from poc_safety.go.
func QueryRunnerFromState(state *config.State) QueryRunner {
	return &dockerExecQueryRunner{state: state}
}

// dockerExecTxBroadcaster is the production TxBroadcaster. Constructs:
//
//	[sudo -E] docker exec -i api inferenced tx inference <action> <args...> \
//	    --from <signerKey> --chain-id <id> --keyring-backend file \
//	    --keyring-dir /root/.inference --node http://node:26657 \
//	    --gas 200000 --fees 0icoin -y
//
// Password is piped via cmd.Stdin so it never appears in argv (/proc/PID/cmdline).
type dockerExecTxBroadcaster struct {
	State *config.State
}

func (b *dockerExecTxBroadcaster) Broadcast(ctx context.Context, signerKey, password, action string, args ...string) (string, error) {
	chainID := b.State.ChainID
	if chainID == "" {
		chainID = "gonka-mainnet"
	}
	// Build the inferenced tx argv inside the api container.
	innerArgs := []string{"tx", "inference", action}
	innerArgs = append(innerArgs, args...)
	innerArgs = append(innerArgs,
		"--from", signerKey,
		"--chain-id", chainID,
		"--keyring-backend", "file",
		"--keyring-dir", "/root/.inference",
		"--node", "http://node:26657",
		"--gas", "200000",
		"--fees", "0icoin",
		"-y",
	)
	// Build the docker exec invocation. `-i` is required so stdin is forwarded.
	execArgs := append([]string{"exec", "-i", "api", "inferenced"}, innerArgs...)

	var name string
	var cmdArgs []string
	if b.State.UseSudo {
		name = docker.CmdSudo
		cmdArgs = append([]string{"-E", docker.CmdDocker}, execArgs...)
	} else {
		name = docker.CmdDocker
		cmdArgs = execArgs
	}

	cmd := exec.CommandContext(ctx, name, cmdArgs...) // #nosec G204
	cmd.Dir = b.State.OutputDir
	cmd.Stdin = strings.NewReader(password + "\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("inferenced tx %s: %w", action, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// NewDefaultTxBroadcaster returns the production broadcaster.
func NewDefaultTxBroadcaster(state *config.State) TxBroadcaster {
	return &dockerExecTxBroadcaster{State: state}
}

// ───────────────────────────────────────────────────────────────────────────
// High-level governance-model tx wrappers
// ───────────────────────────────────────────────────────────────────────────

// SetPoCDelegation broadcasts `inferenced tx inference set-poc-delegation
// <model-id> <delegate-to>` signed by signerKey.
func SetPoCDelegation(ctx context.Context, b TxBroadcaster, signerKey, password, modelID, delegateTo string) (string, error) {
	return b.Broadcast(ctx, signerKey, password, "set-poc-delegation", modelID, delegateTo)
}

// RefusePoCDelegation broadcasts `refuse-poc-delegation <model-id>`.
func RefusePoCDelegation(ctx context.Context, b TxBroadcaster, signerKey, password, modelID string) (string, error) {
	return b.Broadcast(ctx, signerKey, password, "refuse-poc-delegation", modelID)
}

// DeclarePoCIntent broadcasts `declare-poc-intent <model-id>`.
func DeclarePoCIntent(ctx context.Context, b TxBroadcaster, signerKey, password, modelID string) (string, error) {
	return b.Broadcast(ctx, signerKey, password, "declare-poc-intent", modelID)
}

// ───────────────────────────────────────────────────────────────────────────
// Query: list per-participant delegations / refusals / intents
// ───────────────────────────────────────────────────────────────────────────

// PoCDelegationState captures the per-(participant, model) opt-in state
// returned by `inferenced query inference poc-delegation`.
type PoCDelegationState struct {
	ModelID    string `json:"model_id"`
	Delegate   string `json:"delegate,omitempty"`    // delegatee address; empty if not delegated
	Refused    bool   `json:"refused,omitempty"`     // true if participant declared refusal
	IntentOnly bool   `json:"intent_only,omitempty"` // true for bootstrap-intent without action
}

// ListPoCDelegations runs `inferenced query inference poc-delegation
// <participant> -o json` and parses the response. The chain returns one
// of: {delegations: [...]}, {refusals: [...]}, {intents: [...]}, or
// empty, per the multi-model PoC schema.
func ListPoCDelegations(ctx context.Context, qr QueryRunner, participant string) ([]PoCDelegationState, error) {
	out, err := qr.Query(ctx, "inferenced", "query", "inference", "poc-delegation",
		participant, "--node", "http://node:26657", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("query poc-delegation: %w", err)
	}
	return parsePoCDelegationResponse(out)
}

type pocDelegationResp struct {
	Delegations []struct {
		ModelID    string `json:"model_id"`
		Delegator  string `json:"delegator"`   // mainnet returns this — we ignore (== caller's addr)
		DelegateTo string `json:"delegate_to"` // mainnet uses delegate_to, NOT delegate
	} `json:"delegations"`
	Refusals []struct {
		ModelID string `json:"model_id"`
	} `json:"refusals"`
	Intents []struct {
		ModelID string `json:"model_id"`
	} `json:"intents"`
}

func parsePoCDelegationResponse(jsonBytes []byte) ([]PoCDelegationState, error) {
	var r pocDelegationResp
	if err := json.Unmarshal(jsonBytes, &r); err != nil {
		return nil, fmt.Errorf("parse poc-delegation response: %w", err)
	}
	var out []PoCDelegationState
	for _, d := range r.Delegations {
		out = append(out, PoCDelegationState{ModelID: d.ModelID, Delegate: d.DelegateTo})
	}
	for _, ref := range r.Refusals {
		out = append(out, PoCDelegationState{ModelID: ref.ModelID, Refused: true})
	}
	for _, it := range r.Intents {
		out = append(out, PoCDelegationState{ModelID: it.ModelID, IntentOnly: true})
	}
	return out, nil
}
