package docker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inc4/gonka-nop/internal/cosmovisor"
)

// minimalCompose is the small valid compose fixture used by most
// AtomicReplace tests. Hoisted to a constant to silence goconst lint.
const minimalCompose = "services:\n  api:\n    image: a:1\n"

// writeFile is a fixture helper that creates a file with the given content
// and mode under dir/name and returns the absolute path. Test will fail
// hard via t.Fatalf if the underlying write fails — fixture failures are
// not part of the assertion logic.
func writeFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(content), mode); err != nil { // #nosec G306 - test fixture
		t.Fatalf("fixture writeFile(%s): %v", full, err)
	}
	// os.WriteFile honors umask; force exact mode for permission assertions.
	if err := os.Chmod(full, mode); err != nil {
		t.Fatalf("fixture chmod(%s, %o): %v", full, mode, err)
	}
	return full
}

// sha256Hex returns the lowercase hex SHA-256 digest of the file at path.
func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 - test helper, paths under t.TempDir()
	if err != nil {
		t.Fatalf("sha256Hex read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// listDirNames returns all entry names in dir, sorted by os.ReadDir order.
func listDirNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listDirNames(%s): %v", dir, err)
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// findBakFiles returns all entries in dir whose name starts with base+".bak.".
func findBakFiles(t *testing.T, dir, base string) []string {
	t.Helper()
	prefix := base + ".bak."
	var out []string
	for _, n := range listDirNames(t, dir) {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

// findTmpFiles returns all entries in dir whose name starts with base+".tmp.".
func findTmpFiles(t *testing.T, dir, base string) []string {
	t.Helper()
	prefix := base + ".tmp."
	var out []string
	for _, n := range listDirNames(t, dir) {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

// TestAtomicReplace_HappyPath (TC-MUTATION-1).
func TestAtomicReplace_HappyPath(t *testing.T) {
	dir := t.TempDir()
	original := minimalCompose
	path := writeFile(t, dir, "compose.yml", original, 0o644)
	preHash := sha256Hex(t, path)

	transform := func(in []byte) ([]byte, error) {
		return bytes.ReplaceAll(in, []byte("a:1"), []byte("a:2")), nil
	}

	backupPath, err := AtomicReplace(path, transform)
	if err != nil {
		t.Fatalf("AtomicReplace returned err: %v", err)
	}
	if backupPath == "" {
		t.Errorf("expected non-empty backupPath, got empty string")
	}

	got, err := os.ReadFile(path) // #nosec G304 - test path
	if err != nil {
		t.Fatalf("read target after replace: %v", err)
	}
	if !bytes.Contains(got, []byte("a:2")) {
		t.Errorf("target content missing 'a:2'; got: %q", string(got))
	}
	if bytes.Contains(got, []byte("a:1")) {
		t.Errorf("target content still contains old 'a:1'; got: %q", string(got))
	}

	if _, statErr := os.Stat(backupPath); statErr != nil {
		t.Errorf("backup file not present at %s: %v", backupPath, statErr)
	}
	if got := sha256Hex(t, backupPath); got != preHash {
		t.Errorf("backup SHA-256 = %s, want pre-mutation %s", got, preHash)
	}

	if leftover := findTmpFiles(t, dir, "compose.yml"); len(leftover) != 0 {
		t.Errorf("expected no .tmp.* siblings, got %v", leftover)
	}
}

// TestAtomicReplace_TransformError_NoMutation_NoBackup (TC-MUTATION-2).
func TestAtomicReplace_TransformError_NoMutation_NoBackup(t *testing.T) {
	dir := t.TempDir()
	original := minimalCompose
	path := writeFile(t, dir, "compose.yml", original, 0o644)
	preHash := sha256Hex(t, path)

	wantErr := fmt.Errorf("nope")
	transform := func(_ []byte) ([]byte, error) {
		return nil, wantErr
	}

	backupPath, err := AtomicReplace(path, transform)
	if err == nil {
		t.Fatalf("expected error from transform, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected returned error to wrap %v, got %v", wantErr, err)
	}
	if backupPath != "" {
		t.Errorf("expected empty backupPath on failure, got %q", backupPath)
	}

	if got := sha256Hex(t, path); got != preHash {
		t.Errorf("target SHA-256 changed after failed transform: got %s, want %s", got, preHash)
	}
	if baks := findBakFiles(t, dir, "compose.yml"); len(baks) != 0 {
		t.Errorf("expected no .bak.* files after transform error, got %v", baks)
	}
	if tmps := findTmpFiles(t, dir, "compose.yml"); len(tmps) != 0 {
		t.Errorf("expected no .tmp.* files after transform error, got %v", tmps)
	}
}

// TestAtomicReplace_InvalidYAML_RollsBack (TC-MUTATION-3).
func TestAtomicReplace_InvalidYAML_RollsBack(t *testing.T) {
	dir := t.TempDir()
	original := minimalCompose
	path := writeFile(t, dir, "compose.yml", original, 0o644)
	preHash := sha256Hex(t, path)

	transform := func(_ []byte) ([]byte, error) {
		return []byte("foo: : bar\n"), nil
	}

	backupPath, err := AtomicReplace(path, transform)
	if err == nil {
		t.Fatalf("expected YAML-validity error, got nil")
	}
	if !strings.Contains(err.Error(), "post-condition YAML validity failed") {
		t.Errorf("expected error to contain 'post-condition YAML validity failed', got %q", err.Error())
	}
	if backupPath != "" {
		t.Errorf("expected empty backupPath on validation failure, got %q", backupPath)
	}

	if got := sha256Hex(t, path); got != preHash {
		t.Errorf("target SHA-256 changed after invalid YAML: got %s, want %s", got, preHash)
	}
	// Per architect Q1: order is read→transform→validate→bak→tmp→rename, so
	// bak must NOT exist when validation fails.
	if baks := findBakFiles(t, dir, "compose.yml"); len(baks) != 0 {
		t.Errorf("expected no .bak.* files when validation fails, got %v", baks)
	}
	if tmps := findTmpFiles(t, dir, "compose.yml"); len(tmps) != 0 {
		t.Errorf("expected no .tmp.* files when validation fails, got %v", tmps)
	}
}

// TestAtomicReplace_LockContention_FailsFast (TC-MUTATION-4 + TC-SEC-6).
func TestAtomicReplace_LockContention_FailsFast(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "compose.yml", minimalCompose, 0o644)

	lockPath := filepath.Join(dir, ".nop.lock")
	release, err := cosmovisor.AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to pre-acquire lock for test: %v", err)
	}
	t.Cleanup(func() { _ = release() })

	start := time.Now()
	backupPath, err := AtomicReplace(path, func(in []byte) ([]byte, error) {
		return in, nil
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected lock-held error, got nil")
	}
	if backupPath != "" {
		t.Errorf("expected empty backupPath when lock contended, got %q", backupPath)
	}
	if !errors.Is(err, cosmovisor.ErrLockHeld) {
		t.Errorf("expected errors.Is(err, cosmovisor.ErrLockHeld) = true, got err=%v", err)
	}
	if !strings.Contains(err.Error(), lockPath) {
		t.Errorf("expected error to contain lock path %q, got %q", lockPath, err.Error())
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected fail-fast within 100ms, took %v", elapsed)
	}
}

// TestAtomicReplace_RecoveryFromHalfState_BakAndTmpPresent (replaces TC-MUTATION-5).
func TestAtomicReplace_RecoveryFromHalfState_BakAndTmpPresent(t *testing.T) {
	dir := t.TempDir()
	original := minimalCompose
	path := writeFile(t, dir, "compose.yml", original, 0o644)

	// Orphan bak from a prior run (different timestamp + rand suffix).
	orphanBak := "compose.yml.bak.20260101T000000Z.deadbeef"
	writeFile(t, dir, orphanBak, "stale-backup-content\n", 0o600)

	// Orphan tmp from a prior interrupted run (different random suffix so no
	// collision with the new tmp this call would create).
	orphanTmp := "compose.yml.tmp.xyz"
	writeFile(t, dir, orphanTmp, "stale-tmp-content\n", 0o600)

	transform := func(in []byte) ([]byte, error) {
		return bytes.ReplaceAll(in, []byte("a:1"), []byte("a:2")), nil
	}

	backupPath, err := AtomicReplace(path, transform)
	if err != nil {
		t.Fatalf("AtomicReplace returned err: %v", err)
	}
	if backupPath == "" {
		t.Errorf("expected non-empty backupPath, got empty string")
	}

	got, err := os.ReadFile(path) // #nosec G304 - test path
	if err != nil {
		t.Fatalf("read target after replace: %v", err)
	}
	if !bytes.Contains(got, []byte("a:2")) {
		t.Errorf("target content missing 'a:2'; got: %q", string(got))
	}

	// Orphans must remain untouched — AtomicReplace doesn't sweep peer files.
	if _, statErr := os.Stat(filepath.Join(dir, orphanBak)); statErr != nil {
		t.Errorf("orphan bak %s was removed: %v", orphanBak, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, orphanTmp)); statErr != nil {
		t.Errorf("orphan tmp %s was removed: %v", orphanTmp, statErr)
	}

	// New bak must exist with a different suffix from the orphan.
	baks := findBakFiles(t, dir, "compose.yml")
	if len(baks) != 2 {
		t.Errorf("expected exactly 2 .bak.* files (orphan + new), got %d: %v", len(baks), baks)
	}
	foundNew := false
	for _, n := range baks {
		if n != orphanBak {
			foundNew = true
			break
		}
	}
	if !foundNew {
		t.Errorf("expected a new .bak.* file distinct from orphan %q, got %v", orphanBak, baks)
	}
}

// TestAtomicReplace_PathValidation_RejectsRelativeAndDotDot (architect's added security test).
func TestAtomicReplace_PathValidation_RejectsRelativeAndDotDot(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name string
		path string
	}{
		{name: "relative", path: "compose.yml"},
		{name: "dot-dot", path: filepath.Join(dir, "foo", "..", "bar", "x.yml")},
		{name: "nul-byte", path: filepath.Join(dir, "x\x00.yml")},
	}

	preEntries := listDirNames(t, dir)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backupPath, err := AtomicReplace(tc.path, func(in []byte) ([]byte, error) {
				return in, nil
			})
			if err == nil {
				t.Errorf("expected path-validation error for %q, got nil", tc.path)
			}
			if backupPath != "" {
				t.Errorf("expected empty backupPath on path validation failure, got %q", backupPath)
			}
		})
	}

	// Assert no files were created in the tmpdir as a side effect of
	// path validation rejection (must fail BEFORE any I/O).
	postEntries := listDirNames(t, dir)
	if len(postEntries) != len(preEntries) {
		t.Errorf("expected no files created in tmpdir after path-validation rejects, before=%v after=%v",
			preEntries, postEntries)
	}
}

// TestAtomicReplace_PreservesSourceFileMode (architect Q7).
func TestAtomicReplace_PreservesSourceFileMode(t *testing.T) {
	cases := []struct {
		name string
		mode os.FileMode
	}{
		{name: "0644", mode: 0o644},
		{name: "0600", mode: 0o600},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "compose.yml", minimalCompose, tc.mode)

			_, err := AtomicReplace(path, func(in []byte) ([]byte, error) {
				return bytes.ReplaceAll(in, []byte("a:1"), []byte("a:2")), nil
			})
			if err != nil {
				t.Fatalf("AtomicReplace returned err: %v", err)
			}

			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatalf("stat after replace: %v", statErr)
			}
			if info.Mode().Perm() != tc.mode {
				t.Errorf("file mode after replace = %o, want %o", info.Mode().Perm(), tc.mode)
			}
		})
	}
}

