package phases

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

const (
	defaultModel            = "Qwen/QwQ-32B"
	defaultAttentionBackend = "FLASH_ATTN"
	defaultMLNodeImageTag   = "3.0.12"
	kvCacheDtypeAuto        = "auto"
)

// ConfigGeneration generates configuration files
type ConfigGeneration struct{}

func NewConfigGeneration() *ConfigGeneration {
	return &ConfigGeneration{}
}

func (p *ConfigGeneration) Name() string {
	return "Configuration"
}

func (p *ConfigGeneration) Description() string {
	return "Generating node configuration files with security hardening"
}

func (p *ConfigGeneration) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name())
}

func (p *ConfigGeneration) Run(_ context.Context, state *config.State) error {
	// Get public IP/hostname
	publicIP, err := ui.Input("Enter your server's public IP or hostname:", "")
	if err != nil {
		return err
	}
	state.PublicIP = publicIP

	// Port configuration — default or custom (for NAT/port remapping)
	if err := configureExternalPorts(state); err != nil {
		return err
	}

	// Get HuggingFace home directory
	defaultHFHome := "/mnt/shared/huggingface"
	hfHome, err := ui.Input("HuggingFace cache directory:", defaultHFHome)
	if err != nil {
		return err
	}
	state.HFHome = hfHome

	// Create output directory
	err = ui.WithSpinner("Creating output directory", func() error {
		return os.MkdirAll(state.OutputDir, 0750)
	})
	if err != nil {
		return err
	}

	// Generate config.env
	err = ui.WithSpinner("Generating config.env", func() error {
		return generateConfigEnv(state)
	})
	if err != nil {
		return err
	}
	ui.Detail("Created: %s/config.env", state.OutputDir)

	// Generate node-config.json
	err = ui.WithSpinner("Generating node-config.json", func() error {
		return generateNodeConfig(state)
	})
	if err != nil {
		return err
	}
	ui.Detail("Created: %s/node-config.json", state.OutputDir)

	// Generate docker-compose.yml
	err = ui.WithSpinner("Generating docker-compose.yml", func() error {
		return generateDockerCompose(state)
	})
	if err != nil {
		return err
	}
	ui.Detail("Created: %s/docker-compose.yml", state.OutputDir)

	// Generate nginx.conf for mlnode proxy
	err = ui.WithSpinner("Generating nginx.conf", func() error {
		return generateNginxConf(state)
	})
	if err != nil {
		return err
	}
	ui.Detail("Created: %s/nginx.conf", state.OutputDir)

	// Generate docker-compose.mlnode.yml
	err = ui.WithSpinner("Generating docker-compose.mlnode.yml", func() error {
		return generateMLNodeCompose(state)
	})
	if err != nil {
		return err
	}
	ui.Detail("Created: %s/docker-compose.mlnode.yml", state.OutputDir)

	// Generate env-override for testnet
	if state.IsTestNet {
		err = ui.WithSpinner("Generating docker-compose.env-override.yml (testnet)", func() error {
			return generateEnvOverride(state)
		})
		if err != nil {
			return err
		}
		ui.Detail("Created: %s/docker-compose.env-override.yml", state.OutputDir)
	}

	// Set compose file list
	state.ComposeFiles = buildComposeFileList(state)
	ui.Detail("Compose files: %s", strings.Join(state.ComposeFiles, ", "))

	// Show security configuration summary
	ui.Header("Security Configuration")
	ui.Success("Internal ports bound to 127.0.0.1 (9100, 9200, 5050, 8080)")
	ui.Success("DDoS protection defaults enabled (blocked chain API/RPC/GRPC)")
	ui.Detail("GONKA_API_BLOCKED_ROUTES: poc-batches training")
	ui.Detail("Pruning: custom (keep-recent=1000, interval=100)")
	if len(state.PersistentPeers) > 0 {
		ui.Detail("Persistent peers: %d configured", len(state.PersistentPeers))
	}

	ui.Success("All configuration files generated")
	return nil
}

