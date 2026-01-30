package phases

import (
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

func TestRecommendConfig(t *testing.T) {
	tests := []struct {
		name           string
		gpuCount       int
		vramMB         int
		arch           string
		hasNVLink      bool
		wantTP         int
		wantPP         int
		wantModel      string
		wantMemUtil    float64
		wantKVCache    string
		wantMaxModelLe int
	}{
		{
			name:           "Single small GPU (<40GB)",
			gpuCount:       1,
			vramMB:         8000,
			arch:           "sm_89",
			hasNVLink:      false,
			wantTP:         1,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-32B-FP8",
			wantMemUtil:    0.94,
			wantKVCache:    "auto",
			wantMaxModelLe: 8192,
		},
		{
			name:           "2x RTX 4090 (49GB total)",
			gpuCount:       2,
			vramMB:         24564,
			arch:           "sm_89",
			hasNVLink:      false,
			wantTP:         2,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-32B-FP8",
			wantMemUtil:    0.92,
			wantKVCache:    "auto",
			wantMaxModelLe: 24576,
		},
		{
			name:           "4x RTX 4090 (98GB total)",
			gpuCount:       4,
			vramMB:         24564,
			arch:           "sm_89",
			hasNVLink:      false,
			wantTP:         4,
			wantPP:         1,
			wantModel:      "Qwen/QwQ-32B",
			wantMemUtil:    0.90,
			wantKVCache:    "auto",
			wantMaxModelLe: 24576,
		},
		{
			name:           "4x H100 80GB (320GB) no NVLink",
			gpuCount:       4,
			vramMB:         81920,
			arch:           "sm_90",
			hasNVLink:      false,
			wantTP:         4,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			wantMemUtil:    0.88,
			wantKVCache:    "auto",
			wantMaxModelLe: 16384,
		},
		{
			name:           "8x H100 80GB with NVLink",
			gpuCount:       8,
			vramMB:         81920,
			arch:           "sm_90",
			hasNVLink:      true,
			wantTP:         8,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			wantMemUtil:    0.90,
			wantKVCache:    "auto",
			wantMaxModelLe: 240000,
		},
		{
			name:           "8x H100 80GB without NVLink",
			gpuCount:       8,
			vramMB:         81920,
			arch:           "sm_90",
			hasNVLink:      false,
			wantTP:         4,
			wantPP:         2,
			wantModel:      "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			wantMemUtil:    0.90,
			wantKVCache:    "auto",
			wantMaxModelLe: 131072,
		},
		{
			name:           "8x A100 40GB (320GB) — tight VRAM needs fp8 KV cache",
			gpuCount:       8,
			vramMB:         40960,
			arch:           "sm_80",
			hasNVLink:      true,
			wantTP:         8,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			wantMemUtil:    0.90,
			wantKVCache:    "fp8",
			wantMaxModelLe: 240000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := recommendConfig(tt.gpuCount, tt.vramMB, tt.arch, tt.hasNVLink)

			if rec.TP != tt.wantTP {
				t.Errorf("TP = %d, want %d", rec.TP, tt.wantTP)
			}
			if rec.PP != tt.wantPP {
				t.Errorf("PP = %d, want %d", rec.PP, tt.wantPP)
			}
			if rec.Model != tt.wantModel {
				t.Errorf("Model = %s, want %s", rec.Model, tt.wantModel)
			}
			if rec.MemoryUtil != tt.wantMemUtil {
				t.Errorf("MemoryUtil = %.2f, want %.2f", rec.MemoryUtil, tt.wantMemUtil)
			}
			if rec.KVCacheDtype != tt.wantKVCache {
				t.Errorf("KVCacheDtype = %s, want %s", rec.KVCacheDtype, tt.wantKVCache)
			}
			if rec.MaxModelLen != tt.wantMaxModelLe {
				t.Errorf("MaxModelLen = %d, want %d", rec.MaxModelLen, tt.wantMaxModelLe)
			}
		})
	}
}

