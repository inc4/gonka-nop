package cmd

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

const (
	repairGHAPIBase       = "https://api.github.com/repos/gonka-ai/gonka/releases/tags/"
	repairGHListReleases  = "https://api.github.com/repos/gonka-ai/gonka/releases"
	repairHTTPTimeout     = 30 * time.Second
	repairDownloadTimeout = 10 * time.Minute
	repairNodeAsset       = "inferenced-amd64.zip"
	repairAPIAsset        = "decentralized-api-amd64.zip"
	repairRecoveryPoll    = 5 * time.Second
	repairRecoveryTimeout = 30 * time.Second
	defaultAdminURL       = "http://localhost:9200"
)

// repairGHDirectDownload is a var (not const) for test overriding.
var repairGHDirectDownload = "https://github.com/gonka-ai/gonka/releases/download/"

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Detect and fix stuck node issues",
	Long: `Detect and repair common node issues, primarily failed Cosmovisor upgrades.

The repair command diagnoses problems by checking container logs, filesystem
state, and Cosmovisor directory structure, then applies fixes automatically.

Detected issues:
  - Node in restart loop due to missing upgrade handler binary
  - Stale upgrade-info.json blocking node startup
  - Broken Cosmovisor symlinks

Repair actions:
  - Download correct binaries from GitHub releases (with SHA256 verification)
  - Place binaries in Cosmovisor upgrade directories
  - Remove stale upgrade-info.json
  - Fix Cosmovisor symlinks
  - Restart node container

Examples:
  gonka-nop repair              # Diagnose and fix
  gonka-nop repair --check      # Diagnose only, don't fix
  gonka-nop repair --force      # Skip confirmation prompt`,
	RunE: runRepair,
}

var (
	repairCheck    bool
	repairForce    bool
	repairAdminURL string
)

func init() {
	repairCmd.Flags().BoolVar(&repairCheck, "check", false, "Only diagnose, don't repair")
	repairCmd.Flags().BoolVar(&repairForce, "force", false, "Skip confirmation prompts")
	repairCmd.Flags().StringVar(&repairAdminURL, "admin-url", defaultAdminURL, "Admin API URL")
}

// Diagnosis represents a detected problem.
type Diagnosis struct {
	ID          string // "missing_upgrade_handler", "stale_upgrade_info", "broken_symlink"
	Severity    string // "critical", "warning"
	Description string
	FixAction   string
	UpgradeName string
}

// RepairPlan holds the diagnosis results and repair strategy.
type RepairPlan struct {
	Diagnoses   []Diagnosis
	UpgradeName string // from logs or admin API
	NeedsBinary bool   // whether binary download is required
}

// ReleaseAsset holds a downloadable binary from a GitHub release.
type ReleaseAsset struct {
	Name        string
	DownloadURL string
	SHA256      string
	Size        int64
}

// upgradeHandlerMissingRe matches the Cosmovisor upgrade handler panic message.
var upgradeHandlerMissingRe = regexp.MustCompile(
	`upgrade handler is missing for (\S+) upgrade plan`)

