package phases

import (
	"context"
	"fmt"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
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

func (p *Deploy) Run(_ context.Context, state *config.State) error {
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

	// Configure DOCKER-USER firewall rules (mocked)
	err = ui.WithSpinner("Configuring DOCKER-USER iptables chain", func() error {
		time.Sleep(600 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	state.FirewallConfigured = true
	ui.Success("DOCKER-USER chain configured (UFW bypass protection)")
	ui.Detail("Internal ports (9100, 9200, 5050, 8080) blocked from external access")
	ui.Detail("P2P (5000) and RPC (26657) remain publicly accessible")

	// DDoS protection summary
	state.DDoSProtection = true
	ui.Success("DDoS protection enabled via proxy configuration")
	ui.Detail("Blocked routes: /cosmos/*, /nop/*")
	ui.Detail("Chain API disabled, Chain GRPC disabled, Chain RPC enabled (read-only)")

	// Pull images (mocked)
	err = ui.WithSpinner("Pulling container images", func() error {
		time.Sleep(2 * time.Second)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Images pulled: tmkms, node, api, bridge, proxy, explorer")

	// Pull ML node image (architecture-specific)
	mlImage := fmt.Sprintf("ghcr.io/product-science/mlnode:%s", state.MLNodeImageTag)
	if state.MLNodeImageTag == "" {
		mlImage = "ghcr.io/product-science/mlnode:3.0.12"
	}
	err = ui.WithSpinner(fmt.Sprintf("Pulling ML node image (%s)", mlImage), func() error {
		time.Sleep(1500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}

	// Check IPv4/IPv6 resolution (vLLM restart loop issue)
	err = ui.WithSpinner("Checking IPv4/IPv6 resolution", func() error {
		time.Sleep(400 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	// Mocked: IPv4 resolution works fine
	ui.Success("IPv4 resolution OK (no IPv6 conflict for vLLM health endpoint)")

	// Start network node services (mocked)
	err = ui.WithSpinner("Starting network node services (docker compose up -d)", func() error {
		time.Sleep(1500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Started: tmkms, node, api, bridge, proxy, explorer")

	// Sync monitoring (mocked)
	ui.Header("Blockchain Sync")
	err = ui.WithSpinner("Waiting for node to connect to peers", func() error {
		time.Sleep(1 * time.Second)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Connected to %d peers", 8)

	err = ui.WithSpinner("Waiting for state sync to initialize", func() error {
		time.Sleep(1500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("State sync from snapshot — catching up")

	// Simulate sync progress
	syncSteps := []struct {
		block int
		lag   int
	}{
		{1200000, 50000},
		{1230000, 20000},
		{1245000, 5000},
		{1249000, 1000},
		{1249900, 100},
	}
	for _, step := range syncSteps {
		err = ui.WithSpinner(fmt.Sprintf("Syncing... block %s (lag: %d blocks)",
			formatBlockHeight(step.block), step.lag), func() error {
			time.Sleep(500 * time.Millisecond)
			return nil
		})
		if err != nil {
			return err
		}
	}
	ui.Success("Blockchain synced to latest block 1,250,000")

	// Start ML node (mocked)
	ui.Header("ML Node")
	err = ui.WithSpinner("Starting ML node with GPU (docker compose -f docker-compose.mlnode.yml up -d)", func() error {
		time.Sleep(1500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}

	gpuSummary := FormatGPUSummary(state.GPUs)
	ui.Detail("ML node started: %s", gpuSummary)
	ui.Detail("TP=%d, PP=%d, Memory Util=%.2f, Max Model Len=%d",
		state.TPSize, state.PPSize, state.GPUMemoryUtil, state.MaxModelLen)
	if state.KVCacheDtype != "" && state.KVCacheDtype != "auto" {
		ui.Detail("KV Cache Dtype: %s (memory optimization)", state.KVCacheDtype)
	}
	ui.Detail("Attention Backend: %s", state.AttentionBackend)

	// Wait for model to load
	err = ui.WithSpinner("Waiting for model to load (this may take several minutes)", func() error {
		time.Sleep(3 * time.Second)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Success("Model %s loaded successfully", state.SelectedModel)

	// Health check (mocked)
	err = ui.WithSpinner("Running health checks", func() error {
		time.Sleep(1 * time.Second)
		return nil
	})
	if err != nil {
		return err
	}

	checks := []struct {
		name   string
		ok     bool
		detail string
	}{
		{"Blockchain node", true, "Block height 1,250,000, synced"},
		{"TMKMS", true, "Consensus key signing active"},
		{"API service", true, "Admin API responding on 127.0.0.1:9200"},
		{"ML node", true, fmt.Sprintf("Model %s loaded, GPU healthy", state.SelectedModel)},
		{"Proxy", true, fmt.Sprintf("HTTP proxy on port %d", state.APIPort)},
		{"Bridge", true, "Ethereum bridge connected"},
		{"Explorer", true, "Dashboard on port 5173"},
	}
	for _, c := range checks {
		if c.ok {
			ui.Success("%s: %s", c.name, c.detail)
		} else {
			ui.Error("%s: %s", c.name, c.detail)
		}
	}

	ui.Success("All containers started successfully")

	// Show summary
	ui.Header("Deployment Summary")
	ui.Detail("Network Node API: http://%s:%d", state.PublicIP, state.APIPort)
	ui.Detail("P2P Endpoint: tcp://%s:%d", state.PublicIP, state.P2PPort)
	ui.Detail("RPC Endpoint: http://%s:26657", state.PublicIP)
	ui.Detail("ML Node: http://127.0.0.1:8080 (localhost only)")
	ui.Detail("Admin API: http://127.0.0.1:9200 (localhost only)")

	ui.Header("Security")
	ui.Detail("Firewall: DOCKER-USER chain configured")
	ui.Detail("DDoS: Proxy route blocking enabled")
	ui.Detail("Ports: Internal services bound to 127.0.0.1")

	ui.Header("Next Steps")
	ui.Info("1. Register your node on-chain")
	ui.Info("2. Enable ML node via Admin API:")
	ui.Detail("   curl -X POST http://127.0.0.1:9200/admin/v1/nodes")
	ui.Info("3. Monitor status with: gonka-nop status")

	return nil
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
