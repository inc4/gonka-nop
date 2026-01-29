package cmd

import (
	"fmt"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/phases"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

var (
	quick         bool
	secure        bool
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
  gonka-nop setup --quick            # Quick setup (all keys on server)
  gonka-nop setup --secure           # Secure setup (account key separate)
  gonka-nop setup -o /opt/gonka      # Custom output directory`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&quick, "quick", false, "Quick setup: generate all keys on this machine")
	setupCmd.Flags().BoolVar(&secure, "secure", false, "Secure setup: use account key from separate machine")
	setupCmd.Flags().StringVar(&accountPubKey, "account-pubkey", "", "Account public key (for secure setup)")
}

func runSetup(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Load or create state
	state, err := config.Load(outputDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Set workflow from flags
	if quick {
		state.KeyWorkflow = "quick"
	} else if secure {
		state.KeyWorkflow = "secure"
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
