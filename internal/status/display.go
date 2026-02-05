package status

import (
	"fmt"
	"time"

	"github.com/fatih/color"
)

var (
	cyan   = color.New(color.FgCyan, color.Bold)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	red    = color.New(color.FgRed)
	bold   = color.New(color.Bold)
	dimmed = color.New(color.Faint)
)

// Display prints the status in a formatted way
func Display(s *NodeStatus) {
	printHeader("Gonka Node Status")

	printOverview(s)
	printBlockchain(s)
	printEpoch(s)
	printMLNode(s)
	printSecurity(s)
}

func printHeader(title string) {
	fmt.Println()
	_, _ = cyan.Println(title)
	_, _ = dimmed.Println(repeat("─", len(title)+10))
}

func printSection(title string) {
	fmt.Println()
	_, _ = bold.Println(title)
}

func printOverview(s *NodeStatus) {
	printSection("Overview")

	// Setup report summary (primary health indicator)
	if s.Overview.ChecksTotal > 0 {
		if s.Overview.OverallStatus == StatusPass {
			printOK("Health: %d/%d checks passed", s.Overview.ChecksPassed, s.Overview.ChecksTotal)
		} else {
			printFail("Health: %d/%d checks passed", s.Overview.ChecksPassed, s.Overview.ChecksTotal)
			for _, issue := range s.Overview.Issues {
				printInfo("Issue", "%s", issue)
			}
		}
	}

	// Containers
	if s.Overview.ContainersRunning == s.Overview.ContainersTotal {
		printOK("Containers: %d/%d running", s.Overview.ContainersRunning, s.Overview.ContainersTotal)
	} else if s.Overview.ContainersRunning > 0 {
		printWarn("Containers: %d/%d running", s.Overview.ContainersRunning, s.Overview.ContainersTotal)
	} else {
		printFail("Containers: %d/%d running", s.Overview.ContainersRunning, s.Overview.ContainersTotal)
	}

	// Registration
	if s.Overview.NodeRegistered {
		addr := s.Overview.NodeAddress
		if len(addr) > 20 {
			addr = addr[:10] + "..." + addr[len(addr)-6:]
		}
		printOK("Node registered: %s", addr)
	} else {
		printFail("Node not registered")
	}

	// Epoch participation
	if s.Overview.EpochActive {
		printOK("Epoch %d: Active (weight: %d)", s.Overview.EpochNumber, s.Overview.EpochWeight)
	} else if s.Overview.EpochNumber > 0 {
		printWarn("Epoch %d: Not active", s.Overview.EpochNumber)
	} else {
		printWarn("Epoch participation: Inactive")
	}
}

func printBlockchain(s *NodeStatus) {
	printSection("Blockchain")

	// Block height with lag
	if s.Blockchain.BlockLag > 0 && s.Blockchain.BlockLag <= 10 {
		printOK("Block height: %s (lag: %d blocks)", formatNumber(s.Blockchain.BlockHeight), s.Blockchain.BlockLag)
	} else if s.Blockchain.BlockLag > 10 {
		printWarn("Block height: %s (lag: %d blocks — falling behind)", formatNumber(s.Blockchain.BlockHeight), s.Blockchain.BlockLag)
	} else {
		printInfo("Block height", "%s", formatNumber(s.Blockchain.BlockHeight))
	}

	// Last block time
	if !s.Blockchain.LastBlockTime.IsZero() {
		ago := formatDuration(time.Since(s.Blockchain.LastBlockTime))
		if time.Since(s.Blockchain.LastBlockTime) < 30*time.Second {
			printOK("Last block: %s ago", ago)
		} else {
			printWarn("Last block: %s ago (stale)", ago)
		}
	}

	// Sync status
	if s.Blockchain.Synced {
		printOK("Sync status: Synced")
	} else if s.Blockchain.CatchingUp {
		printWarn("Sync status: Syncing (catching up)")
	} else {
		printFail("Sync status: Not synced")
	}

	// Peers
	if !s.Blockchain.PeerCountKnown {
		printInfo("Peers", "Unknown (RPC not accessible)")
	} else if s.Blockchain.PeerCount >= 5 {
		printOK("Peers: %d connected", s.Blockchain.PeerCount)
	} else if s.Blockchain.PeerCount > 0 {
		printWarn("Peers: %d connected (low)", s.Blockchain.PeerCount)
	} else {
		printFail("Peers: 0 connected")
	}

	// Validator status
	if s.Blockchain.IsValidator {
		printOK("Validator: Active")
	} else {
		printInfo("Validator", "Not in active set")
	}

	// Miss rate (critical for validators)
	if s.Blockchain.TotalBlocks > 0 {
		missPercent := s.Blockchain.MissRate * 100
		if missPercent < 5 {
			printOK("Miss rate: %.1f%% (%d/%d blocks missed)", missPercent, s.Blockchain.MissedBlocks, s.Blockchain.TotalBlocks)
		} else if missPercent < 20 {
			printWarn("Miss rate: %.1f%% (%d/%d blocks missed) — investigate", missPercent, s.Blockchain.MissedBlocks, s.Blockchain.TotalBlocks)
		} else {
			printFail("Miss rate: %.1f%% (%d/%d blocks missed) — CRITICAL: risk of jailing", missPercent, s.Blockchain.MissedBlocks, s.Blockchain.TotalBlocks)
		}
	}
}

