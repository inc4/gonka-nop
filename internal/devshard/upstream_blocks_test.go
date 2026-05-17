// Tests for the upstream verbatim service/env constants and
// RenderVersiondBlock substitution. Per security-auditor pre-review of
// 2026-05-17 (APPROVED-WITH-CONDITIONS), additional tests guard against:
//   - silent shell expansion via state values containing `$`
//   - empty required state fields producing silent broken containers
//   - secret leakage into stdout/stderr or error messages
//   - duplicated password rendering
package devshard

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

const canaryPassword = "UNIQUE_CANARY_3344_DEADBEEF"

func newValidState(t *testing.T) *config.State {
	t.Helper()
	return &config.State{
		KeyName:         "gonka-node-warm",
		AccountPubKey:   "A8LK5/SoFXElGT5ZTfpmEHAEALvh8TiYoIGLGWX9kW2w",
		KeyringPassword: "PASS_ok_12345",
	}
}

// TestRenderVersiondBlock_SubstitutesAllPlaceholders (TC-COMPOSE-3 main).
func TestRenderVersiondBlock_SubstitutesAllPlaceholders(t *testing.T) {
	s := newValidState(t)
	s.KeyName = "cold"
	s.AccountPubKey = "pub"
	s.KeyringPassword = "pw"

	out, err := RenderVersiondBlock(s, "v0.2.12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"cold", "pub", "pw", "v0.2.12"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing literal %q\nfull:\n%s", want, out)
		}
	}

	for _, leftover := range []string{"${KEY_NAME}", "${ACCOUNT_PUBKEY}", "${KEYRING_PASSWORD}", "${VERSIOND_TAG}"} {
		if strings.Contains(out, leftover) {
			t.Errorf("rendered output still contains placeholder %q", leftover)
		}
	}
}

// TestRenderVersiondBlock_AllNineEnvKeysPresent (TC-COMPOSE-3 strict).
func TestRenderVersiondBlock_AllNineEnvKeysPresent(t *testing.T) {
	out, err := RenderVersiondBlock(newValidState(t), "v0.2.12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{
		"VERSIOND_ORACLE_URL",
		"VERSIOND_BINARY_NAME",
		"NODE_MANAGER_ADDR",
		"NODE_HOST",
		"KEY_NAME",
		"ACCOUNT_PUBKEY",
		"KEYRING_BACKEND",
		"KEYRING_PASSWORD",
		"KEYRING_DIR",
	} {
		if !strings.Contains(out, key+"=") {
			t.Errorf("rendered output missing env key %q", key)
		}
	}
}

// TestRenderVersiondBlock_NilState_Errors.
func TestRenderVersiondBlock_NilState_Errors(t *testing.T) {
	_, err := RenderVersiondBlock(nil, "v0.2.12")
	if err == nil {
		t.Fatal("expected error on nil state")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "state") {
		t.Errorf("error should mention 'state', got %q", err.Error())
	}
}

// TestRenderVersiondBlock_EmptyTag_Errors.
func TestRenderVersiondBlock_EmptyTag_Errors(t *testing.T) {
	_, err := RenderVersiondBlock(newValidState(t), "")
	if err == nil {
		t.Fatal("expected error on empty tag")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "tag") && !strings.Contains(low, "versiond") {
		t.Errorf("error should name 'tag' or 'versiond', got %q", err.Error())
	}
}

// TestRenderVersiondBlock_EmptyKeyName_Errors.
func TestRenderVersiondBlock_EmptyKeyName_Errors(t *testing.T) {
	s := newValidState(t)
	s.KeyringPassword = canaryPassword
	s.KeyName = ""
	_, err := RenderVersiondBlock(s, "v0.2.12")
	if err == nil {
		t.Fatal("expected error on empty KeyName")
	}
	if !strings.Contains(err.Error(), "KeyName") {
		t.Errorf("error should name 'KeyName', got %q", err.Error())
	}
	if strings.Contains(err.Error(), canaryPassword) {
		t.Errorf("error leaked canary password: %q", err.Error())
	}
}

// TestRenderVersiondBlock_EmptyAccountPubKey_Errors.
func TestRenderVersiondBlock_EmptyAccountPubKey_Errors(t *testing.T) {
	s := newValidState(t)
	s.KeyringPassword = canaryPassword
	s.AccountPubKey = ""
	_, err := RenderVersiondBlock(s, "v0.2.12")
	if err == nil {
		t.Fatal("expected error on empty AccountPubKey")
	}
	if !strings.Contains(err.Error(), "AccountPubKey") {
		t.Errorf("error should name 'AccountPubKey', got %q", err.Error())
	}
	if strings.Contains(err.Error(), canaryPassword) {
		t.Errorf("error leaked canary password: %q", err.Error())
	}
}

// TestRenderVersiondBlock_EmptyKeyringPassword_Errors.
func TestRenderVersiondBlock_EmptyKeyringPassword_Errors(t *testing.T) {
	s := newValidState(t)
	s.KeyringPassword = ""
	_, err := RenderVersiondBlock(s, "v0.2.12")
	if err == nil {
		t.Fatal("expected error on empty KeyringPassword")
	}
	if !strings.Contains(err.Error(), "KeyringPassword") {
		t.Errorf("error should name 'KeyringPassword', got %q", err.Error())
	}
}