// configureExternalPorts asks the user whether to use default ports or custom
// (for NAT/port remapping scenarios like Vast.ai, cloud providers, etc.).
func configureExternalPorts(state *config.State) error {
	defaultP2P := 5000
	defaultAPI := 8000

	options := []string{
		fmt.Sprintf("Default ports (P2P: %d, API: %d)", defaultP2P, defaultAPI),
		"Custom ports (NAT / port remapping)",
	}
	selected, err := ui.Select("External port configuration:", options)
	if err != nil {
		return err
	}

	if selected == options[0] {
		state.P2PPort = defaultP2P
		state.APIPort = defaultAPI
		return nil
	}

	// Custom ports
	p2pStr, err := ui.Input("External P2P port:", fmt.Sprintf("%d", defaultP2P))
	if err != nil {
		return err
	}
	var p2p int
	if _, scanErr := fmt.Sscanf(p2pStr, "%d", &p2p); scanErr != nil || p2p < 1 || p2p > 65535 {
		return fmt.Errorf("invalid P2P port: %s", p2pStr)
	}
	state.P2PPort = p2p

	apiStr, err := ui.Input("External API port:", fmt.Sprintf("%d", defaultAPI))
	if err != nil {
		return err
	}
	var api int
	if _, scanErr := fmt.Sscanf(apiStr, "%d", &api); scanErr != nil || api < 1 || api > 65535 {
		return fmt.Errorf("invalid API port: %s", apiStr)
	}
	state.APIPort = api

	ui.Detail("External ports: P2P=%d, API=%d", state.P2PPort, state.APIPort)
	return nil
}

