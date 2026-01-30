package phases

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// GPUDetection detects available GPUs and recommends configuration
type GPUDetection struct{}

func NewGPUDetection() *GPUDetection {
	return &GPUDetection{}
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

func (p *GPUDetection) Run(_ context.Context, state *config.State) error {
	// Detect GPUs (mocked)
	var gpus []config.GPUInfo
	err := ui.WithSpinner("Detecting NVIDIA GPUs", func() error {
		time.Sleep(800 * time.Millisecond)
		// Mocked GPU data - simulating 4x RTX 4090
		gpus = []config.GPUInfo{
			{Index: 0, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:01:00.0"},
			{Index: 1, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:02:00.0"},
			{Index: 2, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:03:00.0"},
			{Index: 3, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564, DriverVersion: "570.133.20", Architecture: "sm_89", PCIBusID: "0000:04:00.0"},
		}
		return nil
	})
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

	// Detect GPU topology (mocked)
	err = ui.WithSpinner("Detecting GPU topology", func() error {
		time.Sleep(400 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}

	topology := detectTopology(gpus)
	state.GPUTopology = topology

	if topology.HasNVLink {
		ui.Success("NVLink detected - optimal for multi-GPU inference")
	} else {
		ui.Warn("PCIe %s only - no NVLink. Multi-GPU performance may be reduced", topology.PCIeVersion)
	}

	// Calculate recommended configuration
	err = ui.WithSpinner("Calculating optimal configuration", func() error {
		time.Sleep(500 * time.Millisecond)
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
	state.MLNodeImageTag = selectMLNodeImage(gpus[0].Architecture)
	state.AttentionBackend = selectAttentionBackend(gpus[0].Architecture)

	ui.Header("Recommended Configuration")
	ui.Detail("Model: %s", rec.Model)
	ui.Detail("Tensor Parallel Size (TP): %d", rec.TP)
	ui.Detail("Pipeline Parallel Size (PP): %d", rec.PP)
	ui.Detail("GPU Memory Utilization: %.2f", rec.MemoryUtil)
	ui.Detail("Max Model Length: %d", rec.MaxModelLen)
	if rec.KVCacheDtype == "fp8" {
		ui.Detail("KV Cache Dtype: fp8 (tight VRAM — saves memory)")
	}
	ui.Detail("MLNode Image: ghcr.io/product-science/mlnode:%s", state.MLNodeImageTag)
	ui.Detail("Attention Backend: %s", state.AttentionBackend)

	if rec.PP > 1 {
		ui.Warn("pipeline-parallel-size > 1: PoC v2 may not work with MQLLMEngineClient")
	}

	if !topology.HasNVLink && len(gpus) > 1 {
		ui.Warn("Without NVLink, multi-GPU inference may have higher latency from PCIe bottleneck")
	}

	ui.Success("Configuration optimized for %d GPUs", len(gpus))
	return nil
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
func recommendConfig(gpuCount int, vramMB int, _ string, hasNVLink bool) GPURecommendation {
	totalVRAM := gpuCount * vramMB

	switch {
	case totalVRAM >= 320000: // 320GB+ (e.g., 4x H100 80GB or 8x A100 40GB)
		rec := GPURecommendation{
			Model:        "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			MemoryUtil:   0.90,
			KVCacheDtype: "auto",
		}
		// 235B FP8 needs ~120GB for weights. Remaining VRAM for KV cache.
		if gpuCount >= 8 && hasNVLink {
			rec.TP = 8
			rec.PP = 1
			rec.MaxModelLen = 240000
		} else if gpuCount >= 8 {
			rec.TP = 4
			rec.PP = 2
			rec.MaxModelLen = 131072
			ui.Warn("PP=2 may cause PoC v2 issues — consider 8-way TP if possible")
		} else {
			// 4x 80GB = 320GB, tight fit
			rec.TP = gpuCount
			rec.PP = 1
			rec.MemoryUtil = 0.88 // tighter margin needed
			rec.MaxModelLen = 16384
		}
		// 8x A100 40GB = 320GB, needs fp8 KV cache to avoid OOM
		if vramMB <= 41000 {
			rec.KVCacheDtype = "fp8"
			rec.MemoryUtil = 0.90
		}
		return rec

	case totalVRAM >= 80000: // 80GB+ (e.g., 4x RTX 4090, 2x A100 80GB)
		rec := GPURecommendation{
			TP:           gpuCount,
			PP:           1,
			Model:        "Qwen/QwQ-32B",
			MemoryUtil:   0.92,
			MaxModelLen:  32768,
			KVCacheDtype: "auto",
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
			KVCacheDtype: "auto",
		}

	default: // < 40GB — below minimum
		return GPURecommendation{
			TP:           1,
			PP:           1,
			Model:        "Qwen/Qwen3-32B-FP8",
			MemoryUtil:   0.94,
			MaxModelLen:  8192,
			KVCacheDtype: "auto",
		}
	}
}

// detectTopology returns GPU interconnect topology (mocked)
func detectTopology(gpus []config.GPUInfo) config.GPUTopology {
	if len(gpus) <= 1 {
		return config.GPUTopology{HasNVLink: false, PCIeVersion: "4.0", Interconnect: "pcie"}
	}
	// Mocked: RTX 4090 has no NVLink, H100/A100 do
	name := gpus[0].Name
	if strings.Contains(name, "H100") || strings.Contains(name, "H200") || strings.Contains(name, "A100") {
		return config.GPUTopology{HasNVLink: true, PCIeVersion: "5.0", Interconnect: "nvlink"}
	}
	return config.GPUTopology{HasNVLink: false, PCIeVersion: "4.0", Interconnect: "pcie"}
}

// selectMLNodeImage returns the appropriate mlnode image tag based on GPU architecture
func selectMLNodeImage(arch string) string {
	switch arch {
	case "sm_90", "sm_90a": // H100, H200
		return "3.0.12"
	case "sm_100": // B200, B300
		return "3.0.12-blackwell"
	case "sm_120": // RTX 5090
		return "3.0.12-blackwell" // sm120 build when available
	default:
		return "3.0.12"
	}
}

// selectAttentionBackend returns the vLLM attention backend for the GPU architecture
func selectAttentionBackend(arch string) string {
	switch arch {
	case "sm_100", "sm_120": // Blackwell: FlashAttention not available
		return "FLASHINFER"
	default:
		return "FLASH_ATTN"
	}
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
