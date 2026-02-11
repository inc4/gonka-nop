package phases

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	defaultAdminURL   = "http://localhost:9200"
	defaultRPCURL     = "http://localhost:26657"
	syncTimeout       = 30 * time.Minute
	syncPollInterval  = 5 * time.Second
	modelLoadTimeout  = 15 * time.Minute
	modelPollInterval = 10 * time.Second
	cmdSudo           = "sudo"
	cmdDocker         = "docker"
)

// Deploy starts the Docker containers with security hardening
type Deploy struct{}

func NewDeploy() *Deploy {
	return &Deploy{}
}

func (p *Deploy) Name() string {
	return "Deployment"
}

func (p *Deploy) Description() string {
	return "Starting Gonka node containers with firewall and sync monitoring"
}

func (p *Deploy) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *Deploy) Run(ctx context.Context, state *config.State) error {
	// Sudo was already detected in Phase 1 (Prerequisites).
	// Re-detect only if prerequisites was skipped (e.g., resumed run).
	if !state.UseSudo && docker.DetectSudo(ctx) {
		state.UseSudo = true
		ui.Info("Docker requires sudo — commands will use 'sudo -E'")
	}

	// Confirm deployment
	confirm, err := ui.Confirm("Ready to start containers. Proceed?", true)
	if err != nil {
		return err
	}
	if !confirm {
		ui.Warn("Deployment canceled by user")
		return nil
	}

	ui.Info("Starting deployment from: %s", state.OutputDir)

	if err := p.configureFirewall(ctx, state); err != nil {
		return err
	}
	if err := p.pullImages(ctx, state); err != nil {
		return err
	}
	if err := p.startNetworkNode(ctx, state); err != nil {
		return err
	}
	if err := p.fixTMKMSChainID(ctx, state); err != nil {
		ui.Warn("TMKMS chain ID fix failed: %v", err)
	}
	if err := p.monitorSync(ctx, state); err != nil {
		return err
	}
	if err := p.startMLNode(ctx, state); err != nil {
		return err
	}
	if err := p.runHealthChecks(ctx, state); err != nil {
		return err
	}

	p.showSummary(state)
	return nil
}

func (p *Deploy) configureFirewall(_ context.Context, state *config.State) error {
	// Real iptables configuration is deferred to M5.2.
	// For now, warn the user and verify port bindings in compose.
	ui.Warn("Firewall configuration (DOCKER-USER iptables) is not yet automated")
	ui.Detail("Ensure internal ports (5050, 8080, 9100, 9200) are bound to 127.0.0.1 in docker-compose.yml")
	ui.Detail("Refer to: gonka-nop setup --help for manual iptables guidance")

	state.FirewallConfigured = false
	state.DDoSProtection = true
	ui.Info("DDoS protection will be enabled via proxy configuration (GONKA_API_BLOCKED_ROUTES)")
	return nil
}

func (p *Deploy) pullImages(ctx context.Context, state *config.State) error {
	client, err := docker.NewComposeClient(state)
	if err != nil {
		return fmt.Errorf("create compose client: %w", err)
	}

	sp := ui.NewSpinner("Pulling container images (this may take several minutes)...")
	sp.Start()

	pullErr := client.Pull(ctx)

	if pullErr != nil {
		sp.StopWithError("Failed to pull some images")
		ui.Warn("Pull error: %v", pullErr)
		ui.Detail("Some images may already be cached locally")

		proceed, promptErr := ui.Confirm("Continue with cached images?", true)
		if promptErr != nil {
			return promptErr
		}
		if !proceed {
			return fmt.Errorf("pull images: %w", pullErr)
		}
	}

	sp.StopWithSuccess("Container images pulled")
	return nil
}

func (p *Deploy) startNetworkNode(ctx context.Context, state *config.State) error {
	// Start core services using only the first compose file (network node)
	coreClient, err := docker.NewComposeClient(state)
	if err != nil {
		return fmt.Errorf("create compose client: %w", err)
	}

	// Use only the first compose file for core services
	if len(coreClient.Files) > 0 {
		coreClient.Files = coreClient.Files[:1]
	}

	sp := ui.NewSpinner("Starting network node services (docker compose up -d)...")
	sp.Start()

	upErr := coreClient.Up(ctx)

	if upErr != nil {
		sp.StopWithError("Failed to start network node")
		return fmt.Errorf("start network node: %w", upErr)
	}

	sp.StopWithSuccess("Network node services started")
	ui.Detail("Started: tmkms, node, api, bridge, proxy, explorer")

	// Brief pause for containers to initialize
	time.Sleep(5 * time.Second)

	// Verify containers are running
	ps, psErr := coreClient.Ps(ctx)
	if psErr == nil && ps != "" {
		ui.Detail("Running containers:\n%s", ps)
	}

	return nil
}

