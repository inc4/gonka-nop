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
