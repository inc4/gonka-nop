package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// NewBalanceQuerier returns a production BalanceQuerier that shells out
// via `inferenced query bank balances <addr>` inside the api container,
// reusing the existing dockerExecQueryRunner.
func NewBalanceQuerier(qr QueryRunner) BalanceQuerier {
	return &chainBalanceQuerier{qr: qr}
}

type chainBalanceQuerier struct {
	qr QueryRunner
}

// GetBalance returns the spendable balance for address (first non-zero
// coin entry from the chain response). Returns 0/empty if address has
// no on-chain account (treated as not-an-error by callers; GrantMissing
// will then surface "insufficient balance for grant fee").
func (c *chainBalanceQuerier) GetBalance(ctx context.Context, address string) (uint64, string, error) {
	// dockerExecQueryRunner prefixes "docker exec api"; we add the binary name.
	out, err := c.qr.Query(ctx, "inferenced", "query", "bank", "balances", address,
		"--node", "http://node:26657", "-o", "json")
	if err != nil {
		return 0, "", fmt.Errorf("query bank balances: %w", err)
	}
	return parseBalances(out)
}

// parseBalances extracts the first balance entry from the chain's
// bank/balances response. Expected shape:
//
//	{
//	  "balances": [{"denom":"ngonka","amount":"999990000"}],
//	  "pagination": {...}
//	}
//
// Returns (0, "", nil) when balances list is empty (fresh account).
func parseBalances(out []byte) (uint64, string, error) {
	var resp struct {
		Balances []struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, "", fmt.Errorf("parse balances JSON: %w", err)
	}
	if len(resp.Balances) == 0 {
		return 0, "", nil
	}
	first := resp.Balances[0]
	amt, err := strconv.ParseUint(first.Amount, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse amount %q: %w", first.Amount, err)
	}
	return amt, first.Denom, nil
}