func TestDetectTopology(t *testing.T) {
	tests := []struct {
		name          string
		gpus          []config.GPUInfo
		wantNVLink    bool
		wantPCIe      string
		wantInterconn string
	}{
		{
			name:          "Single GPU",
			gpus:          []config.GPUInfo{{Index: 0, Name: "NVIDIA GeForce RTX 4090"}},
			wantNVLink:    false,
			wantPCIe:      "4.0",
			wantInterconn: "pcie",
		},
		{
			name: "Multi RTX 4090 (no NVLink)",
			gpus: []config.GPUInfo{
				{Index: 0, Name: "NVIDIA GeForce RTX 4090"},
				{Index: 1, Name: "NVIDIA GeForce RTX 4090"},
			},
			wantNVLink:    false,
			wantPCIe:      "4.0",
			wantInterconn: "pcie",
		},
		{
			name: "Multi H100 (NVLink)",
			gpus: []config.GPUInfo{
				{Index: 0, Name: "NVIDIA H100 80GB"},
				{Index: 1, Name: "NVIDIA H100 80GB"},
			},
			wantNVLink:    true,
			wantPCIe:      "5.0",
			wantInterconn: "nvlink",
		},
		{
			name: "Multi A100 (NVLink)",
			gpus: []config.GPUInfo{
				{Index: 0, Name: "NVIDIA A100 40GB"},
				{Index: 1, Name: "NVIDIA A100 40GB"},
			},
			wantNVLink:    true,
			wantPCIe:      "5.0",
			wantInterconn: "nvlink",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := detectTopology(tt.gpus)

			if topo.HasNVLink != tt.wantNVLink {
				t.Errorf("HasNVLink = %v, want %v", topo.HasNVLink, tt.wantNVLink)
			}
			if topo.PCIeVersion != tt.wantPCIe {
				t.Errorf("PCIeVersion = %s, want %s", topo.PCIeVersion, tt.wantPCIe)
			}
			if topo.Interconnect != tt.wantInterconn {
				t.Errorf("Interconnect = %s, want %s", topo.Interconnect, tt.wantInterconn)
			}
		})
	}
}

func TestSelectMLNodeImage(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"sm_80", "3.0.12"},
		{"sm_89", "3.0.12"},
		{"sm_90", "3.0.12"},
		{"sm_90a", "3.0.12"},
		{"sm_100", "3.0.12-blackwell"},
		{"sm_120", "3.0.12-blackwell"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := selectMLNodeImage(tt.arch)
			if got != tt.want {
				t.Errorf("selectMLNodeImage(%s) = %s, want %s", tt.arch, got, tt.want)
			}
		})
	}
}

func TestSelectAttentionBackend(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"sm_80", "FLASH_ATTN"},
		{"sm_89", "FLASH_ATTN"},
		{"sm_90", "FLASH_ATTN"},
		{"sm_100", "FLASHINFER"},
		{"sm_120", "FLASHINFER"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := selectAttentionBackend(tt.arch)
			if got != tt.want {
				t.Errorf("selectAttentionBackend(%s) = %s, want %s", tt.arch, got, tt.want)
			}
		})
	}
}

func TestFormatGPUSummary(t *testing.T) {
	tests := []struct {
		name string
		gpus []config.GPUInfo
		want string
	}{
		{
			name: "No GPUs",
			gpus: []config.GPUInfo{},
			want: "No GPUs detected",
		},
		{
			name: "Single GPU",
			gpus: []config.GPUInfo{
				{Index: 0, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
			},
			want: "1x NVIDIA GeForce RTX 4090 (24.0 GB total)",
		},
		{
			name: "Multiple GPUs",
			gpus: []config.GPUInfo{
				{Index: 0, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
				{Index: 1, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
				{Index: 2, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
				{Index: 3, Name: "NVIDIA GeForce RTX 4090", MemoryMB: 24564},
			},
			want: "4x NVIDIA GeForce RTX 4090 (96.0 GB total)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGPUSummary(tt.gpus)
			if got != tt.want {
				t.Errorf("FormatGPUSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}
