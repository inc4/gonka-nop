package cosmovisor

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const testVersion = "v0.2.12-api-post3"

// ── Test fixture helpers ────────────────────────────────────────────────────

// installerHarness wires up an httptest TLS server hosting both the
// manifest and the binary, plus a tmp deploy dir, and returns an
// InstallOpts pre-populated with the test seams.
type installerHarness struct {
	t           *testing.T
	deployDir   string
	server      *httptest.Server
	manifestURL string
	binaryBytes []byte
	binarySHA   string
	restartCh   chan struct{}
	restartErr  error
	healthErr   error
	healthCalls int32
}

func newInstallerHarness(t *testing.T) *installerHarness {
	t.Helper()
	deployDir := t.TempDir()
	binaryBytes := []byte("#!/bin/sh\necho 'fake api binary v0.2.12-api-post3'\n")
	sum := sha256.Sum256(binaryBytes)
	sha := hex.EncodeToString(sum[:])

	h := &installerHarness{
		t:           t,
		deployDir:   deployDir,
		binaryBytes: binaryBytes,
		binarySHA:   sha,
		restartCh:   make(chan struct{}, 4),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		manifest := Manifest{
			SchemaVersion: 1,
			Releases: []ManifestRelease{
				{
					Version:   testVersion,
					BinaryURL: "", // filled in after server starts
					SHA256:    h.binarySHA,
					SizeBytes: int64(len(h.binaryBytes)),
				},
			},
		}
		// Use the server URL we know at request time.
		manifest.Releases[0].BinaryURL = h.binaryURL()
		_ = json.NewEncoder(w).Encode(manifest)
	})
	mux.HandleFunc("/binary.zip", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(h.binaryBytes)
	})
	h.server = httptest.NewTLSServer(mux)
	t.Cleanup(h.server.Close)
	h.manifestURL = h.server.URL + "/manifest.json"
	return h
}

func (h *installerHarness) binaryURL() string { return h.server.URL + "/binary.zip" }

func (h *installerHarness) client() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // #nosec G402 - test seam against httptest TLS
				MinVersion:         tls.VersionTLS12,
			},
		},
	}
}

// installerOptsForTest returns a fully-configured opts ready to drive Install.
// Tests override individual fields before calling Install.
func (h *installerHarness) installerOptsForTest() InstallOpts {
	return InstallOpts{
		Version:     testVersion,
		DeployDir:   h.deployDir,
		ManifestURL: h.manifestURL,
		HTTPClient:  h.client(),
		Safety:      func(_ context.Context) (bool, string, error) { return true, "ok", nil },
		RestartFunc: func(_ context.Context) error {
			h.restartCh <- struct{}{}
			return h.restartErr
		},
		HealthCheckFunc: func(_ context.Context) error {
			atomic.AddInt32(&h.healthCalls, 1)
			return h.healthErr
		},
	}
}

// installerOptsForTestAllowlist returns options where ManifestURL is the
// httptest URL — but the production path expects allowlisted URLs.
// For tests that exercise the install pipeline, we relax the allowlist by
// using the harness URL directly; tests that probe allowlist behavior use
// hand-crafted URL values instead.
func (h *installerHarness) installerOptsAllowAny(t *testing.T) InstallOpts {
	t.Helper()
	// Inject our httptest URL into the allowlist for the duration of the test.
	t.Cleanup(restoreAllowlist(allowedReleaseHostPrefixes))
	u, _ := url.Parse(h.manifestURL)
	allowedReleaseHostPrefixes = append(allowedReleaseHostPrefixes,
		struct{ host, pathPrefix string }{host: u.Host, pathPrefix: "/"},
	)
	return h.installerOptsForTest()
}

// restoreAllowlist returns a cleanup func that restores the package-level
// allowlist after a test mutates it.
func restoreAllowlist(orig []struct{ host, pathPrefix string }) func() {
	saved := make([]struct{ host, pathPrefix string }, len(orig))
	copy(saved, orig)
	return func() { allowedReleaseHostPrefixes = saved }
}

// ── Input validation ─────────────────────────────────────────────────────────

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid release", "v0.2.12", false},
		{"valid post-release", "v0.2.12-api-post3", false},
		{"valid rc", "v1.0.0-rc.1", false},
		{"empty", "", true},
		{"no v prefix", "0.2.12", true},
		{"path traversal", "../../etc/passwd", true},
		{"injection", "v0.2.12; rm -rf /", true},
		{"uppercase", "V0.2.12", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDeployDir(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{"absolute clean", "/GONKA/deploy", false},
		{"empty", "", true},
		{"relative", "deploy", true},
		{"contains ..", "/GONKA/../deploy", true},
		{"unclean trailing slash", "/GONKA/deploy/", true},
		{"under /proc", "/proc/sys/kernel", true},
		{"under /sys", "/sys/devices", true},
		{"under /dev", "/dev/shm", true},
		{"is /proc", "/proc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeployDir(tt.dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeployDir(%q) error = %v, wantErr %v", tt.dir, err, tt.wantErr)
			}
		})
	}
}

