// M12 Slice 8a — ml-ops permissions repair flow.
//
// `DiffPermissions` parses /admin/v1/setup/report → identifies missing
// permissions; `GrantMissing` broadcasts a single grant-ml-ops-permissions
// tx with EXACTLY the missing entries (never with the full list, never
// calling register-new-participant). Balance pre-check via the
// BalanceQuerier interface aborts before tx construction if fees can't
// be covered.
//
// Security-auditor pre-review APPROVED-WITH-CONDITIONS 2026-05-17:
//   - stdin-pipe password ONLY (TxBroadcaster contract from M11)
//   - balance check is an interface for mockable tests
//   - password is redacted from all error wraps via redactPassword
//   - no echo of full inferenced argv in any UI line
//   - register-new-participant guarded by test assertion (not runtime panic)
package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/inc4/gonka-nop/internal/status"
)

// grantAction is the single chain action this package emits. Pinned as a
// constant so tests can assert no other action ever flows to broadcaster.
const grantAction = "grant-ml-ops-permissions"

// Permission identifies one ml-ops chain message that the warm key needs
// authority to broadcast on behalf of the cold key.
type Permission struct {
	Action string // full action path, e.g. "/inference.inference.MsgSubmitPocValidation"
}

// BalanceQuerier returns the warm key's spendable balance in ngonka.
// Implemented by chainBalanceQuerier (production) and mock in tests.
type BalanceQuerier interface {
	GetBalance(ctx context.Context, address string) (amount uint64, denom string, err error)
}

// DiffPermissions extracts the set of ml-ops permissions still missing
// for the warm key from a setup/report response. Returns empty slice
// when all granted. Returns nil + error when the permissions check is
// absent or malformed.
func DiffPermissions(report *status.SetupReport) ([]Permission, error) {
	if report == nil {
		return nil, fmt.Errorf("setup report is nil")
	}
	for _, c := range report.Checks {
		if c.ID != "permissions_granted" {
			continue
		}
		if c.Status == "PASS" {
			return nil, nil // all granted
		}
		// Parse details for the missing list
		var details struct {
			Missing []string `json:"missing_permissions"`
		}
		if c.Details == nil {
			return nil, fmt.Errorf("permissions_granted check missing details")
		}
		if err := json.Unmarshal(c.Details, &details); err != nil {
			return nil, fmt.Errorf("parse permissions details: %w", err)
		}
		out := make([]Permission, 0, len(details.Missing))
		for _, a := range details.Missing {
			out = append(out, Permission{Action: a})
		}
		return out, nil
	}
	return nil, fmt.Errorf("permissions_granted check not found in setup report")
}

// GrantMissing broadcasts a single `grant-ml-ops-permissions` tx
// containing exactly the missing permissions, signed by signerKey
// (cold key — the grantor) and granting to grantee (warm key address).
//
// Pre-checks signerAddress balance >= estimatedFee. Returns wrapped
// error if balance insufficient; tx is NOT constructed. All error
// messages are scrubbed of the password value.
//
// `missing` MUST be non-empty (caller checks via DiffPermissions).
// `action` is hard-coded to grantAction — this function never broadcasts
// any other tx type. Test TestGrantMissing_NeverConstructsForbiddenAction
// enforces this invariant across the entire repair flow.
func GrantMissing(
	ctx context.Context,
	broadcaster TxBroadcaster,
	balancer BalanceQuerier,
	signerKey, signerAddress, password, grantee string,
	missing []Permission,
	estimatedFee uint64,
) (string, error) {
	if len(missing) == 0 {
		return "", fmt.Errorf("GrantMissing called with empty missing slice (caller bug)")
	}
	if signerKey == "" || signerAddress == "" || grantee == "" {
		return "", fmt.Errorf("GrantMissing: signerKey/signerAddress/grantee required")
	}
	if password == "" {
		return "", fmt.Errorf("GrantMissing: password required")
	}

	have, denom, err := balancer.GetBalance(ctx, signerAddress)
	if err != nil {
		return "", fmt.Errorf("balance check failed for %s: %w", signerAddress, err)
	}
	if have < estimatedFee {
		return "", fmt.Errorf("insufficient balance for grant fee: have %d%s, need %d%s on signer %s",
			have, denom, estimatedFee, denom, signerAddress)
	}

	// Build args: <grantee> <perm1> <perm2> ...
	args := make([]string, 0, 1+len(missing))
	args = append(args, grantee)
	for _, p := range missing {
		args = append(args, p.Action)
	}

	out, err := broadcaster.Broadcast(ctx, signerKey, password, grantAction, args...)
	if err != nil {
		return "", fmt.Errorf("grant-ml-ops-permissions tx failed: %s: %w", redactPassword(out, password), redactPasswordErr(err, password))
	}
	return strings.TrimSpace(out), nil
}

// redactPassword replaces every occurrence of password in s with
// [REDACTED]. Used in error wraps so the canary substring never leaks
// even if the chain combined-output echoes it.
func redactPassword(s, password string) string {
	if password == "" {
		return s
	}
	return strings.ReplaceAll(s, password, "[REDACTED]")
}

// redactPasswordErr wraps an error whose Error() may contain the password.
// Wraps so errors.Is/As still work for the underlying error type but the
// stringified message is scrubbed.
type redactedError struct {
	inner    error
	password string
}

func (r *redactedError) Error() string {
	return redactPassword(r.inner.Error(), r.password)
}

func (r *redactedError) Unwrap() error { return r.inner }

func redactPasswordErr(err error, password string) error {
	if err == nil {
		return nil
	}
	return &redactedError{inner: err, password: password}
}
