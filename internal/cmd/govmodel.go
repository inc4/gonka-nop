package cmd

import (
	"context"
	"fmt"

	"github.com/inc4/gonka-nop/internal/chain"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

var govmodelForce bool

var govmodelCmd = &cobra.Command{
	Use:   "govmodel",
	Short: "Manage per-governance-model PoC opt-in (delegate / refuse / declare-intent)",
	Long: `Manage your node's per-model participation in the multi-model PoC governance system.

Each governance-approved model (e.g. Qwen/Qwen3-235B-A22B-Instruct-2507-FP8,
moonshotai/Kimi-K2.6) tracks per-participant state on-chain:

  - delegate <model-id> <addr>  → delegate PoC weight for <model> to <addr>
  - refuse <model-id>           → refuse participation in <model>'s PoC
  - declare-intent <model-id>   → bootstrap signal of intent to run <model>
  - list                        → show current per-model state for this node

All tx commands are signed with the cold key (KEY_NAME from config.env) and
refuse to run during PoC / Confirmation PoC windows unless --force is set.

Examples:
  gonka-nop govmodel list
  gonka-nop govmodel refuse moonshotai/Kimi-K2.6
  gonka-nop govmodel delegate moonshotai/Kimi-K2.6 gonka19f9hkpmjaldncsfly4j63sy932y8hughn4l3d8
  gonka-nop govmodel declare-intent future/Model-v3`,
}

var govmodelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List per-model PoC opt-in state for this node",
	RunE:  runGovmodelList,
}

var govmodelDelegateCmd = &cobra.Command{
	Use:   "delegate <model-id> <delegatee-addr>",
	Short: "Delegate PoC weight for <model-id> to <delegatee-addr>",
	Args:  cobra.ExactArgs(2),
	RunE:  runGovmodelDelegate,
}

var govmodelRefuseCmd = &cobra.Command{
	Use:   "refuse <model-id>",
	Short: "Refuse participation in <model-id>'s PoC",
	Args:  cobra.ExactArgs(1),
	RunE:  runGovmodelRefuse,
}

var govmodelIntentCmd = &cobra.Command{
	Use:   "declare-intent <model-id>",
	Short: "Declare bootstrap intent to run <model-id> (does NOT replace actually running PoC)",
	Args:  cobra.ExactArgs(1),
	RunE:  runGovmodelIntent,
}

var (
	candidatesHealthyOnly bool
	candidatesLimit       int
)