func (p *Deploy) monitorSync(ctx context.Context, state *config.State) error {
	ui.Header("Blockchain Sync")

	rpcURL := state.RPCURL
	if rpcURL == "" {
		rpcURL = defaultRPCURL
	}

	syncCtx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	sp := ui.NewSpinner("Waiting for blockchain to sync...")
	sp.Start()

	err := docker.WaitForSync(syncCtx, rpcURL, syncPollInterval, func(s *docker.SyncStatus) {
		if s.CatchingUp {
			sp.UpdateMessage(fmt.Sprintf("Syncing... block %s (catching up)",
				formatBlockHeight(int(s.LatestBlockHeight))))
		} else {
			sp.UpdateMessage(fmt.Sprintf("Synced to block %s",
				formatBlockHeight(int(s.LatestBlockHeight))))
		}
	})

	if err != nil {
		sp.StopWithError("Blockchain sync failed or timed out")
		ui.Warn("Sync did not complete within %s. The node may still be syncing.", syncTimeout)
		ui.Detail("Check sync status with: gonka-nop status")
		ui.Detail("You can continue and let it sync in the background")

		proceed, promptErr := ui.Confirm("Continue with deployment anyway?", true)
		if promptErr != nil {
			return promptErr
		}
		if !proceed {
			return fmt.Errorf("sync not complete: %w", err)
		}
		return nil
	}

	sp.StopWithSuccess("Blockchain synced")

	// Fetch final status for display
	finalStatus, fetchErr := docker.FetchSyncStatus(ctx, rpcURL)
	if fetchErr == nil {
		ui.Detail("Block height: %s", formatBlockHeight(int(finalStatus.LatestBlockHeight)))
	}

	return nil
}

func (p *Deploy) startMLNode(ctx context.Context, state *config.State) error {
	ui.Header("ML Node")

	// Start all services (including ML node) using full compose file set
	client, err := docker.NewComposeClient(state)
	if err != nil {
		return fmt.Errorf("create compose client: %w", err)
	}

	sp := ui.NewSpinner("Starting ML node services (docker compose up -d)...")
	sp.Start()

	upErr := client.Up(ctx)
	if upErr != nil {
		sp.StopWithError("Failed to start ML node")
		return fmt.Errorf("start ML node: %w", upErr)
	}
	sp.StopWithSuccess("ML node services started")

	// Show GPU config summary
	gpuSummary := FormatGPUSummary(state.GPUs)
	if gpuSummary != "" {
		ui.Detail("ML node: %s", gpuSummary)
	}
	if state.TPSize > 0 {
		ui.Detail("TP=%d, PP=%d, Memory Util=%.2f, Max Model Len=%d",
			state.TPSize, state.PPSize, state.GPUMemoryUtil, state.MaxModelLen)
	}
	if state.KVCacheDtype != "" && state.KVCacheDtype != kvCacheDtypeAuto {
		ui.Detail("KV Cache Dtype: %s (memory optimization)", state.KVCacheDtype)
	}
	if state.AttentionBackend != "" {
		ui.Detail("Attention Backend: %s", state.AttentionBackend)
	}

	// Wait for model to load
	adminURL := state.AdminURL
	if adminURL == "" {
		adminURL = defaultAdminURL
	}

	loadCtx, cancel := context.WithTimeout(ctx, modelLoadTimeout)
	defer cancel()

	sp = ui.NewSpinner("Waiting for model to load (this may take several minutes)...")
	sp.Start()

	loadErr := docker.WaitForModelLoad(loadCtx, adminURL, modelPollInterval, func(s *docker.MLNodeStatus) {
		sp.UpdateMessage(fmt.Sprintf("Model status: %s", s.CurrentStatus))
	})

	if loadErr != nil {
		sp.StopWithError("Model load failed or timed out")
		ui.Warn("Model did not load within %s.", modelLoadTimeout)
		ui.Detail("Check model status with: gonka-nop ml-node status")
		ui.Detail("Check logs with: docker compose logs mlnode-308 --tail 100")

		proceed, promptErr := ui.Confirm("Continue with deployment anyway?", true)
		if promptErr != nil {
			return promptErr
		}
		if !proceed {
			return fmt.Errorf("model load failed: %w", loadErr)
		}
		return nil
	}

	sp.StopWithSuccess(fmt.Sprintf("Model %s loaded", state.SelectedModel))
	return nil
}

