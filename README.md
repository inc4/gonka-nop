# Gonka NOP

CLI tool that simplifies Gonka node deployment. One command to detect GPUs, configure runtime, and deploy.

## Install

```bash
curl -fsSL https://github.com/gonka-ai/gonka-nop/releases/latest/download/gonka-nop -o /usr/local/bin/gonka-nop
chmod +x /usr/local/bin/gonka-nop
```

## Usage

```bash
gonka-nop setup      # Interactive setup wizard
gonka-nop status     # Check node health
gonka-nop gpu-info   # Show GPU configuration
```

## Old vs New Approach

| Task | Manual Approach | With gonka-nop |
|------|-----------------|----------------|
| Check prerequisites | Run `docker --version`, `nvidia-smi`, verify toolkit | `gonka-nop setup` (auto-detected) |
| Detect GPUs | Parse `nvidia-smi` output manually | Auto-detected with TP/PP recommendation |
| Select node-config.json | Choose from 6+ config variants | Auto-generated based on GPU count/VRAM |
| Fill config.env | Edit 15+ variables manually | Interactive prompts with validation |
| Configure NVIDIA runtime | Add repo, install toolkit, edit daemon.json, restart Docker | One confirmation prompt |
| Generate keys | Run `inferenced keys add` multiple times | Guided workflow (quick/secure) |
| Start containers | `docker compose up -d` for each compose file | Single command with health checks |
| Check status | Query multiple endpoints manually | `gonka-nop status` (unified view) |
| Debug issues | Check logs, ports, configs separately | Guided diagnostics |

## Commands

| Command | Description |
|---------|-------------|
| `setup` | Run interactive setup wizard |
| `status` | Show node health (blockchain, MLNode, containers) |
| `gpu-info` | Display detected GPUs and recommended config |
| `reset` | Stop containers and clean up |
| `version` | Print version info |
