package phases

// ModelRecipe declares the per-model constraints and vLLM argument profile
// for a governance-approved Gonka model.
//
// Two backward-compat modes are supported:
//   - Legacy (Qwen3-235B / Qwen3-32B-FP8 / QwQ-32B): VLLMArgs is nil. The
//     recommender derives TP/MemoryUtil/MaxModelLen from the GPU tier and
//     buildVLLMArgs() emits args from state. Existing operators upgrade with
//     zero config changes (NFR-M11-5).
//   - Recipe-driven (Kimi-K2.6 and any future model with non-nil VLLMArgs):
//     the args list is emitted verbatim into node-config.json. The recipe
//     is authoritative; state.TPSize / state.GPUMemoryUtil are populated
//     for UI display but bypassed by buildVLLMArgs.
//
// GPUArchAllowed uses plain string IDs matching config.GPUInfo.Architecture
// ("sm_80", "sm_86", "sm_89", "sm_90", "sm_100", "sm_103", "sm_120"). No
// typed GPUArch is introduced — see architect Review Notes F3.
type ModelRecipe struct {
	// GPUArchAllowed restricts the recipe to specific GPU architectures.
	// Empty slice means "any architecture is allowed".
	GPUArchAllowed []string

	// VLLMArgs, when non-nil, is emitted verbatim into the MLNode's
	// node-config.json args list. The order is significant.
	VLLMArgs []string

	// MinVRAMPerGPUGB is the floor per individual GPU (e.g. 80 for Kimi).
	MinVRAMPerGPUGB int

	// MinTotalVRAMGB is the floor summed across all GPUs on the MLNode.
	// recommendConfig() compares this to gpuCount * vramMB.
	MinTotalVRAMGB int

	// ImageVariant selects the MLNode container image variant.
	// "standard" → ghcr.io/product-science/mlnode:<tag>
	// "blackwell" → ghcr.io/product-science/mlnode:<tag>-blackwell
	// Empty defaults to "standard".
	ImageVariant string
}

// kimiVLLMArgs is the canonical 20-token argument list for moonshotai/Kimi-K2.6
// (12 flag groups: 5 boolean flags + 7 flag-value pairs).
//
// Source of truth: docs/PRD.md FR-M11-3, mirrored in
// docs/qa/multi_model_poc_test_cases.md TC-1.1, and pinned in
// testdata/kimi_vllm_args.golden.json. Any divergence between these three
// representations is a release blocker — the golden test guards the contract.
var kimiVLLMArgs = []string{
	"--enable-auto-tool-choice",
	"--tensor-parallel-size", "4",
	"--enable-expert-parallel",
	"--trust-remote-code",
	"--mm-encoder-tp-mode", "data",
	"--tool-call-parser", "kimi_k2",
	"--reasoning-parser", "kimi_k2",
	"--attention-backend", "FLASHINFER_MLA",
	"--disable-custom-all-reduce",
	"--gpu-memory-utilization", "0.95",
	"--max-num-seqs", "128",
	"--max-model-len", "240000",
}

// SupportedModels is the registry of governance-approved models that NOP
// knows how to deploy. The map key is the upstream model ID exactly as
// declared on-chain (the same string used in node-config.json's `models`
// map and in `inferenced tx inference set-poc-delegation [model-id] ...`).
//
// To add a new model: append a new entry here, update PRD FR-M11-3 (and
// AC-M11-1), append a Kimi-style use-case to docs/use-cases/, and pin the
// vLLM args golden file when VLLMArgs is populated.
var SupportedModels = map[string]ModelRecipe{
	// Legacy entries: VLLMArgs left nil so existing operators continue
	// to consume buildVLLMArgs()'s state-driven path without changes.
	// MinVRAM fields encode the historical tier minimums for documentation
	// even though the recommender's switch is still authoritative for them.
	"Qwen/Qwen3-235B-A22B-Instruct-2507-FP8": {
		MinVRAMPerGPUGB: 40,
		MinTotalVRAMGB:  320,
		ImageVariant:    "standard",
	},
	"Qwen/Qwen3-32B-FP8": {
		MinVRAMPerGPUGB: 20,
		MinTotalVRAMGB:  40,
		ImageVariant:    "standard",
	},
	"Qwen/QwQ-32B": {
		MinVRAMPerGPUGB: 20,
		MinTotalVRAMGB:  80,
		ImageVariant:    "standard",
	},

	// Recipe-driven entry: VLLMArgs are authoritative for emission.
	// Blackwell-only — recommender rejects on A100/H100/H200/L40 etc.
	"moonshotai/Kimi-K2.6": {
		GPUArchAllowed:  []string{"sm_100", "sm_103", "sm_120"},
		VLLMArgs:        kimiVLLMArgs,
		MinVRAMPerGPUGB: 80,
		MinTotalVRAMGB:  320,
		ImageVariant:    "blackwell",
	},
}

// SupportedModelNames returns the model IDs from SupportedModels in
// deterministic (alphabetical) order. Used by the interactive download-model
// selector and the setup wizard menu (kept stable so screenshots and
// non-interactive scripts don't break across releases).
func SupportedModelNames() []string {
	out := make([]string, 0, len(SupportedModels))
	for k := range SupportedModels {
		out = append(out, k)
	}
	// stdlib sort to avoid a slices import here (cheap for a 4-entry slice;
	// callers that need it via slices.Sorted can use SupportedModelNames directly).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// LookupRecipe returns the recipe for a model ID, or zero value + false if
// not registered. Convenience wrapper to keep call sites two-line.
func LookupRecipe(modelID string) (ModelRecipe, bool) {
	r, ok := SupportedModels[modelID]
	return r, ok
}
