package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inc4/gonka-nop/internal/status"
)

const (
	testNodeID1       = "node1"
	testNodeID2       = "node2"
	testStatusInfer   = "INFERENCE"
	testStatusFailed  = "FAILED"
	testModelNameNone = notAvailable
)

func newTestAdminServer(t *testing.T, entries []status.AdminNodesEntry) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	})
	mux.HandleFunc("/admin/v1/nodes/node1/enable", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/admin/v1/nodes/node1/disable", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/admin/v1/nodes/bad-node/enable", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "node not found", http.StatusInternalServerError)
	})
	mux.HandleFunc("/admin/v1/nodes/bad-node/disable", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "node not found", http.StatusInternalServerError)
	})
	return httptest.NewServer(mux)
}

func twoNodeEntries() []status.AdminNodesEntry {
	return []status.AdminNodesEntry{
		{
			Node: status.AdminNodesNodeInfo{
				ID:            "node1",
				Host:          "inference",
				InferencePort: 5000,
				PoCPort:       8080,
				MaxConcurrent: 500,
				Models: map[string]status.AdminModelConfig{
					"Qwen/Qwen3-4B-Instruct-2507": {
						Args: []string{"--tensor-parallel-size", "1", "--gpu-memory-utilization", "0.90"},
					},
				},
				Hardware: []status.AdminHardware{
					{Type: "NVIDIA A10", Count: 1},
				},
			},
			State: status.AdminNodesState{
				IntendedStatus:    "INFERENCE",
				CurrentStatus:     "INFERENCE",
				PoCIntendedStatus: "IDLE",
				PoCCurrentStatus:  "IDLE",
				StatusTimestamp:   "2026-02-05T21:09:29.163752896Z",
				AdminState: struct {
					Enabled bool `json:"enabled"`
					Epoch   int  `json:"epoch"`
				}{Enabled: true, Epoch: 67},
				EpochMLNodes: map[string]status.EpochMLNodeInfo{
					"Qwen/Qwen3-4B-Instruct-2507": {
						NodeID:             "node1",
						PoCWeight:          8880,
						TimeslotAllocation: []bool{true, true},
					},
				},
			},
		},
		{
			Node: status.AdminNodesNodeInfo{
				ID:            "node2",
				Host:          "inference2",
				InferencePort: 5000,
				PoCPort:       8080,
				MaxConcurrent: 800,
				Models: map[string]status.AdminModelConfig{
					"Qwen/Qwen3-235B-A22B-Instruct-2507-FP8": {
						Args: []string{"--tensor-parallel-size", "8", "--gpu-memory-utilization", "0.90"},
					},
				},
				Hardware: []status.AdminHardware{
					{Type: "NVIDIA H100 80GB HBM3", Count: 8},
				},
			},
			State: status.AdminNodesState{
				IntendedStatus: "INFERENCE",
				CurrentStatus:  "FAILED",
				FailureReason:  "OOM during model load",
				AdminState: struct {
					Enabled bool `json:"enabled"`
					Epoch   int  `json:"epoch"`
				}{Enabled: false},
			},
		},
	}
}

