# Gonka NOP

[![CI](https://github.com/inc4/gonka-nop/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/inc4/gonka-nop/actions/workflows/ci.yml)
[![Security](https://github.com/inc4/gonka-nop/actions/workflows/security.yml/badge.svg)](https://github.com/inc4/gonka-nop/actions/workflows/security.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/inc4/gonka-nop)](https://goreportcard.com/report/github.com/inc4/gonka-nop)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

One-command CLI for deploying GPU-accelerated Gonka AI inference nodes. Auto-detects GPUs, configures NVIDIA runtime, generates optimized configs, and deploys containers with security hardening.

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
