package cmd

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

func TestParseUpgradeHandlerError(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "single match",
			input:  `ERR error during handshake: upgrade handler is missing for v0.2.10 upgrade plan`,
			expect: "v0.2.10",
		},
		{
			name: "multiple matches returns last",
			input: `ERR upgrade handler is missing for v0.2.8 upgrade plan
				ERR upgrade handler is missing for v0.2.10 upgrade plan`,
			expect: "v0.2.10",
		},
		{
			name:   "no match",
			input:  `INF starting node module=main`,
			expect: "",
		},
		{
			name:   "empty input",
			input:  "",
			expect: "",
		},
		{
			name:   "version with post suffix",
			input:  `ERR upgrade handler is missing for v0.2.8-post1 upgrade plan`,
			expect: "v0.2.8-post1",
		},
		{
			name: "embedded in full log output",
			input: `node  | INF starting node module=main
node  | ERR CONSENSUS FAILURE!!! err="error during handshake"
node  | panic: upgrade handler is missing for v0.2.10 upgrade plan
node  | goroutine 1 [running]:`,
			expect: "v0.2.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUpgradeHandlerError(tt.input)
			if got != tt.expect {
				t.Errorf("parseUpgradeHandlerError() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestParseUpgradeInfoJSON(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantNil  bool
	}{
		{
			name:     "valid upgrade info",
			content:  `{"name":"v0.2.10","height":2500000}`,
			wantName: "v0.2.10",
		},
		{
			name:    "empty name",
			content: `{"name":"","height":0}`,
			wantNil: true,
		},
		{
			name:    "malformed JSON",
			content: `{invalid}`,
			wantNil: true,
		},
		{
			name:    "empty file",
			content: "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info upgradeInfoJSON
			err := json.Unmarshal([]byte(tt.content), &info)
			if err != nil || info.Name == "" {
				if !tt.wantNil {
					t.Errorf("expected name %q, got parse error", tt.wantName)
				}
				return
			}
			if tt.wantNil {
				t.Errorf("expected nil result, got name %q", info.Name)
				return
			}
			if info.Name != tt.wantName {
				t.Errorf("got name %q, want %q", info.Name, tt.wantName)
			}
		})
	}
}

func TestCheckCosmovisorSymlinks(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		wantCount int
	}{
		{
			name: "valid symlink",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				cosmoDir := filepath.Join(dir, ".inference", "cosmovisor")
				upgradeDir := filepath.Join(cosmoDir, "upgrades", "v0.2.10", "bin")
				if err := os.MkdirAll(upgradeDir, 0o750); err != nil {
					t.Fatal(err)
				}
				_ = os.Symlink(filepath.Join(cosmoDir, "upgrades", "v0.2.10"),
					filepath.Join(cosmoDir, "current"))
			},
			wantCount: 0,
		},
		{
			name: "broken symlink",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				cosmoDir := filepath.Join(dir, ".inference", "cosmovisor")
				if err := os.MkdirAll(cosmoDir, 0o750); err != nil {
					t.Fatal(err)
				}
				_ = os.Symlink(filepath.Join(cosmoDir, "upgrades", "v0.2.99"),
					filepath.Join(cosmoDir, "current"))
			},
			wantCount: 1,
		},
		{
			name: "no cosmovisor dir",
			setup: func(_ *testing.T, _ string) {
				// No setup — dir doesn't have cosmovisor
			},
			wantCount: 0,
		},
		{
			name: "broken symlinks in both inference and dapi",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				for _, sub := range []string{".inference", ".dapi"} {
					cosmoDir := filepath.Join(dir, sub, "cosmovisor")
					if err := os.MkdirAll(cosmoDir, 0o750); err != nil {
						t.Fatal(err)
					}
					_ = os.Symlink(filepath.Join(cosmoDir, "upgrades", "v0.2.99"),
						filepath.Join(cosmoDir, "current"))
				}
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			state := &config.State{OutputDir: dir}
			diags := checkCosmovisorSymlinks(state)
			if len(diags) != tt.wantCount {
				t.Errorf("got %d diagnoses, want %d", len(diags), tt.wantCount)
				for _, d := range diags {
					t.Logf("  %s: %s", d.ID, d.Description)
				}
			}
			for _, d := range diags {
				if d.ID != "broken_symlink" {
					t.Errorf("unexpected diagnosis ID: %s", d.ID)
				}
			}
		})
	}
}

