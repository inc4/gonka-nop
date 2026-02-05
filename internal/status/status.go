package status

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Check status constants
const (
	StatusPass = "PASS"
	StatusFail = "FAIL"
)

// NodeStatus holds the complete status of a Gonka node
type NodeStatus struct {
	Overview   OverviewStatus
	Blockchain BlockchainStatus
	Epoch      EpochStatus
	MLNode     MLNodeStatus
	Security   SecurityStatus

	// Raw data from setup/report for display
	SetupReport *SetupReport
}

// OverviewStatus holds general node status
type OverviewStatus struct {
	ContainersRunning int
	ContainersTotal   int
	NodeRegistered    bool
	NodeAddress       string
	EpochActive       bool
	EpochNumber       int
	EpochWeight       int

	// From setup/report
	OverallStatus string // "PASS" or "FAIL"
	ChecksPassed  int
	ChecksTotal   int
	Issues        []string // failed check messages
}

// BlockchainStatus holds blockchain-related metrics
type BlockchainStatus struct {
	BlockHeight     int64
	NetworkHeight   int64 // highest known block on network
	BlockLag        int64 // NetworkHeight - BlockHeight
	Synced          bool
	CatchingUp      bool
	PeerCount       int
	PeerCountKnown  bool // true if RPC was reachable and peer count is accurate
	ValidatorAddr   string
	IsValidator     bool
	VotingPower     int64
	MissRate        float64 // percentage of missed blocks in current epoch
	MissedBlocks    int
	TotalBlocks     int
	LastBlockTime   time.Time
	SecondsSinceBlk int // from setup/report block_sync details
}

// EpochStatus holds epoch participation details
type EpochStatus struct {
	EpochNumber    int
	Active         bool
	Weight         int
	MissPercentage float64
	MissedCount    int
	TotalCount     int
}

// MLNodeStatus holds ML node metrics
type MLNodeStatus struct {
	Enabled     bool
	ModelName   string
	ModelLoaded bool
	GPUCount    int
	GPUName     string
	GPUs        []GPUDetail
	TPSize      int
	PPSize      int
	MemoryUtil  float64
	MaxModelLen int
	PoCStatus   string
	LastPoCTime time.Time
	LastPoCOK   bool
	Hardware    string // formatted hardware string from report
}

// GPUDetail holds individual GPU info from setup/report
type GPUDetail struct {
	Name           string  `json:"name"`
	TotalMemoryGB  float64 `json:"total_memory_gb"`
	UsedMemoryGB   float64 `json:"used_memory_gb"`
	FreeMemoryGB   float64 `json:"free_memory_gb"`
	UtilizationPct int     `json:"utilization_percent"`
	TemperatureC   int     `json:"temperature_c"`
	Available      bool    `json:"available"`
}

// SecurityStatus holds security configuration status
type SecurityStatus struct {
	FirewallConfigured bool
	DDoSProtection     bool
	InternalPortsBound bool // ports bound to 127.0.0.1
	DriverConsistent   bool

	// From setup/report key checks
	ColdKeyConfigured  bool
	WarmKeyConfigured  bool
	PermissionsGranted bool
}

// --- Setup Report types (from /admin/v1/setup/report) ---

// SetupReport represents the response from /admin/v1/setup/report
type SetupReport struct {
	OverallStatus string       `json:"overall_status"`
	Checks        []SetupCheck `json:"checks"`
	Summary       SetupSummary `json:"summary"`
}

