package cosmovisor

import (
	"os"
	"path/filepath"
	"testing"
)

const testUpgradeVersion = "v0.2.10"

func TestFixSymlink(t *testing.T) {
	// This test verifies symlink creation on the local filesystem
	// without sudo (which is fine for testing)
	dir := t.TempDir()

	cosmoDir := filepath.Join(dir, ".inference", "cosmovisor")
	upgradeDir := filepath.Join(cosmoDir, "upgrades", testUpgradeVersion, "bin")
	if err := os.MkdirAll(upgradeDir, 0o750); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(cosmoDir, "current")
	targetDir := filepath.Join(cosmoDir, "upgrades", testUpgradeVersion)

	t.Run("create new symlink", func(t *testing.T) {
		err := FixSymlink(t.Context(), false, dir, symlinkPath, targetDir)
		if err != nil {
			t.Fatalf("FixSymlink() error: %v", err)
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
		err := FixSymlink(t.Context(), false, dir, symlinkPath, newTarget)
		if err != nil {
			t.Fatalf("FixSymlink() error: %v", err)
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