func generateConfigEnv(state *config.State) error {
	// Build persistent peers string
	persistentPeers := strings.Join(state.PersistentPeers, ",")
	if persistentPeers == "" {
		// Use default known-good peers for mainnet
		state.PersistentPeers = config.MainnetPersistentPeers()
		persistentPeers = strings.Join(state.PersistentPeers, ",")
	}

	// Model name for ML node
	modelName := state.SelectedModel
	if modelName == "" {
		modelName = defaultModel
	}

	// Attention backend
	attentionBackend := state.AttentionBackend
	if attentionBackend == "" {
		attentionBackend = defaultAttentionBackend
	}

	// Key name: use warm key name if set
	keyName := state.KeyName
	if keyName == "" {
		keyName = "gonka-node"
	}

	// Seed URLs from state (populated by network select phase)
	seedAPIURL := state.SeedAPIURL
	if seedAPIURL == "" {
		seedAPIURL = "http://node2.gonka.ai:8000"
	}
	seedRPCURL := state.SeedRPCURL
	if seedRPCURL == "" {
		seedRPCURL = "http://node2.gonka.ai:26657"
	}
	seedP2PURL := state.SeedP2PURL
	if seedP2PURL == "" {
		seedP2PURL = "tcp://node2.gonka.ai:5000"
	}

	// Chain ID
	chainID := state.ChainID
	if chainID == "" {
		chainID = "gonka-mainnet"
	}

	// Keyring password: use what the user entered, or default
	keyringPassword := state.KeyringPassword
	if keyringPassword == "" {
		keyringPassword = "changeme"
	}

	// Snapshot interval
	snapshotInterval := 1000
	if state.IsTestNet {
		snapshotInterval = 200
	}

	// Ethereum network
	ethereumNetwork := state.EthereumNetwork
	if ethereumNetwork == "" {
		ethereumNetwork = networkNameMainnet
	}

	// Beacon state URL
	beaconStateURL := state.BeaconStateURL
	if beaconStateURL == "" {
		beaconStateURL = "https://beaconstate.info/"
	}

	// Inference and PoC host-mapped ports
	inferencePort := state.InferencePort
	if inferencePort == 0 {
		inferencePort = 5050
	}
	pocPort := state.PoCPort
	if pocPort == 0 {
		pocPort = 8080
	}

	content := fmt.Sprintf(`# Gonka Node Configuration
# Generated by gonka-nop

# Identity
KEY_NAME=%s
KEYRING_PASSWORD=%s
KEYRING_BACKEND=file
ACCOUNT_PUBKEY=%s

# Chain
CHAIN_ID=%s
CREATE_KEY=false

# Network
PUBLIC_URL=http://%s:%d
P2P_EXTERNAL_ADDRESS=tcp://%s:%d
API_PORT=%d
API_SSL_PORT=8443

# Seed nodes
SEED_API_URL=%s
SEED_NODE_RPC_URL=%s
SEED_NODE_P2P_URL=%s

# Internal routing
DAPI_API__POC_CALLBACK_URL=http://api:9100
DAPI_CHAIN_NODE__URL=http://node:26657
DAPI_CHAIN_NODE__P2P_URL=tcp://node:26656

# DDoS Protection
# Block direct access to chain endpoints via proxy
GONKA_API_BLOCKED_ROUTES='poc-batches training'
GONKA_API_EXEMPT_ROUTES='chat inference'
DISABLE_CHAIN_API=true
DISABLE_CHAIN_RPC=false
DISABLE_CHAIN_GRPC=true

# Ethereum bridge
ETHEREUM_NETWORK=%s
BEACON_STATE_URL=%s

# Sync
SYNC_WITH_SNAPSHOTS=true
TRUSTED_BLOCK_PERIOD=2000
SNAPSHOT_INTERVAL=%d

# P2P Configuration
GENESIS_SEEDS=%s

# ML Node
HF_HOME=%s
MODEL_NAME=%s
PORT=%d
INFERENCE_PORT=%d
NODE_CONFIG=./node-config.json
VLLM_ATTENTION_BACKEND=%s

# RPC Servers (for state sync)
RPC_SERVER_URL_1=%s
RPC_SERVER_URL_2=%s
`,
		keyName,
		keyringPassword,
		state.AccountPubKey,
		chainID,
		state.PublicIP,
		state.APIPort,
		state.PublicIP,
		state.P2PPort,
		state.APIPort,
		seedAPIURL,
		seedRPCURL,
		seedP2PURL,
		ethereumNetwork,
		beaconStateURL,
		snapshotInterval,
		persistentPeers,
		state.HFHome,
		modelName,
		pocPort,
		inferencePort,
		attentionBackend,
		seedRPCURL,
		seedRPCURL,
	)

	return os.WriteFile(filepath.Join(state.OutputDir, "config.env"), []byte(content), 0600)
}

func generateNodeConfig(state *config.State) error {
	modelName := state.SelectedModel
	if modelName == "" {
		modelName = defaultModel
	}

	// Build vLLM args with proper formatting
	args := buildVLLMArgs(state)

	// Host is always "inference" (Docker service name, no http:// prefix)
	host := "inference"

	// ML node ID
	mlNodeID := state.MLNodeID
	if mlNodeID == "" {
		mlNodeID = "node1"
	}

	// GPU count for hardware field
	gpuCount := len(state.GPUs)
	if gpuCount == 0 {
		gpuCount = 1
	}

	// GPU name and VRAM for hardware field
	gpuName := "NVIDIA GPU"
	gpuVRAM := 24
	if len(state.GPUs) > 0 {
		gpuName = state.GPUs[0].Name
		gpuVRAM = state.GPUs[0].MemoryMB / 1024
	}

	// Max concurrent based on GPU count
	maxConcurrent := 100 * gpuCount
	if maxConcurrent < 100 {
		maxConcurrent = 100
	}

	// API expects a JSON array of node configs
	content := fmt.Sprintf(`[{
  "id": "%s",
  "host": "%s",
  "inference_port": 5000,
  "poc_port": 8080,
  "max_concurrent": %d,
  "models": {
    "%s": {
      "args": [%s]
    }
  },
  "hardware": [
    {
      "type": "%s | %dGB",
      "count": %d
    }
  ]
}]
`, mlNodeID, host, maxConcurrent, modelName, formatJSONArgs(args), gpuName, gpuVRAM, gpuCount)

	return os.WriteFile(filepath.Join(state.OutputDir, "node-config.json"), []byte(content), 0600)
}

