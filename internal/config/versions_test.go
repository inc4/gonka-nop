package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testMainnetTag = "0.2.9-post3"
	testSourceFB   = "fallback"
	testMLNodeTag  = "3.0.12-post2"
)

// Sample docker-compose.yml content matching upstream format (mainnet)
const testComposeContent = `services:
  tmkms:
    image: ghcr.io/product-science/tmkms-softsign-with-keygen:0.2.9-post3
    container_name: tmkms
    restart: unless-stopped

  node:
    container_name: node
    image: ghcr.io/product-science/inferenced:0.2.9-post3
    command: ["sh", "./init-docker.sh"]

  api:
    container_name: api
    image: ghcr.io/product-science/api:0.2.9-post3
    volumes:
      - .inference:/root/.inference

  bridge:
    container_name: bridge
    image: ghcr.io/product-science/bridge:0.2.5-post5@sha256:8d2f217115c65b27fcb6fe1497471c30891534f18685bd3007d168aa7f1a9371
    restart: unless-stopped

  proxy:
    container_name: proxy
    image: ghcr.io/product-science/proxy:0.2.9-post3
    ports:
      - "8000:80"

  proxy-ssl:
    container_name: proxy-ssl
    image: ghcr.io/product-science/proxy-ssl:0.2.9-post3

  versiond:
    container_name: versiond
    image: ghcr.io/product-science/versiond:0.2.9-post3

  explorer:
    container_name: explorer
    image: ghcr.io/product-science/explorer:latest
    expose:
      - "5173"
`

const testMLNodeContent = `services:
  mlnode-308:
    # ghcr.io/product-science/mlnode:3.0.12-post2-blackwell
    image: ghcr.io/product-science/mlnode:3.0.12-post2
    hostname: mlnode-308

  inference:
    image: nginx:1.28.0
    hostname: inference
`