// ── Happy paths ─────────────────────────────────────────────────────────────

func TestInstall_HappyPath_NoPreviousSymlink(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)

	result, err := NewAPIInstaller().Install(context.Background(), opts)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.InstalledVersion != testVersion {
		t.Errorf("InstalledVersion = %q, want %q", result.InstalledVersion, testVersion)
	}
	if result.RolledBack {
		t.Error("RolledBack = true on happy path")
	}
	// Verify symlink + binary on disk.
	current := filepath.Join(h.deployDir, ".dapi", "cosmovisor", "current")
	target, _ := os.Readlink(current)
	if target != filepath.Join("upgrades", testVersion) {
		t.Errorf("symlink target = %q, want upgrades/%s", target, testVersion)
	}
	binary := filepath.Join(h.deployDir, ".dapi", "cosmovisor", target, "bin", "decentralized-api")
	info, statErr := os.Stat(binary)
	if statErr != nil {
		t.Fatalf("binary not found: %v", statErr)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("binary mode = %v, want 0755", info.Mode().Perm())
	}
	// Verify restart was called exactly once.
	select {
	case <-h.restartCh:
	default:
		t.Error("RestartFunc not called")
	}
	if atomic.LoadInt32(&h.healthCalls) != 1 {
		t.Errorf("HealthCheckFunc called %d times, want 1", h.healthCalls)
	}
}

func TestInstall_Idempotent_CurrentAlreadyTargetsVersion(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)

	// First install.
	if _, err := NewAPIInstaller().Install(context.Background(), opts); err != nil {
		t.Fatalf("first install: %v", err)
	}

	// Second install: should short-circuit, no restart.
	h.restartCh = make(chan struct{}, 4)
	prevHealthCalls := atomic.LoadInt32(&h.healthCalls)
	result, err := NewAPIInstaller().Install(context.Background(), opts)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if result.InstalledVersion != testVersion {
		t.Errorf("InstalledVersion = %q, want %q", result.InstalledVersion, testVersion)
	}
	select {
	case <-h.restartCh:
		t.Error("RestartFunc should not be called on idempotent reinstall")
	default:
	}
	if atomic.LoadInt32(&h.healthCalls) != prevHealthCalls {
		t.Errorf("HealthCheckFunc called on idempotent reinstall")
	}
}

// ── Allowlist / URL validation ───────────────────────────────────────────────

func TestInstall_RejectsManifestURLOffAllowlist(t *testing.T) {
	// Do NOT use installerOptsAllowAny — keep production allowlist intact.
	opts := InstallOpts{
		Version:     testVersion,
		DeployDir:   t.TempDir(),
		ManifestURL: "https://evil.com/manifest.json",
		Safety:      func(_ context.Context) (bool, string, error) { return true, "ok", nil },
	}
	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for off-allowlist manifest URL")
	}
	if !strings.Contains(err.Error(), "allowlist") {
		t.Errorf("error should mention allowlist: %v", err)
	}
}

// ── Safety gate ─────────────────────────────────────────────────────────────

func TestInstall_SafetyUnsafe_NoFiles(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	opts.Safety = func(_ context.Context) (bool, string, error) {
		return false, "PoC validation in progress", nil
	}
	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected safety-unsafe rejection")
	}
	if !strings.Contains(err.Error(), "PoC validation in progress") {
		t.Errorf("error should surface safety reason: %v", err)
	}
	// Verify no upgrade dir created (Install short-circuits BEFORE mkdir).
	upgradeDir := filepath.Join(h.deployDir, ".dapi", "cosmovisor", "upgrades", testVersion)
	if _, err := os.Stat(upgradeDir); !os.IsNotExist(err) {
		t.Errorf("upgrade dir should not exist after safety reject: %v", err)
	}
}

func TestInstall_ForceBypassesSafety(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	opts.Force = true
	// Safety would say unsafe, but Force should short-circuit it (not call at all).
	called := false
	opts.Safety = func(_ context.Context) (bool, string, error) {
		called = true
		return false, "should-not-be-called", nil
	}
	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err != nil {
		t.Fatalf("Install with Force: %v", err)
	}
	if called {
		t.Error("Safety check was invoked despite Force=true (F20: should be short-circuited)")
	}
}

// ── SHA mismatch ────────────────────────────────────────────────────────────

