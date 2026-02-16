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
	mockedSetup   bool

	// Non-interactive flags
	yesFlag         bool
	flagNetwork     string
	flagKeyWorkflow string
	flagKeyName     string
	flagKeyringPass string
	flagPublicIP    string
	flagHFHome      string
	flagPorts       string
	flagExtP2PPort  string
	flagExtAPIPort  string
	flagIntP2PPort  string
	flagIntAPIPort  string
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
  gonka-nop setup --account-pubkey=<key>  # Provide account key
  gonka-nop setup --mocked           # Demo mode with mocked data

  # Non-interactive setup (for scripting / SSH):
  gonka-nop setup -y --network testnet --key-workflow quick \
    --key-name mynode --keyring-password pass123 \
    --public-ip 1.2.3.4 --ports custom \
    --ext-p2p-port 19245 --ext-api-port 19246`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().StringVar(&accountPubKey, "account-pubkey", "", "Account public key (for secure setup)")
	setupCmd.Flags().BoolVar(&mockedSetup, "mocked", false, "Use mocked data (demo mode)")

	// Non-interactive flags
	setupCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Non-interactive mode (auto-accept confirmations)")
	setupCmd.Flags().StringVar(&flagNetwork, "network", "", "Network selection (mainnet or testnet)")
	setupCmd.Flags().StringVar(&flagKeyWorkflow, "key-workflow", "", "Key management workflow (quick or secure)")
	setupCmd.Flags().StringVar(&flagKeyName, "key-name", "", "Base name for keys")
	setupCmd.Flags().StringVar(&flagKeyringPass, "keyring-password", "", "Keyring password")
	setupCmd.Flags().StringVar(&flagPublicIP, "public-ip", "", "Server public IP or hostname")
	setupCmd.Flags().StringVar(&flagHFHome, "hf-home", "", "HuggingFace cache directory")
	setupCmd.Flags().StringVar(&flagPorts, "ports", "", "Port configuration mode (default or custom)")
	setupCmd.Flags().StringVar(&flagExtP2PPort, "ext-p2p-port", "", "External P2P port (NAT)")
	setupCmd.Flags().StringVar(&flagExtAPIPort, "ext-api-port", "", "External API port (NAT)")
	setupCmd.Flags().StringVar(&flagIntP2PPort, "int-p2p-port", "", "Internal P2P port (Docker binding)")
	setupCmd.Flags().StringVar(&flagIntAPIPort, "int-api-port", "", "Internal API port (Docker binding)")
}

// setupOverrides maps CLI flag values to ui prompt overrides.
func setupOverrides() {
	ui.SetNonInteractive(true)

	overrides := []struct {
		flag, prompt string
	}{
		{flagNetwork, "Select network"},
		{flagKeyWorkflow, "key management workflow"},
		{flagKeyName, "base name"},
		{flagKeyringPass, "keyring password"},
		{flagPublicIP, "public IP"},
		{flagHFHome, "HuggingFace"},
		{flagPorts, "port configuration"},
		{flagExtP2PPort, "External P2P"},
		{flagExtAPIPort, "External API"},
		{flagIntP2PPort, "Internal P2P"},
		{flagIntAPIPort, "Internal API"},
	}
	for _, o := range overrides {
		if o.flag != "" {
			ui.SetOverride(o.prompt, o.flag)
		}
	}

	// key-name also matches "name for your server" prompt (secure workflow)
	if flagKeyName != "" {
		ui.SetOverride("name for your server", flagKeyName)
	}

	// keyring-password also matches generic "password" prompt
	if flagKeyringPass != "" {
		ui.SetOverride("password", flagKeyringPass)
	}
}

func runSetup(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Enable non-interactive mode if --yes flag is set
	if yesFlag {
		setupOverrides()
	}

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
	if mockedSetup {
		ui.Info("Running in demo mode (--mocked)")
	}
	if yesFlag {
		ui.Info("Running in non-interactive mode (--yes)")
	}

	if len(state.CompletedPhases) > 0 {
		ui.Info("Resuming from previous run (%d phases completed)", len(state.CompletedPhases))
	}

	// Build phase list
	phaseList := []phases.Phase{
		phases.NewPrerequisites(mockedSetup),
		phases.NewGPUDetection(mockedSetup),
		phases.NewNetworkSelect(),
		phases.NewKeyManagement(state.KeyWorkflow, mockedSetup),
		phases.NewConfigGeneration(),
		phases.NewDeploy(),
		phases.NewRegistration(),
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