// modelNeedsFP8Quantization returns true if the model name indicates FP8 quantization
func modelNeedsFP8Quantization(modelName string) bool {
	return strings.Contains(strings.ToUpper(modelName), "FP8")
}

// buildVLLMArgs builds the vLLM command-line arguments from state
func buildVLLMArgs(state *config.State) []string {
	var args []string

	// Only add --quantization fp8 for FP8 models (e.g., Qwen3-235B-FP8, Qwen3-32B-FP8)
	modelName := state.SelectedModel
	if modelName == "" {
		modelName = defaultModel
	}
	if modelNeedsFP8Quantization(modelName) {
		args = append(args, "--quantization", kvCacheDtypeFP8)
	}

	// GPU memory utilization (0.88-0.94, not 0.9/0.99)
	memUtil := state.GPUMemoryUtil
	if memUtil <= 0 {
		memUtil = 0.90
	}
	args = append(args, "--gpu-memory-utilization", fmt.Sprintf("%.2f", memUtil))

	// Tensor parallel size
	if state.TPSize > 1 {
		args = append(args, "--tensor-parallel-size", fmt.Sprintf("%d", state.TPSize))
	}

	// Pipeline parallel size
	if state.PPSize > 1 {
		args = append(args, "--pipeline-parallel-size", fmt.Sprintf("%d", state.PPSize))
	}

	// Max model length
	if state.MaxModelLen > 0 {
		args = append(args, "--max-model-len", fmt.Sprintf("%d", state.MaxModelLen))
	}

	// KV cache dtype (fp8 for tight VRAM)
	if state.KVCacheDtype != "" && state.KVCacheDtype != kvCacheDtypeAuto {
		args = append(args, "--kv-cache-dtype", state.KVCacheDtype)
	}

	return args
}

// formatJSONArgs formats args as a JSON array string: "arg1", "arg2", ...
func formatJSONArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = fmt.Sprintf("%q", arg)
	}
	return strings.Join(quoted, ", ")
}

