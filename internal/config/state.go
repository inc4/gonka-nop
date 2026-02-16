package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// GPUInfo holds information about a detected GPU
type GPUInfo struct {
	Index         int    `json:"index"`
	Name          string `json:"name"`
	MemoryMB      int    `json:"memory_mb"`
	CUDAVersion   string `json:"cuda_version,omitempty"`
	DriverVersion string `json:"driver_version,omitempty"`
	Architecture  string `json:"architecture,omitempty"` // e.g. "sm_80", "sm_89", "sm_90", "sm_120"
	PCIBusID      string `json:"pci_bus_id,omitempty"`
}

// GPUTopology holds inter-GPU connectivity info
type GPUTopology struct {
	HasNVLink    bool   `json:"has_nvlink"`
	PCIeVersion  string `json:"pcie_version,omitempty"` // "3.0", "4.0", "5.0"
	Interconnect string `json:"interconnect,omitempty"` // "nvlink", "pcie", "unknown"
}

// DriverInfo holds NVIDIA driver version details
type DriverInfo struct {
	UserVersion   string `json:"user_version,omitempty"`   // userspace lib version
	KernelVersion string `json:"kernel_version,omitempty"` // kernel module version
	FMVersion     string `json:"fm_version,omitempty"`     // Fabric Manager version
	Consistent    bool   `json:"consistent"`               // all versions match
}

// State holds the persistent state of the setup process
type State struct {
	// Setup progress
	CurrentPhase    string   `json:"current_phase"`
	CompletedPhases []string `json:"completed_phases"`

	// Network
	Network   string `json:"network"`
	ChainID   string `json:"chain_id,omitempty"`
	IsTestNet bool   `json:"is_test_net,omitempty"`

	// Network seeds & images
	ImageVersion    string `json:"image_version,omitempty"`
	SeedAPIURL      string `json:"seed_api_url,omitempty"`
	SeedRPCURL      string `json:"seed_rpc_url,omitempty"`
	SeedP2PURL      string `json:"seed_p2p_url,omitempty"`
	EnforcedModelID string `json:"enforced_model_id,omitempty"`
	EthereumNetwork string `json:"ethereum_network,omitempty"` // "mainnet" or "sepolia"
	BeaconStateURL  string `json:"beacon_state_url,omitempty"`
	BridgeImageTag  string `json:"bridge_image_tag,omitempty"`

	// Keys
	KeyWorkflow     string `json:"key_workflow"` // "quick" or "secure"
	AccountPubKey   string `json:"account_pubkey,omitempty"`
	KeyName         string `json:"key_name,omitempty"`
	ColdKeyName     string `json:"cold_key_name,omitempty"`
	ColdKeyAddress  string `json:"cold_key_address,omitempty"`
	WarmKeyAddress  string `json:"warm_key_address,omitempty"`
	KeyringPassword string `json:"-"` // never persisted
	KeyringDir      string `json:"keyring_dir,omitempty"`

	// GPU Configuration
	GPUs             []GPUInfo   `json:"gpus,omitempty"`
	GPUTopology      GPUTopology `json:"gpu_topology,omitempty"`
	DriverInfo       DriverInfo  `json:"driver_info,omitempty"`
	SelectedModel    string      `json:"selected_model,omitempty"`
	TPSize           int         `json:"tp_size,omitempty"`
	PPSize           int         `json:"pp_size,omitempty"`
	GPUMemoryUtil    float64     `json:"gpu_memory_util,omitempty"`   // 0.88-0.94 recommended
	MaxModelLen      int         `json:"max_model_len,omitempty"`     // calculated from VRAM
	KVCacheDtype     string      `json:"kv_cache_dtype,omitempty"`    // "auto" or "fp8"
	MLNodeImageTag   string      `json:"mlnode_image_tag,omitempty"`  // "3.0.12", "3.0.12-blackwell", etc.
	AttentionBackend string      `json:"attention_backend,omitempty"` // "FLASH_ATTN" or "FLASHINFER"

	// Paths
	OutputDir string `json:"output_dir"`
	HFHome    string `json:"hf_home,omitempty"`

	// Network Configuration
	PublicIP        string   `json:"public_ip,omitempty"`
	P2PPort         int      `json:"p2p_port,omitempty"`          // external-facing P2P port (advertised to peers)
	APIPort         int      `json:"api_port,omitempty"`          // external-facing API port (used in PUBLIC_URL)
	InternalP2PPort int      `json:"internal_p2p_port,omitempty"` // Docker binding inside VM (default 5000)
	InternalAPIPort int      `json:"internal_api_port,omitempty"` // Docker binding inside VM (default 8000)
	PersistentPeers []string `json:"persistent_peers,omitempty"`

	// ML Node ports & identity
	InferencePort int    `json:"inference_port,omitempty"` // host-mapped port, default 5050
	PoCPort       int    `json:"poc_port,omitempty"`       // host-mapped port, default 8080
	MLNodeID      string `json:"mlnode_id,omitempty"`      // default "node1"

	// Deploy
	UseSudo      bool     `json:"use_sudo,omitempty"`
	ComposeFiles []string `json:"compose_files,omitempty"` // defaults: ["docker-compose.yml", "docker-compose.mlnode.yml"]
	AdminURL     string   `json:"admin_url,omitempty"`     // default: "http://localhost:9200"
	RPCURL       string   `json:"rpc_url,omitempty"`       // default: "http://localhost:26657"

	// Registration
	ConsensusKey   string `json:"consensus_key,omitempty"` // ed25519 base64 from TMKMS
	NodeRegistered bool   `json:"node_registered,omitempty"`
	PermGranted    bool   `json:"perm_granted,omitempty"`
	PublicURL      string `json:"public_url,omitempty"` // full URL: http://IP:port

	// Security
	FirewallConfigured bool `json:"firewall_configured,omitempty"`
	DDoSProtection     bool `json:"ddos_protection,omitempty"`

	// System
	DiskFreeGB    int  `json:"disk_free_gb,omitempty"`
	AutoUpdateOff bool `json:"auto_update_off,omitempty"` // unattended-upgrades disabled

	// Internal
	statePath string `json:"-"`
}

