package phases

import (
	"context"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// Deploy starts the Docker containers
type Deploy struct{}

func NewDeploy() *Deploy {
	return &Deploy{}
}

func (p *Deploy) Name() string {
	return "Deployment"
}

func (p *Deploy) Description() string {
	return "Starting Gonka node containers"
}

func (p *Deploy) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *Deploy) Run(ctx context.Context, state *config.State) error {
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

	// Pull images (mocked)
	err = ui.WithSpinner("Pulling container images", func() error {
		time.Sleep(2 * time.Second) // Simulated pull
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Images pulled: tmkms, node, api, bridge, proxy, explorer, mlnode")

	// Start network node services (mocked)
	err = ui.WithSpinner("Starting network node services", func() error {
		time.Sleep(1500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Started: tmkms, node, api, bridge, proxy, explorer")

	// Wait for network node to sync (mocked)
	err = ui.WithSpinner("Waiting for node to initialize", func() error {
		time.Sleep(2 * time.Second)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Network node initialized and syncing")

	// Start ML node (mocked)
	err = ui.WithSpinner("Starting ML node with GPU", func() error {
		time.Sleep(1500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("ML node started with %d GPUs (TP=%d, PP=%d)", len(state.GPUs), state.TPSize, state.PPSize)

	// Health check (mocked)
	err = ui.WithSpinner("Running health checks", func() error {
		time.Sleep(1 * time.Second)
		return nil
	})
	if err != nil {
		return err
	}

	ui.Success("All containers started successfully")

	// Show summary
	ui.Header("Deployment Summary")
	ui.Detail("Network Node API: http://%s:%d", state.PublicIP, state.APIPort)
	ui.Detail("P2P Endpoint: tcp://%s:%d", state.PublicIP, state.P2PPort)
	ui.Detail("RPC Endpoint: http://%s:26657", state.PublicIP)
	ui.Detail("Explorer: http://%s:3000", state.PublicIP)
	ui.Detail("ML Node: http://localhost:8080 (internal)")

	ui.Header("Next Steps")
	ui.Info("1. Wait for blockchain sync to complete")
	ui.Info("2. Register your node on-chain")
	ui.Info("3. Monitor status with: gonka-nop status")

	return nil
}
