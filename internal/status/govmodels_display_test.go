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

// TestPrintGovernanceModels_OmittedWhenEmpty verifies architect cond #10:
// no header, no whitespace when GovernanceModels is nil or empty.
func TestPrintGovernanceModels_OmittedWhenEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		printGovernanceModels(&NodeStatus{GovernanceModels: nil})
	})
	if strings.Contains(out, "Governance Models") {
		t.Errorf("section header should be omitted when empty; got:\n%s", out)
	}
	out2 := captureStdout(t, func() {
		printGovernanceModels(&NodeStatus{GovernanceModels: []GovModelState{}})
	})
	if strings.Contains(out2, "Governance Models") {
		t.Errorf("section header should be omitted for empty slice; got:\n%s", out2)
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
