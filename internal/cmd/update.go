package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Safely update node containers",
	Long: `Fetch the latest container image versions from the Gonka GitHub repository
and apply updates with a safe rolling procedure.

For ML node updates, the command follows a safe rollout:
  1. Check current timeslot allocation
  2. Disable ML node via Admin API
  3. Update image tags in compose files
  4. Pull new container images
  5. Recreate containers
  6. Wait for model to reload
  7. Re-enable ML node

Node and API binaries are managed by Cosmovisor (auto-updated at upgrade blocks).

Examples:
  gonka-nop update                   # Update all containers
  gonka-nop update --check           # Show available updates without applying
  gonka-nop update --service mlnode   # Update ML node only
  gonka-nop update --service proxy    # Update proxy only`,
	RunE: runUpdate,
}

var (
	updateCheck    bool
	updateService  string
	updateAdminURL string
)

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Only check for available updates, don't apply")
	updateCmd.Flags().StringVar(&updateService, "service", "", "Update a specific service only (e.g. mlnode, proxy, node)")
	// adminURL flag is registered by mlnode.go on the ml-node command;
	// update command uses --admin-url as its own local flag.
	updateCmd.Flags().StringVar(&updateAdminURL, "admin-url", defaultAdminURL, "Admin API URL")
}

// VersionDiff represents a version change for a single service.
type VersionDiff struct {
	Service    string
	Current    string
	Latest     string
	HasUpdate  bool
	AutoUpdate bool // true for Cosmovisor-managed (node, api)
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	state, err := config.Load(outputDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if state.OutputDir == "" {
		return fmt.Errorf("no deployment found in %s — run 'gonka-nop setup' first", outputDir)
	}

	// Determine network type for fetching correct versions
	isTestnet := state.IsTestNet

	// 1. Read current versions from local compose files
	currentVersions, err := readLocalComposeVersions(state.OutputDir)
	if err != nil {
		return fmt.Errorf("failed to read current versions: %w", err)
	}

	// 2. Fetch latest versions from GitHub
	ui.Info("Fetching latest versions from GitHub...")
	latestVersions, fetchErr := config.FetchImageVersions(ctx, isTestnet)
	if fetchErr != nil {
		ui.Warn("Could not fetch from GitHub: %v", fetchErr)
		ui.Info("Using fallback versions")
	}
	network := "mainnet"
	if isTestnet {
		network = "testnet"
	}
	ui.Detail("Source: %s (%s)", latestVersions.Source, network)

	// 3. Compute diffs
	diffs := computeVersionDiffs(currentVersions, latestVersions)

	// Filter by service if specified
	if updateService != "" {
		diffs = filterDiffs(diffs, updateService)
		if len(diffs) == 0 {
			return fmt.Errorf("unknown service %q", updateService)
		}
	}

	// 4. Display version comparison
	displayVersionDiffs(diffs)

	updatable := filterUpdatable(diffs)
	if len(updatable) == 0 {
		ui.Success("All services are up to date.")
		return nil
	}

	// --check mode: just show and exit
	if updateCheck {
		return nil
	}

	// 5. Confirm before applying
	confirm, err := ui.Confirm(fmt.Sprintf("Apply %d update(s)?", len(updatable)), true)
	if err != nil {
		return err
	}
	if !confirm {
		ui.Info("Update canceled.")
		return nil
	}

	// 6. Apply updates
	return applyUpdates(ctx, state, updatable, latestVersions)
}

// readLocalComposeVersions reads image tags from local compose files.
func readLocalComposeVersions(dir string) (config.ImageVersions, error) {
	composePath := filepath.Join(dir, "docker-compose.yml")
	mlnodePath := filepath.Join(dir, "docker-compose.mlnode.yml")

	composeContent, err := os.ReadFile(composePath) // #nosec G304 - trusted path
	if err != nil {
		return config.ImageVersions{}, fmt.Errorf("read docker-compose.yml: %w", err)
	}

	var mlnodeContent []byte
	mlnodeContent, err = os.ReadFile(mlnodePath) // #nosec G304 - trusted path
	if err != nil {
		// mlnode compose may not exist (network-only setup)
		mlnodeContent = nil
	}

	versions, err := config.ParseComposeImageVersions(string(composeContent), string(mlnodeContent))
	if err != nil {
		return config.ImageVersions{}, fmt.Errorf("parse compose versions: %w", err)
	}
	versions.Source = "local"
	return versions, nil
}

// computeVersionDiffs compares current vs latest versions for all services.
func computeVersionDiffs(current, latest config.ImageVersions) []VersionDiff {
	return []VersionDiff{
		diffEntry("node", current.Node, latest.Node, true),
		diffEntry("api", current.API, latest.API, true),
		diffEntry("tmkms", current.TMKMS, latest.TMKMS, false),
		diffEntry("proxy", current.Proxy, latest.Proxy, false),
		diffEntry("proxy-ssl", current.ProxySSL, latest.ProxySSL, false),
		diffEntry("bridge", current.Bridge, latest.Bridge, false),
		diffEntry("mlnode", current.MLNode, latest.MLNode, false),
		diffEntry("nginx", current.Nginx, latest.Nginx, false),
		diffEntry("explorer", current.Explorer, latest.Explorer, false),
	}
}

func diffEntry(service, current, latest string, autoUpdate bool) VersionDiff {
	return VersionDiff{
		Service:    service,
		Current:    current,
		Latest:     latest,
		HasUpdate:  current != "" && latest != "" && current != latest,
		AutoUpdate: autoUpdate,
	}
}

func filterDiffs(diffs []VersionDiff, service string) []VersionDiff {
	var result []VersionDiff
	for _, d := range diffs {
		if strings.EqualFold(d.Service, service) {
			result = append(result, d)
		}
	}
	return result
}

func filterUpdatable(diffs []VersionDiff) []VersionDiff {
	var result []VersionDiff
	for _, d := range diffs {
		if d.HasUpdate && !d.AutoUpdate {
			result = append(result, d)
		}
	}
	return result
}

// displayVersionDiffs shows a table of current vs latest versions.
func displayVersionDiffs(diffs []VersionDiff) {
	boldC := color.New(color.Bold)
	greenC := color.New(color.FgGreen)
	yellowC := color.New(color.FgYellow)
	dimC := color.New(color.Faint)

	_, _ = boldC.Println("\nVersion Comparison")
	fmt.Println(strings.Repeat("─", 65))
	fmt.Printf("  %-12s %-22s %-22s %s\n", "Service", "Current", "Latest", "Status")
	fmt.Println(strings.Repeat("─", 65))

	for _, d := range diffs {
		if d.Current == "" && d.Latest == "" {
			continue
		}

		current := d.Current
		if current == "" {
			current = "(not installed)"
		}
		latest := d.Latest
		if latest == "" {
			latest = "(unknown)"
		}

		fmt.Printf("  %-12s %-22s %-22s ", d.Service, current, latest)

		switch {
		case d.HasUpdate && d.AutoUpdate:
			_, _ = dimC.Println("auto-update (Cosmovisor)")
		case d.HasUpdate:
			_, _ = yellowC.Println("UPDATE AVAILABLE")
		default:
			_, _ = greenC.Println("up to date")
		}
	}
	fmt.Println()
}

// applyUpdates performs the actual update for each service.
func applyUpdates(ctx context.Context, state *config.State, diffs []VersionDiff, latest config.ImageVersions) error {
	hasMLNode := false
	for _, d := range diffs {
		if d.Service == "mlnode" || d.Service == "nginx" {
			hasMLNode = true
			break
		}
	}

	// Safe MLNode rollout: disable → update compose → pull → recreate → wait → enable
	if hasMLNode {
		if err := safeMLNodeUpdate(ctx, state, latest); err != nil {
			return fmt.Errorf("ML node update failed: %w", err)
		}
	}

	// Update non-MLNode services
	nonMLDiffs := filterNonMLNode(diffs)
	if len(nonMLDiffs) > 0 {
		if err := updateComposeServices(ctx, state, nonMLDiffs); err != nil {
			return fmt.Errorf("service update failed: %w", err)
		}
	}

	// Update state with new versions
	state.Versions = latest
	if main := latest.MainImageVersion(); main != "" {
		state.ImageVersion = main
	}
	if err := state.Save(); err != nil {
		ui.Warn("Failed to save state: %v", err)
	}

	ui.Success("Update complete.")
	return nil
}

func filterNonMLNode(diffs []VersionDiff) []VersionDiff {
	var result []VersionDiff
	for _, d := range diffs {
		if d.Service != "mlnode" && d.Service != "nginx" {
			result = append(result, d)
		}
	}
	return result
}

// safeMLNodeUpdate performs a safe rolling update for the ML node:
// disable → update compose → pull → recreate → wait model load → enable
func safeMLNodeUpdate(ctx context.Context, state *config.State, latest config.ImageVersions) error {
	boldC := color.New(color.Bold)
	_, _ = boldC.Println("\nSafe ML Node Update")
	fmt.Println(strings.Repeat("─", 40))

	adminAPI := resolveUpdateAdminURL(state)
	nodeID := state.MLNodeID
	if nodeID == "" {
		nodeID = defaultNodeID
	}

	showTimeslotAllocation(adminAPI)
	disableMLNode(adminAPI, nodeID)

	if err := updateMLNodeComposeTags(state.OutputDir, latest); err != nil {
		return fmt.Errorf("update compose tags: %w", err)
	}

	if err := pullAndRecreateMLNode(ctx, state); err != nil {
		return err
	}

	waitForMLNodeReady(ctx, adminAPI)

	ui.Info("Re-enabling ML node %q...", nodeID)
	if err := postAdminAction(adminAPI, nodeID, "enable"); err != nil {
		return fmt.Errorf("failed to re-enable ML node: %w", err)
	}
	ui.Success("ML node re-enabled")
	return nil
}

func showTimeslotAllocation(adminAPI string) {
	ui.Info("Checking timeslot allocation...")
	entries, err := fetchAdminNodes(adminAPI)
	if err != nil {
		ui.Warn("Could not check timeslots: %v", err)
		return
	}
	if len(entries) == 0 {
		return
	}
	for _, info := range entries[0].State.EpochMLNodes {
		if len(info.TimeslotAllocation) > 0 {
			allocated := 0
			for _, s := range info.TimeslotAllocation {
				if s {
					allocated++
				}
			}
			ui.Detail("Timeslots: %d/%d allocated", allocated, len(info.TimeslotAllocation))
		}
	}
}

func disableMLNode(adminAPI, nodeID string) {
	ui.Info("Disabling ML node %q...", nodeID)
	if err := postAdminAction(adminAPI, nodeID, "disable"); err != nil {
		ui.Warn("Could not disable node: %v", err)
		ui.Info("Proceeding anyway (node may already be disabled)")
	} else {
		ui.Success("ML node disabled")
	}
}

func pullAndRecreateMLNode(ctx context.Context, state *config.State) error {
	ui.Info("Pulling new ML node images...")
	cc, err := docker.NewComposeClient(state)
	if err != nil {
		return fmt.Errorf("create compose client: %w", err)
	}

	pullCtx, pullCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer pullCancel()
	if err := cc.Pull(pullCtx); err != nil {
		return fmt.Errorf("pull images: %w", err)
	}

	ui.Info("Recreating ML node container...")
	if err := cc.Up(ctx, "mlnode-308"); err != nil {
		if err2 := cc.Up(ctx); err2 != nil {
			return fmt.Errorf("recreate containers: %w", err2)
		}
	}
	return nil
}

func waitForMLNodeReady(ctx context.Context, adminAPI string) {
	ui.Info("Waiting for model to load (this may take several minutes)...")
	loadCtx, loadCancel := context.WithTimeout(ctx, 15*time.Minute)
	defer loadCancel()

	err := docker.WaitForModelLoad(loadCtx, adminAPI, 10*time.Second,
		func(st *docker.MLNodeStatus) {
			ui.Detail("ML node status: %s", st.CurrentStatus)
		})
	if err != nil {
		ui.Warn("Model load wait timed out or failed: %v", err)
		ui.Info("You can check manually: gonka-nop ml-node status")
	} else {
		ui.Success("Model loaded successfully")
	}
}

// updateMLNodeComposeTags updates image tags in docker-compose.mlnode.yml.
func updateMLNodeComposeTags(dir string, latest config.ImageVersions) error {
	path := filepath.Join(dir, "docker-compose.mlnode.yml")
	content, err := os.ReadFile(path) // #nosec G304 - trusted path
	if err != nil {
		return fmt.Errorf("read compose file: %w", err)
	}

	updated := string(content)

	if latest.MLNode != "" {
		updated = replaceImageTag(updated, "ghcr.io/product-science/mlnode:", latest.MLNode)
	}
	if latest.Nginx != "" {
		updated = replaceImageTag(updated, "nginx:", latest.Nginx)
	}

	if updated == string(content) {
		ui.Detail("No changes needed in docker-compose.mlnode.yml")
		return nil
	}

	if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}
	ui.Detail("Updated: %s", path)
	return nil
}

