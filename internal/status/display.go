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
	printNodeConfig(s)
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

	// Containers — inferred from API checks, not direct docker access
	if s.Overview.ContainersRunning >= 5 {
		printOK("Core services: Running (node, api, mlnode verified)")
	} else if s.Overview.ContainersRunning > 0 {
		printWarn("Core services: %d/%d verified", s.Overview.ContainersRunning, s.Overview.ContainersTotal)
	} else {
		printFail("Core services: Not reachable")
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

	// API version
	if s.NodeConfig.APIVersion != "" {
		printInfo("API version", "%s", s.NodeConfig.APIVersion)
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
		if s.Blockchain.VotingPower > 0 {
			printOK("Validator: Active (power: %d)", s.Blockchain.VotingPower)
		} else {
			printOK("Validator: Active")
		}
	} else {
		printInfo("Validator", "Not in active set")
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

	// PoC weight (from epoch_ml_nodes — the actual weight used for PoC)
	if s.Epoch.PoCWeight > 0 {
		printInfo("PoC weight", "%d", s.Epoch.PoCWeight)
	}

	// Timeslot allocation
	if len(s.Epoch.TimeslotAllocation) > 0 {
		slots := formatTimeslots(s.Epoch.TimeslotAllocation)
		printInfo("Timeslots", "%s", slots)
	}

	// Miss rate
	if s.Epoch.TotalCount > 0 {
		missStr := fmt.Sprintf("%.1f%% (%d/%d requests", s.Epoch.MissPercentage, s.Epoch.MissedCount, s.Epoch.TotalCount)
		if s.Epoch.InferenceCount > 0 {
			missStr += fmt.Sprintf(", %d inferences served", s.Epoch.InferenceCount)
		}
		missStr += ")"
		if s.Epoch.MissPercentage < 5 {
			printOK("Miss rate: %s", missStr)
		} else if s.Epoch.MissPercentage < 20 {
			printWarn("Miss rate: %s", missStr)
		} else {
			printFail("Miss rate: %s — risk of jailing", missStr)
		}
	} else if s.Epoch.MissCheckPassed {
		printOK("Miss rate: 0.0%% (no requests yet)")
	} else {
		printInfo("Miss rate", "No data")
	}

	// Upcoming epoch
	if s.Epoch.UpcomingEpoch > 0 {
		printInfo("Upcoming", "Epoch %d", s.Epoch.UpcomingEpoch)
	}

	// Previous epoch reward claim
	if s.Epoch.PrevEpochIndex > 0 {
		if s.Epoch.PrevEpochClaimed {
			printOK("Reward (epoch %d): Claimed", s.Epoch.PrevEpochIndex)
		} else {
			printWarn("Reward (epoch %d): Not claimed", s.Epoch.PrevEpochIndex)
		}
	}
}

func printMLNode(s *NodeStatus) {
	printSection("ML Node")

	// Enabled state
	if s.MLNode.Enabled {
		printOK("Enabled: Yes")
	} else {
		printWarn("Enabled: No")
	}

	// Model
	if s.MLNode.ModelLoaded {
		printOK("Model: %s (loaded)", s.MLNode.ModelName)
	} else if s.MLNode.ModelName != "" {
		printWarn("Model: %s (not loaded)", s.MLNode.ModelName)
	} else {
		printFail("Model: None configured")
	}

	printMLNodeHardware(s)
	printMLNodeConfig(s)
	printMLNodePoCStatus(s)
	printMLNodeFreshness(s)
}

func printMLNodeHardware(s *NodeStatus) {
	if s.MLNode.Hardware != "" {
		printInfo("Hardware", "%s", s.MLNode.Hardware)
	} else if s.MLNode.GPUCount > 0 {
		printInfo("GPUs", "%dx %s", s.MLNode.GPUCount, s.MLNode.GPUName)
	}

	for _, gpu := range s.MLNode.GPUs {
		printInfo("GPU", "%s (VRAM: %.0f/%.0fGB, Util: %d%%, Temp: %dC)",
			gpu.Name, gpu.UsedMemoryGB, gpu.TotalMemoryGB, gpu.UtilizationPct, gpu.TemperatureC)
	}
}

func printMLNodeConfig(s *NodeStatus) {
	if s.MLNode.TPSize == 0 {
		return
	}
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

func printMLNodePoCStatus(s *NodeStatus) {
	switch s.MLNode.PoCStatus {
	case "INFERENCE":
		printOK("Status: Serving inference")
	case "POC":
		printOK("Status: PoC generation active")
	case "FAILED":
		printFail("Status: FAILED")
	case "":
		// No data available
	default:
		printInfo("Status", "%s", s.MLNode.PoCStatus)
	}

	if s.MLNode.IntendedStatus != "" && s.MLNode.PoCStatus != "" &&
		s.MLNode.IntendedStatus != s.MLNode.PoCStatus {
		printWarn("Status mismatch: intended=%s current=%s (transitioning)", s.MLNode.IntendedStatus, s.MLNode.PoCStatus)
	}

	if s.MLNode.PoCNodeStatus != "" && s.MLNode.PoCNodeStatus != "IDLE" {
		printInfo("PoC subsystem", "%s", s.MLNode.PoCNodeStatus)
	}
}

func printMLNodeFreshness(s *NodeStatus) {
	if !s.MLNode.StatusUpdated.IsZero() {
		ago := time.Since(s.MLNode.StatusUpdated)
		if ago > 10*time.Minute {
			printWarn("Status updated: %s ago (stale — possible issue)", formatDuration(ago))
		} else {
			printInfo("Status updated", "%s ago", formatDuration(ago))
		}
	}

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

func printNodeConfig(s *NodeStatus) {
	// Only show if we have any config data
	if s.NodeConfig.PublicURL == "" && s.NodeConfig.APIVersion == "" {
		return
	}

	printSection("Node Config")

	if s.NodeConfig.PublicURL != "" {
		printInfo("Public URL", "%s", s.NodeConfig.PublicURL)
	}

	if s.NodeConfig.PoCCallbackURL != "" {
		printInfo("PoC callback", "%s", s.NodeConfig.PoCCallbackURL)
	}

	if s.NodeConfig.SeedAPIURL != "" {
		printInfo("Seed API", "%s", s.NodeConfig.SeedAPIURL)
	}

	// API processing lag
	if s.NodeConfig.HeightLag > 5 {
		printWarn("API processing lag: %d blocks behind chain", s.NodeConfig.HeightLag)
	}

	// Upgrade plan
	if s.NodeConfig.UpgradeName != "" {
		printWarn("Upgrade pending: %s at height %s", s.NodeConfig.UpgradeName, formatNumber(s.NodeConfig.UpgradeHeight))
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

func formatTimeslots(slots []bool) string {
	allocated := 0
	for _, s := range slots {
		if s {
			allocated++
		}
	}
	return fmt.Sprintf("%d/%d allocated", allocated, len(slots))
}