// TestRenderVersiondBlock_NeverEchoesPassword (NFR-M12-1) — captures stdout
// and stderr around the call and asserts the canary password does NOT
// appear in either stream.
func TestRenderVersiondBlock_NeverEchoesPassword(t *testing.T) {
	s := newValidState(t)
	s.KeyringPassword = canaryPassword

	stdoutBuf, stderrBuf := captureStdouts(t, func() {
		out, err := RenderVersiondBlock(s, "v0.2.12")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, canaryPassword) {
			t.Errorf("expected canary in returned YAML (proves substitution ran), missing")
		}
	})

	if bytes.Contains(stdoutBuf.Bytes(), []byte(canaryPassword)) {
		t.Errorf("canary password leaked to stdout: %q", stdoutBuf.String())
	}
	if bytes.Contains(stderrBuf.Bytes(), []byte(canaryPassword)) {
		t.Errorf("canary password leaked to stderr: %q", stderrBuf.String())
	}
}

// TestRenderVersiondBlock_DollarSignInPassword_NoExpansion — proves
// strings.ReplaceAll (not os.Expand) is used: dollar signs in the password
// must NOT be expanded.
func TestRenderVersiondBlock_DollarSignInPassword_NoExpansion(t *testing.T) {
	s := newValidState(t)
	s.KeyringPassword = "p$1$rotected${HOSTNAME}"
	out, err := RenderVersiondBlock(s, "v0.2.12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "p$1$rotected${HOSTNAME}") {
		t.Errorf("dollar signs in password were expanded (os.Expand bug); got:\n%s", out)
	}
}

// TestRenderVersiondBlock_DollarSignInKeyName_NoExpansion — same property
// for KeyName.
func TestRenderVersiondBlock_DollarSignInKeyName_NoExpansion(t *testing.T) {
	s := newValidState(t)
	s.KeyName = "node-${HOSTNAME}-cold"
	out, err := RenderVersiondBlock(s, "v0.2.12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "node-${HOSTNAME}-cold") {
		t.Errorf("dollar signs in KeyName were expanded; got:\n%s", out)
	}
}

// TestRenderVersiondBlock_PasswordAppearsExactlyOnce — guards against
// future bugs that duplicate the password into a comment or another env line.
func TestRenderVersiondBlock_PasswordAppearsExactlyOnce(t *testing.T) {
	s := newValidState(t)
	s.KeyringPassword = "ONCE_ONLY_AABB"
	out, err := RenderVersiondBlock(s, "v0.2.12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(out, "ONCE_ONLY_AABB"); got != 1 {
		t.Errorf("password appeared %d times in output, want 1\n%s", got, out)
	}
}

// TestRenderVersiondBlock_ErrorMessages_NeverContainSecrets — three error
// paths must not echo state values.
func TestRenderVersiondBlock_ErrorMessages_NeverContainSecrets(t *testing.T) {
	leak := "CANARY_LEAK_2025"

	cases := []struct {
		name   string
		mutate func(s *config.State)
	}{
		{"empty KeyName", func(s *config.State) { s.KeyName = "" }},
		{"empty AccountPubKey", func(s *config.State) { s.AccountPubKey = "" }},
		{"empty KeyringPassword", func(s *config.State) { s.KeyringPassword = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newValidState(t)
			s.KeyringPassword = leak
			tc.mutate(s)
			_, err := RenderVersiondBlock(s, "v0.2.12")
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), leak) {
				t.Errorf("error leaked secret: %q", err.Error())
			}
		})
	}
}

// TestAPIEnvAdditions_AllSevenVarsPresent (TC-COMPOSE-2 part).
func TestAPIEnvAdditions_AllSevenVarsPresent(t *testing.T) {
	for _, want := range []string{
		"DAPI_API__NODE_MANAGER_GRPC_PORT",
		"DAPI_API__NODE_MANAGER_LOCK_TTL_SECONDS",
		"PGHOST",
		"PGPORT",
		"PGDATABASE",
		"PGUSER",
		"PGPASSWORD",
	} {
		if !strings.Contains(APIEnvAdditions, want) {
			t.Errorf("APIEnvAdditions missing env var %q", want)
		}
	}
}

// TestProxyEnvAdditions_ExactValues (TC-COMPOSE-2 main).
func TestProxyEnvAdditions_ExactValues(t *testing.T) {
	for _, want := range []string{
		"chat inference poc/proofs subnet devshard",
		"EXEMPT_CONN_LIMIT=200",
	} {
		if !strings.Contains(ProxyEnvAdditions, want) {
			t.Errorf("ProxyEnvAdditions missing literal %q", want)
		}
	}
}

// captureStdouts redirects os.Stdout and os.Stderr to in-memory buffers
// for the duration of fn, returning the captured contents. Restores
// originals via t.Cleanup.
func captureStdouts(t *testing.T, fn func()) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}

	origOut, origErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origOut, origErr
	})

	doneOut := make(chan struct{})
	doneErr := make(chan struct{})
	go func() { _, _ = io.Copy(stdout, rOut); close(doneOut) }()
	go func() { _, _ = io.Copy(stderr, rErr); close(doneErr) }()

	fn()

	_ = wOut.Close()
	_ = wErr.Close()
	<-doneOut
	<-doneErr
	return stdout, stderr
}
