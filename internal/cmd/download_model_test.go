package cmd

import (
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/phases"
)

const (
	testModelQwQ  = "Qwen/QwQ-32B"
	testModel235B = "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"
)

func TestResolveDownloadParams_ModelFromArg(t *testing.T) {
	// Reset global flags
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""
	outputDir = t.TempDir()

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.Model != testModelQwQ {
		t.Errorf("Model = %q, want %q", params.Model, testModelQwQ)
	}
}

func TestResolveDownloadParams_ModelFromState(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""

	dir := t.TempDir()
	outputDir = dir
	state := config.NewState(dir)
	state.SelectedModel = testModel235B
	if err := state.Save(); err != nil {
		t.Fatalf("Save state: %v", err)
	}

	params, err := resolveDownloadParams(nil)
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.Model != testModel235B {
		t.Errorf("Model = %q, want %s", params.Model, testModel235B)
	}
}

func TestResolveDownloadParams_HFHomeFromFlag(t *testing.T) {
	dlHFHome = "/custom/hf"
	dlImage = ""
	dlHFToken = ""
	outputDir = t.TempDir()

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.HFHome != "/custom/hf" {
		t.Errorf("HFHome = %q, want /custom/hf", params.HFHome)
	}
}

func TestResolveDownloadParams_HFHomeFromState(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""

	dir := t.TempDir()
	outputDir = dir
	state := config.NewState(dir)
	state.SelectedModel = testModelQwQ
	state.HFHome = "/state/hf"
	if err := state.Save(); err != nil {
		t.Fatalf("Save state: %v", err)
	}

	params, err := resolveDownloadParams(nil)
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.HFHome != "/state/hf" {
		t.Errorf("HFHome = %q, want /state/hf", params.HFHome)
	}
}

func TestResolveDownloadParams_HFHomeDefault(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""
	outputDir = t.TempDir()

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.HFHome != phases.DefaultHFHome {
		t.Errorf("HFHome = %q, want %q", params.HFHome, phases.DefaultHFHome)
	}
}

func TestResolveDownloadParams_ImageFromFlag(t *testing.T) {
	dlHFHome = ""
	dlImage = "custom-registry/mlnode:custom-tag"
	dlHFToken = ""
	outputDir = t.TempDir()

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.Image != "custom-registry/mlnode:custom-tag" {
		t.Errorf("Image = %q, want custom-registry/mlnode:custom-tag", params.Image)
	}
}

func TestResolveDownloadParams_ImageFromStateVersions(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""

	dir := t.TempDir()
	outputDir = dir
	state := config.NewState(dir)
	state.SelectedModel = testModelQwQ
	state.Versions.MLNode = "3.0.13"
	if err := state.Save(); err != nil {
		t.Fatalf("Save state: %v", err)
	}

	params, err := resolveDownloadParams(nil)
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	want := phases.DefaultMLNodeImage + ":3.0.13"
	if params.Image != want {
		t.Errorf("Image = %q, want %q", params.Image, want)
	}
}

func TestResolveDownloadParams_ImageFromStateTag(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""

	dir := t.TempDir()
	outputDir = dir
	state := config.NewState(dir)
	state.SelectedModel = testModelQwQ
	state.MLNodeImageTag = "3.0.12-blackwell"
	if err := state.Save(); err != nil {
		t.Fatalf("Save state: %v", err)
	}

	params, err := resolveDownloadParams(nil)
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	want := phases.DefaultMLNodeImage + ":3.0.12-blackwell"
	if params.Image != want {
		t.Errorf("Image = %q, want %q", params.Image, want)
	}
}

func TestResolveDownloadParams_ImageDefault(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""
	outputDir = t.TempDir()

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	want := phases.DefaultMLNodeImage + ":" + phases.DefaultMLNodeImageTag
	if params.Image != want {
		t.Errorf("Image = %q, want %q", params.Image, want)
	}
}

func TestResolveDownloadParams_HFTokenFromFlag(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = "hf_flag_token"
	outputDir = t.TempDir()

	// Also set env to verify flag takes precedence
	t.Setenv("HF_TOKEN", "hf_env_token")

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.HFToken != "hf_flag_token" {
		t.Errorf("HFToken = %q, want hf_flag_token", params.HFToken)
	}
}

func TestResolveDownloadParams_HFTokenFromEnv(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""
	outputDir = t.TempDir()

	t.Setenv("HF_TOKEN", "hf_env_token")

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.HFToken != "hf_env_token" {
		t.Errorf("HFToken = %q, want hf_env_token", params.HFToken)
	}
}

