package cosmovisor

import (
	"context"
	"fmt"
	"path/filepath"
)

// FixSymlink updates a Cosmovisor `current` symlink to point at target.
// It computes a relative symlink (Cosmovisor expects this) and falls back
// to the absolute path if the relative computation fails. Runs `ln -snf`
// on the host (with sudo when useSudo is true).
//
// NOTE: this helper is NOT atomic — it shells out to `ln -snf`. The M11
// Slice 5 installer uses an atomic same-dir RenameFunc swap path for the
// hotfix installer; FixSymlink is retained for the `repair` command's
// recovery flow where atomicity is not required (the node is already down).
func FixSymlink(ctx context.Context, useSudo bool, outputDir, symlinkPath, target string) error {
	// Use relative symlink (Cosmovisor expects this)
	relTarget, err := filepath.Rel(filepath.Dir(symlinkPath), target)
	if err != nil {
		relTarget = target
	}

	return RunHostCmd(ctx, useSudo, outputDir,
		fmt.Sprintf("ln -snf %s %s", ShellQuote(relTarget), ShellQuote(symlinkPath)))
}
