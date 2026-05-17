// M12 Slice 8a tests — per security-auditor pre-review 2026-05-17:
// canary password isolation, balance check, register-new-participant guard.
package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/status"
)

const canaryPw = "UNIQUE_CANARY_8821_DEADBEEF"

// recordingBalancer captures GetBalance calls and returns canned values.
type recordingBalancer struct {
	amount uint64
	denom  string
	err    error
	calls  int
}

func (b *recordingBalancer) GetBalance(_ context.Context, _ string) (uint64, string, error) {
	b.calls++
	return b.amount, b.denom, b.err
}

// recordingPermBroadcaster captures Broadcast calls for assertion in tests.
type recordingPermBroadcaster struct {
	calls    int
	lastSign string
	lastPass string
	lastAct  string
	lastArgs []string
	resp     string
	err      error
}

func (b *recordingPermBroadcaster) Broadcast(_ context.Context, signerKey, password, action string, args ...string) (string, error) {
	b.calls++
	b.lastSign = signerKey
	b.lastPass = password
	b.lastAct = action
	b.lastArgs = append([]string{}, args...)
	return b.resp, b.err
}

func validBalance() *recordingBalancer {
	return &recordingBalancer{amount: 1_000_000_000, denom: "ngonka"}
}

// --- DiffPermissions ---

func TestDiffPermissions_AllGranted_ReturnsEmpty(t *testing.T) {
	report := &status.SetupReport{
		Checks: []status.SetupCheck{
			{ID: "permissions_granted", Status: "PASS", Details: json.RawMessage(`{}`)},
		},
	}
	got, err := DiffPermissions(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d permissions, want 0 when PASS", len(got))
	}
}

func TestDiffPermissions_EightMissing_ParsesAll(t *testing.T) {
	missing := []string{
		"/inference.inference.MsgSubmitPocValidation",
		"/inference.inference.MsgSubmitTrainingKvRecord",
		"/inference.inference.MsgJoinTraining",
		"/inference.inference.MsgJoinTrainingStatus",
		"/inference.inference.MsgTrainingHeartbeat",
		"/inference.inference.MsgSetBarrier",
		"/inference.inference.MsgClaimTrainingTaskForAssignment",
		"/inference.inference.MsgAssignTrainingTask",
	}
	details, _ := json.Marshal(map[string]interface{}{"missing_permissions": missing})
	report := &status.SetupReport{
		Checks: []status.SetupCheck{
			{ID: "permissions_granted", Status: "FAIL", Details: details},
		},
	}
	got, err := DiffPermissions(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("got %d permissions, want 8", len(got))
	}
	for i, p := range got {
		if p.Action != missing[i] {
			t.Errorf("got %q, want %q", p.Action, missing[i])
		}
	}
}

func TestDiffPermissions_NilReport_Errors(t *testing.T) {
	_, err := DiffPermissions(nil)
	if err == nil {
		t.Fatal("expected error on nil report")
	}
}

func TestDiffPermissions_NoCheck_Errors(t *testing.T) {
	report := &status.SetupReport{Checks: []status.SetupCheck{{ID: "other", Status: "PASS"}}}
	_, err := DiffPermissions(report)
	if err == nil {
		t.Fatal("expected error when permissions_granted check absent")
	}
}

// --- GrantMissing ---

func TestGrantMissing_HappyPath_BroadcastsGranterAndGrantee(t *testing.T) {
	b := &recordingPermBroadcaster{resp: "txhash: ABCD"}
	bal := validBalance()
	// Chain CLI signature is <granter-key-name> <grantee-address>; the
	// list of missing permissions is informational only (used to gate
	// the call and log; chain re-grants the full ml-ops set).
	missing := []Permission{{Action: "/foo"}, {Action: "/bar"}}

	tx, err := GrantMissing(context.Background(), b, bal,
		"cold", "gonka1signer", "pw", "gonka1grantee", missing, 100_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx != "txhash: ABCD" {
		t.Errorf("got %q", tx)
	}
	if b.calls != 1 {
		t.Errorf("Broadcast calls = %d, want 1", b.calls)
	}
	if b.lastAct != grantAction {
		t.Errorf("action = %q, want %q", b.lastAct, grantAction)
	}
	wantArgs := []string{"cold", "gonka1grantee"}
	if !equalSlices(b.lastArgs, wantArgs) {
		t.Errorf("args = %v, want %v (chain CLI takes exactly 2 positional args)", b.lastArgs, wantArgs)
	}
}

func TestGrantMissing_NeverConstructsForbiddenAction(t *testing.T) {
	// Architect test: across ALL GrantMissing invocations, the broadcaster
	// must never see register-new-participant or related actions.
	forbidden := []string{
		"register-new-participant",
		"submit-new-participant",
		"submit-new-unfunded-participant",
	}

	// Run several distinct invocations to widen coverage.
	for _, scenario := range []struct {
		name    string
		missing []Permission
	}{
		{"one missing", []Permission{{Action: "/x"}}},
		{"five missing", []Permission{{Action: "/a"}, {Action: "/b"}, {Action: "/c"}, {Action: "/d"}, {Action: "/e"}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			b := &recordingPermBroadcaster{resp: "ok"}
			_, _ = GrantMissing(context.Background(), b, validBalance(),
				"cold", "gonka1s", "pw", "gonka1g", scenario.missing, 100)
			for _, f := range forbidden {
				if b.lastAct == f {
					t.Errorf("GrantMissing constructed forbidden action %q", f)
				}
			}
		})
	}
}

func TestGrantMissing_ErrorNeverContainsPassword(t *testing.T) {
	b := &recordingPermBroadcaster{
		resp: "tx failed because keyring rejected " + canaryPw + " (chain returned this in error)",
		err:  fmt.Errorf("non-zero exit: %s", canaryPw),
	}
	_, err := GrantMissing(context.Background(), b, validBalance(),
		"cold", "gonka1s", canaryPw, "gonka1g", []Permission{{Action: "/x"}}, 100)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), canaryPw) {
		t.Errorf("error leaked canary password: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("expected '[REDACTED]' in error, got: %q", err.Error())
	}
}

