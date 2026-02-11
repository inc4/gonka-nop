package phases

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// KeyOutput holds parsed output from `inferenced keys add --output json`.
type KeyOutput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Address  string `json:"address"`
	PubKey   string `json:"pubkey"`
	Mnemonic string `json:"mnemonic"`
}

// ParseKeyOutput parses the JSON output from `inferenced keys add --output json`.
func ParseKeyOutput(jsonOutput string) (*KeyOutput, error) {
	jsonOutput = strings.TrimSpace(jsonOutput)
	if jsonOutput == "" {
		return nil, fmt.Errorf("empty key output")
	}
	var key KeyOutput
	if err := json.Unmarshal([]byte(jsonOutput), &key); err != nil {
		return nil, fmt.Errorf("parse key JSON: %w", err)
	}
	if key.Address == "" {
		return nil, fmt.Errorf("key output missing address")
	}
	return &key, nil
}

// ExtractPubKeyBase64 extracts the base64 public key from the pubkey JSON string.
// inferenced returns pubkey as: {"@type":"/cosmos.crypto.secp256k1.PubKey","key":"base64..."}
// The API expects just the base64 key part.
func ExtractPubKeyBase64(pubkeyJSON string) string {
	var pk struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(pubkeyJSON), &pk); err != nil {
		// Not JSON — return as-is (might already be base64)
		return pubkeyJSON
	}
	if pk.Key != "" {
		return pk.Key
	}
	return pubkeyJSON
}

// ExtractMnemonic extracts the mnemonic from inferenced stderr output.
// The mnemonic is printed on stderr as a multi-word phrase.
func ExtractMnemonic(stderr string) string {
	// The mnemonic is typically the last line of stderr with 12+ words
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		words := strings.Fields(line)
		if len(words) >= 12 {
			return line
		}
	}
	return ""
}

// CreateKeyViaDocker creates a key by running inferenced inside a Docker container.
// Returns the parsed key output and the mnemonic phrase.
func CreateKeyViaDocker(ctx context.Context, imageRef, keyName, password, keyringDir string, useSudo bool) (*KeyOutput, string, error) {
	// Build the shell command to pipe:
	// "y" for override prompt (if key exists), then password 3 times (new, confirm, unlock)
	shellCmd := fmt.Sprintf(
		`printf 'y\n%s\n%s\n%s\n' | inferenced keys add %s --keyring-backend file --keyring-dir /root/.inference --output json`,
		password, password, password, keyName,
	)

	args := []string{
		"run", "--rm",
		"-v", keyringDir + ":/root/.inference",
		imageRef,
		"sh", "-c", shellCmd,
	}

	var cmd *exec.Cmd
	if useSudo {
		sudoArgs := append([]string{"-E", "docker"}, args...)
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...) // #nosec G204
	} else {
		cmd = exec.CommandContext(ctx, "docker", args...) // #nosec G204
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("docker run inferenced keys add: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	key, err := ParseKeyOutput(stdout.String())
	if err != nil {
		return nil, "", fmt.Errorf("parse key output: %w\nraw stdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// v0.2.9+ includes mnemonic in JSON output; fall back to stderr extraction
	mnemonic := key.Mnemonic
	if mnemonic == "" {
		mnemonic = ExtractMnemonic(stderr.String())
	}
	return key, mnemonic, nil
}
