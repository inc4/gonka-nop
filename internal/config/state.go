package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// GPUInfo holds information about a detected GPU
type GPUInfo struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	MemoryMB    int    `json:"memory_mb"`
	CUDAVersion string `json:"cuda_version,omitempty"`
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
	GPUs          []GPUInfo `json:"gpus,omitempty"`
	SelectedModel string    `json:"selected_model,omitempty"`
	TPSize        int       `json:"tp_size,omitempty"`
	PPSize        int       `json:"pp_size,omitempty"`

	// Paths
	OutputDir string `json:"output_dir"`
	HFHome    string `json:"hf_home,omitempty"`

	// Network Configuration
	PublicIP string `json:"public_ip,omitempty"`
	P2PPort  int    `json:"p2p_port,omitempty"`
	APIPort  int    `json:"api_port,omitempty"`

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
	statePath := filepath.Join(outputDir, "state.json")

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return new state if file doesn't exist
			return NewState(outputDir), nil
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
	if err := os.MkdirAll(s.OutputDir, 0755); err != nil {
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
	s.SelectedModel = ""
	s.TPSize = 0
	s.PPSize = 0
	s.HFHome = ""
	s.PublicIP = ""
}