func TestParseGitHubReleaseAssets(t *testing.T) {
	body := `{
		"tag_name": "release/v0.2.10",
		"assets": [
			{
				"name": "inferenced-amd64.zip",
				"browser_download_url": "https://github.com/gonka-ai/gonka/releases/download/release%2Fv0.2.10/inferenced-amd64.zip",
				"size": 98000000,
				"digest": "sha256:b118610cfa1f1234567890abcdef1234567890abcdef1234567890abcdef1234"
			},
			{
				"name": "decentralized-api-amd64.zip",
				"browser_download_url": "https://github.com/gonka-ai/gonka/releases/download/release%2Fv0.2.10/decentralized-api-amd64.zip",
				"size": 191000000,
				"digest": "sha256:47d6b64424f31234567890abcdef1234567890abcdef1234567890abcdef1234"
			}
		]
	}`

	assets, err := parseGitHubReleaseAssets([]byte(body))
	if err != nil {
		t.Fatalf("parseGitHubReleaseAssets() error: %v", err)
	}

	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}

	// Check inferenced asset
	if assets[0].Name != "inferenced-amd64.zip" {
		t.Errorf("first asset name: got %q, want %q", assets[0].Name, "inferenced-amd64.zip")
	}
	if assets[0].SHA256 != "b118610cfa1f1234567890abcdef1234567890abcdef1234567890abcdef1234" {
		t.Errorf("first asset SHA256 not parsed correctly: %q", assets[0].SHA256)
	}
	if assets[0].Size != 98000000 {
		t.Errorf("first asset size: got %d, want %d", assets[0].Size, 98000000)
	}

	// Check download URL preserved
	if assets[1].DownloadURL == "" {
		t.Error("download URL should not be empty")
	}
}

