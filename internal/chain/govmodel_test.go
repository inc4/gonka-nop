package chain

import (
	"context"
	"strings"
	"testing"
)

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
		"moonshotai/Kimi-K2.6", "gonka19f9hkpmjaldncsfly4j63sy932y8hughn4l3d8")
	if err != nil {
		t.Fatalf("SetPoCDelegation: %v", err)
	}
	if b.lastAction != "set-poc-delegation" {
		t.Errorf("action = %q, want set-poc-delegation", b.lastAction)
	}
	if len(b.lastArgs) != 2 || b.lastArgs[0] != "moonshotai/Kimi-K2.6" {
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
	_, _ = RefusePoCDelegation(context.Background(), b, "cold", "pwd", "moonshotai/Kimi-K2.6")
	if b.lastAction != "refuse-poc-delegation" || len(b.lastArgs) != 1 || b.lastArgs[0] != "moonshotai/Kimi-K2.6" {
		t.Errorf("Refuse args wrong: action=%s args=%v", b.lastAction, b.lastArgs)
	}
}

func TestDeclarePoCIntent_BuildsCorrectArgs(t *testing.T) {
	b := &recordingBroadcaster{resp: "ok"}
	_, _ = DeclarePoCIntent(context.Background(), b, "cold", "pwd", "moonshotai/Kimi-K2.6")
	if b.lastAction != "declare-poc-intent" {
		t.Errorf("action = %s, want declare-poc-intent", b.lastAction)
	}
}

func TestParsePoCDelegationResponse(t *testing.T) {
	// Matches the actual mainnet schema verified 2026-05-16: chain returns
	// `delegate_to` (not `delegate`), plus an echoed `delegator` field we ignore.
	body := `{
		"delegations": [{"model_id": "moonshotai/Kimi-K2.6", "delegator": "gonka1us", "delegate_to": "gonka1xyz"}],
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
	if states[0].ModelID != "moonshotai/Kimi-K2.6" || states[0].Delegate != "gonka1xyz" {
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
	qr := &fakeRunner{out: []byte(`{"delegations":[{"model_id":"moonshotai/Kimi-K2.6","delegator":"gonka1us","delegate_to":"gonka1xyz"}]}`)}
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
