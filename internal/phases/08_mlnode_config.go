package phases

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/inc4/gonka-nop/internal/config"
	"github.com/inc4/gonka-nop/internal/ui"
)

// MLNodeConfig handles configuration for MLNode-only topology.
// It generates compose files and registration JSON for a standalone GPU server
// that connects to a remote network node.
type MLNodeConfig struct{}

// NewMLNodeConfig creates a new MLNodeConfig phase.
func NewMLNodeConfig() *MLNodeConfig {
	return &MLNodeConfig{}
}

func (p *MLNodeConfig) Name() string {
	return "MLNode Configuration"
}

func (p *MLNodeConfig) Description() string {
	return "Configuring ML node for remote network node"
}

func (p *MLNodeConfig) ShouldRun(state *config.State) bool {
	return !state.IsPhaseComplete(p.Name()) && state.IsMLNodeOnly()
}

func (p *MLNodeConfig) Run(_ context.Context, state *config.State) error {
	ui.Header("ML Node Configuration")

	// 1. Ask for network node URL (Admin API)
	if state.NetworkNodeURL == "" {
		val, err := ui.Input("Enter the network node Admin API URL (e.g., http://10.0.1.100:9200):", "")
		if err != nil {
			return fmt.Errorf("network node URL: %w", err)
		}
		if val == "" {
			return fmt.Errorf("network node Admin API URL is required for mlnode-only setup")
		}
		state.NetworkNodeURL = val
	}
	ui.Detail("Network node Admin API: %s", state.NetworkNodeURL)

	// 2. Ask for network node private IP (for PoC callback reachability)
	if state.NetworkNodeIP == "" {
		val, err := ui.Input("Enter the network node private IP (for PoC callback, e.g., 10.0.1.100):", "")
		if err != nil {
			return fmt.Errorf("network node IP: %w", err)
		}
		if val == "" {
			return fmt.Errorf("network node private IP is required for PoC callback routing")
		}
		state.NetworkNodeIP = val
	}
	ui.Detail("Network node IP (PoC callback): %s", state.NetworkNodeIP)

	// 3. Ask for this server's private IP (for registration host field)
	if state.PublicIP == "" {
		val, err := ui.Input("Enter this ML node's IP (reachable from network node):", "")
		if err != nil {
			return fmt.Errorf("ml node IP: %w", err)
		}
		if val == "" {
			return fmt.Errorf("ml node IP is required (network node must reach this server)")
		}
		state.PublicIP = val
	}
	ui.Detail("ML node IP: %s", state.PublicIP)

	// 4. Ask for HF_HOME
	if state.HFHome == "" {
		val, err := ui.Input("Enter HuggingFace cache directory:", DefaultHFHome)
		if err != nil {
			return fmt.Errorf("hf home: %w", err)
		}
		if val == "" {
			val = DefaultHFHome
		}
		state.HFHome = val
	}
	ui.Detail("HF_HOME: %s", state.HFHome)

	// 5. Set defaults
	if state.InferencePort == 0 {
		state.InferencePort = 5050
	}
	if state.PoCPort == 0 {
		state.PoCPort = 8080
	}
	if state.MLNodeID == "" {
		state.MLNodeID = "node1"
	}

	// 6. Generate configs
	if err := p.generateMLNodeCompose(state); err != nil {
		return fmt.Errorf("generate mlnode compose: %w", err)
	}
	if err := p.generateNginxConf(state); err != nil {
		return fmt.Errorf("generate nginx conf: %w", err)
	}
	if err := p.generateMLNodeEnv(state); err != nil {
		return fmt.Errorf("generate config.env: %w", err)
	}

	// 7. Set compose files for deploy phase
	state.ComposeFiles = []string{"docker-compose.mlnode.yml"}

	// 8. Generate registration JSON
	if err := p.generateRegistrationJSON(state); err != nil {
		return fmt.Errorf("generate registration JSON: %w", err)
	}

	// 9. Show registration instructions
	p.showRegistrationInstructions(state)

	return nil
}

// generateMLNodeCompose generates docker-compose.mlnode.yml for standalone ML node.
func (p *MLNodeConfig) generateMLNodeCompose(state *config.State) error {
	// Image tag priority: GitHub version (most current) > GPU detection > hardcoded fallback
	mlnodeTag := DefaultMLNodeImageTag
	if state.Versions.MLNode != "" {
		mlnodeTag = state.Versions.MLNode
	} else if state.MLNodeImageTag != "" {
		mlnodeTag = state.MLNodeImageTag
	}
	mlnodeImage := DefaultMLNodeImage + ":" + mlnodeTag

	nginxImage := "nginx:1.28.0"
	if state.Versions.Nginx != "" {
		nginxImage = "nginx:" + state.Versions.Nginx
	}

	backend := state.AttentionBackend
	if backend == "" {
		backend = defaultAttentionBackend
	}

	// For MLNode-only: bind ports to this server's IP so only the private network
	// can reach them. NEVER 0.0.0.0 (public exposure = hijack risk per validator chat).
	bindIP := state.PublicIP
	content := fmt.Sprintf(`services:
  mlnode-308:
    image: %s
    hostname: mlnode-308
    restart: always
    ipc: host
    command: uvicorn api.app:app --host=0.0.0.0 --port=8080
    volumes:
      - %s:/root/.cache
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    environment:
      - HF_HOME=/root/.cache
      - VLLM_ATTENTION_BACKEND=%s

  inference:
    image: %s
    hostname: inference
    restart: always
    ports:
      - "%s:%d:5000"
      - "%s:%d:8080"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
`, mlnodeImage, state.HFHome, backend, nginxImage, bindIP, state.InferencePort, bindIP, state.PoCPort)

	outPath := filepath.Join(state.OutputDir, "docker-compose.mlnode.yml")
	ui.Success("Generated %s", outPath)
	return os.WriteFile(outPath, []byte(content), 0600)
}

