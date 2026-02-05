package status

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testValidatorAddr = "ABCD1234"

func TestFetchBlockchainStatus(t *testing.T) {
	// Mock Tendermint /status and /net_info endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		resp := TendermintStatus{}
		resp.Result.SyncInfo.LatestBlockHeight = "1250000"
		resp.Result.SyncInfo.CatchingUp = false
		resp.Result.ValidatorInfo.Address = testValidatorAddr
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
	if status.Blockchain.ValidatorAddr != testValidatorAddr {
		t.Errorf("ValidatorAddr = %q, want %q", status.Blockchain.ValidatorAddr, testValidatorAddr)
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
	s := FetchMockedStatus()

	t.Run("Overview", func(t *testing.T) {
		if s.Overview.ContainersRunning != 8 {
			t.Errorf("ContainersRunning = %d, want 8", s.Overview.ContainersRunning)
		}
		if !s.Overview.NodeRegistered {
			t.Error("NodeRegistered should be true")
		}
		if s.Overview.EpochNumber == 0 {
			t.Error("EpochNumber should be non-zero")
		}
		if s.Overview.OverallStatus != StatusPass {
			t.Errorf("OverallStatus = %q, want PASS", s.Overview.OverallStatus)
		}
	})

	t.Run("Blockchain", func(t *testing.T) {
		if s.Blockchain.BlockHeight == 0 {
			t.Error("BlockHeight should be non-zero")
		}
		if !s.Blockchain.Synced {
			t.Error("Synced should be true")
		}
		if s.Blockchain.PeerCount == 0 {
			t.Error("PeerCount should be non-zero")
		}
	})

	t.Run("MLNode", func(t *testing.T) {
		if !s.MLNode.Enabled {
			t.Error("Enabled should be true")
		}
		if s.MLNode.ModelName == "" {
			t.Error("ModelName should not be empty")
		}
		if !s.MLNode.ModelLoaded {
			t.Error("ModelLoaded should be true")
		}
		if s.MLNode.GPUCount == 0 {
			t.Error("GPUCount should be non-zero")
		}
	})

	t.Run("Security", func(t *testing.T) {
		if !s.Security.FirewallConfigured {
			t.Error("FirewallConfigured should be true")
		}
		if !s.Security.DDoSProtection {
			t.Error("DDoSProtection should be true")
		}
		if !s.Security.ColdKeyConfigured {
			t.Error("ColdKeyConfigured should be true")
		}
		if !s.Security.WarmKeyConfigured {
			t.Error("WarmKeyConfigured should be true")
		}
		if !s.Security.PermissionsGranted {
			t.Error("PermissionsGranted should be true")
		}
	})

	t.Run("Epoch", func(t *testing.T) {
		if s.Epoch.EpochNumber == 0 {
			t.Error("EpochNumber should be non-zero")
		}
		if !s.Epoch.Active {
			t.Error("Active should be true")
		}
	})
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

// setupReportJSON returns a realistic setup/report response for testing
func setupReportJSON() string {
	return `{
		"overall_status": "PASS",
		"checks": [
			{"id": "cold_key_configured", "status": "PASS", "message": "Cold key is configured", "details": {"address": "gonka1abc", "pubkey": "Apub"}},
			{"id": "warm_key_in_keyring", "status": "PASS", "message": "Warm key is in keyring"},
			{"id": "permissions_granted", "status": "PASS", "message": "ML permissions granted", "details": {"granted": 27, "missing": 0}},
			{"id": "consensus_key_match", "status": "PASS", "message": "Consensus key matches"},
			{"id": "active_in_epoch", "status": "PASS", "message": "Active in epoch 62", "details": {"epoch": 62, "weight": 9120}},
			{"id": "block_sync", "status": "PASS", "message": "Block sync OK", "details": {"latest_height": 22216, "seconds_since_block": 8, "catching_up": false}},
			{"id": "missed_requests_threshold", "status": "PASS", "message": "Miss rate within threshold", "details": {"missed_percentage": 2.5, "missed_count": 5, "total_count": 200}},
			{"id": "mlnode_node1", "status": "PASS", "message": "MLNode healthy", "details": {"gpus": [{"name": "NVIDIA A100", "total_memory_gb": 80, "used_memory_gb": 72, "free_memory_gb": 8, "utilization_percent": 95, "temperature_c": 65, "available": true}], "models": ["Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"]}}
		],
		"summary": {"total_checks": 11, "passed_checks": 9, "failed_checks": 2}
	}`
}

func TestFetchSetupReport(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/setup/report", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(setupReportJSON()))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	s := &NodeStatus{}
	cfg := &StatusConfig{AdminURL: ts.URL}
	fetchSetupReport(s, cfg)

	t.Run("Overview", func(t *testing.T) {
		if s.Overview.OverallStatus != StatusPass {
			t.Errorf("OverallStatus = %q, want PASS", s.Overview.OverallStatus)
		}
		if s.Overview.ChecksPassed != 9 {
			t.Errorf("ChecksPassed = %d, want 9", s.Overview.ChecksPassed)
		}
		if s.Overview.ChecksTotal != 11 {
			t.Errorf("ChecksTotal = %d, want 11", s.Overview.ChecksTotal)
		}
	})

	t.Run("Epoch", func(t *testing.T) {
		if s.Epoch.EpochNumber != 62 {
			t.Errorf("EpochNumber = %d, want 62", s.Epoch.EpochNumber)
		}
		if s.Epoch.Weight != 9120 {
			t.Errorf("Weight = %d, want 9120", s.Epoch.Weight)
		}
		if !s.Epoch.Active {
			t.Error("Active should be true")
		}
		if s.Epoch.MissPercentage != 2.5 {
			t.Errorf("MissPercentage = %f, want 2.5", s.Epoch.MissPercentage)
		}
		if s.Epoch.MissedCount != 5 {
			t.Errorf("MissedCount = %d, want 5", s.Epoch.MissedCount)
		}
	})

	t.Run("Blockchain", func(t *testing.T) {
		if s.Blockchain.BlockHeight != 22216 {
			t.Errorf("BlockHeight = %d, want 22216", s.Blockchain.BlockHeight)
		}
		if s.Blockchain.SecondsSinceBlk != 8 {
			t.Errorf("SecondsSinceBlk = %d, want 8", s.Blockchain.SecondsSinceBlk)
		}
		if !s.Blockchain.Synced {
			t.Error("Synced should be true")
		}
	})

	t.Run("Security", func(t *testing.T) {
		if !s.Security.ColdKeyConfigured {
			t.Error("ColdKeyConfigured should be true")
		}
		if !s.Security.WarmKeyConfigured {
			t.Error("WarmKeyConfigured should be true")
		}
		if !s.Security.PermissionsGranted {
			t.Error("PermissionsGranted should be true")
		}
	})

	t.Run("MLNode", func(t *testing.T) {
		if s.MLNode.GPUCount != 1 {
			t.Errorf("GPUCount = %d, want 1", s.MLNode.GPUCount)
		}
		if s.MLNode.GPUName != "NVIDIA A100" {
			t.Errorf("GPUName = %q, want NVIDIA A100", s.MLNode.GPUName)
		}
		if s.MLNode.ModelName != "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8" {
			t.Errorf("ModelName = %q, unexpected", s.MLNode.ModelName)
		}
		if !s.MLNode.ModelLoaded {
			t.Error("ModelLoaded should be true")
		}
		// GPU detail fields
		if len(s.MLNode.GPUs) != 1 {
			t.Fatalf("GPUs len = %d, want 1", len(s.MLNode.GPUs))
		}
		gpu := s.MLNode.GPUs[0]
		if gpu.TotalMemoryGB != 80 {
			t.Errorf("TotalMemoryGB = %f, want 80", gpu.TotalMemoryGB)
		}
		if gpu.UtilizationPct != 95 {
			t.Errorf("UtilizationPct = %d, want 95", gpu.UtilizationPct)
		}
		if gpu.TemperatureC != 65 {
			t.Errorf("TemperatureC = %d, want 65", gpu.TemperatureC)
		}
	})

	t.Run("Registration", func(t *testing.T) {
		if !s.Overview.NodeRegistered {
			t.Error("NodeRegistered should be true from consensus_key_match")
		}
	})

	t.Run("RawReport", func(t *testing.T) {
		if s.SetupReport == nil {
			t.Fatal("SetupReport should not be nil")
		}
		if len(s.SetupReport.Checks) != 8 {
			t.Errorf("Checks len = %d, want 8", len(s.SetupReport.Checks))
		}
	})
}

func TestFetchSetupReport_WithFailures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/setup/report", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"overall_status": "FAIL",
			"checks": [
				{"id": "active_in_epoch", "status": "FAIL", "message": "Not active in epoch", "details": {"epoch": 0, "weight": 0}},
				{"id": "cold_key_configured", "status": "FAIL", "message": "Cold key not configured"},
				{"id": "warm_key_in_keyring", "status": "FAIL", "message": "Warm key not found"},
				{"id": "consensus_key_match", "status": "FAIL", "message": "Not registered"}
			],
			"summary": {"total_checks": 11, "passed_checks": 5, "failed_checks": 6}
		}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	status := &NodeStatus{}
	cfg := &StatusConfig{AdminURL: ts.URL}
	fetchSetupReport(status, cfg)

	if status.Overview.OverallStatus != "FAIL" {
		t.Errorf("OverallStatus = %q, want %q", status.Overview.OverallStatus, "FAIL")
	}
	if len(status.Overview.Issues) != 4 {
		t.Errorf("Issues count = %d, want 4", len(status.Overview.Issues))
	}
	if status.Epoch.Active {
		t.Error("Epoch.Active should be false")
	}
	if status.Security.ColdKeyConfigured {
		t.Error("ColdKeyConfigured should be false when check fails")
	}
	if status.Security.WarmKeyConfigured {
		t.Error("WarmKeyConfigured should be false when check fails")
	}
	if status.Overview.NodeRegistered {
		t.Error("NodeRegistered should be false when consensus_key_match fails")
	}
}