func TestInstall_SHA256Mismatch(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	// Replace harness binary with different content but keep sha as the old one.
	// We do this by serving altered bytes after the manifest is fetched.
	original := h.binaryBytes
	t.Cleanup(func() { h.binaryBytes = original })
	h.binaryBytes = []byte("tampered binary content")

	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected sha256 mismatch error")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error should mention sha256: %v", err)
	}
	// Verify symlink NOT swapped.
	current := filepath.Join(h.deployDir, ".dapi", "cosmovisor", "current")
	if _, err := os.Lstat(current); !os.IsNotExist(err) {
		t.Errorf("current symlink should not exist after sha256 failure: %v", err)
	}
}

// ── Rollback ────────────────────────────────────────────────────────────────

func TestInstall_RestartFails_Rollback(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	// Seed a previous symlink so rollback has somewhere to go.
	cosmoDir := filepath.Join(h.deployDir, ".dapi", "cosmovisor")
	_ = os.MkdirAll(filepath.Join(cosmoDir, "upgrades", "v0.2.12-api-post2", "bin"), 0o750)
	currentSymlink := filepath.Join(cosmoDir, "current")
	if err := os.Symlink(filepath.Join("upgrades", "v0.2.12-api-post2"), currentSymlink); err != nil {
		t.Fatalf("seed symlink: %v", err)
	}

	h.restartErr = errors.New("docker restart failed")
	defer func() { h.restartErr = nil }()

	result, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected post-swap failure error")
	}
	if result == nil || !result.RolledBack {
		t.Errorf("expected RolledBack=true; got result=%+v", result)
	}
	// Verify symlink restored to previous target.
	got, _ := os.Readlink(currentSymlink)
	if got != filepath.Join("upgrades", "v0.2.12-api-post2") {
		t.Errorf("symlink not rolled back: target = %q", got)
	}
	// Verify upgrade dir for new version IS preserved (operator can inspect).
	newUpgrade := filepath.Join(cosmoDir, "upgrades", testVersion)
	if _, statErr := os.Stat(newUpgrade); statErr != nil {
		t.Errorf("new upgrade dir should be preserved after rollback: %v", statErr)
	}
}

func TestInstall_HealthFails_Rollback(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	// Seed previous symlink.
	cosmoDir := filepath.Join(h.deployDir, ".dapi", "cosmovisor")
	_ = os.MkdirAll(filepath.Join(cosmoDir, "upgrades", "v0.2.12-api-post2", "bin"), 0o750)
	currentSymlink := filepath.Join(cosmoDir, "current")
	_ = os.Symlink(filepath.Join("upgrades", "v0.2.12-api-post2"), currentSymlink)

	h.healthErr = errors.New("health check 500")
	defer func() { h.healthErr = nil }()

	result, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected health-fail error")
	}
	if !result.RolledBack {
		t.Error("expected RolledBack=true")
	}
	got, _ := os.Readlink(currentSymlink)
	if got != filepath.Join("upgrades", "v0.2.12-api-post2") {
		t.Errorf("symlink not rolled back: target = %q", got)
	}
}

// ── EXDEV (atomic rename across filesystems) — RenameFunc seam ──────────────

func TestInstall_EXDEV_ViaRenameFunc(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	// Inject a RenameFunc that returns EXDEV on the *symlink swap* but
	// passes the binary placement. We detect the symlink case by the
	// presence of ".current.swap" in the path.
	opts.RenameFunc = func(oldpath, newpath string) error {
		if strings.Contains(oldpath, ".current.swap") {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
		}
		return os.Rename(oldpath, newpath)
	}
	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected EXDEV error")
	}
	if !strings.Contains(err.Error(), "rename") && !strings.Contains(err.Error(), "EXDEV") && !strings.Contains(err.Error(), "swap") {
		t.Errorf("error should reference rename / EXDEV / swap: %v", err)
	}
}

// ── Lock contention ──────────────────────────────────────────────────────────

func TestInstall_LockContention(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	// Pre-acquire the lock.
	cosmoDir := filepath.Join(h.deployDir, ".dapi", "cosmovisor")
	_ = os.MkdirAll(cosmoDir, 0o750)
	lockPath := filepath.Join(cosmoDir, DefaultLockFileName)
	release, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("pre-acquire lock: %v", err)
	}
	defer func() { _ = release() }()

	_, err = NewAPIInstaller().Install(context.Background(), opts)
	if !errors.Is(err, ErrLockHeld) {
		t.Errorf("expected ErrLockHeld, got %v", err)
	}
}

// ── Version not in manifest ──────────────────────────────────────────────────

