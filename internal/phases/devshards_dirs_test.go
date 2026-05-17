// M12 Slice 5b tests: ensureDevshardsDirs idempotent helper +
// port 9400 listed in checkPorts (mocked path).
package phases

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureDevshardsDirs_CreatesWith0750 (TC-1.1 dir-creation aspect).
func TestEnsureDevshardsDirs_CreatesWith0750(t *testing.T) {
	dir := t.TempDir()
	if err := ensureDevshardsDirs(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, sub := range []string{"bin", "data"} {
		full := filepath.Join(dir, "devshards", sub)
		info, err := os.Stat(full)
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", full)
		}
		if got := info.Mode().Perm(); got != 0o750 {
			t.Errorf("%s mode = %o, want 0750", full, got)
		}
	}
}

// TestEnsureDevshardsDirs_LeavesExistingUntouched (TC-1.4, UC-1-A3).
// Pre-existing dir with different mode must NOT be modified.
func TestEnsureDevshardsDirs_LeavesExistingUntouched(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "devshards", "bin")
	if err := os.MkdirAll(existing, 0o755); err != nil { // #nosec G301 - test fixture
		t.Fatalf("mkdir: %v", err)
	}
	marker := filepath.Join(existing, "marker")
	if err := os.WriteFile(marker, []byte("preexisting"), 0o644); err != nil { // #nosec G306 - test fixture
		t.Fatalf("marker: %v", err)
	}

	if err := ensureDevshardsDirs(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(existing)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("pre-existing dir mode mutated: got %o, want 0755", got)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker file removed: %v", err)
	}
}

// TestEnsureDevshardsDirs_Idempotent (TC-IDEM-3).
func TestEnsureDevshardsDirs_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := ensureDevshardsDirs(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := ensureDevshardsDirs(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

// TestEnsureDevshardsDirs_ConflictingFile — pre-existing `devshards/bin`
// as a file (not dir) returns clear error.
func TestEnsureDevshardsDirs_ConflictingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "devshards"), 0o750); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	conflict := filepath.Join(dir, "devshards", "bin")
	if err := os.WriteFile(conflict, []byte("not-a-dir"), 0o600); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}
	err := ensureDevshardsDirs(dir)
	if err == nil {
		t.Fatal("expected error when 'bin' exists as a file")
	}
}