func TestFetchSetupReport_Unavailable(t *testing.T) {
	status := &NodeStatus{}
	cfg := &StatusConfig{AdminURL: "http://127.0.0.1:1"}
	fetchSetupReport(status, cfg)

	if status.SetupReport != nil {
		t.Error("SetupReport should be nil when unavailable")
	}
	if status.Overview.OverallStatus != "" {
		t.Errorf("OverallStatus should be empty, got %q", status.Overview.OverallStatus)
	}
}

func TestFetchAdminConfig(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"current_seed": {"epoch_index": 62},
			"previous_seed": {"epoch_index": 61},
			"current_height": 22300,
			"nodes": [{
				"id": "node1",
				"host": "inference",
				"inference_port": 5000,
				"poc_port": 8080,
				"max_concurrent": 500,
				"enabled": true,
				"models": {
					"Qwen/QwQ-32B": {
						"args": ["--tensor-parallel-size", "4", "--gpu-memory-utilization", "0.90", "--max-model-len", "32768"]
					}
				},
				"hardware": [{"type": "NVIDIA GeForce RTX 4090 | 24GB", "count": 4}]
			}]
		}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	status := &NodeStatus{}
	status.Blockchain.BlockHeight = 22200 // Pretend we got this from Tendermint
	cfg := &StatusConfig{AdminURL: ts.URL}
	fetchAdminConfig(status, cfg)

	// Epoch
	if status.Overview.EpochNumber != 62 {
		t.Errorf("EpochNumber = %d, want 62", status.Overview.EpochNumber)
	}

	// Block lag
	if status.Blockchain.NetworkHeight != 22300 {
		t.Errorf("NetworkHeight = %d, want 22300", status.Blockchain.NetworkHeight)
	}
	if status.Blockchain.BlockLag != 100 {
		t.Errorf("BlockLag = %d, want 100", status.Blockchain.BlockLag)
	}

	// ML Node config
	if status.MLNode.ModelName != "Qwen/QwQ-32B" {
		t.Errorf("ModelName = %q, want Qwen/QwQ-32B", status.MLNode.ModelName)
	}
	if status.MLNode.TPSize != 4 {
		t.Errorf("TPSize = %d, want 4", status.MLNode.TPSize)
	}
	if status.MLNode.MemoryUtil != 0.90 {
		t.Errorf("MemoryUtil = %f, want 0.90", status.MLNode.MemoryUtil)
	}
	if status.MLNode.MaxModelLen != 32768 {
		t.Errorf("MaxModelLen = %d, want 32768", status.MLNode.MaxModelLen)
	}
	if status.MLNode.Hardware != "4x NVIDIA GeForce RTX 4090 | 24GB" {
		t.Errorf("Hardware = %q, unexpected", status.MLNode.Hardware)
	}
	if !status.MLNode.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestFetchAdminConfig_Unavailable(t *testing.T) {
	status := &NodeStatus{}
	cfg := &StatusConfig{AdminURL: "http://127.0.0.1:1"}
	fetchAdminConfig(status, cfg)

	if status.Blockchain.NetworkHeight != 0 {
		t.Errorf("NetworkHeight should be 0 when unavailable")
	}
}