func TestFetchAdminNodes(t *testing.T) {
	entries := twoNodeEntries()
	ts := newTestAdminServer(t, entries)
	defer ts.Close()

	got, err := fetchAdminNodes(ts.URL)
	if err != nil {
		t.Fatalf("fetchAdminNodes() error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}

	if got[0].Node.ID != testNodeID1 {
		t.Errorf("entry[0].Node.ID = %q, want %q", got[0].Node.ID, testNodeID1)
	}
	if got[1].Node.ID != testNodeID2 {
		t.Errorf("entry[1].Node.ID = %q, want %q", got[1].Node.ID, testNodeID2)
	}
	if got[0].State.CurrentStatus != testStatusInfer {
		t.Errorf("entry[0] status = %q, want %s", got[0].State.CurrentStatus, testStatusInfer)
	}
	if got[1].State.FailureReason != "OOM during model load" {
		t.Errorf("entry[1] failure = %q, want OOM message", got[1].State.FailureReason)
	}

	checkFetchAdminNodesExtended(t, got)
}

func checkFetchAdminNodesExtended(t *testing.T, got []status.AdminNodesEntry) {
	t.Helper()

	if got[0].State.IntendedStatus != testStatusInfer {
		t.Errorf("entry[0] intended = %q, want %s", got[0].State.IntendedStatus, testStatusInfer)
	}
	if got[0].State.AdminState.Epoch != 67 {
		t.Errorf("entry[0] enabled epoch = %d, want 67", got[0].State.AdminState.Epoch)
	}
	info, ok := got[0].State.EpochMLNodes["Qwen/Qwen3-4B-Instruct-2507"]
	if !ok {
		t.Fatal("entry[0] missing epoch_ml_nodes for model")
	}
	if info.PoCWeight != 8880 {
		t.Errorf("entry[0] poc_weight = %d, want 8880", info.PoCWeight)
	}
	if len(info.TimeslotAllocation) != 2 || !info.TimeslotAllocation[0] || !info.TimeslotAllocation[1] {
		t.Errorf("entry[0] timeslot = %v, want [true, true]", info.TimeslotAllocation)
	}

	// Status mismatch on node2 (intended=INFERENCE, current=FAILED)
	if got[1].State.IntendedStatus != testStatusInfer || got[1].State.CurrentStatus != testStatusFailed {
		t.Errorf("entry[1] status mismatch not detected: intended=%q current=%q",
			got[1].State.IntendedStatus, got[1].State.CurrentStatus)
	}
}

func TestFetchAdminNodes_EmptyResponse(t *testing.T) {
	ts := newTestAdminServer(t, []status.AdminNodesEntry{})
	defer ts.Close()

	got, err := fetchAdminNodes(ts.URL)
	if err != nil {
		t.Fatalf("fetchAdminNodes() error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0", len(got))
	}
}

func TestFetchAdminNodes_APIDown(t *testing.T) {
	_, err := fetchAdminNodes("http://127.0.0.1:1")
	if err == nil {
		t.Fatal("fetchAdminNodes() should error when API is down")
	}
}

func TestFetchAdminNodes_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	_, err := fetchAdminNodes(ts.URL)
	if err == nil {
		t.Fatal("fetchAdminNodes() should error on 500")
	}
}

func TestPostAdminAction_Enable(t *testing.T) {
	ts := newTestAdminServer(t, nil)
	defer ts.Close()

	err := postAdminAction(ts.URL, testNodeID1, "enable")
	if err != nil {
		t.Fatalf("postAdminAction(enable) error: %v", err)
	}
}

func TestPostAdminAction_Disable(t *testing.T) {
	ts := newTestAdminServer(t, nil)
	defer ts.Close()

	err := postAdminAction(ts.URL, testNodeID1, "disable")
	if err != nil {
		t.Fatalf("postAdminAction(disable) error: %v", err)
	}
}

func TestPostAdminAction_Error(t *testing.T) {
	ts := newTestAdminServer(t, nil)
	defer ts.Close()

	err := postAdminAction(ts.URL, "bad-node", "enable")
	if err == nil {
		t.Fatal("postAdminAction() should error on 500")
	}
}

func TestPostAdminAction_APIDown(t *testing.T) {
	err := postAdminAction("http://127.0.0.1:1", testNodeID1, "enable")
	if err == nil {
		t.Fatal("postAdminAction() should error when API is down")
	}
}

func TestFirstModelName(t *testing.T) {
	models := map[string]status.AdminModelConfig{
		"Qwen/Qwen3-4B": {Args: []string{"--tp", "1"}},
	}
	name := firstModelName(models)
	if name != "Qwen/Qwen3-4B" {
		t.Errorf("firstModelName() = %q, want Qwen/Qwen3-4B", name)
	}
}

func TestFirstModelName_Empty(t *testing.T) {
	name := firstModelName(map[string]status.AdminModelConfig{})
	if name != testModelNameNone {
		t.Errorf("firstModelName() = %q, want %s", name, testModelNameNone)
	}
}

func TestRunMLNodeList(t *testing.T) {
	entries := twoNodeEntries()
	ts := newTestAdminServer(t, entries)
	defer ts.Close()

	// Set the admin URL to our test server
	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	// Should not error
	err := runMLNodeList(nil, nil)
	if err != nil {
		t.Fatalf("runMLNodeList() error: %v", err)
	}
}

func TestRunMLNodeList_Empty(t *testing.T) {
	ts := newTestAdminServer(t, []status.AdminNodesEntry{})
	defer ts.Close()

	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	err := runMLNodeList(nil, nil)
	if err != nil {
		t.Fatalf("runMLNodeList() error: %v", err)
	}
}

func TestRunMLNodeStatus(t *testing.T) {
	entries := twoNodeEntries()
	ts := newTestAdminServer(t, entries)
	defer ts.Close()

	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	// Default (node1)
	err := runMLNodeStatus(nil, nil)
	if err != nil {
		t.Fatalf("runMLNodeStatus() error: %v", err)
	}

	// Explicit node2
	err = runMLNodeStatus(nil, []string{"node2"})
	if err != nil {
		t.Fatalf("runMLNodeStatus(node2) error: %v", err)
	}
}

func TestRunMLNodeStatus_NotFound(t *testing.T) {
	entries := twoNodeEntries()
	ts := newTestAdminServer(t, entries)
	defer ts.Close()

	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	err := runMLNodeStatus(nil, []string{"nonexistent"})
	if err == nil {
		t.Fatal("runMLNodeStatus() should error for nonexistent node")
	}
}

func TestRunMLNodeEnable(t *testing.T) {
	ts := newTestAdminServer(t, nil)
	defer ts.Close()

	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	err := runMLNodeEnable(nil, nil) // defaults to node1
	if err != nil {
		t.Fatalf("runMLNodeEnable() error: %v", err)
	}
}

func TestRunMLNodeDisable(t *testing.T) {
	ts := newTestAdminServer(t, nil)
	defer ts.Close()

	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	err := runMLNodeDisable(nil, nil) // defaults to node1
	if err != nil {
		t.Fatalf("runMLNodeDisable() error: %v", err)
	}
}

func TestRunMLNodeEnable_Error(t *testing.T) {
	ts := newTestAdminServer(t, nil)
	defer ts.Close()

	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	err := runMLNodeEnable(nil, []string{"bad-node"})
	if err == nil {
		t.Fatal("runMLNodeEnable() should error on bad node")
	}
}

func TestRunMLNodeDisable_Error(t *testing.T) {
	ts := newTestAdminServer(t, nil)
	defer ts.Close()

	oldURL := adminURL
	adminURL = ts.URL
	defer func() { adminURL = oldURL }()

	err := runMLNodeDisable(nil, []string{"bad-node"})
	if err == nil {
		t.Fatal("runMLNodeDisable() should error on bad node")
	}
}
