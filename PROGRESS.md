# Gonka NOP - Implementation Progress

## Milestone 1: Core CLI Framework

- [x] Project scaffolding (go.mod, main.go, cobra root command)
- [x] UI utilities (colored output, spinner, interactive prompts)
- [x] State management (Save/Load JSON)
- [x] Phase system (Phase interface + runner)
- [x] Setup phases (prerequisites, GPU detection, network select, keys, config, deploy)
- [x] Status command (Overview, Blockchain, MLNode sections)
- [x] gpu-info command

## Milestone 2: Test Coverage

- [x] Config package tests (state_test.go — 96% coverage)
- [x] GPU detection tests (recommendConfig, FormatGPUSummary)
- [x] Config generation tests
- [x] GPU parser tests (nvidia-smi, docker version, disk space parsing)
- [x] Prerequisites install tests (Secure Boot, driver install)
- [x] Status tests (reconciliation, setup summary)
- [x] Prompt override tests (non-interactive mode)
- [x] Version parsing tests
- [x] Update command tests
- [x] Repair command tests
- [x] Download model tests
- [ ] Phase runner tests
- [ ] Display formatting tests
- [ ] End-to-end integration tests

## Milestone 3: GPU & Prerequisites

- [x] Real nvidia-smi parsing (index, name, memory, driver, PCI bus)
- [x] GPU architecture detection (sm_80 through sm_120)
- [x] NVLink/PCIe topology detection
- [x] Auto-select MLNode image tag (standard vs blackwell)
- [x] Docker availability and version check
- [x] NVIDIA Container Toolkit detection
- [x] CUDA verification inside Docker container
- [x] Docker Compose v2 check
- [x] NVIDIA driver state detection (not installed / installed / mismatch)
- [x] Auto-install NVIDIA driver with user confirmation
- [x] Fabric Manager install for NVLink setups
- [x] 3-way driver version consistency check
- [x] Detect unattended-upgrades and warn
- [x] Linux distro detection
- [x] Disk space pre-check (250GB minimum)
- [x] NVIDIA Container Toolkit auto-installation
- [x] Docker runtime configuration (nvidia-ctk)
- [ ] Secure Boot pre-check
- [ ] Kernel headers availability check
- [ ] CUDA version compatibility verification
- [ ] Reboot requirement warning
- [ ] Port availability check

## Milestone 4: Configuration

- [x] Model-aware TP/PP calculation (Qwen3-235B, Qwen3-32B, QwQ-32B)
- [x] gpu-memory-utilization recommendation (0.88-0.94)
- [x] max-model-len calculation based on VRAM headroom
- [x] Auto-add kv-cache-dtype fp8 for tight VRAM
- [x] node-config.json generation (template-based)
- [x] Host field validation (no http:// prefix)
- [x] Hardware field auto-populated from GPU detection
- [x] config.env generation with all environment variables
- [x] MODEL_NAME environment variable
- [x] VLLM_ATTENTION_BACKEND = FLASHINFER for all architectures
- [x] docker-compose.yml generation with port security (127.0.0.1 binding)
- [x] MLNode image tag selection based on GPU architecture
- [x] DDoS protection defaults (blocked routes, disabled chain API/RPC/GRPC)
- [x] Persistent peers auto-configuration
- [x] Pruning defaults (custom, keep-recent=1000, interval=100)
- [x] State sync enabled by default

## Milestone 5: Deployment

- [x] Multi-compose file handling
- [x] Automatic config.env loading + sudo -E
- [x] Image pull with progress
- [x] Ordered container startup (network first, then ML)
- [x] Quick key workflow (all keys on server)
- [x] Secure key workflow (accept account pubkey)
- [x] grant-ml-ops-permissions automation
- [x] HuggingFace model download with resume support
- [x] Standalone download-model command
- [x] Container health verification (setup/report polling)
- [x] Blockchain sync monitoring (block height progress, 30min timeout)
- [x] Wait for sync before registration
- [ ] DOCKER-USER iptables chain configuration
- [ ] iptables-persistent integration
- [ ] IPv4/IPv6 resolution check
- [ ] SHA256 model verification
- [ ] PUBLIC_URL reachability check

## Milestone 6: Operations

- [x] Status: block height, sync status, blocks behind
- [x] Status: epoch participation, weight, miss rate
- [x] Status: PoC weight, timeslot allocation, inference count
- [x] Status: MLNode model, GPU utilization, PoC status
- [x] Status: node config (public URL, API version, upgrade plan)
- [x] Status: security checks (cold key, warm key, ML permissions)
- [x] Status: validator_in_set reconciliation (pagination fix)
- [x] Update: safe MLNode rollout (disable, pull, recreate, wait, enable)
- [x] Update: version comparison table
- [x] Update: --check and --service flags
- [x] Update: distinguish Cosmovisor auto-update vs manual
- [x] ml-node list (status, allocation, model, hardware)
- [x] ml-node enable/disable
- [x] ml-node status (detailed per-node view)
- [ ] ml-node add (POST /admin/v1/nodes)
- [ ] ml-node update (PUT /admin/v1/nodes/:id)
- [ ] Reset command (blockchain data cleanup preserving keys)
- [ ] Cleanup command (disk space recovery)
- [ ] Model switching via Admin API
- [ ] PUBLIC_URL reachability check
- [ ] Miss rate timeline visualization

## Milestone 7: Registration & On-chain

- [x] submit-new-participant with correct flags
- [x] Auto-fetch consensus_pubkey from setup report
- [x] grant-ml-ops-permissions automation
- [x] Standalone register command with --force
- [x] Testnet registration (gas-free)
- [x] Secure workflow manual instructions
- [ ] PoC endpoint verification
- [ ] Reward claiming (claim-rewards command)
- [ ] Force-claim recovery for missed epochs
- [ ] Governance voting

## Milestone 8: Advanced & Polish

- [x] Centralized monitoring (Ansible: exporter, Prometheus, Grafana, Alertmanager)
- [x] 2 Grafana dashboards (Fleet Overview, Node Deep Dive)
- [x] 11 alert rules (miss rate, block lag, GPU, node status)
- [x] Client-deploy playbook for external validators
- [ ] Multi-node batch management
- [ ] Cloud provider compatibility (Vast.ai, GCore)
- [ ] Self-update mechanism for gonka-nop binary
- [ ] Performance benchmarking integration

## Milestone 9: Version Management

- [x] Dynamic image version fetching from GitHub
- [x] Per-service version parsing (handles comments, digest pins)
- [x] Fallback to hardcoded versions
- [x] Safe version update (gonka-nop update)
- [x] Repair stuck nodes (upgrade-info.json parsing, binary download)
- [x] Cosmovisor symlink management
- [ ] Governance proposal pre-seeding

## Milestone 10: Multi-MLNode Support (Planned)

- [ ] ml-node add/update/delete commands
- [ ] Setup type flag (--type full|network|mlnode)
- [ ] MLNode-only setup phases
- [ ] PoC callback URL handling for multi-server setups

## Non-Interactive Mode

- [x] --yes/-y flag for all prompts
- [x] --network, --key-workflow, --key-name, --keyring-password flags
- [x] --public-ip, --hf-home flags
- [x] NAT port split (--ext-p2p-port, --ext-api-port, --int-p2p-port, --int-api-port)
- [x] Prompt override mechanism (substring matching)