func TestFetchValidatorSet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": {
				"validators": [
					{"address": "ABCD1234", "pub_key": {"type": "tendermint/PubKeyEd25519", "value": "abc"}, "voting_power": "9120"},
					{"address": "OTHER5678", "pub_key": {"type": "tendermint/PubKeyEd25519", "value": "def"}, "voting_power": "5000"}
				]
			}
		}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	status := &NodeStatus{}
	status.Blockchain.ValidatorAddr = testValidatorAddr
	cfg := &StatusConfig{TendermintURL: ts.URL}
	fetchValidatorSet(status, cfg)

	if !status.Blockchain.IsValidator {
		t.Error("IsValidator should be true")
	}
	if status.Blockchain.VotingPower != 9120 {
		t.Errorf("VotingPower = %d, want 9120", status.Blockchain.VotingPower)
	}
}

func TestFetchValidatorSet_NotInSet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": {
				"validators": [
					{"address": "OTHER5678", "pub_key": {"type": "tendermint/PubKeyEd25519", "value": "def"}, "voting_power": "5000"}
				]
			}
		}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	status := &NodeStatus{}
	status.Blockchain.ValidatorAddr = "NOTFOUND"
	cfg := &StatusConfig{TendermintURL: ts.URL}
	fetchValidatorSet(status, cfg)

	if status.Blockchain.IsValidator {
		t.Error("IsValidator should be false when not in set")
	}
	if status.Blockchain.VotingPower != 0 {
		t.Errorf("VotingPower should be 0, got %d", status.Blockchain.VotingPower)
	}
}

func TestParseModelArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantTP      int
		wantPP      int
		wantMemUtil float64
		wantMaxLen  int
	}{
		{
			name:        "Full args",
			args:        []string{"--tensor-parallel-size", "8", "--pipeline-parallel-size", "2", "--gpu-memory-utilization", "0.88", "--max-model-len", "240000"},
			wantTP:      8,
			wantPP:      2,
			wantMemUtil: 0.88,
			wantMaxLen:  240000,
		},
		{
			name:   "TP only",
			args:   []string{"--tensor-parallel-size", "4"},
			wantTP: 4,
		},
		{
			name: "Empty args",
			args: []string{},
		},
		{
			name: "Unknown args",
			args: []string{"--quantization", "fp8", "--kv-cache-dtype", "fp8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ml := &MLNodeStatus{}
			parseModelArgs(ml, tt.args)
			if ml.TPSize != tt.wantTP {
				t.Errorf("TPSize = %d, want %d", ml.TPSize, tt.wantTP)
			}
			if ml.PPSize != tt.wantPP {
				t.Errorf("PPSize = %d, want %d", ml.PPSize, tt.wantPP)
			}
			if ml.MemoryUtil != tt.wantMemUtil {
				t.Errorf("MemoryUtil = %f, want %f", ml.MemoryUtil, tt.wantMemUtil)
			}
			if ml.MaxModelLen != tt.wantMaxLen {
				t.Errorf("MaxModelLen = %d, want %d", ml.MaxModelLen, tt.wantMaxLen)
			}
		})
	}
}

