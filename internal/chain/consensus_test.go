package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

const (
	testStatusPass = "PASS"
	testStatusFail = "FAIL"
)

func TestFetchConsensusKey_FromConsensusKeyMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"checks": []map[string]interface{}{
				{
					"id":      "consensus_key_match",
					"status":  testStatusPass,
					"message": "Consensus key matches",
					"details": map[string]interface{}{
						"validator_key": "abc123ed25519key",
					},
				},
			},
			"summary": map[string]interface{}{
				"total_checks":  1,
				"passed_checks": 1,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	key, err := FetchConsensusKey(context.Background(), srv.URL, &config.State{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "abc123ed25519key" {
		t.Errorf("got key %q, want %q", key, "abc123ed25519key")
	}
}

func TestFetchConsensusKey_FallbackToValidatorInSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"checks": []map[string]interface{}{
				{
					"id":      "consensus_key_match",
					"status":  testStatusFail,
					"message": "Not registered",
					// No details
				},
				{
					"id":      "validator_in_set",
					"status":  testStatusPass,
					"message": "Validator is active",
					"details": map[string]interface{}{
						"consensus_pubkey": "fallbackPubKey456",
					},
				},
			},
			"summary": map[string]interface{}{
				"total_checks":  2,
				"passed_checks": 1,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	key, err := FetchConsensusKey(context.Background(), srv.URL, &config.State{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "fallbackPubKey456" {
		t.Errorf("got key %q, want %q", key, "fallbackPubKey456")
	}
}

func TestFetchConsensusKey_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"checks": []map[string]interface{}{
				{
					"id":      "block_sync",
					"status":  testStatusPass,
					"message": "Block synced",
				},
			},
			"summary": map[string]interface{}{
				"total_checks":  1,
				"passed_checks": 1,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := FetchConsensusKey(context.Background(), srv.URL, &config.State{})
	if err == nil {
		t.Fatal("expected error when consensus key not found")
	}
}

func TestFetchConsensusKey_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FetchConsensusKey(context.Background(), srv.URL, &config.State{})
	if err == nil {
		t.Fatal("expected error on server error")
	}
}
