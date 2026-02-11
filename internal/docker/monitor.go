package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SyncStatus holds blockchain sync state from Tendermint RPC.
type SyncStatus struct {
	LatestBlockHeight int64
	CatchingUp        bool
}

// MLNodeStatus holds ML node state from the admin API.
type MLNodeStatus struct {
	CurrentStatus string
	ModelLoaded   bool
	FailureReason string
}

// HealthCheck represents one check from /admin/v1/setup/report.
type HealthCheck struct {
	ID      string
	Status  string // "PASS" or "FAIL"
	Message string
}

// tendermintStatusResp maps the /status JSON response.
type tendermintStatusResp struct {
	Result struct {
		SyncInfo struct {
			LatestBlockHeight string `json:"latest_block_height"`
			CatchingUp        bool   `json:"catching_up"`
		} `json:"sync_info"`
	} `json:"result"`
}

// FetchSyncStatus polls the Tendermint RPC /status endpoint.
func FetchSyncStatus(ctx context.Context, rpcURL string) (*SyncStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rpcURL+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch sync status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC returned status %d", resp.StatusCode)
	}

	var tmStatus tendermintStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&tmStatus); err != nil {
		return nil, fmt.Errorf("decode sync status: %w", err)
	}

	var height int64
	_, _ = fmt.Sscanf(tmStatus.Result.SyncInfo.LatestBlockHeight, "%d", &height)

	return &SyncStatus{
		LatestBlockHeight: height,
		CatchingUp:        tmStatus.Result.SyncInfo.CatchingUp,
	}, nil
}

// WaitForSync polls until catching_up=false or context is canceled.
// Calls progressFn on each poll with current status.
func WaitForSync(ctx context.Context, rpcURL string, interval time.Duration,
	progressFn func(status *SyncStatus)) error {

	for {
		status, err := FetchSyncStatus(ctx, rpcURL)
		if err == nil {
			if progressFn != nil {
				progressFn(status)
			}
			if !status.CatchingUp {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// adminNodesResp maps a single entry from /admin/v1/nodes.
type adminNodesResp struct {
	Node struct {
		Models map[string]interface{} `json:"models"`
	} `json:"node"`
	State struct {
		CurrentStatus string `json:"current_status"`
		FailureReason string `json:"failure_reason"`
	} `json:"state"`
}

// FetchMLNodeStatus polls the admin API /admin/v1/nodes for ML node readiness.
func FetchMLNodeStatus(ctx context.Context, adminURL string) (*MLNodeStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adminURL+"/admin/v1/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ML node status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("admin API returned status %d", resp.StatusCode)
	}

	var entries []adminNodesResp
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode ML node status: %w", err)
	}

	if len(entries) == 0 {
		return &MLNodeStatus{CurrentStatus: "NO_NODES"}, nil
	}

	entry := entries[0]
	loaded := entry.State.CurrentStatus == "INFERENCE" || entry.State.CurrentStatus == "POC"

	return &MLNodeStatus{
		CurrentStatus: entry.State.CurrentStatus,
		ModelLoaded:   loaded,
		FailureReason: entry.State.FailureReason,
	}, nil
}

// WaitForModelLoad polls until model is loaded (status=INFERENCE or POC)
// or context is canceled.
func WaitForModelLoad(ctx context.Context, adminURL string, interval time.Duration,
	progressFn func(status *MLNodeStatus)) error {

	for {
		status, err := FetchMLNodeStatus(ctx, adminURL)
		if err == nil {
			if progressFn != nil {
				progressFn(status)
			}
			if status.ModelLoaded {
				return nil
			}
			if status.CurrentStatus == "FAILED" {
				return fmt.Errorf("ML node failed: %s", status.FailureReason)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// setupReportResp maps the /admin/v1/setup/report response.
type setupReportResp struct {
	Checks []struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"checks"`
	Summary struct {
		TotalChecks  int `json:"total_checks"`
		PassedChecks int `json:"passed_checks"`
	} `json:"summary"`
}

// FetchHealthReport fetches /admin/v1/setup/report and returns parsed checks.
func FetchHealthReport(ctx context.Context, adminURL string) ([]HealthCheck, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch health report: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("setup/report returned status %d", resp.StatusCode)
	}

	var report setupReportResp
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("decode health report: %w", err)
	}

	checks := make([]HealthCheck, len(report.Checks))
	for i, c := range report.Checks {
		checks[i] = HealthCheck{
			ID:      c.ID,
			Status:  c.Status,
			Message: c.Message,
		}
	}
	return checks, nil
}
