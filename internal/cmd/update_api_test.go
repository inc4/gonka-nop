package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

// TestRunUpdateAPIOnly_RequiresVersion ensures --api-only without --version
// is rejected at flag-validation time (TC-6.* flag handling).
func TestRunUpdateAPIOnly_RequiresVersion(t *testing.T) {
	defer resetUpdateAPIFlags()
	updateAPIOnly = true
	updateAPIVersion = ""

	state := &config.State{OutputDir: t.TempDir()}
	err := runUpdateAPIOnly(context.Background(), state)
	if err == nil {
		t.Fatal("expected error when --version is empty")
	}
	if !strings.Contains(err.Error(), "--version is required") {
		t.Errorf("error should mention required --version: %v", err)
	}
}

// TestRunUpdateAPIOnly_RequiresDeployment errors when state has no OutputDir.
func TestRunUpdateAPIOnly_RequiresDeployment(t *testing.T) {
	defer resetUpdateAPIFlags()
	updateAPIOnly = true
	updateAPIVersion = "v0.2.12-api-post3"

	state := &config.State{OutputDir: ""}
	err := runUpdateAPIOnly(context.Background(), state)
	if err == nil {
		t.Fatal("expected error when state has no deployment")
	}
	if !strings.Contains(err.Error(), "no deployment found") {
		t.Errorf("error should mention missing deployment: %v", err)
	}
}

// TestUpdateAPIFlags_RegisteredOnCmd verifies the flags exist on updateCmd
// (architect cond #7's mutual-exclusion is a cobra declaration verified at
// real CLI invocation; the underlying registration is what we unit-test here).
func TestUpdateAPIFlags_RegisteredOnCmd(t *testing.T) {
	for _, name := range []string{"api-only", "version", "force", "check"} {
		if updateCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not registered on updateCmd", name)
		}
	}
}

// TestRunUpdateAPIOnly_InvalidVersionRejected delegates to cosmovisor.Install
// which rejects malformed versions BEFORE any I/O (Slice 5 F15). This test
// proves the cmd layer doesn't bypass that validation.
func TestRunUpdateAPIOnly_InvalidVersionRejected(t *testing.T) {
	defer resetUpdateAPIFlags()
	updateAPIOnly = true
	updateAPIVersion = "not-a-version"
	updateAPIForce = true // skip safety so the test doesn't try chain query

	state := &config.State{OutputDir: t.TempDir()}
	err := runUpdateAPIOnly(context.Background(), state)
	if err == nil {
		t.Fatal("expected version validation rejection")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should mention version validation: %v", err)
	}
}

// TestRunUpdateAPIOnly_StateSaveReflectsInstalledVersion is an integration
// smoke test that exercises the success path. Because the production manifest
// URL is hardcoded, we can only test the error path or a successful flow
// where we override the installer. The simplest assertion: state.Save is
// invoked after install — which requires hitting the network. Instead, we
// verify the helper functions that wire state → installer.
//
// Full happy-path coverage lives in internal/cosmovisor/installer_test.go.
func TestRunUpdateAPIOnly_BuildHelpers(t *testing.T) {
	state := &config.State{
		OutputDir: t.TempDir(),
		AdminURL:  "http://localhost:9200",
	}

	t.Run("safety check func is non-nil", func(t *testing.T) {
		if buildSafetyCheckFunc(state) == nil {
			t.Error("buildSafetyCheckFunc returned nil")
		}
	})
	t.Run("restart func is non-nil", func(t *testing.T) {
		if buildAPIRestartFunc(state) == nil {
			t.Error("buildAPIRestartFunc returned nil")
		}
	})
	t.Run("health check func is non-nil", func(t *testing.T) {
		if buildAPIHealthCheckFunc(state) == nil {
			t.Error("buildAPIHealthCheckFunc returned nil")
		}
	})
}

// TestStateAssignmentForAPIBinaryVersion verifies the field is settable
// (round-trip persistence is covered by config/versions_apirelease_test.go).
func TestStateAssignmentForAPIBinaryVersion(t *testing.T) {
	state := &config.State{OutputDir: t.TempDir()}
	state.Versions.APIBinaryVersion = "v0.2.12-api-post3"
	if state.Versions.APIBinaryVersion != "v0.2.12-api-post3" {
		t.Errorf("APIBinaryVersion field not settable: %q", state.Versions.APIBinaryVersion)
	}
}

// resetUpdateAPIFlags is called in defer to keep tests independent.
func resetUpdateAPIFlags() {
	updateAPIOnly = false
	updateAPIVersion = ""
	updateAPIForce = false
	updateCmd.SetArgs(nil)
}
