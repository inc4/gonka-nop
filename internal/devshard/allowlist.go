package devshard

import "regexp"

// apiImageAllowlistRe matches upstream-blessed api images:
//
//	ghcr.io/product-science/api:<semver>      e.g. 0.2.12, v0.2.12, 0.2.12-post3
//	ghcr.io/product-science/api@sha256:<64-hex>
//
// Anchored at both ends with `^...$` (Go default: string anchors, NOT
// line anchors — multiline matching would require `(?m)` which is NOT
// used). This rejects prefix attacks (`evil.io/...`), suffix attacks
// (`...evil`), substring attacks (`...apix...`), newline injection
// (`foo\nghcr...`), and NUL-byte injection. RE2-only — no
// backtracking, ReDoS structurally impossible.
//
// Security-reviewed 2026-05-17 (security-auditor APPROVED-WITH-CONDITIONS).
var apiImageAllowlistRe = regexp.MustCompile(
	`^ghcr\.io/product-science/api(:v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?|@sha256:[a-f0-9]{64})$`,
)

// bareSemverRe matches a bare semver tag like "0.2.12" or "v0.2.12-post3".
// Used by ExpandBareSemver to detect input that should be promoted to a
// canonical upstream reference.
var bareSemverRe = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$`)

// IsUpstreamAPIImage returns true if image is in the upstream api allowlist.
// Anchored regex match — substring/prefix/suffix/newline/NUL-injection
// attacks all return false.
func IsUpstreamAPIImage(image string) bool {
	return apiImageAllowlistRe.MatchString(image)
}

// ExpandBareSemver promotes a bare semver tag like "0.2.12" to the
// canonical upstream form "ghcr.io/product-science/api:0.2.12".
// Returns input unchanged if it doesn't look like a bare semver.
//
// Expansion is intentionally narrow — operators who want non-semver tags
// (e.g., "latest", "otel-test-4") must use `--allow-non-upstream` and pass
// the full image reference.
func ExpandBareSemver(input string) string {
	if bareSemverRe.MatchString(input) {
		return "ghcr.io/product-science/api:" + input
	}
	return input
}
