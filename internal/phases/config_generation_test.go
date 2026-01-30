package phases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

const (
	testKVCacheFP8           = "fp8"
	testIP                   = "10.0.0.1"
	testMLNodeImageBlackwell = "3.0.12-blackwell"
)

func TestBuildVLLMArgs(t *testing.T) {
	tests := []struct {
		name     string
		state    *config.State
		wantArgs []string
	}{
		{
			name:  "Default state (zero memory util defaults to 0.90)",
			state: config.NewState("/tmp/test"),
			wantArgs: []string{
				"--quantization", "fp8",
				"--gpu-memory-utilization", "0.90",
			},
		},
		{
			name: "TP > 1",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.TPSize = 4
				s.GPUMemoryUtil = 0.92
				return s
			}(),
			wantArgs: []string{
				"--quantization", "fp8",
				"--gpu-memory-utilization", "0.92",
				"--tensor-parallel-size", "4",
			},
		},
		{
			name: "PP > 1",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.PPSize = 2
				s.GPUMemoryUtil = 0.88
				return s
			}(),
			wantArgs: []string{
				"--quantization", "fp8",
				"--gpu-memory-utilization", "0.88",
				"--pipeline-parallel-size", "2",
			},
		},
		{
			name: "KV cache fp8 for tight VRAM",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.TPSize = 8
				s.GPUMemoryUtil = 0.90
				s.MaxModelLen = 240000
				s.KVCacheDtype = testKVCacheFP8
				return s
			}(),
			wantArgs: []string{
				"--quantization", "fp8",
				"--gpu-memory-utilization", "0.90",
				"--tensor-parallel-size", "8",
				"--max-model-len", "240000",
				"--kv-cache-dtype", "fp8",
			},
		},
		{
			name: "All args combined",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.TPSize = 4
				s.PPSize = 2
				s.GPUMemoryUtil = 0.90
				s.MaxModelLen = 131072
				s.KVCacheDtype = testKVCacheFP8
				return s
			}(),
			wantArgs: []string{
				"--quantization", "fp8",
				"--gpu-memory-utilization", "0.90",
				"--tensor-parallel-size", "4",
				"--pipeline-parallel-size", "2",
				"--max-model-len", "131072",
				"--kv-cache-dtype", "fp8",
			},
		},
		{
			name: "KV cache auto is excluded",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.GPUMemoryUtil = 0.92
				s.KVCacheDtype = kvCacheDtypeAuto
				return s
			}(),
			wantArgs: []string{
				"--quantization", "fp8",
				"--gpu-memory-utilization", "0.92",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildVLLMArgs(tt.state)
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("buildVLLMArgs() returned %d args, want %d\ngot:  %v\nwant: %v", len(got), len(tt.wantArgs), got, tt.wantArgs)
			}
			for i, arg := range got {
				if arg != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestBuildVLLMArgs_DefaultMemoryUtil(t *testing.T) {
	state := config.NewState("/tmp/test")
	// GPUMemoryUtil is 0 (zero value)
	args := buildVLLMArgs(state)
	for i, arg := range args {
		if arg == "--gpu-memory-utilization" {
			if i+1 >= len(args) {
				t.Fatal("missing value after --gpu-memory-utilization")
			}
			if args[i+1] != "0.90" {
				t.Errorf("default memory util = %s, want 0.90", args[i+1])
			}
			return
		}
	}
	t.Error("--gpu-memory-utilization not found in args")
}

func TestFormatJSONArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "Empty args",
			args: []string{},
			want: "",
		},
		{
			name: "Single arg",
			args: []string{"--quantization"},
			want: `"--quantization"`,
		},
		{
			name: "Multiple args",
			args: []string{"--quantization", "fp8", "--tensor-parallel-size", "4"},
			want: `"--quantization", "fp8", "--tensor-parallel-size", "4"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatJSONArgs(tt.args)
			if got != tt.want {
				t.Errorf("formatJSONArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultPersistentPeers(t *testing.T) {
	peers := defaultPersistentPeers()

	if len(peers) != 11 {
		t.Errorf("expected 11 peers, got %d", len(peers))
	}

	for i, peer := range peers {
		if !strings.Contains(peer, "@") {
			t.Errorf("peer[%d] = %q: missing @", i, peer)
		}
		if !strings.Contains(peer, ":") {
			t.Errorf("peer[%d] = %q: missing port", i, peer)
		}
	}
}

func TestGenerateConfigEnv(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	state.KeyName = "test-key"
	state.AccountPubKey = "gonka1abc123"
	state.PublicIP = testIP
	state.P2PPort = 5000
	state.APIPort = 8000
	state.HFHome = "/mnt/hf"
	state.SelectedModel = defaultModel
	state.AttentionBackend = "FLASH_ATTN"

	if err := generateConfigEnv(state); err != nil {
		t.Fatalf("generateConfigEnv() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "config.env"))
	if err != nil {
		t.Fatalf("failed to read config.env: %v", err)
	}
	content := string(data)

	checks := []struct {
		label    string
		contains string
	}{
		{"KEY_NAME", "KEY_NAME=test-key"},
		{"ACCOUNT_PUBKEY", "ACCOUNT_PUBKEY=gonka1abc123"},
		{"PUBLIC_URL", "PUBLIC_URL=http://" + testIP + ":8000"},
		{"P2P_EXTERNAL_ADDRESS", "P2P_EXTERNAL_ADDRESS=tcp://" + testIP + ":5000"},
		{"HF_HOME", "HF_HOME=/mnt/hf"},
		{"MODEL_NAME", "MODEL_NAME=" + defaultModel},
		{"VLLM_ATTENTION_BACKEND", "VLLM_ATTENTION_BACKEND=FLASH_ATTN"},
		{"DDoS blocked routes", "GONKA_API_BLOCKED_ROUTES"},
		{"DISABLE_CHAIN_API", "DISABLE_CHAIN_API=true"},
		{"DISABLE_CHAIN_GRPC", "DISABLE_CHAIN_GRPC=true"},
		{"SYNC_WITH_SNAPSHOTS", "SYNC_WITH_SNAPSHOTS=true"},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("config.env missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestGenerateConfigEnv_DefaultsPeers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	// PersistentPeers left empty — should auto-populate

	if err := generateConfigEnv(state); err != nil {
		t.Fatalf("generateConfigEnv() error: %v", err)
	}

	if len(state.PersistentPeers) != 11 {
		t.Errorf("expected 11 default peers, got %d", len(state.PersistentPeers))
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "config.env"))
	if err != nil {
		t.Fatalf("failed to read config.env: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "gonka.spv.re:5000") {
		t.Error("config.env missing default persistent peer")
	}
}

func TestGenerateNodeConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	state.PublicIP = testIP
	state.SelectedModel = defaultModel
	state.TPSize = 4
	state.GPUMemoryUtil = 0.92
	state.MaxModelLen = 32768

	if err := generateNodeConfig(state); err != nil {
		t.Fatalf("generateNodeConfig() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "node-config.json"))
	if err != nil {
		t.Fatalf("failed to read node-config.json: %v", err)
	}
	content := string(data)

	// Must be valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("node-config.json is not valid JSON: %v", err)
	}

	// Host should NOT have http:// prefix
	if strings.Contains(content, `"host": "http://`) {
		t.Error("node-config.json host has http:// prefix")
	}
	if !strings.Contains(content, `"host": "`+testIP+`"`) {
		t.Error("node-config.json missing host field")
	}

	// Model name should be present
	if !strings.Contains(content, defaultModel) {
		t.Error("node-config.json missing model name")
	}

	// vLLM args should contain TP
	if !strings.Contains(content, "--tensor-parallel-size") {
		t.Error("node-config.json missing tensor-parallel-size")
	}
}

