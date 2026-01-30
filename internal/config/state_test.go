package config

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testNetworkMainnet   = "mainnet"
	testKeyWorkflowQuick = "quick"
)

func TestNewState(t *testing.T) {
	state := NewState("/tmp/test-gonka")

	if state.OutputDir != "/tmp/test-gonka" {
		t.Errorf("expected OutputDir '/tmp/test-gonka', got '%s'", state.OutputDir)
	}
	if state.P2PPort != 5000 {
		t.Errorf("expected P2PPort 5000, got %d", state.P2PPort)
	}
	if state.APIPort != 8000 {
		t.Errorf("expected APIPort 8000, got %d", state.APIPort)
	}
	if len(state.CompletedPhases) != 0 {
		t.Errorf("expected empty CompletedPhases, got %d", len(state.CompletedPhases))
	}
}

func TestStateSaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create and save state
	state := NewState(tmpDir)
	state.Network = testNetworkMainnet
	state.KeyWorkflow = testKeyWorkflowQuick
	state.PublicIP = "1.2.3.4"

	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Verify file exists
	statePath := filepath.Join(tmpDir, "state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	// Load state
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Verify loaded values
	if loaded.Network != testNetworkMainnet {
		t.Errorf("expected Network %q, got %q", testNetworkMainnet, loaded.Network)
	}
	if loaded.KeyWorkflow != testKeyWorkflowQuick {
		t.Errorf("expected KeyWorkflow %q, got %q", testKeyWorkflowQuick, loaded.KeyWorkflow)
	}
	if loaded.PublicIP != "1.2.3.4" {
		t.Errorf("expected PublicIP '1.2.3.4', got '%s'", loaded.PublicIP)
	}
}

func TestLoadNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Load from empty directory should return new state
	state, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error loading non-existent state: %v", err)
	}
	if state == nil {
		t.Fatal("expected new state, got nil")
	}
}

func TestMarkPhaseComplete(t *testing.T) {
	state := NewState("/tmp/test")

	state.MarkPhaseComplete("Prerequisites")
	state.MarkPhaseComplete("GPU Detection")

	if len(state.CompletedPhases) != 2 {
		t.Errorf("expected 2 completed phases, got %d", len(state.CompletedPhases))
	}
	if !state.IsPhaseComplete("Prerequisites") {
		t.Error("Prerequisites should be complete")
	}
	if !state.IsPhaseComplete("GPU Detection") {
		t.Error("GPU Detection should be complete")
	}
	if state.IsPhaseComplete("Deployment") {
		t.Error("Deployment should not be complete")
	}
}

func TestReset(t *testing.T) {
	state := NewState("/tmp/test")
	state.Network = testNetworkMainnet
	state.KeyWorkflow = testKeyWorkflowQuick
	state.GPUs = []GPUInfo{{Index: 0, Name: "RTX 4090"}}
	state.MarkPhaseComplete("Test")

	state.Reset()

	if state.Network != "" {
		t.Error("Network should be empty after reset")
	}
	if state.KeyWorkflow != "" {
		t.Error("KeyWorkflow should be empty after reset")
	}
	if len(state.GPUs) != 0 {
		t.Error("GPUs should be empty after reset")
	}
	if len(state.CompletedPhases) != 0 {
		t.Error("CompletedPhases should be empty after reset")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write invalid JSON to state file
	statePath := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(statePath, []byte("not valid json{{{"), 0600); err != nil {
		t.Fatalf("failed to write invalid state: %v", err)
	}

	_, err = Load(tmpDir)
	if err == nil {
		t.Error("expected error loading invalid JSON, got nil")
	}
}

func TestSaveReadOnlyDir(t *testing.T) {
	// Attempt to save to a path that cannot exist
	state := NewState("/proc/nonexistent/deeply/nested/path")
	err := state.Save()
	if err == nil {
		t.Error("expected error saving to invalid path, got nil")
	}
}

func TestMarkPhaseCompleteDuplicate(t *testing.T) {
	state := NewState("/tmp/test")

	state.MarkPhaseComplete("Phase1")
	state.MarkPhaseComplete("Phase1")

	// Current behavior: duplicates are added
	if len(state.CompletedPhases) != 2 {
		t.Errorf("expected 2 entries (duplicate allowed), got %d", len(state.CompletedPhases))
	}
	// But IsPhaseComplete still returns true
	if !state.IsPhaseComplete("Phase1") {
		t.Error("IsPhaseComplete should return true")
	}
}

func TestIsPhaseCompleteEmpty(t *testing.T) {
	state := NewState("/tmp/test")

	if state.IsPhaseComplete("Prerequisites") {
		t.Error("empty CompletedPhases should return false for any phase")
	}
	if state.IsPhaseComplete("GPU Detection") {
		t.Error("empty CompletedPhases should return false for any phase")
	}
	if state.IsPhaseComplete("") {
		t.Error("empty CompletedPhases should return false for empty string")
	}
}

func TestResetPreservesOutputDir(t *testing.T) {
	state := NewState("/test/output")
	state.Network = testNetworkMainnet
	state.Reset()

	if state.OutputDir != "/test/output" {
		t.Errorf("OutputDir should be preserved after reset, got %q", state.OutputDir)
	}
}

func TestSaveAndLoadPreservesGPUFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := NewState(tmpDir)
	state.GPUs = []GPUInfo{
		{Index: 0, Name: "NVIDIA H100 80GB", MemoryMB: 81920, Architecture: "sm_90"},
		{Index: 1, Name: "NVIDIA H100 80GB", MemoryMB: 81920, Architecture: "sm_90"},
	}
	state.GPUTopology = GPUTopology{HasNVLink: true, PCIeVersion: "5.0", Interconnect: "nvlink"}
	state.DriverInfo = DriverInfo{UserVersion: "570.133.20", KernelVersion: "570.133.20", Consistent: true}
	state.TPSize = 8
	state.PPSize = 1
	state.GPUMemoryUtil = 0.90
	state.MaxModelLen = 240000
	state.KVCacheDtype = "fp8"
	state.MLNodeImageTag = "3.0.12"
	state.AttentionBackend = "FLASH_ATTN"

	if err := state.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(loaded.GPUs) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(loaded.GPUs))
	}
	if loaded.GPUs[0].Architecture != "sm_90" {
		t.Errorf("GPU arch = %q, want %q", loaded.GPUs[0].Architecture, "sm_90")
	}
	if !loaded.GPUTopology.HasNVLink {
		t.Error("expected HasNVLink=true")
	}
	if loaded.TPSize != 8 {
		t.Errorf("TPSize = %d, want 8", loaded.TPSize)
	}
	if loaded.GPUMemoryUtil != 0.90 {
		t.Errorf("GPUMemoryUtil = %f, want 0.90", loaded.GPUMemoryUtil)
	}
	if loaded.KVCacheDtype != "fp8" {
		t.Errorf("KVCacheDtype = %q, want %q", loaded.KVCacheDtype, "fp8")
	}
}
