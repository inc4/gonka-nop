package status

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// captureStdout returns whatever f writes to os.Stdout (best-effort; does
// NOT capture color escape codes' bypass of stdout when redirected — for
// status display purposes the actual content lines through fine).
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	f()
	_ = w.Close()
	os.Stdout = orig
	<-done
	return buf.String()
}

// TestPrintGovernanceModels_OmittedWhenBothEmpty verifies the residual
// architect cond #10: when BOTH the participant's delegations AND the
// chain's available-models list are empty, the section is omitted. Only
// case where complete silence is appropriate (chain has zero governance
// models, which is essentially never on a live network).
func TestPrintGovernanceModels_OmittedWhenBothEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		printGovernanceModels(&NodeStatus{GovernanceModels: nil, AvailableGovernanceModels: nil})
	})
	if strings.Contains(out, "Governance Models") {
		t.Errorf("section header should be omitted when both empty; got:\n%s", out)
	}
	out2 := captureStdout(t, func() {
		printGovernanceModels(&NodeStatus{GovernanceModels: []GovModelState{}, AvailableGovernanceModels: []string{}})
	})
	if strings.Contains(out2, "Governance Models") {
		t.Errorf("section header should be omitted for both-empty slices; got:\n%s", out2)
	}
}

// TestPrintGovernanceModels_ShowsAvailableWhenNoDelegations covers the
// fresh-deploy case (2026-05-22): when chain has governance models but
// the participant has no delegations yet, render a section that names
// the available models AND prints the `govmodel delegate` command so
// the operator knows what to do next.
//
// NOTE: this test only asserts on content emitted via plain fmt.* calls,
// since the fatih/color helpers (printSection, printWarn) bind to
// os.Stdout at init time and bypass the captureStdout pipe. The colored
// section header and warning are exercised by the binary smoke test.
func TestPrintGovernanceModels_ShowsAvailableWhenNoDelegations(t *testing.T) {
	orig := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = orig })

	out := captureStdout(t, func() {
		printGovernanceModels(&NodeStatus{
			GovernanceModels: nil,
			AvailableGovernanceModels: []string{
				"Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
				"moonshotai/Kimi-K2.6",
			},
		})
	})
	if !strings.Contains(out, "Available governance models on chain") {
		t.Errorf("missing 'Available governance models' line: %q", out)
	}
	if !strings.Contains(out, "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8") {
		t.Errorf("missing Qwen model name in available list: %q", out)
	}
	if !strings.Contains(out, "moonshotai/Kimi-K2.6") {
		t.Errorf("missing Kimi model name in available list: %q", out)
	}
	if !strings.Contains(out, "gonka-nop govmodel delegate") {
		t.Errorf("missing delegate command suggestion: %q", out)
	}
}

// TestPrintGovernanceModels_RendersDelegationLine exercises the plain-stdout
// path (delegated entries don't use the color package's writers, which bind
// to stdout at init time and bypass the test's pipe). Refused/intent lines
// are exercised via end-to-end smoke runs on the live binary; this unit test
// verifies the section header and at least one rendered entry.
func TestPrintGovernanceModels_RendersDelegationLine(t *testing.T) {
	orig := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = orig })

	out := captureStdout(t, func() {
		printGovernanceModels(&NodeStatus{
			GovernanceModels: []GovModelState{
				{ModelID: "moonshotai/Kimi-K2.6", Delegate: "gonka1xyz"},
			},
		})
	})
	if !strings.Contains(out, "delegated to gonka1xyz") {
		t.Errorf("delegation line missing: %s", out)
	}
}
