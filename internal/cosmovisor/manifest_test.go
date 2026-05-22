package cosmovisor

import (
	"strings"
	"testing"
)

const happyManifest = `{
  "schema_version": 1,
  "releases": [
    {
      "version": "v0.2.12-api-post2",
      "binary_url": "https://github.com/product-science/race-releases/releases/download/release%2Fv0.2.12-api-post2/decentralized-api-amd64.zip",
      "sha256": "7bef88106fc3464d0141a2d14245cc06c341be186250f5d096e27e901deb185e",
      "size_bytes": 191000000
    },
    {
      "version": "v0.2.12-api-post3",
      "binary_url": "https://github.com/product-science/race-releases/releases/download/release%2Fv0.2.12-api-post3/decentralized-api-amd64.zip",
      "sha256": "3f2bc481b8320c53f0abe428dc262eaac5a86e8f38b8d796c409bd7116ba5017",
      "size_bytes": 191500000
    }
  ]
}`

func TestParseManifest_HappyPath(t *testing.T) {
	m, err := ParseManifest([]byte(happyManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", m.SchemaVersion)
	}
	if len(m.Releases) != 2 {
		t.Fatalf("Releases length = %d, want 2", len(m.Releases))
	}
	if m.Releases[1].Version != "v0.2.12-api-post3" {
		t.Errorf("Releases[1].Version = %q", m.Releases[1].Version)
	}
}

func TestParseManifest_MissingSchemaVersion(t *testing.T) {
	body := `{"releases": [{"version": "v1", "binary_url": "https://github.com/gonka-ai/gonka/x", "sha256": "0000000000000000000000000000000000000000000000000000000000000000"}]}`
	_, err := ParseManifest([]byte(body))
	if err == nil {
		t.Fatal("expected error on missing schema_version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("error should mention schema_version: %v", err)
	}
}

func TestParseManifest_UnknownSchemaVersion(t *testing.T) {
	body := `{"schema_version": 2, "releases": []}`
	_, err := ParseManifest([]byte(body))
	if err == nil {
		t.Fatal("expected error on schema_version 2")
	}
	if !strings.Contains(err.Error(), "schema_version 2 not supported") {
		t.Errorf("error should reject schema_version 2 explicitly: %v", err)
	}
}

func TestParseManifest_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty version",
			body: `{"schema_version": 1, "releases": [{"version": "", "binary_url": "https://x", "sha256": "` + strings.Repeat("0", 64) + `"}]}`,
		},
		{
			name: "empty binary_url",
			body: `{"schema_version": 1, "releases": [{"version": "v1", "binary_url": "", "sha256": "` + strings.Repeat("0", 64) + `"}]}`,
		},
		{
			name: "empty sha256",
			body: `{"schema_version": 1, "releases": [{"version": "v1", "binary_url": "https://x", "sha256": ""}]}`,
		},
		{
			name: "short sha256",
			body: `{"schema_version": 1, "releases": [{"version": "v1", "binary_url": "https://x", "sha256": "abc"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tt.body))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseManifest_NotJSON(t *testing.T) {
	_, err := ParseManifest([]byte("not json at all"))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestManifest_FindRelease(t *testing.T) {
	m, err := ParseManifest([]byte(happyManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	r, ok := m.FindRelease("v0.2.12-api-post3")
	if !ok {
		t.Fatal("FindRelease post3 returned not-found")
	}
	if r.SHA256 != "3f2bc481b8320c53f0abe428dc262eaac5a86e8f38b8d796c409bd7116ba5017" {
		t.Errorf("FindRelease returned wrong sha: %s", r.SHA256)
	}

	if _, ok := m.FindRelease("v9.9.9-nonexistent"); ok {
		t.Error("FindRelease should return false for unknown version")
	}
}

func TestParseManifest_ForwardCompat_UnknownFieldsTolerated(t *testing.T) {
	// Upstream adds metadata in a non-breaking way; we should still parse.
	body := `{
		"schema_version": 1,
		"published_at": "2026-05-16T00:00:00Z",
		"releases": [
			{"version": "v0.2.12-api-post3", "binary_url": "https://github.com/x/y/z", "sha256": "` + strings.Repeat("a", 64) + `", "size_bytes": 100, "release_notes": "hotfix for kimi parser"}
		]
	}`
	m, err := ParseManifest([]byte(body))
	if err != nil {
		t.Fatalf("expected forward-compat tolerance; got %v", err)
	}
	if len(m.Releases) != 1 {
		t.Errorf("expected 1 release, got %d", len(m.Releases))
	}
}
