package phases

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
)

const (
	familyDebian  = "debian"
	familyRHEL    = "rhel"
	familyUnknown = "unknown"
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
	return familyUnknown
}

// ParseOSRelease parses /etc/os-release content into a Distro struct.
// Expected format: KEY=VALUE or KEY="VALUE" lines.
func ParseOSRelease(content string) (config.Distro, error) {
	vals := parseOSReleaseKV(content)

	d := config.Distro{
		ID:      strings.ToLower(vals["ID"]),
		Version: vals["VERSION_ID"],
	}
	if d.ID == "" {
		return d, fmt.Errorf("could not determine distro ID from os-release")
	}

	d.Family = inferDistroFamily(d.ID, strings.ToLower(vals["ID_LIKE"]))
	return d, nil
}

// parseOSReleaseKV extracts KEY=VALUE pairs from os-release content.
func parseOSReleaseKV(content string) map[string]string {
	vals := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			vals[parts[0]] = strings.Trim(parts[1], `"`)
		}
	}
	return vals
}

// inferDistroFamily determines the package manager family from distro ID and ID_LIKE.
func inferDistroFamily(id, idLike string) string {
	switch {
	case id == "ubuntu" || id == familyDebian || strings.Contains(idLike, familyDebian):
		return familyDebian
	case id == "centos" || id == familyRHEL || id == "fedora" || id == "rocky" || id == "almalinux" || strings.Contains(idLike, familyRHEL):
		return familyRHEL
	case id == "amzn":
		return familyRHEL
	default:
		return familyUnknown
	}
}

// ParseModinfoVersion extracts version from `modinfo nvidia` output.
// Expected line: "version:        570.133.20"
func ParseModinfoVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		}
	}
	return ""
}

// ParseFabricManagerVersion extracts version from dpkg -l output.
// Expected line: "ii  nvidia-fabricmanager-570  570.133.20-1  amd64  ..."
func ParseFabricManagerVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "ii" && strings.Contains(fields[1], "fabricmanager") {
			return fields[2]
		}
	}
	return ""
}

// DriverMajorVersion extracts major version from driver string.
// "570.133.20" → "570"
func DriverMajorVersion(driverVersion string) string {
	parts := strings.SplitN(driverVersion, ".", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// BlockDevice represents a block device from lsblk JSON output.
type BlockDevice struct {
	Name       string        `json:"name"`
	Size       int64         `json:"size"`       // bytes
	Type       string        `json:"type"`       // "disk", "part", "loop"
	Mountpoint *string       `json:"mountpoint"` // nil or empty if unmounted
	Fstype     *string       `json:"fstype"`     // nil or empty if no filesystem
	Children   []BlockDevice `json:"children"`   // partitions
}

// lsblkOutput is the top-level JSON structure from lsblk -J.
type lsblkOutput struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

// ParseLsblkJSON parses JSON output from `lsblk -J -b -o NAME,SIZE,TYPE,MOUNTPOINT,FSTYPE`.
func ParseLsblkJSON(jsonOutput string) ([]BlockDevice, error) {
	var out lsblkOutput
	if err := json.Unmarshal([]byte(jsonOutput), &out); err != nil {
		return nil, fmt.Errorf("parse lsblk JSON: %w", err)
	}
	return out.BlockDevices, nil
}

// FindUnmountedDrives returns drives that are unmounted and ≥ minSizeGB.
// Excludes loop devices, drives with mounted partitions, and drives with existing filesystems.
func FindUnmountedDrives(devices []BlockDevice, minSizeGB int) []BlockDevice {
	minBytes := int64(minSizeGB) * 1e9
	var result []BlockDevice
	for _, d := range devices {
		if d.Type != "disk" {
			continue
		}
		if strings.HasPrefix(d.Name, "loop") {
			continue
		}
		if d.Size < minBytes {
			continue
		}
		// Skip if the drive itself is mounted
		if d.Mountpoint != nil && *d.Mountpoint != "" {
			continue
		}
		// Skip if any child partition is mounted
		if hasAnyMountedChild(d.Children) {
			continue
		}
		result = append(result, d)
	}
	return result
}

// hasAnyMountedChild returns true if any child device (or nested children) has a mountpoint.
func hasAnyMountedChild(children []BlockDevice) bool {
	for _, c := range children {
		if c.Mountpoint != nil && *c.Mountpoint != "" {
			return true
		}
		if hasAnyMountedChild(c.Children) {
			return true
		}
	}
	return false
}

// ParseDfSource extracts the device name from `df --output=source <path>` output.
// Expected format:
//
//	Filesystem
//	/dev/nvme0n1p3
func ParseDfSource(dfOutput string) (string, error) {
	lines := strings.Split(strings.TrimSpace(dfOutput), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("expected at least 2 lines from df output, got %d", len(lines))
	}
	return strings.TrimSpace(lines[len(lines)-1]), nil
}

// FormatDriveSize formats bytes as a human-readable size string (e.g., "3.5 TB").
func FormatDriveSize(bytes int64) string {
	const (
		tb = 1e12
		gb = 1e9
	)
	if bytes >= int64(tb) {
		return fmt.Sprintf("%.1f TB", float64(bytes)/tb)
	}
	return fmt.Sprintf("%.0f GB", float64(bytes)/gb)
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
