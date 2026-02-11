package phases

import (
	"context"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	// testnetVRAMThreshold is the VRAM below which testnet overrides the model to a smaller one.
	testnetVRAMThreshold = 320000 // 320 GB in MB

	// networkNameMainnet is the string identifier for the mainnet network.
	networkNameMainnet = "mainnet"
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

func (p *NetworkSelect) Run(_ context.Context, state *config.State) error {
	networks := []string{
		"mainnet - Production network",
		"testnet - Test network",
	}

	selected, err := ui.Select("Select network to join:", networks)
	if err != nil {
		return err
	}

	// Parse selection
	if selected == networks[0] {
		state.Network = networkNameMainnet
	} else {
		state.Network = "testnet"
	}

	ui.Success("Selected network: %s", state.Network)

	// Populate state from network config
	applyNetworkConfig(state)

	// Display
	ui.Header("Network Configuration")
	ui.Detail("Chain ID: %s", state.ChainID)
	ui.Detail("Seed API: %s", state.SeedAPIURL)
	ui.Detail("Image version: %s", state.ImageVersion)
	if len(state.PersistentPeers) > 0 {
		ui.Detail("Persistent peers: %d configured", len(state.PersistentPeers))
	}

	if state.EnforcedModelID != "" {
		ui.Info("Testnet model override: %s (GPU VRAM below 320 GB)", state.EnforcedModelID)
		ui.Detail("TP=%d, PP=%d, Memory Util=%.2f, Max Model Len=%d",
			state.TPSize, state.PPSize, state.GPUMemoryUtil, state.MaxModelLen)
	}

	return nil
}

// applyNetworkConfig populates state fields from the selected network config.
func applyNetworkConfig(state *config.State) {
	var netCfg config.NetworkConfig
	if state.Network == networkNameMainnet {
		netCfg = config.MainnetConfig()
	} else {
		netCfg = config.TestnetConfig()
	}

	state.ChainID = netCfg.ChainID
	state.IsTestNet = netCfg.IsTestNet
	state.SeedAPIURL = netCfg.SeedAPIURL
	state.SeedRPCURL = netCfg.SeedRPCURL
	state.SeedP2PURL = netCfg.SeedP2PURL
	state.ImageVersion = netCfg.ImageVersion
	state.EthereumNetwork = netCfg.EthereumNetwork
	state.BeaconStateURL = netCfg.BeaconStateURL
	state.BridgeImageTag = netCfg.BridgeImageTag
	if len(netCfg.PersistentPeers) > 0 {
		state.PersistentPeers = netCfg.PersistentPeers
	}

	// Testnet model override for small GPUs
	if state.IsTestNet {
		totalVRAM := 0
		for _, gpu := range state.GPUs {
			totalVRAM += gpu.MemoryMB
		}
		if totalVRAM < testnetVRAMThreshold {
			state.EnforcedModelID = "Qwen/Qwen3-4B-Instruct-2507"
			state.SelectedModel = state.EnforcedModelID
			state.TPSize = 1
			state.PPSize = 1
			state.MaxModelLen = 25000
			state.GPUMemoryUtil = 0.88
		}
	}
}
