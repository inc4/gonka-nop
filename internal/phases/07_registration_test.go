package phases

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
)

const testStatusFail = "FAIL"

// FetchConsensusKey and RunComposeExec tests live in internal/chain/ as of M11 Slice 1.

func TestWaitForRegistration_ImmediatePass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"checks": []map[string]interface{}{
				{"id": "consensus_key_match", "status": statusPass, "message": "Registered"},
				{"id": "permissions_granted", "status": statusPass, "message": "Granted"},
			},
			"summary": map[string]interface{}{"total_checks": 2, "passed_checks": 2},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	err := WaitForRegistration(context.Background(), srv.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForRegistration_ProgressivePass(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := atomic.AddInt32(&callCount, 1)

		regStatus := testStatusFail
		permStatus := testStatusFail
		if count >= 2 {
			regStatus = statusPass
		}
		if count >= 3 {
			permStatus = statusPass
		}

		resp := map[string]interface{}{
			"checks": []map[string]interface{}{
				{"id": "consensus_key_match", "status": regStatus, "message": "check"},
				{"id": "permissions_granted", "status": permStatus, "message": "check"},
			},
			"summary": map[string]interface{}{"total_checks": 2, "passed_checks": 0},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Override the poll interval for faster testing
	err := waitForRegistrationWithInterval(context.Background(), srv.URL, 10*time.Second, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 calls, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestWaitForRegistration_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"checks": []map[string]interface{}{
				{"id": "consensus_key_match", "status": testStatusFail, "message": "Not registered"},
				{"id": "permissions_granted", "status": testStatusFail, "message": "Not granted"},
			},
			"summary": map[string]interface{}{"total_checks": 2, "passed_checks": 0},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	err := waitForRegistrationWithInterval(context.Background(), srv.URL, 200*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRegistrationPhase_ShouldRun(t *testing.T) {
	phase := NewRegistration()

	tests := []struct {
		name     string
		state    *config.State
		expected bool
	}{
		{
			name:     "not completed",
			state:    &config.State{CompletedPhases: []string{}},
			expected: true,
		},
		{
			name:     "already completed",
			state:    &config.State{CompletedPhases: []string{"Registration"}},
			expected: false,
		},
		{
			name:     "other phases completed",
			state:    &config.State{CompletedPhases: []string{"Deployment", "Key Management"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phase.ShouldRun(tt.state)
			if got != tt.expected {
				t.Errorf("ShouldRun() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRegistrationPhase_Name(t *testing.T) {
	phase := NewRegistration()
	if phase.Name() != "Registration" {
		t.Errorf("Name() = %q, want %q", phase.Name(), "Registration")
	}
}

func TestRegistrationPhase_Description(t *testing.T) {
	phase := NewRegistration()
	if phase.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

// waitForRegistrationWithInterval is a helper for testing with custom poll interval.
func waitForRegistrationWithInterval(ctx context.Context, adminURL string, timeout, interval time.Duration) error {
	regCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		checks, err := fetchRegistrationChecks(regCtx, adminURL)
		if err == nil {
			regOK := checks["consensus_key_match"] == statusPass
			permOK := checks["permissions_granted"] == statusPass
			if regOK && permOK {
				return nil
			}
		}

		select {
		case <-regCtx.Done():
			return regCtx.Err()
		case <-time.After(interval):
		}
	}
}