// generateNginxConf generates nginx.conf for local routing to mlnode-308.
// Uses the official Gonka nginx template with version-prefix stripping
// (e.g., /v3.0.8/api/v1/state → /api/v1/state) and long timeouts.
func (p *MLNodeConfig) generateNginxConf(state *config.State) error {
	content := `events {}

http {
    resolver 127.0.0.11 valid=10s;
    resolver_timeout 5s;

    upstream mlnode_v308 {
        zone mlnode_v308 64k;
        server mlnode-308:8080 resolve;
    }

    server {
        listen 8080;
        client_max_body_size 0;
        proxy_connect_timeout 24h;
        proxy_send_timeout 24h;
        proxy_read_timeout 24h;

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
        client_max_body_size 0;
        proxy_connect_timeout 24h;
        proxy_send_timeout 24h;
        proxy_read_timeout 24h;

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
	outPath := filepath.Join(state.OutputDir, "nginx.conf")
	ui.Success("Generated %s", outPath)
	return os.WriteFile(outPath, []byte(content), 0600)
}

// generateMLNodeEnv generates a minimal config.env for ML-related variables only.
func (p *MLNodeConfig) generateMLNodeEnv(state *config.State) error {
	model := state.SelectedModel
	if model == "" {
		model = DefaultModel
	}
	backend := state.AttentionBackend
	if backend == "" {
		backend = defaultAttentionBackend
	}

	content := fmt.Sprintf(`# Gonka NOP - ML Node config.env (mlnode-only topology)
# Generated by gonka-nop setup --type mlnode

# Model
MODEL_NAME=%s
VLLM_ATTENTION_BACKEND=%s

# HuggingFace
HF_HOME=%s

# Ports
PORT=%d
INFERENCE_PORT=%d
`, model, backend, state.HFHome, state.PoCPort, state.InferencePort)

	outPath := filepath.Join(state.OutputDir, "config.env")
	ui.Success("Generated %s", outPath)
	return os.WriteFile(outPath, []byte(content), 0600)
}

// mlnodeRegistration is the JSON structure for Admin API POST /admin/v1/nodes.
type mlnodeRegistration struct {
	ID            string                       `json:"id"`
	Host          string                       `json:"host"`
	InferencePort int                          `json:"inference_port"`
	PoCPort       int                          `json:"poc_port"`
	MaxConcurrent int                          `json:"max_concurrent"`
	Models        map[string]mlnodeModelConfig `json:"models"`
	Hardware      []mlnodeHardware             `json:"hardware,omitempty"`
}

type mlnodeModelConfig struct {
	Args []string `json:"args"`
}

type mlnodeHardware struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// generateRegistrationJSON creates mlnode-registration.json for use on the network node.
func (p *MLNodeConfig) generateRegistrationJSON(state *config.State) error {
	model := state.SelectedModel
	if model == "" {
		model = DefaultModel
	}

	args := buildVLLMArgs(state)

	maxConcurrent := 100 * len(state.GPUs)
	if maxConcurrent < 100 {
		maxConcurrent = 500
	}

	reg := mlnodeRegistration{
		ID:            state.MLNodeID,
		Host:          state.PublicIP,
		InferencePort: state.InferencePort,
		PoCPort:       state.PoCPort,
		MaxConcurrent: maxConcurrent,
		Models: map[string]mlnodeModelConfig{
			model: {Args: args},
		},
	}

	// Add hardware info if GPUs detected
	if len(state.GPUs) > 0 {
		gpuName := state.GPUs[0].Name
		vram := state.GPUs[0].MemoryMB / 1024
		reg.Hardware = []mlnodeHardware{
			{Type: fmt.Sprintf("%s | %dGB", gpuName, vram), Count: len(state.GPUs)},
		}
	}

	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registration: %w", err)
	}

	outPath := filepath.Join(state.OutputDir, "mlnode-registration.json")
	ui.Success("Generated %s", outPath)
	return os.WriteFile(outPath, data, 0600)
}

// showRegistrationInstructions prints the commands to register this MLNode
// from the network node server.
func (p *MLNodeConfig) showRegistrationInstructions(state *config.State) {
	fmt.Println()
	ui.Header("Register This ML Node")
	ui.Info("Run the following command on your NETWORK NODE server:")
	fmt.Println()
	fmt.Printf("  curl -X POST http://localhost:9200/admin/v1/nodes \\\n")
	fmt.Printf("    -H \"Content-Type: application/json\" \\\n")
	fmt.Printf("    -d @%s/mlnode-registration.json\n", state.OutputDir)
	fmt.Println()
	ui.Info("Or use: gonka-nop ml-node add --config %s/mlnode-registration.json", state.OutputDir)
	fmt.Println()
}
