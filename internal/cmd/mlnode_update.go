package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/inc4/gonka-nop/internal/chain"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/phases"
	"github.com/inc4/gonka-nop/internal/status"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

var (
	mlNodeUpdateID            string
	mlNodeUpdateModel         string
	mlNodeUpdateTP            int
	mlNodeUpdateMaxConcurrent int
	mlNodeUpdateVLLMArgs      string
	mlNodeUpdateForce         bool
)

var mlNodeUpdateCmd = &cobra.Command{
	Use:   "update --id <node-id> [--model X] [--tp N] [--max-concurrent M] [--vllm-args \"...\"] [--force]",
	Short: "Update an existing ML node via PUT /admin/v1/nodes/:id (PoC-gated)",
	Long: `Update an existing ML node's configuration. Performs a read-merge-write
PUT against the Admin API:

  1. PoC safety check (refuses during PoC/CPoC unless --force)
  2. GET /admin/v1/nodes — fetch existing record
  3. Merge CLI flags onto the record (flags override; unrelated fields preserved)
  4. PUT /admin/v1/nodes/:id with the merged body

When --model is set to a known recipe (e.g. moonshotai/Kimi-K2.6), the
recipe's VLLMArgs are applied unless --vllm-args is also passed (which
overrides the recipe).

Model changes take effect at the NEXT epoch boundary (chain semantics).

Examples:
  gonka-nop ml-node update --id node1 --model moonshotai/Kimi-K2.6
  gonka-nop ml-node update --id node1 --tp 4 --max-concurrent 128
  gonka-nop ml-node update --id node1 --model Qwen/QwQ-32B --vllm-args "--max-num-seqs 64"
  gonka-nop ml-node update --id node1 --force --model X  # skip PoC gate`,
	RunE: runMLNodeUpdate,
}

func init() {
	mlNodeUpdateCmd.Flags().StringVar(&mlNodeUpdateID, "id", "", "ML node ID (required)")
	mlNodeUpdateCmd.Flags().StringVar(&mlNodeUpdateModel, "model", "", "Governance-approved model ID (e.g. moonshotai/Kimi-K2.6)")
	mlNodeUpdateCmd.Flags().IntVar(&mlNodeUpdateTP, "tp", 0, "Tensor parallel size (1-8); 0 = leave unchanged")
	mlNodeUpdateCmd.Flags().IntVar(&mlNodeUpdateMaxConcurrent, "max-concurrent", 0, "Max concurrent inferences; 0 = leave unchanged")
	mlNodeUpdateCmd.Flags().StringVar(&mlNodeUpdateVLLMArgs, "vllm-args", "", "Override vLLM args (space-separated); when set, the recipe's args for --model are ignored with a warning")
	mlNodeUpdateCmd.Flags().BoolVar(&mlNodeUpdateForce, "force", false, "Skip PoC safety check (dangerous)")
	_ = mlNodeUpdateCmd.MarkFlagRequired("id")
	mlNodeCmd.AddCommand(mlNodeUpdateCmd)
}

func runMLNodeUpdate(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	state, err := config.Load(outputDir)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if state.OutputDir == "" {
		return fmt.Errorf("no deployment found — run 'gonka-nop setup' first")
	}

	// 1. Safety gate BEFORE any HTTP.
	if !mlNodeUpdateForce {
		checker := chain.NewPoCSafetyChecker(state)
		c, qErr := checker.Check(ctx)
		if qErr != nil {
			return fmt.Errorf("safety check (fail-closed): %w", qErr)
		}
		if !c.Safe {
			return fmt.Errorf("unsafe to update ml-node during current chain phase: %s\n  Use --force to override (may cause missed inference)", c.Reason)
		}
	} else {
		ui.Warn("--force: skipping PoC safety check")
	}

	// 2. GET existing record.
	entries, err := fetchAdminNodes(adminURL)
	if err != nil {
		return fmt.Errorf("fetch nodes: %w", err)
	}
	var existing *status.AdminNodesEntry
	for i := range entries {
		if entries[i].Node.ID == mlNodeUpdateID {
			existing = &entries[i]
			break
		}
	}
	if existing == nil {
		return fmt.Errorf("ml-node id %q not found (known: %s)", mlNodeUpdateID, listKnownNodeIDs(entries))
	}

	// 3. Merge flags onto the record.
	merged, warnings, err := mergeNodeUpdate(existing.Node, updateFlags{
		Model:         mlNodeUpdateModel,
		TP:            mlNodeUpdateTP,
		MaxConcurrent: mlNodeUpdateMaxConcurrent,
		VLLMArgs:      mlNodeUpdateVLLMArgs,
	})
	if err != nil {
		return err
	}
	for _, w := range warnings {
		ui.Warn("%s", w)
	}

	// 4. Idempotency check: if merged body byte-equals existing record, no-op.
	existingBody, _ := json.Marshal(existing.Node)
	mergedBody, _ := json.Marshal(merged)
	if bytes.Equal(existingBody, mergedBody) {
		ui.Info("No changes required — current record already matches the requested update")
		return nil
	}

	// 5. PUT the merged record.
	if err := putAdminNode(ctx, adminURL, mlNodeUpdateID, merged); err != nil {
		return err
	}
	ui.Success("ml-node %s updated", mlNodeUpdateID)
	if mlNodeUpdateModel != "" {
		ui.Info("Model change applies at the next epoch boundary")
	}
	return nil
}

