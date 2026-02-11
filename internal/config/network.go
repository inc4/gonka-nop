package config

// DefaultImageVersion is the current release version for Gonka container images.
const DefaultImageVersion = "0.2.9-post2"

// NetworkConfig holds network-specific configuration for mainnet or testnet.
type NetworkConfig struct {
	ChainID         string
	SeedAPIURL      string
	SeedRPCURL      string
	SeedP2PURL      string
	PersistentPeers []string
	IsTestNet       bool
	ImageVersion    string
	EthereumNetwork string // "mainnet" or "sepolia"
	BeaconStateURL  string // Ethereum beacon state checkpoint URL
	BridgeImageTag  string // bridge container image version
}

// MainnetConfig returns the configuration for the Gonka mainnet.
func MainnetConfig() NetworkConfig {
	return NetworkConfig{
		ChainID:         "gonka-mainnet",
		SeedAPIURL:      "http://node2.gonka.ai:8000",
		SeedRPCURL:      "http://node2.gonka.ai:26657",
		SeedP2PURL:      "tcp://node2.gonka.ai:5000",
		PersistentPeers: MainnetPersistentPeers(),
		IsTestNet:       false,
		ImageVersion:    DefaultImageVersion,
		EthereumNetwork: "mainnet",
		BeaconStateURL:  "https://beaconstate.info/",
		BridgeImageTag:  DefaultImageVersion,
	}
}

// TestnetConfig returns the configuration for the Gonka testnet.
func TestnetConfig() NetworkConfig {
	return NetworkConfig{
		ChainID:         "gonka-testnet",
		SeedAPIURL:      "http://89.169.111.79:8000",
		SeedRPCURL:      "http://89.169.111.79:26657",
		SeedP2PURL:      "tcp://89.169.111.79:5000",
		PersistentPeers: []string{},
		IsTestNet:       true,
		ImageVersion:    DefaultImageVersion,
		EthereumNetwork: "sepolia",
		BeaconStateURL:  "https://sepolia.checkpoint-sync.ethpandaops.io",
		BridgeImageTag:  DefaultImageVersion,
	}
}

// MainnetPersistentPeers returns the known-good persistent peers for mainnet.
func MainnetPersistentPeers() []string {
	return []string{
		"780e60b5defca577a160590e0bf51c6bb916d2c6@gonka.spv.re:5000",
		"39ebfea6d2ab91e90c26cb702345cfa2f9bc611b@47.236.26.199:5000",
		"645fbce2dedcc7166f4df7931d2c87ca5188b569@node2.gonka.ai:5000",
		"6140f7090137d93c272ff5ccd863484d1592949d@node3.gonka.ai:5000",
		"947a89a2d5f2af45cb7853f56be0bab8303ffce9@36.189.234.197:18027",
		"78f3279bd30fe6f1b84a9c40c3b97bd74e575981@185.216.21.98:5000",
		"d53a970a40231474fb4092ee64609b975d906085@47.236.19.22:15000",
		"b7dd3863523d78cc5a7c56ddb786395fe49c954a@93.119.168.58:5000",
		"b4ad5a33520ce10d0b7ed5193e2de71e2f1f7a51@36.189.234.237:17240",
		"0772f16cc65cb4d19341b192bd7eba964f11d124@node1.gonka.ai:5000",
		"4d63a0411a257669e794ff62f801550a8449d239@69.19.136.233:5000",
	}
}
