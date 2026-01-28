package phases

import (
	"context"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// NetworkSelect allows user to select the network
type NetworkSelect struct{}

func NewNetworkSelect() *NetworkSelect {
	return &NetworkSelect{}
}

func (p *NetworkSelect) Name() string {
	return "Network Selection"
}

func (p *NetworkSelect) Description() string {
	return "Select the Gonka network to join"
}

func (p *NetworkSelect) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *NetworkSelect) Run(ctx context.Context, state *config.State) error {
	networks := []string{
		"mainnet - Production network",
		"testnet - Test network (coming soon)",
	}

	selected, err := ui.Select("Select network to join:", networks)
	if err != nil {
		return err
	}

	// Parse selection
	if selected == networks[0] {
		state.Network = "mainnet"
	} else {
		state.Network = "testnet"
	}

	ui.Success("Selected network: %s", state.Network)

	// Show seed nodes for selected network
	ui.Header("Network Configuration")
	if state.Network == "mainnet" {
		ui.Detail("Seed API: http://node2.gonka.ai:8000")
		ui.Detail("Seed RPC: http://node2.gonka.ai:26657")
		ui.Detail("Seed P2P: tcp://node2.gonka.ai:5000")
	} else {
		ui.Detail("Testnet configuration not yet available")
	}

	return nil
}