// SetupCheck represents one check from setup/report
type SetupCheck struct {
	ID      string          `json:"id"`
	Status  string          `json:"status"` // "PASS" or "FAIL"
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

// SetupSummary holds totals from setup/report
type SetupSummary struct {
	TotalChecks  int `json:"total_checks"`
	PassedChecks int `json:"passed_checks"`
	FailedChecks int `json:"failed_checks"`
}

// --- Admin Config types (from /admin/v1/config) ---

// AdminConfig represents the response from /admin/v1/config
type AdminConfig struct {
	CurrentSeed   *EpochSeed        `json:"current_seed"`
	PreviousSeed  *EpochSeed        `json:"previous_seed"`
	CurrentHeight int64             `json:"current_height"`
	Nodes         []AdminNodeConfig `json:"nodes"`
}

// EpochSeed holds epoch seed info
type EpochSeed struct {
	EpochIndex int `json:"epoch_index"`
}

// AdminNodeConfig represents a node entry from /admin/v1/config
type AdminNodeConfig struct {
	ID            string                      `json:"id"`
	Host          string                      `json:"host"`
	InferencePort int                         `json:"inference_port"`
	PoCPort       int                         `json:"poc_port"`
	MaxConcurrent int                         `json:"max_concurrent"`
	Models        map[string]AdminModelConfig `json:"models"`
	Hardware      []AdminHardware             `json:"hardware"`
	Enabled       bool                        `json:"enabled"`
}

// AdminModelConfig holds model args from config
type AdminModelConfig struct {
	Args []string `json:"args"`
}

// AdminHardware represents a GPU entry in the config
type AdminHardware struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// --- Validator Set types (from /26657/validators) ---

// TendermintValidators represents the response from /validators endpoint
type TendermintValidators struct {
	Result struct {
		Validators []TendermintValidator `json:"validators"`
	} `json:"result"`
}

// TendermintValidator holds one validator entry
type TendermintValidator struct {
	Address     string          `json:"address"`
	PubKey      json.RawMessage `json:"pub_key"`
	VotingPower string          `json:"voting_power"`
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

// StatusConfig holds the URLs for status endpoints.
// When nil is passed, defaults are used.
type StatusConfig struct {
	TendermintURL string // default "http://localhost:26657"
	AdminURL      string // default "http://localhost:9200"
	VLLMHealthURL string // default "http://localhost:8080"
}

func defaultConfig() *StatusConfig {
	return &StatusConfig{
		TendermintURL: "http://localhost:26657",
		AdminURL:      "http://localhost:9200",
		VLLMHealthURL: "http://localhost:8080",
	}
}

// FetchStatus fetches status from all endpoints
func FetchStatus(outputDir string) (*NodeStatus, error) {
	return FetchStatusWithConfig(outputDir, nil)
}

// FetchStatusWithConfig fetches status using the given config.
// If cfg is nil, default localhost URLs are used.
func FetchStatusWithConfig(_ string, cfg *StatusConfig) (*NodeStatus, error) {
	if cfg == nil {
		cfg = defaultConfig()
	}
	status := &NodeStatus{}

	// Primary: fetch setup/report (provides most data)
	fetchSetupReport(status, cfg)

	// Supplemental: epoch/config details
	fetchAdminConfig(status, cfg)

	// Fetch blockchain status from Tendermint RPC
	fetchBlockchainStatus(status, cfg)

	// Fetch validator set to check voting power
	fetchValidatorSet(status, cfg)

	// Fetch MLNode status from admin API
	fetchMLNodeStatus(status, cfg)

	// Compute overview from collected data
	fetchOverviewStatus(status)

	return status, nil
}

func fetchSetupReport(status *NodeStatus, cfg *StatusConfig) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(cfg.AdminURL + "/admin/v1/setup/report")
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var report SetupReport
	if json.NewDecoder(resp.Body).Decode(&report) != nil {
		return
	}

	status.SetupReport = &report
	status.Overview.OverallStatus = report.OverallStatus
	status.Overview.ChecksPassed = report.Summary.PassedChecks
	status.Overview.ChecksTotal = report.Summary.TotalChecks

	// Extract data from individual checks
	for _, check := range report.Checks {
		if check.Status == "FAIL" {
			status.Overview.Issues = append(status.Overview.Issues, check.Message)
		}
		parseCheckDetails(status, check)
	}
}

// parseCheckDetails extracts structured data from individual setup/report checks.
func parseCheckDetails(status *NodeStatus, check SetupCheck) {
	passed := check.Status == StatusPass

	// Key status checks only need PASS/FAIL, not details
	switch check.ID {
	case "cold_key_configured":
		status.Security.ColdKeyConfigured = passed
	case "warm_key_in_keyring":
		status.Security.WarmKeyConfigured = passed
	case "permissions_granted":
		status.Security.PermissionsGranted = passed
	case "consensus_key_match":
		status.Overview.NodeRegistered = passed
	}

	if check.Details == nil {
		return
	}

	switch check.ID {
	case "active_in_epoch":
		parseEpochCheck(status, check.Details, passed)
	case "block_sync":
		parseBlockSyncCheck(status, check.Details, passed)
	case "missed_requests_threshold":
		parseMissedRequestsCheck(status, check.Details)
	}

	if strings.HasPrefix(check.ID, "mlnode_") {
		parseMLNodeCheck(status, check.Details, passed)
	}
}

func parseEpochCheck(status *NodeStatus, details json.RawMessage, passed bool) {
	var d struct {
		Epoch  int `json:"epoch"`
		Weight int `json:"weight"`
	}
	if json.Unmarshal(details, &d) != nil {
		return
	}
	status.Epoch.EpochNumber = d.Epoch
	status.Epoch.Weight = d.Weight
	status.Epoch.Active = passed
	status.Overview.EpochActive = passed
	status.Overview.EpochNumber = d.Epoch
	status.Overview.EpochWeight = d.Weight
}

func parseBlockSyncCheck(status *NodeStatus, details json.RawMessage, passed bool) {
	var d struct {
		LatestHeight    int64 `json:"latest_height"`
		SecondsSinceBlk int   `json:"seconds_since_block"`
		CatchingUp      bool  `json:"catching_up"`
	}
	if json.Unmarshal(details, &d) != nil {
		return
	}
	if d.LatestHeight > 0 {
		status.Blockchain.BlockHeight = d.LatestHeight
	}
	status.Blockchain.SecondsSinceBlk = d.SecondsSinceBlk
	if d.SecondsSinceBlk > 0 {
		status.Blockchain.LastBlockTime = time.Now().Add(-time.Duration(d.SecondsSinceBlk) * time.Second)
	}
	status.Blockchain.Synced = passed
	status.Blockchain.CatchingUp = d.CatchingUp
}

func parseMissedRequestsCheck(status *NodeStatus, details json.RawMessage) {
	var d struct {
		MissedPercentage float64 `json:"missed_percentage"`
		MissedCount      int     `json:"missed_count"`
		TotalCount       int     `json:"total_count"`
	}
	if json.Unmarshal(details, &d) != nil {
		return
	}
	status.Epoch.MissPercentage = d.MissedPercentage
	status.Epoch.MissedCount = d.MissedCount
	status.Epoch.TotalCount = d.TotalCount
	status.Blockchain.MissRate = d.MissedPercentage / 100
	status.Blockchain.MissedBlocks = d.MissedCount
	status.Blockchain.TotalBlocks = d.TotalCount
}

func parseMLNodeCheck(status *NodeStatus, details json.RawMessage, passed bool) {
	var d struct {
		GPUs   []GPUDetail `json:"gpus"`
		Models []string    `json:"models"`
	}
	if json.Unmarshal(details, &d) != nil {
		return
	}
	if len(d.GPUs) > 0 {
		status.MLNode.GPUs = d.GPUs
		status.MLNode.GPUCount = len(d.GPUs)
		status.MLNode.GPUName = d.GPUs[0].Name
	}
	if len(d.Models) > 0 {
		status.MLNode.ModelName = d.Models[0]
	}
	status.MLNode.ModelLoaded = passed
}

func fetchAdminConfig(status *NodeStatus, cfg *StatusConfig) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(cfg.AdminURL + "/admin/v1/config")
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var config AdminConfig
	if json.NewDecoder(resp.Body).Decode(&config) != nil {
		return
	}

	// Update epoch number from config if setup/report didn't provide it
	if config.CurrentSeed != nil && status.Epoch.EpochNumber == 0 {
		status.Epoch.EpochNumber = config.CurrentSeed.EpochIndex
		status.Overview.EpochNumber = config.CurrentSeed.EpochIndex
	}

	// Use config's chain height as network height reference
	if config.CurrentHeight > 0 {
		status.Blockchain.NetworkHeight = config.CurrentHeight
		if status.Blockchain.BlockHeight > 0 {
			status.Blockchain.BlockLag = config.CurrentHeight - status.Blockchain.BlockHeight
		}
	}

	// Extract model/hardware from node config
	if len(config.Nodes) > 0 {
		node := config.Nodes[0]
		status.MLNode.Enabled = node.Enabled
		for modelName, modelCfg := range node.Models {
			status.MLNode.ModelName = modelName
			parseModelArgs(&status.MLNode, modelCfg.Args)
		}
		if len(node.Hardware) > 0 {
			hw := node.Hardware[0]
			status.MLNode.Hardware = fmt.Sprintf("%dx %s", hw.Count, hw.Type)
			if status.MLNode.GPUCount == 0 {
				status.MLNode.GPUCount = hw.Count
				status.MLNode.GPUName = hw.Type
			}
		}
	}
}

