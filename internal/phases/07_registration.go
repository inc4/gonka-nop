package phases

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	statusPass           = "PASS"
	registrationTimeout  = 2 * time.Minute
	registrationPollTime = 10 * time.Second
	apiReadyTimeout      = 2 * time.Minute
	apiReadyPollTime     = 5 * time.Second
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

	// Wait for API to become responsive
	if err := p.waitForAPI(ctx, adminURL); err != nil {
		return fmt.Errorf("wait for API: %w", err)
	}

	// Fetch consensus key for display and verification
	consensusKey, err := FetchConsensusKey(ctx, adminURL, state)
	if err != nil {
		ui.Warn("Could not fetch consensus key: %v", err)
		ui.Detail("The consensus key may become available after the node finishes syncing")
	} else {
		state.ConsensusKey = consensusKey
		ui.Detail("Consensus key: %s", consensusKey)
	}

	// Build public URL
	state.PublicURL = fmt.Sprintf("http://%s:%d", state.PublicIP, state.APIPort)

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
	ui.Detail("Command: %s", registerCmd)

	err := ui.WithSpinner("Registering node on-chain", func() error {
		_, execErr := RunComposeExec(ctx, state, "api", registerCmd)
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
		p.loadKeyringPassword(state)
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
		_, execErr := RunComposeExec(ctx, state, "api", grantCmd)
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
	// KeyringPassword is json:"-" so it may be empty if loaded from disk.
	// Fall back to reading from config.env.
	if state.KeyringPassword == "" {
		p.loadKeyringPassword(state)
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
	ui.Detail("Command: %s", registerCmd)

	err := ui.WithSpinner("Registering node on-chain", func() error {
		_, execErr := RunComposeExec(ctx, state, "api", registerCmd)
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
		_, execErr := RunComposeExec(ctx, state, "api", grantCmd)
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

// loadKeyringPassword tries to recover the keyring password from config.env.
// KeyringPassword is json:"-" so it's lost when state is loaded from disk.
func (p *Registration) loadKeyringPassword(state *config.State) {
	envFile := state.OutputDir + "/config.env"
	envVars, err := docker.ParseEnvFile(envFile)
	if err != nil {
		return
	}
	for _, kv := range envVars {
		if strings.HasPrefix(kv, "KEYRING_PASSWORD=") {
			state.KeyringPassword = strings.TrimPrefix(kv, "KEYRING_PASSWORD=")
			return
		}
	}
}

// waitForAPI polls setup/report until the API is responsive.
func (p *Registration) waitForAPI(ctx context.Context, adminURL string) error {
	apiCtx, cancel := context.WithTimeout(ctx, apiReadyTimeout)
	defer cancel()

	sp := ui.NewSpinner("Waiting for Admin API to become responsive...")
	sp.Start()
	defer sp.Stop()

	for {
		req, err := http.NewRequestWithContext(apiCtx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				sp.StopWithSuccess("Admin API is responsive")
				return nil
			}
		}

		select {
		case <-apiCtx.Done():
			return fmt.Errorf("API not responsive after %s", apiReadyTimeout)
		case <-time.After(apiReadyPollTime):
		}
	}
}

// FetchConsensusKey fetches the consensus (validator) key.
// Tries three sources in order:
//  1. setup/report consensus_key_match details (if API includes it)
//  2. setup/report validator_in_set details (if already a validator)
//  3. Tendermint RPC /status via docker exec (most reliable for fresh nodes)
func FetchConsensusKey(ctx context.Context, adminURL string, state *config.State) (string, error) {
	// Try Admin API first (no docker exec needed)
	key, err := fetchConsensusKeyFromReport(ctx, adminURL)
	if err == nil {
		return key, nil
	}

	// Fall back to Tendermint RPC via docker exec into node container
	key, err = fetchConsensusKeyFromRPC(ctx, state)
	if err == nil {
		return key, nil
	}

	return "", fmt.Errorf("consensus key not available from setup/report or Tendermint RPC: %w", err)
}

// fetchConsensusKeyFromReport tries to extract the key from setup/report checks.
func fetchConsensusKeyFromReport(ctx context.Context, adminURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch setup/report: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("setup/report returned status %d", resp.StatusCode)
	}

	var report struct {
		Checks []struct {
			ID      string          `json:"id"`
			Status  string          `json:"status"`
			Details json.RawMessage `json:"details"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return "", fmt.Errorf("decode setup/report: %w", err)
	}

	// Try consensus_key_match first (may have validator_key in details)
	for _, check := range report.Checks {
		if check.ID == "consensus_key_match" && check.Details != nil {
			var d struct {
				ValidatorKey string `json:"validator_key"`
			}
			if json.Unmarshal(check.Details, &d) == nil && d.ValidatorKey != "" {
				return d.ValidatorKey, nil
			}
		}
	}

	// Fall back to validator_in_set (has consensus_pubkey when active)
	for _, check := range report.Checks {
		if check.ID == "validator_in_set" && check.Details != nil {
			var d struct {
				ConsensusPubKey string `json:"consensus_pubkey"`
			}
			if json.Unmarshal(check.Details, &d) == nil && d.ConsensusPubKey != "" {
				return d.ConsensusPubKey, nil
			}
		}
	}

	return "", fmt.Errorf("consensus key not found in setup/report checks")
}

// fetchConsensusKeyFromRPC gets the validator key from Tendermint RPC via docker exec.
// This is the most reliable method — works even on fresh unregistered nodes.
// Parses: /status -> result.validator_info.pub_key.value
func fetchConsensusKeyFromRPC(ctx context.Context, state *config.State) (string, error) {
	// Use wget (available in node container) since curl may not be present
	shellCmd := `wget -qO- http://localhost:26657/status 2>/dev/null`

	execArgs := []string{"exec", "node", "sh", "-c", shellCmd}

	var name string
	var args []string
	if state.UseSudo {
		name = cmdSudo
		args = append([]string{"-E", cmdDocker}, execArgs...)
	} else {
		name = cmdDocker
		args = execArgs
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...) // #nosec G204
	cmd.Dir = state.OutputDir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker exec node RPC status: %w", err)
	}

	var rpcStatus struct {
		Result struct {
			ValidatorInfo struct {
				PubKey struct {
					Value string `json:"value"`
				} `json:"pub_key"`
			} `json:"validator_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &rpcStatus); err != nil {
		return "", fmt.Errorf("parse RPC status: %w", err)
	}

	key := rpcStatus.Result.ValidatorInfo.PubKey.Value
	if key == "" {
		return "", fmt.Errorf("validator pub_key.value is empty in RPC status")
	}
	return key, nil
}

// RunComposeExec runs a shell command inside a container via docker compose run.
// Returns combined stdout+stderr output.
func RunComposeExec(ctx context.Context, state *config.State, service, shellCmd string) (string, error) {
	files := state.ComposeFiles
	if len(files) == 0 {
		files = []string{"docker-compose.yml"}
	}
	// 1 (compose) + 2*len(files) (-f file) + 7 (run --rm --no-deps service sh -c cmd)
	composeArgs := make([]string, 0, 1+2*len(files)+7)
	composeArgs = append(composeArgs, "compose")
	for _, f := range files {
		composeArgs = append(composeArgs, "-f", f)
	}
	composeArgs = append(composeArgs, "run", "--rm", "--no-deps", service, "sh", "-c", shellCmd)

	var cmd *exec.Cmd
	if state.UseSudo {
		sudoArgs := append([]string{"-E", cmdDocker}, composeArgs...)
		cmd = exec.CommandContext(ctx, cmdSudo, sudoArgs...) // #nosec G204
	} else {
		cmd = exec.CommandContext(ctx, cmdDocker, composeArgs...) // #nosec G204
	}
	cmd.Dir = state.OutputDir

	// Load config.env to suppress Docker Compose "variable is not set" warnings
	envFile := state.OutputDir + "/config.env"
	fileEnv, envErr := docker.ParseEnvFile(envFile)
	if envErr == nil && len(fileEnv) > 0 {
		cmd.Env = docker.MergeEnv(fileEnv)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return combined, fmt.Errorf("docker compose run %s: %w\n%s", service, err, combined)
	}

	return strings.TrimSpace(stdout.String()), nil
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