func TestFetchFullStatus_AllEndpoints(t *testing.T) {
	// Create a mock server that serves all endpoints
	mux := http.NewServeMux()

	// Setup report
	mux.HandleFunc("/admin/v1/setup/report", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(setupReportJSON()))
	})

	// Admin config
	mux.HandleFunc("/admin/v1/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"current_seed": {"epoch_index": 62},
			"current_height": 22300,
			"nodes": [{
				"id": "node1", "host": "inference", "inference_port": 5000, "poc_port": 8080,
				"max_concurrent": 500, "enabled": true,
				"models": {"Qwen/Qwen3-235B-A22B-Instruct-2507-FP8": {"args": ["--tensor-parallel-size", "8"]}},
				"hardware": [{"type": "NVIDIA A100 80GB", "count": 8}]
			}]
		}`))
	})

	// Admin nodes
	mux.HandleFunc("/admin/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id": "node1", "host": "inference", "inference_port": 5000, "poc_port": 8080, "max_concurrent": 500, "models": ["Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"], "enabled": true}]`))
	})

	adminTS := httptest.NewServer(mux)
	defer adminTS.Close()

	// Tendermint mock
	tmMux := http.NewServeMux()
	tmMux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		resp := TendermintStatus{}
		resp.Result.SyncInfo.LatestBlockHeight = "22250"
		resp.Result.SyncInfo.CatchingUp = false
		resp.Result.ValidatorInfo.Address = "VAL_ADDR_123"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	tmMux.HandleFunc("/net_info", func(w http.ResponseWriter, _ *http.Request) {
		resp := TendermintNetInfo{}
		resp.Result.NPeers = "8"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	tmMux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result": {"validators": [{"address": "VAL_ADDR_123", "voting_power": "9120"}]}}`))
	})
	tmTS := httptest.NewServer(tmMux)
	defer tmTS.Close()

	// vLLM mock
	vllmMux := http.NewServeMux()
	vllmMux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": []}`))
	})
	vllmTS := httptest.NewServer(vllmMux)
	defer vllmTS.Close()

	cfg := &StatusConfig{
		TendermintURL: tmTS.URL,
		AdminURL:      adminTS.URL,
		VLLMHealthURL: vllmTS.URL,
	}

	s, err := FetchStatusWithConfig("", cfg)
	if err != nil {
		t.Fatalf("FetchStatusWithConfig() error: %v", err)
	}

	t.Run("Overview", func(t *testing.T) {
		if s.Overview.OverallStatus != StatusPass {
			t.Errorf("OverallStatus = %q, want PASS", s.Overview.OverallStatus)
		}
		if s.Overview.ChecksPassed != 9 {
			t.Errorf("ChecksPassed = %d, want 9", s.Overview.ChecksPassed)
		}
		if !s.Overview.NodeRegistered {
			t.Error("NodeRegistered should be true")
		}
	})

	t.Run("Blockchain", func(t *testing.T) {
		// Tendermint RPC overrides setup/report height
		if s.Blockchain.BlockHeight != 22250 {
			t.Errorf("BlockHeight = %d, want 22250 (from Tendermint RPC)", s.Blockchain.BlockHeight)
		}
		if s.Blockchain.PeerCount != 8 {
			t.Errorf("PeerCount = %d, want 8", s.Blockchain.PeerCount)
		}
		if s.Blockchain.NetworkHeight != 22300 {
			t.Errorf("NetworkHeight = %d, want 22300", s.Blockchain.NetworkHeight)
		}
		if !s.Blockchain.IsValidator {
			t.Error("IsValidator should be true")
		}
		if s.Blockchain.VotingPower != 9120 {
			t.Errorf("VotingPower = %d, want 9120", s.Blockchain.VotingPower)
		}
	})

	t.Run("Epoch", func(t *testing.T) {
		if s.Epoch.EpochNumber != 62 {
			t.Errorf("EpochNumber = %d, want 62", s.Epoch.EpochNumber)
		}
		if !s.Epoch.Active {
			t.Error("Active should be true")
		}
	})

	t.Run("MLNode", func(t *testing.T) {
		if !s.MLNode.Enabled {
			t.Error("Enabled should be true")
		}
		if !s.MLNode.ModelLoaded {
			t.Error("ModelLoaded should be true")
		}
		if s.MLNode.TPSize != 8 {
			t.Errorf("TPSize = %d, want 8", s.MLNode.TPSize)
		}
	})

	t.Run("Security", func(t *testing.T) {
		if !s.Security.ColdKeyConfigured {
			t.Error("ColdKeyConfigured should be true")
		}
		if !s.Security.WarmKeyConfigured {
			t.Error("WarmKeyConfigured should be true")
		}
		if !s.Security.PermissionsGranted {
			t.Error("PermissionsGranted should be true")
		}
	})
}
