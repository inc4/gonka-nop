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

// Verify dockerExecTxBroadcaster implements TxModuleBroadcaster.
var _ TxModuleBroadcaster = (*dockerExecTxBroadcaster)(nil)

// BroadcastModule runs `inferenced tx <module> <action> <args...>` allowing
// targets other than the default "inference" module (e.g. "collateral").
// Password is piped via cmd.Stdin (NEVER argv) per NFR-M11-6.
func (b *dockerExecTxBroadcaster) BroadcastModule(ctx context.Context, module, signerKey, password, action string, args ...string) (string, error) {
	chainID := b.State.ChainID
	if chainID == "" {
		chainID = "gonka-mainnet"
	}
	innerArgs := make([]string, 0, 3+len(args)+15)
	innerArgs = append(innerArgs, "tx", module, action)
	innerArgs = append(innerArgs, args...)
	innerArgs = append(innerArgs,
		"--from", signerKey,
		"--chain-id", chainID,
		"--keyring-backend", "file",
		"--keyring-dir", "/root/.inference",
		"--node", "http://node:26657",
		"--gas", "2000000",
		"--fees", "0icoin",
		"-y",
	)
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
		return strings.TrimSpace(string(out)), fmt.Errorf("inferenced tx %s %s: %w", module, action, err)
	}
	return strings.TrimSpace(string(out)), nil
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
	innerArgs := make([]string, 0, 3+len(args)+15)
	innerArgs = append(innerArgs, "tx", "inference", action)
	innerArgs = append(innerArgs, args...)
	innerArgs = append(innerArgs,
		"--from", signerKey,
		"--chain-id", chainID,
		"--keyring-backend", "file",
		"--keyring-dir", "/root/.inference",
		"--node", "http://node:26657",
		"--gas", "2000000",
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

// modelsAllResp matches `inferenced query inference models-all -o json`
// — chain returns {"model": [{"id": "...", ...}, ...]} (singular field
// despite multi-element slice, per Cosmos SDK convention).
type modelsAllResp struct {
	Model []struct {
		ID string `json:"id"`
	} `json:"model"`
}

// ListGovernanceModels returns the chain's current set of governance-
// approved model IDs (independent of any participant's per-model
// delegations). Used by `gonka-nop status` to surface available models
// to operators who haven't declared any opt-in yet.
//
// Returns nil + error on any query/parse failure. Callers in best-effort
// paths (status display) should treat both nil-with-error and empty as
// "skip section".
func ListGovernanceModels(ctx context.Context, qr QueryRunner) ([]string, error) {
	out, err := qr.Query(ctx, "inferenced", "query", "inference", "models-all",
		"--node", "http://node:26657", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("query models-all: %w", err)
	}
	return parseModelsAllResponse(out)
}

func parseModelsAllResponse(jsonBytes []byte) ([]string, error) {
	var r modelsAllResp
	if err := json.Unmarshal(jsonBytes, &r); err != nil {
		return nil, fmt.Errorf("parse models-all response: %w", err)
	}
	out := make([]string, 0, len(r.Model))
	for _, m := range r.Model {
		if m.ID == "" {
			continue
		}
		out = append(out, m.ID)
	}
	return out, nil
}

// HardwareUnit is one row in a hardware_nodes[].hardware[] array.
type HardwareUnit struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// HardwareNodeSpec is one entry in a participant's hardware_nodes array.
type HardwareNodeSpec struct {
	LocalID  string         `json:"local_id"`
	Status   string         `json:"status"`
	Models   []string       `json:"models"`
	Hardware []HardwareUnit `json:"hardware"`
}

// ParticipantHardware groups all of one participant's hardware_nodes.
type ParticipantHardware struct {
	Participant   string             `json:"participant"`
	HardwareNodes []HardwareNodeSpec `json:"hardware_nodes"`
}

// hardwareNodesAllResp matches the wire shape of
// `inferenced query inference hardware-nodes-all -o json`.
type hardwareNodesAllResp struct {
	Nodes []ParticipantHardware `json:"nodes"`
}

// ListHardwareNodesAll returns every participant's hardware_nodes registry
// entry on chain. Used by `gonka-nop govmodel candidates` to discover hosts
// running a given governance model.
func ListHardwareNodesAll(ctx context.Context, qr QueryRunner) ([]ParticipantHardware, error) {
	out, err := qr.Query(ctx, "inferenced", "query", "inference", "hardware-nodes-all",
		"--node", "http://node:26657", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("query hardware-nodes-all: %w", err)
	}
	return parseHardwareNodesAllResponse(out)
}

func parseHardwareNodesAllResponse(jsonBytes []byte) ([]ParticipantHardware, error) {
	var r hardwareNodesAllResp
	if err := json.Unmarshal(jsonBytes, &r); err != nil {
		return nil, fmt.Errorf("parse hardware-nodes-all response: %w", err)
	}
	return r.Nodes, nil
}

// DelegationCandidate aggregates a participant's nodes for a target model:
// counts by status and a compact hardware summary. Used to rank delegation
// targets — operators want the most active fleet with the fewest failures.
type DelegationCandidate struct {
	Participant string
	// InferenceNodes is the count of this participant's hardware_nodes that
	// have the target model registered AND are currently in INFERENCE state.
	InferenceNodes int
	// FailedNodes is the same set but in FAILED state.
	FailedNodes int
	// OtherNodes covers any other status (POC, IDLE, LOADING, ...).
	OtherNodes int
	// Hardware is a human-readable summary like "8x NVIDIA B200 | 179GB ×3"
	// limited to a handful of dominant entries to keep tables tight.
	Hardware string
}

// TotalNodes returns the sum across all statuses for the target model.
func (c DelegationCandidate) TotalNodes() int {
	return c.InferenceNodes + c.FailedNodes + c.OtherNodes
}

// IsHealthy returns true when at least one node is INFERENCE and zero are
// FAILED. Used by `--healthy-only` filtering.
func (c DelegationCandidate) IsHealthy() bool {
	return c.InferenceNodes > 0 && c.FailedNodes == 0
}

// RankCandidatesForModel aggregates hardware_nodes by participant for the
// target model and returns a slice sorted best-first:
//  1. INFERENCE count DESC (more active nodes = better delegate)
//  2. FAILED count ASC (fewer failures = better)
//  3. total nodes DESC (bigger fleet = better tiebreaker)
//  4. participant address ASC (stable ordering for golden tests)
//
// Participants with zero nodes for the target model are dropped entirely.
// Model match is case-sensitive (chain governance IDs are canonical).
func RankCandidatesForModel(entries []ParticipantHardware, modelID string) []DelegationCandidate {
	bucket := make(map[string]*DelegationCandidate)
	hwCounts := make(map[string]map[string]int) // participant -> hwType -> count

	for _, e := range entries {
		for _, hw := range e.HardwareNodes {
			if !containsModel(hw.Models, modelID) {
				continue
			}
			c, ok := bucket[e.Participant]
			if !ok {
				c = &DelegationCandidate{Participant: e.Participant}
				bucket[e.Participant] = c
				hwCounts[e.Participant] = make(map[string]int)
			}
			switch hw.Status {
			case "INFERENCE":
				c.InferenceNodes++
			case "FAILED":
				c.FailedNodes++
			default:
				c.OtherNodes++
			}
			for _, h := range hw.Hardware {
				key := fmt.Sprintf("%dx %s", h.Count, h.Type)
				hwCounts[e.Participant][key]++
			}
		}
	}

	out := make([]DelegationCandidate, 0, len(bucket))
	for addr, c := range bucket {
		c.Hardware = summarizeHardware(hwCounts[addr])
		out = append(out, *c)
	}

	sortCandidates(out)
	return out
}

func containsModel(models []string, target string) bool {
	for _, m := range models {
		if m == target {
			return true
		}
	}
	return false
}

// summarizeHardware reduces a hardware-type-count map to a stable compact
// string like "8x B200 ×3 + 4x B300 ×2". Sorted by count DESC then key ASC
// so the output is deterministic. At most 3 entries shown; the rest are
// folded into "+N more".
func summarizeHardware(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	type kv struct {
		key   string
		count int
	}
	pairs := make([]kv, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, kv{k, v})
	}
	// Sort by count desc, then key asc (stable for tests).
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].count > pairs[i].count ||
				(pairs[j].count == pairs[i].count && pairs[j].key < pairs[i].key) {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	const maxEntries = 3
	parts := make([]string, 0, maxEntries+1)
	for i, p := range pairs {
		if i >= maxEntries {
			parts = append(parts, fmt.Sprintf("+%d more", len(pairs)-maxEntries))
			break
		}
		if p.count > 1 {
			parts = append(parts, fmt.Sprintf("%s ×%d", p.key, p.count))
		} else {
			parts = append(parts, p.key)
		}
	}
	return strings.Join(parts, " + ")
}

// sortCandidates sorts in place by the documented ranking rules.
func sortCandidates(c []DelegationCandidate) {
	for i := 0; i < len(c); i++ {
		for j := i + 1; j < len(c); j++ {
			if candidateLess(c[j], c[i]) {
				c[i], c[j] = c[j], c[i]
			}
		}
	}
}

// candidateLess returns true if a should sort BEFORE b (i.e., a is the
// better delegate). Rules in priority order:
//  1. higher INFERENCE count wins
//  2. lower FAILED count wins
//  3. higher total nodes wins
//  4. lexicographically smaller participant address wins
func candidateLess(a, b DelegationCandidate) bool {
	if a.InferenceNodes != b.InferenceNodes {
		return a.InferenceNodes > b.InferenceNodes
	}
	if a.FailedNodes != b.FailedNodes {
		return a.FailedNodes < b.FailedNodes
	}
	if a.TotalNodes() != b.TotalNodes() {
		return a.TotalNodes() > b.TotalNodes()
	}
	return a.Participant < b.Participant
}
