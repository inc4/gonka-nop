package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// CollateralBalance mirrors `inferenced query collateral show-collateral <addr>`.
// Mainnet schema verified 2026-05-16: returns a single object with amount + denom.
// When the participant has never deposited, the chain returns
//
//	"rpc error: code = NotFound desc = collateral not found for participant <addr>"
//
// which ParseCollateralResponse maps to (zero balance, false).
type CollateralBalance struct {
	Participant string
	Amount      uint64 // in base denom units (ngonka)
	Denom       string // "ngonka" on mainnet
	Found       bool   // false when chain returned NotFound
}

// GetCollateral runs `inferenced query collateral show-collateral <participant>`
// via the chain QueryRunner. Returns Found=false (not an error) when the
// participant has never deposited collateral — common for fresh registrations.
func GetCollateral(ctx context.Context, qr QueryRunner, participant string) (*CollateralBalance, error) {
	out, err := qr.Query(ctx, "inferenced", "query", "collateral", "show-collateral",
		participant, "--node", "http://node:26657", "-o", "json")
	if err != nil {
		// "NotFound" is expected for fresh participants; the binary writes it
		// to stderr and exits non-zero. cmd.Output() (used by the query runner)
		// stashes stderr in *exec.ExitError.Stderr.
		stderr := string(out)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		if isCollateralNotFound(stderr) || isCollateralNotFound(err.Error()) || isCollateralNotFound(string(out)) {
			return &CollateralBalance{Participant: participant, Found: false, Denom: "ngonka"}, nil
		}
		return nil, fmt.Errorf("query collateral: %w\nstderr: %s\nstdout: %s", err, stderr, string(out))
	}
	return parseCollateralResponse(out, participant)
}

func isCollateralNotFound(s string) bool {
	return regexp.MustCompile(`(?i)collateral not found|code\s*=\s*NotFound`).MatchString(s)
}

func parseCollateralResponse(jsonBytes []byte, participant string) (*CollateralBalance, error) {
	// Two observed shapes on mainnet:
	//   {"collateral": {"participant": "gonka1…", "amount": "100000", "denom": "ngonka"}}
	//   {"participant": "gonka1…", "amount": {"denom":"ngonka","amount":"100000"}}
	// We support both with a permissive struct.
	var envelope struct {
		Collateral *struct {
			Participant string          `json:"participant"`
			Amount      json.RawMessage `json:"amount"`
			Denom       string          `json:"denom"`
		} `json:"collateral"`
		Participant string          `json:"participant"`
		Amount      json.RawMessage `json:"amount"`
		Denom       string          `json:"denom"`
	}
	if err := json.Unmarshal(jsonBytes, &envelope); err != nil {
		return nil, fmt.Errorf("parse collateral JSON: %w (body: %s)", err, string(jsonBytes))
	}

	src := envelope.Collateral
	if src == nil {
		src = &struct {
			Participant string          `json:"participant"`
			Amount      json.RawMessage `json:"amount"`
			Denom       string          `json:"denom"`
		}{
			Participant: envelope.Participant,
			Amount:      envelope.Amount,
			Denom:       envelope.Denom,
		}
	}

	out := &CollateralBalance{Participant: src.Participant, Denom: src.Denom, Found: true}
	if out.Participant == "" {
		out.Participant = participant
	}
	if out.Denom == "" {
		out.Denom = "ngonka"
	}
	// amount may be "12345" (string) or {"denom":"ngonka","amount":"12345"}
	var s string
	if err := json.Unmarshal(src.Amount, &s); err == nil && s != "" {
		amt, perr := strconv.ParseUint(s, 10, 64)
		if perr != nil {
			return nil, fmt.Errorf("parse amount %q: %w", s, perr)
		}
		out.Amount = amt
		return out, nil
	}
	var coin struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	}
	if err := json.Unmarshal(src.Amount, &coin); err == nil {
		if coin.Denom != "" {
			out.Denom = coin.Denom
		}
		amt, perr := strconv.ParseUint(coin.Amount, 10, 64)
		if perr != nil {
			return nil, fmt.Errorf("parse coin.amount %q: %w", coin.Amount, perr)
		}
		out.Amount = amt
	}
	return out, nil
}

