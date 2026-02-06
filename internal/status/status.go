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
	NodeConfig NodeConfigStatus

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
	EpochNumber     int
	Active          bool
	Weight          int
	PoCWeight       int // from epoch_ml_nodes — actual PoC weight assigned
	MissPercentage  float64
	MissedCount     int
	TotalCount      int
	InferenceCount  int  // total inferences served (not just misses)
	MissCheckPassed bool // true if the missed_requests_threshold check passed

	// Timeslot allocation from epoch_ml_nodes
	TimeslotAllocation []bool

	// Reward claim status from /admin/v1/config seeds
	PrevEpochClaimed bool
	PrevEpochIndex   int
	UpcomingEpoch    int
}

// MLNodeStatus holds ML node metrics
type MLNodeStatus struct {
	Enabled        bool
	EnabledEpoch   int // epoch when enable took effect
	ModelName      string
	ModelLoaded    bool
	GPUCount       int
	GPUName        string
	GPUs           []GPUDetail
	TPSize         int
	PPSize         int
	MemoryUtil     float64
	MaxModelLen    int
	PoCStatus      string // current_status from admin API
	IntendedStatus string // intended_status — mismatch with PoCStatus = transitioning
	PoCNodeStatus  string // poc_current_status
	LastPoCTime    time.Time
	LastPoCOK      bool
	Hardware       string    // formatted hardware string from report
	StatusUpdated  time.Time // when state was last updated
}

// NodeConfigStatus holds node configuration details from /admin/v1/config
type NodeConfigStatus struct {
	PublicURL      string
	PoCCallbackURL string
	APIVersion     string
	SeedAPIURL     string
	UpgradeName    string
	UpgradeHeight  int64
	HeightLag      int64 // current_height - last_processed_height
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
	API                 AdminAPIConfig    `json:"api"`
	Nodes               []AdminNodeConfig `json:"nodes"`
	CurrentSeed         *EpochSeed        `json:"current_seed"`
	PreviousSeed        *EpochSeed        `json:"previous_seed"`
	UpcomingSeed        *EpochSeed        `json:"upcoming_seed"`
	CurrentHeight       int64             `json:"current_height"`
	LastProcessedHeight int64             `json:"last_processed_height"`
	CurrentNodeVersion  string            `json:"current_node_version"`
	UpgradePlan         AdminUpgradePlan  `json:"upgrade_plan"`
	ChainNode           AdminChainNode    `json:"chain_node"`
}

// EpochSeed holds epoch seed info
type EpochSeed struct {
	Seed       int64 `json:"seed"`
	EpochIndex int   `json:"epoch_index"`
	Claimed    bool  `json:"claimed"`
}

// AdminAPIConfig holds API configuration from /admin/v1/config
type AdminAPIConfig struct {
	PublicURL      string `json:"public_url"`
	PoCCallbackURL string `json:"poc_callback_url"`
}

// AdminUpgradePlan holds upcoming chain upgrade info
type AdminUpgradePlan struct {
	Name   string `json:"name"`
	Height int64  `json:"height"`
}

