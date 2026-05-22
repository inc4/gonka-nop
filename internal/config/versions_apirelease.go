package config

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// RaceReleasesManifestURL is the canonical production URL for the
// api-binary hotfix manifest published by the gonka dev team.
// Tests do NOT use this — they construct httptest servers and pass their
// own URLs into cosmovisor.Installer.Install() directly.
const RaceReleasesManifestURL = "https://raw.githubusercontent.com/product-science/race-releases/main/manifest.json"

// postNRe extracts the integer N from a version like "v0.2.12-api-post3".
// Negative result (-1) means no -postN suffix.
var postNRe = regexp.MustCompile(`-post(\d+)$`)

// ParsePostNumber returns (base, postN). If the version has no -postN
// suffix, returns (version, -1). E.g.:
//
//	"v0.2.12-api-post3"  -> ("v0.2.12-api", 3)
//	"v0.2.12-api-post10" -> ("v0.2.12-api", 10)
//	"v0.2.12-api"        -> ("v0.2.12-api", -1)
//	"v0.2.13"            -> ("v0.2.13",    -1)
//
// Used to sort versions correctly: "v0.2.12-api-post10" must rank ABOVE
// "v0.2.12-api-post2", which lexicographic compare gets wrong.
func ParsePostNumber(version string) (string, int) {
	m := postNRe.FindStringSubmatchIndex(version)
	if m == nil {
		return version, -1
	}
	base := version[:m[0]]
	n, err := strconv.Atoi(version[m[2]:m[3]])
	if err != nil {
		return version, -1
	}
	return base, n
}

// SortVersionsByPostN sorts a slice of version strings in ASCENDING order
// by (base, postN). Caller typically wants the LAST element for "latest".
func SortVersionsByPostN(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool {
		baseI, nI := ParsePostNumber(versions[i])
		baseJ, nJ := ParsePostNumber(versions[j])
		if baseI != baseJ {
			return baseI < baseJ
		}
		return nI < nJ
	})
}

// LatestVersion returns the lexicographically/postN-greatest version,
// or "" if the input is empty. Convenience wrapper around SortVersionsByPostN.
func LatestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	sorted := make([]string, len(versions))
	copy(sorted, versions)
	SortVersionsByPostN(sorted)
	return sorted[len(sorted)-1]
}

// ValidateRaceReleasesManifestURL is a defensive guard called by the update
// command before invoking the installer. Production code passes
// RaceReleasesManifestURL which trivially passes; tests passing custom
// URLs are responsible for their own validation.
func ValidateRaceReleasesManifestURL(url string) error {
	if url == "" {
		return fmt.Errorf("manifest URL is empty")
	}
	return nil
}