func TestResolveDownloadParams_NoState(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""
	outputDir = t.TempDir() // empty dir, no state file

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	// Should use defaults without error
	if params.HFHome != phases.DefaultHFHome {
		t.Errorf("HFHome = %q, want default", params.HFHome)
	}
	if params.Image != phases.DefaultMLNodeImage+":"+phases.DefaultMLNodeImageTag {
		t.Errorf("Image = %q, want default", params.Image)
	}
}

func TestBuildDockerRunArgs(t *testing.T) {
	tests := []struct {
		name   string
		params *downloadParams
		want   []string
	}{
		{
			name: "basic",
			params: &downloadParams{
				Model:  testModelQwQ,
				HFHome: "/data/hf",
				Image:  "ghcr.io/product-science/mlnode:3.0.12",
			},
			want: []string{
				"run", "--rm",
				"-v", "/data/hf:/data/hf",
				"-e", "HF_HOME=/data/hf",
				"ghcr.io/product-science/mlnode:3.0.12",
				"huggingface-cli", "download", testModelQwQ,
			},
		},
		{
			name: "with token",
			params: &downloadParams{
				Model:   testModel235B,
				HFHome:  "/mnt/shared/huggingface",
				Image:   "ghcr.io/product-science/mlnode:3.0.12",
				HFToken: "hf_secret",
			},
			want: []string{
				"run", "--rm",
				"-v", "/mnt/shared/huggingface:/mnt/shared/huggingface",
				"-e", "HF_HOME=/mnt/shared/huggingface",
				"-e", "HF_TOKEN=hf_secret",
				"ghcr.io/product-science/mlnode:3.0.12",
				"huggingface-cli", "download", testModel235B,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDockerRunArgs(tt.params)
			if len(got) != len(tt.want) {
				t.Fatalf("buildDockerRunArgs() len = %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSupportedModels(t *testing.T) {
	if len(phases.SupportedModels) < 3 {
		t.Errorf("SupportedModels has %d entries, want at least 3", len(phases.SupportedModels))
	}

	expected := map[string]bool{
		testModel235B:        false,
		testModelQwQ:         false,
		"Qwen/Qwen3-32B-FP8": false,
	}
	for _, m := range phases.SupportedModels {
		if _, ok := expected[m]; ok {
			expected[m] = true
		}
	}
	for model, found := range expected {
		if !found {
			t.Errorf("SupportedModels missing %q", model)
		}
	}
}

func TestResolveDownloadParams_ArgOverridesState(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""

	dir := t.TempDir()
	outputDir = dir
	state := config.NewState(dir)
	state.SelectedModel = testModelQwQ
	if err := state.Save(); err != nil {
		t.Fatalf("Save state: %v", err)
	}

	// Arg should override state
	params, err := resolveDownloadParams([]string{testModel235B})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.Model != testModel235B {
		t.Errorf("Model = %q, want Qwen/Qwen3-235B-A22B-Instruct-2507-FP8", params.Model)
	}
}

func TestResolveDownloadParams_VersionsPriorityOverTag(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""

	dir := t.TempDir()
	outputDir = dir
	state := config.NewState(dir)
	state.SelectedModel = testModelQwQ
	state.Versions.MLNode = "3.0.13"
	state.MLNodeImageTag = "3.0.12-blackwell"
	if err := state.Save(); err != nil {
		t.Fatalf("Save state: %v", err)
	}

	params, err := resolveDownloadParams(nil)
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}

	// Versions.MLNode should take priority over MLNodeImageTag
	want := phases.DefaultMLNodeImage + ":3.0.13"
	if params.Image != want {
		t.Errorf("Image = %q, want %q (Versions.MLNode should take priority)", params.Image, want)
	}
}

func TestResolveDownloadParams_HFTokenEmpty(t *testing.T) {
	dlHFHome = ""
	dlImage = ""
	dlHFToken = ""
	outputDir = t.TempDir()

	// Ensure HF_TOKEN env is unset
	t.Setenv("HF_TOKEN", "")

	params, err := resolveDownloadParams([]string{testModelQwQ})
	if err != nil {
		t.Fatalf("resolveDownloadParams() error: %v", err)
	}
	if params.HFToken != "" {
		t.Errorf("HFToken = %q, want empty", params.HFToken)
	}
}