// parseModelArgs extracts TP, PP, memory util, max-model-len from vLLM args.
func parseModelArgs(ml *MLNodeStatus, args []string) {
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--tensor-parallel-size":
			_, _ = fmt.Sscanf(args[i+1], "%d", &ml.TPSize)
		case "--pipeline-parallel-size":
			_, _ = fmt.Sscanf(args[i+1], "%d", &ml.PPSize)
		case "--gpu-memory-utilization":
			_, _ = fmt.Sscanf(args[i+1], "%f", &ml.MemoryUtil)
		case "--max-model-len":
			_, _ = fmt.Sscanf(args[i+1], "%d", &ml.MaxModelLen)
		}
	}
}

func fetchValidatorSet(status *NodeStatus, cfg *StatusConfig) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(cfg.TendermintURL + "/validators")
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var validators TendermintValidators
	if json.NewDecoder(resp.Body).Decode(&validators) != nil {
		return
	}

	// Check if our validator is in the set and get voting power
	for _, v := range validators.Result.Validators {
		if v.Address == status.Blockchain.ValidatorAddr {
			status.Blockchain.IsValidator = true
			_, _ = fmt.Sscanf(v.VotingPower, "%d", &status.Blockchain.VotingPower)
			break
		}
	}
}

func fetchBlockchainStatus(status *NodeStatus, cfg *StatusConfig) {
	// Try to fetch from Tendermint RPC
	client := &http.Client{Timeout: 5 * time.Second}

	// Fetch /status
	resp, err := client.Get(cfg.TendermintURL + "/status")
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
	resp2, err := client.Get(cfg.TendermintURL + "/net_info")
	if err == nil && resp2.StatusCode == 200 {
		defer func() { _ = resp2.Body.Close() }()
		var netInfo TendermintNetInfo
		if json.NewDecoder(resp2.Body).Decode(&netInfo) == nil {
			var peers int
			_, _ = fmt.Sscanf(netInfo.Result.NPeers, "%d", &peers)
			status.Blockchain.PeerCount = peers
			status.Blockchain.PeerCountKnown = true
		}
	}
}

