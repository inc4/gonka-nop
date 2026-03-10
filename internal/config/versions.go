package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ImageVersions holds per-service container image versions parsed from upstream
// docker-compose files. These are fetched from the gonka-ai/gonka GitHub repo
// at setup time, so NOP always uses the latest official versions.
type ImageVersions struct {
	// From docker-compose.yml
	Node     string `json:"node"`      // inferenced image tag
	API      string `json:"api"`       // api image tag
	TMKMS    string `json:"tmkms"`     // tmkms-softsign-with-keygen image tag
	Proxy    string `json:"proxy"`     // proxy image tag
	ProxySSL string `json:"proxy_ssl"` // proxy-ssl image tag
	Bridge   string `json:"bridge"`    // bridge image tag (may include @sha256: digest)
	Explorer string `json:"explorer"`  // explorer image tag

	// From docker-compose.mlnode.yml
	MLNode string `json:"mlnode"` // mlnode image tag (e.g. "3.0.12-post2")
	Nginx  string `json:"nginx"`  // nginx image tag

	// Metadata
	FetchedAt time.Time `json:"fetched_at,omitempty"`
	Source    string    `json:"source,omitempty"` // "github" or "fallback"
}

const (
	// GitHub raw content URLs for docker-compose files.
	// Mainnet: main branch, Testnet: testnet/main branch.
	ghRawBase          = "https://raw.githubusercontent.com/gonka-ai/gonka"
	mainnetBranch      = "main"
	testnetBranch      = "testnet/main"
	composeRelPath     = "deploy/join/docker-compose.yml"
	mlnodeRelPath      = "deploy/join/docker-compose.mlnode.yml"
	fetchTimeout       = 15 * time.Second
	imageRegistryGHCR  = "ghcr.io/product-science/"
	imageRegistryNginx = "nginx:"
)

// FallbackMainnetVersions returns hardcoded mainnet versions as a safety net
// when GitHub is unreachable. These should be updated periodically.
func FallbackMainnetVersions() ImageVersions {
	return ImageVersions{
		Node:     "0.2.9-post3",
		API:      "0.2.9-post3",
		TMKMS:    "0.2.9-post3",
		Proxy:    "0.2.9-post3",
		ProxySSL: "0.2.9-post3",
		Bridge:   "0.2.5-post5",
		Explorer: "latest",
		MLNode:   "3.0.12-post2",
		Nginx:    "1.28.0",
		Source:   "fallback",
	}
}

// FallbackTestnetVersions returns hardcoded testnet versions as a safety net.
func FallbackTestnetVersions() ImageVersions {
	return ImageVersions{
		Node:     "0.2.9-post2",
		API:      "0.2.9-post2",
		TMKMS:    "0.2.10-pre-release",
		Proxy:    "0.2.9-post2",
		ProxySSL: "0.2.9-post2",
		Bridge:   "0.2.10-pre-release",
		Explorer: "latest",
		MLNode:   "3.0.12-post3",
		Nginx:    "1.28.0",
		Source:   "fallback",
	}
}

