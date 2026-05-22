package phases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/chain"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	statusPass           = "PASS"
	registrationTimeout  = 2 * time.Minute
	registrationPollTime = 10 * time.Second
)

// Registration handles on-chain node registration and ML permissions.
type Registration struct{}

// NewRegistration creates a new Registration phase.
func NewRegistration() *Registration {
	return &Registration{}
}

func (p *Registration) Name() string {
	return "Registration"
}

func (p *Registration) Description() string {
	return "Registering node on-chain and granting ML permissions"
}

func (p *Registration) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *Registration) Run(ctx context.Context, state *config.State) error {
	adminURL := state.AdminURL
	if adminURL == "" {
		adminURL = defaultAdminURL
	}

	// Build public URL early (needed for manual instructions even if API isn't ready)
	state.PublicURL = fmt.Sprintf("http://%s:%d", state.PublicIP, state.APIPort)

	// Quick check if Admin API is responsive — don't wait, just probe once.
	// On fresh deploys the node syncs for hours; no point blocking setup.
	if !p.isAPIReady(ctx, adminURL) {
		ui.Warn("Admin API is not responsive yet (node is likely still syncing)")
		ui.Info("Once the node is synced, register with: gonka-nop register")
		ui.Detail("Monitor sync progress with: gonka-nop status")
		return nil
	}

	// Fetch consensus key for display and verification
	consensusKey, err := chain.FetchConsensusKey(ctx, adminURL, state)
	if err != nil {
		ui.Warn("Could not fetch consensus key: %v", err)
		ui.Detail("The consensus key may become available after the node finishes syncing")
	} else {
		state.ConsensusKey = consensusKey
		ui.Detail("Consensus key: %s", consensusKey)
	}

	// Route to correct workflow
	if state.IsTestNet {
		return p.runTestnet(ctx, state)
	}
	if state.KeyWorkflow == workflowSecure {
		return p.runSecure(ctx, state)
	}
	return p.runQuick(ctx, state)
}

// runQuick automates registration for mainnet + quick workflow (cold key on server).
func (p *Registration) runQuick(ctx context.Context, state *config.State) error {
	ui.Header("Automated Registration")

	// Validate required fields
	if state.AccountPubKey == "" {
		return fmt.Errorf("account public key not set — run setup from Phase 4")
	}
	if state.PublicIP == "" {
		return fmt.Errorf("public IP not set — run setup from Phase 5")
	}

	// Step 1: Register node on-chain
	ui.Info("Registering node on-chain...")
	seedURL := state.SeedAPIURL
	if seedURL == "" {
		seedURL = config.MainnetConfig().SeedAPIURL
	}

	registerCmd := fmt.Sprintf(
		`inferenced register-new-participant %s %s --node-address %s`,
		state.PublicURL, state.AccountPubKey, seedURL,
	)
	// Always pass --consensus-key since docker compose run --no-deps cannot
	// auto-fetch from DAPI_CHAIN_NODE__URL (node container is not linked).
	if state.ConsensusKey != "" {
		registerCmd += fmt.Sprintf(` --consensus-key %s`, state.ConsensusKey)
	}
	ui.Detail("Command: %s", registerCmd)

	err := ui.WithSpinner("Registering node on-chain", func() error {
		_, execErr := chain.RunComposeExec(ctx, state, "api", registerCmd)
		return execErr
	})
	if err != nil {
		return fmt.Errorf("register node: %w", err)
	}
	ui.Success("Node registration submitted")

	// Step 2: Grant ML permissions
	if state.ColdKeyName == "" || state.WarmKeyAddress == "" {
		ui.Warn("Cold key name or warm key address not set — skipping grant-ml-ops-permissions")
		ui.Detail("Run manually: inferenced tx inference grant-ml-ops-permissions <cold-key> <warm-address> ...")
		state.NodeRegistered = true
		return nil
	}
	if state.KeyringPassword == "" {
		state.KeyringPassword = chain.LoadKeyringPassword(state)
	}

	ui.Info("Granting ML operations permissions...")
	nodeURL := seedURL + "/chain-rpc/"
	chainID := state.ChainID
	if chainID == "" {
		chainID = config.MainnetConfig().ChainID
	}

	grantCmd := fmt.Sprintf(
		`printf '%%s\n' '%s' | inferenced tx inference grant-ml-ops-permissions %s %s --from %s --keyring-backend file --gas 2000000 --node %s --chain-id %s --yes`,
		state.KeyringPassword, state.ColdKeyName, state.WarmKeyAddress,
		state.ColdKeyName, nodeURL, chainID,
	)

	err = ui.WithSpinner("Granting ML permissions", func() error {
		_, execErr := chain.RunComposeExec(ctx, state, "api", grantCmd)
		return execErr
	})
	if err != nil {
		ui.Warn("Grant ML permissions failed: %v", err)
		ui.Detail("You can retry with: gonka-nop register --force")
		ui.Detail("Or run manually inside the api container")
		state.NodeRegistered = true
		return nil
	}
	ui.Success("ML permissions granted")

	// Step 3: Verify registration
	adminURL := state.AdminURL
	if adminURL == "" {
		adminURL = defaultAdminURL
	}

	ui.Info("Verifying registration...")
	if err := WaitForRegistration(ctx, adminURL, registrationTimeout); err != nil {
		ui.Warn("Registration verification timed out: %v", err)
		ui.Detail("The node may need time to sync before checks pass")
		ui.Detail("Check status with: gonka-nop status")
	} else {
		ui.Success("Registration verified — node is registered on-chain")
	}

	state.NodeRegistered = true
	state.PermGranted = true
	return nil
}

