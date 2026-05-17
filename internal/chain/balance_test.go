package chain

import "testing"

func TestParseBalances_HappyPath(t *testing.T) {
	body := []byte(`{"balances":[{"denom":"ngonka","amount":"999990000"}]}`)
	amt, denom, err := parseBalances(body)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if amt != 999990000 || denom != "ngonka" {
		t.Errorf("got %d %s, want 999990000 ngonka", amt, denom)
	}
}

func TestParseBalances_EmptyBalances_ReturnsZero(t *testing.T) {
	body := []byte(`{"balances":[]}`)
	amt, denom, err := parseBalances(body)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if amt != 0 || denom != "" {
		t.Errorf("expected (0, ''), got (%d, %s)", amt, denom)
	}
}

func TestParseBalances_MalformedJSON_Errors(t *testing.T) {
	if _, _, err := parseBalances([]byte("not json")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseBalances_NonNumericAmount_Errors(t *testing.T) {
	body := []byte(`{"balances":[{"denom":"ngonka","amount":"not-a-number"}]}`)
	if _, _, err := parseBalances(body); err == nil {
		t.Fatal("expected parse error")
	}
}
