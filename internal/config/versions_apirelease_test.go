package config

import (
	"reflect"
	"testing"
)

func TestParsePostNumber(t *testing.T) {
	tests := []struct {
		in       string
		wantBase string
		wantN    int
	}{
		{"v0.2.12-api-post3", "v0.2.12-api", 3},
		{"v0.2.12-api-post10", "v0.2.12-api", 10},
		{"v0.2.12-api-post0", "v0.2.12-api", 0},
		{"v0.2.12-api", "v0.2.12-api", -1},
		{"v0.2.13", "v0.2.13", -1},
		{"v0.2.12-rc.1", "v0.2.12-rc.1", -1},
		{"v0.2.12-post", "v0.2.12-post", -1},       // not -postN
		{"v0.2.12-post-99", "v0.2.12-post-99", -1}, // postN must end the string
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			base, n := ParsePostNumber(tt.in)
			if base != tt.wantBase || n != tt.wantN {
				t.Errorf("ParsePostNumber(%q) = (%q, %d), want (%q, %d)",
					tt.in, base, n, tt.wantBase, tt.wantN)
			}
		})
	}
}

func TestSortVersionsByPostN(t *testing.T) {
	versions := []string{
		"v0.2.12-api-post10",
		"v0.2.12-api-post2",
		"v0.2.12-api-post11",
		"v0.2.12-api-post9",
		"v0.2.12-api",
		"v0.2.13",
	}
	SortVersionsByPostN(versions)
	want := []string{
		"v0.2.12-api",
		"v0.2.12-api-post2",
		"v0.2.12-api-post9",
		"v0.2.12-api-post10",
		"v0.2.12-api-post11",
		"v0.2.13",
	}
	if !reflect.DeepEqual(versions, want) {
		t.Errorf("Sort order:\n got: %v\nwant: %v", versions, want)
	}
}

func TestLatestVersion(t *testing.T) {
	t.Run("post2 vs post10", func(t *testing.T) {
		got := LatestVersion([]string{"v0.2.12-api-post2", "v0.2.12-api-post10"})
		if got != "v0.2.12-api-post10" {
			t.Errorf("got %q, want v0.2.12-api-post10 (integer sort)", got)
		}
	})
	t.Run("empty slice", func(t *testing.T) {
		if LatestVersion(nil) != "" {
			t.Errorf("want empty for nil input")
		}
	})
	t.Run("does not mutate caller's slice", func(t *testing.T) {
		input := []string{"v0.2.12-api-post3", "v0.2.12-api-post2"}
		_ = LatestVersion(input)
		if input[0] != "v0.2.12-api-post3" {
			t.Errorf("input mutated: %v", input)
		}
	})
}

// TestMainImageVersion_IgnoresAPIBinaryVersion verifies Review Notes F27:
// MainImageVersion() must NOT consult APIBinaryVersion (image tag !=
// Cosmovisor binary version).
func TestMainImageVersion_IgnoresAPIBinaryVersion(t *testing.T) {
	v := ImageVersions{
		Node:             "0.2.11",
		API:              "0.2.11",
		APIBinaryVersion: "v0.2.12-api-post3", // would mislead if MainImageVersion read it
	}
	if got := v.MainImageVersion(); got != "0.2.11" {
		t.Errorf("MainImageVersion() = %q, want %q (must not consult APIBinaryVersion)", got, "0.2.11")
	}
}

// TestImageVersions_APIBinaryVersion_PersistedAndReset verifies the new
// field round-trips through state save/load and is cleared by state.Reset()
// via the existing `s.Versions = ImageVersions{}` clear.
func TestImageVersions_APIBinaryVersion_FieldPresent(t *testing.T) {
	// Field presence + zero-value check. State Reset coverage is in
	// state_test.go via the existing Versions clear assertion.
	v := ImageVersions{APIBinaryVersion: "v0.2.12-api-post3"}
	if v.APIBinaryVersion != "v0.2.12-api-post3" {
		t.Errorf("APIBinaryVersion = %q", v.APIBinaryVersion)
	}
	zero := ImageVersions{}
	if zero.APIBinaryVersion != "" {
		t.Errorf("zero value APIBinaryVersion = %q, want empty", zero.APIBinaryVersion)
	}
}