func printEpoch(s *NodeStatus) {
	if s.Epoch.EpochNumber == 0 && !s.Epoch.Active {
		return // No epoch data available
	}

	printSection("Epoch")

	if s.Epoch.Active {
		printOK("Epoch %d: Active (weight: %d)", s.Epoch.EpochNumber, s.Epoch.Weight)
	} else if s.Epoch.EpochNumber > 0 {
		printWarn("Epoch %d: Not active", s.Epoch.EpochNumber)
	}

	// Miss rate
	if s.Epoch.TotalCount > 0 {
		if s.Epoch.MissPercentage < 5 {
			printOK("Miss rate: %.1f%% (%d/%d requests missed)", s.Epoch.MissPercentage, s.Epoch.MissedCount, s.Epoch.TotalCount)
		} else if s.Epoch.MissPercentage < 20 {
			printWarn("Miss rate: %.1f%% (%d/%d requests missed)", s.Epoch.MissPercentage, s.Epoch.MissedCount, s.Epoch.TotalCount)
		} else {
			printFail("Miss rate: %.1f%% (%d/%d requests missed) — risk of jailing", s.Epoch.MissPercentage, s.Epoch.MissedCount, s.Epoch.TotalCount)
		}
	} else {
		printInfo("Miss rate", "No requests in current epoch")
	}
}

func printMLNode(s *NodeStatus) {
	printSection("ML Node")

	// Status
	if s.MLNode.Enabled {
		printOK("Status: Enabled")
	} else {
		printWarn("Status: Disabled")
	}

	// Model
	if s.MLNode.ModelLoaded {
		printOK("Model: %s (loaded)", s.MLNode.ModelName)
	} else if s.MLNode.ModelName != "" {
		printWarn("Model: %s (not loaded)", s.MLNode.ModelName)
	} else {
		printFail("Model: None configured")
	}

	// GPUs
	if s.MLNode.Hardware != "" {
		printInfo("Hardware", "%s", s.MLNode.Hardware)
	} else if s.MLNode.GPUCount > 0 {
		printInfo("GPUs", "%dx %s", s.MLNode.GPUCount, s.MLNode.GPUName)
	}

	// GPU details from setup/report
	for _, gpu := range s.MLNode.GPUs {
		printInfo("GPU", "%s (VRAM: %.0f/%.0fGB, Util: %d%%, Temp: %dC)",
			gpu.Name, gpu.UsedMemoryGB, gpu.TotalMemoryGB, gpu.UtilizationPct, gpu.TemperatureC)
	}

	// TP/PP configuration
	if s.MLNode.TPSize > 0 {
		configStr := fmt.Sprintf("TP=%d", s.MLNode.TPSize)
		if s.MLNode.PPSize > 0 {
			configStr += fmt.Sprintf(" PP=%d", s.MLNode.PPSize)
		}
		if s.MLNode.MemoryUtil > 0 {
			configStr += fmt.Sprintf(" MemUtil=%.2f", s.MLNode.MemoryUtil)
		}
		if s.MLNode.MaxModelLen > 0 {
			configStr += fmt.Sprintf(" MaxLen=%d", s.MLNode.MaxModelLen)
		}
		printInfo("Config", "%s", configStr)
	}

	// PoC Status
	switch s.MLNode.PoCStatus {
	case "Participating":
		printOK("PoC Status: Participating")
	case "Pending":
		printWarn("PoC Status: Pending verification")
	default:
		printInfo("PoC Status", "%s", s.MLNode.PoCStatus)
	}

	// Last PoC
	if !s.MLNode.LastPoCTime.IsZero() {
		ago := formatDuration(time.Since(s.MLNode.LastPoCTime))
		if s.MLNode.LastPoCOK {
			printOK("Last PoC: %s ago (success)", ago)
		} else {
			printFail("Last PoC: %s ago (failed)", ago)
		}
	}
}

func printSecurity(s *NodeStatus) {
	printSection("Security")

	// Key checks from setup/report
	if s.Security.ColdKeyConfigured {
		printOK("Cold key: Configured")
	} else {
		printFail("Cold key: Not configured")
	}

	if s.Security.WarmKeyConfigured {
		printOK("Warm key: In keyring")
	} else {
		printFail("Warm key: Not in keyring")
	}

	if s.Security.PermissionsGranted {
		printOK("ML permissions: Granted")
	} else {
		printFail("ML permissions: Not granted")
	}

	// Infrastructure security (not yet detected from API — shown only when set)
	if s.Security.FirewallConfigured {
		printOK("Firewall: DOCKER-USER chain configured")
	}

	if s.Security.DDoSProtection {
		printOK("DDoS protection: Enabled")
	}

	if s.Security.InternalPortsBound {
		printOK("Internal ports: Bound to 127.0.0.1")
	}
}

// Helper functions

func printOK(format string, args ...interface{}) {
	_, _ = green.Print("  ✓ ")
	fmt.Printf(format+"\n", args...)
}

func printWarn(format string, args ...interface{}) {
	_, _ = yellow.Print("  ⚠ ")
	fmt.Printf(format+"\n", args...)
}

func printFail(format string, args ...interface{}) {
	_, _ = red.Print("  ✗ ")
	fmt.Printf(format+"\n", args...)
}

func printInfo(label, format string, args ...interface{}) {
	_, _ = dimmed.Print("  → ")
	fmt.Printf("%s: ", label)
	fmt.Printf(format+"\n", args...)
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func formatNumber(n int64) string {
	if n == 0 {
		return "0"
	}

	str := fmt.Sprintf("%d", n)
	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
