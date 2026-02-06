package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/inc4/gonka-nop/internal/status"
	"github.com/spf13/cobra"
)

var adminURL string

func init() {
	mlNodeCmd.PersistentFlags().StringVar(&adminURL, "admin-url", "http://localhost:9200", "Admin API URL")
	mlNodeCmd.AddCommand(mlNodeListCmd)
	mlNodeCmd.AddCommand(mlNodeStatusCmd)
	mlNodeCmd.AddCommand(mlNodeEnableCmd)
	mlNodeCmd.AddCommand(mlNodeDisableCmd)
}

var mlNodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured ML nodes",
	RunE:  runMLNodeList,
}

var mlNodeStatusCmd = &cobra.Command{
	Use:   "status [node-id]",
	Short: "Show detailed ML node status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMLNodeStatus,
}

var mlNodeEnableCmd = &cobra.Command{
	Use:   "enable [node-id]",
	Short: "Enable an ML node for PoC/inference",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMLNodeEnable,
}

var mlNodeDisableCmd = &cobra.Command{
	Use:   "disable [node-id]",
	Short: "Disable an ML node",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMLNodeDisable,
}

func runMLNodeList(_ *cobra.Command, _ []string) error {
	entries, err := fetchAdminNodes(adminURL)
	if err != nil {
		return fmt.Errorf("failed to fetch ML nodes: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No ML nodes configured.")
		return nil
	}

	greenC := color.New(color.FgGreen)
	yellowC := color.New(color.FgYellow)
	redC := color.New(color.FgRed)
	boldC := color.New(color.Bold)
	dimC := color.New(color.Faint)

	_, _ = boldC.Println("\nML Nodes")
	_, _ = dimC.Println(strings.Repeat("─", 40))

	for _, e := range entries {
		// Node ID and model
		modelName := firstModelName(e.Node.Models)

		// Status color
		statusStr := e.State.CurrentStatus
		if statusStr == "" {
			statusStr = "UNKNOWN"
		}

		enabledStr := "disabled"
		if e.State.AdminState.Enabled {
			enabledStr = "enabled"
		}

		// Hardware
		hw := "N/A"
		if len(e.Node.Hardware) > 0 {
			hw = fmt.Sprintf("%dx %s", e.Node.Hardware[0].Count, e.Node.Hardware[0].Type)
		}

		fmt.Printf("\n  %-10s ", e.Node.ID)

		switch statusStr {
		case "INFERENCE":
			_, _ = greenC.Printf("%-12s", statusStr)
		case "POC":
			_, _ = greenC.Printf("%-12s", statusStr)
		case "FAILED":
			_, _ = redC.Printf("%-12s", statusStr)
		default:
			_, _ = yellowC.Printf("%-12s", statusStr)
		}

		if e.State.AdminState.Enabled {
			_, _ = greenC.Printf("  %-10s", enabledStr)
		} else {
			_, _ = yellowC.Printf("  %-10s", enabledStr)
		}

		// PoC weight
		pocWeight := 0
		for _, info := range e.State.EpochMLNodes {
			pocWeight = info.PoCWeight
			break
		}
		if pocWeight > 0 {
			fmt.Printf("  weight:%d", pocWeight)
		}

		fmt.Println()
		_, _ = dimC.Printf("             Model: %s\n", modelName)
		_, _ = dimC.Printf("             Hardware: %s\n", hw)

		if e.State.FailureReason != "" {
			_, _ = redC.Printf("             Failure: %s\n", e.State.FailureReason)
		}
	}

	fmt.Println()
	return nil
}

func runMLNodeStatus(_ *cobra.Command, args []string) error {
	entries, err := fetchAdminNodes(adminURL)
	if err != nil {
		return fmt.Errorf("failed to fetch ML nodes: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No ML nodes configured.")
		return nil
	}

	nodeID := "node1"
	if len(args) > 0 {
		nodeID = args[0]
	}

	var entry *status.AdminNodesEntry
	for i := range entries {
		if entries[i].Node.ID == nodeID {
			entry = &entries[i]
			break
		}
	}

	if entry == nil {
		available := make([]string, len(entries))
		for i, e := range entries {
			available[i] = e.Node.ID
		}
		return fmt.Errorf("node %q not found (available: %s)", nodeID, strings.Join(available, ", "))
	}

	printNodeDetail(entry)
	return nil
}

func runMLNodeEnable(_ *cobra.Command, args []string) error {
	nodeID := "node1"
	if len(args) > 0 {
		nodeID = args[0]
	}

	if err := postAdminAction(adminURL, nodeID, "enable"); err != nil {
		return fmt.Errorf("failed to enable node %q: %w", nodeID, err)
	}

	color.New(color.FgGreen).Printf("Node %q enabled successfully.\n", nodeID)
	return nil
}

func runMLNodeDisable(_ *cobra.Command, args []string) error {
	nodeID := "node1"
	if len(args) > 0 {
		nodeID = args[0]
	}

	if err := postAdminAction(adminURL, nodeID, "disable"); err != nil {
		return fmt.Errorf("failed to disable node %q: %w", nodeID, err)
	}

	color.New(color.FgYellow).Printf("Node %q disabled successfully.\n", nodeID)
	return nil
}

