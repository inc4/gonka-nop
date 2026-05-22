package chain

import (
	"context"
	"strings"
	"testing"
)

const testKimiModel = "moonshotai/Kimi-K2.6"

func TestValidateGonkaAddress(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"valid mainnet address", "gonka19f9hkpmjaldncsfly4j63sy932y8hughn4l3d8", false},
		{"valid 45-char", "gonka1jvmumx6rwyq302m6y6z5hdkc4cgu845mcgjwud", false},
		{"empty", "", true},
		{"wrong prefix", "cosmos19f9hkpmjaldncsfly4j63sy932y8hughn4l3d8", true},
		{"too short", "gonka1abc", true},
		{"contains uppercase", "gonka1ABCDEFGHIJKLMNOPQRSTUVWXYZ234567890ABC", true},
		{"contains forbidden bech32 char", "gonka1bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true}, // 'b' is allowed actually
		{"contains '1' in payload", "gonka1111111111111111111111111111111111111111", true},      // '1' separator forbidden in payload
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGonkaAddress(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGonkaAddress(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}

// recordingBroadcaster is a fake TxBroadcaster for canary tests.
type recordingBroadcaster struct {
	lastSigner   string
	lastPassword string
	lastAction   string
	lastArgs     []string
	resp         string
	err          error
}

func (r *recordingBroadcaster) Broadcast(_ context.Context, signerKey, password, action string, args ...string) (string, error) {
	r.lastSigner = signerKey
	r.lastPassword = password
	r.lastAction = action
	r.lastArgs = append([]string{}, args...)
	return r.resp, r.err
}

func TestSetPoCDelegation_BuildsCorrectArgs(t *testing.T) {
	b := &recordingBroadcaster{resp: "txhash: ABCD"}
	out, err := SetPoCDelegation(context.Background(), b, "gonka-node-cold", "secret123",
		testKimiModel, "gonka19f9hkpmjaldncsfly4j63sy932y8hughn4l3d8")
	if err != nil {
		t.Fatalf("SetPoCDelegation: %v", err)
	}
	if b.lastAction != "set-poc-delegation" {
		t.Errorf("action = %q, want set-poc-delegation", b.lastAction)
	}
	if len(b.lastArgs) != 2 || b.lastArgs[0] != testKimiModel {
		t.Errorf("args = %v, want [moonshotai/Kimi-K2.6, gonka19f9...]", b.lastArgs)
	}
	if b.lastSigner != "gonka-node-cold" {
		t.Errorf("signer = %q", b.lastSigner)
	}
	if b.lastPassword != "secret123" {
		t.Errorf("password = %q", b.lastPassword)
	}
	if !strings.Contains(out, "txhash") {
		t.Errorf("output = %q", out)
	}
}

func TestRefusePoCDelegation_BuildsCorrectArgs(t *testing.T) {
	b := &recordingBroadcaster{resp: "ok"}
	_, _ = RefusePoCDelegation(context.Background(), b, "cold", "pwd", testKimiModel)
	if b.lastAction != "refuse-poc-delegation" || len(b.lastArgs) != 1 || b.lastArgs[0] != testKimiModel {
		t.Errorf("Refuse args wrong: action=%s args=%v", b.lastAction, b.lastArgs)
	}
}

func TestDeclarePoCIntent_BuildsCorrectArgs(t *testing.T) {
	b := &recordingBroadcaster{resp: "ok"}
	_, _ = DeclarePoCIntent(context.Background(), b, "cold", "pwd", testKimiModel)
	if b.lastAction != "declare-poc-intent" {
		t.Errorf("action = %s, want declare-poc-intent", b.lastAction)
	}
}

func TestParsePoCDelegationResponse(t *testing.T) {
	// Matches the actual mainnet schema verified 2026-05-16: chain returns
	// `delegate_to` (not `delegate`), plus an echoed `delegator` field we ignore.
	body := `{
		"delegations": [{"model_id": "` + testKimiModel + `", "delegator": "gonka1us", "delegate_to": "gonka1xyz"}],
		"refusals":    [{"model_id": "Qwen/QwQ-32B"}],
		"intents":     [{"model_id": "future/Model-v3"}]
	}`
	states, err := parsePoCDelegationResponse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("got %d states, want 3", len(states))
	}
	if states[0].ModelID != testKimiModel || states[0].Delegate != "gonka1xyz" {
		t.Errorf("delegation parse: %+v", states[0])
	}
	if !states[1].Refused {
		t.Errorf("refusal parse: %+v", states[1])
	}
	if !states[2].IntentOnly {
		t.Errorf("intent parse: %+v", states[2])
	}
}

func TestParsePoCDelegationResponse_Empty(t *testing.T) {
	states, err := parsePoCDelegationResponse([]byte(`{}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected empty slice for empty body, got %d", len(states))
	}
}

func TestParsePoCDelegationResponse_Malformed(t *testing.T) {
	_, err := parsePoCDelegationResponse([]byte("not json"))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestListPoCDelegations_HappyPath(t *testing.T) {
	qr := &fakeRunner{out: []byte(`{"delegations":[{"model_id":"` + testKimiModel + `","delegator":"gonka1us","delegate_to":"gonka1xyz"}]}`)}
	states, err := ListPoCDelegations(context.Background(), qr, "gonka1us")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(states) != 1 || states[0].Delegate != "gonka1xyz" {
		t.Errorf("got %+v", states)
	}
}

func TestListPoCDelegations_QueryError(t *testing.T) {
	qr := &fakeRunner{err: context.DeadlineExceeded}
	_, err := ListPoCDelegations(context.Background(), qr, "gonka1us")
	if err == nil {
		t.Fatal("expected error from query failure")
	}
}

// realisticMainnetModelsAll matches the actual payload shape verified on
// mainnet 2026-05-22 via `inferenced query inference models-all -o json`.
// Includes the side fields the chain returns (proposed_by, hf_repo, etc.)
// to ensure parseModelsAllResponse ignores unknown keys cleanly.
const realisticMainnetModelsAll = `{
  "model": [
    {
      "proposed_by": "gonka1kx9mca3xm8u8ypzfuhmxey66u0ufxhs7nm6wc5",
      "id": "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
      "units_of_compute_per_token": "10000",
      "hf_repo": "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"
    },
    {
      "proposed_by": "gonka10d07y265gmmuvt4z0w9aw880jnsr700j2h5m33",
      "id": "moonshotai/Kimi-K2.6",
      "units_of_compute_per_token": "10000",
      "hf_repo": "moonshotai/Kimi-K2.6"
    }
  ]
}`

func TestListGovernanceModels_HappyPath(t *testing.T) {
	qr := &fakeRunner{out: []byte(realisticMainnetModelsAll)}
	ids, err := ListGovernanceModels(context.Background(), qr)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2: %v", len(ids), ids)
	}
	if ids[0] != "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8" || ids[1] != "moonshotai/Kimi-K2.6" {
		t.Errorf("ids out of order or wrong: %v", ids)
	}
}

func TestListGovernanceModels_EmptyResponse(t *testing.T) {
	qr := &fakeRunner{out: []byte(`{"model":[]}`)}
	ids, err := ListGovernanceModels(context.Background(), qr)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty slice, got %v", ids)
	}
}

func TestListGovernanceModels_QueryError(t *testing.T) {
	qr := &fakeRunner{err: context.DeadlineExceeded}
	_, err := ListGovernanceModels(context.Background(), qr)
	if err == nil {
		t.Fatal("expected error from query failure")
	}
}

func TestListGovernanceModels_MalformedJSON(t *testing.T) {
	qr := &fakeRunner{out: []byte("not json")}
	_, err := ListGovernanceModels(context.Background(), qr)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestListGovernanceModels_SkipsEmptyIDs(t *testing.T) {
	// Edge case: chain returns an entry with empty ID — skip rather than emit "".
	qr := &fakeRunner{out: []byte(`{"model":[{"id":"X"},{"id":""},{"id":"Y"}]}`)}
	ids, err := ListGovernanceModels(context.Background(), qr)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 2 || ids[0] != "X" || ids[1] != "Y" {
		t.Errorf("expected [X Y], got %v", ids)
	}
}

// --- hardware-nodes-all + DelegationCandidate ranking ----------------------

// realisticMainnetHardwareNodesAll is a trimmed slice of the actual mainnet
// payload verified 2026-05-22 — picks a mix of healthy (gonka1qa90 with 17
// INFERENCE B300s), mixed-health (gonka15munk with one INFERENCE B200), and
// failing (gonka125n6 with all FAILED B200s) participants serving Kimi-K2.6
// plus a participant serving only Qwen3-235B (gonka10000627). The aggregator
// must filter to Kimi candidates, drop the Qwen-only one, and rank by health.
const realisticMainnetHardwareNodesAll = `{
  "nodes": [
    {
      "participant": "gonka10000627dkz6nvmf09ctqy073v0fls696uznwgg",
      "hardware_nodes": [
        {"local_id":"node1","status":"INFERENCE","models":["Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"],
         "hardware":[{"type":"NVIDIA H200","count":8}]}
      ]
    },
    {
      "participant": "gonka1qa90tgczc0k5dvk4l5nvlf5y6phgm6mg22sfjv",
      "hardware_nodes": [
        {"local_id":"s1-8b300-01","status":"INFERENCE","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B300 SXM6 AC","count":8}]},
        {"local_id":"s1-8b300-02","status":"INFERENCE","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B300 SXM6 AC","count":8}]},
        {"local_id":"s1-8b300-03","status":"INFERENCE","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B300 SXM6 AC","count":8}]}
      ]
    },
    {
      "participant": "gonka15munkmx6x7k6rqqeexjet4556p7at39ks9qgr5",
      "hardware_nodes": [
        {"local_id":"ml1","status":"INFERENCE","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B200","count":8}]},
        {"local_id":"ml5","status":"INFERENCE","models":["Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"],
         "hardware":[{"type":"NVIDIA B300 SXM6 AC","count":8}]}
      ]
    },
    {
      "participant": "gonka125n6kr5gvdup0lndfkps7t6rd6592panhrg3np",
      "hardware_nodes": [
        {"local_id":"node481","status":"FAILED","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B200","count":8}]},
        {"local_id":"node482","status":"FAILED","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B200","count":8}]}
      ]
    },
    {
      "participant": "gonka19cjm4c5mt3j3qdr8vhytmm4hef3pnkvkm0x7m2",
      "hardware_nodes": [
        {"local_id":"node218","status":"INFERENCE","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B200","count":8}]},
        {"local_id":"node228","status":"FAILED","models":["moonshotai/Kimi-K2.6"],
         "hardware":[{"type":"NVIDIA B200","count":8}]}
      ]
    }
  ]
}`

func TestListHardwareNodesAll_HappyPath(t *testing.T) {
	qr := &fakeRunner{out: []byte(realisticMainnetHardwareNodesAll)}
	entries, err := ListHardwareNodesAll(context.Background(), qr)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(entries))
	}
	if entries[1].Participant != "gonka1qa90tgczc0k5dvk4l5nvlf5y6phgm6mg22sfjv" {
		t.Errorf("entry[1] participant wrong: %q", entries[1].Participant)
	}
	if len(entries[1].HardwareNodes) != 3 {
		t.Errorf("entry[1] hardware_nodes count: got %d want 3", len(entries[1].HardwareNodes))
	}
}

func TestListHardwareNodesAll_QueryError(t *testing.T) {
	qr := &fakeRunner{err: context.DeadlineExceeded}
	_, err := ListHardwareNodesAll(context.Background(), qr)
	if err == nil {
		t.Fatal("expected error from query failure")
	}
}

func TestListHardwareNodesAll_MalformedJSON(t *testing.T) {
	qr := &fakeRunner{out: []byte("not json")}
	_, err := ListHardwareNodesAll(context.Background(), qr)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRankCandidatesForModel_KimiRanksByHealth(t *testing.T) {
	qr := &fakeRunner{out: []byte(realisticMainnetHardwareNodesAll)}
	entries, err := ListHardwareNodesAll(context.Background(), qr)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	cands := RankCandidatesForModel(entries, "moonshotai/Kimi-K2.6")

	// gonka10000627 has only Qwen → must be excluded entirely.
	for _, c := range cands {
		if c.Participant == "gonka10000627dkz6nvmf09ctqy073v0fls696uznwgg" {
			t.Errorf("Qwen-only participant should not appear in Kimi candidates: %+v", c)
		}
	}
	if len(cands) != 4 {
		t.Fatalf("got %d candidates, want 4 (Qwen-only filtered): %+v", len(cands), cands)
	}

	// Best should be gonka1qa90 with 3 INFERENCE, 0 FAILED.
	if cands[0].Participant != "gonka1qa90tgczc0k5dvk4l5nvlf5y6phgm6mg22sfjv" {
		t.Errorf("rank[0] should be gonka1qa90, got %s", cands[0].Participant)
	}
	if cands[0].InferenceNodes != 3 || cands[0].FailedNodes != 0 {
		t.Errorf("rank[0] node counts wrong: %+v", cands[0])
	}

	// Second healthiest: gonka15munk (1 INFERENCE Kimi node, 0 FAILED — its
	// Qwen ml5 must NOT inflate Kimi counts).
	if cands[1].Participant != "gonka15munkmx6x7k6rqqeexjet4556p7at39ks9qgr5" {
		t.Errorf("rank[1] should be gonka15munk, got %s", cands[1].Participant)
	}
	if cands[1].InferenceNodes != 1 || cands[1].FailedNodes != 0 {
		t.Errorf("rank[1] node counts wrong (Qwen ml5 leaking?): %+v", cands[1])
	}

	// Third: gonka19cjm with 1 INFERENCE + 1 FAILED (mixed health).
	if cands[2].Participant != "gonka19cjm4c5mt3j3qdr8vhytmm4hef3pnkvkm0x7m2" {
		t.Errorf("rank[2] should be gonka19cjm, got %s", cands[2].Participant)
	}

	// Worst: gonka125n6 with 0 INFERENCE + 2 FAILED.
	last := cands[len(cands)-1]
	if last.Participant != "gonka125n6kr5gvdup0lndfkps7t6rd6592panhrg3np" {
		t.Errorf("last should be gonka125n6 (all FAILED), got %s", last.Participant)
	}
	if last.IsHealthy() {
		t.Errorf("all-FAILED participant should not be IsHealthy(): %+v", last)
	}
}

func TestRankCandidatesForModel_EmptyEntries(t *testing.T) {
	cands := RankCandidatesForModel(nil, "moonshotai/Kimi-K2.6")
	if len(cands) != 0 {
		t.Errorf("nil entries should yield empty: %v", cands)
	}
}

func TestRankCandidatesForModel_ModelNotFound(t *testing.T) {
	qr := &fakeRunner{out: []byte(realisticMainnetHardwareNodesAll)}
	entries, _ := ListHardwareNodesAll(context.Background(), qr)
	cands := RankCandidatesForModel(entries, "no/such-model")
	if len(cands) != 0 {
		t.Errorf("unknown model should yield empty: %v", cands)
	}
}

func TestDelegationCandidate_IsHealthy(t *testing.T) {
	cases := []struct {
		name string
		c    DelegationCandidate
		want bool
	}{
		{"all-inference", DelegationCandidate{InferenceNodes: 5}, true},
		{"one-failed", DelegationCandidate{InferenceNodes: 5, FailedNodes: 1}, false},
		{"all-failed", DelegationCandidate{FailedNodes: 3}, false},
		{"empty", DelegationCandidate{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.IsHealthy(); got != tc.want {
				t.Errorf("IsHealthy()=%v want %v for %+v", got, tc.want, tc.c)
			}
		})
	}
}

func TestSummarizeHardware_StableOrderAndCap(t *testing.T) {
	// More than maxEntries (3) types — must show top 3 + "+N more".
	counts := map[string]int{
		"8x NVIDIA B200":         3,
		"8x NVIDIA B300 SXM6 AC": 5,
		"4x NVIDIA B200":         2,
		"8x NVIDIA H200":         1,
		"4x NVIDIA B300 SXM6 AC": 1,
	}
	got := summarizeHardware(counts)
	want := "8x NVIDIA B300 SXM6 AC ×5 + 8x NVIDIA B200 ×3 + 4x NVIDIA B200 ×2 + +2 more"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSummarizeHardware_Empty(t *testing.T) {
	if got := summarizeHardware(nil); got != "" {
		t.Errorf("nil map should yield empty string, got %q", got)
	}
	if got := summarizeHardware(map[string]int{}); got != "" {
		t.Errorf("empty map should yield empty string, got %q", got)
	}
}
