// Tests for the upstream-api image allowlist regex and bare-semver
// expansion. Per security-auditor pre-review of 2026-05-17, this includes
// the standard TC-SEC-4 truth table plus injection cases (newline, NUL)
// and a ReDoS guard (RE2 makes it structurally impossible, but lock it in).
package devshard

import (
	"strings"
	"testing"
	"time"
)

// TestIsUpstreamAPIImage_TruthTable — TC-SEC-4 14-row truth table.
func TestIsUpstreamAPIImage_TruthTable(t *testing.T) {
	cases := []struct {
		name  string
		image string
		want  bool
	}{
		{"plain semver", "ghcr.io/product-science/api:0.2.12", true},
		{"v-prefixed semver", "ghcr.io/product-science/api:v0.2.12", true},
		{"semver with post suffix", "ghcr.io/product-science/api:0.2.12-post3", true},
		{"v-prefixed semver with post suffix", "ghcr.io/product-science/api:v0.2.12-post3", true},
		{"sha256 digest all a", "ghcr.io/product-science/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"sha256 digest hex", "ghcr.io/product-science/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"sha256 non-hex chars", "ghcr.io/product-science/api@sha256:ZZZaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"sha256 wrong length", "ghcr.io/product-science/api@sha256:aaaa", false},
		{"prefix attack", "evil.io/ghcr.io/product-science/api:0.2.12", false},
		{"suffix attack 4-segment tag", "ghcr.io/product-science/api:0.2.12.evil", false},
		{"substring attack apix", "ghcr.io/product-science/apix:0.2.12", false},
		{"no tag", "ghcr.io/product-science/api", false},
		{"empty tag", "ghcr.io/product-science/api:", false},
		{"different org segovchik", "ghcr.io/segovchik/api:0.2.12", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUpstreamAPIImage(tc.image); got != tc.want {
				t.Errorf("IsUpstreamAPIImage(%q) = %v, want %v", tc.image, got, tc.want)
			}
		})
	}
}

// TestSecurity_ImageAllowlistRegex_NewlineInjection — confirms Go's default
// regex anchors are string-bound (not line-bound), so multi-line input is
// rejected even if a line in the middle would match.
func TestSecurity_ImageAllowlistRegex_NewlineInjection(t *testing.T) {
	cases := []string{
		"foo\nghcr.io/product-science/api:0.2.12",
		"ghcr.io/product-science/api:0.2.12\n",
		"ghcr.io/product-science/api:0.2.12\nrm",
	}
	for _, in := range cases {
		t.Run(strings.ReplaceAll(in, "\n", "\\n"), func(t *testing.T) {
			if IsUpstreamAPIImage(in) {
				t.Errorf("newline-injected input was accepted: %q", in)
			}
		})
	}
}

// TestSecurity_ImageAllowlistRegex_NulByteInjection.
func TestSecurity_ImageAllowlistRegex_NulByteInjection(t *testing.T) {
	in := "ghcr.io/product-science/api:0.2.12\x00rm"
	if IsUpstreamAPIImage(in) {
		t.Errorf("NUL-byte-injected input was accepted: %q", in)
	}
}

// TestSecurity_ImageAllowlistRegex_NoReDoS — RE2 makes ReDoS structurally
// impossible, but lock it in. 10000 chars in <10ms is comfortable.
func TestSecurity_ImageAllowlistRegex_NoReDoS(t *testing.T) {
	pathological := strings.Repeat("0.2.12-aaaa", 10000)
	start := time.Now()
	_ = IsUpstreamAPIImage(pathological)
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("ReDoS guard: regex took %v on 10k-char input, want <10ms", elapsed)
	}
}

// TestExpandBareSemver_TruthTable.
func TestExpandBareSemver_TruthTable(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain semver", "0.2.12", "ghcr.io/product-science/api:0.2.12"},
		{"v-prefixed semver", "v0.2.12", "ghcr.io/product-science/api:v0.2.12"},
		{"semver with post", "0.2.12-post3", "ghcr.io/product-science/api:0.2.12-post3"},
		{"v-prefixed with post", "v0.2.12-post3", "ghcr.io/product-science/api:v0.2.12-post3"},
		{"latest unchanged", "latest", "latest"},
		{"otel-test-4 unchanged", "otel-test-4", "otel-test-4"},
		{"full ref unchanged", "ghcr.io/product-science/api:0.2.12", "ghcr.io/product-science/api:0.2.12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExpandBareSemver(tc.in); got != tc.want {
				t.Errorf("ExpandBareSemver(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestExpandBareSemver_TwoSegment_Unchanged.
func TestExpandBareSemver_TwoSegment_Unchanged(t *testing.T) {
	if got := ExpandBareSemver("0.2"); got != "0.2" {
		t.Errorf("two-segment expanded incorrectly: %q", got)
	}
}

// TestExpandBareSemver_FourSegment_Unchanged.
func TestExpandBareSemver_FourSegment_Unchanged(t *testing.T) {
	if got := ExpandBareSemver("0.2.12.4"); got != "0.2.12.4" {
		t.Errorf("four-segment expanded incorrectly: %q", got)
	}
}

// TestExpandThenAllowlist_BareSemverFlow — integration sanity.
func TestExpandThenAllowlist_BareSemverFlow(t *testing.T) {
	if !IsUpstreamAPIImage(ExpandBareSemver("0.2.12")) {
		t.Error(`IsUpstreamAPIImage(ExpandBareSemver("0.2.12")) should be true`)
	}
	if IsUpstreamAPIImage(ExpandBareSemver("otel-test-4")) {
		t.Error(`IsUpstreamAPIImage(ExpandBareSemver("otel-test-4")) should be false`)
	}
}

// TestExpandThenAllowlist_DigestPin.
func TestExpandThenAllowlist_DigestPin(t *testing.T) {
	digest := "ghcr.io/product-science/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if !IsUpstreamAPIImage(digest) {
		t.Errorf("digest pin %q should pass allowlist", digest)
	}
}

// TestExpandThenAllowlist_NonUpstreamRejected.
func TestExpandThenAllowlist_NonUpstreamRejected(t *testing.T) {
	if IsUpstreamAPIImage("ghcr.io/segovchik/api:otel-test-4") {
		t.Error("non-upstream image was accepted")
	}
}
