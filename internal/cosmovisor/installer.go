package cosmovisor

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Defaults for installer configuration. Production builds use these via
// InstallOpts zero-values; tests override per case.
const (
	BinaryDownloadTimeout = 5 * time.Minute
	ManifestFetchTimeout  = 30 * time.Second
	HealthCheckTimeout    = 3 * time.Minute
	RestartGraceTimeout   = 90 * time.Second
	DownloadRetries       = 3

	// DefaultLockFileName is appended to <DeployDir>/.dapi/cosmovisor/
	// when InstallOpts.LockPath is empty. Same-dir as `current` symlink to
	// avoid /var/lock permission issues on non-root deploys.
	DefaultLockFileName = ".update.lock"
)

// versionPattern enforces a strict semver-ish format with optional pre-release
// suffix (e.g. "v0.2.12-api-post3"). Reject everything else BEFORE any
// filesystem write so an attacker can't probe paths via crafted versions.
var versionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z0-9.-]+)*$`)

// disallowedDeployRoots are filesystem hierarchies where the installer
// MUST refuse to operate even if DeployDir is otherwise valid.
var disallowedDeployRoots = []string{"/proc", "/sys", "/dev"}

// RestartFunc is invoked after the symlink swap to bounce the api container.
// Production passes a closure that runs `docker compose restart api`.
type RestartFunc func(ctx context.Context) error

// HealthCheckFunc is polled after restart until healthy or the deadline lapses.
// Production polls `<adminURL>/admin/v1/setup/report` and returns nil when the
// report includes the new binary version.
type HealthCheckFunc func(ctx context.Context) error

// SafetyCheckFunc is the bridge to chain.PoCSafetyChecker, kept as a function
// type here so internal/cosmovisor does not import internal/chain (preserves
// the module-boundary direction recommended by the architect).
type SafetyCheckFunc func(ctx context.Context) (safe bool, reason string, err error)

// RenameFunc is the seam for atomic-rename testing (TC-6.13 EXDEV). Production
// uses os.Rename; tests inject a stub that returns syscall.EXDEV on demand.
type RenameFunc func(oldpath, newpath string) error

// InstallOpts gathers all knobs for Install.
//
// Required for production use: Version, DeployDir. Everything else has a
// safe default; passing zero values leaves the installer in its production
// configuration. Test-only fields (HTTPClient, RenameFunc, ManifestURL,
// LockPath) are exposed publicly so tests in this package can inject without
// reflection — production code constructs InstallOpts from cmd/update_api.go
// and never sets them.
type InstallOpts struct {
	// Required.
	Version   string // e.g. "v0.2.12-api-post3"; must match versionPattern
	DeployDir string // absolute, no `..`, not under /proc /sys /dev

	// Sources.
	ManifestURL string // override; production uses DefaultManifestURL set by caller

	// Network / FS test seams.
	HTTPClient *http.Client
	RenameFunc RenameFunc
	LockPath   string

	// Safety + restart hooks.
	Safety          SafetyCheckFunc
	Force           bool // when true, SafetyCheck is NOT invoked at all (F20)
	RestartFunc     RestartFunc
	HealthCheckFunc HealthCheckFunc

	// Timeouts (override defaults).
	BinaryTimeout   time.Duration
	ManifestTimeout time.Duration
	HealthTimeout   time.Duration
}

// InstallResult summarizes a completed (or rolled-back) install.
type InstallResult struct {
	InstalledVersion string
	PreviousVersion  string // symlink target before swap; empty on first install
	RolledBack       bool   // true if the swap was reverted due to post-swap failure
}

// Installer is the orchestrator for Slice 5. Embeds no state; all
// inputs and seams arrive via InstallOpts so the same instance is safe
// for concurrent Install calls against different opts (though the flock
// gate makes that academic in practice).
type Installer struct {
	// BinaryName allows reusing the installer for inferenced hotfixes in the
	// future. Defaults to "decentralized-api".
	BinaryName string
	// ServiceDir under DeployDir, e.g. ".dapi" or ".inference".
	ServiceDir string
}

// NewAPIInstaller returns an Installer configured for the decentralized-api
// binary (Cosmovisor managed by the api container).
func NewAPIInstaller() *Installer {
	return &Installer{BinaryName: "decentralized-api", ServiceDir: ".dapi"}
}

// Install runs the full hotfix install pipeline. Returns the result on
// success (including RolledBack=true when post-swap recovery succeeded) and
// a non-nil error on hard failure.
//
// fragment the deliberately-sequential error paths covered by ~20 test cases.
//
//nolint:gocyclo // architect-approved single linear pipeline; splitting would
func (i *Installer) Install(ctx context.Context, opts InstallOpts) (*InstallResult, error) {
	// 1. Validate inputs BEFORE any I/O.
	if err := ValidateVersion(opts.Version); err != nil {
		return nil, err
	}
	if err := ValidateDeployDir(opts.DeployDir); err != nil {
		return nil, err
	}

	// 2. Safety gate (F20: Force skips the call entirely).
	if !opts.Force {
		if opts.Safety != nil {
			safe, reason, sErr := opts.Safety(ctx)
			if sErr != nil {
				return nil, fmt.Errorf("safety check error (fail-closed): %w", sErr)
			}
			if !safe {
				return nil, fmt.Errorf("unsafe to install during current chain phase: %s", reason)
			}
		}
	}

	// 3. Acquire flock to prevent concurrent installs.
	cosmoDir := filepath.Join(opts.DeployDir, i.ServiceDir, "cosmovisor")
	lockPath := opts.LockPath
	if lockPath == "" {
		lockPath = filepath.Join(cosmoDir, DefaultLockFileName)
	}
	// Ensure the cosmovisor directory exists so the lock file can be created
	// even on fresh installs (where the operator may not yet have an upgrade dir).
	if err := os.MkdirAll(cosmoDir, 0o750); err != nil {
		return nil, fmt.Errorf("ensure cosmovisor dir %s: %w", cosmoDir, err)
	}
	release, err := AcquireLock(lockPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	// 4. Fetch + parse manifest.
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = newTLSStrictClient(orDuration(opts.ManifestTimeout, ManifestFetchTimeout))
	}
	if opts.ManifestURL == "" {
		return nil, fmt.Errorf("manifest URL is required")
	}
	if err := ValidateReleaseURL(opts.ManifestURL); err != nil {
		// Same allowlist applies to manifest URL — fixes Review Notes F16.
		return nil, fmt.Errorf("manifest URL rejected by allowlist: %w", err)
	}
	manifestBytes, err := fetchURL(ctx, httpClient, opts.ManifestURL, orDuration(opts.ManifestTimeout, ManifestFetchTimeout))
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, err
	}
	release2, ok := manifest.FindRelease(opts.Version)
	if !ok {
		return nil, fmt.Errorf("version %q not present in manifest (known: %s)", opts.Version, summarizeVersions(manifest))
	}
	if err := ValidateReleaseURL(release2.BinaryURL); err != nil {
		return nil, fmt.Errorf("binary URL rejected by allowlist: %w", err)
	}

	// 5. Prepare upgrade directory + read PreviousVersion (capture string for rollback).
	upgradeDir := filepath.Join(cosmoDir, "upgrades", opts.Version)
	upgradeBinDir := filepath.Join(upgradeDir, "bin")
	if err := os.MkdirAll(upgradeBinDir, 0o750); err != nil {
		return nil, fmt.Errorf("ensure upgrade dir: %w", err)
	}
	currentSymlink := filepath.Join(cosmoDir, "current")
	previousTarget, _ := os.Readlink(currentSymlink) // empty on first install / missing

	// Short-circuit if `current` already points at this version (TC-6.18).
	if previousTarget == filepath.Join("upgrades", opts.Version) {
		return &InstallResult{
			InstalledVersion: opts.Version,
			PreviousVersion:  previousTarget,
		}, nil
	}

	// 6. Download + sha256 verify + atomic place.
	// Reuse the caller-injected client when present (preserves test seams +
	// any custom transport); otherwise build a TLS-strict client with the
	// binary-specific timeout.
	dlClient := httpClient
	if opts.HTTPClient == nil {
		dlClient = newTLSStrictClient(orDuration(opts.BinaryTimeout, BinaryDownloadTimeout))
	}
	destBinary := filepath.Join(upgradeBinDir, i.BinaryName)
	tmpBinary := destBinary + ".download"
	if err := downloadWithRetry(ctx, dlClient, release2.BinaryURL, tmpBinary, opts.BinaryTimeout); err != nil {
		_ = os.Remove(tmpBinary)
		return nil, fmt.Errorf("download binary: %w", err)
	}
	if err := VerifySHA256(tmpBinary, release2.SHA256); err != nil {
		_ = os.Remove(tmpBinary)
		return nil, fmt.Errorf("verify sha256: %w", err)
	}
	if err := os.Chmod(tmpBinary, 0o755); err != nil { // #nosec G302 - binary MUST be executable
		_ = os.Remove(tmpBinary)
		return nil, fmt.Errorf("chmod binary: %w", err)
	}
	renameFn := opts.RenameFunc
	if renameFn == nil {
		renameFn = os.Rename
	}
	if err := renameFn(tmpBinary, destBinary); err != nil {
		_ = os.Remove(tmpBinary)
		return nil, fmt.Errorf("place binary: %w", err)
	}

	// 7. Atomic symlink swap (same-dir sibling rename).
	relTarget := filepath.Join("upgrades", opts.Version)
	swapSymlink := filepath.Join(cosmoDir, ".current.swap")
	_ = os.Remove(swapSymlink) // cleanup any stale sibling
	if err := os.Symlink(relTarget, swapSymlink); err != nil {
		return nil, fmt.Errorf("create swap symlink: %w", err)
	}
	if err := renameFn(swapSymlink, currentSymlink); err != nil {
		_ = os.Remove(swapSymlink)
		return nil, fmt.Errorf("atomic symlink swap: %w", err)
	}

	// 8. Restart + health check; rollback on failure.
	result := &InstallResult{
		InstalledVersion: opts.Version,
		PreviousVersion:  previousTarget,
	}
	if opts.RestartFunc != nil {
		rCtx, cancel := context.WithTimeout(ctx, RestartGraceTimeout)
		restartErr := opts.RestartFunc(rCtx)
		cancel()
		if restartErr != nil {
			rb := rollback(currentSymlink, swapSymlink, previousTarget, renameFn, opts.RestartFunc, ctx)
			result.RolledBack = true
			if rb != nil {
				return result, fmt.Errorf("post-swap restart failed (%w); rollback ALSO failed (%v) — node is in inconsistent state", restartErr, rb)
			}
			return result, fmt.Errorf("post-swap restart failed: %w (rolled back to %s)", restartErr, previousTarget)
		}
	}
	if opts.HealthCheckFunc != nil {
		hCtx, cancel := context.WithTimeout(ctx, orDuration(opts.HealthTimeout, HealthCheckTimeout))
		healthErr := opts.HealthCheckFunc(hCtx)
		cancel()
		if healthErr != nil {
			rb := rollback(currentSymlink, swapSymlink, previousTarget, renameFn, opts.RestartFunc, ctx)
			result.RolledBack = true
			if rb != nil {
				return result, fmt.Errorf("post-swap health check failed (%w); rollback ALSO failed (%v) — node is in inconsistent state", healthErr, rb)
			}
			return result, fmt.Errorf("post-swap health check failed: %w (rolled back to %s)", healthErr, previousTarget)
		}
	}

	return result, nil
}

// ValidateVersion checks the version against versionPattern.
func ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version is required")
	}
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("version %q does not match required pattern (e.g. v0.2.12-api-post3)", version)
	}
	return nil
}

// ValidateDeployDir requires an absolute, clean path that does not reside
// inside /proc, /sys, or /dev (Review Notes F15).
func ValidateDeployDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("deploy dir is required")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("deploy dir %q must be absolute", dir)
	}
	cleaned := filepath.Clean(dir)
	if cleaned != dir {
		return fmt.Errorf("deploy dir %q is not clean (try %q)", dir, cleaned)
	}
	if strings.Contains(dir, "..") {
		return fmt.Errorf("deploy dir %q contains '..'", dir)
	}
	for _, root := range disallowedDeployRoots {
		if strings.HasPrefix(dir, root+"/") || dir == root {
			return fmt.Errorf("deploy dir %q is under disallowed root %s", dir, root)
		}
	}
	return nil
}

// rollback attempts to restore the previous symlink target via the same
// atomic-rename strategy as the forward swap. Restart is re-invoked if
// available to leave the api container running on the old binary.
//
// Returns an error only when BOTH the swap-back AND the restart fail. A
// successful swap-back with a failed restart is non-ideal but recoverable
// by the operator manually restarting the container.
func rollback(currentSymlink, _swapSymlink, previousTarget string,
	renameFn RenameFunc, restartFn RestartFunc, ctx context.Context) error {
	if previousTarget == "" {
		// Nothing to roll back to (this was a first install).
		return errors.New("no previous symlink target captured; cannot roll back")
	}
	swap := currentSymlink + ".rollback"
	_ = os.Remove(swap)
	if err := os.Symlink(previousTarget, swap); err != nil {
		return fmt.Errorf("create rollback symlink: %w", err)
	}
	if err := renameFn(swap, currentSymlink); err != nil {
		_ = os.Remove(swap)
		return fmt.Errorf("atomic rollback swap: %w", err)
	}
	if restartFn != nil {
		rCtx, cancel := context.WithTimeout(ctx, RestartGraceTimeout)
		defer cancel()
		if err := restartFn(rCtx); err != nil {
			return fmt.Errorf("rollback restart: %w", err)
		}
	}
	return nil
}

// downloadWithRetry attempts to download URL to dest up to DownloadRetries
// times with simple exponential backoff. Each attempt honors per-attempt
// timeout via context.WithTimeout. Aborts immediately on a non-retryable
// error (4xx, allowlist violation).
func downloadWithRetry(ctx context.Context, client *http.Client, url, dest string, perAttempt time.Duration) error {
	if perAttempt <= 0 {
		perAttempt = BinaryDownloadTimeout
	}
	var lastErr error
	for attempt := 1; attempt <= DownloadRetries; attempt++ {
		aCtx, cancel := context.WithTimeout(ctx, perAttempt)
		err := downloadOnce(aCtx, client, url, dest)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		// Non-retryable: 4xx is permanent.
		var permErr *httpStatusError
		if errors.As(err, &permErr) && permErr.code >= 400 && permErr.code < 500 {
			return err
		}
		// Backoff between attempts.
		if attempt < DownloadRetries {
			select {
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("download after %d retries: %w", DownloadRetries, lastErr)
}

// downloadOnce streams a URL to dest in a single attempt.
func downloadOnce(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{code: resp.StatusCode}
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 - dest constructed by installer
	if err != nil {
		return fmt.Errorf("open dest: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write dest: %w", err)
	}
	return nil
}

// fetchURL is the smaller cousin of downloadOnce for in-memory JSON manifests.
func fetchURL(ctx context.Context, client *http.Client, url string, timeout time.Duration) ([]byte, error) {
	rCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{code: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
}

// httpStatusError is a structured non-OK response error so downloadWithRetry
// can distinguish 4xx (permanent) from 5xx (retryable).
type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP status %d", e.code) }

// newTLSStrictClient returns an http.Client with TLS verification enabled
// and the given total timeout. Never use InsecureSkipVerify (NFR-M11-3).
func newTLSStrictClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
	}
}

func summarizeVersions(m *Manifest) string {
	out := make([]string, 0, len(m.Releases))
	for _, r := range m.Releases {
		out = append(out, r.Version)
	}
	return strings.Join(out, ", ")
}

func orDuration(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}
