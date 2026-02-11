package config

import "testing"

func TestMainnetConfig(t *testing.T) {
	cfg := MainnetConfig()

	if cfg.ChainID != "gonka-mainnet" {
		t.Errorf("ChainID = %q, want %q", cfg.ChainID, "gonka-mainnet")
	}
	if cfg.IsTestNet {
		t.Error("IsTestNet should be false for mainnet")
	}
	if cfg.SeedAPIURL != "http://node2.gonka.ai:8000" {
		t.Errorf("SeedAPIURL = %q, want %q", cfg.SeedAPIURL, "http://node2.gonka.ai:8000")
	}
	if cfg.ImageVersion != DefaultImageVersion {
		t.Errorf("ImageVersion = %q, want %q", cfg.ImageVersion, DefaultImageVersion)
	}
	if len(cfg.PersistentPeers) != 11 {
		t.Errorf("PersistentPeers count = %d, want 11", len(cfg.PersistentPeers))
	}
	if cfg.EthereumNetwork != "mainnet" {
		t.Errorf("EthereumNetwork = %q, want %q", cfg.EthereumNetwork, "mainnet")
	}
	if cfg.BeaconStateURL != "https://beaconstate.info/" {
		t.Errorf("BeaconStateURL = %q, want %q", cfg.BeaconStateURL, "https://beaconstate.info/")
	}
	if cfg.BridgeImageTag != DefaultImageVersion {
		t.Errorf("BridgeImageTag = %q, want %q", cfg.BridgeImageTag, DefaultImageVersion)
	}
}

func TestTestnetConfig(t *testing.T) {
	cfg := TestnetConfig()

	if cfg.ChainID != "gonka-testnet" {
		t.Errorf("ChainID = %q, want %q", cfg.ChainID, "gonka-testnet")
	}
	if !cfg.IsTestNet {
		t.Error("IsTestNet should be true for testnet")
	}
	if cfg.SeedAPIURL != "http://89.169.111.79:8000" {
		t.Errorf("SeedAPIURL = %q, want %q", cfg.SeedAPIURL, "http://89.169.111.79:8000")
	}
	if cfg.ImageVersion != DefaultImageVersion {
		t.Errorf("ImageVersion = %q, want %q", cfg.ImageVersion, DefaultImageVersion)
	}
	if len(cfg.PersistentPeers) != 0 {
		t.Errorf("PersistentPeers count = %d, want 0 for testnet", len(cfg.PersistentPeers))
	}
	if cfg.EthereumNetwork != "sepolia" {
		t.Errorf("EthereumNetwork = %q, want %q", cfg.EthereumNetwork, "sepolia")
	}
	if cfg.BeaconStateURL != "https://sepolia.checkpoint-sync.ethpandaops.io" {
		t.Errorf("BeaconStateURL = %q, want %q", cfg.BeaconStateURL, "https://sepolia.checkpoint-sync.ethpandaops.io")
	}
	if cfg.BridgeImageTag != DefaultImageVersion {
		t.Errorf("BridgeImageTag = %q, want %q", cfg.BridgeImageTag, DefaultImageVersion)
	}
}

func TestMainnetPersistentPeers(t *testing.T) {
	peers := MainnetPersistentPeers()
	if len(peers) != 11 {
		t.Errorf("MainnetPersistentPeers() count = %d, want 11", len(peers))
	}
	// Spot check first peer format
	if peers[0] != "780e60b5defca577a160590e0bf51c6bb916d2c6@gonka.spv.re:5000" {
		t.Errorf("first peer = %q, unexpected", peers[0])
	}
}