func TestInstall_VersionNotInManifest(t *testing.T) {
	h := newInstallerHarness(t)
	opts := h.installerOptsAllowAny(t)
	opts.Version = "v9.9.9-nonexistent"

	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
	if !strings.Contains(err.Error(), "v9.9.9-nonexistent") {
		t.Errorf("error should mention requested version: %v", err)
	}
}

// ── Schema version mismatch ──────────────────────────────────────────────────

func TestInstall_ManifestSchemaVersion2_Rejected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version": 2, "releases": []}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	t.Cleanup(restoreAllowlist(allowedReleaseHostPrefixes))
	u, _ := url.Parse(srv.URL)
	allowedReleaseHostPrefixes = append(allowedReleaseHostPrefixes,
		struct{ host, pathPrefix string }{host: u.Host, pathPrefix: "/"})

	opts := InstallOpts{
		Version:     testVersion,
		DeployDir:   t.TempDir(),
		ManifestURL: srv.URL + "/manifest.json",
		Safety:      func(_ context.Context) (bool, string, error) { return true, "ok", nil },
		HTTPClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}}, // #nosec G402
		},
	}
	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected schema_version 2 rejection")
	}
	if !strings.Contains(err.Error(), "schema_version 2") {
		t.Errorf("error should cite schema_version 2: %v", err)
	}
}

// ── Constant-time sha256 guard ──────────────────────────────────────────────

func TestVerifySHA256_ConstantTimeCompare(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test")
	content := []byte("hello world")
	if err := os.WriteFile(f, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	correct := hex.EncodeToString(sum[:])

	if err := VerifySHA256(f, correct); err != nil {
		t.Errorf("correct hash: %v", err)
	}

	// Wrong but valid-length hex.
	bad := strings.Repeat("0", 64)
	if err := VerifySHA256(f, bad); err == nil {
		t.Error("expected mismatch error for wrong hash")
	}

	// Non-hex.
	if err := VerifySHA256(f, "not-hex-at-all"); err == nil {
		t.Error("expected error for non-hex hash")
	}

	// Short hex.
	if err := VerifySHA256(f, "abc"); err == nil {
		t.Error("expected error for short hash")
	}
}

// ── Backoff retry on transient 5xx ───────────────────────────────────────────

func TestInstall_TransientServerError_RetriedThenSucceeds(t *testing.T) {
	h := newInstallerHarness(t)
	// Replace binary handler to fail twice then succeed.
	var attempts int32
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		m := Manifest{
			SchemaVersion: 1,
			Releases: []ManifestRelease{
				{Version: testVersion, BinaryURL: h.server.URL + "/binary.zip", SHA256: h.binarySHA, SizeBytes: int64(len(h.binaryBytes))},
			},
		}
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/binary.zip", func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			http.Error(w, "transient", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(h.binaryBytes)
	})
	// Replace the harness's server.
	h.server.Close()
	h.server = httptest.NewTLSServer(mux)
	t.Cleanup(h.server.Close)
	h.manifestURL = h.server.URL + "/manifest.json"

	opts := h.installerOptsAllowAny(t)
	opts.ManifestURL = h.manifestURL // refresh from new server
	opts.BinaryTimeout = 2 * time.Second
	if _, err := NewAPIInstaller().Install(context.Background(), opts); err != nil {
		t.Fatalf("Install with 2x transient 500: %v", err)
	}
	if atomic.LoadInt32(&attempts) < 3 {
		t.Errorf("attempts = %d, want >=3 (2 transient + 1 success)", atomic.LoadInt32(&attempts))
	}
}

// ── 4xx is non-retryable ─────────────────────────────────────────────────────

func TestInstall_404OnBinary_Aborts(t *testing.T) {
	h := newInstallerHarness(t)
	var attempts int32
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		m := Manifest{
			SchemaVersion: 1,
			Releases: []ManifestRelease{
				{Version: testVersion, BinaryURL: h.server.URL + "/binary.zip", SHA256: h.binarySHA, SizeBytes: int64(len(h.binaryBytes))},
			},
		}
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/binary.zip", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "not found", http.StatusNotFound)
	})
	h.server.Close()
	h.server = httptest.NewTLSServer(mux)
	t.Cleanup(h.server.Close)
	h.manifestURL = h.server.URL + "/manifest.json"

	opts := h.installerOptsAllowAny(t)
	opts.ManifestURL = h.manifestURL
	opts.BinaryTimeout = 2 * time.Second

	_, err := NewAPIInstaller().Install(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	// Only ONE attempt — 4xx is non-retryable.
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts = %d, want 1 (4xx should not retry)", got)
	}
}

// Sentinel: keep the gosec G402 InsecureSkipVerify uses justifiable (#nosec).
var _ = fmt.Sprintf