func runRepair(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	state, err := config.Load(outputDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	if state.OutputDir == "" {
		return fmt.Errorf("no deployment found in %s — run 'gonka-nop setup' first", outputDir)
	}

	// Diagnose
	ui.Info("Diagnosing node...")
	plan := diagnoseNode(ctx, state)

	if len(plan.Diagnoses) == 0 {
		ui.Success("No issues detected. Node appears healthy.")
		return nil
	}

	// Display diagnosis
	displayDiagnosis(plan)

	// --check: stop here
	if repairCheck {
		return nil
	}

	// Confirm
	if !repairForce {
		confirm, confirmErr := ui.Confirm("Apply repair?", true)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirm {
			ui.Info("Repair canceled.")
			return nil
		}
	}

	// Execute repair
	return executeRepair(ctx, state, plan)
}

// diagnoseNode runs all diagnostic checks and returns a repair plan.
func diagnoseNode(ctx context.Context, state *config.State) *RepairPlan {
	plan := &RepairPlan{}

	// 1. Check container logs for upgrade handler errors
	upgradeName, logErr := checkContainerLogs(ctx, state)
	if logErr != nil {
		ui.Detail("Could not check container logs: %v", logErr)
	}
	if upgradeName != "" {
		plan.Diagnoses = append(plan.Diagnoses, Diagnosis{
			ID:          "missing_upgrade_handler",
			Severity:    "critical",
			Description: fmt.Sprintf("Node in restart loop: upgrade handler is missing for %s", upgradeName),
			FixAction:   fmt.Sprintf("Download %s binaries and place in Cosmovisor upgrade directory", upgradeName),
			UpgradeName: upgradeName,
		})
		plan.UpgradeName = upgradeName

		// Check if binary already exists
		nodeBin := filepath.Join(state.OutputDir, ".inference", "cosmovisor", "upgrades", upgradeName, "bin", "inferenced")
		if _, statErr := os.Stat(nodeBin); os.IsNotExist(statErr) {
			plan.NeedsBinary = true
		}
	}

	// 2. Check for stale upgrade-info.json
	staleInfo := checkUpgradeInfo(ctx, state, resolveRepairAdmin(state))
	if staleInfo != nil {
		plan.Diagnoses = append(plan.Diagnoses, *staleInfo)
		if plan.UpgradeName == "" && staleInfo.UpgradeName != "" {
			plan.UpgradeName = staleInfo.UpgradeName
		}
	}

	// 3. Check Cosmovisor symlinks
	symlinkDiags := checkCosmovisorSymlinks(state)
	plan.Diagnoses = append(plan.Diagnoses, symlinkDiags...)

	return plan
}

// checkContainerLogs looks for upgrade handler errors in node container logs.
func checkContainerLogs(ctx context.Context, state *config.State) (string, error) {
	cc, err := docker.NewComposeClient(state)
	if err != nil {
		return "", fmt.Errorf("create compose client: %w", err)
	}

	// Use only first compose file (node is in docker-compose.yml)
	if len(cc.Files) > 1 {
		cc.Files = cc.Files[:1]
	}

	logs, err := cc.Logs(ctx, "node", 200)
	if err != nil {
		return "", fmt.Errorf("get node logs: %w", err)
	}

	return parseUpgradeHandlerError(logs), nil
}

// parseUpgradeHandlerError extracts the upgrade name from log output.
// Returns empty string if no upgrade handler error is found.
func parseUpgradeHandlerError(logOutput string) string {
	matches := upgradeHandlerMissingRe.FindAllStringSubmatch(logOutput, -1)
	if len(matches) == 0 {
		return ""
	}
	// Return the last match (most recent occurrence)
	return matches[len(matches)-1][1]
}

// upgradeInfoJSON maps the upgrade-info.json file.
type upgradeInfoJSON struct {
	Name   string `json:"name"`
	Height int64  `json:"height"`
}

// checkUpgradeInfo checks if upgrade-info.json exists and is stale.
func checkUpgradeInfo(ctx context.Context, state *config.State, adminURL string) *Diagnosis {
	infoPath := filepath.Join(state.OutputDir, ".inference", "data", "upgrade-info.json")

	data, err := readFileOptionalSudo(ctx, state, infoPath)
	if err != nil {
		return nil // file doesn't exist or unreadable — not an issue
	}

	var info upgradeInfoJSON
	if jsonErr := json.Unmarshal(data, &info); jsonErr != nil {
		return nil // malformed file — not actionable
	}

	if info.Name == "" {
		return nil
	}

	// Try to get current height to determine if upgrade is stale
	heightDesc := ""
	currentHeight := fetchCurrentHeight(ctx, adminURL)
	if currentHeight > 0 && info.Height > 0 && currentHeight > info.Height {
		heightDesc = fmt.Sprintf(" (upgrade height %d, current height %d)", info.Height, currentHeight)
	}

	return &Diagnosis{
		ID:          "stale_upgrade_info",
		Severity:    "warning",
		Description: fmt.Sprintf("upgrade-info.json exists for %s%s", info.Name, heightDesc),
		FixAction:   "Remove .inference/data/upgrade-info.json",
		UpgradeName: info.Name,
	}
}

// fetchCurrentHeight tries to get the current block height from Admin API config.
func fetchCurrentHeight(ctx context.Context, adminURL string) int64 {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, adminURL+"/admin/v1/config", nil)
	if err != nil {
		return 0
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var cfg struct {
		ChainHeight int64 `json:"chain_height"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&cfg); decErr != nil {
		return 0
	}
	return cfg.ChainHeight
}

// checkCosmovisorSymlinks verifies .inference/cosmovisor/current and .dapi/cosmovisor/current.
func checkCosmovisorSymlinks(state *config.State) []Diagnosis {
	services := []struct {
		dir  string
		name string
	}{
		{".inference", "node"},
		{".dapi", "api"},
	}

	var diags []Diagnosis
	for _, svc := range services {
		symlinkPath := filepath.Join(state.OutputDir, svc.dir, "cosmovisor", "current")

		target, err := os.Readlink(symlinkPath)
		if err != nil {
			continue // not a symlink or doesn't exist — skip
		}

		// Resolve relative to symlink's directory
		resolvedTarget := target
		if !filepath.IsAbs(target) {
			resolvedTarget = filepath.Join(filepath.Dir(symlinkPath), target)
		}

		if _, statErr := os.Stat(resolvedTarget); os.IsNotExist(statErr) {
			diags = append(diags, Diagnosis{
				ID:          "broken_symlink",
				Severity:    "critical",
				Description: fmt.Sprintf("Cosmovisor symlink for %s points to non-existent %s", svc.name, target),
				FixAction:   "Relink to the latest available upgrade directory",
			})
		}
	}
	return diags
}

// displayDiagnosis shows the diagnosis results to the user.
func displayDiagnosis(plan *RepairPlan) {
	boldC := color.New(color.Bold)
	redC := color.New(color.FgRed)
	yellowC := color.New(color.FgYellow)

	_, _ = boldC.Println("\nNode Diagnosis")
	fmt.Println(strings.Repeat("─", 60))

	for _, d := range plan.Diagnoses {
		switch d.Severity {
		case "critical":
			_, _ = redC.Printf("  [CRITICAL] ")
		default:
			_, _ = yellowC.Printf("  [WARNING]  ")
		}
		fmt.Println(d.Description)
		ui.Detail("  Fix: %s", d.FixAction)
		fmt.Println()
	}

	if plan.NeedsBinary && plan.UpgradeName != "" {
		ui.Info("Binaries will be downloaded from: github.com/gonka-ai/gonka/releases")
	}
}

// executeRepair applies the repair plan.
func executeRepair(ctx context.Context, state *config.State, plan *RepairPlan) error {
	// Stop node container
	ui.Info("Stopping node container...")
	if err := stopRepairNode(ctx, state); err != nil {
		ui.Warn("Could not stop node: %v", err)
		ui.Info("Proceeding anyway (container may already be stopped)")
	} else {
		ui.Success("Node container stopped")
	}

	// Download and place binaries if needed
	if plan.NeedsBinary && plan.UpgradeName != "" {
		if err := downloadAndPlaceBinaries(ctx, state, plan.UpgradeName); err != nil {
			return fmt.Errorf("binary download/placement failed: %w", err)
		}
	}

	// Remove stale upgrade-info.json
	for _, d := range plan.Diagnoses {
		if d.ID == "stale_upgrade_info" {
			if err := removeUpgradeInfo(ctx, state); err != nil {
				ui.Warn("Could not remove upgrade-info.json: %v", err)
			} else {
				ui.Success("Removed stale upgrade-info.json")
			}
		}
	}

	// Fix broken symlinks
	for _, d := range plan.Diagnoses {
		if d.ID == "broken_symlink" && plan.UpgradeName != "" {
			fixSymlinks(ctx, state, plan.UpgradeName)
		}
	}

	// Start node container
	ui.Info("Starting node container...")
	if err := startRepairNode(ctx, state); err != nil {
		return fmt.Errorf("failed to start node: %w", err)
	}
	ui.Success("Node container started")

	// Verify recovery
	verifyRecovery(ctx, state)

	ui.Success("Repair complete. Monitor with: gonka-nop status")
	return nil
}

// downloadAndPlaceBinaries downloads upgrade binaries from GitHub and places them
// in the correct Cosmovisor directories.
func downloadAndPlaceBinaries(ctx context.Context, state *config.State, upgradeName string) error {
	// Fetch release info from GitHub
	assets, err := fetchReleaseAssets(ctx, upgradeName)
	if err != nil {
		return fmt.Errorf("fetch release info: %w", err)
	}

	// Download and place each binary
	binaries := []struct {
		assetName string
		destDir   string // relative to state.OutputDir
		binName   string
	}{
		{repairNodeAsset, filepath.Join(".inference", "cosmovisor", "upgrades", upgradeName, "bin"), "inferenced"},
		{repairAPIAsset, filepath.Join(".dapi", "cosmovisor", "upgrades", upgradeName, "bin"), "decentralized-api"},
	}

	for _, bin := range binaries {
		asset := findBinaryAsset(assets, bin.assetName)
		if asset == nil {
			ui.Warn("Asset %s not found in release, skipping", bin.assetName)
			continue
		}

		// Download
		ui.Info("Downloading %s (%d MB)...", asset.Name, asset.Size/(1024*1024))
		tmpPath, dlErr := downloadAsset(ctx, *asset)
		if dlErr != nil {
			return fmt.Errorf("download %s: %w", asset.Name, dlErr)
		}
		defer func(p string) { _ = os.Remove(p) }(tmpPath)

		// Verify SHA256
		if asset.SHA256 != "" {
			if verErr := verifySHA256(tmpPath, asset.SHA256); verErr != nil {
				return fmt.Errorf("verify %s: %w", asset.Name, verErr)
			}
			ui.Success("SHA256 verified: %s...%s", asset.SHA256[:8], asset.SHA256[len(asset.SHA256)-8:])
		} else {
			ui.Warn("SHA256 not available for %s, skipping verification", asset.Name)
		}

		// Create directory and extract binary
		destDir := filepath.Join(state.OutputDir, bin.destDir)
		if err := runHostCmd(ctx, state.UseSudo, state.OutputDir,
			fmt.Sprintf("mkdir -p %s", shellQuote(destDir))); err != nil {
			return fmt.Errorf("create dir %s: %w", destDir, err)
		}

		destPath := filepath.Join(destDir, bin.binName)
		if err := extractZipBinary(tmpPath, destPath, bin.binName, state.UseSudo); err != nil {
			return fmt.Errorf("extract %s: %w", asset.Name, err)
		}

		// chmod +x
		if err := runHostCmd(ctx, state.UseSudo, state.OutputDir,
			fmt.Sprintf("chmod +x %s", shellQuote(destPath))); err != nil {
			return fmt.Errorf("chmod %s: %w", destPath, err)
		}

		ui.Success("%s -> %s", bin.binName, bin.destDir)
	}

	return nil
}

// fetchReleaseAssets queries GitHub API for release assets matching the upgrade name.
// Falls back to direct download URLs if the GitHub API is unreachable.
func fetchReleaseAssets(ctx context.Context, upgradeName string) ([]ReleaseAsset, error) {
	// Try exact tag match first: release/{upgradeName}
	releaseTag := "release/" + upgradeName
	assets, err := fetchReleaseByTag(ctx, releaseTag)
	if err == nil {
		return assets, nil
	}

	// Try with -post1 suffix
	assets, err = fetchReleaseByTag(ctx, releaseTag+"-post1")
	if err == nil {
		return assets, nil
	}

	// Try listing recent releases and find best match
	assets, listErr := fetchReleaseBestMatch(ctx, upgradeName)
	if listErr == nil {
		return assets, nil
	}

	// Fallback: construct direct download URLs without API (works when api.github.com
	// is blocked but github.com is reachable)
	ui.Warn("GitHub API unreachable, trying direct download URLs...")
	return buildDirectDownloadAssets(ctx, upgradeName)
}

// ghReleaseResp maps the GitHub API release response.
type ghReleaseResp struct {
	TagName string           `json:"tag_name"`
	Assets  []ghReleaseAsset `json:"assets"`
}

// ghReleaseAsset maps a single asset in a GitHub release.
type ghReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"` // "sha256:<hex>"
}

// fetchReleaseByTag fetches a specific release by tag.
func fetchReleaseByTag(ctx context.Context, tag string) ([]ReleaseAsset, error) {
	reqCtx, cancel := context.WithTimeout(ctx, repairHTTPTimeout)
	defer cancel()

	url := repairGHAPIBase + tag
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: repairHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("release %s not found", tag)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d for %s", resp.StatusCode, tag)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseGitHubReleaseAssets(body)
}

// parseGitHubReleaseAssets parses GitHub release JSON into ReleaseAsset list.
func parseGitHubReleaseAssets(body []byte) ([]ReleaseAsset, error) {
	var release ghReleaseResp
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("parse release JSON: %w", err)
	}

	var assets []ReleaseAsset
	for _, a := range release.Assets {
		assets = append(assets, ReleaseAsset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			SHA256:      parseDigest(a.Digest),
			Size:        a.Size,
		})
	}
	return assets, nil
}