// updateComposeServices updates non-MLNode services in docker-compose.yml.
func updateComposeServices(ctx context.Context, state *config.State, diffs []VersionDiff) error {
	boldC := color.New(color.Bold)
	_, _ = boldC.Println("\nUpdating Services")
	fmt.Println(strings.Repeat("─", 40))

	// Update image tags in docker-compose.yml
	path := filepath.Join(state.OutputDir, "docker-compose.yml")
	content, err := os.ReadFile(path) // #nosec G304 - trusted path
	if err != nil {
		return fmt.Errorf("read compose file: %w", err)
	}

	updated := string(content)
	for _, d := range diffs {
		imageName := serviceToImageName(d.Service)
		if imageName != "" && d.Latest != "" {
			updated = replaceImageTag(updated, imageName, d.Latest)
		}
	}

	if updated != string(content) {
		if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
			return fmt.Errorf("write compose file: %w", err)
		}
		ui.Detail("Updated: %s", path)
	}

	// Pull and recreate
	cc, err := docker.NewComposeClient(state)
	if err != nil {
		return fmt.Errorf("create compose client: %w", err)
	}

	ui.Info("Pulling updated images...")
	pullCtx, pullCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer pullCancel()
	if err := cc.Pull(pullCtx); err != nil {
		return fmt.Errorf("pull images: %w", err)
	}

	ui.Info("Recreating updated containers...")
	if err := cc.Up(ctx); err != nil {
		return fmt.Errorf("recreate containers: %w", err)
	}

	for _, d := range diffs {
		ui.Success("Updated %s: %s -> %s", d.Service, d.Current, d.Latest)
	}

	return nil
}

