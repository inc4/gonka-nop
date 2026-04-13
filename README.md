# Gonka NOP

[![CI](https://github.com/inc4/gonka-nop/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/inc4/gonka-nop/actions/workflows/ci.yml)
[![Security](https://github.com/inc4/gonka-nop/actions/workflows/security.yml/badge.svg)](https://github.com/inc4/gonka-nop/actions/workflows/security.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/inc4/gonka-nop)](https://goreportcard.com/report/github.com/inc4/gonka-nop)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

One-command CLI for deploying GPU-accelerated Gonka AI inference nodes. Auto-detects GPUs, configures NVIDIA runtime, generates optimized configs, and deploys containers with security hardening.

## What's New (v0.2.1-rc5)

### Blackwell GPU support improvements
- **Auto-detect latest Blackwell image**: queries GHCR registry for the latest `-blackwell` tag (e.g., `3.0.12-post6-blackwell`) instead of using a hardcoded version. Falls back to suffix convention if registry is unreachable.
- **`--mlnode-image` flag**: override the MLNode Docker image entirely for custom builds (e.g., `ghcr.io/segovchik/gonka-b300-image:3.0.13-b300-tp1`). Takes highest priority over all auto-detection.
- **`--attention-backend` flag**: choose between `FLASHINFER` (default) and `FLASH_ATTN` (lower memory footprint for constrained setups like 2×B200 where FlashInfer workspace causes OOM).
- **Interactive prompts**: setup wizard now asks for custom image and attention backend after showing recommended config. Skipped in `--yes` mode when flags are provided.

### Bug fix
- **`gonka-nop status` broken as root**: when `state.json` loaded successfully but `AdminURL` was empty, all HTTP requests silently failed. Non-root users weren't affected because they couldn't read `state.json` (0600 permissions), triggering the default-URL fallback path. Fixed by always initializing `StatusConfig` from defaults before overriding with state values.

## Demo

[![Gonka NOP Demo](https://img.youtube.com/vi/0w6bIEROxUQ/maxresdefault.jpg)](https://youtu.be/0w6bIEROxUQ?si=FJywggqlVax90Ohn)

## Install

```bash
curl -fsSL https://github.com/inc4/gonka-nop/releases/latest/download/gonka-nop -o /usr/local/bin/gonka-nop
chmod +x /usr/local/bin/gonka-nop
```

## Quick Start

```bash
gonka-nop setup      # Interactive setup wizard (installs everything, deploys node)
gonka-nop status     # Check node health, epoch, PoC weight, miss rate
gonka-nop update     # Safe rolling update of containers
```

## Deployment Topologies

Gonka NOP supports three deployment topologies via the `--type` flag:

### Full (default) -- Single server

Network node + ML node on the same server. Best for getting started.

```bash
# Interactive
gonka-nop setup

# Non-interactive
gonka-nop setup --yes --type full \
  --network mainnet \
  --key-workflow quick \
  --key-name my-key \
  --keyring-password "pass" \
  --public-ip 1.2.3.4 \
  --hf-home /data/hf
```

### Network only -- Chain services, no GPU

Runs the blockchain node, API, proxy, and supporting services on a CPU-only server. ML nodes connect remotely.

```bash
# Interactive
gonka-nop setup --type network

# Non-interactive
gonka-nop setup --yes --type network \
  --network mainnet \
  --key-workflow quick \
  --key-name my-key \
  --keyring-password "pass" \
  --public-ip 203.0.113.10
```

What happens:
- Installs Docker (skips NVIDIA drivers)
- Generates keys, config.env, docker-compose.yml (no mlnode compose)
- Exposes port 9100 for PoC callback from remote ML nodes
- Starts chain services and waits for blockchain sync

### ML node only -- GPU inference, no chain

Runs the vLLM inference engine on a dedicated GPU server. Connects to a remote network node.

```bash
# Interactive
gonka-nop setup --type mlnode

# Non-interactive
gonka-nop setup --yes --type mlnode \
  --network mainnet \
  --network-node-url http://203.0.113.10:9200 \
  --public-ip 10.0.1.50 \
  --hf-home /data/hf
```

What happens:
- Installs Docker + NVIDIA drivers + Container Toolkit + Fabric Manager
- Detects GPUs, calculates optimal TP/PP
- Generates docker-compose.mlnode.yml, nginx.conf, config.env
- Downloads model weights and starts ML node containers
- Generates `mlnode-registration.json` for registering with the network node

### Multi-server workflow

```
                          +-----------------------+
                          |  Network Node (.10)   |
                          |  --type network       |
                          |  Chain + API + Proxy  |
                          |  Port 9100 exposed    |
                          +----------+------------+
                                     |
                            private network
                                     |
              +----------------------+----------------------+
              |                                             |
   +----------v-----------+                  +--------------v-------+
   |  ML Node A (.50)     |                  |  ML Node B (.51)     |
   |  --type mlnode        |                  |  --type mlnode        |
   |  8x H100, TP=4       |                  |  8x A100, TP=4       |
   |  2x vLLM instances   |                  |  2x vLLM instances   |
   +----------------------+                  +----------------------+
```

**Step 1:** Deploy network node

```bash
# On the network node server (no GPU needed)
gonka-nop setup --type network -y --network mainnet \
  --key-workflow quick --key-name my-key --keyring-password "pass" \
  --public-ip 203.0.113.10
```

**Step 2:** Deploy ML node(s)

```bash
# On each GPU server
gonka-nop setup --type mlnode -y --network mainnet \
  --network-node-url http://203.0.113.10:9200 \
  --public-ip 10.0.1.50 --hf-home /data/hf
```

**Step 3:** Register ML node(s) from the network node

```bash
# Option A: Use the generated registration file
gonka-nop ml-node add --config /path/to/mlnode-registration.json

# Option B: Interactive registration
gonka-nop ml-node add

# Verify
gonka-nop ml-node list
```

## GPU-Specific Deployment Guides

NOP auto-detects GPU architecture and selects optimal settings. These guides document real-world tested configurations and known issues per hardware class.

### 8× A100 SXM4 80GB (Ampere, sm_80)

Standard configuration. Works out of the box.

```bash
gonka-nop setup
```

| Setting | Value |
|---------|-------|
| Image | `mlnode:3.0.12-post6` (auto-selected) |
| TP | 4 (auto, NOP calculates PP=2 for 8 GPUs) |
| Backend | FLASHINFER |
| gpu-memory-utilization | 0.90 |
| Weight (observed) | ~860 per ML node |

### 2× B200 (Blackwell, sm_100)

Blackwell image auto-detected. **Use FLASH_ATTN** to avoid FlashInfer workspace OOM with chain-enforced `max_model_len=240000`.

```bash
gonka-nop setup --attention-backend FLASH_ATTN
```

| Setting | Value |
|---------|-------|
| Image | `mlnode:3.0.12-post6-blackwell` (auto from GHCR) |
| TP | 2 |
| Backend | **FLASH_ATTN** (FLASHINFER causes OOM on 2×B200) |
| gpu-memory-utilization | 0.88 |
| Weight (observed) | ~920 with [T,T] timeslots |

**Known issue:** The chain enforces `--max-model-len 240000` which pre-allocates a large KV cache. With TP=2 on 2×B200, FlashInfer's workspace buffer (~285MB) doesn't fit in the remaining free memory. Switching to FLASH_ATTN eliminates the workspace allocation entirely.

### 4× B200 (Blackwell, sm_100)

Works with alpha4 image or blackwell image. FLASHINFER is OK because TP=4 means less model weight per GPU → more headroom.

```bash
gonka-nop setup
```

| Setting | Value |
|---------|-------|
| Image | `mlnode:3.0.13-alpha4` or `3.0.12-post6-blackwell` |
| TP | 4 |
| Backend | FLASHINFER (enough headroom at TP=4) |
| gpu-memory-utilization | 0.90 |
| Weight (observed) | ~2,178 |

### 8× B300 SXM6 AC (Blackwell Ultra, sm_103a)

Requires custom image — standard and blackwell images lack sm_103a CUTLASS kernels. Must use `vllm/vllm-openai:v0.15.1-cu130` as base.

```bash
gonka-nop setup \
  --mlnode-image ghcr.io/segovchik/gonka-b300-image:3.0.13-b300-tp1
```

| Setting | Value |
|---------|-------|
| Image | **Custom** (`ghcr.io/segovchik/gonka-b300-image:3.0.13-b300-tp1`) |
| TP | 1 (8 independent instances, one per GPU) |
| Backend | FLASHINFER |
| gpu-memory-utilization | 0.95 |
| max-model-len | 131072 |
| max-num-seqs | 128 |
| Weight (observed) | ~7,700–8,300 |
| PoC throughput | ~8,700 nonces/min |

**Why custom image?** B300 (sm_103a) needs:
1. CUDA 13.0 base (`vllm/vllm-openai:v0.15.1-cu130`) for CUTLASS kernel compatibility
2. Triton ptxas 13.0 (replacing bundled 12.8 that doesn't know sm_103a)
3. Runner patches: TP=1 for maximum PoC throughput (8 instances vs 2 with TP=4)

**Host requirement:** `cuda-compat-13-0` package must be installed if host CUDA toolkit < 13.0. Mount compat libs into container:
```yaml
# docker-compose.mlnode.yml
volumes:
  - /usr/local/cuda-13.0/compat:/usr/local/cuda/compat:ro
environment:
  - LD_LIBRARY_PATH=/usr/local/cuda/compat
```

See [B300 deployment report](docs/b300-mlnode-deployment.md) for the full build process.

### 8× H100/H200 SXM 80GB (Hopper, sm_90)

Standard configuration similar to A100.

```bash
gonka-nop setup
```

| Setting | Value |
|---------|-------|
| Image | `mlnode:3.0.12-post6` (auto-selected) |
| TP | 4 or 8 (NVLink full mesh enables TP=8) |
| Backend | FLASHINFER |
| gpu-memory-utilization | 0.90 |

### Changing Image on a Running Node

Use `ml-node set-image` to swap the MLNode image without full re-setup:

```bash
# Switch to blackwell image
gonka-nop ml-node set-image ghcr.io/product-science/mlnode:3.0.12-post6-blackwell

# Switch to custom B300 image
gonka-nop ml-node set-image ghcr.io/segovchik/gonka-b300-image:3.0.13-b300-tp1
```

This performs a safe rollout: disable → update compose → pull → recreate → enable.

### Spot Instance Recovery

If a spot/preemptible instance is killed and reprovisioned with the old data disk attached:

1. Mount old disk: `mount /dev/vdb4 /mnt`
2. Move Docker storage: set `data-root` in `/etc/docker/daemon.json` to mounted disk
3. Symlink gonka-node: `ln -sf /mnt/root/gonka-node /root/gonka-node`
4. Fix HF cache path: `ln -sf /mnt/mnt/shared/huggingface /mnt/shared/huggingface`
5. Install CUDA compat if needed: `apt-get install cuda-compat-13-0`
6. Start: `set -a && source config.env && set +a && docker compose up -d`

Keys, chain data, and model cache survive on the persistent disk. No re-registration needed if IP stays the same.

## Commands

| Command | Description |
|---------|-------------|
| `setup` | Interactive setup wizard (full node deployment) |
| `setup --type network` | Network-only setup (chain services, no GPU) |
| `setup --type mlnode` | ML node only (GPU inference, remote network node) |
| `status` | Node health: blockchain, epoch, MLNode, security checks |
| `gpu-info` | Detected GPUs with TP/PP/model recommendation |
| `update` | Safe rolling update (`--check` for dry run, `--service` for specific) |
| `repair` | Fix stuck nodes (missing upgrade binaries) |
| `register` | On-chain registration and ML permissions |
| `ml-node list` | List registered ML nodes with status |
| `ml-node add` | Register a new ML node (from file or interactive) |
| `ml-node status` | Detailed ML node status |
| `ml-node enable/disable` | Enable or disable an ML node |
| `ml-node set-image` | Change MLNode Docker image and restart (safe rollout) |
| `download-model` | Pre-download model weights before setup |
| `reset` | Stop containers and clean up |
| `cleanup` | Recover disk space |
| `version` | Print version info |

## Setup Flags

| Flag | Description | Used in |
|------|-------------|---------|
| `--type` | Node topology: `full`, `network`, `mlnode` | All |
| `--network` | Network: `mainnet` or `testnet` | All |
| `--network-node-url` | Admin API URL of network node | `mlnode` |
| `--key-workflow` | Key management: `quick` or `secure` | `full`, `network` |
| `--key-name` | Base name for keys | `full`, `network` |
| `--keyring-password` | Keyring password | `full`, `network` |
| `--public-ip` | Server public/private IP | All |
| `--hf-home` | HuggingFace cache directory | `full`, `mlnode` |
| `--account-pubkey` | Account public key (secure workflow) | `full`, `network` |
| `--mlnode-image` | Custom MLNode Docker image (overrides auto-detection) | `full`, `mlnode` |
| `--attention-backend` | vLLM attention backend: `FLASHINFER` or `FLASH_ATTN` | `full`, `mlnode` |
| `-y, --yes` | Non-interactive mode | All |
| `-o, --output` | Output directory (default: `./gonka-node`) | All |

## Manual vs Automated

| Task | Manual | With gonka-nop |
|------|--------|----------------|
| Install NVIDIA drivers + toolkit | Add repos, install packages, configure runtime, restart Docker | One confirmation prompt |
| Detect GPUs and choose config | Parse nvidia-smi, pick from 6+ node-config variants | Auto-detected, optimal TP/PP calculated |
| Fill config.env | Edit 15+ variables, look up seed nodes | Interactive prompts with validation |
| Port security | Manually edit compose, set DOCKER-USER iptables | Ports bound to 127.0.0.1 by default |
| DDoS protection | Configure proxy routes, disable chain API/RPC/GRPC | Enabled by default |
| Deploy containers | Multi-file docker compose with env sourcing and sudo | Single command with health monitoring |
| Check node status | Query 5+ API endpoints, parse JSON | `gonka-nop status` (unified dashboard) |
| Update MLNode | 6-step manual process (disable, pull, recreate, wait, enable) | `gonka-nop update` |
| Fix stuck node | Search GitHub releases, download binaries, place in cosmovisor dirs | `gonka-nop repair` |
| Multi-server ML node | Clone repo, edit compose, download model, register via curl | `gonka-nop setup --type mlnode` + `ml-node add` |

## Security Defaults

- Internal ports (5050, 8080, 9100, 9200) bound to `127.0.0.1` in full mode
- Port 9100 exposed for network-only topology (remote ML nodes need PoC callback access)
- DDoS protection: `GONKA_API_BLOCKED_ROUTES=poc-batches training`
- Chain API/RPC/GRPC disabled by default
- `gpu-memory-utilization` capped at 0.88-0.94 (not 0.99 -- prevents OOM)
- ML node ports bound to server IP, not 0.0.0.0 (prevents public exposure)

## License

MIT License. See [LICENSE](LICENSE).