type updateFlags struct {
	Model         string
	TP            int
	MaxConcurrent int
	VLLMArgs      string
}

// mergeNodeUpdate produces an updated AdminNodesNodeInfo from a starting
// record + CLI flags. Returns (merged, warnings, error). Flags override;
// unspecified flags preserve the existing field. Recipe lookups happen
// only when --model is provided.
func mergeNodeUpdate(existing status.AdminNodesNodeInfo, f updateFlags) (status.AdminNodesNodeInfo, []string, error) {
	merged := existing
	var warnings []string

	if f.MaxConcurrent > 0 {
		merged.MaxConcurrent = f.MaxConcurrent
	}

	if f.Model != "" {
		recipe, ok := phases.LookupRecipe(f.Model)
		if !ok {
			return existing, nil, fmt.Errorf("model %q is not a governance-approved recipe (known: %v)", f.Model, phases.SupportedModelNames())
		}
		// Build args: recipe-driven by default; --vllm-args / --tp overrides.
		args := []string{}
		switch {
		case f.VLLMArgs != "":
			args = strings.Fields(f.VLLMArgs)
			if len(recipe.VLLMArgs) > 0 {
				warnings = append(warnings, fmt.Sprintf("--vllm-args overrides recipe %q (recipe args ignored)", f.Model))
			}
		case len(recipe.VLLMArgs) > 0:
			args = make([]string, len(recipe.VLLMArgs))
			copy(args, recipe.VLLMArgs)
		default:
			// Legacy recipe with no VLLMArgs; synthesize from TP + sane defaults.
			if f.TP > 0 {
				args = append(args, "--tensor-parallel-size", fmt.Sprintf("%d", f.TP))
			}
		}
		// --tp flag override: replace --tensor-parallel-size in args.
		if f.TP > 0 {
			args = overrideTP(args, f.TP)
		}
		merged.Models = map[string]status.AdminModelConfig{
			f.Model: {Args: args},
		}
	} else if f.TP > 0 && len(merged.Models) > 0 {
		// No model flag, but --tp set: patch TP in existing model's args.
		for k, v := range merged.Models {
			v.Args = overrideTP(v.Args, f.TP)
			merged.Models[k] = v
		}
	} else if f.VLLMArgs != "" && len(merged.Models) > 0 {
		// --vllm-args alone with existing model.
		args := strings.Fields(f.VLLMArgs)
		for k := range merged.Models {
			merged.Models[k] = status.AdminModelConfig{Args: args}
		}
	}

	return merged, warnings, nil
}

// overrideTP replaces (or inserts) --tensor-parallel-size in an args slice.
func overrideTP(args []string, tp int) []string {
	out := make([]string, 0, len(args)+2)
	skipNext := false
	replaced := false
	for i, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if a == "--tensor-parallel-size" && i+1 < len(args) {
			out = append(out, "--tensor-parallel-size", fmt.Sprintf("%d", tp))
			skipNext = true
			replaced = true
			continue
		}
		out = append(out, a)
	}
	if !replaced {
		out = append(out, "--tensor-parallel-size", fmt.Sprintf("%d", tp))
	}
	return out
}

func listKnownNodeIDs(entries []status.AdminNodesEntry) string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.Node.ID)
	}
	return strings.Join(ids, ", ")
}

func putAdminNode(ctx context.Context, baseURL, nodeID string, node status.AdminNodesNodeInfo) error {
	body, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal node: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/admin/v1/nodes/" + nodeID
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("admin API PUT %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		out, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin API PUT returned %d: %s", resp.StatusCode, strings.TrimSpace(string(out)))
	}
	return nil
}
