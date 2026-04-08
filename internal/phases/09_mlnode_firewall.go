package phases

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// inferenceContainerPort is the port uvicorn listens on inside the mlnode-308
// container. The host maps state.InferencePort (default 5050) → 5000 via Docker,
// so in the DOCKER-USER chain (post-DNAT) we must use the container port.
const inferenceContainerPort = 5000

// MLNodeFirewall configures iptables DOCKER-USER rules to restrict mlnode ports
// to accept connections only from the network node IP.
type MLNodeFirewall struct{}

func NewMLNodeFirewall() *MLNodeFirewall {
	return &MLNodeFirewall{}
}

func (p *MLNodeFirewall) Name() string {
	return "ML Node Firewall"
}

func (p *MLNodeFirewall) Description() string {
	return "Restricting mlnode ports to network node IP only via DOCKER-USER iptables"
}

// ShouldRun returns true when:
//   - topology is mlnode-only (separate GPU server)
//   - public IP is routable (not RFC1918 — private nets don't need restriction)
//   - firewall has not been configured yet in this state
func (p *MLNodeFirewall) ShouldRun(state *config.State) bool {
	return state.IsMLNodeOnly() && isPublicIP(state.PublicIP) && !state.FirewallConfigured
}

// Run applies iptables DROP rules in the DOCKER-USER chain for ports 8080 (PoC)
// and 5000 (inference container port), then persists the rules.
//
// Two cases:
//   - Same server (public_ip == network_node_ip): Docker bridge traffic arrives
//     from 172.16.0.0/12, so we allow that subnet and drop the rest.
//   - Separate server: traffic arrives from the real network node IP, so we
//     allow only that specific IP and drop the rest.
//
// Non-fatal: prints manual commands on iptables failure and continues.
func (p *MLNodeFirewall) Run(_ context.Context, state *config.State) error {
	networkNodeIP := state.NetworkNodeIP
	if networkNodeIP == "" {
		ui.Warn("Network node IP unknown — skipping port restriction")
		ui.Detail("Restrict ports %d and %d manually via iptables DOCKER-USER", state.PoCPort, state.InferencePort)
		return nil
	}

	pocPort := state.PoCPort
	if pocPort == 0 {
		pocPort = 8080
	}

	// Determine allowed source and topology label.
	var allowedSrc, topoLabel string
	if state.PublicIP == networkNodeIP {
		// Same-server: network node containers use the Docker bridge network.
		allowedSrc = "172.16.0.0/12"
		topoLabel = "same-server (Docker bridge)"
	} else {
		// Separate-server: allow the real network node IP only.
		allowedSrc = networkNodeIP
		topoLabel = "separate-server (" + networkNodeIP + ")"
	}

	ui.Info("Restricting ports %d (PoC) and %d (inference) — %s",
		pocPort, inferenceContainerPort, topoLabel)

	// Container ports used in DOCKER-USER (post-DNAT):
	//   PoCPort host == PoCPort container (both 8080 by default)
	//   InferencePort host (5050) → container port 5000
	ports := []int{pocPort, inferenceContainerPort}

	var failed []int
	for _, port := range ports {
		portStr := fmt.Sprintf("%d", port)
		if err := runIPTables(state.UseSudo, "-I", "DOCKER-USER",
			"-p", "tcp", "--dport", portStr,
			"!", "-s", allowedSrc, "-j", "DROP"); err != nil {
			ui.Warn("iptables DROP port %d: %v", port, err)
			failed = append(failed, port)
		} else {
			ui.Success("Port %d: blocked for all except %s", port, allowedSrc)
		}
	}

	if len(failed) > 0 {
		ui.Warn("Firewall not fully configured — apply manually on this server:")
		for _, port := range ports {
			ui.Detail("  sudo iptables -I DOCKER-USER -p tcp --dport %d ! -s %s -j DROP",
				port, allowedSrc)
		}
		ui.Detail("  sudo mkdir -p /etc/iptables && sudo iptables-save > /etc/iptables/rules.v4")
		return nil
	}

	if err := persistIPTables(state.UseSudo); err != nil {
		ui.Warn("Rules active but not persisted across reboots: %v", err)
		ui.Detail("To persist manually: sudo mkdir -p /etc/iptables && sudo iptables-save > /etc/iptables/rules.v4")
	} else {
		ui.Success("Firewall rules saved (persistent across reboots)")
	}

	state.FirewallConfigured = true
	return nil
}

// runIPTables runs an iptables command, optionally prefixed with sudo.
func runIPTables(useSudo bool, args ...string) error {
	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.Command(cmdSudo, append([]string{"iptables"}, args...)...)
	} else {
		cmd = exec.Command("iptables", args...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// persistIPTables saves iptables rules so they survive reboots.
// Tries netfilter-persistent first, then writes rules.v4 and installs
// an if-pre-up hook for auto-restore on network bring-up.
func persistIPTables(useSudo bool) error {
	// Try netfilter-persistent (Debian/Ubuntu with the package installed)
	args := []string{"netfilter-persistent", "save"}
	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.Command(cmdSudo, args...)
	} else {
		cmd = exec.Command(args[0], args[1:]...)
	}
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Fallback: write rules.v4 + install if-pre-up.d restore hook
	if err := os.MkdirAll("/etc/iptables", 0750); err != nil {
		return fmt.Errorf("mkdir /etc/iptables: %w", err)
	}

	saveCmd := "iptables-save > /etc/iptables/rules.v4"
	if useSudo {
		cmd = exec.Command(cmdSudo, "sh", "-c", saveCmd)
	} else {
		cmd = exec.Command("sh", "-c", saveCmd)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("iptables-save: %w", err)
	}

	// Install if-pre-up.d hook so rules reload on every network restart
	hookPath := "/etc/network/if-pre-up.d/iptables"
	hookContent := "#!/bin/sh\niptables-restore < /etc/iptables/rules.v4\n"
	writeCmd := fmt.Sprintf("printf '%%s' %q > %s && chmod +x %s",
		hookContent, hookPath, hookPath)
	if useSudo {
		cmd = exec.Command(cmdSudo, "sh", "-c", writeCmd)
	} else {
		cmd = exec.Command("sh", "-c", writeCmd)
	}
	_ = cmd.Run() // non-fatal if hook install fails

	return nil
}

// isPublicIP returns true if ip is a routable public address (not RFC1918/loopback/link-local).
func isPublicIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
	}
	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsed) {
			return false
		}
	}
	return true
}
