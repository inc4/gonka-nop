package phases

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	defaultAdminURL    = "http://localhost:9200"
	defaultRPCURL      = "http://localhost:26657"
	pullTimeout        = 30 * time.Minute
	syncStartupTimeout = 90 * time.Second
	syncPollInterval   = 5 * time.Second
	cmdSudo            = "sudo"
	cmdDocker          = "docker"
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
	if err := p.monitorSync(ctx, state); err != nil {
		return err
	}
	if err := p.preDownloadModel(ctx, state); err != nil {
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

	ui.Info("Pulling container images (this may take several minutes)...")

	// Stream pull output so the user sees download progress per layer.
	client.Stdout = os.Stdout
	client.Stderr = os.Stderr

	pullCtx, pullCancel := context.WithTimeout(ctx, pullTimeout)
	defer pullCancel()

	pullErr := client.Pull(pullCtx)

	// Reset output streams for subsequent compose operations
	client.Stdout = nil
	client.Stderr = nil

	if pullErr != nil {
		ui.Warn("Pull error: %v", pullErr)
		ui.Detail("Some images may already be cached locally")

		proceed, promptErr := ui.Confirm("Continue with cached images?", true)
		if promptErr != nil {
			return promptErr
		}
		if !proceed {
			return fmt.Errorf("pull images: %w", pullErr)
		}
	} else {
		ui.Success("Container images pulled")
	}

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

	// Wait for the node RPC to become reachable (up to 90s),
	// then check sync status briefly — don't block for full sync.
	waitCtx, cancel := context.WithTimeout(ctx, syncStartupTimeout)
	defer cancel()

	sp := ui.NewSpinner("Waiting for node RPC to become available...")
	sp.Start()

	var lastStatus *docker.SyncProgress
	_ = docker.WaitForSync(waitCtx, rpcURL, syncPollInterval, func(s *docker.SyncProgress) {
		lastStatus = s

		if s.ConsecutiveFailures > 0 {
			if s.ConsecutiveFailures >= 12 { // 60s+
				sp.StopWithError("Node appears stuck in restart loop")
				ui.Warn("The node container may be stuck restarting (upgrade handler missing?)")
				ui.Detail("Check logs: docker compose logs node --tail 50")
				ui.Detail("If Cosmovisor upgrade failed, run: gonka-nop repair")
				cancel() // stop waiting
				return
			}
			sp.UpdateMessage(fmt.Sprintf("Waiting for node RPC... (attempt %d)", s.ConsecutiveFailures))
			return
		}

		// RPC is reachable — we got a sync status. Report and move on.
		if s.CatchingUp {
			sp.StopWithSuccess(fmt.Sprintf("Node is syncing — block %s (catching up)",
				formatBlockHeight(int(s.LatestBlockHeight))))
		} else {
			sp.StopWithSuccess(fmt.Sprintf("Node synced — block %s",
				formatBlockHeight(int(s.LatestBlockHeight))))
		}
		cancel() // got status, stop polling
	})

	if lastStatus != nil && lastStatus.ConsecutiveFailures >= 12 {
		// Node appears stuck — check logs for upgrade handler error
		upgradeName := p.checkUpgradeHandlerError(ctx, state)
		if upgradeName != "" {
			ui.Error("Node is stuck: missing upgrade handler for %s", upgradeName)
			ui.Info("Run 'gonka-nop repair' to download the upgrade binary and fix the node")
			return fmt.Errorf("node stuck in restart loop: upgrade handler missing for %s — run 'gonka-nop repair'", upgradeName)
		}
	}

	if lastStatus == nil || lastStatus.ConsecutiveFailures > 0 {
		ui.Warn("Could not reach node RPC at %s", rpcURL)
		ui.Detail("The node may still be starting up. Check with: gonka-nop status")
	} else if lastStatus.CatchingUp {
		ui.Info("Sync will continue in the background while we set up the ML node")
		ui.Detail("Monitor progress with: gonka-nop status")
	}

	return nil
}

func (p *Deploy) preDownloadModel(ctx context.Context, state *config.State) error {
	modelName := state.SelectedModel
	if modelName == "" {
		modelName = defaultModel
	}

	hfHome := state.HFHome
	if hfHome == "" {
		hfHome = defaultHFHome
	}

	ui.Header("Model Cache Check")
	ui.Info("Checking model %s in %s", modelName, hfHome)
	ui.Detail("If already cached, this completes instantly")

	client, err := docker.NewComposeClient(state)
	if err != nil {
		return fmt.Errorf("create compose client: %w", err)
	}

	// Stream output so user sees download progress
	client.Stdout = os.Stdout
	client.Stderr = os.Stderr

	dlErr := client.Run(ctx, "mlnode-308", "huggingface-cli", "download", modelName)
	if dlErr != nil {
		ui.Warn("Model pre-download failed: %v", dlErr)
		ui.Detail("The ML node will attempt to download the model at startup")
		ui.Detail("To pre-download manually: gonka-nop download-model %s", modelName)

		proceed, promptErr := ui.Confirm("Continue without pre-download?", true)
		if promptErr != nil {
			return promptErr
		}
		if !proceed {
			return fmt.Errorf("model download: %w", dlErr)
		}
		return nil
	}

	ui.Success("Model %s cached in %s", modelName, hfHome)
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
		ui.Detail("TP=%d, Memory Util=%.2f, Max Model Len=%d (PP auto-calculated by runner)",
			state.TPSize, state.GPUMemoryUtil, state.MaxModelLen)
	}
	if state.KVCacheDtype != "" && state.KVCacheDtype != kvCacheDtypeAuto {
		ui.Detail("KV Cache Dtype: %s (memory optimization)", state.KVCacheDtype)
	}
	if state.AttentionBackend != "" {
		ui.Detail("Attention Backend: %s", state.AttentionBackend)
	}

	// Model loading is triggered by the API container once the blockchain node
	// is synced. On a fresh deploy, this can take a while. Don't block here —
	// let the user check status later.
	ui.Info("Model will load automatically once the blockchain node is synced")
	ui.Detail("Monitor progress: gonka-nop status")
	ui.Detail("Check ML node: gonka-nop ml-node status")
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

// checkUpgradeHandlerError checks node container logs for upgrade handler errors.
// Returns the upgrade name if found, empty string otherwise.
func (p *Deploy) checkUpgradeHandlerError(ctx context.Context, state *config.State) string {
	cc, err := docker.NewComposeClient(state)
	if err != nil {
		return ""
	}
	// Node is in first compose file only
	if len(cc.Files) > 1 {
		cc.Files = cc.Files[:1]
	}
	return docker.CheckUpgradeHandlerError(ctx, cc)
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
