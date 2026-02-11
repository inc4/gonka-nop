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
	testIP             = "10.0.0.1"
	testBeaconStateURL = "https://beaconstate.info/"
	testDefaultHFHome  = "/mnt/shared/huggingface"
)

func TestBuildVLLMArgs(t *testing.T) {
	fp8Model := "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"
	nonFP8Model := "Qwen/QwQ-32B"

	tests := []struct {
		name     string
		state    *config.State
		wantArgs []string
	}{
		{
			name: "Default state (non-FP8 model, no quantization)",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = nonFP8Model
				return s
			}(),
			wantArgs: []string{
				"--gpu-memory-utilization", "0.90",
			},
		},
		{
			name: "FP8 model gets quantization flag",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = fp8Model
				return s
			}(),
			wantArgs: []string{
				"--quantization", kvCacheDtypeFP8,
				"--gpu-memory-utilization", "0.90",
			},
		},
		{
			name: "TP > 1 with FP8 model",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = fp8Model
				s.TPSize = 4
				s.GPUMemoryUtil = 0.92
				return s
			}(),
			wantArgs: []string{
				"--quantization", kvCacheDtypeFP8,
				"--gpu-memory-utilization", "0.92",
				"--tensor-parallel-size", "4",
			},
		},
		{
			name: "PP > 1 with non-FP8 model",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = nonFP8Model
				s.PPSize = 2
				s.GPUMemoryUtil = 0.88
				return s
			}(),
			wantArgs: []string{
				"--gpu-memory-utilization", "0.88",
				"--pipeline-parallel-size", "2",
			},
		},
		{
			name: "KV cache fp8 for tight VRAM",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = fp8Model
				s.TPSize = 8
				s.GPUMemoryUtil = 0.90
				s.MaxModelLen = 240000
				s.KVCacheDtype = kvCacheDtypeFP8
				return s
			}(),
			wantArgs: []string{
				"--quantization", kvCacheDtypeFP8,
				"--gpu-memory-utilization", "0.90",
				"--tensor-parallel-size", "8",
				"--max-model-len", "240000",
				"--kv-cache-dtype", kvCacheDtypeFP8,
			},
		},
		{
			name: "All args combined with FP8 model",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = fp8Model
				s.TPSize = 4
				s.PPSize = 2
				s.GPUMemoryUtil = 0.90
				s.MaxModelLen = 131072
				s.KVCacheDtype = kvCacheDtypeFP8
				return s
			}(),
			wantArgs: []string{
				"--quantization", kvCacheDtypeFP8,
				"--gpu-memory-utilization", "0.90",
				"--tensor-parallel-size", "4",
				"--pipeline-parallel-size", "2",
				"--max-model-len", "131072",
				"--kv-cache-dtype", kvCacheDtypeFP8,
			},
		},
		{
			name: "KV cache auto is excluded",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = fp8Model
				s.GPUMemoryUtil = 0.92
				s.KVCacheDtype = kvCacheDtypeAuto
				return s
			}(),
			wantArgs: []string{
				"--quantization", kvCacheDtypeFP8,
				"--gpu-memory-utilization", "0.92",
			},
		},
		{
			name: "Testnet model (non-FP8) has no quantization",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.SelectedModel = "Qwen/Qwen3-4B-Instruct-2507"
				s.GPUMemoryUtil = 0.88
				return s
			}(),
			wantArgs: []string{
				"--gpu-memory-utilization", "0.88",
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
			args: []string{"--gpu-memory-utilization", "0.90", "--tensor-parallel-size", "4"},
			want: `"--gpu-memory-utilization", "0.90", "--tensor-parallel-size", "4"`,
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
	peers := config.MainnetPersistentPeers()

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
		{"KEYRING_BACKEND", "KEYRING_BACKEND=file"},
		{"ACCOUNT_PUBKEY", "ACCOUNT_PUBKEY=gonka1abc123"},
		{"PUBLIC_URL", "PUBLIC_URL=http://" + testIP + ":8000"},
		{"P2P_EXTERNAL_ADDRESS", "P2P_EXTERNAL_ADDRESS=tcp://" + testIP + ":5000"},
		{"API_SSL_PORT", "API_SSL_PORT=8443"},
		{"HF_HOME", "HF_HOME=/mnt/hf"},
		{"MODEL_NAME", "MODEL_NAME=" + defaultModel},
		{"VLLM_ATTENTION_BACKEND", "VLLM_ATTENTION_BACKEND=FLASH_ATTN"},
		{"DDoS blocked routes", "GONKA_API_BLOCKED_ROUTES='poc-batches training'"},
		{"DDoS exempt routes", "GONKA_API_EXEMPT_ROUTES='chat inference'"},
		{"DISABLE_CHAIN_API", "DISABLE_CHAIN_API=true"},
		{"DISABLE_CHAIN_GRPC", "DISABLE_CHAIN_GRPC=true"},
		{"SYNC_WITH_SNAPSHOTS", "SYNC_WITH_SNAPSHOTS=true"},
		{"ETHEREUM_NETWORK", "ETHEREUM_NETWORK=" + networkMainnet},
		{"BEACON_STATE_URL", "BEACON_STATE_URL=" + testBeaconStateURL},
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
	state.GPUs = []config.GPUInfo{
		{Name: "NVIDIA H100 80GB HBM3", MemoryMB: 81920},
		{Name: "NVIDIA H100 80GB HBM3", MemoryMB: 81920},
		{Name: "NVIDIA H100 80GB HBM3", MemoryMB: 81920},
		{Name: "NVIDIA H100 80GB HBM3", MemoryMB: 81920},
	}

	if err := generateNodeConfig(state); err != nil {
		t.Fatalf("generateNodeConfig() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "node-config.json"))
	if err != nil {
		t.Fatalf("failed to read node-config.json: %v", err)
	}
	content := string(data)

	// Must be valid JSON array (API expects [{...}])
	var parsed []interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("node-config.json is not valid JSON array: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 node config in array, got %d", len(parsed))
	}

	checks := []struct {
		label    string
		contains string
	}{
		{"host is inference", `"host": "inference"`},
		{"id field", `"id": "node1"`},
		{"inference_port", `"inference_port": 5000`},
		{"poc_port", `"poc_port": 8080`},
		{"max_concurrent", `"max_concurrent": 400`},
		{"model name", defaultModel},
		{"tensor-parallel-size", "--tensor-parallel-size"},
		{"hardware type", `NVIDIA H100 80GB HBM3 | 80GB`},
		{"hardware count", `"count": 4`},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("node-config.json missing %s: expected to contain %q", c.label, c.contains)
		}
	}

	// Host should NOT have http:// prefix or public IP
	if strings.Contains(content, `"host": "http://`) {
		t.Error("node-config.json host has http:// prefix")
	}
}

