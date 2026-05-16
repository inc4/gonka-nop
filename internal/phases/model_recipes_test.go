package phases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	kimiModelID      = "moonshotai/Kimi-K2.6"
	qwen32BID        = "Qwen/Qwen3-32B-FP8"
	qwqModelID       = "Qwen/QwQ-32B"
	kimiArgCount     = 20 // 12 flag groups; see model_recipes.go kimiVLLMArgs
	blackwellVariant = "blackwell"
)

// testnetLargeModel is defined in 03_network_select_test.go (same package).

// TestKimiRecipe_VLLMArgsGoldenOrder pins the exact 20-token argument list
// for moonshotai/Kimi-K2.6 against the canonical golden file.
// Source of truth: docs/PRD.md FR-M11-3 + docs/qa/.. TC-1.1.
func TestKimiRecipe_VLLMArgsGoldenOrder(t *testing.T) {
	r, ok := SupportedModels[kimiModelID]
	if !ok {
		t.Fatalf("Kimi-K2.6 not present in SupportedModels")
	}

	goldenPath := filepath.Join("testdata", "kimi_vllm_args.golden.json")
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}

	var golden []string
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("parse golden file: %v", err)
	}

	if len(r.VLLMArgs) != kimiArgCount {
		t.Errorf("Kimi VLLMArgs length = %d, want %d (FR-M11-3 specifies 12 flag groups / 20 tokens)",
			len(r.VLLMArgs), kimiArgCount)
	}

	if !reflect.DeepEqual(r.VLLMArgs, golden) {
		t.Errorf("Kimi VLLMArgs mismatch against golden file.\n got: %v\nwant: %v", r.VLLMArgs, golden)
	}
}

// TestSupportedModels_AllFourPresent verifies the registry membership.
// New governance-approved models should be added explicitly.
func TestSupportedModels_AllFourPresent(t *testing.T) {
	wantKeys := []string{kimiModelID, testnetLargeModel, qwen32BID, qwqModelID}
	for _, k := range wantKeys {
		if _, ok := SupportedModels[k]; !ok {
			t.Errorf("SupportedModels missing %q", k)
		}
	}
	if len(SupportedModels) != len(wantKeys) {
		t.Errorf("SupportedModels has %d entries, want exactly %d (%v)",
			len(SupportedModels), len(wantKeys), wantKeys)
	}
}

// TestSupportedModelNames_DeterministicOrder verifies the ASCII-sort
// contract used by interactive menus and non-interactive scripts. Stable
// order matters more than which order — UI screenshots and `--yes` flag
// matchers depend on it being identical across releases.
//
// Order is plain byte-wise: "Qwen/QwQ-32B" precedes "Qwen/Qwen3-..." because
// at byte index 7 the comparison is 'Q' (0x51) vs 'e' (0x65).
func TestSupportedModelNames_DeterministicOrder(t *testing.T) {
	names := SupportedModelNames()
	want := []string{qwqModelID, testnetLargeModel, qwen32BID, kimiModelID}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("SupportedModelNames() order:\n got: %v\nwant: %v", names, want)
	}
}

// TestKimiRecipe_BlackwellOnly verifies the architecture restriction.
func TestKimiRecipe_BlackwellOnly(t *testing.T) {
	r := SupportedModels[kimiModelID]
	want := []string{"sm_100", "sm_103", "sm_120"}
	if !reflect.DeepEqual(r.GPUArchAllowed, want) {
		t.Errorf("Kimi GPUArchAllowed = %v, want %v", r.GPUArchAllowed, want)
	}
	if r.ImageVariant != blackwellVariant {
		t.Errorf("Kimi ImageVariant = %q, want %q", r.ImageVariant, blackwellVariant)
	}
}

// TestLegacyRecipes_NoVLLMArgs verifies the backward-compat contract
// (NFR-M11-5): existing Qwen models leave VLLMArgs nil so buildVLLMArgs
// uses the historical state-driven path.
func TestLegacyRecipes_NoVLLMArgs(t *testing.T) {
	for _, id := range []string{testnetLargeModel, qwen32BID, qwqModelID} {
		r := SupportedModels[id]
		if r.VLLMArgs != nil {
			t.Errorf("recipe %q VLLMArgs = %v, want nil (legacy state-driven path)", id, r.VLLMArgs)
		}
		if r.ImageVariant != "standard" {
			t.Errorf("recipe %q ImageVariant = %q, want %q", id, r.ImageVariant, "standard")
		}
	}
}

// TestLookupRecipe verifies the convenience wrapper.
func TestLookupRecipe(t *testing.T) {
	r, ok := LookupRecipe(kimiModelID)
	if !ok {
		t.Fatal("LookupRecipe(Kimi) returned not-found")
	}
	if r.ImageVariant != blackwellVariant {
		t.Errorf("LookupRecipe(Kimi).ImageVariant = %q, want %q", r.ImageVariant, blackwellVariant)
	}

	if _, ok := LookupRecipe("not/a/real/model"); ok {
		t.Error("LookupRecipe(unknown) returned ok=true, want false")
	}
}
