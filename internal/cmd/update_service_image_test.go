// M12 Slice 7 tests: image swap helpers (pure-function path).
// The cobra dispatch + docker compose integration are covered by
// integration tests outside CI (Slice 4 deploy verification).
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sliceSampleCompose = `services:
  tmkms:
    image: ghcr.io/product-science/tmkms-softsign-with-keygen:0.2.12
  node:
    image: ghcr.io/product-science/inferenced:0.2.12
  api:
    image: ghcr.io/segovchik/api:otel-test-4
  bridge:
    image: ghcr.io/product-science/bridge:0.2.5-post5
  proxy:
    image: ghcr.io/product-science/proxy:0.2.12
  proxy-ssl:
    image: ghcr.io/product-science/proxy-ssl:0.2.12
  explorer:
    image: ghcr.io/product-science/explorer:latest
`

// TestReadCurrentAPIImage_FindsSegovchikImage (TC-5.1 setup precondition).
func TestReadCurrentAPIImage_FindsSegovchikImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(sliceSampleCompose), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := readCurrentAPIImage(path)
	if err != nil {
		t.Fatalf("readCurrentAPIImage: %v", err)
	}
	if got != "ghcr.io/segovchik/api:otel-test-4" {
		t.Errorf("got %q, want %q", got, "ghcr.io/segovchik/api:otel-test-4")
	}
}

// TestReadCurrentAPIImage_NoAPI_Errors.
func TestReadCurrentAPIImage_NoAPI_Errors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services:\n  node:\n    image: x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := readCurrentAPIImage(path); err == nil {
		t.Error("expected error when no api line present")
	}
}

// TestSwapAPIImageInCompose_RewritesOnlyAPI — swap must NOT touch
// proxy/bridge/proxy-ssl lines.
func TestSwapAPIImageInCompose_RewritesOnlyAPI(t *testing.T) {
	target := "ghcr.io/product-science/api:0.2.12"
	out := swapAPIImageInCompose(sliceSampleCompose, target)

	if !strings.Contains(out, target) {
		t.Errorf("output missing target %q", target)
	}
	if strings.Contains(out, "segovchik/api:otel-test-4") {
		t.Error("output still contains old api image segovchik/api:otel-test-4")
	}
	// Other services must be untouched
	for _, untouched := range []string{
		"tmkms-softsign-with-keygen:0.2.12",
		"inferenced:0.2.12",
		"bridge:0.2.5-post5",
		"proxy:0.2.12",
		"proxy-ssl:0.2.12",
		"explorer:latest",
	} {
		if !strings.Contains(out, untouched) {
			t.Errorf("output missing untouched service tag %q", untouched)
		}
	}
}

// TestSwapAPIImageInCompose_PreservesIndentation — leading whitespace
// on the image line must survive the swap (Docker Compose YAML cares).
func TestSwapAPIImageInCompose_PreservesIndentation(t *testing.T) {
	out := swapAPIImageInCompose(sliceSampleCompose, "ghcr.io/product-science/api:0.2.12")
	want := "    image: ghcr.io/product-science/api:0.2.12"
	if !strings.Contains(out, want) {
		t.Errorf("output missing properly-indented %q\ngot:\n%s", want, out)
	}
}

// TestSwapAPIImageInCompose_OnlyFirstMatchSwapped — if for some reason
// two `image: */api:*` lines exist, only the first is touched. This is
// belt-and-suspenders since our compose has one api service.
func TestSwapAPIImageInCompose_OnlyFirstMatchSwapped(t *testing.T) {
	content := `services:
  api:
    image: ghcr.io/segovchik/api:test-1
  api2:
    image: ghcr.io/segovchik/api:test-2
`
	out := swapAPIImageInCompose(content, "ghcr.io/product-science/api:0.2.12")
	if strings.Count(out, "test-1") != 0 {
		t.Error("first api image not swapped")
	}
	if strings.Count(out, "test-2") != 1 {
		t.Error("second api image was incorrectly swapped — should only swap first")
	}
}

// TestSwapAPIImageInCompose_SkipsCommentedLines — commented `image:`
// lines must not be swapped.
func TestSwapAPIImageInCompose_SkipsCommentedLines(t *testing.T) {
	content := `services:
  api:
    # image: ghcr.io/product-science/api:old
    image: ghcr.io/segovchik/api:test-1
`
	out := swapAPIImageInCompose(content, "ghcr.io/product-science/api:0.2.12")
	if !strings.Contains(out, "# image: ghcr.io/product-science/api:old") {
		t.Error("comment line was modified")
	}
	if strings.Contains(out, "test-1") {
		t.Error("active image not swapped")
	}
}

// TestImageIsAPI_TruthTable.
func TestImageIsAPI_TruthTable(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"ghcr.io/product-science/api:0.2.12", true},
		{"ghcr.io/segovchik/api:otel-test-4", true},
		{"ghcr.io/product-science/api@sha256:abc", true},
		{"ghcr.io/product-science/proxy:0.2.12", false},
		{"ghcr.io/product-science/inferenced:0.2.12", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := imageIsAPI(tc.in); got != tc.want {
				t.Errorf("imageIsAPI(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
