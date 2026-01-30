package status

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchBlockchainStatus(t *testing.T) {
	// Mock Tendermint /status and /net_info endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		resp := TendermintStatus{}
		resp.Result.SyncInfo.LatestBlockHeight = "1250000"
		resp.Result.SyncInfo.CatchingUp = false
		resp.Result.ValidatorInfo.Address = "ABCD1234"
		resp.Result.NodeInfo.Network = "gonka-mainnet"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/net_info", func(w http.ResponseWriter, _ *http.Request) {
		resp := TendermintNetInfo{}
		resp.Result.NPeers = "12"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := &StatusConfig{
		TendermintURL: ts.URL,
		AdminURL:      "http://127.0.0.1:1", // unreachable on purpose
		VLLMHealthURL: "http://127.0.0.1:1",
	}

	status, err := FetchStatusWithConfig("", cfg)
	if err != nil {
		t.Fatalf("FetchStatusWithConfig() error: %v", err)
	}

	if status.Blockchain.BlockHeight != 1250000 {
		t.Errorf("BlockHeight = %d, want 1250000", status.Blockchain.BlockHeight)
	}
	if status.Blockchain.CatchingUp {
		t.Error("CatchingUp should be false")
	}
	if !status.Blockchain.Synced {
		t.Error("Synced should be true")
	}
	if status.Blockchain.ValidatorAddr != "ABCD1234" {
		t.Errorf("ValidatorAddr = %q, want %q", status.Blockchain.ValidatorAddr, "ABCD1234")
	}
	if !status.Blockchain.IsValidator {
		t.Error("IsValidator should be true")
	}
	if status.Blockchain.PeerCount != 12 {
		t.Errorf("PeerCount = %d, want 12", status.Blockchain.PeerCount)
	}
}

func TestFetchBlockchainStatus_Unavailable(t *testing.T) {
	// Point to unreachable server
	cfg := &StatusConfig{
		TendermintURL: "http://127.0.0.1:1",
		AdminURL:      "http://127.0.0.1:1",
		VLLMHealthURL: "http://127.0.0.1:1",
	}

	status, err := FetchStatusWithConfig("", cfg)
	if err != nil {
		t.Fatalf("FetchStatusWithConfig() should not error on unreachable: %v", err)
	}

	if status.Blockchain.BlockHeight != 0 {
		t.Errorf("BlockHeight = %d, want 0 when unavailable", status.Blockchain.BlockHeight)
	}
	if status.Blockchain.PeerCount != 0 {
		t.Errorf("PeerCount = %d, want 0 when unavailable", status.Blockchain.PeerCount)
	}
	if status.Blockchain.Synced {
		t.Error("Synced should be false when unavailable")
	}
}

func TestFetchMLNodeStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		nodes := []AdminMLNode{
			{
				ID:            "node1",
				Host:          "inference",
				InferencePort: 5000,
				PoCPort:       8080,
				MaxConcurrent: 500,
				Models:        []string{"Qwen/QwQ-32B"},
				Enabled:       true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodes)
	})
	adminTS := httptest.NewServer(mux)
	defer adminTS.Close()

	vllmMux := http.NewServeMux()
	vllmMux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": []}`))
	})
	vllmTS := httptest.NewServer(vllmMux)
	defer vllmTS.Close()

	cfg := &StatusConfig{
		TendermintURL: "http://127.0.0.1:1", // unreachable
		AdminURL:      adminTS.URL,
		VLLMHealthURL: vllmTS.URL,
	}

	status, err := FetchStatusWithConfig("", cfg)
	if err != nil {
		t.Fatalf("FetchStatusWithConfig() error: %v", err)
	}

	if !status.MLNode.Enabled {
		t.Error("MLNode.Enabled should be true")
	}
	if status.MLNode.ModelName != "Qwen/QwQ-32B" {
		t.Errorf("MLNode.ModelName = %q, want %q", status.MLNode.ModelName, "Qwen/QwQ-32B")
	}
	if !status.MLNode.ModelLoaded {
		t.Error("MLNode.ModelLoaded should be true")
	}
}

