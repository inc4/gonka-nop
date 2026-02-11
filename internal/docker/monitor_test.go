package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const statusInference = "INFERENCE"

func TestFetchSyncStatus_Synced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"result": map[string]interface{}{
				"sync_info": map[string]interface{}{
					"latest_block_height": "12345",
					"catching_up":         false,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	status, err := FetchSyncStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchSyncStatus() error: %v", err)
	}
	if status.LatestBlockHeight != 12345 {
		t.Errorf("LatestBlockHeight = %d, want 12345", status.LatestBlockHeight)
	}
	if status.CatchingUp {
		t.Error("CatchingUp should be false")
	}
}

func TestFetchSyncStatus_CatchingUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"result": map[string]interface{}{
				"sync_info": map[string]interface{}{
					"latest_block_height": "500",
					"catching_up":         true,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	status, err := FetchSyncStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchSyncStatus() error: %v", err)
	}
	if !status.CatchingUp {
		t.Error("CatchingUp should be true")
	}
}

func TestFetchSyncStatus_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FetchSyncStatus(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestFetchSyncStatus_Unreachable(t *testing.T) {
	_, err := FetchSyncStatus(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}

func TestWaitForSync(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		catching := n < 3 // First two calls: catching up. Third: synced.
		resp := map[string]interface{}{
			"result": map[string]interface{}{
				"sync_info": map[string]interface{}{
					"latest_block_height": "12345",
					"catching_up":         catching,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var progressCalls int
	err := WaitForSync(ctx, srv.URL, 50*time.Millisecond, func(_ *SyncStatus) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("WaitForSync() error: %v", err)
	}
	if progressCalls < 2 {
		t.Errorf("expected at least 2 progress calls, got %d", progressCalls)
	}
}

func TestWaitForSync_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"result": map[string]interface{}{
				"sync_info": map[string]interface{}{
					"latest_block_height": "100",
					"catching_up":         true,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := WaitForSync(ctx, srv.URL, 50*time.Millisecond, nil)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestFetchMLNodeStatus_Inference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]interface{}{
			{
				"node": map[string]interface{}{
					"models": map[string]interface{}{
						"Qwen/QwQ-32B": map[string]interface{}{},
					},
				},
				"state": map[string]interface{}{
					"current_status": statusInference,
					"failure_reason": "",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	status, err := FetchMLNodeStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMLNodeStatus() error: %v", err)
	}
	if status.CurrentStatus != statusInference {
		t.Errorf("CurrentStatus = %q, want %q", status.CurrentStatus, statusInference)
	}
	if !status.ModelLoaded {
		t.Error("ModelLoaded should be true for INFERENCE status")
	}
}

func TestFetchMLNodeStatus_Loading(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]interface{}{
			{
				"node":  map[string]interface{}{},
				"state": map[string]interface{}{"current_status": "LOADING"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	status, err := FetchMLNodeStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMLNodeStatus() error: %v", err)
	}
	if status.ModelLoaded {
		t.Error("ModelLoaded should be false for LOADING status")
	}
}

func TestFetchMLNodeStatus_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]interface{}{
			{
				"node": map[string]interface{}{},
				"state": map[string]interface{}{
					"current_status": "FAILED",
					"failure_reason": "OOM killed",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	status, err := FetchMLNodeStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMLNodeStatus() error: %v", err)
	}
	if status.CurrentStatus != "FAILED" {
		t.Errorf("CurrentStatus = %q, want %q", status.CurrentStatus, "FAILED")
	}
	if status.FailureReason != "OOM killed" {
		t.Errorf("FailureReason = %q, want %q", status.FailureReason, "OOM killed")
	}
}

func TestFetchMLNodeStatus_NoNodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	status, err := FetchMLNodeStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMLNodeStatus() error: %v", err)
	}
	if status.CurrentStatus != "NO_NODES" {
		t.Errorf("CurrentStatus = %q, want %q", status.CurrentStatus, "NO_NODES")
	}
}

func TestFetchMLNodeStatus_POC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]interface{}{
			{
				"node":  map[string]interface{}{},
				"state": map[string]interface{}{"current_status": "POC"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	status, err := FetchMLNodeStatus(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMLNodeStatus() error: %v", err)
	}
	if !status.ModelLoaded {
		t.Error("ModelLoaded should be true for POC status")
	}
}

func TestWaitForModelLoad(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		status := "LOADING"
		if n >= 3 {
			status = "INFERENCE"
		}
		resp := []map[string]interface{}{
			{
				"node":  map[string]interface{}{},
				"state": map[string]interface{}{"current_status": status},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForModelLoad(ctx, srv.URL, 50*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("WaitForModelLoad() error: %v", err)
	}
}

func TestWaitForModelLoad_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]interface{}{
			{
				"node": map[string]interface{}{},
				"state": map[string]interface{}{
					"current_status": "FAILED",
					"failure_reason": "CUDA OOM",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := WaitForModelLoad(ctx, srv.URL, 50*time.Millisecond, nil)
	if err == nil {
		t.Error("expected error for FAILED status, got nil")
	}
}

func TestFetchHealthReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"checks": []map[string]interface{}{
				{"id": "block_sync", "status": "PASS", "message": "Block synced"},
				{"id": "mlnode_health", "status": "PASS", "message": "ML node healthy"},
				{"id": "consensus_key_match", "status": "FAIL", "message": "Not registered"},
			},
			"summary": map[string]interface{}{
				"total_checks":  3,
				"passed_checks": 2,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	checks, err := FetchHealthReport(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchHealthReport() error: %v", err)
	}
	if len(checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(checks))
	}

	if checks[0].ID != "block_sync" || checks[0].Status != "PASS" {
		t.Errorf("check[0]: got %+v", checks[0])
	}
	if checks[2].Status != "FAIL" {
		t.Errorf("check[2] status = %q, want %q", checks[2].Status, "FAIL")
	}
}

func TestFetchHealthReport_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := FetchHealthReport(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error for 503 response, got nil")
	}
}

func TestFetchHealthReport_Unreachable(t *testing.T) {
	_, err := FetchHealthReport(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}
