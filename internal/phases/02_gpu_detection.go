package phases

import (
	"context"
	"fmt"
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
	return "Detecting NVIDIA GPUs and calculating optimal configuration"
}

func (p *GPUDetection) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *GPUDetection) Run(ctx context.Context, state *config.State) error {
	// Detect GPUs (mocked)
	var gpus []config.GPUInfo
	err := ui.WithSpinner("Detecting NVIDIA GPUs", func() error {
		time.Sleep(800 * time.Millisecond)
		// Mocked GPU data - simulating 4x RTX 4090
		gpus = []config.GPUInfo{
			{Index: 0, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
			{Index: 1, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
			{Index: 2, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
			{Index: 3, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Store in state
	state.GPUs = gpus

	// Display detected GPUs
	ui.Header("Detected GPUs")
	totalVRAM := 0
	for _, gpu := range gpus {
		ui.Detail("[%d] %s - %d MB VRAM", gpu.Index, gpu.Name, gpu.MemoryMB)
		totalVRAM += gpu.MemoryMB
	}
	ui.Info("Total: %d GPUs, %.1f GB VRAM", len(gpus), float64(totalVRAM)/1024)

	// Calculate recommended configuration
	err = ui.WithSpinner("Calculating optimal TP/PP configuration", func() error {
		time.Sleep(500 * time.Millisecond)
		return nil
	})
	if err != nil {
		return err
	}

	// Recommend model and TP/PP based on GPU count
	tp, pp, model := recommendConfig(len(gpus), gpus[0].MemoryMB)
	state.TPSize = tp
	state.PPSize = pp
	state.SelectedModel = model

	ui.Header("Recommended Configuration")
	ui.Detail("Model: %s", model)
	ui.Detail("Tensor Parallel Size (TP): %d", tp)
	ui.Detail("Pipeline Parallel Size (PP): %d", pp)
	ui.Success("Configuration optimized for %d GPUs", len(gpus))

	return nil
}

// recommendConfig returns recommended TP, PP, and model based on GPU setup
func recommendConfig(gpuCount int, vramMB int) (tp, pp int, model string) {
	totalVRAM := gpuCount * vramMB

	switch {
	case totalVRAM >= 320000: // 320GB+ (e.g., 4x H100 80GB)
		// Large model needs full TP across all GPUs
		return 8, 1, "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"
	case totalVRAM >= 160000 && gpuCount >= 8: // 160GB+ with 8+ GPUs
		// Use pipeline parallelism for better throughput
		return gpuCount / 2, 2, "Qwen/QwQ-32B"
	case totalVRAM >= 80000: // 80GB+ (e.g., 4x RTX 4090)
		return gpuCount, 1, "Qwen/QwQ-32B"
	case totalVRAM >= 40000: // 40GB+ (e.g., 2x RTX 4090)
		return gpuCount, 1, "Qwen/Qwen2.5-7B-Instruct"
	default:
		return 1, 1, "Qwen/Qwen2.5-7B-Instruct"
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