func TestGenerateNodeConfig_HostIsInference(t *testing.T) {
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

	// Host should always be "inference" (Docker service name), not the public IP
	if !strings.Contains(content, `"host": "inference"`) {
		t.Error("expected host to be 'inference' (Docker service name)")
	}
	if strings.Contains(content, "http://") {
		t.Error("node-config.json should not contain http://")
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
	state.BridgeImageTag = "0.2.9-post2"
	state.EthereumNetwork = networkMainnet
	state.BeaconStateURL = testBeaconStateURL

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
		{"RPC port internal", "127.0.0.1:26657:26657"},
		{"Persistent peers", "abc123@node1.example.com:5000"},
		{"Pruning config", "PRUNING=custom"},
		{"Pruning keep recent", "PRUNING_KEEP_RECENT=1000"},
		{"Pruning interval", "PRUNING_INTERVAL=100"},
		{"DDoS blocked routes", "poc-batches training"},
		{"DDoS exempt routes", "chat inference"},
		{"DISABLE_CHAIN_API", "DISABLE_CHAIN_API"},
		{"DISABLE_CHAIN_GRPC", "DISABLE_CHAIN_GRPC"},
		{"SYNC_WITH_SNAPSHOTS", "SYNC_WITH_SNAPSHOTS"},
		{"tmkms service", "tmkms"},
		{"node service", "node"},
		{"api service", "api"},
		{"proxy service", "proxy"},
		{"explorer service", "explorer"},
		{"bridge image tag", "bridge:0.2.9-post2"},
		{"ETHEREUM_NETWORK in bridge", "ETHEREUM_NETWORK=" + networkMainnet},
		{"BEACON_STATE_URL in bridge", "BEACON_STATE_URL=" + testBeaconStateURL},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("docker-compose.yml missing %s: expected to contain %q", c.label, c.contains)
		}
	}

	// Verify bridge image is NOT hardcoded to old version
	if strings.Contains(content, "bridge:0.2.5-post5") {
		t.Error("docker-compose.yml still has hardcoded bridge:0.2.5-post5")
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
	state.MLNodeImageTag = mlnodeBlackwellTag
	state.AttentionBackend = "FLASHINFER"
	state.HFHome = testDefaultHFHome

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
		{"mlnode-308 service", "mlnode-308:"},
		{"mlnode-308 container name", "container_name: mlnode-308"},
		{"MLNode image tag", "mlnode:" + mlnodeBlackwellTag},
		{"Attention backend", "VLLM_ATTENTION_BACKEND=FLASHINFER"},
		{"HF_HOME env", "HF_HOME=" + testDefaultHFHome},
		{"HF_HOME volume", testDefaultHFHome + ":" + testDefaultHFHome},
		{"Model name", "MODEL_NAME=Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"},
		{"GPU reservation", "driver: nvidia"},
		{"node-config volume", "node-config.json"},
		{"config.env", "config.env"},
		{"inference service", "inference:"},
		{"nginx image", "nginx:alpine"},
		{"nginx.conf mount", "nginx.conf:/etc/nginx/nginx.conf"},
		{"PoC port on inference", "127.0.0.1:8080:8080"},
		{"ML inference port on inference", "127.0.0.1:5050:5000"},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("docker-compose.mlnode.yml missing %s: expected to contain %q", c.label, c.contains)
		}
	}

	// mlnode-308 should NOT have published ports (nginx handles that)
	// Count occurrences of "ports:" — should only appear once (for inference service)
	portsSections := strings.Count(content, "ports:")
	if portsSections != 1 {
		t.Errorf("expected 1 ports section (inference only), got %d", portsSections)
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
	// Both services present
	if !strings.Contains(content, "mlnode-308:") {
		t.Error("expected mlnode-308 service")
	}
	if !strings.Contains(content, "inference:") {
		t.Error("expected inference (nginx) service")
	}
}