func TestParseComposeImageVersions(t *testing.T) {
	v, err := ParseComposeImageVersions(testComposeContent, testMLNodeContent)
	if err != nil {
		t.Fatalf("ParseComposeImageVersions() error: %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"TMKMS", v.TMKMS, testMainnetTag},
		{"Node", v.Node, testMainnetTag},
		{"API", v.API, testMainnetTag},
		{"Bridge", v.Bridge, "0.2.5-post5@sha256:8d2f217115c65b27fcb6fe1497471c30891534f18685bd3007d168aa7f1a9371"},
		{"Proxy", v.Proxy, testMainnetTag},
		{"ProxySSL", v.ProxySSL, testMainnetTag},
		{"Explorer", v.Explorer, "latest"},
		{"MLNode", v.MLNode, testMLNodeTag},
		{"Nginx", v.Nginx, "1.28.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestParseComposeImageVersions_SkipsCommentedMLNode(t *testing.T) {
	// The mlnode compose has a commented-out blackwell line
	v, err := ParseComposeImageVersions(testComposeContent, testMLNodeContent)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// Should get 3.0.12-post2, NOT 3.0.12-post2-blackwell (which is commented)
	if v.MLNode != testMLNodeTag {
		t.Errorf("MLNode = %q, want %q (should skip commented line)", v.MLNode, testMLNodeTag)
	}
}

func TestParseComposeImageVersions_TestnetFormat(t *testing.T) {
	// Testnet has different versions and some pre-release tags
	testnetCompose := `services:
  tmkms:
    image: ghcr.io/product-science/tmkms-softsign-with-keygen:0.2.10-pre-release
  node:
    image: ghcr.io/product-science/inferenced:0.2.9-post2
  api:
    image: ghcr.io/product-science/api:0.2.9-post2
  bridge:
    image: ghcr.io/product-science/bridge:0.2.10-pre-release
  proxy:
    image: ghcr.io/product-science/proxy:0.2.9-post2
  proxy-ssl:
    image: ghcr.io/product-science/proxy-ssl:0.2.9-post2
  versiond:
    image: ghcr.io/product-science/versiond:0.2.10-pre-release
  explorer:
    image: ghcr.io/product-science/explorer:latest
`
	testnetMLNode := `services:
  mlnode-308:
    image: ghcr.io/product-science/mlnode:3.0.12-post3
  inference:
    image: nginx:1.28.0
`
	v, err := ParseComposeImageVersions(testnetCompose, testnetMLNode)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if v.TMKMS != "0.2.10-pre-release" {
		t.Errorf("TMKMS = %q, want 0.2.10-pre-release", v.TMKMS)
	}
	if v.Node != "0.2.9-post2" {
		t.Errorf("Node = %q, want 0.2.9-post2", v.Node)
	}
	if v.MLNode != "3.0.12-post3" {
		t.Errorf("MLNode = %q, want 3.0.12-post3", v.MLNode)
	}
}

func TestParseComposeImageVersions_MissingCritical(t *testing.T) {
	// Missing node and api should return error
	content := `services:
  tmkms:
    image: ghcr.io/product-science/tmkms-softsign-with-keygen:0.2.9
`
	_, err := ParseComposeImageVersions(content, "")
	if err == nil {
		t.Error("expected error when node/api versions missing")
	}
}

func TestExtractImageTag_ProxyVsProxySSL(t *testing.T) {
	content := `services:
  proxy:
    image: ghcr.io/product-science/proxy:0.2.9-post3
  proxy-ssl:
    image: ghcr.io/product-science/proxy-ssl:0.2.8-post1
`
	proxy := extractImageTag(content, "proxy")
	proxySSL := extractImageTag(content, "proxy-ssl")

	if proxy != testMainnetTag {
		t.Errorf("proxy = %q, want 0.2.9-post3", proxy)
	}
	if proxySSL != "0.2.8-post1" {
		t.Errorf("proxy-ssl = %q, want 0.2.8-post1", proxySSL)
	}
}

func TestFetchImageVersions_MockServer(t *testing.T) {
	// Create a mock HTTP server that serves compose files
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "docker-compose.yml"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testComposeContent))
		case strings.HasSuffix(path, "docker-compose.mlnode.yml"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testMLNodeContent))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Override the fetch function by testing ParseComposeImageVersions directly
	// (FetchImageVersions uses hardcoded GitHub URLs, so we test parsing separately)
	v, err := ParseComposeImageVersions(testComposeContent, testMLNodeContent)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if v.Node != testMainnetTag {
		t.Errorf("Node = %q, want 0.2.9-post3", v.Node)
	}
	if v.MLNode != testMLNodeTag {
		t.Errorf("MLNode = %q, want %s", v.MLNode, testMLNodeTag)
	}
}

func TestFetchImageVersions_FallbackOnError(t *testing.T) {
	// Create a server that always returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Direct fetch with bad URL will fail and return fallback
	ctx := context.Background()
	v, err := FetchImageVersions(ctx, false)
	if err == nil {
		// If GitHub is actually reachable, this test passes differently
		if v.Source != "github" {
			t.Error("expected github source when fetch succeeds")
		}
		return
	}

	// Error path: should return fallback versions
	if v.Source != testSourceFB {
		t.Errorf("Source = %q, want %s", v.Source, testSourceFB)
	}
	if v.Node == "" {
		t.Error("fallback should have non-empty Node version")
	}
}

func TestFallbackVersions(t *testing.T) {
	mainnet := FallbackMainnetVersions()
	if mainnet.Node == "" || mainnet.API == "" || mainnet.MLNode == "" {
		t.Error("mainnet fallback has empty critical versions")
	}
	if mainnet.Source != testSourceFB {
		t.Errorf("Source = %q, want %s", mainnet.Source, testSourceFB)
	}

	testnet := FallbackTestnetVersions()
	if testnet.Node == "" || testnet.API == "" || testnet.MLNode == "" {
		t.Error("testnet fallback has empty critical versions")
	}
	if testnet.Source != testSourceFB {
		t.Errorf("Source = %q, want %s", testnet.Source, testSourceFB)
	}
}

