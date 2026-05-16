package cosmovisor

import (
	"fmt"
	"net/url"
	"strings"
)

// allowedReleaseHostPrefixes is the closed set of (host, path-prefix) pairs
// from which the installer is permitted to download binaries. The pairs are
// matched as `URL.Host == host AND strings.HasPrefix(URL.Path, pathPrefix)`
// — NOT substring contains — to reject the classic subdomain confusion
// like `github.com.evil.com/gonka-ai/gonka/...` or path-confusion like
// `github.com/gonka-ai/gonka.evil.com/...`.
var allowedReleaseHostPrefixes = []struct {
	host, pathPrefix string
}{
	{"github.com", "/gonka-ai/gonka/"},
	{"github.com", "/product-science/race-releases/"},
}

// ValidateReleaseURL accepts only HTTPS URLs whose host+path matches the
// allowlist exactly. Any rejection is returned BEFORE any network I/O so
// callers can short-circuit before constructing an http.Client.
//
// Examples:
//
//	OK:     https://github.com/gonka-ai/gonka/releases/download/release%2Fv0.2.12/foo.zip
//	OK:     https://github.com/product-science/race-releases/releases/download/release%2Fv0.2.12-api-post3/decentralized-api-amd64.zip
//	REJECT: http://github.com/...                  (non-TLS)
//	REJECT: https://github.com.evil.com/...        (host suffix attack)
//	REJECT: https://github.com/gonka-ai/gonka.evil.com/...  (path-prefix attack)
//	REJECT: https://raw.githubusercontent.com/...  (wrong host even if path is benign)
func ValidateReleaseURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("release URL is empty")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("release URL scheme must be https, got %q", u.Scheme)
	}
	// url.Parse lowercases the host. Strip any user-info to defang
	// `https://user@github.com/...` (would be parsed with Host == "github.com"
	// but it's still suspicious — the path-prefix check below catches the
	// common case but we explicitly disallow user-info just in case).
	if u.User != nil {
		return fmt.Errorf("release URL must not contain user info")
	}
	if u.Host == "" {
		return fmt.Errorf("release URL has no host")
	}
	// url.Parse keeps the leading slash on Path. The allowlist entries also
	// keep their trailing slash so prefix matching is unambiguous about
	// the segment boundary (rejects /gonka-ai/gonka.evil.com/... because
	// /gonka-ai/gonka. does NOT have prefix /gonka-ai/gonka/).
	for _, allowed := range allowedReleaseHostPrefixes {
		if u.Host == allowed.host && strings.HasPrefix(u.Path, allowed.pathPrefix) {
			return nil
		}
	}
	return fmt.Errorf("release URL %s is not in the allowlist (allowed: github.com/{gonka-ai/gonka, product-science/race-releases})", rawURL)
}