// runSecure shows manual instructions for mainnet + secure workflow.
func (p *Registration) runSecure(_ context.Context, state *config.State) error {
	ui.Header("Manual Registration Required")
	ui.Info("Your account key is on a separate machine (secure workflow)")
	ui.Info("Run the following commands from your LOCAL machine with the account key:")

	fmt.Println()
	seedURL := state.SeedAPIURL
	if seedURL == "" {
		seedURL = config.MainnetConfig().SeedAPIURL
	}
	chainID := state.ChainID
	if chainID == "" {
		chainID = config.MainnetConfig().ChainID
	}

	// Registration command
	ui.Header("Step 1: Register Node")
	fmt.Printf("  inferenced tx inference submit-new-participant \\\n")
	fmt.Printf("    %s \\\n", state.PublicURL)
	if state.ConsensusKey != "" {
		fmt.Printf("    --validator-key %s \\\n", state.ConsensusKey)
	} else {
		fmt.Printf("    --validator-key <consensus-key-from-setup-report> \\\n")
	}
	fmt.Printf("    --from <your-account-key-name> \\\n")
	fmt.Printf("    --keyring-backend file \\\n")
	fmt.Printf("    --node %s/chain-rpc/ \\\n", seedURL)
	fmt.Printf("    --chain-id %s\n", chainID)

	// Grant ML permissions
	fmt.Println()
	ui.Header("Step 2: Grant ML Permissions")
	fmt.Printf("  inferenced tx inference grant-ml-ops-permissions \\\n")
	fmt.Printf("    <your-account-key-name> %s \\\n", state.WarmKeyAddress)
	fmt.Printf("    --from <your-account-key-name> \\\n")
	fmt.Printf("    --keyring-backend file \\\n")
	fmt.Printf("    --gas 2000000 \\\n")
	fmt.Printf("    --node %s/chain-rpc/ \\\n", seedURL)
	fmt.Printf("    --chain-id %s\n", chainID)

	fmt.Println()
	ui.Info("After running both commands, verify with: gonka-nop status")

	// Wait for user confirmation
	confirmed, err := ui.Confirm("Have you completed registration?", false)
	if err != nil {
		return err
	}
	if confirmed {
		state.NodeRegistered = true
		state.PermGranted = true
		ui.Success("Registration marked as complete")
	} else {
		ui.Info("You can register later with: gonka-nop register")
		state.NodeRegistered = false
	}

	return nil
}

// runTestnet attempts automated registration for testnet, falling back to manual instructions on failure.
func (p *Registration) runTestnet(ctx context.Context, state *config.State) error {
	ui.Header("Testnet Registration")

	seedURL := state.SeedAPIURL
	if seedURL == "" {
		seedURL = config.TestnetConfig().SeedAPIURL
	}
	chainID := state.ChainID
	if chainID == "" {
		chainID = config.TestnetConfig().ChainID
	}
	rpcURL := seedURL + "/chain-rpc/"

	// Step 1: Register node on-chain (uses seed URL directly, not /chain-rpc/ path)
	registered := p.tryTestnetRegister(ctx, state, seedURL)

	// Step 2: Grant ML permissions (uses /chain-rpc/ path for tx commands)
	// Fall back to config.env if password is empty (old state files).
	if state.KeyringPassword == "" {
		state.KeyringPassword = chain.LoadKeyringPassword(state)
	}
	if state.ColdKeyName != "" && state.WarmKeyAddress != "" && state.KeyringPassword != "" {
		p.tryGrantPermissions(ctx, state, rpcURL, chainID)
	} else {
		ui.Warn("Cold key name, warm key address, or keyring password not available — skipping grant-ml-ops-permissions")
		p.showGrantManual(state, seedURL, chainID)
	}

	// Step 3: Verify
	if registered {
		adminURL := state.AdminURL
		if adminURL == "" {
			adminURL = defaultAdminURL
		}
		ui.Info("Verifying registration...")
		if err := WaitForRegistration(ctx, adminURL, registrationTimeout); err != nil {
			ui.Warn("Registration verification timed out: %v", err)
			ui.Detail("The node may need time to sync before checks pass")
			ui.Detail("Check status with: gonka-nop status")
		} else {
			ui.Success("Registration verified — node is registered on-chain")
		}
	}

	return nil
}

