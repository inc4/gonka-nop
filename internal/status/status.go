package status

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// NodeStatus holds the complete status of a Gonka node
type NodeStatus struct {
	Overview   OverviewStatus
	Blockchain BlockchainStatus
	MLNode     MLNodeStatus
}

// OverviewStatus holds general node status
type OverviewStatus struct {
	ContainersRunning int
	ContainersTotal   int
	NodeRegistered    bool
	NodeAddress       string
	EpochActive       bool
	EpochWeight       int
}

// BlockchainStatus holds blockchain-related metrics
type BlockchainStatus struct {
	BlockHeight   int64
	Synced        bool
	CatchingUp    bool
	PeerCount     int
	ValidatorAddr string
	IsValidator   bool
}

// MLNodeStatus holds ML node metrics
type MLNodeStatus struct {
	Enabled      bool
	ModelName    string
	ModelLoaded  bool
	GPUCount     int
	GPUName      string
	PoCStatus    string
	LastPoCTime  time.Time
	LastPoCOK    bool
}

// TendermintStatus represents the response from /status endpoint
type TendermintStatus struct {
	Result struct {
		NodeInfo struct {
			Network string `json:"network"`
			Moniker string `json:"moniker"`
		} `json:"node_info"`
		SyncInfo struct {
			LatestBlockHeight string `json:"latest_block_height"`
			CatchingUp        bool   `json:"catching_up"`
		} `json:"sync_info"`
		ValidatorInfo struct {
			Address string `json:"address"`
		} `json:"validator_info"`
	} `json:"result"`
}

// TendermintNetInfo represents the response from /net_info endpoint
type TendermintNetInfo struct {
	Result struct {
		NPeers string `json:"n_peers"`
		Peers  []struct {
			NodeInfo struct {
				Moniker string `json:"moniker"`
			} `json:"node_info"`
		} `json:"peers"`
	} `json:"result"`
}

// AdminMLNode represents an MLNode from the admin API
type AdminMLNode struct {
	ID            string   `json:"id"`
	Host          string   `json:"host"`
	InferencePort int      `json:"inference_port"`
	PoCPort       int      `json:"poc_port"`
	MaxConcurrent int      `json:"max_concurrent"`
	Models        []string `json:"models"`
	Enabled       bool     `json:"enabled"`
}

// FetchStatus fetches status from all endpoints
func FetchStatus(outputDir string) (*NodeStatus, error) {
	status := &NodeStatus{}

	// Fetch blockchain status
	fetchBlockchainStatus(status)

	// Fetch MLNode status
	fetchMLNodeStatus(status)

	// Fetch overview (container status, registration)
	fetchOverviewStatus(status)

	return status, nil
}

func fetchBlockchainStatus(status *NodeStatus) {
	// Try to fetch from Tendermint RPC
	client := &http.Client{Timeout: 5 * time.Second}

	// Fetch /status
	resp, err := client.Get("http://localhost:26657/status")
	if err == nil && resp.StatusCode == 200 {
		defer func() { _ = resp.Body.Close() }()
		var tmStatus TendermintStatus
		if json.NewDecoder(resp.Body).Decode(&tmStatus) == nil {
			var height int64
			_, _ = fmt.Sscanf(tmStatus.Result.SyncInfo.LatestBlockHeight, "%d", &height)
			status.Blockchain.BlockHeight = height
			status.Blockchain.CatchingUp = tmStatus.Result.SyncInfo.CatchingUp
			status.Blockchain.Synced = !tmStatus.Result.SyncInfo.CatchingUp
			status.Blockchain.ValidatorAddr = tmStatus.Result.ValidatorInfo.Address
			status.Blockchain.IsValidator = tmStatus.Result.ValidatorInfo.Address != ""
		}
	}

	// Fetch /net_info for peer count
	resp2, err := client.Get("http://localhost:26657/net_info")
	if err == nil && resp2.StatusCode == 200 {
		defer func() { _ = resp2.Body.Close() }()
		var netInfo TendermintNetInfo
		if json.NewDecoder(resp2.Body).Decode(&netInfo) == nil {
			var peers int
			_, _ = fmt.Sscanf(netInfo.Result.NPeers, "%d", &peers)
			status.Blockchain.PeerCount = peers
		}
	}
}

func fetchMLNodeStatus(status *NodeStatus) {
	client := &http.Client{Timeout: 5 * time.Second}

	// Fetch from Admin API
	resp, err := client.Get("http://localhost:9200/admin/v1/nodes")
	if err == nil && resp.StatusCode == 200 {
		defer func() { _ = resp.Body.Close() }()
		var nodes []AdminMLNode
		if json.NewDecoder(resp.Body).Decode(&nodes) == nil && len(nodes) > 0 {
			node := nodes[0]
			status.MLNode.Enabled = node.Enabled
			if len(node.Models) > 0 {
				status.MLNode.ModelName = node.Models[0]
			}
		}
	}

	// Check vLLM health
	resp2, err := client.Get("http://localhost:8080/v1/models")
	if err == nil && resp2.StatusCode == 200 {
		status.MLNode.ModelLoaded = true
		_ = resp2.Body.Close()
	}
}

func fetchOverviewStatus(status *NodeStatus) {
	// This would check docker containers
	// For now, we infer from other status checks
	containersRunning := 0

	// If blockchain is accessible, node container is running
	if status.Blockchain.BlockHeight > 0 {
		containersRunning += 3 // node, tmkms, api
	}

	// If MLNode is accessible
	if status.MLNode.ModelLoaded {
		containersRunning += 1
	}

	status.Overview.ContainersRunning = containersRunning
	status.Overview.ContainersTotal = 7 // Expected total

	// Node is registered if it has a validator address
	status.Overview.NodeRegistered = status.Blockchain.ValidatorAddr != ""
	status.Overview.NodeAddress = status.Blockchain.ValidatorAddr
}

// FetchMockedStatus returns mocked status for demo
func FetchMockedStatus() *NodeStatus {
	return &NodeStatus{
		Overview: OverviewStatus{
			ContainersRunning: 7,
			ContainersTotal:   7,
			NodeRegistered:    true,
			NodeAddress:       "gonka1x8q2k9f5p7w3m6n4v2c8b1a0z9y8x7w6v5u4t3",
			EpochActive:       true,
			EpochWeight:       1000,
		},
		Blockchain: BlockchainStatus{
			BlockHeight:   1234567,
			Synced:        true,
			CatchingUp:    false,
			PeerCount:     12,
			ValidatorAddr: "ABCD1234EFGH5678",
			IsValidator:   true,
		},
		MLNode: MLNodeStatus{
			Enabled:     true,
			ModelName:   "Qwen/QwQ-32B",
			ModelLoaded: true,
			GPUCount:    4,
			GPUName:     "NVIDIA GeForce RTX 4090",
			PoCStatus:   "Participating",
			LastPoCTime: time.Now().Add(-2 * time.Hour),
			LastPoCOK:   true,
		},
	}
}
