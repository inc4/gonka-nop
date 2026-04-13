package phases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	kvCacheDtypeFP8        = "fp8"
	mlnodeBlackwellSuffix  = "-blackwell"
	mlnodeBlackwellDefault = "3.0.12-blackwell"
)

// GPUDetection detects available GPUs and recommends configuration.
type GPUDetection struct {
	mocked bool
}

// NewGPUDetection creates a new GPUDetection phase.
func NewGPUDetection(mocked bool) *GPUDetection {
	return &GPUDetection{mocked: mocked}
}

func (p *GPUDetection) Name() string {
	return "GPU Detection"
}

func (p *GPUDetection) Description() string {
	return "Detecting NVIDIA GPUs, topology, and calculating optimal configuration"
}

func (p *GPUDetection) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *GPUDetection) Run(ctx context.Context, state *config.State) error {
	gpus, err := p.detectGPUs(ctx)
	if err != nil {
		return err
	}
	state.GPUs = gpus

	// Display detected GPUs
	ui.Header("Detected GPUs")
	totalVRAM := 0
	for _, gpu := range gpus {
		ui.Detail("[%d] %s - %d MB VRAM (driver: %s, arch: %s)", gpu.Index, gpu.Name, gpu.MemoryMB, gpu.DriverVersion, gpu.Architecture)
		totalVRAM += gpu.MemoryMB
	}
	ui.Info("Total: %d GPUs, %.1f GB VRAM", len(gpus), float64(totalVRAM)/1024)

	// Detect topology
	topology, err := p.detectTopologyPhase(gpus)
	if err != nil {
		return err
	}
	state.GPUTopology = topology

	if topology.HasNVLink {
		ui.Success("NVLink detected - optimal for multi-GPU inference")
	} else if len(gpus) > 1 {
		ui.Warn("PCIe %s only - no NVLink. Multi-GPU performance may be reduced", topology.PCIeVersion)
	}

	// Calculate recommended configuration
	err = ui.WithSpinner("Calculating optimal configuration", func() error {
		if p.mocked {
			time.Sleep(500 * time.Millisecond)
		}
		return nil
	})
	if err != nil {
		return err
	}

	rec := recommendConfig(len(gpus), gpus[0].MemoryMB, gpus[0].Architecture, topology.HasNVLink)
	state.TPSize = rec.TP
	state.PPSize = rec.PP
	state.SelectedModel = rec.Model
	state.GPUMemoryUtil = rec.MemoryUtil
	state.MaxModelLen = rec.MaxModelLen
	state.KVCacheDtype = rec.KVCacheDtype
	// For Blackwell GPUs, try to discover latest blackwell tag from registry
	var registryBlackwellTag string
	if IsBlackwellArch(gpus[0].Architecture) {
		ui.Info("Blackwell GPU detected, checking registry for latest image...")
		registryBlackwellTag = fetchLatestBlackwellTag()
		if registryBlackwellTag != "" {
			ui.Success("Found latest blackwell image: %s", registryBlackwellTag)
		}
	}

	state.MLNodeImageTag = selectMLNodeImage(gpus[0].Architecture, state.Versions.MLNode, registryBlackwellTag)
	state.AttentionBackend = selectAttentionBackend(gpus[0].Architecture)

	ui.Header("Recommended Configuration")
	ui.Detail("Model: %s", rec.Model)
	ui.Detail("Tensor Parallel Size (TP): %d", rec.TP)
	ui.Detail("Pipeline Parallel Size (PP): %d", rec.PP)
	ui.Detail("GPU Memory Utilization: %.2f", rec.MemoryUtil)
	ui.Detail("Max Model Length: %d", rec.MaxModelLen)
	if rec.KVCacheDtype == kvCacheDtypeFP8 {
		ui.Detail("KV Cache Dtype: fp8 (tight VRAM — saves memory)")
	}
	defaultImage := "ghcr.io/product-science/mlnode:" + state.MLNodeImageTag
	ui.Detail("MLNode Image: %s", defaultImage)
	ui.Detail("Attention Backend: %s", state.AttentionBackend)

	if !topology.HasNVLink && len(gpus) > 1 {
		ui.Warn("Without NVLink, multi-GPU inference may have higher latency from PCIe bottleneck")
	}

	ui.Success("Configuration optimized for %d GPUs", len(gpus))

	// Prompt for custom MLNode image override (skip if already set via --mlnode-image flag)
	if state.CustomMLNodeImage == "" {
		customImage, err := ui.Input(
			"Custom MLNode image (leave empty for default)",
			"",
		)
		if err != nil {
			return fmt.Errorf("MLNode image prompt: %w", err)
		}
		if customImage != "" {
			state.CustomMLNodeImage = customImage
			ui.Info("Using custom MLNode image: %s", customImage)
		}
	} else {
		ui.Info("Using custom MLNode image (from --mlnode-image): %s", state.CustomMLNodeImage)
	}

	// Prompt for attention backend selection
	backendOptions := []string{
		"FLASHINFER (default, recommended)",
		"FLASH_ATTN (lower memory, for constrained setups)",
	}
	selectedBackend, err := ui.Select("Attention backend", backendOptions)
	if err != nil {
		return fmt.Errorf("attention backend prompt: %w", err)
	}
	if strings.Contains(selectedBackend, "FLASH_ATTN") {
		state.AttentionBackend = "FLASH_ATTN"
	} else {
		state.AttentionBackend = "FLASHINFER"
	}

	return nil
}

