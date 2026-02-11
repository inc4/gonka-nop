package phases

import (
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

const (
	networkMainnet    = networkNameMainnet
	networkTestnet    = "testnet"
	chainIDMainnet    = "gonka-mainnet"
	testnetSmallModel = "Qwen/Qwen3-4B-Instruct-2507"
	testnetLargeModel = "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"
)

func TestApplyNetworkConfig_Mainnet(t *testing.T) {
	state := config.NewState("/tmp/test")
	state.Network = networkMainnet

	applyNetworkConfig(state)

	if state.ChainID != chainIDMainnet {
		t.Errorf("ChainID = %q, want %q", state.ChainID, chainIDMainnet)
	}
	if state.IsTestNet {
		t.Error("IsTestNet should be false for mainnet")
	}
	if state.SeedAPIURL != "http://node2.gonka.ai:8000" {
		t.Errorf("SeedAPIURL = %q", state.SeedAPIURL)
	}
	if state.ImageVersion != config.DefaultImageVersion {
		t.Errorf("ImageVersion = %q, want %q", state.ImageVersion, config.DefaultImageVersion)
	}
	if len(state.PersistentPeers) != 11 {
		t.Errorf("PersistentPeers = %d, want 11", len(state.PersistentPeers))
	}
	if state.EnforcedModelID != "" {
		t.Errorf("EnforcedModelID should be empty for mainnet, got %q", state.EnforcedModelID)
	}
}

func TestApplyNetworkConfig_Testnet(t *testing.T) {
	state := config.NewState("/tmp/test")
	state.Network = networkTestnet

	applyNetworkConfig(state)

	if state.ChainID != "gonka-testnet" {
		t.Errorf("ChainID = %q, want %q", state.ChainID, "gonka-testnet")
	}
	if !state.IsTestNet {
		t.Error("IsTestNet should be true for testnet")
	}
	if state.SeedAPIURL != "http://89.169.111.79:8000" {
		t.Errorf("SeedAPIURL = %q", state.SeedAPIURL)
	}
	if len(state.PersistentPeers) != 0 {
		t.Errorf("PersistentPeers should be empty for testnet, got %d", len(state.PersistentPeers))
	}
}

func TestApplyNetworkConfig_TestnetSmallGPU(t *testing.T) {
	state := config.NewState("/tmp/test")
	state.Network = networkTestnet
	state.GPUs = []config.GPUInfo{
		{Index: 0, Name: "NVIDIA GeForce RTX 3090", MemoryMB: 24576},
	}
	state.SelectedModel = "Qwen/QwQ-32B"
	state.TPSize = 1
	state.PPSize = 1

	applyNetworkConfig(state)

	if state.EnforcedModelID != testnetSmallModel {
		t.Errorf("EnforcedModelID = %q, want testnet model", state.EnforcedModelID)
	}
	if state.SelectedModel != testnetSmallModel {
		t.Errorf("SelectedModel = %q, should be overridden to testnet model", state.SelectedModel)
	}
	if state.TPSize != 1 {
		t.Errorf("TPSize = %d, want 1", state.TPSize)
	}
	if state.MaxModelLen != 25000 {
		t.Errorf("MaxModelLen = %d, want 25000", state.MaxModelLen)
	}
	if state.GPUMemoryUtil != 0.88 {
		t.Errorf("GPUMemoryUtil = %.2f, want 0.88", state.GPUMemoryUtil)
	}
}

func TestApplyNetworkConfig_TestnetLargeGPU(t *testing.T) {
	// 8x H100 80GB = 655360 MB > 320000 threshold
	state := config.NewState("/tmp/test")
	state.Network = networkTestnet
	state.GPUs = make([]config.GPUInfo, 8)
	for i := range state.GPUs {
		state.GPUs[i] = config.GPUInfo{Index: i, Name: "NVIDIA H100 80GB", MemoryMB: 81920}
	}
	state.SelectedModel = testnetLargeModel

	applyNetworkConfig(state)

	// Should NOT override model — GPU is large enough
	if state.EnforcedModelID != "" {
		t.Errorf("EnforcedModelID should be empty for large GPU testnet, got %q", state.EnforcedModelID)
	}
	if state.SelectedModel != testnetLargeModel {
		t.Errorf("SelectedModel should not change, got %q", state.SelectedModel)
	}
}
