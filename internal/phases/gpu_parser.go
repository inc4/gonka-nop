package phases

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
)

// ParseDockerVersion parses "Docker version 27.4.1, build ..." into "27.4.1".
func ParseDockerVersion(output string) (string, error) {
	// Expected format: "Docker version 27.4.1, build abc1234"
	output = strings.TrimSpace(output)
	parts := strings.Fields(output)
	for i, p := range parts {
		if p == "version" && i+1 < len(parts) {
			ver := strings.TrimRight(parts[i+1], ",")
			return ver, nil
		}
	}
	return "", fmt.Errorf("could not parse Docker version from: %q", output)
}

// ParseDockerComposeVersion parses "Docker Compose version v2.32.4" into "v2.32.4".
func ParseDockerComposeVersion(output string) (string, error) {
	output = strings.TrimSpace(output)
	parts := strings.Fields(output)
	// Look for "version" keyword and take the next token
	for i, p := range parts {
		if strings.EqualFold(p, "version") && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("could not parse Docker Compose version from: %q", output)
}

// ParseNvidiaSMICSV parses nvidia-smi CSV output into a GPUInfo slice.
// Expected input per line: "0, NVIDIA GeForce RTX 3090, 24576, 560.35.03, 0000:01:00.0"
// Fields: index, name, memory.total (MiB), driver_version, pci.bus_id
func ParseNvidiaSMICSV(csvOutput string) ([]config.GPUInfo, error) {
	var gpus []config.GPUInfo
	lines := strings.Split(strings.TrimSpace(csvOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ", ")
		if len(fields) < 5 {
			return nil, fmt.Errorf("expected 5 CSV fields, got %d in: %q", len(fields), line)
		}
		idx, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			return nil, fmt.Errorf("parse GPU index %q: %w", fields[0], err)
		}
		mem, err := strconv.Atoi(strings.TrimSpace(fields[2]))
		if err != nil {
			return nil, fmt.Errorf("parse GPU memory %q: %w", fields[2], err)
		}
		name := strings.TrimSpace(fields[1])
		gpus = append(gpus, config.GPUInfo{
			Index:         idx,
			Name:          name,
			MemoryMB:      mem,
			DriverVersion: strings.TrimSpace(fields[3]),
			PCIBusID:      strings.TrimSpace(fields[4]),
			Architecture:  GPUArchFromName(name),
		})
	}
	if len(gpus) == 0 {
		return nil, fmt.Errorf("no GPUs found in nvidia-smi output")
	}
	return gpus, nil
}

// gpuArchEntry maps a GPU name substring to its compute architecture.
type gpuArchEntry struct {
	substr string
	arch   string
}

// gpuArchLookup is ordered so more-specific names come first.
var gpuArchLookup = []gpuArchEntry{
	// Blackwell / Next-gen
	{"B300", "sm_100"},
	{"B200", "sm_100"},
	{"RTX 5090", "sm_120"},
	// Hopper
	{"H200", "sm_90"},
	{"H100", "sm_90"},
	// Ampere datacenter
	{"A100", "sm_80"},
	// Ada Lovelace
	{"RTX 6000 Ada", "sm_89"},
	{"RTX 4090", "sm_89"},
	{"RTX 4080", "sm_89"},
	{"L40", "sm_89"},
	{"L4", "sm_89"},
	// Ampere consumer / workstation
	{"RTX A6000", "sm_86"},
	{"A40", "sm_86"},
	{"RTX 3090", "sm_86"},
	{"RTX 3080", "sm_86"},
}

// GPUArchFromName returns the GPU compute architecture from a GPU name.
// Returns "unknown" for unrecognized GPUs.
func GPUArchFromName(gpuName string) string {
	upper := strings.ToUpper(gpuName)
	for _, entry := range gpuArchLookup {
		if strings.Contains(upper, strings.ToUpper(entry.substr)) {
			return entry.arch
		}
	}
	return "unknown"
}

// ParseDiskFreeGB parses output from `df --output=avail -BG <path>` into GB.
// Expected output:
//
//	 Avail
//	133G
func ParseDiskFreeGB(dfOutput string) (int, error) {
	lines := strings.Split(strings.TrimSpace(dfOutput), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("expected at least 2 lines from df output, got %d", len(lines))
	}
	// The value is on the last non-empty line
	valLine := strings.TrimSpace(lines[len(lines)-1])
	valLine = strings.TrimSuffix(valLine, "G")
	valLine = strings.TrimSpace(valLine)
	gb, err := strconv.Atoi(valLine)
	if err != nil {
		return 0, fmt.Errorf("parse disk free %q: %w", valLine, err)
	}
	return gb, nil
}
