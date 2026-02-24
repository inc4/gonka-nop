package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/docker"
	"github.com/inc4/gonka-nop/internal/phases"
	"github.com/inc4/gonka-nop/internal/ui"
	"github.com/spf13/cobra"
)

const (
	imagePullTimeout = 15 * time.Minute
)

var (
	dlHFHome  string
	dlImage   string
	dlHFToken string
	dlYes     bool
)

var downloadModelCmd = &cobra.Command{
	Use:   "download-model [model-name]",
	Short: "Pre-download model weights to HuggingFace cache",
	Long: `Download model weights before or after setup.

Uses docker run to pull model weights via huggingface-cli inside the
mlnode container. No GPU access or compose files are required.

Can be run before 'gonka-nop setup' to pre-cache large models
(Qwen3-235B is ~120GB+ and can take hours to download).

HuggingFace CLI supports resume — interrupted downloads continue
from where they left off.

Examples:
  gonka-nop download-model
  gonka-nop download-model Qwen/Qwen3-235B-A22B-Instruct-2507-FP8
  gonka-nop download-model --hf-home /data/hf
  gonka-nop download-model --hf-token hf_xxx`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDownloadModel,
}

func init() {
	downloadModelCmd.Flags().StringVar(&dlHFHome, "hf-home", "",
		fmt.Sprintf("HuggingFace cache directory (default %q)", phases.DefaultHFHome))
	downloadModelCmd.Flags().StringVar(&dlImage, "image", "",
		"MLNode container image (default: auto-detect from state or "+phases.DefaultMLNodeImage+":"+phases.DefaultMLNodeImageTag+")")
	downloadModelCmd.Flags().StringVar(&dlHFToken, "hf-token", "",
		"HuggingFace API token (for gated models; also reads HF_TOKEN env)")
	downloadModelCmd.Flags().BoolVarP(&dlYes, "yes", "y", false, "Skip confirmation prompts")
}

// downloadParams holds resolved parameters for the download command.
type downloadParams struct {
	Model   string
	HFHome  string
	Image   string
	HFToken string
}

func runDownloadModel(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if dlYes {
		ui.SetNonInteractive(true)
	}

	params, err := resolveDownloadParams(args)
	if err != nil {
		return err
	}

	if err := checkDockerAvailable(ctx); err != nil {
		return err
	}

	useSudo := docker.DetectSudo(ctx)

	displayDownloadPlan(params)
	if !dlYes {
		proceed, promptErr := ui.Confirm("Start download?", true)
		if promptErr != nil {
			return promptErr
		}
		if !proceed {
			ui.Info("Download canceled.")
			return nil
		}
	}

	if err := ensureHFHomeDir(ctx, params.HFHome, useSudo); err != nil {
		return fmt.Errorf("create HF_HOME directory: %w", err)
	}

	if err := pullMLNodeImage(ctx, params.Image, useSudo); err != nil {
		return err
	}

	return executeModelDownload(ctx, params, useSudo)
}

func resolveDownloadParams(args []string) (*downloadParams, error) {
	params := &downloadParams{}

	// Try loading state (may not exist pre-setup — that's OK)
	state, _ := config.Load(outputDir)

	// 1. Model name: arg > state > interactive prompt
	if len(args) > 0 {
		params.Model = args[0]
	} else if state != nil && state.SelectedModel != "" {
		params.Model = state.SelectedModel
	} else {
		selected, err := ui.Select("Select model to download:", phases.SupportedModels)
		if err != nil {
			return nil, err
		}
		params.Model = selected
	}

	// 2. HF_HOME: flag > state > default
	if dlHFHome != "" {
		params.HFHome = dlHFHome
	} else if state != nil && state.HFHome != "" {
		params.HFHome = state.HFHome
	} else {
		params.HFHome = phases.DefaultHFHome
	}

	// 3. Image: flag > state.Versions.MLNode > state.MLNodeImageTag > default
	if dlImage != "" {
		params.Image = dlImage
	} else if state != nil && state.Versions.MLNode != "" {
		params.Image = phases.DefaultMLNodeImage + ":" + state.Versions.MLNode
	} else if state != nil && state.MLNodeImageTag != "" {
		params.Image = phases.DefaultMLNodeImage + ":" + state.MLNodeImageTag
	} else {
		params.Image = phases.DefaultMLNodeImage + ":" + phases.DefaultMLNodeImageTag
	}

	// 4. HF token: flag > env
	if dlHFToken != "" {
		params.HFToken = dlHFToken
	} else if token := os.Getenv("HF_TOKEN"); token != "" {
		params.HFToken = token
	}

	return params, nil
}

// buildDockerRunArgs constructs the docker run arguments for model download.
func buildDockerRunArgs(params *downloadParams) []string {
	args := []string{
		"run", "--rm",
		"-v", params.HFHome + ":" + params.HFHome,
		"-e", "HF_HOME=" + params.HFHome,
	}

	if params.HFToken != "" {
		args = append(args, "-e", "HF_TOKEN="+params.HFToken)
	}

	args = append(args, params.Image, "huggingface-cli", "download", params.Model)
	return args
}

func checkDockerAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "version")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not available: %w\nInstall Docker: https://docs.docker.com/engine/install/", err)
	}
	return nil
}

func pullMLNodeImage(ctx context.Context, image string, useSudo bool) error {
	sp := ui.NewSpinner(fmt.Sprintf("Pulling image %s...", image))
	sp.Start()

	pullCtx, cancel := context.WithTimeout(ctx, imagePullTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.CommandContext(pullCtx, "sudo", "docker", "pull", image)
	} else {
		cmd = exec.CommandContext(pullCtx, "docker", "pull", image)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		sp.StopWithError("Image pull failed")
		return fmt.Errorf("pull image %s: %w", image, err)
	}

	sp.StopWithSuccess(fmt.Sprintf("Image ready: %s", image))
	return nil
}

func executeModelDownload(ctx context.Context, params *downloadParams, useSudo bool) error {
	ui.Header("Model Download")
	ui.Info("Model: %s", params.Model)
	ui.Info("Cache: %s", params.HFHome)
	ui.Detail("Resume support: interrupted downloads continue automatically")
	fmt.Println()

	dockerArgs := buildDockerRunArgs(params)

	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{"docker"}, dockerArgs...)...) //nolint:gosec // docker args are constructed internally
	} else {
		cmd = exec.CommandContext(ctx, "docker", dockerArgs...) //nolint:gosec // docker args are constructed internally
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("model download failed: %w\nIf the model requires authentication, use --hf-token flag", err)
	}

	ui.Success("Model %s downloaded to %s", params.Model, params.HFHome)
	return nil
}

func ensureHFHomeDir(ctx context.Context, dir string, useSudo bool) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}

	ui.Info("Creating directory: %s", dir)
	var cmd *exec.Cmd
	if useSudo {
		cmd = exec.CommandContext(ctx, "sudo", "mkdir", "-p", dir)
	} else {
		cmd = exec.CommandContext(ctx, "mkdir", "-p", dir)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return nil
}

func displayDownloadPlan(params *downloadParams) {
	bold := color.New(color.Bold)
	_, _ = bold.Println("\nDownload Plan")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  Model:  %s\n", params.Model)
	fmt.Printf("  Cache:  %s\n", params.HFHome)
	fmt.Printf("  Image:  %s\n", params.Image)
	if params.HFToken != "" {
		fmt.Printf("  Token:  (provided)\n")
	}
	fmt.Println()
}