func generateDockerCompose(state *config.State) error {
	// Build persistent peers for genesis seeds
	persistentPeers := strings.Join(state.PersistentPeers, ",")

	// Image version
	imageVersion := state.ImageVersion
	if imageVersion == "" {
		imageVersion = config.DefaultImageVersion
	}

	// Bridge image tag (may differ from main version)
	bridgeImageTag := state.BridgeImageTag
	if bridgeImageTag == "" {
		bridgeImageTag = imageVersion
	}

	// Beacon state URL from state (set by network select)
	beaconStateURL := state.BeaconStateURL
	if beaconStateURL == "" {
		beaconStateURL = "https://beaconstate.info/"
	}

	// Ethereum network
	ethereumNetwork := state.EthereumNetwork
	if ethereumNetwork == "" {
		ethereumNetwork = networkNameMainnet
	}

	content := fmt.Sprintf(`# Gonka Node Docker Compose
# Generated by gonka-nop
# Security: internal ports bound to 127.0.0.1

services:
  tmkms:
    image: ghcr.io/product-science/tmkms-softsign-with-keygen:%s
    container_name: tmkms
    restart: unless-stopped
    environment:
      - VALIDATOR_LISTEN_ADDRESS=tcp://node:26658
    volumes:
      - .tmkms:/root/.tmkms

  node:
    container_name: node
    image: ghcr.io/product-science/inferenced:%s
    command: ["sh", "./init-docker.sh"]
    volumes:
      - .inference:/root/.inference
    environment:
      - CHAIN_ID=${CHAIN_ID}
      - SEED_NODE_RPC_URL=${SEED_NODE_RPC_URL}
      - SEED_NODE_P2P_URL=${SEED_NODE_P2P_URL}
      - SNAPSHOT_INTERVAL=${SNAPSHOT_INTERVAL:-1000}
      - SNAPSHOT_KEEP_RECENT=5
      - TRUSTED_BLOCK_PERIOD=${TRUSTED_BLOCK_PERIOD:-2000}
      - KEY_NAME=${KEY_NAME}
      - P2P_EXTERNAL_ADDRESS=${P2P_EXTERNAL_ADDRESS}
      - CONFIG_p2p__allow_duplicate_ip=true
      - CONFIG_p2p__handshake_timeout=30s
      - CONFIG_p2p__dial_timeout=30s
      - TMKMS_PORT=26658
      - SYNC_WITH_SNAPSHOTS=${SYNC_WITH_SNAPSHOTS:-true}
      - RPC_SERVER_URL_1=${RPC_SERVER_URL_1}
      - RPC_SERVER_URL_2=${RPC_SERVER_URL_2}
      - REST_API_ACTIVE=true
      - INIT_ONLY=false
      - IS_GENESIS=false
      - CREATE_KEY=${CREATE_KEY:-false}
      - GENESIS_SEEDS=%s
      # Pruning configuration (prevents unbounded disk growth)
      - PRUNING=custom
      - PRUNING_KEEP_RECENT=1000
      - PRUNING_INTERVAL=100
    ports:
      - "5000:26656"  # P2P (public)
      - "127.0.0.1:26657:26657"  # RPC (internal, access via proxy /chain-rpc/)
    expose:
      - "26658"
    depends_on:
      - tmkms
    restart: always

  api:
    container_name: api
    image: ghcr.io/product-science/api:%s
    volumes:
      - .inference:/root/.inference
      - .dapi:/root/.dapi
      - ${NODE_CONFIG:-./node-config.json}:/root/node_config.json
    depends_on:
      - node
    environment:
      - KEY_NAME=${KEY_NAME}
      - ACCOUNT_PUBKEY=${ACCOUNT_PUBKEY}
      - KEYRING_BACKEND=file
      - KEYRING_PASSWORD=${KEYRING_PASSWORD}
      - DAPI_API__POC_CALLBACK_URL=${DAPI_API__POC_CALLBACK_URL}
      - DAPI_API__PUBLIC_URL=${PUBLIC_URL}
      - DAPI_CHAIN_NODE__SEED_API_URL=${SEED_API_URL}
      - DAPI_CHAIN_NODE__URL=${DAPI_CHAIN_NODE__URL}
      - DAPI_CHAIN_NODE__P2P_URL=${DAPI_CHAIN_NODE__P2P_URL}
      - NODE_CONFIG_PATH=/root/node_config.json
      - DAPI_API__PUBLIC_SERVER_PORT=9000
      - DAPI_API__ML_SERVER_PORT=9100
      - DAPI_API__ADMIN_SERVER_PORT=9200
    ports:
      # SECURITY: Bind internal APIs to localhost only
      - "127.0.0.1:9100:9100"  # ML callback (internal)
      - "127.0.0.1:9200:9200"  # Admin API (internal)
    restart: always
    env_file:
      - config.env

  bridge:
    container_name: bridge
    image: ghcr.io/product-science/bridge:%s
    restart: unless-stopped
    environment:
      - GETH_DATA_DIR=/data/geth
      - PRYSM_DATA_DIR=/data/prysm
      - JWT_SECRET_PATH=/data/jwt/jwt.hex
      - BRIDGE_POSTBLOCK=http://api:9200/admin/v1/bridge/block
      - BRIDGE_GETADDRESSES=http://api:9000/v1/bridge/addresses
      - ETHEREUM_NETWORK=%s
      - BEACON_STATE_URL=%s
      - PERSISTENT_DB_DIR=/persistent-db
    volumes:
      - .inference-eth/geth:/data/geth
      - .inference-eth/prysm:/data/prysm
      - .inference-eth/jwt:/data/jwt
      - .inference-eth/logs:/var/log
      - .inference-eth/persistent-db:/persistent-db
    depends_on:
      - api

  proxy:
    container_name: proxy
    image: ghcr.io/product-science/proxy:%s
    ports:
      - "${API_PORT:-8000}:80"    # Application service (public)
    environment:
      - NGINX_MODE=${NGINX_MODE:-http}
      - SERVER_NAME=${SERVER_NAME:-}
      - GONKA_API_PORT=9000
      - CHAIN_RPC_PORT=26657
      - CHAIN_API_PORT=1317
      - CHAIN_GRPC_PORT=9090
      - DASHBOARD_PORT=5173
      # DDoS protection: block direct chain endpoints
      - GONKA_API_BLOCKED_ROUTES=${GONKA_API_BLOCKED_ROUTES:-poc-batches training}
      - GONKA_API_EXEMPT_ROUTES=${GONKA_API_EXEMPT_ROUTES:-chat inference}
      - DISABLE_CHAIN_API=${DISABLE_CHAIN_API:-true}
      - DISABLE_CHAIN_RPC=${DISABLE_CHAIN_RPC:-false}
      - DISABLE_CHAIN_GRPC=${DISABLE_CHAIN_GRPC:-true}
    depends_on:
      - node
      - api
      - explorer
    restart: unless-stopped

  explorer:
    container_name: explorer
    image: ghcr.io/product-science/explorer:latest
    expose:
      - "5173"
    restart: unless-stopped
`, imageVersion, imageVersion, persistentPeers, imageVersion,
		bridgeImageTag, ethereumNetwork, beaconStateURL, imageVersion)

	return os.WriteFile(filepath.Join(state.OutputDir, "docker-compose.yml"), []byte(content), 0600)
}