func fetchMLNodeStatus(status *NodeStatus, cfg *StatusConfig) {
	client := &http.Client{Timeout: 5 * time.Second}

	// Fetch from Admin API
	resp, err := client.Get(cfg.AdminURL + "/admin/v1/nodes")
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
	resp2, err := client.Get(cfg.VLLMHealthURL + "/v1/models")
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
		containersRunning++
	}

	status.Overview.ContainersRunning = containersRunning
	status.Overview.ContainersTotal = 8 // Expected total (tmkms, node, api, bridge, proxy, explorer, mlnode, proxy-ssl optional)

	// Node is registered if setup/report consensus_key_match passed, or has validator address
	if status.Blockchain.ValidatorAddr != "" {
		status.Overview.NodeRegistered = true
		status.Overview.NodeAddress = status.Blockchain.ValidatorAddr
	}
}

// FetchMockedStatus returns mocked status for demo with full validator details
func FetchMockedStatus() *NodeStatus {
	return &NodeStatus{
		Overview: OverviewStatus{
			ContainersRunning: 8,
			ContainersTotal:   8,
			NodeRegistered:    true,
			NodeAddress:       "gonka1x8q2k9f5p7w3m6n4v2c8b1a0z9y8x7w6v5u4t3",
			EpochActive:       true,
			EpochNumber:       427,
			EpochWeight:       1000,
			OverallStatus:     StatusPass,
			ChecksPassed:      11,
			ChecksTotal:       11,
		},
		Blockchain: BlockchainStatus{
			BlockHeight:    1250000,
			NetworkHeight:  1250003,
			BlockLag:       3,
			Synced:         true,
			CatchingUp:     false,
			PeerCount:      12,
			PeerCountKnown: true,
			ValidatorAddr:  "ABCD1234EFGH5678",
			IsValidator:    true,
			VotingPower:    1000,
			MissRate:       0.02,
			MissedBlocks:   3,
			TotalBlocks:    150,
			LastBlockTime:  time.Now().Add(-6 * time.Second),
		},
		Epoch: EpochStatus{
			EpochNumber:    427,
			Active:         true,
			Weight:         1000,
			MissPercentage: 2.0,
			MissedCount:    3,
			TotalCount:     150,
		},
		MLNode: MLNodeStatus{
			Enabled:     true,
			ModelName:   "Qwen/QwQ-32B",
			ModelLoaded: true,
			GPUCount:    4,
			GPUName:     "NVIDIA GeForce RTX 4090",
			TPSize:      4,
			PPSize:      1,
			MemoryUtil:  0.92,
			MaxModelLen: 32768,
			PoCStatus:   "Participating",
			LastPoCTime: time.Now().Add(-2 * time.Minute),
			LastPoCOK:   true,
		},
		Security: SecurityStatus{
			FirewallConfigured: true,
			DDoSProtection:     true,
			InternalPortsBound: true,
			DriverConsistent:   true,
			ColdKeyConfigured:  true,
			WarmKeyConfigured:  true,
			PermissionsGranted: true,
		},
	}
}