// FetchImageVersions fetches the latest container image versions from the
// gonka-ai/gonka GitHub repository. It parses the docker-compose.yml and
// docker-compose.mlnode.yml files from the appropriate branch.
//
// If fetching fails, it returns fallback versions with a non-nil error.
func FetchImageVersions(ctx context.Context, isTestnet bool) (ImageVersions, error) {
	branch := mainnetBranch
	fallback := FallbackMainnetVersions
	if isTestnet {
		branch = testnetBranch
		fallback = FallbackTestnetVersions
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	// Fetch both compose files
	composeURL := fmt.Sprintf("%s/%s/%s", ghRawBase, branch, composeRelPath)
	mlnodeURL := fmt.Sprintf("%s/%s/%s", ghRawBase, branch, mlnodeRelPath)

	composeContent, err := fetchURL(ctx, composeURL)
	if err != nil {
		fb := fallback()
		return fb, fmt.Errorf("failed to fetch docker-compose.yml from GitHub: %w", err)
	}

	mlnodeContent, err := fetchURL(ctx, mlnodeURL)
	if err != nil {
		fb := fallback()
		return fb, fmt.Errorf("failed to fetch docker-compose.mlnode.yml from GitHub: %w", err)
	}

	versions, err := ParseComposeImageVersions(composeContent, mlnodeContent)
	if err != nil {
		fb := fallback()
		return fb, fmt.Errorf("failed to parse image versions: %w", err)
	}

	versions.FetchedAt = time.Now()
	versions.Source = "github"

	return versions, nil
}

// ParseComposeImageVersions extracts image tags from docker-compose file contents.
// It understands the gonka-ai/gonka compose file format with GHCR image references.
func ParseComposeImageVersions(composeContent, mlnodeContent string) (ImageVersions, error) {
	var v ImageVersions

	// Parse main docker-compose.yml
	v.TMKMS = extractImageTag(composeContent, "tmkms-softsign-with-keygen")
	v.Node = extractImageTag(composeContent, "inferenced")
	v.API = extractImageTag(composeContent, "api")
	v.Bridge = extractBridgeTag(composeContent)
	v.Proxy = extractImageTag(composeContent, "proxy")
	v.ProxySSL = extractImageTag(composeContent, "proxy-ssl")
	v.Explorer = extractImageTag(composeContent, "explorer")

	// Parse docker-compose.mlnode.yml
	v.MLNode = extractMLNodeTag(mlnodeContent)
	v.Nginx = extractNginxTag(mlnodeContent)

	// Validate that we got at least the critical versions
	if v.Node == "" || v.API == "" {
		return v, fmt.Errorf("could not parse node or api image versions from compose files")
	}

	return v, nil
}

// imageTagRe matches "image: ghcr.io/product-science/<name>:<tag>" lines.
var imageTagRe = regexp.MustCompile(`image:\s*ghcr\.io/product-science/([^:]+):(\S+)`)

// extractImageTag finds the tag for a specific GHCR image name.
// For images like "ghcr.io/product-science/proxy:0.2.9-post3", pass name="proxy".
// For "proxy-ssl", it specifically matches "proxy-ssl" to avoid matching "proxy".
func extractImageTag(content, imageName string) string {
	for _, match := range imageTagRe.FindAllStringSubmatch(content, -1) {
		if len(match) >= 3 {
			name := match[1]
			tag := match[2]
			if name == imageName {
				return tag
			}
		}
	}
	return ""
}

// extractBridgeTag handles the bridge image which may include a @sha256: digest.
// Example: "ghcr.io/product-science/bridge:0.2.5-post5@sha256:8d2f..."
func extractBridgeTag(content string) string {
	re := regexp.MustCompile(`image:\s*ghcr\.io/product-science/bridge:(\S+)`)
	match := re.FindStringSubmatch(content)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

// extractMLNodeTag finds the mlnode image tag from docker-compose.mlnode.yml.
// Ignores commented-out lines (lines starting with #).
func extractMLNodeTag(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "ghcr.io/product-science/mlnode:") {
			re := regexp.MustCompile(`mlnode:(\S+)`)
			match := re.FindStringSubmatch(trimmed)
			if len(match) >= 2 {
				return match[1]
			}
		}
	}
	return ""
}

// extractNginxTag finds the nginx image tag.
func extractNginxTag(content string) string {
	re := regexp.MustCompile(`image:\s*nginx:(\S+)`)
	match := re.FindStringSubmatch(content)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

// fetchURL performs an HTTP GET with the given context and returns the response body.
func fetchURL(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// MainImageVersion returns the primary image version (node/api version) from
// ImageVersions, for use as the general "image version" where a single version
// is needed (e.g. state.ImageVersion).
func (v ImageVersions) MainImageVersion() string {
	if v.Node != "" {
		return v.Node
	}
	return v.API
}