// --- Helpers ---

func fetchAdminNodes(baseURL string) ([]status.AdminNodesEntry, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(baseURL + "/admin/v1/nodes")
	if err != nil {
		return nil, fmt.Errorf("connecting to Admin API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Admin API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var entries []status.AdminNodesEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return entries, nil
}

func postAdminAction(baseURL, nodeID, action string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/admin/v1/nodes/%s/%s", baseURL, nodeID, action)
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("connecting to Admin API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Admin API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func firstModelName(models map[string]status.AdminModelConfig) string {
	for name := range models {
		return name
	}
	return "N/A"
}

func printNodeDetail(e *status.AdminNodesEntry) {
	boldC := color.New(color.Bold)
	dimC := color.New(color.Faint)
	greenC := color.New(color.FgGreen)
	yellowC := color.New(color.FgYellow)
	redC := color.New(color.FgRed)

	_, _ = boldC.Printf("\nML Node: %s\n", e.Node.ID)
	_, _ = dimC.Println(strings.Repeat("─", 40))

	// Connection
	fmt.Printf("  Host:           %s\n", e.Node.Host)
	fmt.Printf("  Inference Port: %d\n", e.Node.InferencePort)
	fmt.Printf("  PoC Port:       %d\n", e.Node.PoCPort)
	fmt.Printf("  Max Concurrent: %d\n", e.Node.MaxConcurrent)

	// Status
	fmt.Println()
	currentStr := e.State.CurrentStatus
	if currentStr == "" {
		currentStr = "UNKNOWN"
	}
	fmt.Print("  Status:         ")
	switch currentStr {
	case "INFERENCE":
		_, _ = greenC.Println(currentStr)
	case "POC":
		_, _ = greenC.Println(currentStr)
	case "FAILED":
		_, _ = redC.Println(currentStr)
	default:
		_, _ = yellowC.Println(currentStr)
	}

	// Intended vs current mismatch
	if e.State.IntendedStatus != "" && e.State.IntendedStatus != e.State.CurrentStatus {
		_, _ = yellowC.Printf("  Intended:       %s (transitioning)\n", e.State.IntendedStatus)
	}

	if e.State.PoCCurrentStatus != "" {
		fmt.Printf("  PoC Status:     %s\n", e.State.PoCCurrentStatus)
	}
	if e.State.PoCIntendedStatus != "" && e.State.PoCIntendedStatus != e.State.PoCCurrentStatus {
		fmt.Printf("  PoC Intended:   %s\n", e.State.PoCIntendedStatus)
	}

	if e.State.AdminState.Enabled {
		fmt.Print("  Enabled:        ")
		_, _ = greenC.Printf("Yes")
		if e.State.AdminState.Epoch > 0 {
			_, _ = dimC.Printf(" (since epoch %d)", e.State.AdminState.Epoch)
		}
		fmt.Println()
	} else {
		fmt.Print("  Enabled:        ")
		_, _ = yellowC.Println("No")
	}

	if e.State.FailureReason != "" {
		fmt.Print("  Failure:        ")
		_, _ = redC.Println(e.State.FailureReason)
	}

	// Status timestamp
	if e.State.StatusTimestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, e.State.StatusTimestamp); err == nil {
			ago := time.Since(t)
			agoStr := formatStatusAge(ago)
			if ago > 10*time.Minute {
				_, _ = yellowC.Printf("  Last Updated:   %s (stale)\n", agoStr)
			} else {
				_, _ = dimC.Printf("  Last Updated:   %s\n", agoStr)
			}
		}
	}

	// Model
	fmt.Println()
	for name, cfg := range e.Node.Models {
		_, _ = boldC.Printf("  Model: %s\n", name)
		if len(cfg.Args) > 0 {
			_, _ = dimC.Printf("    Args: %s\n", strings.Join(cfg.Args, " "))
		}
	}

	// Hardware
	if len(e.Node.Hardware) > 0 {
		fmt.Println()
		for _, hw := range e.Node.Hardware {
			fmt.Printf("  Hardware: %dx %s\n", hw.Count, hw.Type)
		}
	}

	// Epoch allocation (from epoch_ml_nodes)
	if len(e.State.EpochMLNodes) > 0 {
		fmt.Println()
		_, _ = boldC.Println("  Epoch Allocation")
		for model, info := range e.State.EpochMLNodes {
			fmt.Printf("    Model:     %s\n", model)
			fmt.Printf("    PoC Weight: %d\n", info.PoCWeight)
			if len(info.TimeslotAllocation) > 0 {
				allocated := 0
				for _, s := range info.TimeslotAllocation {
					if s {
						allocated++
					}
				}
				fmt.Printf("    Timeslots: %d/%d allocated\n", allocated, len(info.TimeslotAllocation))
			}
		}
	}

	fmt.Println()
}

func formatStatusAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}