// TestAtomicReplace_BackupMode_Is0600 (architect Q7 + security).
func TestAtomicReplace_BackupMode_Is0600(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "compose.yml", minimalCompose, 0o644)

	backupPath, err := AtomicReplace(path, func(in []byte) ([]byte, error) {
		return bytes.ReplaceAll(in, []byte("a:1"), []byte("a:2")), nil
	})
	if err != nil {
		t.Fatalf("AtomicReplace returned err: %v", err)
	}
	if backupPath == "" {
		t.Fatalf("expected non-empty backupPath")
	}

	info, statErr := os.Stat(backupPath)
	if statErr != nil {
		t.Fatalf("stat backup: %v", statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %o, want 0600", info.Mode().Perm())
	}
}

// TestAtomicReplace_LockErrorWrappedForCallers (architect Q3).
func TestAtomicReplace_LockErrorWrappedForCallers(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "compose.yml", minimalCompose, 0o644)

	lockPath := filepath.Join(dir, ".nop.lock")
	release, err := cosmovisor.AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to pre-acquire lock: %v", err)
	}
	t.Cleanup(func() { _ = release() })

	_, callErr := AtomicReplace(path, func(in []byte) ([]byte, error) {
		return in, nil
	})
	if callErr == nil {
		t.Fatalf("expected lock-held error, got nil")
	}
	if !errors.Is(callErr, cosmovisor.ErrLockHeld) {
		t.Errorf("expected errors.Is(err, cosmovisor.ErrLockHeld) = true, got err=%v", callErr)
	}
	if !strings.Contains(callErr.Error(), lockPath) {
		t.Errorf("expected error message to contain lock path %q, got %q", lockPath, callErr.Error())
	}
}

