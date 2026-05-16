package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
)

// FetchConsensusKey fetches the consensus (validator) key.
// Tries three sources in order:
//  1. setup/report consensus_key_match details (if API includes it)
//  2. setup/report validator_in_set details (if already a validator)
//  3. Tendermint RPC /status via docker exec (most reliable for fresh nodes)
func FetchConsensusKey(ctx context.Context, adminURL string, state *config.State) (string, error) {
	// Try Admin API first (no docker exec needed)
	key, err := fetchConsensusKeyFromReport(ctx, adminURL)
	if err == nil {
		return key, nil
	}

	// Fall back to Tendermint RPC via docker exec into node container
	key, err = fetchConsensusKeyFromRPC(ctx, state)
	if err == nil {
		return key, nil
	}

	return "", fmt.Errorf("consensus key not available from setup/report or Tendermint RPC: %w", err)
}

// fetchConsensusKeyFromReport tries to extract the key from setup/report checks.
func fetchConsensusKeyFromReport(ctx context.Context, adminURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adminURL+"/admin/v1/setup/report", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch setup/report: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("setup/report returned status %d", resp.StatusCode)
	}

	var report struct {
		Checks []struct {
			ID      string          `json:"id"`
			Status  string          `json:"status"`
			Details json.RawMessage `json:"details"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return "", fmt.Errorf("decode setup/report: %w", err)
	}

	// Try consensus_key_match first (may have validator_key in details)
	for _, check := range report.Checks {
		if check.ID == "consensus_key_match" && check.Details != nil {
			var d struct {
				ValidatorKey string `json:"validator_key"`
			}
			if json.Unmarshal(check.Details, &d) == nil && d.ValidatorKey != "" {
				return d.ValidatorKey, nil
			}
		}
	}

	// Fall back to validator_in_set (has consensus_pubkey when active)
	for _, check := range report.Checks {
		if check.ID == "validator_in_set" && check.Details != nil {
			var d struct {
				ConsensusPubKey string `json:"consensus_pubkey"`
			}
			if json.Unmarshal(check.Details, &d) == nil && d.ConsensusPubKey != "" {
				return d.ConsensusPubKey, nil
			}
		}
	}

	return "", fmt.Errorf("consensus key not found in setup/report checks")
}

// fetchConsensusKeyFromRPC gets the validator key from Tendermint RPC via docker exec.
// This is the most reliable method — works even on fresh unregistered nodes.
// Parses: /status -> result.validator_info.pub_key.value
func fetchConsensusKeyFromRPC(ctx context.Context, state *config.State) (string, error) {
	// Use wget (available in node container) since curl may not be present
	shellCmd := `wget -qO- http://localhost:26657/status 2>/dev/null`

	execArgs := []string{"exec", "node", "sh", "-c", shellCmd}

	var name string
	var args []string
	if state.UseSudo {
		name = docker.CmdSudo
		args = append([]string{"-E", docker.CmdDocker}, execArgs...)
	} else {
		name = docker.CmdDocker
		args = execArgs
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...) // #nosec G204
	cmd.Dir = state.OutputDir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker exec node RPC status: %w", err)
	}

	var rpcStatus struct {
		Result struct {
			ValidatorInfo struct {
				PubKey struct {
					Value string `json:"value"`
				} `json:"pub_key"`
			} `json:"validator_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &rpcStatus); err != nil {
		return "", fmt.Errorf("parse RPC status: %w", err)
	}

	key := rpcStatus.Result.ValidatorInfo.PubKey.Value
	if key == "" {
		return "", fmt.Errorf("validator pub_key.value is empty in RPC status")
	}
	return key, nil
}