func TestGenerateNginxConf(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gonka-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	state := config.NewState(tmpDir)

	if err := generateNginxConf(state); err != nil {
		t.Fatalf("generateNginxConf() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "nginx.conf"))
	if err != nil {
		t.Fatalf("failed to read nginx.conf: %v", err)
	}
	content := string(data)

	checks := []struct {
		label    string
		contains string
	}{
		{"upstream port 8080", "mlnode-308:8080"},
		{"upstream port 5000", "mlnode-308:5000"},
		{"listen 5000", "listen 5000"},
		{"listen 8080", "listen 8080"},
		{"Docker DNS resolver", "resolver 127.0.0.11"},
		{"versioned route", "location /v3.0.8/"},
		{"proxy_pass v308", "proxy_pass http://mlnode_v308"},
		{"unlimited body size", "client_max_body_size"},
		{"24h timeout", "proxy_read_timeout        24h"},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("nginx.conf missing %s: expected to contain %q", c.label, c.contains)
		}
	}
}

func TestBuildEnforcedModelArgs(t *testing.T) {
	tests := []struct {
		name     string
		state    *config.State
		contains []string
	}{
		{
			name: "Defaults only (no GPU config)",
			state: func() *config.State {
				return config.NewState("/tmp/test")
			}(),
			contains: []string{
				"--enable-auto-tool-choice",
				"--tool-call-parser hermes",
			},
		},
		{
			name: "With max model len and memory util",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.MaxModelLen = 25000
				s.GPUMemoryUtil = 0.88
				return s
			}(),
			contains: []string{
				"--enable-auto-tool-choice",
				"--tool-call-parser hermes",
				"--max-model-len 25000",
				"--gpu-memory-utilization 0.88",
			},
		},
		{
			name: "With TP and KV cache",
			state: func() *config.State {
				s := config.NewState("/tmp/test")
				s.MaxModelLen = 240000
				s.GPUMemoryUtil = 0.90
				s.TPSize = 8
				s.KVCacheDtype = "fp8"
				return s
			}(),
			contains: []string{
				"--enable-auto-tool-choice",
				"--max-model-len 240000",
				"--gpu-memory-utilization 0.90",
				"--tensor-parallel-size 8",
				"--kv-cache-dtype fp8",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEnforcedModelArgs(tt.state)
			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("buildEnforcedModelArgs() = %q, missing %q", result, want)
				}
			}
		})
	}
}