// TestAtomicReplace_BackupHasRandomSuffix (architect Q4).
func TestAtomicReplace_BackupHasRandomSuffix(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "compose.yml", minimalCompose, 0o644)

	bak1, err := AtomicReplace(path, func(in []byte) ([]byte, error) {
		return bytes.ReplaceAll(in, []byte("a:1"), []byte("a:2")), nil
	})
	if err != nil {
		t.Fatalf("first AtomicReplace returned err: %v", err)
	}

	bak2, err := AtomicReplace(path, func(in []byte) ([]byte, error) {
		return bytes.ReplaceAll(in, []byte("a:2"), []byte("a:3")), nil
	})
	if err != nil {
		t.Fatalf("second AtomicReplace returned err: %v", err)
	}

	if bak1 == "" || bak2 == "" {
		t.Fatalf("expected non-empty backup paths, got %q and %q", bak1, bak2)
	}
	if bak1 == bak2 {
		t.Errorf("expected distinct backup paths across runs, got identical %q", bak1)
	}

	// Both bak files should still exist (production does not clean older bak files).
	if _, statErr := os.Stat(bak1); statErr != nil {
		t.Errorf("first backup %s missing: %v", bak1, statErr)
	}
	if _, statErr := os.Stat(bak2); statErr != nil {
		t.Errorf("second backup %s missing: %v", bak2, statErr)
	}

	// Suffix (last segment after the final ".") must differ.
	suffix1 := bak1[strings.LastIndex(bak1, ".")+1:]
	suffix2 := bak2[strings.LastIndex(bak2, ".")+1:]
	if suffix1 == suffix2 {
		t.Errorf("expected distinct random suffix between runs, got %q and %q", suffix1, suffix2)
	}
}