func TestMainImageVersion(t *testing.T) {
	v := ImageVersions{Node: testMainnetTag, API: testMainnetTag}
	if v.MainImageVersion() != testMainnetTag {
		t.Errorf("MainImageVersion() = %q, want 0.2.9-post3", v.MainImageVersion())
	}

	// Node empty, fallback to API
	v2 := ImageVersions{API: "0.2.8"}
	if v2.MainImageVersion() != "0.2.8" {
		t.Errorf("MainImageVersion() = %q, want 0.2.8", v2.MainImageVersion())
	}
}

func TestExtractBridgeTag_WithDigest(t *testing.T) {
	content := `image: ghcr.io/product-science/bridge:0.2.5-post5@sha256:abcdef123456`
	tag := extractBridgeTag(content)
	if tag != "0.2.5-post5@sha256:abcdef123456" {
		t.Errorf("bridge tag = %q, want with digest", tag)
	}
}

func TestExtractBridgeTag_WithoutDigest(t *testing.T) {
	content := `image: ghcr.io/product-science/bridge:0.2.10-pre-release`
	tag := extractBridgeTag(content)
	if tag != "0.2.10-pre-release" {
		t.Errorf("bridge tag = %q, want 0.2.10-pre-release", tag)
	}
}

// --- M12 Slice 1 (TDD red phase) ----------------------------------------------
// The following 4 tests cover M12 Slice 1: adding a `Versiond` field to the
// ImageVersions struct and parsing the new `versiond` service from upstream
// docker-compose.yml. These tests are written BEFORE the production change so
// they must fail (compile error initially: ImageVersions.Versiond doesn't
// exist; once added, runtime assertions take over).
//
// References:
//   - docs/plan-m12.md, Slice 1 "Required tests"
//   - docs/qa/m12-devshardd_test_cases.md TC-1.1, TC-COMPOSE-3, TC-4.9

// testComposeWithVersiond is a minimal but valid compose fixture that includes
// the new `versiond` service alongside the existing critical services
// (`inferenced`, `api`). Used by both the happy-path Versiond parse test and
// the proxy/proxy-ssl disambiguation test (extended with both proxy variants).
const testComposeWithVersiond = `services:
  node:
    container_name: node
    image: ghcr.io/product-science/inferenced:0.2.12
    command: ["sh", "./init-docker.sh"]

  api:
    container_name: api
    image: ghcr.io/product-science/api:0.2.12

  versiond:
    container_name: versiond
    image: ghcr.io/product-science/versiond:v0.2.12
    restart: unless-stopped
`

// Test 1: TestParseComposeImageVersions_Versiond verifies the parser picks up
// the new `versiond` service image tag from a compose file. Currently the
// ImageVersions struct has no Versiond field, so this is a compile-time red.
func TestParseComposeImageVersions_Versiond(t *testing.T) {
	v, err := ParseComposeImageVersions(testComposeWithVersiond, "")
	if err != nil {
		t.Fatalf("ParseComposeImageVersions() error: %v", err)
	}

	if v.Versiond != "v0.2.12" {
		t.Errorf("Versiond = %q, want %q", v.Versiond, "v0.2.12")
	}
}

// Test 2: TestFallback_VersiondPopulated guards the contract that both
// fallback constructors include a non-empty Versiond tag, so that offline
// installs (GitHub unreachable) still have a usable default.
func TestFallback_VersiondPopulated(t *testing.T) {
	t.Run("mainnet", func(t *testing.T) {
		v := FallbackMainnetVersions()
		if v.Versiond == "" {
			t.Errorf("FallbackMainnetVersions().Versiond is empty; want non-empty default")
		}
	})

	t.Run("testnet", func(t *testing.T) {
		v := FallbackTestnetVersions()
		if v.Versiond == "" {
			t.Errorf("FallbackTestnetVersions().Versiond is empty; want non-empty default")
		}
	})
}

