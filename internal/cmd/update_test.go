package cmd

import (
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

const (
	testNodeTag     = "0.2.9-post3"
	testMLTag       = "3.0.12-post2"
	testNginxTag    = "1.28.0"
	testBridgeTag   = "0.2.5-post5"
	svcMLNode       = "mlnode"
	svcNginx        = "nginx"
	testAdminAddr   = defaultAdminURL
	testCustomAdmin = "http://custom:9200"
)

func TestComputeVersionDiffs_NoChanges(t *testing.T) {
	current := config.ImageVersions{
		Node:   testNodeTag,
		API:    testNodeTag,
		MLNode: testMLTag,
	}
	latest := config.ImageVersions{
		Node:   testNodeTag,
		API:    testNodeTag,
		MLNode: testMLTag,
	}

	diffs := computeVersionDiffs(current, latest)
	for _, d := range diffs {
		if d.HasUpdate {
			t.Errorf("expected no updates, but %s has update: %s -> %s", d.Service, d.Current, d.Latest)
		}
	}
}

func TestComputeVersionDiffs_MLNodeUpdate(t *testing.T) {
	current := config.ImageVersions{
		Node:   testNodeTag,
		API:    testNodeTag,
		MLNode: "3.0.11-post1",
	}
	latest := config.ImageVersions{
		Node:   testNodeTag,
		API:    testNodeTag,
		MLNode: testMLTag,
	}

	diffs := computeVersionDiffs(current, latest)

	foundMLUpdate := false
	for _, d := range diffs {
		if d.Service == svcMLNode && d.HasUpdate {
			foundMLUpdate = true
			if d.Current != "3.0.11-post1" || d.Latest != testMLTag {
				t.Errorf("mlnode diff: got %s->%s, want 3.0.11-post1->%s", d.Current, d.Latest, testMLTag)
			}
		}
		// node/api should not be marked as updatable (they are auto-updated)
		if d.Service == "node" && d.HasUpdate {
			if !d.AutoUpdate {
				t.Error("node should be marked as auto-update")
			}
		}
	}

	if !foundMLUpdate {
		t.Error("expected mlnode update not found")
	}
}

func TestComputeVersionDiffs_AutoUpdate(t *testing.T) {
	current := config.ImageVersions{
		Node: "0.2.8",
		API:  "0.2.8",
	}
	latest := config.ImageVersions{
		Node: testNodeTag,
		API:  testNodeTag,
	}

	diffs := computeVersionDiffs(current, latest)

	for _, d := range diffs {
		if d.Service == "node" {
			if !d.AutoUpdate {
				t.Error("node should be AutoUpdate")
			}
			if !d.HasUpdate {
				t.Error("node should have update")
			}
		}
		if d.Service == "api" {
			if !d.AutoUpdate {
				t.Error("api should be AutoUpdate")
			}
		}
	}
}

func TestFilterUpdatable(t *testing.T) {
	diffs := []VersionDiff{
		{Service: "node", HasUpdate: true, AutoUpdate: true},
		{Service: "api", HasUpdate: true, AutoUpdate: true},
		{Service: svcMLNode, HasUpdate: true, AutoUpdate: false},
		{Service: "proxy", HasUpdate: true, AutoUpdate: false},
		{Service: "bridge", HasUpdate: false, AutoUpdate: false},
	}

	updatable := filterUpdatable(diffs)
	if len(updatable) != 2 {
		t.Fatalf("expected 2 updatable, got %d", len(updatable))
	}

	services := make(map[string]bool)
	for _, d := range updatable {
		services[d.Service] = true
	}
	if !services[svcMLNode] || !services["proxy"] {
		t.Errorf("expected mlnode and proxy, got %v", services)
	}
}

func TestFilterDiffs(t *testing.T) {
	diffs := []VersionDiff{
		{Service: "node"},
		{Service: svcMLNode},
		{Service: "proxy"},
	}

	result := filterDiffs(diffs, svcMLNode)
	if len(result) != 1 || result[0].Service != svcMLNode {
		t.Errorf("expected mlnode only, got %v", result)
	}

	// Case insensitive
	result = filterDiffs(diffs, "PROXY")
	if len(result) != 1 || result[0].Service != "proxy" {
		t.Errorf("expected proxy only, got %v", result)
	}

	// Unknown service
	result = filterDiffs(diffs, "unknown")
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestReplaceImageTag(t *testing.T) {
	tests := []struct {
		name    string
		content string
		prefix  string
		newTag  string
		want    string
	}{
		{
			name:    "mlnode simple",
			content: "    image: ghcr.io/product-science/mlnode:3.0.11-post1",
			prefix:  "ghcr.io/product-science/mlnode:",
			newTag:  testMLTag,
			want:    "    image: ghcr.io/product-science/mlnode:" + testMLTag,
		},
		{
			name:    svcNginx,
			content: "    image: nginx:1.27.0",
			prefix:  "nginx:",
			newTag:  testNginxTag,
			want:    "    image: nginx:" + testNginxTag,
		},
		{
			name:    "skip commented line",
			content: "    # image: ghcr.io/product-science/mlnode:3.0.11-blackwell\n    image: ghcr.io/product-science/mlnode:3.0.11",
			prefix:  "ghcr.io/product-science/mlnode:",
			newTag:  testMLTag,
			want:    "    # image: ghcr.io/product-science/mlnode:3.0.11-blackwell\n    image: ghcr.io/product-science/mlnode:" + testMLTag,
		},
		{
			name:    "no match",
			content: "    image: ghcr.io/product-science/api:0.2.9",
			prefix:  "ghcr.io/product-science/mlnode:",
			newTag:  testMLTag,
			want:    "    image: ghcr.io/product-science/api:0.2.9",
		},
		{
			name:    "proxy does not match proxy-ssl",
			content: "    image: ghcr.io/product-science/proxy-ssl:0.2.8\n    image: ghcr.io/product-science/proxy:0.2.8",
			prefix:  "ghcr.io/product-science/proxy:",
			newTag:  testNodeTag,
			want:    "    image: ghcr.io/product-science/proxy-ssl:0.2.8\n    image: ghcr.io/product-science/proxy:" + testNodeTag,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceImageTag(tt.content, tt.prefix, tt.newTag)
			if got != tt.want {
				t.Errorf("replaceImageTag() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestReplaceImageTag_MultiLine(t *testing.T) {
	content := `services:
  mlnode-308:
    # ghcr.io/product-science/mlnode:3.0.11-blackwell
    image: ghcr.io/product-science/mlnode:3.0.11
    hostname: mlnode-308

  inference:
    image: nginx:1.27.0
    hostname: inference`

	// Update mlnode
	updated := replaceImageTag(content, "ghcr.io/product-science/mlnode:", testMLTag)
	if !strings.Contains(updated, "mlnode:"+testMLTag) {
		t.Error("mlnode tag not updated")
	}
	if strings.Contains(updated, "mlnode:3.0.11\n") {
		t.Error("old mlnode tag still present")
	}
	// Commented line should NOT be changed
	if !strings.Contains(updated, "# ghcr.io/product-science/mlnode:3.0.11-blackwell") {
		t.Error("commented line was modified")
	}

	// Update nginx
	updated = replaceImageTag(updated, "nginx:", testNginxTag)
	if !strings.Contains(updated, "nginx:"+testNginxTag) {
		t.Error("nginx tag not updated")
	}
}

func TestServiceToImageName(t *testing.T) {
	tests := []struct {
		service string
		want    string
	}{
		{"tmkms", "ghcr.io/product-science/tmkms-softsign-with-keygen:"},
		{"proxy", "ghcr.io/product-science/proxy:"},
		{"proxy-ssl", "ghcr.io/product-science/proxy-ssl:"},
		{"bridge", "ghcr.io/product-science/bridge:"},
		{"explorer", "ghcr.io/product-science/explorer:"},
		{"node", ""}, // auto-updated by cosmovisor
		{"api", ""},  // auto-updated by cosmovisor
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			got := serviceToImageName(tt.service)
			if got != tt.want {
				t.Errorf("serviceToImageName(%q) = %q, want %q", tt.service, got, tt.want)
			}
		})
	}
}

func TestFilterNonMLNode(t *testing.T) {
	diffs := []VersionDiff{
		{Service: svcMLNode},
		{Service: svcNginx},
		{Service: "proxy"},
		{Service: "tmkms"},
	}

	result := filterNonMLNode(diffs)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	for _, d := range result {
		if d.Service == svcMLNode || d.Service == svcNginx {
			t.Errorf("unexpected service %q in non-ml result", d.Service)
		}
	}
}

func TestResolveUpdateAdminURL(t *testing.T) {
	// Default: use state's AdminURL
	state := &config.State{AdminURL: "http://10.0.0.1:9200"}
	updateAdminURL = testAdminAddr // default value
	got := resolveUpdateAdminURL(state)
	if got != "http://10.0.0.1:9200" {
		t.Errorf("expected state URL, got %q", got)
	}

	// Explicit flag overrides
	updateAdminURL = testCustomAdmin
	got = resolveUpdateAdminURL(state)
	if got != testCustomAdmin {
		t.Errorf("expected custom URL, got %q", got)
	}

	// Fallback to default
	updateAdminURL = testAdminAddr
	state.AdminURL = ""
	got = resolveUpdateAdminURL(state)
	if got != testAdminAddr {
		t.Errorf("expected default URL, got %q", got)
	}
}

func TestDiffEntry(t *testing.T) {
	d := diffEntry(svcMLNode, "3.0.11", testMLTag, false)
	if !d.HasUpdate {
		t.Error("expected HasUpdate=true")
	}
	if d.AutoUpdate {
		t.Error("expected AutoUpdate=false")
	}

	// Same version
	d = diffEntry("proxy", testNodeTag, testNodeTag, false)
	if d.HasUpdate {
		t.Error("expected HasUpdate=false for same version")
	}

	// Empty current
	d = diffEntry("bridge", "", testBridgeTag, false)
	if d.HasUpdate {
		t.Error("expected HasUpdate=false when current is empty")
	}
}