func TestFetchOverviewStatus(t *testing.T) {
	tests := []struct {
		name           string
		blockHeight    int64
		modelLoaded    bool
		wantContainers int
		wantRegistered bool
		validatorAddr  string
	}{
		{
			name:           "All services down",
			blockHeight:    0,
			modelLoaded:    false,
			wantContainers: 0,
			wantRegistered: false,
		},
		{
			name:           "Blockchain up only",
			blockHeight:    100,
			modelLoaded:    false,
			wantContainers: 3,
			wantRegistered: true,
			validatorAddr:  "ABC123",
		},
		{
			name:           "Blockchain + ML up",
			blockHeight:    100,
			modelLoaded:    true,
			wantContainers: 4,
			wantRegistered: true,
			validatorAddr:  "ABC123",
		},
		{
			name:           "ML up but blockchain down",
			blockHeight:    0,
			modelLoaded:    true,
			wantContainers: 1,
			wantRegistered: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &NodeStatus{}
			status.Blockchain.BlockHeight = tt.blockHeight
			status.Blockchain.ValidatorAddr = tt.validatorAddr
			status.MLNode.ModelLoaded = tt.modelLoaded

			fetchOverviewStatus(status)

			if status.Overview.ContainersRunning != tt.wantContainers {
				t.Errorf("ContainersRunning = %d, want %d", status.Overview.ContainersRunning, tt.wantContainers)
			}
			if status.Overview.ContainersTotal != 8 {
				t.Errorf("ContainersTotal = %d, want 8", status.Overview.ContainersTotal)
			}
			if status.Overview.NodeRegistered != tt.wantRegistered {
				t.Errorf("NodeRegistered = %v, want %v", status.Overview.NodeRegistered, tt.wantRegistered)
			}
		})
	}
}

func TestFetchMockedStatus(t *testing.T) {
	status := FetchMockedStatus()

	if status.Overview.ContainersRunning != 8 {
		t.Errorf("ContainersRunning = %d, want 8", status.Overview.ContainersRunning)
	}
	if !status.Overview.NodeRegistered {
		t.Error("NodeRegistered should be true")
	}
	if status.Overview.EpochNumber == 0 {
		t.Error("EpochNumber should be non-zero")
	}
	if status.Blockchain.BlockHeight == 0 {
		t.Error("BlockHeight should be non-zero")
	}
	if !status.Blockchain.Synced {
		t.Error("Synced should be true")
	}
	if status.Blockchain.PeerCount == 0 {
		t.Error("PeerCount should be non-zero")
	}
	if !status.MLNode.Enabled {
		t.Error("MLNode.Enabled should be true")
	}
	if status.MLNode.ModelName == "" {
		t.Error("MLNode.ModelName should not be empty")
	}
	if !status.MLNode.ModelLoaded {
		t.Error("MLNode.ModelLoaded should be true")
	}
	if status.MLNode.GPUCount == 0 {
		t.Error("MLNode.GPUCount should be non-zero")
	}
	if !status.Security.FirewallConfigured {
		t.Error("Security.FirewallConfigured should be true")
	}
	if !status.Security.DDoSProtection {
		t.Error("Security.DDoSProtection should be true")
	}
	if !status.Security.DriverConsistent {
		t.Error("Security.DriverConsistent should be true")
	}
}

func TestFetchStatusWithConfig_NilConfig(t *testing.T) {
	// nil config should use defaults (which point to localhost and likely fail gracefully)
	status, err := FetchStatusWithConfig("", nil)
	if err != nil {
		t.Fatalf("FetchStatusWithConfig(nil) should not error: %v", err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
}

func TestFetchMLNodeStatus_EmptyNodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := &StatusConfig{
		TendermintURL: "http://127.0.0.1:1",
		AdminURL:      ts.URL,
		VLLMHealthURL: "http://127.0.0.1:1",
	}

	status, err := FetchStatusWithConfig("", cfg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if status.MLNode.Enabled {
		t.Error("MLNode.Enabled should be false with empty nodes")
	}
	if status.MLNode.ModelName != "" {
		t.Errorf("MLNode.ModelName should be empty, got %q", status.MLNode.ModelName)
	}
}
