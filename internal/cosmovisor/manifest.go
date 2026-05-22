package cosmovisor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ManifestSchemaVersion is the only schema version this installer accepts.
// Upstream signals a breaking change by bumping to 2+ — the installer then
// refuses to parse the manifest, forcing operators to upgrade NOP before
// applying further hotfixes (R-M11-3 mitigation).
const ManifestSchemaVersion = 1

// Manifest is the parsed view of a race-releases hotfix manifest.
type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	Releases      []ManifestRelease `json:"releases"`
}

// ManifestRelease is one downloadable API binary release.
type ManifestRelease struct {
	Version   string `json:"version"`    // e.g. "v0.2.12-api-post3"
	BinaryURL string `json:"binary_url"` // full https://... allowlist-checked
	SHA256    string `json:"sha256"`     // hex (lowercase)
	SizeBytes int64  `json:"size_bytes"` // optional sanity-check; 0 = not reported
}

// ParseManifest decodes a manifest body and enforces the versioned-contract
// schema check (Review Notes F16 + architect condition #4). Returns an error
// when:
//   - the body is not JSON
//   - schema_version is missing or != ManifestSchemaVersion
//   - any release has empty version / binary_url / sha256
//   - the sha256 string is not 64 hex chars
func ParseManifest(body []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		// Forward-compat: unknown fields are common when upstream adds metadata.
		// Retry once without DisallowUnknownFields; if THAT also fails, surface
		// the original error.
		var m2 Manifest
		if err2 := json.Unmarshal(body, &m2); err2 != nil {
			return nil, fmt.Errorf("parse manifest JSON: %w", err)
		}
		m = m2
	}

	if m.SchemaVersion == 0 {
		return nil, fmt.Errorf("manifest schema_version missing or zero — refusing to parse (expected %d)", ManifestSchemaVersion)
	}
	if m.SchemaVersion != ManifestSchemaVersion {
		return nil, fmt.Errorf("manifest schema_version %d not supported (this installer requires %d) — upgrade NOP before applying further hotfixes", m.SchemaVersion, ManifestSchemaVersion)
	}

	for i, r := range m.Releases {
		if r.Version == "" {
			return nil, fmt.Errorf("manifest releases[%d]: version empty", i)
		}
		if r.BinaryURL == "" {
			return nil, fmt.Errorf("manifest releases[%d] %q: binary_url empty", i, r.Version)
		}
		if r.SHA256 == "" {
			return nil, fmt.Errorf("manifest releases[%d] %q: sha256 empty", i, r.Version)
		}
		if len(r.SHA256) != 64 {
			return nil, fmt.Errorf("manifest releases[%d] %q: sha256 must be 64 hex chars, got %d", i, r.Version, len(r.SHA256))
		}
	}

	return &m, nil
}

// FindRelease returns the release with the exact matching version, or
// (nil, false) when absent. Caller is responsible for ValidateReleaseURL
// on the BinaryURL before downloading.
func (m *Manifest) FindRelease(version string) (*ManifestRelease, bool) {
	for i := range m.Releases {
		if m.Releases[i].Version == version {
			return &m.Releases[i], true
		}
	}
	return nil, false
}
