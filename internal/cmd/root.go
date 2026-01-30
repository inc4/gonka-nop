package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/inc4/gonka-nop/internal/status"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"

	// Global flags
	outputDir string
	verbose   bool
)

// SetVersionInfo sets the version information for the CLI
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	buildDate = d
}

var rootCmd = &cobra.Command{
	Use:   "gonka-nop",
	Short: "Gonka Node Onboarding Package",
	Long: `Gonka NOP simplifies the deployment of GPU-accelerated AI inference nodes
for the Gonka decentralized AI network.

It automatically detects your GPU hardware, configures the NVIDIA container
runtime, generates optimal configurations, and deploys all required services.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		printLogo()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output", "o", "./gonka-node", "Output directory for configuration files")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(gpuInfoCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(mlNodeCmd)
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func printLogo() {
	cyan := color.New(color.FgCyan, color.Bold)
	_, _ = cyan.Println(`
   ____             _           _   _  ___  ____
  / ___| ___  _ __ | | ____ _  | \ | |/ _ \|  _ \
 | |  _ / _ \| '_ \| |/ / _' | |  \| | | | | |_) |
 | |_| | (_) | | | |   < (_| | | |\  | |_| |  __/
  \____|\___/|_| |_|_|\_\__,_| |_| \_|\___/|_|

  Node Onboarding Package`)
	fmt.Println()
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gonka-nop %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", buildDate)
	},
}

// --- Status Command ---

var statusMocked bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check node status",
	Long: `Display the current status of your Gonka node including:
  - Container health (running/stopped)
  - Blockchain sync status, block height, block lag, peer count
  - Miss rate and epoch participation
  - ML Node status, model loaded, PoC participation
  - Security configuration (firewall, DDoS, ports)

Examples:
  gonka-nop status           # Check real node status
  gonka-nop status --mocked  # Show mocked demo status`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusMocked, "mocked", false, "Show mocked demo status")
}

func runStatus(cmd *cobra.Command, _ []string) error {
	var nodeStatus *status.NodeStatus

	if statusMocked {
		nodeStatus = status.FetchMockedStatus()
	} else {
		var err error
		nodeStatus, err = status.FetchStatus(outputDir)
		if err != nil {
			return fmt.Errorf("failed to fetch status: %w", err)
		}
	}

	status.Display(nodeStatus)
	return nil
}

// --- Reset Command ---

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset blockchain data while preserving keys",
	Long: `Stop all containers and reset blockchain data.
Preserves keys and configuration files by default.

This is useful when:
  - Your node is stuck or corrupted
  - You need to re-sync from scratch
  - You want to clear old blockchain data

Examples:
  gonka-nop reset             # Reset blockchain data, keep keys
  gonka-nop reset --all       # Reset everything (WARNING: deletes keys)`,
	RunE: runReset,
}

func runReset(_ *cobra.Command, _ []string) error {
	fmt.Println("Reset command — planned for M6 (Operations)")
	fmt.Println()
	fmt.Println("Will implement:")
	fmt.Println("  - Stop all containers")
	fmt.Println("  - Run unsafe-reset-all on blockchain data")
	fmt.Println("  - Preserve keys and config files")
	fmt.Println("  - Option to delete everything with --all")
	return nil
}

// --- GPU Info Command ---

var gpuMocked bool

var gpuInfoCmd = &cobra.Command{
	Use:   "gpu-info",
	Short: "Display detected GPU information",
	Long: `Detect and display information about available NVIDIA GPUs including:
  - GPU count, names, VRAM, architecture
  - NVLink/PCIe topology
  - Recommended model and TP/PP configuration
  - Memory utilization and KV cache dtype recommendations

Examples:
  gonka-nop gpu-info           # Detect real GPUs
  gonka-nop gpu-info --mocked  # Show mocked demo GPU info`,
	RunE: runGPUInfo,
}

func init() {
	gpuInfoCmd.Flags().BoolVar(&gpuMocked, "mocked", false, "Show mocked demo GPU info")
}

func runGPUInfo(_ *cobra.Command, _ []string) error {
	if gpuMocked {
		displayMockedGPUInfo()
	} else {
		displayRealGPUInfo()
	}
	return nil
}

func displayMockedGPUInfo() {
	cyan := color.New(color.FgCyan, color.Bold)
	green := color.New(color.FgGreen)
	dimmed := color.New(color.Faint)
	yellow := color.New(color.FgYellow)

	_, _ = cyan.Println("\nDetected GPUs")
	_, _ = dimmed.Println("─────────────────────────────────")

	// Mocked 4x RTX 4090
	gpus := []struct {
		index  int
		name   string
		memory int
		arch   string
		pci    string
	}{
		{0, "NVIDIA GeForce RTX 4090", 24564, "sm_89", "0000:01:00.0"},
		{1, "NVIDIA GeForce RTX 4090", 24564, "sm_89", "0000:02:00.0"},
		{2, "NVIDIA GeForce RTX 4090", 24564, "sm_89", "0000:03:00.0"},
		{3, "NVIDIA GeForce RTX 4090", 24564, "sm_89", "0000:04:00.0"},
	}

	totalVRAM := 0
	for _, gpu := range gpus {
		fmt.Printf("  [%d] %s - %d MB VRAM (arch: %s, PCI: %s)\n", gpu.index, gpu.name, gpu.memory, gpu.arch, gpu.pci)
		totalVRAM += gpu.memory
	}

	fmt.Println()
	_, _ = green.Printf("  Total: %d GPUs, %.1f GB VRAM\n", len(gpus), float64(totalVRAM)/1024)

	_, _ = cyan.Println("\nTopology")
	_, _ = dimmed.Println("─────────────────────────────────")
	_, _ = yellow.Println("  ⚠ PCIe 4.0 only — no NVLink")
	fmt.Println("  Multi-GPU inference may have higher latency from PCIe bottleneck")

	_, _ = cyan.Println("\nRecommended Configuration")
	_, _ = dimmed.Println("─────────────────────────────────")
	fmt.Println("  Model:                  Qwen/QwQ-32B")
	fmt.Println("  Tensor Parallel (TP):   4")
	fmt.Println("  Pipeline Parallel (PP): 1")
	fmt.Println("  GPU Memory Util:        0.92")
	fmt.Println("  Max Model Length:       32768")
	fmt.Println("  KV Cache Dtype:         auto")
	fmt.Println("  MLNode Image:           ghcr.io/product-science/mlnode:3.0.12")
	fmt.Println("  Attention Backend:      FLASH_ATTN")
}

