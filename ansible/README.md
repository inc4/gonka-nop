# Gonka Monitoring

Optional opt-in monitoring for Gonka validators. Deploys a lightweight exporter on your node that pushes metrics to a centralized Prometheus + Grafana server.

**What you get:** real-time dashboards for block height, sync status, miss rate, PoC weight, GPU utilization, epoch earnings — with alerts for critical issues.

**What it costs:** three small containers on your server. No ports opened, no inbound connections. Metrics are pushed outbound only.

## For Validators (client setup)

### Prerequisites

- [Ansible](https://docs.ansible.com/ansible/latest/installation_guide/) installed on your local machine
- SSH access to your Gonka validator server
- Docker running on your server with Gonka node containers up

### 1. Install Ansible Docker collection

```bash
ansible-galaxy collection install community.docker
```

### 2. Clone this repo and configure your node

```bash
git clone https://github.com/inc4/gonka-nop.git
cd gonka-nop/ansible

cp inventory/client.yml.example inventory/client.yml
```

Edit `inventory/client.yml` with your server details:

```yaml
all:
  hosts:
    my-node:
      ansible_host: 1.2.3.4              # Your server IP
      ansible_user: ubuntu                # SSH user
      node_alias: "my-validator"          # Name shown in Grafana
      deploy_dir: /home/ubuntu/gonka-node # Your gonka deploy directory
      participant_address: "gonka1..."    # Your on-chain address

      # Uncomment if Docker requires sudo on your server:
      # ansible_become: true
```

**Finding your participant address:** check your `config.env` for `ACCOUNT_PUBKEY`, then look it up on [gonkahub.com](https://gonkahub.com/network).

### 3. Deploy

```bash
ansible-playbook playbooks/client-deploy.yml -i inventory/client.yml
```

This deploys three containers on your server:
- `gonka-exporter` — Go-based metrics collector ([inc4/gonka-exporter-go](https://github.com/inc4/gonka-exporter-go)), joins your Gonka node's Docker network to collect metrics from local APIs
- `node-exporter` — Prometheus node-exporter for system metrics (CPU, memory, disk, network)
- `gonka-prometheus` — scrapes both exporters every 30s and pushes to the central server

### 4. View your dashboards

Open **[http://202.78.161.151:3000](http://202.78.161.151:3000)** (Grafana) and select your node alias in the Node Deep Dive dashboard.

### Remove monitoring

```bash
ansible-playbook playbooks/client-teardown.yml -i inventory/client.yml
```

This removes only the monitoring containers. Your Gonka node is not affected.

---

## Architecture

```
Your Server (3 containers added)        Central Server (operated by inc4)
┌─────────────────────────────┐         ┌──────────────────────────────┐
│                             │         │                              │
│  Gonka Node (existing)      │         │  Prometheus (storage)        │
│  ┌─────┐ ┌────┐ ┌───────┐  │         │  ┌─────────────────────────┐ │
│  │node │ │api │ │mlnode  │  │         │  │ Receives remote_write   │ │
│  │26657│ │9200│ │5050    │  │         │  │ from all validator nodes│ │
│  └──┬──┘ └─┬──┘ └───┬───┘  │         │  └─────────────────────────┘ │
│     │      │        │       │         │                              │
│  ┌──▼──────▼────────▼────┐  │         │  Grafana (dashboards)        │
│  │ gonka-exporter (:9404)│  │         │  ┌─────────────────────────┐ │
│  │ Docker network bridge │  │         │  │ Fleet Overview          │ │
│  └──────────┬────────────┘  │         │  │ Node Deep Dive         │ │
│             │               │  push   │  └─────────────────────────┘ │
│  ┌──────────┴────────────┐  │ ──────► │                              │
│  │ node-exporter (:9101) │  │         │  Alertmanager                │
│  │ host network (system) │  │         │  ┌─────────────────────────┐ │
│  └──────────┬────────────┘  │  HTTP   │  │ Telegram/Discord alerts │ │
│             │               │ outbound│  └─────────────────────────┘ │
│  ┌──────────▼────────────┐  │  only   │                              │
│  │ prometheus (:9090)    │  │         │                              │
│  │ remote_write to       │  │         │                              │
│  │ central server        │  │         │                              │
│  └───────────────────────┘  │         │                              │
│                             │         │                              │
└─────────────────────────────┘         └──────────────────────────────┘
```

**No inbound ports opened.** Prometheus pushes metrics outbound via HTTP to the central server.

## Metrics Collected

### Gonka Exporter (from [inc4/gonka-exporter-go](https://github.com/inc4/gonka-exporter-go))

| Metric | Source | Description |
|--------|--------|-------------|
| `gonka_block_height` | Tendermint RPC | Current block height |
| `gonka_block_height_max` | Public nodes | Max height across network |
| `gonka_chain_catching_up` | Tendermint RPC | 1 if syncing, 0 if synced |
| `gonka_chain_epoch` | Network API | Current epoch number |
| `gonka_node_status` | Admin API | INFERENCE/POC/STOPPED/FAILED |
| `gonka_node_poc_weight` | Admin API | PoC weight per model |
| `gonka_node_gpu_avg_utilization_percent` | GPU API | Average GPU utilization |
| `gonka_node_gpu_device_count` | GPU API | Number of GPUs |
| `gonka_participant_inference_count` | Network API | Inferences this epoch |
| `gonka_participant_missed_requests` | Network API | Missed requests this epoch |
| `gonka_participant_earned_coins` | Network API | Earnings this epoch |
| `gonka_participant_coin_balance` | Network API | Current balance |
| `gonka_epoch_*` | Persistent | Historical per-epoch metrics with epoch history |
| `gonka_network_*` | Network API | Network-wide participant weights |
| `gonka_model_*` | Network API | Model VRAM, throughput, thresholds |

### Node Exporter (system metrics)

| Metric | Description |
|--------|-------------|
| `node_cpu_*` | CPU usage |
| `node_memory_*` | Memory usage |
| `node_disk_*` | Disk I/O stats |
| `node_filesystem_*` | Filesystem usage |
| `node_load*` | System load averages |
| `node_network_*` | Network interface stats |

## Alert Rules (on central server)

| Alert | Severity | Trigger |
|-------|----------|---------|
| GonkaMissRateHigh | critical | Miss rate > 20% for 5m |
| GonkaNodeFailed | critical | ML node FAILED for 2m |
| GonkaBlockLag | warning | > 50 blocks behind for 5m |
| GonkaBlockLagCritical | critical | > 200 blocks behind for 3m |
| GonkaGPUUtilizationHigh | warning | GPU > 95% for 10m |
| GonkaZeroWeight | warning | Zero PoC weight for 30m |
| GonkaExporterDown | critical | Exporter unreachable for 5m |

## Security

- **No inbound ports** — metrics are pushed outbound, no new ports opened on your server
- **No node access** — the exporter only reads from local APIs (same as `curl localhost:9200`)
- **No credentials sent** — only performance metrics (block height, GPU stats, miss rate). No keys, passwords, or IP addresses
- **Your node alias** — you choose what name appears in Grafana. Use a pseudonym if you prefer
- **Opt-out anytime** — run `client-teardown.yml` to remove everything. Your node is not affected

## For Operators (central server)

To deploy the full central monitoring stack (Prometheus + Grafana + Alertmanager):

```bash
ansible-playbook playbooks/deploy-all.yml -l monitoring_servers
```

To add internal nodes with metric export:

```bash
ansible-playbook playbooks/add-node.yml -l my-node
```

See `playbooks/` for all available playbooks:

| Playbook | Purpose |
|----------|---------|
| `client-deploy.yml` | Validator: deploy exporter + push to central |
| `client-teardown.yml` | Validator: remove monitoring |
| `deploy-all.yml` | Operator: full stack on one server |
| `deploy-central.yml` | Operator: central server only |
| `deploy-exporter.yml` | Operator: exporter only on validator |
| `add-node.yml` | Operator: add internal node |
| `teardown.yml` | Operator: remove everything |
