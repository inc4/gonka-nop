package docker

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/inc4/gonka-nop/internal/cosmovisor"
)

// composeLockFile is the per-directory advisory flock file. Sibling to the
// file being mutated; one lock covers any file-level mutation in that dir.
// Higher-level transactional edits across files must coordinate externally.
const composeLockFile = ".nop.lock"

// AtomicReplace mutates the file at path by passing its current contents
// through transform, validates the result is valid YAML, writes a backup of
// the original, then atomically swaps the file via os.CreateTemp + os.Rename.
//
// Order: read → transform → yaml.Unmarshal validate → backup → tmp → rename.
// Backup is created AFTER transform succeeds so a failing transform leaves
// no .bak. YAML validation happens BEFORE rename so an invalid result
// produces no .bak and no rename. The original file is unchanged on any error.
//
// Concurrency: acquires <dir>/.nop.lock via cosmovisor.AcquireLock
// (non-blocking; on contention returns wrapped cosmovisor.ErrLockHeld
// — errors.Is(err, cosmovisor.ErrLockHeld) detects this). The lock
// is released when AtomicReplace returns.
//
// Path: must be absolute, clean, and free of NUL bytes. Source file mode
// is preserved on the new file; backup is always 0600 (may contain
// sensitive env values pass-through).
//
// Returns the backup path on success ("" + err on failure).
//
// transform MUST be a pure function over []byte; it may be invoked once and
// its result is otherwise discarded. Side effects are undefined behavior.
//
// TODO(M12.future): backup rotation. Operators currently must prune
// .bak.* files manually; tracked under UC-3-EDGE2.
func AtomicReplace(path string, transform func([]byte) ([]byte, error)) (backupPath string, err error) {
	if err := validatePath(path); err != nil {
		return "", err
	}

	dir := filepath.Dir(path)
	lockPath := filepath.Join(dir, composeLockFile)
	release, err := cosmovisor.AcquireLock(lockPath)
	if err != nil {
		return "", fmt.Errorf("atomic replace %s: another gonka-nop command is running (lock: %s): %w",
			path, lockPath, err)
	}
	defer func() { _ = release() }()

	srcInfo, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("atomic replace %s: stat: %w", path, err)
	}
	srcMode := srcInfo.Mode().Perm()

	original, err := os.ReadFile(path) // #nosec G304 - path validated above
	if err != nil {
		return "", fmt.Errorf("atomic replace %s: read: %w", path, err)
	}

	transformed, err := transform(original)
	if err != nil {
		return "", fmt.Errorf("atomic replace %s: transform: %w", path, err)
	}

	var dummy any
	if err := yaml.Unmarshal(transformed, &dummy); err != nil {
		return "", fmt.Errorf("atomic replace %s: post-condition YAML validity failed: %w", path, err)
	}

	backupPath, err = writeBackup(path, original)
	if err != nil {
		return "", fmt.Errorf("atomic replace %s: backup: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return "", fmt.Errorf("atomic replace %s: tmp create: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // idempotent: no-op after successful rename

	if _, err := tmp.Write(transformed); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("atomic replace %s: tmp write: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("atomic replace %s: tmp close: %w", path, err)
	}

	if err := os.Chmod(tmpPath, srcMode); err != nil {
		return "", fmt.Errorf("atomic replace %s: chmod tmp: %w", path, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("atomic replace %s: rename: %w", path, err)
	}

	return backupPath, nil
}

// validatePath rejects relative paths, paths containing "..", and paths
// containing NUL bytes. Mirrors the security pattern from M11 ValidateDeployDir.
func validatePath(path string) error {
	if strings.ContainsRune(path, 0) {
		return errors.New("path contains NUL byte")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}
	if filepath.Clean(path) != path {
		return fmt.Errorf("path must be clean (no .. or redundant separators): %s", path)
	}
	return nil
}

// writeBackup writes original to <path>.bak.<UTC-ISO-8601-compact>.<8-hex-rand>
// with mode 0600. The random suffix prevents collision when two mutations
// fire within the same second (defense in depth — the flock already
// serializes within a directory).
func writeBackup(path string, original []byte) (string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	suffix, err := randHex(4)
	if err != nil {
		return "", fmt.Errorf("random suffix: %w", err)
	}
	bakPath := fmt.Sprintf("%s.bak.%s.%s", path, ts, suffix)
	if err := os.WriteFile(bakPath, original, 0600); err != nil {
		return "", err
	}
	return bakPath, nil
}

// randHex returns n random bytes encoded as 2n hex characters.
func randHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