func TestGrantMissing_StdinReceivesPassword(t *testing.T) {
	b := &recordingPermBroadcaster{resp: "ok"}
	_, _ = GrantMissing(context.Background(), b, validBalance(),
		"cold", "gonka1s", "secret-pw", "gonka1g", []Permission{{Action: "/x"}}, 100)
	if b.lastPass != "secret-pw" {
		t.Errorf("password not passed through Broadcast: got %q", b.lastPass)
	}
}

func TestGrantMissing_ArgsNeverContainsPassword(t *testing.T) {
	b := &recordingPermBroadcaster{resp: "ok"}
	_, _ = GrantMissing(context.Background(), b, validBalance(),
		"cold", "gonka1s", canaryPw, "gonka1g", []Permission{{Action: "/x"}}, 100)
	joined := strings.Join(b.lastArgs, " ")
	if strings.Contains(joined, canaryPw) {
		t.Errorf("password leaked into argv: %q", joined)
	}
}

func TestGrantMissing_BalanceInsufficient_NoBroadcast(t *testing.T) {
	b := &recordingPermBroadcaster{resp: "should not be called"}
	bal := &recordingBalancer{amount: 50, denom: "ngonka"}
	_, err := GrantMissing(context.Background(), b, bal,
		"cold", "gonka1signer", "pw", "gonka1g", []Permission{{Action: "/x"}}, 1000)
	if err == nil {
		t.Fatal("expected balance-insufficient error")
	}
	if !strings.Contains(err.Error(), "insufficient balance") {
		t.Errorf("error should name 'insufficient balance', got: %v", err)
	}
	if !strings.Contains(err.Error(), "gonka1signer") {
		t.Errorf("error should name signer address, got: %v", err)
	}
	if b.calls != 0 {
		t.Errorf("Broadcast called %d times despite insufficient balance", b.calls)
	}
}

func TestGrantMissing_BalanceQueryFails_NoBroadcast(t *testing.T) {
	b := &recordingPermBroadcaster{resp: "should not be called"}
	bal := &recordingBalancer{err: errors.New("rpc timeout")}
	_, err := GrantMissing(context.Background(), b, bal,
		"cold", "gonka1signer", "pw", "gonka1g", []Permission{{Action: "/x"}}, 100)
	if err == nil {
		t.Fatal("expected balance-query error")
	}
	if b.calls != 0 {
		t.Errorf("Broadcast called %d times despite balance query failure", b.calls)
	}
}

func TestGrantMissing_EmptyMissing_Errors(t *testing.T) {
	b := &recordingPermBroadcaster{resp: "should not be called"}
	_, err := GrantMissing(context.Background(), b, validBalance(),
		"cold", "gonka1s", "pw", "gonka1g", nil, 100)
	if err == nil {
		t.Fatal("expected error on empty missing slice")
	}
	if b.calls != 0 {
		t.Errorf("Broadcast called %d times despite empty missing", b.calls)
	}
}

// --- redactPassword ---

func TestRedactPassword_ReplacesCanary(t *testing.T) {
	got := redactPassword("here is the password: "+canaryPw+" in stderr", canaryPw)
	if strings.Contains(got, canaryPw) {
		t.Errorf("canary not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("missing [REDACTED] marker: %q", got)
	}
}

func TestRedactPassword_EmptyPassword_NoOp(t *testing.T) {
	got := redactPassword("normal output", "")
	if got != "normal output" {
		t.Errorf("empty password should be no-op, got: %q", got)
	}
}

func TestRedactedError_UnwrapPreservesUnderlying(t *testing.T) {
	inner := errors.New("original error")
	wrapped := redactPasswordErr(inner, "pw")
	if !errors.Is(wrapped, inner) {
		t.Error("errors.Is should still match the underlying error")
	}
}

// helpers

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