// tryTestnetRegister attempts automated registration using register-new-participant.
// This works on testnet because gas is free (0icoin) — no funded account needed.
func (p *Registration) tryTestnetRegister(ctx context.Context, state *config.State, seedURL string) bool {
	// Validate required fields
	var missing []string
	if state.AccountPubKey == "" {
		missing = append(missing, "account public key")
	}
	if state.PublicURL == "" {
		missing = append(missing, "public URL")
	}
	if len(missing) > 0 {
		ui.Warn("Cannot automate registration — missing: %s", strings.Join(missing, ", "))
		return p.waitForManualRegistration(state)
	}

	ui.Info("Attempting automated registration...")

	// Use register-new-participant (same as mainnet quick workflow)
	registerCmd := fmt.Sprintf(
		`inferenced register-new-participant %s %s --node-address %s`,
		state.PublicURL, state.AccountPubKey, seedURL,
	)
	// Always pass --consensus-key since docker compose run --no-deps cannot
	// auto-fetch from DAPI_CHAIN_NODE__URL (node container is not linked).
	if state.ConsensusKey != "" {
		registerCmd += fmt.Sprintf(` --consensus-key %s`, state.ConsensusKey)
	}
	ui.Detail("Command: %s", registerCmd)

	err := ui.WithSpinner("Registering node on-chain", func() error {
		_, execErr := chain.RunComposeExec(ctx, state, "api", registerCmd)
		return execErr
	})
	if err != nil {
		ui.Warn("Automated registration failed: %v", err)
		ui.Detail("You can retry with: gonka-nop register --force")
		return p.waitForManualRegistration(state)
	}

	ui.Success("Node registration submitted")
	state.NodeRegistered = true
	return true
}

// tryGrantPermissions attempts automated grant-ml-ops-permissions.
func (p *Registration) tryGrantPermissions(ctx context.Context, state *config.State, nodeURL, chainID string) {
	ui.Info("Granting ML operations permissions...")

	grantCmd := fmt.Sprintf(
		`printf '%%s\n' '%s' | inferenced tx inference grant-ml-ops-permissions %s %s --from %s --keyring-backend file --gas 2000000 --node %s --chain-id %s --yes`,
		state.KeyringPassword, state.ColdKeyName, state.WarmKeyAddress,
		state.ColdKeyName, nodeURL, chainID,
	)

	err := ui.WithSpinner("Granting ML permissions", func() error {
		_, execErr := chain.RunComposeExec(ctx, state, "api", grantCmd)
		return execErr
	})
	if err != nil {
		ui.Warn("Grant ML permissions failed: %v", err)
		ui.Detail("You can retry with: gonka-nop register --force")
		return
	}

	ui.Success("ML permissions granted")
	state.PermGranted = true
}

// showGrantManual displays the manual grant-ml-ops-permissions command.
func (p *Registration) showGrantManual(state *config.State, seedURL, chainID string) {
	fmt.Println()
	ui.Header("Manual Grant ML Permissions Command")
	fmt.Printf("  inferenced tx inference grant-ml-ops-permissions \\\n")
	fmt.Printf("    <cold-key-name> %s \\\n", state.WarmKeyAddress)
	fmt.Printf("    --from <cold-key-name> \\\n")
	fmt.Printf("    --keyring-backend file \\\n")
	fmt.Printf("    --gas 2000000 \\\n")
	fmt.Printf("    --node %s/chain-rpc/ \\\n", seedURL)
	fmt.Printf("    --chain-id %s\n", chainID)
	fmt.Println()
}

// waitForManualRegistration prompts the user to confirm manual registration.
func (p *Registration) waitForManualRegistration(state *config.State) bool {
	confirmed, err := ui.Confirm("Have you completed registration from another node?", false)
	if err != nil || !confirmed {
		ui.Info("You can register later with: gonka-nop register")
		state.NodeRegistered = false
		return false
	}
	state.NodeRegistered = true
	ui.Success("Registration marked as complete")
	return true
}

// isAPIReady does a single probe of the Admin API — returns true if responsive.
func (p *Registration) isAPIReady(ctx context.Context, adminURL string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// WaitForRegistration polls setup/report until consensus_key_match and
// permissions_granted both show PASS, or the timeout expires.
func WaitForRegistration(ctx context.Context, adminURL string, timeout time.Duration) error {
	regCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		checks, err := fetchRegistrationChecks(regCtx, adminURL)
		if err == nil {
			regOK := checks["consensus_key_match"] == statusPass
			permOK := checks["permissions_granted"] == statusPass
			if regOK && permOK {
				return nil
			}
		}

		select {
		case <-regCtx.Done():
			return fmt.Errorf("registration checks did not pass within %s", timeout)
		case <-time.After(registrationPollTime):
		}
	}
}

// fetchRegistrationChecks returns a map of check_id -> status from setup/report.
func fetchRegistrationChecks(ctx context.Context, adminURL string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("setup/report returned status %d", resp.StatusCode)
	}

	var report struct {
		Checks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(report.Checks))
	for _, c := range report.Checks {
		result[c.ID] = c.Status
	}
	return result, nil
}