func displayRealGPUInfo() {
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow)

	_, _ = cyan.Println("\nDetected GPUs")
	fmt.Println("─────────────────────────────────")

	_, _ = yellow.Println("  ⚠ nvidia-smi not available or no GPUs detected")
	fmt.Println("  Run with --mocked to see demo output")
}

// --- Update Command ---

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Safely update node containers",
	Long: `Perform a safe rolling update of node containers.

The update follows a safe rollout procedure:
  1. Check current timeslot allocation
  2. Disable ML node via Admin API
  3. Pull new container images
  4. Recreate containers
  5. Wait for model to reload
  6. Re-enable ML node
  7. Verify PoC participation

Examples:
  gonka-nop update                # Update all containers
  gonka-nop update --service mlnode  # Update ML node only
  gonka-nop update --dry-run      # Show what would be updated`,
	RunE: runUpdate,
}

func runUpdate(_ *cobra.Command, _ []string) error {
	fmt.Println("Update command — planned for M6 (Operations)")
	fmt.Println()
	fmt.Println("Safe rollout procedure:")
	fmt.Println("  1. Check timeslot_allocation (avoid updating during active slot)")
	fmt.Println("  2. Disable ML node: POST http://127.0.0.1:9200/admin/v1/nodes/:id/disable")
	fmt.Println("  3. Pull new images: docker compose pull")
	fmt.Println("  4. Recreate containers: docker compose up -d")
	fmt.Println("  5. Wait for model to load (health check loop)")
	fmt.Println("  6. Re-enable ML node: POST http://127.0.0.1:9200/admin/v1/nodes/:id/enable")
	fmt.Println("  7. Verify PoC participation resumes")
	return nil
}

// --- Cleanup Command ---

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Recover disk space",
	Long: `Clean up old data to recover disk space.

Actions:
  - Prune old Cosmovisor backups
  - Remove old Docker images
  - Clean blockchain WAL files
  - Report disk space recovered

Examples:
  gonka-nop cleanup              # Interactive cleanup
  gonka-nop cleanup --dry-run    # Show what would be cleaned`,
	RunE: runCleanup,
}

func runCleanup(_ *cobra.Command, _ []string) error {
	fmt.Println("Cleanup command — planned for M6 (Operations)")
	fmt.Println()
	fmt.Println("Will implement:")
	fmt.Println("  - Prune Cosmovisor upgrade backups (keep latest 2)")
	fmt.Println("  - Remove unused Docker images")
	fmt.Println("  - Clean blockchain WAL/tmp files")
	fmt.Println("  - Report disk space before/after")
	return nil
}

// --- ML Node Command ---

var mlNodeCmd = &cobra.Command{
	Use:   "ml-node",
	Short: "Manage ML nodes via Admin API",
	Long: `Manage ML nodes through the Admin API (http://127.0.0.1:9200).

Subcommands:
  list      - List all configured ML nodes
  add       - Register a new ML node
  enable    - Enable an ML node for PoC
  disable   - Disable an ML node
  status    - Show detailed ML node status

Examples:
  gonka-nop ml-node list
  gonka-nop ml-node add --host 127.0.0.1 --port 5050 --poc-port 8080
  gonka-nop ml-node enable <node-id>
  gonka-nop ml-node disable <node-id>
  gonka-nop ml-node status`,
	RunE: runMLNode,
}

func runMLNode(_ *cobra.Command, _ []string) error {
	fmt.Println("ML Node command — planned for M6 (Operations)")
	fmt.Println()
	fmt.Println("Admin API wrapper for:")
	fmt.Println("  GET    /admin/v1/nodes          — List nodes")
	fmt.Println("  POST   /admin/v1/nodes          — Add node")
	fmt.Println("  POST   /admin/v1/nodes/:id/enable  — Enable node")
	fmt.Println("  POST   /admin/v1/nodes/:id/disable — Disable node")
	fmt.Println()
	fmt.Println("Example (manual):")
	fmt.Println("  curl http://127.0.0.1:9200/admin/v1/nodes")
	return nil
}

// GetOutputDir returns the output directory
func GetOutputDir() string {
	return outputDir
}

// IsVerbose returns whether verbose mode is enabled
func IsVerbose() bool {
	return verbose
}
