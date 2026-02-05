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
	Network string `json:"network"`

	// Keys
	KeyWorkflow   string `json:"key_workflow"` // "quick" or "secure"
	AccountPubKey string `json:"account_pubkey,omitempty"`
	KeyName       string `json:"key_name,omitempty"`

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
	P2PPort         int      `json:"p2p_port,omitempty"`
	APIPort         int      `json:"api_port,omitempty"`
	PersistentPeers []string `json:"persistent_peers,omitempty"`

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
	s.KeyWorkflow = ""
	s.AccountPubKey = ""
	s.KeyName = ""
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
	s.PersistentPeers = nil
	s.FirewallConfigured = false
	s.DDoSProtection = false
	s.DiskFreeGB = 0
	s.AutoUpdateOff = false
}
