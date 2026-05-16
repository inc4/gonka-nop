package cmd

import (
	"context"
	"fmt"

	"github.com/inc4/gonka-nop/internal/chain"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

var collateralCmd = &cobra.Command{
	Use:   "collateral",
	Short: "Show, deposit, or withdraw participant collateral",
	Long: `Manage your participant collateral on the Gonka chain.

Collateral is a per-participant stake that affects your eligibility to
participate in PoC validation. The current "Collateral %" shown on the
dashboard derives from your deposited amount vs the chain-configured
minimum required for active participation.

Examples:
  gonka-nop collateral show
  gonka-nop collateral deposit 1000000ngonka
  gonka-nop collateral withdraw 500000ngonka   # subject to unbonding period

All tx commands sign with the cold key (KEY_NAME from config.env).
Amount MUST include the denomination (e.g. "1000000ngonka").
Withdrawals enter an unbonding period (mainnet: 1 epoch).`,
}

var collateralShowCmd = &cobra.Command{
	Use:   "show [participant-addr]",
	Short: "Show current collateral balance (defaults to our cold key)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCollateralShow,
}

var collateralDepositCmd = &cobra.Command{
	Use:   "deposit <amount>",
	Short: "Deposit collateral (amount must include denom, e.g. 1000000ngonka)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCollateralDeposit,
}

var collateralWithdrawCmd = &cobra.Command{
	Use:   "withdraw <amount>",
	Short: "Start unbonding of collateral (released after the unbonding period)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCollateralWithdraw,
}

func init() {
	collateralCmd.AddCommand(collateralShowCmd, collateralDepositCmd, collateralWithdrawCmd)
}

func runCollateralShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	state, err := loadStateForCollateral()
	if err != nil {
		return err
	}
	addr := state.ColdKeyAddress
	if len(args) == 1 {
		addr = args[0]
	}
	if addr == "" {
		return fmt.Errorf("cold key address unknown — pass participant addr explicitly or run 'gonka-nop setup' first")
	}

	bal, err := chain.GetCollateral(ctx, chain.QueryRunnerFromState(state), addr)
	if err != nil {
		return err
	}

	ui.Header("Collateral")
	ui.Detail("participant: %s", addr)
	if !bal.Found {
		ui.Warn("No collateral deposited (chain returned NotFound)")
		ui.Info("To deposit: gonka-nop collateral deposit <amount> (e.g. 1000000ngonka)")
		return nil
	}
	ui.Success("amount: %d %s", bal.Amount, bal.Denom)
	return nil
}

func runCollateralDeposit(cmd *cobra.Command, args []string) error {
	return runCollateralTx(cmd.Context(), "deposit", "deposit-collateral", args[0])
}

func runCollateralWithdraw(cmd *cobra.Command, args []string) error {
	return runCollateralTx(cmd.Context(), "withdraw", "withdraw-collateral", args[0])
}

// runCollateralTx is the shared shell for the two tx commands.
//
// Collateral txs are not gated on PoC phase — they're independent of
// validation. We do verify the amount string format defensively before
// constructing the docker exec invocation (Slice 7 govmodel pattern).
func runCollateralTx(ctx context.Context, verb, action, amount string) error {
	if err := chain.ValidateCoinAmount(amount); err != nil {
		return fmt.Errorf("invalid amount: %w", err)
	}

	state, err := loadStateForCollateral()
	if err != nil {
		return err
	}

	signerKey := state.ColdKeyName
	if signerKey == "" {
		return fmt.Errorf("cold key name unknown (run 'gonka-nop setup' first)")
	}
	password := state.KeyringPassword
	if password == "" {
		password = chain.LoadKeyringPassword(state)
	}
	if password == "" {
		return fmt.Errorf("keyring password not set (add KEYRING_PASSWORD=... to config.env)")
	}

	// Use BroadcastModule since "collateral" is its own cosmos module
	// (not a subcommand of `tx inference`).
	b := chain.NewDefaultTxBroadcaster(state).(chain.TxModuleBroadcaster)
	out, err := b.BroadcastModule(ctx, "collateral", signerKey, password, action, amount)
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", verb, err, out)
	}
	ui.Success("%s broadcast: %s", verb, amount)
	if out != "" {
		ui.Detail("%s", out)
	}
	return nil
}

func loadStateForCollateral() (*config.State, error) {
	state, err := config.Load(outputDir)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if state.OutputDir == "" {
		return nil, fmt.Errorf("no deployment found — run 'gonka-nop setup' first")
	}
	return state, nil
}