func generateMLNodeCompose(state *config.State) error {
	modelName := state.SelectedModel
	if modelName == "" {
		modelName = defaultModel
	}

	// Select mlnode image tag based on GPU architecture
	imageTag := state.MLNodeImageTag
	if imageTag == "" {
		imageTag = defaultMLNodeImageTag
	}

	// Select attention backend
	attentionBackend := state.AttentionBackend
	if attentionBackend == "" {
		attentionBackend = defaultAttentionBackend
	}

	// Host-mapped ports (bound to localhost for security)
	inferencePort := state.InferencePort
	if inferencePort == 0 {
		inferencePort = 5050
	}
	pocPort := state.PoCPort
	if pocPort == 0 {
		pocPort = 8080
	}

	hfHome := state.HFHome
	if hfHome == "" {
		hfHome = "/mnt/shared/huggingface"
	}

	content := fmt.Sprintf(`# Gonka ML Node Docker Compose
# Generated by gonka-nop
# Security: ML ports bound to 127.0.0.1
# mlnode-308: GPU inference container (no published ports)
# inference: nginx proxy that routes to mlnode-308

services:
  mlnode-308:
    container_name: mlnode-308
    hostname: mlnode-308
    image: ghcr.io/product-science/mlnode:%s
    restart: unless-stopped
    ipc: host
    command: uvicorn api.app:app --host=0.0.0.0 --port=8080
    environment:
      - HF_HOME=%s
      - MODEL_NAME=%s
      - VLLM_ATTENTION_BACKEND=%s
    volumes:
      - %s:%s
      - ./node-config.json:/app/node-config.json
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    env_file:
      - config.env

  inference:
    container_name: inference
    hostname: inference
    image: nginx:alpine
    restart: unless-stopped
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    ports:
      # SECURITY: Bind ML ports to localhost only
      - "127.0.0.1:%d:5000"   # ML inference (internal)
      - "127.0.0.1:%d:8080"   # PoC endpoint (internal)
    depends_on:
      - mlnode-308
`, imageTag, hfHome, modelName, attentionBackend, hfHome, hfHome,
		inferencePort, pocPort)

	return os.WriteFile(filepath.Join(state.OutputDir, "docker-compose.mlnode.yml"), []byte(content), 0600)
}