func (p *GPUDetection) detectGPUs(ctx context.Context) ([]config.GPUInfo, error) {
	var gpus []config.GPUInfo
	err := ui.WithSpinner("Detecting NVIDIA GPUs", func() error {
		if p.mocked {
			time.Sleep(800 * time.Millisecond)
			gpus = []config.GPUInfo{
				{Index: 0, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:01:00.0"},
				{Index: 1, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:02:00.0"},
				{Index: 2, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:03:00.0"},
				{Index: 3, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:04:00.0"},
			}
			return nil
		}
		out, cmdErr := runCmd(ctx, "nvidia-smi",
			"--query-gpu=index,name,memory.total,driver_version,pci.bus_id",
			"--format=csv,noheader,nounits")
		if cmdErr != nil {
			return fmt.Errorf("nvidia-smi failed — GPU required: %w", cmdErr)
		}
		parsed, parseErr := ParseNvidiaSMICSV(out)
		if parseErr != nil {
			return parseErr
		}
		gpus = parsed
		return nil
	})
	return gpus, err
}

func (p *GPUDetection) detectTopologyPhase(gpus []config.GPUInfo) (config.GPUTopology, error) {
	var topology config.GPUTopology
	err := ui.WithSpinner("Detecting GPU topology", func() error {
		if p.mocked {
			time.Sleep(400 * time.Millisecond)
		}
		topology = detectTopology(gpus)
		return nil
	})
	return topology, err
}

// GPURecommendation holds the full GPU config recommendation
type GPURecommendation struct {
	TP           int
	PP           int
	Model        string
	MemoryUtil   float64
	MaxModelLen  int
	KVCacheDtype string // "auto" or "fp8"
}

// recommendConfig returns recommended configuration based on GPU setup.
// Incorporates validator chat findings: memory utilization 0.88-0.94, fp8 kv-cache for tight VRAM.
func recommendConfig(gpuCount int, vramMB int, _ string, _ bool) GPURecommendation {
	totalVRAM := gpuCount * vramMB

	switch {
	case totalVRAM >= 320000: // 320GB+ (e.g., 4x H100 80GB or 8x A100 40GB)
		rec := GPURecommendation{
			Model:        "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			MemoryUtil:   0.90,
			KVCacheDtype: kvCacheDtypeAuto,
		}
		// 235B FP8 needs ~120GB for weights. Remaining VRAM for KV cache.
		// MLNode runner auto-calculates PP from available GPUs (e.g. 8 GPUs / TP=4 → PP=2).
		// We only set TP here; PP is always 1 in node-config.json.
		if gpuCount >= 8 {
			rec.TP = 4
			rec.PP = 1
			rec.MaxModelLen = 240000
		} else {
			// 4x 80GB = 320GB, tight fit
			rec.TP = gpuCount
			rec.PP = 1
			rec.MemoryUtil = 0.88 // tighter margin needed
			rec.MaxModelLen = 16384
		}
		// 8x A100 40GB = 320GB, needs fp8 KV cache to avoid OOM
		if vramMB <= 41000 {
			rec.KVCacheDtype = kvCacheDtypeFP8
			rec.MemoryUtil = 0.90
		}
		return rec

	case totalVRAM >= 80000: // 80GB+ (e.g., 4x RTX 4090, 2x A100 80GB)
		rec := GPURecommendation{
			TP:           gpuCount,
			PP:           1,
			Model:        defaultModel,
			MemoryUtil:   0.92,
			MaxModelLen:  32768,
			KVCacheDtype: kvCacheDtypeAuto,
		}
		// Tight VRAM: 4x 24GB = 96GB with 32B model = ~8GB per GPU for KV
		if vramMB < 30000 {
			rec.MemoryUtil = 0.90
			rec.MaxModelLen = 24576
		}
		return rec

	case totalVRAM >= 40000: // 40GB+ (e.g., 2x RTX 4090)
		return GPURecommendation{
			TP:           gpuCount,
			PP:           1,
			Model:        "Qwen/Qwen3-32B-FP8",
			MemoryUtil:   0.92,
			MaxModelLen:  24576,
			KVCacheDtype: kvCacheDtypeAuto,
		}

	default: // < 40GB — below minimum
		return GPURecommendation{
			TP:           1,
			PP:           1,
			Model:        "Qwen/Qwen3-32B-FP8",
			MemoryUtil:   0.94,
			MaxModelLen:  8192,
			KVCacheDtype: kvCacheDtypeAuto,
		}
	}
}

// detectTopology returns GPU interconnect topology.
// NVLink detection is name-based for known GPUs.
func detectTopology(gpus []config.GPUInfo) config.GPUTopology {
	if len(gpus) <= 1 {
		return config.GPUTopology{HasNVLink: false, PCIeVersion: "4.0", Interconnect: "pcie"}
	}
	name := gpus[0].Name
	if strings.Contains(name, "H100") || strings.Contains(name, "H200") || strings.Contains(name, "A100") {
		return config.GPUTopology{HasNVLink: true, PCIeVersion: "5.0", Interconnect: "nvlink"}
	}
	return config.GPUTopology{HasNVLink: false, PCIeVersion: "4.0", Interconnect: "pcie"}
}

// IsBlackwellArch returns true if the GPU architecture requires the blackwell mlnode image.
func IsBlackwellArch(arch string) bool {
	switch arch {
	case "sm_100", "sm_103", "sm_120":
		return true
	default:
		return false
	}
}

// selectMLNodeImage returns the appropriate mlnode image tag based on GPU architecture.
// For Blackwell GPUs, uses registryTag if available (from GHCR lookup), otherwise
// appends "-blackwell" suffix to the fetched version.
func selectMLNodeImage(arch string, fetchedVersion string, registryTag string) string {
	if !IsBlackwellArch(arch) {
		if fetchedVersion != "" {
			return fetchedVersion
		}
		return "3.0.12"
	}

	// Blackwell: prefer registry-discovered tag
	if registryTag != "" {
		return registryTag
	}

	// Fallback: append -blackwell suffix to fetched version
	baseTag := fetchedVersion
	if baseTag == "" {
		baseTag = "3.0.12"
	}
	if !strings.Contains(baseTag, "blackwell") {
		return baseTag + mlnodeBlackwellSuffix
	}
	return baseTag
}

// fetchLatestBlackwellTag queries the GHCR registry for the latest mlnode blackwell tag.
// Returns empty string on failure (caller should fall back to suffix convention).
func fetchLatestBlackwellTag() string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get anonymous token for GHCR
	tokenURL := "https://ghcr.io/token?scope=repository:product-science/mlnode:pull"
	req, err := http.NewRequestWithContext(ctx, "GET", tokenURL, nil)
	if err != nil {
		return ""
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	var tokenResp struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(resp.Body).Decode(&tokenResp) != nil || tokenResp.Token == "" {
		return ""
	}

	// Fetch tags list
	tagsURL := "https://ghcr.io/v2/product-science/mlnode/tags/list"
	req, err = http.NewRequestWithContext(ctx, "GET", tagsURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)

	resp, err = client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	var tagsResp struct {
		Tags []string `json:"tags"`
	}
	if json.NewDecoder(resp.Body).Decode(&tagsResp) != nil {
		return ""
	}

	// Find latest blackwell tag (convention: "X.Y.Z-postN-blackwell", not sm120/alpha)
	var best string
	for _, tag := range tagsResp.Tags {
		if !strings.HasSuffix(tag, "-blackwell") {
			continue
		}
		// Skip experimental tags (sm120, alpha, fp8 variants)
		if strings.Contains(tag, "sm120") || strings.Contains(tag, "alpha") || strings.Contains(tag, "fp8") {
			continue
		}
		if best == "" || tag > best {
			best = tag
		}
	}
	return best
}

// selectAttentionBackend returns the vLLM attention backend for the GPU architecture.
// FLASHINFER is the standard backend across all architectures in mlnode 3.0.12+.
func selectAttentionBackend(_ string) string {
	return "FLASHINFER"
}

// FormatGPUSummary returns a formatted GPU summary string
func FormatGPUSummary(gpus []config.GPUInfo) string {
	if len(gpus) == 0 {
		return "No GPUs detected"
	}
	totalVRAM := 0
	for _, gpu := range gpus {
		totalVRAM += gpu.MemoryMB
	}
	return fmt.Sprintf("%dx %s (%.1f GB total)", len(gpus), gpus[0].Name, float64(totalVRAM)/1024)
}
