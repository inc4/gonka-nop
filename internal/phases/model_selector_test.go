package phases

import (
	"reflect"
	"testing"
)

func TestBuildModelMenuOptions_OmitsKimiOnNonBlackwell(t *testing.T) {
	// Recipes without GPUArchAllowed restrictions are always included
	// (Qwen3-235B, Qwen3-32B-FP8, QwQ-32B). Kimi-K2.6 (sm_100/sm_103/sm_120)
	// is filtered out on non-Blackwell arches.
	tests := []struct {
		name string
		arch string
		want []string
	}{
		{"A100 (sm_80)", "sm_80", []string{qwqModelID, testnetLargeModel, qwen32BID}},
		{"L40 (sm_89)", "sm_89", []string{qwqModelID, testnetLargeModel, qwen32BID}},
		{"H100 (sm_90)", "sm_90", []string{qwqModelID, testnetLargeModel, qwen32BID}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildModelMenuOptions(tt.arch)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildModelMenuOptions(%q):\n got: %v\nwant: %v", tt.arch, got, tt.want)
			}
		})
	}
}

func TestBuildModelMenuOptions_IncludesKimiOnBlackwell(t *testing.T) {
	tests := []string{"sm_100", "sm_103", "sm_120"}
	for _, arch := range tests {
		t.Run(arch, func(t *testing.T) {
			got := BuildModelMenuOptions(arch)
			found := false
			for _, id := range got {
				if id == kimiModelID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("BuildModelMenuOptions(%q) = %v; expected Kimi to be included on Blackwell", arch, got)
			}
		})
	}
}

func TestBuildModelMenuOptions_AlwaysIncludesUnrestrictedRecipes(t *testing.T) {
	// Even on an exotic / unknown arch, recipes WITHOUT GPUArchAllowed
	// restrictions remain in the menu (the recommender will gate on VRAM).
	got := BuildModelMenuOptions("sm_999")
	want := []string{qwqModelID, testnetLargeModel, qwen32BID}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildModelMenuOptions(sm_999):\n got: %v\nwant: %v", got, want)
	}
}

func TestArchAllowedByRecipe(t *testing.T) {
	t.Run("empty allowed list = any arch", func(t *testing.T) {
		r := ModelRecipe{GPUArchAllowed: nil}
		if !archAllowedByRecipe(r, "sm_80") {
			t.Error("expected unrestricted recipe to allow any arch")
		}
	})

	t.Run("arch in list", func(t *testing.T) {
		r := ModelRecipe{GPUArchAllowed: []string{"sm_100", "sm_120"}}
		if !archAllowedByRecipe(r, "sm_120") {
			t.Error("expected sm_120 to be allowed")
		}
	})

	t.Run("arch not in list", func(t *testing.T) {
		r := ModelRecipe{GPUArchAllowed: []string{"sm_100", "sm_120"}}
		if archAllowedByRecipe(r, "sm_80") {
			t.Error("expected sm_80 to be rejected")
		}
	})
}