// NewState creates a new state with defaults
func NewState(outputDir string) *State {
	return &State{
		OutputDir:       outputDir,
		CompletedPhases: []string{},
		P2PPort:         5000,
		APIPort:         8000,
		InternalP2PPort: 5000,
		InternalAPIPort: 8000,
		InferencePort:   5050,
		PoCPort:         8080,
		MLNodeID:        "node1",
		statePath:       filepath.Join(outputDir, "state.json"),
	}
}

// Load loads state from the output directory
func Load(outputDir string) (*State, error) {
	// Clean and validate path to prevent directory traversal
	cleanDir := filepath.Clean(outputDir)
	statePath := filepath.Join(cleanDir, "state.json")

	data, err := os.ReadFile(statePath) // #nosec G304 - path is cleaned above
	if err != nil {
		if os.IsNotExist(err) {
			// Return new state if file doesn't exist
			return NewState(cleanDir), nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	state.statePath = statePath
	state.OutputDir = outputDir
	return &state, nil
}

// Save persists the state to disk
func (s *State) Save() error {
	// Ensure output directory exists
	if err := os.MkdirAll(s.OutputDir, 0750); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.statePath, data, 0600)
}

// MarkPhaseComplete marks a phase as completed
func (s *State) MarkPhaseComplete(phaseName string) {
	s.CompletedPhases = append(s.CompletedPhases, phaseName)
	s.CurrentPhase = ""
}

// IsPhaseComplete checks if a phase has been completed
func (s *State) IsPhaseComplete(phaseName string) bool {
	for _, p := range s.CompletedPhases {
		if p == phaseName {
			return true
		}
	}
	return false
}

// Reset clears all state
func (s *State) Reset() {
	s.CurrentPhase = ""
	s.CompletedPhases = []string{}
	s.Network = ""
	s.ChainID = ""
	s.IsTestNet = false
	s.ImageVersion = ""
	s.SeedAPIURL = ""
	s.SeedRPCURL = ""
	s.SeedP2PURL = ""
	s.EnforcedModelID = ""
	s.EthereumNetwork = ""
	s.BeaconStateURL = ""
	s.BridgeImageTag = ""
	s.KeyWorkflow = ""
	s.AccountPubKey = ""
	s.KeyName = ""
	s.ColdKeyName = ""
	s.ColdKeyAddress = ""
	s.WarmKeyAddress = ""
	s.KeyringPassword = ""
	s.KeyringDir = ""
	s.GPUs = nil
	s.GPUTopology = GPUTopology{}
	s.DriverInfo = DriverInfo{}
	s.SelectedModel = ""
	s.TPSize = 0
	s.PPSize = 0
	s.GPUMemoryUtil = 0
	s.MaxModelLen = 0
	s.KVCacheDtype = ""
	s.MLNodeImageTag = ""
	s.AttentionBackend = ""
	s.HFHome = ""
	s.PublicIP = ""
	s.InternalP2PPort = 0
	s.InternalAPIPort = 0
	s.PersistentPeers = nil
	s.InferencePort = 0
	s.PoCPort = 0
	s.MLNodeID = ""
	s.UseSudo = false
	s.ComposeFiles = nil
	s.AdminURL = ""
	s.RPCURL = ""
	s.ConsensusKey = ""
	s.NodeRegistered = false
	s.PermGranted = false
	s.PublicURL = ""
	s.FirewallConfigured = false
	s.DDoSProtection = false
	s.DiskFreeGB = 0
	s.AutoUpdateOff = false
}