// TestAtomicReplace_BackupContainsPreMutationContent (TC-MUTATION-1 detail).
func TestAtomicReplace_BackupContainsPreMutationContent(t *testing.T) {
	dir := t.TempDir()
	original := minimalCompose
	path := writeFile(t, dir, "compose.yml", original, 0o644)
	preHash := sha256.Sum256([]byte(original))
	preHex := hex.EncodeToString(preHash[:])

	backupPath, err := AtomicReplace(path, func(in []byte) ([]byte, error) {
		return bytes.ReplaceAll(in, []byte("a:1"), []byte("a:2")), nil
	})
	if err != nil {
		t.Fatalf("AtomicReplace returned err: %v", err)
	}
	if backupPath == "" {
		t.Fatalf("expected non-empty backupPath")
	}

	got := sha256Hex(t, backupPath)
	if got != preHex {
		t.Errorf("backup SHA-256 = %s, want pre-mutation %s", got, preHex)
	}
}

// TestAtomicReplace_ReturnsBackupPath (TC-MUTATION-1 detail).
func TestAtomicReplace_ReturnsBackupPath(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "compose.yml", minimalCompose, 0o644)

	backupPath, err := AtomicReplace(path, func(in []byte) ([]byte, error) {
		return bytes.ReplaceAll(in, []byte("a:1"), []byte("a:2")), nil
	})
	if err != nil {
		t.Fatalf("AtomicReplace returned err: %v", err)
	}
	if backupPath == "" {
		t.Errorf("expected non-empty backupPath")
	}
	if !filepath.IsAbs(backupPath) {
		t.Errorf("expected absolute backupPath, got %q", backupPath)
	}
	if _, statErr := os.Stat(backupPath); statErr != nil {
		t.Errorf("backup path %q does not exist: %v", backupPath, statErr)
	}
	gotDir := filepath.Dir(backupPath)
	wantDir := filepath.Dir(path)
	if gotDir != wantDir {
		t.Errorf("backup dir = %q, want same dir as original %q", gotDir, wantDir)
	}
}