var govmodelCandidatesCmd = &cobra.Command{
	Use:   "candidates <model-id>",
	Short: "List on-chain hosts running <model-id>, ranked best-first for delegation",
	Long: `Query the chain for every participant's hardware_nodes and rank those
running <model-id> by health. Use to pick a delegatee for
'gonka-nop govmodel delegate' when you can't run the model locally.

Ranking (best-first):
  1. More INFERENCE nodes win
  2. Fewer FAILED nodes win (tiebreaker)
  3. Bigger fleet wins (tiebreaker)

Examples:
  gonka-nop govmodel candidates moonshotai/Kimi-K2.6
  gonka-nop govmodel candidates moonshotai/Kimi-K2.6 --healthy-only --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: runGovmodelCandidates,
}

func init() {
	govmodelCmd.AddCommand(govmodelListCmd, govmodelDelegateCmd, govmodelRefuseCmd, govmodelIntentCmd, govmodelCandidatesCmd)

	for _, c := range []*cobra.Command{govmodelDelegateCmd, govmodelRefuseCmd, govmodelIntentCmd} {
		c.Flags().BoolVar(&govmodelForce, "force", false, "Skip PoC safety check (dangerous)")
	}

	govmodelCandidatesCmd.Flags().BoolVar(&candidatesHealthyOnly, "healthy-only", false,
		"Hide participants with any FAILED nodes for this model")
	govmodelCandidatesCmd.Flags().IntVar(&candidatesLimit, "limit", 15,
		"Maximum candidates to show (0 = no limit)")
}

func runGovmodelList(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	state, err := loadStateForGovmodel()
	if err != nil {
		return err
	}
	addr := state.ColdKeyAddress
	if addr == "" {
		return fmt.Errorf("cold key address unknown (run 'gonka-nop setup' first)")
	}
	qr := chain.QueryRunnerFromState(state)
	states, err := chain.ListPoCDelegations(ctx, qr, addr)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		ui.Info("No per-model opt-ins recorded for %s", addr)
		return nil
	}
	ui.Header("Per-Model Governance State")
	for _, s := range states {
		switch {
		case s.Refused:
			ui.Detail("%s → REFUSED", s.ModelID)
		case s.IntentOnly:
			ui.Detail("%s → intent (bootstrap signal only)", s.ModelID)
		case s.Delegate != "":
			ui.Detail("%s → delegated to %s", s.ModelID, s.Delegate)
		}
	}
	return nil
}

func runGovmodelDelegate(cmd *cobra.Command, args []string) error {
	modelID, delegatee := args[0], args[1]
	if err := chain.ValidateGonkaAddress(delegatee); err != nil {
		return fmt.Errorf("invalid delegatee address: %w", err)
	}
	return runGovmodelTx(cmd.Context(), "delegate", "set-poc-delegation", modelID, delegatee)
}

func runGovmodelRefuse(cmd *cobra.Command, args []string) error {
	return runGovmodelTx(cmd.Context(), "refuse", "refuse-poc-delegation", args[0])
}

func runGovmodelIntent(cmd *cobra.Command, args []string) error {
	return runGovmodelTx(cmd.Context(), "declare-intent", "declare-poc-intent", args[0])
}

func runGovmodelCandidates(cmd *cobra.Command, args []string) error {
	modelID := args[0]
	state, err := loadStateForGovmodel()
	if err != nil {
		return err
	}
	qr := chain.QueryRunnerFromState(state)
	entries, err := chain.ListHardwareNodesAll(cmd.Context(), qr)
	if err != nil {
		return fmt.Errorf("query hardware-nodes-all: %w", err)
	}
	cands := chain.RankCandidatesForModel(entries, modelID)
	if len(cands) == 0 {
		ui.Warn("No participants on chain are running %q.", modelID)
		ui.Info("Check available governance models with: gonka-nop status")
		return nil
	}
	if candidatesHealthyOnly {
		filtered := cands[:0]
		for _, c := range cands {
			if c.IsHealthy() {
				filtered = append(filtered, c)
			}
		}
		cands = filtered
		if len(cands) == 0 {
			ui.Warn("No fully-healthy candidates found for %q (all have at least one FAILED node).", modelID)
			ui.Info("Re-run without --healthy-only to see mixed-health candidates.")
			return nil
		}
	}
	limit := candidatesLimit
	if limit > 0 && len(cands) > limit {
		cands = cands[:limit]
	}

	ui.Header(fmt.Sprintf("Delegation candidates for %s", modelID))
	for i, c := range cands {
		status := "✓"
		if !c.IsHealthy() {
			status = "⚠"
		}
		fmt.Printf(" %s %2d. %s\n", status, i+1, c.Participant)
		nodeSummary := fmt.Sprintf("    %d INFERENCE", c.InferenceNodes)
		if c.FailedNodes > 0 {
			nodeSummary += fmt.Sprintf(" · %d FAILED", c.FailedNodes)
		}
		if c.OtherNodes > 0 {
			nodeSummary += fmt.Sprintf(" · %d other", c.OtherNodes)
		}
		fmt.Println(nodeSummary)
		if c.Hardware != "" {
			fmt.Printf("    %s\n", c.Hardware)
		}
	}
	fmt.Println()
	ui.Info("Delegate with:  gonka-nop govmodel delegate %s <participant-address>", modelID)
	return nil
}

// runGovmodelTx is the shared shell for the three tx commands.
//
// Order of operations (architect cond #9):
//  1. Load state
//  2. PoCSafetyChecker.Check() — fail-closed if unsafe (unless --force)
//  3. ONLY THEN load keyring password (avoid decrypting if we'd refuse anyway)
//  4. Broadcast via TxBroadcaster
func runGovmodelTx(ctx context.Context, verb, action string, args ...string) error {
	state, err := loadStateForGovmodel()
	if err != nil {
		return err
	}

	// Step 1: safety gate (BEFORE keyring password decryption — architect cond #9)
	if !govmodelForce {
		checker := chain.NewPoCSafetyChecker(state)
		c, qErr := checker.Check(ctx)
		if qErr != nil {
			return fmt.Errorf("safety check (fail-closed): %w", qErr)
		}
		if !c.Safe {
			return fmt.Errorf("unsafe to %s during current chain phase: %s\n  Use --force to override (may miss PoC)", verb, c.Reason)
		}
	} else {
		ui.Warn("--force: skipping PoC safety check; this may cause missed inference requests")
	}

	// Step 2: load keyring password
	signerKey := state.ColdKeyName
	if signerKey == "" {
		return fmt.Errorf("cold key name unknown (run 'gonka-nop setup' first)")
	}
	password := state.KeyringPassword
	if password == "" {
		password = chain.LoadKeyringPassword(state)
	}
	if password == "" {
		// Non-interactive contexts (SSH scripts) must provide via config.env
		// rather than prompt; surface a clear actionable error.
		return fmt.Errorf("keyring password not set (add KEYRING_PASSWORD=... to config.env or pass via --yes flow)")
	}

	// Step 3: broadcast
	b := chain.NewDefaultTxBroadcaster(state)
	out, err := b.Broadcast(ctx, signerKey, password, action, args...)
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", verb, err, out)
	}
	ui.Success("%s broadcast for %s", verb, args[0])
	if out != "" {
		ui.Detail("%s", out)
	}
	return nil
}

func loadStateForGovmodel() (*config.State, error) {
	state, err := config.Load(outputDir)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if state.OutputDir == "" {
		return nil, fmt.Errorf("no deployment found — run 'gonka-nop setup' first")
	}
	return state, nil
}
