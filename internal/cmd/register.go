package cmd

import (
	"fmt"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/phases"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

var forceRegister bool

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register node on-chain",
	Long: `Register your node on-chain and grant ML permissions.

This command can be used after initial setup or for re-registration
scenarios (e.g., IP changed, key rotated).

The registration workflow depends on your setup:
  - Quick (mainnet): Automated registration using on-server keys
  - Secure (mainnet): Shows manual commands for local machine
  - Testnet: Automated attempt, falls back to manual if account not funded

Examples:
  gonka-nop register           # Register based on saved state
  gonka-nop register --force   # Re-register even if already registered`,
	RunE: runRegister,
}

func init() {
	registerCmd.Flags().BoolVar(&forceRegister, "force", false, "Force re-registration even if already registered")
}

func runRegister(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	state, err := config.Load(outputDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Validate that setup has been run
	if state.PublicIP == "" {
		return fmt.Errorf("node not configured — run 'gonka-nop setup' first")
	}

	// Check if already registered (unless --force)
	if state.NodeRegistered && !forceRegister {
		ui.Success("Node is already registered")
		ui.Detail("Use --force to re-register")
		return nil
	}

	// Prompt for keyring password (not persisted to disk for security)
	if state.KeyringPassword == "" && state.ColdKeyName != "" {
		password, promptErr := ui.Password("Enter keyring password:")
		if promptErr != nil {
			return fmt.Errorf("password prompt: %w", promptErr)
		}
		state.KeyringPassword = password
	}

	ui.Header("Node Registration")

	phase := phases.NewRegistration()
	if err := phase.Run(ctx, state); err != nil {
		return err
	}

	// Save updated registration state
	if saveErr := state.Save(); saveErr != nil {
		ui.Warn("Could not save state: %v", saveErr)
	}

	return nil
}
