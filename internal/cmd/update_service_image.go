// M12 Slice 7: `gonka-nop update --service api --image <tag>` — compose
// image swap (separate code path from M11 `--api-only` Cosmovisor
// hotfix). Validates against the devshard upstream allowlist, expands
// bare semver to canonical form, performs an atomic compose mutation
// via internal/docker.AtomicReplace, pulls the new image, and recreates
// ONLY the api container (--no-deps --force-recreate).
//
// Architect pre-review: APPROVED-WITH-CONDITIONS 2026-05-17
// (allowlist + AtomicReplace + RecreateService no-deps contract).
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/devshard"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/ui"
)

const apiImagePrefix = "ghcr.io/product-science/api:"

// runUpdateServiceImage swaps the api image tag in docker-compose.yml,
// pulls the new image, and recreates only the api container.
func runUpdateServiceImage(ctx context.Context, state *config.State) error {
	target := devshard.ExpandBareSemver(updateImage)

	if !devshard.IsUpstreamAPIImage(target) {
		if !updateAllowNonUps {
			return fmt.Errorf(
				"image %q is not in the upstream allowlist (ghcr.io/product-science/api:<semver> or @sha256:<digest>); pass --allow-non-upstream to override",
				target,
			)
		}
		ui.Warn("Using non-upstream api image: %s", target)
	}

	composePath := filepath.Join(state.OutputDir, "docker-compose.yml")
	current, err := readCurrentAPIImage(composePath)
	if err != nil {
		return err
	}

	ui.Info("API image swap")
	ui.Detail("  current: %s", current)
	ui.Detail("  new:     %s", target)

	if current == target {
		ui.Info("Already at %s — nothing to do", target)
		return nil
	}

	if !updateYes {
		confirm, cerr := ui.Confirm("Proceed with swap and recreate api container?", true)
		if cerr != nil {
			return cerr
		}
		if !confirm {
			ui.Info("Update canceled.")
			return nil
		}
	}

	backup, err := docker.AtomicReplace(composePath, func(in []byte) ([]byte, error) {
		out := swapAPIImageInCompose(string(in), target)
		return []byte(out), nil
	})
	if err != nil {
		return fmt.Errorf("rewrite compose: %w", err)
	}
	ui.Detail("Backup: %s", backup)

	cc, err := docker.NewComposeClient(state)
	if err != nil {
		return fmt.Errorf("create compose client: %w", err)
	}

	pullCtx, pullCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer pullCancel()
	ui.Info("Pulling api image...")
	if err := cc.PullService(pullCtx, "api"); err != nil {
		return fmt.Errorf("pull api: %w", err)
	}

	ui.Info("Recreating api container (no-deps, force-recreate)...")
	if err := cc.RecreateService(ctx, "api"); err != nil {
		return fmt.Errorf("recreate api: %w", err)
	}

	ui.Success("api swapped to %s", target)
	ui.Detail("Other containers untouched. State.Versions.API NOT updated (non-upstream possible).")
	return nil
}

// readCurrentAPIImage extracts the current api image tag from compose.
// Returns the full tag (e.g. "ghcr.io/product-science/api:0.2.12" or
// "ghcr.io/segovchik/api:otel-test-4").
func readCurrentAPIImage(composePath string) (string, error) {
	data, err := os.ReadFile(composePath) // #nosec G304 - path under state.OutputDir
	if err != nil {
		return "", fmt.Errorf("read compose: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		const marker = "image: "
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		tag := strings.TrimSpace(line[idx+len(marker):])
		// Match either upstream or non-upstream api images. We're looking
		// for any image that ends in `/api:<...>` and could be the api
		// service. The simplest heuristic: contains "/api:" (with the
		// colon) AND does not match other services like "/proxy-ssl:" etc.
		if strings.Contains(tag, "/api:") || strings.Contains(tag, "/api@sha256:") {
			return tag, nil
		}
	}
	return "", fmt.Errorf("no api image line found in %s", composePath)
}

// swapAPIImageInCompose replaces the api image line in compose content
// with the new tag. Only touches the FIRST uncommented `image: */api:*`
// line — matches the api service's image, leaves proxy/bridge/etc.
// unchanged.
func swapAPIImageInCompose(content, newImage string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		const marker = "image: "
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		tag := strings.TrimSpace(line[idx+len(marker):])
		if !strings.Contains(tag, "/api:") && !strings.Contains(tag, "/api@sha256:") {
			continue
		}
		// Preserve indentation prefix (everything before `image: `).
		lines[i] = line[:idx+len(marker)] + newImage
		break // only swap first match
	}
	return strings.Join(lines, "\n")
}

// imageIsAPI returns true if the given tag string references an api image
// (upstream or non-upstream). Used by validation paths.
func imageIsAPI(tag string) bool {
	return strings.HasPrefix(tag, apiImagePrefix) ||
		strings.Contains(tag, "/api:") ||
		strings.Contains(tag, "/api@sha256:")
}