func TestParseGitHubReleaseAssets_Empty(t *testing.T) {
	body := `{"tag_name": "release/v0.2.10", "assets": []}`
	assets, err := parseGitHubReleaseAssets([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assets) != 0 {
		t.Errorf("expected 0 assets, got %d", len(assets))
	}
}

func TestParseGitHubReleaseAssets_InvalidJSON(t *testing.T) {
	_, err := parseGitHubReleaseAssets([]byte(`{invalid}`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseDigest(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "sha256 prefix",
			input:  "sha256:b118610cfa1fabcdef",
			expect: "b118610cfa1fabcdef",
		},
		{
			name:   "no prefix",
			input:  "b118610cfa1fabcdef",
			expect: "b118610cfa1fabcdef",
		},
		{
			name:   "empty string",
			input:  "",
			expect: "",
		},
		{
			name:   "full digest",
			input:  "sha256:b118610cfa1f1234567890abcdef1234567890abcdef1234567890abcdef1234",
			expect: "b118610cfa1f1234567890abcdef1234567890abcdef1234567890abcdef1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDigest(tt.input)
			if got != tt.expect {
				t.Errorf("parseDigest(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	testFile := filepath.Join(dir, "testfile.bin")
	content := []byte("hello world test content for sha256")

	if err := os.WriteFile(testFile, content, 0o600); err != nil {
		t.Fatal(err)
	}

	// Compute expected hash
	h := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(h[:])

	t.Run("matching hash", func(t *testing.T) {
		err := verifySHA256(testFile, expectedHash)
		if err != nil {
			t.Errorf("expected no error for matching hash, got: %v", err)
		}
	})

	t.Run("mismatched hash", func(t *testing.T) {
		err := verifySHA256(testFile, "0000000000000000000000000000000000000000000000000000000000000000")
		if err == nil {
			t.Error("expected error for mismatched hash")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		err := verifySHA256(filepath.Join(dir, "nonexistent"), expectedHash)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestUpgradeNameToReleaseTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple version",
			input:    "v0.2.10",
			expected: []string{"release/v0.2.10", "release/v0.2.10-post1"},
		},
		{
			name:     "post suffix already",
			input:    "v0.2.8-post1",
			expected: []string{"release/v0.2.8-post1", "release/v0.2.8-post1-post1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := upgradeNameToReleaseTags(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("got %d tags, want %d", len(got), len(tt.expected))
			}
			for i, tag := range got {
				if tag != tt.expected[i] {
					t.Errorf("tag[%d] = %q, want %q", i, tag, tt.expected[i])
				}
			}
		})
	}
}

func TestResolveRepairAdminURL(t *testing.T) {
	// Save and restore global state
	origURL := repairAdminURL
	defer func() { repairAdminURL = origURL }()

	t.Run("state AdminURL used when flag is default", func(t *testing.T) {
		repairAdminURL = defaultAdminURL
		state := &config.State{AdminURL: "http://10.0.0.1:9200"}
		got := resolveRepairAdmin(state)
		if got != "http://10.0.0.1:9200" {
			t.Errorf("expected state URL, got %q", got)
		}
	})

	t.Run("explicit flag overrides state", func(t *testing.T) {
		repairAdminURL = testCustomAdmin
		state := &config.State{AdminURL: "http://10.0.0.1:9200"}
		got := resolveRepairAdmin(state)
		if got != testCustomAdmin {
			t.Errorf("expected custom URL, got %q", got)
		}
	})

	t.Run("fallback to default", func(t *testing.T) {
		repairAdminURL = defaultAdminURL
		state := &config.State{}
		got := resolveRepairAdmin(state)
		if got != defaultAdminURL {
			t.Errorf("expected default URL, got %q", got)
		}
	})
}

func TestFixCosmovisorSymlink(t *testing.T) {
	// This test verifies symlink creation on the local filesystem
	// without sudo (which is fine for testing)
	dir := t.TempDir()

	cosmoDir := filepath.Join(dir, ".inference", "cosmovisor")
	upgradeDir := filepath.Join(cosmoDir, "upgrades", "v0.2.10", "bin")
	if err := os.MkdirAll(upgradeDir, 0o750); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(cosmoDir, "current")
	targetDir := filepath.Join(cosmoDir, "upgrades", "v0.2.10")

	state := &config.State{OutputDir: dir, UseSudo: false}

	t.Run("create new symlink", func(t *testing.T) {
		err := fixCosmovisorSymlink(t.Context(), state, symlinkPath, targetDir)
		if err != nil {
			t.Fatalf("fixCosmovisorSymlink() error: %v", err)
		}

		// Verify symlink exists and points correctly
		target, linkErr := os.Readlink(symlinkPath)
		if linkErr != nil {
			t.Fatalf("readlink error: %v", linkErr)
		}

		// Should be relative path
		if target != "upgrades/v0.2.10" {
			t.Errorf("symlink target = %q, want %q", target, "upgrades/v0.2.10")
		}

		// Verify target resolves
		resolved := filepath.Join(filepath.Dir(symlinkPath), target)
		if _, err := os.Stat(resolved); os.IsNotExist(err) {
			t.Error("symlink target does not exist")
		}
	})

	t.Run("update existing symlink", func(t *testing.T) {
		// Create a new upgrade version
		newUpgradeDir := filepath.Join(cosmoDir, "upgrades", "v0.2.11", "bin")
		if err := os.MkdirAll(newUpgradeDir, 0o750); err != nil {
			t.Fatal(err)
		}

		newTarget := filepath.Join(cosmoDir, "upgrades", "v0.2.11")
		err := fixCosmovisorSymlink(t.Context(), state, symlinkPath, newTarget)
		if err != nil {
			t.Fatalf("fixCosmovisorSymlink() error: %v", err)
		}

		target, linkErr := os.Readlink(symlinkPath)
		if linkErr != nil {
			t.Fatalf("readlink error: %v", linkErr)
		}

		if target != "upgrades/v0.2.11" {
			t.Errorf("symlink target = %q, want %q", target, "upgrades/v0.2.11")
		}
	})
}

func TestFindBinaryAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "inferenced-amd64.zip", DownloadURL: "https://example.com/inferenced.zip", SHA256: "abc123", Size: 98000000},
		{Name: "decentralized-api-amd64.zip", DownloadURL: "https://example.com/api.zip", SHA256: "def456", Size: 191000000},
		{Name: "checksums.txt", DownloadURL: "https://example.com/checksums.txt", Size: 256},
	}

	t.Run("find inferenced", func(t *testing.T) {
		a := findBinaryAsset(assets, "inferenced-amd64.zip")
		if a == nil {
			t.Fatal("expected to find inferenced asset")
		}
		if a.SHA256 != "abc123" {
			t.Errorf("SHA256 = %q, want %q", a.SHA256, "abc123")
		}
	})

	t.Run("find api", func(t *testing.T) {
		a := findBinaryAsset(assets, "decentralized-api-amd64.zip")
		if a == nil {
			t.Fatal("expected to find api asset")
		}
		if a.Size != 191000000 {
			t.Errorf("Size = %d, want %d", a.Size, 191000000)
		}
	})

	t.Run("not found", func(t *testing.T) {
		a := findBinaryAsset(assets, "nonexistent.zip")
		if a != nil {
			t.Error("expected nil for nonexistent asset")
		}
	})

	t.Run("empty list", func(t *testing.T) {
		a := findBinaryAsset(nil, "inferenced-amd64.zip")
		if a != nil {
			t.Error("expected nil for empty asset list")
		}
	})
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "simple path",
			input:  "/home/user/gonka",
			expect: "'/home/user/gonka'",
		},
		{
			name:   "path with spaces",
			input:  "/home/user/my node",
			expect: "'/home/user/my node'",
		},
		{
			name:   "path with single quote",
			input:  "/home/user/it's",
			expect: "'/home/user/it'\\''s'",
		},
		{
			name:   "empty string",
			input:  "",
			expect: "''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.expect {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestExtractZipBinary(t *testing.T) {
	dir := t.TempDir()

	// Create a zip file with a known binary
	zipPath := filepath.Join(dir, "test.zip")
	destPath := filepath.Join(dir, "extracted-binary")
	binaryContent := []byte("#!/bin/sh\necho 'hello'\n")

	// Create the zip
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)

	fw, err := zw.Create("inferenced")
	if err != nil {
		t.Fatal(err)
	}
	if _, writeErr := fw.Write(binaryContent); writeErr != nil {
		t.Fatal(writeErr)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	t.Run("extract existing binary", func(t *testing.T) {
		err := extractZipBinary(zipPath, destPath, "inferenced", false)
		if err != nil {
			t.Fatalf("extractZipBinary() error: %v", err)
		}

		// Verify extracted content
		data, readErr := os.ReadFile(destPath)
		if readErr != nil {
			t.Fatalf("read extracted file: %v", readErr)
		}
		if string(data) != string(binaryContent) {
			t.Errorf("content mismatch: got %q, want %q", string(data), string(binaryContent))
		}
	})

	t.Run("binary not in zip", func(t *testing.T) {
		err := extractZipBinary(zipPath, filepath.Join(dir, "other"), "nonexistent", false)
		if err == nil {
			t.Error("expected error for missing binary in zip")
		}
	})

	t.Run("invalid zip file", func(t *testing.T) {
		invalidZip := filepath.Join(dir, "invalid.zip")
		if writeErr := os.WriteFile(invalidZip, []byte("not a zip"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		err := extractZipBinary(invalidZip, filepath.Join(dir, "out"), "inferenced", false)
		if err == nil {
			t.Error("expected error for invalid zip")
		}
	})
}

func TestDiagnosisStruct(t *testing.T) {
	d := Diagnosis{
		ID:          "missing_upgrade_handler",
		Severity:    "critical",
		Description: "Node in restart loop: upgrade handler is missing for v0.2.10",
		FixAction:   "Download v0.2.10 binaries",
		UpgradeName: "v0.2.10",
	}

	if d.ID != "missing_upgrade_handler" {
		t.Errorf("unexpected ID: %s", d.ID)
	}
	if d.Severity != "critical" {
		t.Errorf("unexpected severity: %s", d.Severity)
	}
	if d.UpgradeName != "v0.2.10" {
		t.Errorf("unexpected upgrade name: %s", d.UpgradeName)
	}
}

func TestRepairPlan(t *testing.T) {
	plan := &RepairPlan{
		Diagnoses: []Diagnosis{
			{ID: "missing_upgrade_handler", UpgradeName: "v0.2.10"},
			{ID: "stale_upgrade_info", UpgradeName: "v0.2.10"},
		},
		UpgradeName: "v0.2.10",
		NeedsBinary: true,
	}

	if len(plan.Diagnoses) != 2 {
		t.Errorf("expected 2 diagnoses, got %d", len(plan.Diagnoses))
	}
	if !plan.NeedsBinary {
		t.Error("expected NeedsBinary=true")
	}
	if plan.UpgradeName != "v0.2.10" {
		t.Errorf("expected UpgradeName=v0.2.10, got %s", plan.UpgradeName)
	}
}
