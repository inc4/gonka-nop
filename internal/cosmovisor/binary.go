// Package cosmovisor provides utilities for managing Cosmovisor upgrade
// directories: downloading release binaries, verifying their SHA-256
// integrity, extracting them from zip archives, and swapping the
// `current` symlink. Used by both the `repair` command (recovery from
// stuck nodes) and (in M11+) the `update --api-only` hotfix installer.
package cosmovisor

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// DownloadTimeout caps a single binary download (zip).
// Matches the previous repair-command timeout to preserve behavior across the
// M11 Slice 1 extraction; the upcoming Slice 5 installer uses an injected
// http.Client and a per-attempt context deadline instead.
const DownloadTimeout = 10 * time.Minute

// ReleaseAsset holds a downloadable binary from a GitHub release.
type ReleaseAsset struct {
	Name        string
	DownloadURL string
	SHA256      string
	Size        int64
}

// FindBinaryAsset finds an asset by name from the asset list.
// Returns nil when no asset matches.
func FindBinaryAsset(assets []ReleaseAsset, name string) *ReleaseAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

// DownloadAsset downloads a release asset to a temporary file and returns
// the temp file path. Caller is responsible for removing the temp file.
func DownloadAsset(ctx context.Context, asset ReleaseAsset) (string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, DownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: DownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "gonka-repair-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	if _, cpErr := io.Copy(tmpFile, resp.Body); cpErr != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write temp file: %w", cpErr)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("close temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

// VerifySHA256 verifies a file's SHA-256 hash matches the expected value.
// NOTE (M11 Slice 1): moved verbatim from cmd/repair.go. Slice 5 will replace
// the string comparison with `crypto/subtle.ConstantTimeCompare` over the
// hex-decoded byte slices (F8 fix in plan-m11.md Review Notes).
func VerifySHA256(filePath, expected string) error {
	f, err := os.Open(filePath) // #nosec G304 - trusted path from our temp file
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

// ExtractZipBinary extracts a named binary from a zip file to destPath.
// When useSudo is true, the extracted binary is moved into place via `sudo mv`
// (needed when destPath is a root-owned Cosmovisor upgrade directory).
func ExtractZipBinary(zipPath, destPath, binaryName string, useSudo bool) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		// Match the binary name (may be in root or subdirectory)
		if filepath.Base(f.Name) != binaryName {
			continue
		}

		rc, openErr := f.Open()
		if openErr != nil {
			return fmt.Errorf("open zip entry: %w", openErr)
		}

		// Extract to a temp file first, then move (handles sudo case)
		tmpFile, tmpErr := os.CreateTemp("", "gonka-repair-bin-*")
		if tmpErr != nil {
			_ = rc.Close()
			return fmt.Errorf("create temp binary: %w", tmpErr)
		}
		tmpPath := tmpFile.Name()

		// Limit extraction to 500MB to prevent decompression bombs.
		const maxBinarySize = 500 * 1024 * 1024
		_, cpErr := io.Copy(tmpFile, io.LimitReader(rc, maxBinarySize))
		_ = rc.Close()
		_ = tmpFile.Close()
		if cpErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("extract binary: %w", cpErr)
		}

		// Move to destination (may need sudo)
		if useSudo {
			moveCmd := exec.Command("sudo", "mv", tmpPath, destPath) // #nosec G204
			if out, mvErr := moveCmd.CombinedOutput(); mvErr != nil {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("move binary: %w\n%s", mvErr, string(out))
			}
		} else {
			if mvErr := os.Rename(tmpPath, destPath); mvErr != nil {
				// Fallback: copy if rename fails (cross-device)
				if cpErr2 := copyFile(tmpPath, destPath); cpErr2 != nil {
					_ = os.Remove(tmpPath)
					return fmt.Errorf("move binary: %w", cpErr2)
				}
				_ = os.Remove(tmpPath)
			}
		}
		return nil
	}

	return fmt.Errorf("binary %s not found in zip", binaryName)
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 - trusted path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst) // #nosec G304 - trusted path
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}