// parseDigest extracts the hex hash from a "sha256:<hex>" digest string.
func parseDigest(digest string) string {
	if strings.HasPrefix(digest, "sha256:") {
		return strings.TrimPrefix(digest, "sha256:")
	}
	return digest
}

// fetchReleaseBestMatch lists recent releases and finds the best match.
func fetchReleaseBestMatch(ctx context.Context, upgradeName string) ([]ReleaseAsset, error) {
	reqCtx, cancel := context.WithTimeout(ctx, repairHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, repairGHListReleases+"?per_page=10", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: repairHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []ghReleaseResp
	if decErr := json.NewDecoder(resp.Body).Decode(&releases); decErr != nil {
		return nil, fmt.Errorf("parse releases: %w", decErr)
	}

	// Find best match: tag contains the upgrade name
	for _, r := range releases {
		if strings.Contains(r.TagName, upgradeName) {
			var assets []ReleaseAsset
			for _, a := range r.Assets {
				assets = append(assets, ReleaseAsset{
					Name:        a.Name,
					DownloadURL: a.BrowserDownloadURL,
					SHA256:      parseDigest(a.Digest),
					Size:        a.Size,
				})
			}
			ui.Detail("Found matching release: %s", r.TagName)
			return assets, nil
		}
	}

	// List available releases for user reference
	var tags []string
	for _, r := range releases {
		tags = append(tags, r.TagName)
	}
	return nil, fmt.Errorf("no release found matching %q. Recent releases: %s",
		upgradeName, strings.Join(tags, ", "))
}

// buildDirectDownloadAssets constructs download URLs without the GitHub API.
// Used when api.github.com is unreachable but github.com works.
// Tries candidate tag patterns and verifies via HEAD request.
func buildDirectDownloadAssets(ctx context.Context, upgradeName string) ([]ReleaseAsset, error) {
	candidates := upgradeNameToReleaseTags(upgradeName)

	client := &http.Client{
		Timeout: repairHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects for HEAD
		},
	}

	for _, tag := range candidates {
		// GitHub encodes "/" as "%2F" in download URLs
		encodedTag := strings.ReplaceAll(tag, "/", "%2F")
		baseURL := repairGHDirectDownload + encodedTag + "/"

		// Verify the release exists by checking one asset with HEAD
		testURL := baseURL + repairNodeAsset
		reqCtx, cancel := context.WithTimeout(ctx, repairHTTPTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, testURL, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req)
		cancel()
		if err != nil {
			continue
		}
		_ = resp.Body.Close()

		// GitHub returns 302 redirect for valid assets, 404 for missing
		if resp.StatusCode == http.StatusNotFound {
			continue
		}

		ui.Detail("Found release via direct URL: %s", tag)
		return []ReleaseAsset{
			{Name: repairNodeAsset, DownloadURL: baseURL + repairNodeAsset},
			{Name: repairAPIAsset, DownloadURL: baseURL + repairAPIAsset},
		}, nil
	}

	return nil, fmt.Errorf("could not find release for %q via direct download (tried %v)", upgradeName, candidates)
}