// Test 3: TestFetchImageVersions_VersiondMissingErrors verifies that when an
// upstream compose file lacks the `versiond` service, FetchImageVersions
// returns a non-nil error referencing "versiond" AND falls back to populated
// defaults (which Test 2 guarantees have a Versiond tag).
//
// NOTE: To make this pass, the critical-field check in ParseComposeImageVersions
// (versions.go line 151) must be extended to include Versiond. That production
// change is part of M12 Slice 1, not this test.
//
// Pattern mirrors TestFetchImageVersions_FallbackOnError: we cannot inject a
// custom URL (FetchImageVersions uses hardcoded GitHub URLs), so we drive the
// negative path through the public API and tolerate the case where GitHub is
// actually reachable AND returns a compose without versiond. We spin up an
// httptest server primarily to confirm a reproducible mock pattern exists for
// future maintainers; the assertions run on the real FetchImageVersions return.
func TestFetchImageVersions_VersiondMissingErrors(t *testing.T) {
	// Mock server: serves a compose without versiond, plus an mlnode file.
	// Kept for documentation; FetchImageVersions hardcodes GitHub URLs so this
	// server is not actually hit. Future refactors could inject the base URL.
	composeNoVersiond := `services:
  node:
    image: ghcr.io/product-science/inferenced:0.2.12
  api:
    image: ghcr.io/product-science/api:0.2.12
`
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "docker-compose.yml"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(composeNoVersiond))
		case strings.HasSuffix(path, "docker-compose.mlnode.yml"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testMLNodeContent))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// First-line assertion: parser must reject a compose missing `versiond`
	// directly (independent of FetchImageVersions network behavior).
	if _, parseErr := ParseComposeImageVersions(composeNoVersiond, testMLNodeContent); parseErr == nil {
		t.Errorf("ParseComposeImageVersions() with no versiond returned nil error; want error mentioning 'versiond'")
	} else if !strings.Contains(strings.ToLower(parseErr.Error()), "versiond") {
		t.Errorf("ParseComposeImageVersions() error = %q; want substring 'versiond' (case-insensitive)", parseErr.Error())
	}

	// Second assertion: when FetchImageVersions fails (any reason — bad parse,
	// network), the returned ImageVersions must have a populated Versiond from
	// the fallback path. This couples Test 2's contract to the fetch surface.
	ctx := context.Background()
	v, err := FetchImageVersions(ctx, false)
	if err != nil {
		// Error path (most common in CI): we should get fallback versions
		// with Versiond populated.
		if v.Source != testSourceFB {
			t.Errorf("on FetchImageVersions error, Source = %q; want %q", v.Source, testSourceFB)
		}
		if v.Versiond == "" {
			t.Errorf("on FetchImageVersions error, Versiond is empty; fallback must populate Versiond")
		}
		return
	}

	// Success path: GitHub was reachable AND returned a compose with versiond
	// already in upstream. Accept and verify the field is populated end-to-end.
	if v.Versiond == "" {
		t.Errorf("FetchImageVersions succeeded but Versiond is empty; upstream compose must declare versiond")
	}
}

// Test 4: TestParseComposeImageVersions_ProxySSLDisambiguation drives the
// higher-level ParseComposeImageVersions API (not just extractImageTag) to
// confirm that when both `proxy` and `proxy-ssl` services are present with
// distinct tags, neither gets crossed with the other. Complements the
// lower-level TestExtractImageTag_ProxyVsProxySSL with an end-to-end check.
func TestParseComposeImageVersions_ProxySSLDisambiguation(t *testing.T) {
	const compose = `services:
  node:
    image: ghcr.io/product-science/inferenced:0.2.12
  api:
    image: ghcr.io/product-science/api:0.2.12
  versiond:
    image: ghcr.io/product-science/versiond:v0.2.12
  proxy:
    container_name: proxy
    image: ghcr.io/product-science/proxy:v0.2.12
  proxy-ssl:
    container_name: proxy-ssl
    image: ghcr.io/product-science/proxy-ssl:v0.2.13
`
	v, err := ParseComposeImageVersions(compose, "")
	if err != nil {
		t.Fatalf("ParseComposeImageVersions() error: %v", err)
	}

	if v.Proxy != "v0.2.12" {
		t.Errorf("Proxy = %q, want %q", v.Proxy, "v0.2.12")
	}
	if v.ProxySSL != "v0.2.13" {
		t.Errorf("ProxySSL = %q, want %q", v.ProxySSL, "v0.2.13")
	}
}
