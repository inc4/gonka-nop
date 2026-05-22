// M12 Slice 8a — `gonka-nop repair-permissions` command.
// Queries /admin/v1/setup/report → identifies missing ml-ops permissions
// for the warm key → broadcasts a SINGLE grant-ml-ops-permissions tx
// (NEVER register-new-participant). Idempotent (NFR-M12-5): when all
// already granted, exits success without broadcasting.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/inc4/gonka-nop/internal/chain"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/status"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

// defaultGrantFee is the conservative ngonka estimate for one
// grant-ml-ops-permissions tx. Used by the balance pre-check.
// Mainnet fees historically cost ~0 for chain inference module txs;
// keep a non-zero floor so insufficient-balance is catchable.
const defaultGrantFee uint64 = 200_000

var repairPermissionsYes bool

var repairPermissionsCmd = &cobra.Command{
	Use:   "repair-permissions",
	Short: "Re-grant missing ml-ops permissions to the warm key",
	Long: `Query /admin/v1/setup/report, identify which ml-ops permissions
are missing on the warm key, and submit a single grant-ml-ops-permissions
transaction containing ONLY the missing entries.

Idempotent: when all permissions are already granted, exits success
without broadcasting any transaction.

This command NEVER calls register-new-participant (which would error
with chain code 1110 "account already exists" on existing participants).

Examples:
  gonka-nop repair-permissions          # interactive (confirms before broadcast)
  gonka-nop repair-permissions --yes    # non-interactive`,
	RunE: runRepairPermissions,
}

func init() {
	repairPermissionsCmd.Flags().BoolVarP(&repairPermissionsYes, "yes", "y", false, "Skip confirmation prompts")
}

//nolint:gocyclo // linear flow: load state → fetch report → diff → confirm → broadcast → re-verify. Splitting hurts readability.
func runRepairPermissions(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	if repairPermissionsYes {
		ui.SetNonInteractive(true)
	}

	state, err := config.Load(outputDir)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if state.OutputDir == "" {
		return fmt.Errorf("no deployment found in %s — run 'gonka-nop setup' first", outputDir)
	}
	if state.ColdKeyName == "" {
		return fmt.Errorf("state.ColdKeyName is empty — run 'gonka-nop register' first")
	}
	if state.WarmKeyAddress == "" {
		return fmt.Errorf("state.WarmKeyAddress is empty — run 'gonka-nop register' first")
	}
	if state.KeyringPassword == "" {
		return fmt.Errorf("state.KeyringPassword is empty — KEYRING_PASSWORD missing from config.env")
	}

	// 1. Fetch setup report
	adminURL := state.AdminURL
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	report, err := fetchSetupReport(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("fetch setup report: %w", err)
	}

	// 2. Diff permissions
	missing, err := chain.DiffPermissions(report)
	if err != nil {
		return fmt.Errorf("diff permissions: %w", err)
	}
	if len(missing) == 0 {
		ui.Success("All 27/27 permissions already granted — nothing to do.")
		return nil
	}

	ui.Info("%d permission(s) missing on warm key %s:", len(missing), state.WarmKeyAddress)
	for _, p := range missing {
		ui.Detail("  - %s", p.Action)
	}

	if !repairPermissionsYes {
		confirm, cerr := ui.Confirm("Broadcast grant-ml-ops-permissions tx?", true)
		if cerr != nil {
			return cerr
		}
		if !confirm {
			ui.Info("Repair canceled.")
			return nil
		}
	}

	// 3. Build broadcaster + balance querier from production wiring
	broadcaster := chain.NewDefaultTxBroadcaster(state)
	balancer := chain.NewBalanceQuerier(chain.QueryRunnerFromState(state))

	// 4. Cold key signs the grant (grantor = cold, grantee = warm address)
	tx, err := chain.GrantMissing(
		ctx, broadcaster, balancer,
		state.ColdKeyName, state.ColdKeyAddress, state.KeyringPassword,
		state.WarmKeyAddress, missing, defaultGrantFee,
	)
	if err != nil {
		return fmt.Errorf("grant-ml-ops-permissions: %w", err)
	}
	ui.Success("Broadcast OK: %s", tx)
	ui.Detail("Re-querying setup/report to verify…")

	// 5. Verify
	verify, err := fetchSetupReport(ctx, adminURL)
	if err != nil {
		ui.Warn("Could not re-verify setup/report: %v", err)
		ui.Info("Run 'gonka-nop status' shortly to confirm 27/27 granted.")
		return nil
	}
	remaining, _ := chain.DiffPermissions(verify)
	if len(remaining) == 0 {
		ui.Success("Verified: 27/27 permissions granted")
	} else {
		ui.Warn("%d permission(s) still missing — tx may need a block to propagate", len(remaining))
	}
	return nil
}

// fetchSetupReport GETs /admin/v1/setup/report and unmarshals.
func fetchSetupReport(ctx context.Context, adminURL string) (*status.SetupReport, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("admin API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("admin API returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var report status.SetupReport
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("parse setup report: %w", err)
	}
	return &report, nil
}