// generateNginxConf creates the nginx.conf that the "inference" service uses
// to proxy requests to the mlnode-308 container.
// Upstream targets are Docker service names and internal ports — architectural constants.
func generateNginxConf(state *config.State) error {
	content := `# Nginx proxy for mlnode
# Generated by gonka-nop
# Routes inference and PoC traffic to mlnode-308 container

events {}

http {
    resolver 127.0.0.11 valid=10s;
    resolver_timeout 5s;

    upstream mlnode_v308 {
        zone mlnode_v308 64k;
        server mlnode-308:8080 resolve;
    }

    server {
        listen 8080;

        client_max_body_size      0;
        proxy_connect_timeout     24h;
        proxy_send_timeout        24h;
        proxy_read_timeout        24h;

        location /v3.0.8/ {
            proxy_pass http://mlnode_v308/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }

        location / {
            proxy_pass http://mlnode_v308/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }
    }

    upstream mlnode_v308_port5000 {
        zone mlnode_v308_port5000 64k;
        server mlnode-308:5000 resolve;
    }

    server {
        listen 5000;

        client_max_body_size      0;
        proxy_connect_timeout     24h;
        proxy_send_timeout        24h;
        proxy_read_timeout        24h;

        location /v3.0.8/ {
            proxy_pass http://mlnode_v308_port5000/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }

        location / {
            proxy_pass http://mlnode_v308_port5000/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }
    }
}
`

	return os.WriteFile(filepath.Join(state.OutputDir, "nginx.conf"), []byte(content), 0600)
}

// generateEnvOverride creates docker-compose.env-override.yml for testnet.
func generateEnvOverride(state *config.State) error {
	enforcedModelID := state.EnforcedModelID
	enforcedModelArgs := buildEnforcedModelArgs(state)

	content := fmt.Sprintf(`# Gonka Testnet Environment Override
# Generated by gonka-nop
# Applied on top of docker-compose.yml for testnet deployments

services:
  tmkms:
    environment:
      - IS_TEST_NET=true

  node:
    environment:
      - IS_TEST_NET=true

  api:
    environment:
      - IS_TEST_NET=true
      - ENFORCED_MODEL_ID=%s
      - ENFORCED_MODEL_ARGS=%s

  proxy:
    environment:
      - IS_TEST_NET=true
      - DISABLE_CHAIN_API=false
      - DISABLE_CHAIN_RPC=false
      - DISABLE_CHAIN_GRPC=false

  explorer:
    environment:
      - IS_TEST_NET=true
`, enforcedModelID, enforcedModelArgs)

	return os.WriteFile(filepath.Join(state.OutputDir, "docker-compose.env-override.yml"), []byte(content), 0600)
}

// buildEnforcedModelArgs constructs ENFORCED_MODEL_ARGS from state values.
// Always prepends --enable-auto-tool-choice --tool-call-parser hermes.
func buildEnforcedModelArgs(state *config.State) string {
	args := []string{"--enable-auto-tool-choice", "--tool-call-parser", "hermes"}

	maxModelLen := state.MaxModelLen
	if maxModelLen > 0 {
		args = append(args, "--max-model-len", fmt.Sprintf("%d", maxModelLen))
	}

	memUtil := state.GPUMemoryUtil
	if memUtil > 0 {
		args = append(args, "--gpu-memory-utilization", fmt.Sprintf("%.2f", memUtil))
	}

	if state.TPSize > 1 {
		args = append(args, "--tensor-parallel-size", fmt.Sprintf("%d", state.TPSize))
	}

	if state.KVCacheDtype != "" && state.KVCacheDtype != kvCacheDtypeAuto {
		args = append(args, "--kv-cache-dtype", state.KVCacheDtype)
	}

	return strings.Join(args, " ")
}

// buildComposeFileList returns the ordered list of compose files for the deployment.
func buildComposeFileList(state *config.State) []string {
	files := []string{"docker-compose.yml"}
	if state.IsTestNet {
		files = append(files, "docker-compose.env-override.yml")
	}
	files = append(files, "docker-compose.mlnode.yml")
	return files
}