func TestGenerateNodeConfig_StripHTTPPrefix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	state.PublicIP = "http://1.2.3.4"
	state.SelectedModel = defaultModel

	if err := generateNodeConfig(state); err != nil {
		t.Fatalf("generateNodeConfig() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "node-config.json"))
	if err != nil {
		t.Fatalf("failed to read node-config.json: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "http://") {
		t.Error("node-config.json should strip http:// prefix from host")
	}
	if !strings.Contains(content, `"host": "1.2.3.4"`) {
		t.Error("expected host to be '1.2.3.4' after stripping http://")
	}
}

func TestGenerateNodeConfig_StripHTTPSPrefix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	state.PublicIP = "https://1.2.3.4"
	state.SelectedModel = defaultModel

	if err := generateNodeConfig(state); err != nil {
		t.Fatalf("generateNodeConfig() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "node-config.json"))
	if err != nil {
		t.Fatalf("failed to read node-config.json: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "https://") {
		t.Error("node-config.json should strip https:// prefix from host")
	}
}

func TestGenerateNodeConfig_DefaultModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	state.PublicIP = testIP
	// SelectedModel left empty

	if err := generateNodeConfig(state); err != nil {
		t.Fatalf("generateNodeConfig() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "node-config.json"))
	if err != nil {
		t.Fatalf("failed to read node-config.json: %v", err)
	}

	if !strings.Contains(string(data), defaultModel) {
		t.Errorf("expected default model %q in node-config.json", defaultModel)
	}
}

func TestGenerateDockerCompose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	state.PersistentPeers = []string{
		"abc123@node1.example.com:5000",
		"def456@node2.example.com:5000",
	}

	if err := generateDockerCompose(state); err != nil {
		t.Fatalf("generateDockerCompose() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}
	content := string(data)

	checks := []struct {
		label    string
		contains string
	}{
		{"Internal ML callback port", "127.0.0.1:9100:9100"},
		{"Internal admin API port", "127.0.0.1:9200:9200"},
		{"P2P port", "5000:26656"},
		{"RPC port", "26657:26657"},
		{"Persistent peers", "abc123@node1.example.com:5000"},
		{"Pruning config", "PRUNING=custom"},
		{"Pruning keep recent", "PRUNING_KEEP_RECENT=1000"},
		{"Pruning interval", "PRUNING_INTERVAL=100"},
		{"DDoS blocked routes", "GONKA_API_BLOCKED_ROUTES"},
		{"DISABLE_CHAIN_API", "DISABLE_CHAIN_API"},
		{"DISABLE_CHAIN_GRPC", "DISABLE_CHAIN_GRPC"},
		{"SYNC_WITH_SNAPSHOTS", "SYNC_WITH_SNAPSHOTS"},
		{"tmkms service", "tmkms"},
		{"node service", "node"},
		{"api service", "api"},
		{"proxy service", "proxy"},
		{"explorer service", "explorer"},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("docker-compose.yml missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestGenerateMLNodeCompose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	state.SelectedModel = "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"
	state.MLNodeImageTag = testMLNodeImageBlackwell
	state.AttentionBackend = "FLASHINFER"
	state.HFHome = "/mnt/shared/huggingface"

	if err := generateMLNodeCompose(state); err != nil {
		t.Fatalf("generateMLNodeCompose() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "docker-compose.mlnode.yml"))
	if err != nil {
		t.Fatalf("failed to read docker-compose.mlnode.yml: %v", err)
	}
	content := string(data)

	checks := []struct {
		label    string
		contains string
	}{
		{"MLNode image tag", "mlnode:" + testMLNodeImageBlackwell},
		{"Attention backend", "VLLM_ATTENTION_BACKEND=FLASHINFER"},
		{"HF_HOME env", "HF_HOME=/mnt/shared/huggingface"},
		{"HF_HOME volume", "/mnt/shared/huggingface:/mnt/shared/huggingface"},
		{"Model name", "MODEL_NAME=Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"},
		{"PoC port internal", "127.0.0.1:8080:8080"},
		{"ML inference port internal", "127.0.0.1:5050:5050"},
		{"GPU reservation", "driver: nvidia"},
		{"node-config volume", "node-config.json"},
		{"config.env", "config.env"},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("docker-compose.mlnode.yml missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestGenerateMLNodeCompose_Defaults(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)
	// Leave SelectedModel, MLNodeImageTag, AttentionBackend, HFHome empty

	if err := generateMLNodeCompose(state); err != nil {
		t.Fatalf("generateMLNodeCompose() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "docker-compose.mlnode.yml"))
	if err != nil {
		t.Fatalf("failed to read docker-compose.mlnode.yml: %v", err)
	}
	content := string(data)

	// Should use defaults
	if !strings.Contains(content, "mlnode:"+defaultMLNodeImageTag) {
		t.Errorf("expected default image tag %q", defaultMLNodeImageTag)
	}
	if !strings.Contains(content, "MODEL_NAME="+defaultModel) {
		t.Errorf("expected default model %q", defaultModel)
	}
	if !strings.Contains(content, "VLLM_ATTENTION_BACKEND="+defaultAttentionBackend) {
		t.Errorf("expected default attention backend %q", defaultAttentionBackend)
	}
}
