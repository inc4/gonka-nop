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

## What It Does

Gonka NOP automates the entire lifecycle of a Gonka validator node:

**Setup (6 phases):**
1. **Prerequisites** -- Detects and installs Docker, NVIDIA drivers, Container Toolkit, Fabric Manager
2. **GPU Detection** -- Identifies GPUs, architecture (sm_80-sm_120), NVLink topology, recommends TP/PP
3. **Network Select** -- Mainnet or testnet, fetches latest image versions from GitHub
4. **Key Management** -- Quick (all on server) or Secure (cold key on separate machine) workflow
5. **Config Generation** -- docker-compose.yml, node-config.json, config.env with security defaults
6. **Deploy** -- Container orchestration, blockchain sync monitoring, model download, health checks

**Day-2 Operations:**
- Real-time status with epoch participation, PoC weight, miss rate, block lag
- Safe update rollout (disable ML node, pull, recreate, wait for model load, re-enable)
- Repair stuck nodes (detect missing upgrade handler, download correct binaries)
- ML node management (list, status, enable, disable)

## Commands

| Command | Description |
|---------|-------------|
| `setup` | Interactive setup wizard (full node deployment) |
| `status` | Node health: blockchain, epoch, MLNode, security checks |
| `gpu-info` | Detected GPUs with TP/PP/model recommendation |
| `update` | Safe rolling update (`--check` for dry run, `--service` for specific) |
| `repair` | Fix stuck nodes (missing upgrade binaries) |
| `register` | On-chain registration and ML permissions |
| `ml-node` | ML node management (list, status, enable, disable) |
| `download-model` | Pre-download model weights before setup |
| `reset` | Stop containers and clean up |
| `cleanup` | Recover disk space |
| `version` | Print version info |

## Non-Interactive Mode

For automated deployments (CI/CD, batch provisioning):

```bash
gonka-nop setup --yes \
  --network mainnet \
  --key-workflow quick \
  --key-name my-key \
  --keyring-password "pass" \
  --public-ip 1.2.3.4 \
  --hf-home /data/hf
```

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

## Security Defaults

- Internal ports (5050, 8080, 9100, 9200) bound to `127.0.0.1`
- DDoS protection: `GONKA_API_BLOCKED_ROUTES=poc-batches training`
- Chain API/RPC/GRPC disabled by default
- `gpu-memory-utilization` capped at 0.88-0.94 (not 0.99 -- prevents OOM)

## License

MIT License. See [LICENSE](LICENSE).