// AdminChainNode holds chain node connection config
type AdminChainNode struct {
	SeedAPIURL string `json:"seed_api_url"`
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

// AdminNodesEntry represents one entry from /admin/v1/nodes (nested structure)
type AdminNodesEntry struct {
	Node  AdminNodesNodeInfo `json:"node"`
	State AdminNodesState    `json:"state"`
}

// AdminNodesNodeInfo holds the static node config from /admin/v1/nodes
type AdminNodesNodeInfo struct {
	ID            string                      `json:"id"`
	Host          string                      `json:"host"`
	InferencePort int                         `json:"inference_port"`
	PoCPort       int                         `json:"poc_port"`
	MaxConcurrent int                         `json:"max_concurrent"`
	Models        map[string]AdminModelConfig `json:"models"`
	Hardware      []AdminHardware             `json:"hardware"`
}

// AdminNodesState holds runtime state from /admin/v1/nodes
type AdminNodesState struct {
	IntendedStatus    string `json:"intended_status"`
	CurrentStatus     string `json:"current_status"`
	PoCIntendedStatus string `json:"poc_intended_status"`
	PoCCurrentStatus  string `json:"poc_current_status"`
	FailureReason     string `json:"failure_reason"`
	StatusTimestamp   string `json:"status_timestamp"`
	AdminState        struct {
		Enabled bool `json:"enabled"`
		Epoch   int  `json:"epoch"`
	} `json:"admin_state"`
	EpochMLNodes map[string]EpochMLNodeInfo `json:"epoch_ml_nodes"`
}

// EpochMLNodeInfo holds per-model node allocation from epoch_ml_nodes
type EpochMLNodeInfo struct {
	NodeID             string `json:"node_id"`
	PoCWeight          int    `json:"poc_weight"`
	TimeslotAllocation []bool `json:"timeslot_allocation"`
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
	case "validator_in_set":
		status.Blockchain.IsValidator = passed
		// Voting power equals epoch weight
		if passed {
			status.Blockchain.VotingPower = int64(status.Epoch.Weight)
		}
	case "block_sync":
		parseBlockSyncCheck(status, check.Details, passed)
	case "missed_requests_threshold":
		parseMissedRequestsCheck(status, check.Details, passed)
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

func parseMissedRequestsCheck(status *NodeStatus, details json.RawMessage, passed bool) {
	var d struct {
		MissedPercentage float64 `json:"missed_percentage"`
		MissedRequests   int     `json:"missed_requests"`
		TotalRequests    int     `json:"total_requests"`
		InferenceCount   int     `json:"inference_count"`
	}
	if json.Unmarshal(details, &d) != nil {
		return
	}
	status.Epoch.MissPercentage = d.MissedPercentage
	status.Epoch.MissedCount = d.MissedRequests
	status.Epoch.TotalCount = d.TotalRequests
	status.Epoch.InferenceCount = d.InferenceCount
	status.Epoch.MissCheckPassed = passed
	status.Blockchain.MissRate = d.MissedPercentage / 100
	status.Blockchain.MissedBlocks = d.MissedRequests
	status.Blockchain.TotalBlocks = d.TotalRequests
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

	// Node config: public URL, PoC callback, API version, seed API
	status.NodeConfig.PublicURL = config.API.PublicURL
	status.NodeConfig.PoCCallbackURL = config.API.PoCCallbackURL
	status.NodeConfig.APIVersion = config.CurrentNodeVersion
	status.NodeConfig.SeedAPIURL = config.ChainNode.SeedAPIURL

	// Upgrade plan
	if config.UpgradePlan.Name != "" {
		status.NodeConfig.UpgradeName = config.UpgradePlan.Name
		status.NodeConfig.UpgradeHeight = config.UpgradePlan.Height
	}

	// API processing lag
	if config.CurrentHeight > 0 && config.LastProcessedHeight > 0 {
		status.NodeConfig.HeightLag = config.CurrentHeight - config.LastProcessedHeight
	}

	applyConfigSeeds(status, &config)
	applyConfigNodes(status, &config)
}

func applyConfigSeeds(status *NodeStatus, config *AdminConfig) {
	if config.PreviousSeed != nil {
		status.Epoch.PrevEpochClaimed = config.PreviousSeed.Claimed
		status.Epoch.PrevEpochIndex = config.PreviousSeed.EpochIndex
	}
	if config.UpcomingSeed != nil {
		status.Epoch.UpcomingEpoch = config.UpcomingSeed.EpochIndex
	}
}

func applyConfigNodes(status *NodeStatus, config *AdminConfig) {
	if len(config.Nodes) == 0 {
		return
	}
	node := config.Nodes[0]
	// Note: config nodes do NOT have an "enabled" field — don't set MLNode.Enabled here
	for modelName, modelCfg := range node.Models {
		if status.MLNode.ModelName == "" {
			status.MLNode.ModelName = modelName
		}
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

	// Fetch from Admin API (/admin/v1/nodes returns nested {node, state} entries)
	resp, err := client.Get(cfg.AdminURL + "/admin/v1/nodes")
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var entries []AdminNodesEntry
	if json.NewDecoder(resp.Body).Decode(&entries) != nil || len(entries) == 0 {
		return
	}

	entry := entries[0]
	status.MLNode.Enabled = entry.State.AdminState.Enabled
	status.MLNode.EnabledEpoch = entry.State.AdminState.Epoch

	// Current vs intended status
	status.MLNode.PoCStatus = entry.State.CurrentStatus
	status.MLNode.IntendedStatus = entry.State.IntendedStatus
	status.MLNode.PoCNodeStatus = entry.State.PoCCurrentStatus

	// Status timestamp
	if entry.State.StatusTimestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, entry.State.StatusTimestamp); err == nil {
			status.MLNode.StatusUpdated = t
		}
	}

	// PoC weight and timeslot allocation from epoch_ml_nodes
	for _, info := range entry.State.EpochMLNodes {
		status.Epoch.PoCWeight = info.PoCWeight
		status.Epoch.TimeslotAllocation = info.TimeslotAllocation
		break // first model entry
	}

	// Model name from node config
	for modelName, modelCfg := range entry.Node.Models {
		if status.MLNode.ModelName == "" {
			status.MLNode.ModelName = modelName
		}
		parseModelArgs(&status.MLNode, modelCfg.Args)
	}

	// Hardware
	if len(entry.Node.Hardware) > 0 {
		hw := entry.Node.Hardware[0]
		status.MLNode.Hardware = fmt.Sprintf("%dx %s", hw.Count, hw.Type)
		if status.MLNode.GPUCount == 0 {
			status.MLNode.GPUCount = hw.Count
			status.MLNode.GPUName = hw.Type
		}
	}
}

func fetchOverviewStatus(status *NodeStatus) {
	// Infer container health from setup/report checks
	if status.SetupReport != nil {
		confirmed := 1 // API is running (we got the report)
		for _, check := range status.SetupReport.Checks {
			switch {
			case check.ID == "block_sync" && check.Status == StatusPass:
				confirmed += 2 // node + tmkms
			case strings.HasPrefix(check.ID, "mlnode_") && check.Status == StatusPass:
				confirmed += 2 // mlnode + inference (nginx)
			}
		}
		status.Overview.ContainersRunning = confirmed
		status.Overview.ContainersTotal = 8
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
			EpochNumber:        427,
			Active:             true,
			Weight:             1000,
			PoCWeight:          4200,
			MissPercentage:     2.0,
			MissedCount:        3,
			TotalCount:         150,
			InferenceCount:     147,
			TimeslotAllocation: []bool{true, true},
			PrevEpochClaimed:   true,
			PrevEpochIndex:     426,
			UpcomingEpoch:      428,
		},
		MLNode: MLNodeStatus{
			Enabled:        true,
			EnabledEpoch:   420,
			ModelName:      "Qwen/QwQ-32B",
			ModelLoaded:    true,
			GPUCount:       4,
			GPUName:        "NVIDIA GeForce RTX 4090",
			TPSize:         4,
			PPSize:         1,
			MemoryUtil:     0.92,
			MaxModelLen:    32768,
			PoCStatus:      "INFERENCE",
			IntendedStatus: "INFERENCE",
			PoCNodeStatus:  "IDLE",
			LastPoCTime:    time.Now().Add(-2 * time.Minute),
			LastPoCOK:      true,
			StatusUpdated:  time.Now().Add(-30 * time.Second),
		},
		NodeConfig: NodeConfigStatus{
			PublicURL:      "http://my-node.example.com:8000",
			PoCCallbackURL: "http://api:9100",
			APIVersion:     "v3.0.8",
			SeedAPIURL:     "http://89.169.111.79:8000",
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
