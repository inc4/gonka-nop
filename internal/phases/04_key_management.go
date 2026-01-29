package phases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// KeyManagement handles key generation workflow
type KeyManagement struct {
	workflow string // "quick" or "secure"
}

func NewKeyManagement(workflow string) *KeyManagement {
	return &KeyManagement{workflow: workflow}
}

func (p *KeyManagement) Name() string {
	return "Key Management"
}

func (p *KeyManagement) Description() string {
	return "Setting up node keys (Account, Consensus, ML Operational)"
}

func (p *KeyManagement) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *KeyManagement) Run(ctx context.Context, state *config.State) error {
	// If workflow not set, ask user
	workflow := p.workflow
	if workflow == "" {
		options := []string{
			"Quick Setup - Generate all keys on this machine (less secure)",
			"Secure Setup - Account key on separate machine (recommended)",
		}
		selected, err := ui.Select("Select key management workflow:", options)
		if err != nil {
			return err
		}
		if selected == options[0] {
			workflow = "quick"
		} else {
			workflow = "secure"
		}
	}
	state.KeyWorkflow = workflow

	if workflow == "secure" {
		return p.runSecureWorkflow(ctx, state)
	}
	return p.runQuickWorkflow(ctx, state)
}

func (p *KeyManagement) runQuickWorkflow(_ context.Context, state *config.State) error {
	ui.Info("Running quick setup - all keys will be generated on this machine")
	ui.Warn("For production, consider using secure setup with cold account key")

	// Get key name
	keyName, err := ui.Input("Enter a name for your node keys:", "gonka-node")
	if err != nil {
		return err
	}
	state.KeyName = keyName

	// Get keyring password
	password, err := ui.Password("Enter keyring password:")
	if err != nil {
		return err
	}
	_ = password // Would be used for actual key generation

	// Generate Account Key (mocked)
	err = ui.WithSpinner("Generating Account Key", func() error {
		time.Sleep(800 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	accountPubKey := generateMockPubKey()
	state.AccountPubKey = accountPubKey
	ui.Detail("Account Public Key: %s...", accountPubKey[:20])

	// Generate Consensus Key for TMKMS (mocked)
	err = ui.WithSpinner("Generating Consensus Key (TMKMS)", func() error {
		time.Sleep(600 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Consensus key configured for TMKMS")

	// Generate ML Operational Key (mocked)
	err = ui.WithSpinner("Generating ML Operational Key", func() error {
		time.Sleep(600 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("ML Operational key ready for automated transactions")

	ui.Success("All keys generated successfully")
	return nil
}

func (p *KeyManagement) runSecureWorkflow(_ context.Context, state *config.State) error {
	ui.Info("Running secure setup - Account key should be on a separate machine")

	// Check if account pubkey was provided
	if state.AccountPubKey == "" {
		ui.Header("Account Key Setup")
		ui.Info("You need to provide your Account Public Key.")
		ui.Detail("Generate it on your local machine with: gonka-nop init-account")

		pubkey, err := ui.Input("Enter your Account Public Key:", "")
		if err != nil {
			return err
		}
		state.AccountPubKey = pubkey
	}

	// Get key name
	keyName, err := ui.Input("Enter a name for your server keys:", "gonka-node")
	if err != nil {
		return err
	}
	state.KeyName = keyName

	// Generate Consensus Key for TMKMS (mocked)
	err = ui.WithSpinner("Generating Consensus Key (TMKMS)", func() error {
		time.Sleep(600 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("Consensus key configured for TMKMS")

	// Generate ML Operational Key (mocked)
	err = ui.WithSpinner("Generating ML Operational Key", func() error {
		time.Sleep(600 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}
	ui.Detail("ML Operational key ready")

	ui.Success("Server keys generated successfully")
	ui.Info("Remember: Register your node using your Account Key on your local machine")

	return nil
}

// generateMockPubKey generates a mock public key for demo
func generateMockPubKey() string {
	bytes := make([]byte, 32)
	_, _ = rand.Read(bytes)
	return "gonkapub1" + hex.EncodeToString(bytes)[:40]
}
