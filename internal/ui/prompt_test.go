package ui

import (
	"testing"
)

func TestResetOverrides(t *testing.T) {
	SetNonInteractive(true)
	SetOverride("test", "value")
	ResetOverrides()

	if isNonInteractive() {
		t.Error("expected non-interactive to be false after reset")
	}
	if _, ok := findOverride("test prompt"); ok {
		t.Error("expected no override after reset")
	}
}

func TestSelectWithOverride(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	options := []string{
		"mainnet - Production network",
		"testnet - Test network",
	}

	SetOverride("Select network", "testnet")
	result, err := Select("Select network to join:", options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "testnet - Test network" {
		t.Errorf("expected 'testnet - Test network', got %q", result)
	}
}

func TestSelectWithOverrideNoMatch(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	options := []string{"option A", "option B"}
	SetOverride("pick one", "nonexistent")

	_, err := Select("pick one:", options)
	if err == nil {
		t.Error("expected error for unmatched override value")
	}
}

func TestSelectNonInteractiveDefault(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	options := []string{"first option", "second option"}
	result, err := Select("choose something:", options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "first option" {
		t.Errorf("expected first option, got %q", result)
	}
}

func TestSelectNonInteractiveEmpty(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	_, err := Select("choose:", []string{})
	if err == nil {
		t.Error("expected error for empty options")
	}
}

func TestConfirmNonInteractive(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	result, err := Confirm("Proceed?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result {
		t.Error("expected true (default)")
	}

	result, err = Confirm("Are you sure?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Error("expected false (default)")
	}
}

func TestInputWithOverride(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	SetOverride("public IP", "10.0.0.1")
	result, err := Input("Enter your server's public IP or hostname:", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "10.0.0.1" {
		t.Errorf("expected '10.0.0.1', got %q", result)
	}
}

func TestInputNonInteractiveDefault(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	result, err := Input("Enter something:", "default-val")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "default-val" {
		t.Errorf("expected 'default-val', got %q", result)
	}
}

func TestInputNonInteractiveNoDefault(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	_, err := Input("Enter required value:", "")
	if err == nil {
		t.Error("expected error for required input with no default and no override")
	}
}

func TestPasswordWithOverride(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	SetOverride("password", "secret123")
	result, err := Password("Enter keyring password (min 8 characters):")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "secret123" {
		t.Errorf("expected 'secret123', got %q", result)
	}
}

func TestPasswordNonInteractiveNoOverride(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	_, err := Password("Enter password:")
	if err == nil {
		t.Error("expected error when password not provided in non-interactive mode")
	}
}

func TestMultiSelectNonInteractive(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	options := []string{"a", "b", "c"}
	result, err := MultiSelect("Choose all:", options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected all 3 options, got %d", len(result))
	}
}

func TestOverrideCaseInsensitive(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	SetOverride("PUBLIC IP", "192.168.1.1")
	result, err := Input("Enter your server's public ip or hostname:", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "192.168.1.1" {
		t.Errorf("expected '192.168.1.1', got %q", result)
	}
}

func TestSelectOverrideCaseInsensitive(t *testing.T) {
	defer ResetOverrides()
	SetNonInteractive(true)

	options := []string{
		"Quick Setup - All keys on server",
		"Secure Setup - Account key on separate machine (recommended)",
	}

	SetOverride("key management workflow", "quick")
	result, err := Select("Select key management workflow:", options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Quick Setup - All keys on server" {
		t.Errorf("expected Quick Setup option, got %q", result)
	}
}