// findBinaryAsset finds an asset by name from the asset list.
func findBinaryAsset(assets []ReleaseAsset, name string) *ReleaseAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

// downloadAsset downloads a release asset to a temporary file.
func downloadAsset(ctx context.Context, asset ReleaseAsset) (string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, repairDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: repairDownloadTimeout}
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

// verifySHA256 verifies a file's SHA256 hash matches the expected value.
func verifySHA256(filePath, expected string) error {
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
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

// extractZipBinary extracts a named binary from a zip file to destPath.
func extractZipBinary(zipPath, destPath, binaryName string, useSudo bool) error {
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

// removeUpgradeInfo removes the stale upgrade-info.json file.
func removeUpgradeInfo(ctx context.Context, state *config.State) error {
	infoPath := filepath.Join(state.OutputDir, ".inference", "data", "upgrade-info.json")
	return runHostCmd(ctx, state.UseSudo, state.OutputDir,
		fmt.Sprintf("rm -f %s", shellQuote(infoPath)))
}

// fixSymlinks fixes broken Cosmovisor current symlinks for both node and api.
func fixSymlinks(ctx context.Context, state *config.State, upgradeName string) {
	services := []struct {
		dir  string
		name string
	}{
		{".inference", "node"},
		{".dapi", "api"},
	}

	for _, svc := range services {
		cosmoDir := filepath.Join(state.OutputDir, svc.dir, "cosmovisor")
		upgradeDir := filepath.Join(cosmoDir, "upgrades", upgradeName)

		// Check if upgrade directory exists
		if _, err := os.Stat(upgradeDir); os.IsNotExist(err) {
			continue
		}

		symlinkPath := filepath.Join(cosmoDir, "current")
		if err := fixCosmovisorSymlink(ctx, state, symlinkPath, upgradeDir); err != nil {
			ui.Warn("Could not fix %s symlink: %v", svc.name, err)
		} else {
			// Use relative path for display
			relTarget := filepath.Join("upgrades", upgradeName)
			ui.Success("Cosmovisor %s symlink -> %s", svc.name, relTarget)
		}
	}
}

// fixCosmovisorSymlink updates a Cosmovisor current symlink to point to target.
func fixCosmovisorSymlink(ctx context.Context, state *config.State, symlinkPath, target string) error {
	// Use relative symlink (Cosmovisor expects this)
	relTarget, err := filepath.Rel(filepath.Dir(symlinkPath), target)
	if err != nil {
		relTarget = target
	}

	return runHostCmd(ctx, state.UseSudo, state.OutputDir,
		fmt.Sprintf("ln -snf %s %s", shellQuote(relTarget), shellQuote(symlinkPath)))
}

// stopRepairNode stops the node container using only the first compose file.
func stopRepairNode(ctx context.Context, state *config.State) error {
	cc, err := docker.NewComposeClient(state)
	if err != nil {
		return err
	}
	// Node is in first compose file only
	if len(cc.Files) > 1 {
		cc.Files = cc.Files[:1]
	}

	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Use run() to execute "stop node" — but ComposeClient doesn't have Stop.
	// Use the underlying run method by constructing args.
	// Actually, let's just use exec directly.
	return composeStop(stopCtx, state, "node")
}

// composeStop runs docker compose stop <service>.
func composeStop(ctx context.Context, state *config.State, service string) error {
	files := state.ComposeFiles
	if len(files) == 0 {
		files = []string{"docker-compose.yml"}
	}
	// Use only first file for node operations
	if len(files) > 1 {
		files = files[:1]
	}

	cmdArgs := make([]string, 0, 1+2*len(files)+2)
	cmdArgs = append(cmdArgs, "compose")
	for _, f := range files {
		cmdArgs = append(cmdArgs, "-f", f)
	}
	cmdArgs = append(cmdArgs, "stop", service)

	var cmd *exec.Cmd
	if state.UseSudo {
		sudoArgs := append([]string{"-E", "docker"}, cmdArgs...)
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...) // #nosec G204
	} else {
		cmd = exec.CommandContext(ctx, "docker", cmdArgs...) // #nosec G204
	}
	cmd.Dir = state.OutputDir

	// Load env for compose
	envFile := state.OutputDir + "/config.env"
	fileEnv, envErr := docker.ParseEnvFile(envFile)
	if envErr == nil && len(fileEnv) > 0 {
		cmd.Env = docker.MergeEnv(fileEnv)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose stop %s: %w\n%s", service, err, string(out))
	}
	return nil
}

// startRepairNode starts the node container.
func startRepairNode(ctx context.Context, state *config.State) error {
	cc, err := docker.NewComposeClient(state)
	if err != nil {
		return err
	}
	// Node is in first compose file
	if len(cc.Files) > 1 {
		cc.Files = cc.Files[:1]
	}
	return cc.Up(ctx, "node")
}

// verifyRecovery polls the RPC endpoint briefly to confirm the node is starting.
func verifyRecovery(ctx context.Context, state *config.State) {
	ui.Info("Verifying recovery...")

	rpcURL := state.RPCURL
	if rpcURL == "" {
		rpcURL = "http://localhost:26657"
	}

	verifyCtx, cancel := context.WithTimeout(ctx, repairRecoveryTimeout)
	defer cancel()

	ticker := time.NewTicker(repairRecoveryPoll)
	defer ticker.Stop()

	for {
		select {
		case <-verifyCtx.Done():
			ui.Warn("Recovery verification timed out — node may still be starting")
			ui.Detail("Check manually: gonka-nop status")
			return
		case <-ticker.C:
			syncStatus, err := docker.FetchSyncStatus(verifyCtx, rpcURL)
			if err != nil {
				continue // node still starting
			}
			if syncStatus.CatchingUp {
				ui.Success("Node is syncing (block %d)", syncStatus.LatestBlockHeight)
			} else {
				ui.Success("Node is running (block %d)", syncStatus.LatestBlockHeight)
			}
			return
		}
	}
}

// runHostCmd executes a shell command on the host, with sudo if needed.
func runHostCmd(ctx context.Context, useSudo bool, dir, shellCmd string) error {
	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.CommandContext(ctx, "sudo", "sh", "-c", shellCmd) // #nosec G204
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", shellCmd) // #nosec G204
	}
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", shellCmd, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// readFileOptionalSudo reads a file, falling back to sudo cat if needed.
func readFileOptionalSudo(ctx context.Context, state *config.State, path string) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 - path from trusted state
	if err == nil {
		return data, nil
	}

	if !state.UseSudo || !os.IsPermission(err) {
		return nil, err
	}

	// Fallback: sudo cat
	catCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(catCtx, "sudo", "cat", path) // #nosec G204
	return cmd.Output()
}

// shellQuote wraps a path in single quotes for shell safety.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// resolveRepairAdmin returns the admin URL from flag, state, or default.
func resolveRepairAdmin(state *config.State) string {
	if repairAdminURL != "" && repairAdminURL != defaultAdminURL {
		return repairAdminURL
	}
	if state.AdminURL != "" {
		return state.AdminURL
	}
	return defaultAdminURL
}

// upgradeNameToReleaseTags returns candidate GitHub release tags for an upgrade name.
func upgradeNameToReleaseTags(name string) []string {
	return []string{
		"release/" + name,
		"release/" + name + "-post1",
	}
}
