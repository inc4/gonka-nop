package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/inc4/gonka-nop/internal/chain"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/cosmovisor"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
)

// updateAPIOnlyFlags state for the --api-only flow.
var (
	updateAPIOnly    bool
	updateAPIVersion string
	updateAPIForce   bool
)

// registerUpdateAPIFlags is called from update.go init(); kept in a separate
// file so the api-only feature is self-contained and easy to grep.
func registerUpdateAPIFlags() {
	updateCmd.Flags().BoolVar(&updateAPIOnly, "api-only", false, "Install only an api-binary Cosmovisor hotfix (skips image diff and other services)")
	updateCmd.Flags().StringVar(&updateAPIVersion, "version", "", "Exact api-binary version to install (e.g. v0.2.12-api-post3); required with --api-only")
	updateCmd.Flags().BoolVar(&updateAPIForce, "force", false, "Skip PoC safety check (--api-only only); dangerous — may cause missed inference requests")
	// Architect condition #7: --api-only and --check are mutually exclusive.
	updateCmd.MarkFlagsMutuallyExclusive("api-only", "check")
}

// runUpdateAPIOnly is the entrypoint for `gonka-nop update --api-only --version X`.
// Called from runUpdate's flag-dispatch when updateAPIOnly is true.
func runUpdateAPIOnly(ctx context.Context, state *config.State) error {
	if updateAPIVersion == "" {
		return fmt.Errorf("--version is required with --api-only (e.g. --version v0.2.12-api-post3)")
	}
	if state.OutputDir == "" {
		return fmt.Errorf("no deployment found — run 'gonka-nop setup' first")
	}

	ui.Header("API Binary Hotfix Install")
	ui.Info("Version: %s", updateAPIVersion)
	ui.Info("Manifest: %s", config.RaceReleasesManifestURL)
	if updateAPIForce {
		ui.Warn("--force enabled: PoC safety check will be SKIPPED")
		ui.Warn("This may cause missed inference requests if PoC is in progress")
	}

	installer := cosmovisor.NewAPIInstaller()
	opts := cosmovisor.InstallOpts{
		Version:         updateAPIVersion,
		DeployDir:       state.OutputDir,
		ManifestURL:     config.RaceReleasesManifestURL,
		Force:           updateAPIForce,
		Safety:          buildSafetyCheckFunc(state),
		RestartFunc:     buildAPIRestartFunc(state),
		HealthCheckFunc: buildAPIHealthCheckFunc(state),
	}

	result, err := installer.Install(ctx, opts)
	if err != nil {
		if result != nil && result.RolledBack {
			ui.Error("Install failed and was rolled back to %s", result.PreviousVersion)
		}
		return err
	}

	state.Versions.APIBinaryVersion = result.InstalledVersion
	if saveErr := state.Save(); saveErr != nil {
		ui.Warn("Install succeeded but state save failed: %v", saveErr)
	}

	ui.Success("API binary upgraded to %s", result.InstalledVersion)
	if result.PreviousVersion != "" {
		ui.Detail("Previous: %s", result.PreviousVersion)
	}
	ui.Info("Verify with: gonka-nop status")
	return nil
}

// buildSafetyCheckFunc bridges chain.PoCSafetyChecker into the
// cosmovisor.SafetyCheckFunc shape (keeps cosmovisor independent of chain).
func buildSafetyCheckFunc(state *config.State) cosmovisor.SafetyCheckFunc {
	return func(ctx context.Context) (safe bool, reason string, err error) {
		checker := chain.NewPoCSafetyChecker(state)
		c, qErr := checker.Check(ctx)
		if qErr != nil {
			return false, "safety check internal error", qErr
		}
		return c.Safe, c.Reason, nil
	}
}

// buildAPIRestartFunc returns a closure that restarts the api container via
// `docker [compose] restart api` (with sudo when state.UseSudo).
func buildAPIRestartFunc(state *config.State) cosmovisor.RestartFunc {
	return func(ctx context.Context) error {
		var name string
		args := []string{"compose", "restart", "api"}
		if state.UseSudo {
			name = docker.CmdSudo
			args = append([]string{"-E", docker.CmdDocker}, args...)
		} else {
			name = docker.CmdDocker
		}
		cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 - state-controlled
		cmd.Dir = state.OutputDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker compose restart api: %w\n%s", err, string(out))
		}
		return nil
	}
}

// buildAPIHealthCheckFunc polls the api admin URL until the setup report
// returns OK (or context deadline). The simplest health predicate that
// catches "api never came up" without requiring the report to declare any
// specific version.
func buildAPIHealthCheckFunc(state *config.State) cosmovisor.HealthCheckFunc {
	return func(ctx context.Context) error {
		adminURL := state.AdminURL
		if adminURL == "" {
			adminURL = "http://localhost:9200"
		}
		client := &http.Client{Timeout: 5 * time.Second}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("api never returned 200 from setup/report within deadline: %w", ctx.Err())
			case <-ticker.C:
			}
		}
	}
}
