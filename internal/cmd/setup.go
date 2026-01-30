package cmd

import (
	"fmt"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/phases"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

var (
	accountPubKey string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run the setup wizard",
	Long: `Run the interactive setup wizard to configure and deploy your Gonka node.

This will guide you through:
  - Checking prerequisites (Docker, NVIDIA drivers)
  - Detecting GPU hardware
  - Selecting network and generating keys
  - Creating configuration files
  - Starting node containers

Examples:
  gonka-nop setup                    # Interactive setup
  gonka-nop setup -o /opt/gonka      # Custom output directory
  gonka-nop setup --account-pubkey=<key>  # Provide account key`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().StringVar(&accountPubKey, "account-pubkey", "", "Account public key (for secure setup)")
}

func runSetup(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Load or create state
	state, err := config.Load(outputDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Set account pubkey if provided
	if accountPubKey != "" {
		state.AccountPubKey = accountPubKey
	}

	ui.Header("Gonka Node Setup")
	ui.Info("Output directory: %s", outputDir)

	if len(state.CompletedPhases) > 0 {
		ui.Info("Resuming from previous run (%d phases completed)", len(state.CompletedPhases))
	}

	// Build phase list
	phaseList := []phases.Phase{
		phases.NewPrerequisites(),
		phases.NewGPUDetection(),
		phases.NewNetworkSelect(),
		phases.NewKeyManagement(state.KeyWorkflow),
		phases.NewConfigGeneration(),
		phases.NewDeploy(),
	}

	// Create and run phase runner
	runner := phases.NewRunner(phaseList, state)

	if err := runner.Run(ctx); err != nil {
		return err
	}

	// Final success message
	fmt.Println()
	ui.Success("Setup complete!")
	ui.Info("Your Gonka node is now running.")
	ui.Info("Check status with: gonka-nop status")

	return nil
}