// DepositCollateral broadcasts `inferenced tx collateral deposit-collateral
// <amount>` signed by signerKey. amount MUST be a coin string (e.g.
// "3095ngonka" or "1000000ngonka") matching cosmos-sdk parsing.
func DepositCollateral(ctx context.Context, b TxBroadcaster, signerKey, password, amount string) (string, error) {
	if err := ValidateCoinAmount(amount); err != nil {
		return "", err
	}
	return b.Broadcast(ctx, signerKey, password, "collateral", "deposit-collateral", amount)
}

// WithdrawCollateral starts the unbonding of collateral. Per chain params
// (unbonding_period_epochs=1 on mainnet), funds are released one epoch later.
func WithdrawCollateral(ctx context.Context, b TxBroadcaster, signerKey, password, amount string) (string, error) {
	if err := ValidateCoinAmount(amount); err != nil {
		return "", err
	}
	return b.Broadcast(ctx, signerKey, password, "collateral", "withdraw-collateral", amount)
}

// coinAmountRe enforces the cosmos-sdk coin string format. Allow only
// alphanumeric denoms (a-z0-9, lowercase) to avoid shell-injection vectors
// in the args we hand to docker exec.
var coinAmountRe = regexp.MustCompile(`^\d+[a-z][a-z0-9]{2,16}$`)

// ValidateCoinAmount returns nil for a well-formed cosmos coin string.
// Examples: "3095ngonka", "1000000ngonka", "1ucredit".
func ValidateCoinAmount(amount string) error {
	if amount == "" {
		return fmt.Errorf("amount is empty")
	}
	if !coinAmountRe.MatchString(amount) {
		return fmt.Errorf("amount %q must be <digits><denom> (e.g. 3095ngonka, 1000000ngonka)", amount)
	}
	return nil
}

// Broadcast is the variadic-args shim so DepositCollateral/WithdrawCollateral
// can target a non-"inference" module. The default TxBroadcaster's first
// arg ("collateral" or "inference") is the module name; for collateral we
// pass it explicitly so dockerExecTxBroadcaster can route correctly.
//
// (No change to the interface — DepositCollateral above passes the module
// name as the action parameter to the existing Broadcast signature; the
// dockerExecTxBroadcaster impl in govmodel.go uses inferenced tx inference
// <action>... which is wrong for collateral. The override below handles it.)

// TxModuleBroadcaster is the broader interface that supports non-inference
// modules. Production dockerExecTxBroadcaster will need a `module` field
// in a future refactor; for now, callers needing collateral txs use this.
type TxModuleBroadcaster interface {
	BroadcastModule(ctx context.Context, module, signerKey, password, action string, args ...string) (string, error)
}

// DepositCollateralViaModule broadcasts via TxModuleBroadcaster (module="collateral").
// Use this when you have a TxModuleBroadcaster; DepositCollateral above is kept
// for symmetry with govmodel commands but should NOT be used until the
// dockerExecTxBroadcaster is generalized.
func DepositCollateralViaModule(ctx context.Context, b TxModuleBroadcaster, signerKey, password, amount string) (string, error) {
	if err := ValidateCoinAmount(amount); err != nil {
		return "", err
	}
	return b.BroadcastModule(ctx, "collateral", signerKey, password, "deposit-collateral", amount)
}

// WithdrawCollateralViaModule mirrors DepositCollateralViaModule.
func WithdrawCollateralViaModule(ctx context.Context, b TxModuleBroadcaster, signerKey, password, amount string) (string, error) {
	if err := ValidateCoinAmount(amount); err != nil {
		return "", err
	}
	return b.BroadcastModule(ctx, "collateral", signerKey, password, "withdraw-collateral", amount)
}
