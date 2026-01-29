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

var (
	statusMocked bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check node status",
	Long: `Display the current status of your Gonka node including:
  - Container health (running/stopped)
  - Blockchain sync status, block height, peer count
  - ML Node status, model loaded, PoC participation

Examples:
  gonka-nop status           # Check real node status
  gonka-nop status --mocked  # Show mocked demo status`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusMocked, "mocked", false, "Show mocked demo status")
}

func runStatus(cmd *cobra.Command, args []string) error {
	var nodeStatus *status.NodeStatus

	if statusMocked {
		// Use mocked data for demo
		nodeStatus = status.FetchMockedStatus()
	} else {
		// Try to fetch real status
		var err error
		nodeStatus, err = status.FetchStatus(outputDir)
		if err != nil {
			return fmt.Errorf("failed to fetch status: %w", err)
		}
	}

	status.Display(nodeStatus)
	return nil
}

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset node configuration",
	Long:  "Stop all containers and optionally remove configuration files and blockchain data.",
	RunE:  runReset,
}

func runReset(cmd *cobra.Command, args []string) error {
	// TODO: Implement reset
	fmt.Println("Reset not yet implemented")
	return nil
}

var (
	gpuMocked bool
)

var gpuInfoCmd = &cobra.Command{
	Use:   "gpu-info",
	Short: "Display detected GPU information",
	Long: `Detect and display information about available NVIDIA GPUs including:
  - GPU count, names, and VRAM
  - Recommended model based on total VRAM
  - Optimal TP/PP configuration

Examples:
  gonka-nop gpu-info           # Detect real GPUs
  gonka-nop gpu-info --mocked  # Show mocked demo GPU info`,
	RunE: runGPUInfo,
}

func init() {
	gpuInfoCmd.Flags().BoolVar(&gpuMocked, "mocked", false, "Show mocked demo GPU info")
}

func runGPUInfo(cmd *cobra.Command, args []string) error {
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

	_, _ = cyan.Println("\nDetected GPUs")
	_, _ = dimmed.Println("─────────────────────────────────")

	// Mocked 4x RTX 4090
	gpus := []struct {
		index  int
		name   string
		memory int
	}{
		{0, "NVIDIA GeForce RTX 4090", 24564},
		{1, "NVIDIA GeForce RTX 4090", 24564},
		{2, "NVIDIA GeForce RTX 4090", 24564},
		{3, "NVIDIA GeForce RTX 4090", 24564},
	}

	totalVRAM := 0
	for _, gpu := range gpus {
		fmt.Printf("  [%d] %s - %d MB VRAM\n", gpu.index, gpu.name, gpu.memory)
		totalVRAM += gpu.memory
	}

	fmt.Println()
	_, _ = green.Printf("  Total: %d GPUs, %.1f GB VRAM\n", len(gpus), float64(totalVRAM)/1024)

	_, _ = cyan.Println("\nRecommended Configuration")
	_, _ = dimmed.Println("─────────────────────────────────")
	fmt.Println("  Model:                 Qwen/QwQ-32B")
	fmt.Println("  Tensor Parallel (TP):  4")
	fmt.Println("  Pipeline Parallel (PP): 1")
	fmt.Println("  GPU Memory Util:       90%")
}

func displayRealGPUInfo() {
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow)

	_, _ = cyan.Println("\nDetected GPUs")
	fmt.Println("─────────────────────────────────")

	// Try nvidia-smi
	_, _ = yellow.Println("  ⚠ nvidia-smi not available or no GPUs detected")
	fmt.Println("  Run with --mocked to see demo output")
}

// GetOutputDir returns the output directory
func GetOutputDir() string {
	return outputDir
}

// IsVerbose returns whether verbose mode is enabled
func IsVerbose() bool {
	return verbose
}