func (p *Deploy) runHealthChecks(ctx context.Context, state *config.State) error {
	ui.Header("Health Checks")

	adminURL := state.AdminURL
	if adminURL == "" {
		adminURL = defaultAdminURL
	}

	sp := ui.NewSpinner("Running health checks via setup/report...")
	sp.Start()

	checks, err := docker.FetchHealthReport(ctx, adminURL)
	sp.Stop()

	if err != nil {
		ui.Warn("Could not fetch health report: %v", err)
		ui.Detail("The API may still be starting up. Check later with: gonka-nop status")
		return nil
	}

	passed := 0
	failed := 0
	for _, c := range checks {
		if c.Status == statusPass {
			passed++
			ui.Success("%s: %s", c.ID, c.Message)
		} else {
			failed++
			ui.Error("%s: %s", c.ID, c.Message)
		}
	}

	total := passed + failed
	if failed > 0 {
		ui.Warn("Health checks: %d/%d passed (%d failed)", passed, total, failed)
		ui.Detail("Some checks may pass after the node finishes syncing or registers on-chain")
	} else {
		ui.Success("All %d health checks passed", total)
	}

	return nil
}

// fixTMKMSChainID patches the TMKMS config when chain_id differs from the hardcoded default.
// The TMKMS init script hardcodes "gonka-mainnet" regardless of CHAIN_ID env var.
func (p *Deploy) fixTMKMSChainID(ctx context.Context, state *config.State) error {
	if state.ChainID == "" || state.ChainID == "gonka-mainnet" {
		return nil // no fix needed
	}

	ui.Info("Fixing TMKMS chain_id: gonka-mainnet -> %s", state.ChainID)

	// Wait for tmkms to initialize and create tmkms.toml
	time.Sleep(5 * time.Second)

	// sed -i "s/gonka-mainnet/<chainID>/g" /root/.tmkms/tmkms.toml
	sedCmd := fmt.Sprintf(`sed -i "s/gonka-mainnet/%s/g" /root/.tmkms/tmkms.toml`, state.ChainID)
	execArgs := []string{"exec", "tmkms", "sh", "-c", sedCmd}

	var name string
	var args []string
	if state.UseSudo {
		name = cmdSudo
		args = append([]string{"-E", cmdDocker}, execArgs...)
	} else {
		name = cmdDocker
		args = execArgs
	}

	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204
	cmd.Dir = state.OutputDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sed tmkms.toml: %w\n%s", err, string(out))
	}

	// Restart tmkms to pick up the new chain_id
	restartArgs := []string{"restart", "tmkms"}
	if state.UseSudo {
		name = cmdSudo
		args = append([]string{"-E", cmdDocker}, restartArgs...)
	} else {
		name = cmdDocker
		args = restartArgs
	}

	cmd = exec.CommandContext(ctx, name, args...) // #nosec G204
	cmd.Dir = state.OutputDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restart tmkms: %w\n%s", err, string(out))
	}

	ui.Success("TMKMS chain_id updated to %s", state.ChainID)
	return nil
}

func (p *Deploy) showSummary(state *config.State) {
	ui.Header("Deployment Summary")
	ui.Detail("Network Node API: http://%s:%d", state.PublicIP, state.APIPort)
	ui.Detail("P2P Endpoint: tcp://%s:%d", state.PublicIP, state.P2PPort)
	ui.Detail("ML Node: http://127.0.0.1:8080 (localhost only)")
	ui.Detail("Admin API: http://127.0.0.1:9200 (localhost only)")

	ui.Header("Security")
	if state.FirewallConfigured {
		ui.Detail("Firewall: DOCKER-USER chain configured")
	} else {
		ui.Warn("Firewall: Not configured. Please set up DOCKER-USER iptables rules manually")
	}
	ui.Detail("DDoS: Proxy route blocking enabled by default")
	ui.Detail("Ports: Internal services should be bound to 127.0.0.1")

	ui.Header("Next Steps")
	ui.Info("1. Registration will follow in the next phase")
	ui.Info("2. Monitor status: gonka-nop status")
	ui.Info("3. Check ML node: gonka-nop ml-node list")
}

// formatBlockHeight formats a block height with comma separators
func formatBlockHeight(n int) string {
	str := fmt.Sprintf("%d", n)
	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}