// replaceImageTag replaces an image tag in compose file content.
// prefix is like "ghcr.io/product-science/mlnode:" or "nginx:".
// Only replaces uncommented image lines.
func replaceImageTag(content, prefix, newTag string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Find lines with the image reference
		idx := strings.Index(line, prefix)
		if idx < 0 {
			continue
		}
		// Replace everything after the prefix up to the next whitespace or end of line
		afterPrefix := line[idx+len(prefix):]
		endIdx := strings.IndexAny(afterPrefix, " \t\n\r")
		if endIdx < 0 {
			lines[i] = line[:idx+len(prefix)] + newTag
		} else {
			lines[i] = line[:idx+len(prefix)] + newTag + afterPrefix[endIdx:]
		}
	}
	return strings.Join(lines, "\n")
}

// serviceToImageName maps service names to their image prefix in compose files.
func serviceToImageName(service string) string {
	switch service {
	case "tmkms":
		return "ghcr.io/product-science/tmkms-softsign-with-keygen:"
	case "proxy":
		return "ghcr.io/product-science/proxy:"
	case "proxy-ssl":
		return "ghcr.io/product-science/proxy-ssl:"
	case "bridge":
		return "ghcr.io/product-science/bridge:"
	case "explorer":
		return "ghcr.io/product-science/explorer:"
	default:
		return ""
	}
}

func resolveUpdateAdminURL(state *config.State) string {
	if updateAdminURL != "" && updateAdminURL != defaultAdminURL {
		return updateAdminURL
	}
	if state.AdminURL != "" {
		return state.AdminURL
	}
	return defaultAdminURL
}
