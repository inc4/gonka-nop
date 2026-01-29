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
	printMLNode(s)
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
		printOK("Epoch participation: Active (weight: %d)", s.Overview.EpochWeight)
	} else {
		printWarn("Epoch participation: Inactive")
	}
}

func printBlockchain(s *NodeStatus) {
	printSection("Blockchain")

	// Block height
	printInfo("Block height", "%s", formatNumber(s.Blockchain.BlockHeight))

	// Sync status
	if s.Blockchain.Synced {
		printOK("Sync status: Synced")
	} else if s.Blockchain.CatchingUp {
		printWarn("Sync status: Syncing (catching up)")
	} else {
		printFail("Sync status: Not synced")
	}

	// Peers
	if s.Blockchain.PeerCount >= 5 {
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
	if s.MLNode.GPUCount > 0 {
		printInfo("GPUs", "%dx %s", s.MLNode.GPUCount, s.MLNode.GPUName)
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
