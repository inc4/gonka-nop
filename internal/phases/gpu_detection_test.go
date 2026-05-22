package phases

import (
	"strings"
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
			wantKVCache:    kvCacheDtypeAuto,
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
			wantKVCache:    kvCacheDtypeAuto,
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
			wantModel:      defaultModel,
			wantMemUtil:    0.90,
			wantKVCache:    kvCacheDtypeAuto,
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
			wantKVCache:    kvCacheDtypeAuto,
			wantMaxModelLe: 16384,
		},
		{
			name:           "8x H100 80GB with NVLink",
			gpuCount:       8,
			vramMB:         81920,
			arch:           "sm_90",
			hasNVLink:      true,
			wantTP:         4,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			wantMemUtil:    0.90,
			wantKVCache:    kvCacheDtypeAuto,
			wantMaxModelLe: 240000,
		},
		{
			name:           "8x H100 80GB without NVLink",
			gpuCount:       8,
			vramMB:         81920,
			arch:           "sm_90",
			hasNVLink:      false,
			wantTP:         4,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			wantMemUtil:    0.90,
			wantKVCache:    kvCacheDtypeAuto,
			wantMaxModelLe: 240000,
		},
		{
			name:           "8x A100 40GB (320GB) — tight VRAM needs fp8 KV cache",
			gpuCount:       8,
			vramMB:         40960,
			arch:           "sm_80",
			hasNVLink:      true,
			wantTP:         4,
			wantPP:         1,
			wantModel:      "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
			wantMemUtil:    0.90,
			wantKVCache:    kvCacheDtypeFP8,
			wantMaxModelLe: 240000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := recommendConfig(tt.gpuCount, tt.vramMB, tt.arch, tt.hasNVLink, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

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

// TestRecommendConfig_KimiArchValidation exercises the M11 Slice 2 recipe
// gating: Kimi-K2.6 is rejected on non-Blackwell arch and accepted on Blackwell.
func TestRecommendConfig_KimiArchValidation(t *testing.T) {
	kimiID := "moonshotai/Kimi-K2.6"

	t.Run("rejected on A100 (sm_80)", func(t *testing.T) {
		_, err := recommendConfig(8, 81920, "sm_80", true, kimiID)
		if err == nil {
			t.Fatal("expected error for Kimi on sm_80")
		}
		if !strings.Contains(err.Error(), "sm_80") || !strings.Contains(err.Error(), "sm_100") {
			t.Errorf("error message should mention both observed arch (sm_80) and required arch (sm_100): %v", err)
		}
	})

	t.Run("rejected on H100 (sm_90)", func(t *testing.T) {
		_, err := recommendConfig(8, 81920, "sm_90", true, kimiID)
		if err == nil {
			t.Fatal("expected error for Kimi on sm_90")
		}
	})

	t.Run("accepted on B200 (sm_100)", func(t *testing.T) {
		rec, err := recommendConfig(8, 81920, "sm_100", true, kimiID)
		if err != nil {
			t.Fatalf("unexpected error for Kimi on sm_100: %v", err)
		}
		if rec.Model != kimiID {
			t.Errorf("Model = %q, want %q", rec.Model, kimiID)
		}
		// VLLMArgs-derived display fields:
		if rec.TP != 4 {
			t.Errorf("derived TP = %d, want 4 (from Kimi recipe args)", rec.TP)
		}
		if rec.MaxModelLen != 240000 {
			t.Errorf("derived MaxModelLen = %d, want 240000", rec.MaxModelLen)
		}
		if rec.MemoryUtil < 0.94 || rec.MemoryUtil > 0.96 {
			t.Errorf("derived MemoryUtil = %.2f, want ~0.95 (from Kimi recipe args)", rec.MemoryUtil)
		}
	})

	t.Run("accepted on sm_120 (forward-looking Blackwell+) with sufficient VRAM", func(t *testing.T) {
		// Hypothetical: 4x B300-class @ 96GB = 384GB total
		_, err := recommendConfig(4, 98304, "sm_120", true, kimiID)
		if err != nil {
			t.Fatalf("unexpected error for Kimi on sm_120: %v", err)
		}
	})
}

// TestRecommendConfig_KimiVRAMRejection verifies the MinTotalVRAMGB gate.
func TestRecommendConfig_KimiVRAMRejection(t *testing.T) {
	// 2x B200 = 160GB < 320GB minimum
	_, err := recommendConfig(2, 81920, "sm_100", true, "moonshotai/Kimi-K2.6")
	if err == nil {
		t.Fatal("expected VRAM-too-low error")
	}
	if !strings.Contains(err.Error(), "320") {
		t.Errorf("error should mention required 320 GB minimum: %v", err)
	}
}

// TestRecommendConfig_UnknownModel rejects unregistered requestedModel.
func TestRecommendConfig_UnknownModel(t *testing.T) {
	_, err := recommendConfig(8, 81920, "sm_100", true, "not/a/real/model")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
}

// TestRecommendConfig_EmptyModelFallsBackToTier preserves NFR-M11-5
// (legacy single-arg behavior unchanged when requestedModel is "").
func TestRecommendConfig_EmptyModelFallsBackToTier(t *testing.T) {
	rec, err := recommendConfig(8, 81920, "sm_80", true, "")
	if err != nil {
		t.Fatalf("unexpected error on legacy path: %v", err)
	}
	if rec.Model != "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8" {
		t.Errorf("legacy tier picked %q, want Qwen3-235B (8x A100-80GB tier)", rec.Model)
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
		name     string
		arch     string
		fetched  string
		registry string
		want     string
	}{
		{"ampere no fetch", "sm_80", "", "", "3.0.12"},
		{"ampere with fetch", "sm_80", "3.0.12-post6", "", "3.0.12-post6"},
		{"hopper no fetch", "sm_90", "", "", "3.0.12"},
		{"hopper with fetch", "sm_90", "3.0.12-post6", "", "3.0.12-post6"},
		{"blackwell no fetch no registry", "sm_100", "", "", "3.0.12-blackwell"},
		{"blackwell with fetch no registry", "sm_100", "3.0.12-post6", "", "3.0.12-post6-blackwell"},
		{"blackwell with registry", "sm_100", "", "3.0.12-post6-blackwell", "3.0.12-post6-blackwell"},
		{"blackwell registry overrides fetch", "sm_100", "3.0.12", "3.0.12-post6-blackwell", "3.0.12-post6-blackwell"},
		{"blackwell fetch already has suffix", "sm_100", "3.0.12-blackwell", "", "3.0.12-blackwell"},
		{"sm120 fallback", "sm_120", "", "", "3.0.12-blackwell"},
		{"sm103 fallback", "sm_103", "", "", "3.0.12-blackwell"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectMLNodeImage(tt.arch, tt.fetched, tt.registry, "")
			if got != tt.want {
				t.Errorf("selectMLNodeImage(%q, %q, %q, \"\") = %q, want %q", tt.arch, tt.fetched, tt.registry, got, tt.want)
			}
		})
	}
}

// TestSelectMLNodeImage_RecipeImageVariant verifies Review Notes F4:
// when requestedModel resolves to a recipe with ImageVariant=="blackwell",
// the blackwell image is selected even if the host arch is not Blackwell
// (defensive — recommendConfig would have rejected such a host already).
func TestSelectMLNodeImage_RecipeImageVariant(t *testing.T) {
	t.Run("Kimi forces blackwell even on sm_80 host", func(t *testing.T) {
		// Hypothetical: recommendConfig would reject this earlier, but
		// the image selector is independently defensive.
		got := selectMLNodeImage("sm_80", "3.0.12-post6", "", "moonshotai/Kimi-K2.6")
		if got != "3.0.12-post6-blackwell" {
			t.Errorf("got %q, want 3.0.12-post6-blackwell", got)
		}
	})

	t.Run("Qwen3-235B on sm_80 stays standard (ImageVariant=standard)", func(t *testing.T) {
		got := selectMLNodeImage("sm_80", "3.0.12-post6", "", "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8")
		if got != "3.0.12-post6" {
			t.Errorf("got %q, want 3.0.12-post6 (standard variant)", got)
		}
	})

	t.Run("unknown model falls back to arch-based logic", func(t *testing.T) {
		got := selectMLNodeImage("sm_80", "3.0.12-post6", "", "not/a/real/model")
		if got != "3.0.12-post6" {
			t.Errorf("got %q, want 3.0.12-post6 (arch fallback)", got)
		}
	})
}

func TestSelectAttentionBackend(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"sm_80", "FLASHINFER"},
		{"sm_89", "FLASHINFER"},
		{"sm_90", "FLASHINFER"},
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
