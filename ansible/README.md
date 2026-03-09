# Gonka Monitoring

Optional opt-in monitoring for Gonka validators. Deploys a lightweight exporter on your node that pushes metrics to a centralized Prometheus + Grafana server.

**What you get:** real-time dashboards for block height, sync status, miss rate, PoC weight, GPU utilization, epoch earnings — with alerts for critical issues.

**What it costs:** two small containers (~50 MB RAM) on your server. No ports opened, no inbound connections. Metrics are pushed outbound only.

---

## For Validators (client setup)

This section is for **node operators** who want to push metrics to an existing central monitoring server.

### Prerequisites

- [Ansible](https://docs.ansible.com/ansible/latest/installation_guide/) installed on your local machine (the machine you SSH from)
- SSH access to your Gonka validator server
- Docker running on your server with Gonka node containers up (`node`, `api`, `mlnode`)

### Step 1: Install Ansible Docker collection

```bash
ansible-galaxy collection install community.docker
```

### Step 2: Clone and configure

```bash
git clone https://github.com/inc4/gonka-nop.git
cd gonka-nop/ansible

cp inventory/client.yml.example inventory/client.yml
```

Edit `inventory/client.yml`:

```yaml
all:
  hosts:
    my-node:
      ansible_host: 1.2.3.4              # Your server IP
      ansible_user: ubuntu                # SSH user
      node_alias: "my-validator"          # Name shown in Grafana dashboards
      deploy_dir: /home/ubuntu/gonka-node # Path to your gonka deploy directory
      participant_address: "gonka1..."    # Your on-chain address (for network metrics)

      # Uncomment if Docker requires sudo on your server:
      # ansible_become: true
```

**Required variables:**

| Variable | Description | Example |
|----------|-------------|---------|
| `ansible_host` | Server IP or hostname | `1.2.3.4` |
| `ansible_user` | SSH username | `ubuntu` |
| `node_alias` | Display name in Grafana (your choice) | `my-validator` |
| `deploy_dir` | Path where Gonka node is deployed (contains `docker-compose.yml`, `config.env`) | `/home/ubuntu/gonka-node` |
| `participant_address` | Your on-chain `gonka1...` address | `gonka1abc...xyz` |

**Optional variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `ansible_become` | `false` | Set to `true` if Docker requires `sudo` |
| `ansible_ssh_private_key_file` | SSH default | Path to SSH private key |

**Finding your participant address:** check your `config.env` for `ACCOUNT_PUBKEY`, then look it up on [gonkahub.com](https://gonkahub.com/network).

### Step 3: Deploy

```bash
ansible-playbook playbooks/client-deploy.yml -i inventory/client.yml
```

This deploys two containers on your server:
- `gonka-exporter` — collects metrics from your node's local APIs
- `gonka-prometheus` — scrapes the exporter every 30s and pushes to the central server

The playbook validates:
- Docker is running
- Deploy directory exists
- Gonka `node` container is running
- Exporter produces metrics
- Central server receives your metrics

### Step 4: View your dashboards

Open **[http://202.78.161.151:3000](http://202.78.161.151:3000)** (Grafana) and select your `node_alias` in the **Node Deep Dive** dashboard.

### Remove monitoring

```bash
ansible-playbook playbooks/client-teardown.yml -i inventory/client.yml
```

Removes only the two monitoring containers and their data. Your Gonka node is not affected.

### Troubleshooting

**Exporter can't reach node APIs:**
- Verify your Gonka `node` container is running: `docker ps | grep node`
- Check the exporter logs: `docker logs gonka-exporter`

**Metrics not appearing in Grafana:**
- Wait 60s after deploy for metrics to propagate
- Test locally: `curl http://localhost:9401/metrics | grep gonka_`
- Check Prometheus is pushing: `docker logs gonka-prometheus 2>&1 | grep remote_write`

**Permission denied:**
- Add `ansible_become: true` to your inventory if Docker requires `sudo`

---

## For Operators (central server setup)

This section is for **infrastructure operators** who run the central Prometheus + Grafana + Alertmanager stack that receives metrics from all validator nodes.

### Prerequisites

- A dedicated server with Docker installed (2 CPU, 4 GB RAM minimum)
- Ansible + `community.docker` collection on your local machine
- Port 9090 (Prometheus), 3000 (Grafana), 9093 (Alertmanager) accessible to validator nodes

### Step 1: Create your operator inventory

Create `inventory/hosts.yml` (gitignored — never commit this file):

```yaml
all:
  children:
    validators:
      hosts:
        node-1:
          ansible_host: 10.0.0.1
          ansible_user: ubuntu
          ansible_become: true
          node_alias: "node-1"
          deploy_dir: /data/gonka-node
          participant_address: "gonka1..."

        node-2:
          ansible_host: 10.0.0.2
          ansible_user: ubuntu
          node_alias: "node-2"
          deploy_dir: /home/ubuntu/gonka-node
          participant_address: "gonka1..."

    # The server that runs Prometheus + Grafana + Alertmanager
    monitoring_servers:
      hosts:
        node-2:   # Can be one of the validators or a separate server
```

**Per-host variables (required):**

| Variable | Description |
|----------|-------------|
| `ansible_host` | Server IP |
| `ansible_user` | SSH user |
| `node_alias` | Unique name per node (shown in Grafana) |
| `deploy_dir` | Gonka deploy directory path |
| `participant_address` | On-chain `gonka1...` address |

**Per-host variables (optional):**

| Variable | Default | Description |
|----------|---------|-------------|
| `ansible_become` | `false` | `true` if Docker needs sudo |
| `deploy_dir` | `/data/gonka-node` | Override per host if paths differ |

### Step 2: Configure shared variables

Edit `inventory/group_vars/all.yml` to customize defaults:

```yaml
# Monitoring data lives under deploy_dir/monitoring
monitoring_dir: "{{ deploy_dir | default('/data/gonka-node') }}/monitoring"

# Exporter settings
exporter_port: 9401
exporter_refresh_interval: 30     # Seconds between metric collections
export_network_metrics: "true"    # Fetch network-wide participant data

# Prometheus
prometheus_image: "prom/prometheus:v3.2.1"
prometheus_port: 9090
prometheus_retention: "30d"       # How long to keep metrics
prometheus_scrape_interval: "30s"

# Grafana
grafana_image: "grafana/grafana:11.5.2"
grafana_port: 3000
grafana_admin_password: "change-me-to-something-secure"

# Alertmanager
alertmanager_image: "prom/alertmanager:v0.28.1"
alertmanager_port: 9093
```

**All configurable variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `monitoring_dir` | `{{ deploy_dir }}/monitoring` | Where monitoring configs and data are stored |
| `exporter_port` | `9401` | Port for the metrics exporter |
| `exporter_refresh_interval` | `30` | Seconds between metric scrapes |
| `export_network_metrics` | `"true"` | Collect network-wide participant data (set `"false"` for client-only nodes to avoid duplicates) |
| `enable_node_fetch` | `"true"` | Access Tendermint RPC via `docker exec` |
| `prometheus_image` | `prom/prometheus:v3.2.1` | Prometheus container image |
| `prometheus_port` | `9090` | Prometheus port |
| `prometheus_retention` | `"30d"` | Metric retention period |
| `prometheus_scrape_interval` | `"30s"` | How often Prometheus scrapes the exporter |
| `grafana_image` | `grafana/grafana:11.5.2` | Grafana container image |
| `grafana_port` | `3000` | Grafana port |
| `grafana_admin_password` | `gonka-monitoring-change-me` | Grafana admin password |
| `alertmanager_image` | `prom/alertmanager:v0.28.1` | Alertmanager container image |
| `alertmanager_port` | `9093` | Alertmanager port |
| `remote_write_url` | `""` | Prometheus remote_write target (auto-set by playbooks) |
| `remote_write_user` | `""` | Basic auth username for remote_write |
| `remote_write_password` | `""` | Basic auth password for remote_write |

### Step 3: Configure alert notifications (optional)

Add notification channel variables to `inventory/group_vars/all.yml`:

**Telegram:**
```yaml
telegram_bot_token: "123456:ABC-DEF..."
telegram_chat_id: "-1001234567890"
```

**Discord:**
```yaml
discord_webhook_url: "https://discord.com/api/webhooks/..."
```

**Slack:**
```yaml
slack_webhook_url: "https://hooks.slack.com/services/..."
```

If no notification channels are configured, alerts are still visible in the Alertmanager UI at `http://<server>:9093`.

### Step 4: Deploy the central stack

**Option A: Full stack on one server** (central monitoring + exporter for that node):

```bash
ansible-playbook playbooks/deploy-all.yml -l monitoring_servers
```

This deploys: exporter + Prometheus (with `--web.enable-remote-write-receiver`) + Alertmanager + Grafana.

**Option B: Central server only** (no exporter — server doesn't run a Gonka node):

```bash
ansible-playbook playbooks/deploy-central.yml -l monitoring_servers
```

This deploys: Prometheus + Alertmanager + Grafana only.

After deploy, access:
- **Grafana:** `http://<server>:3000` (admin / your password)
- **Prometheus:** `http://<server>:9090`
- **Alertmanager:** `http://<server>:9093`

### Step 5: Add validator nodes

For each additional validator node that should push metrics to central:

```bash
ansible-playbook playbooks/add-node.yml -l <node-name>
```

This deploys exporter + Prometheus-agent on the target node, configured to `remote_write` to the central server. The central Prometheus URL is auto-resolved from the `monitoring_servers` group.

### Additional playbooks

| Playbook | Command | Purpose |
|----------|---------|---------|
| `deploy-all.yml` | `ansible-playbook playbooks/deploy-all.yml -l monitoring_servers` | Full stack on central server |
| `deploy-central.yml` | `ansible-playbook playbooks/deploy-central.yml -l monitoring_servers` | Central only (no exporter) |
| `deploy-exporter.yml` | `ansible-playbook playbooks/deploy-exporter.yml -l <node>` | Exporter only (no Prometheus) |
| `add-node.yml` | `ansible-playbook playbooks/add-node.yml -l <node>` | Exporter + push to central |
| `teardown.yml` | `ansible-playbook playbooks/teardown.yml -l <node>` | Remove everything from a host |
| `client-deploy.yml` | `ansible-playbook playbooks/client-deploy.yml -i inventory/client.yml` | External validator self-service |
| `client-teardown.yml` | `ansible-playbook playbooks/client-teardown.yml -i inventory/client.yml` | External validator removal |

---

## Architecture

```
Validator Server (2 containers added)      Central Server (operator)
┌─────────────────────────────┐           ┌──────────────────────────────┐
│                             │           │                              │
│  Gonka Node (existing)      │           │  Prometheus (storage)        │
│  ┌─────┐ ┌────┐ ┌───────┐  │           │  ┌─────────────────────────┐ │
│  │node │ │api │ │mlnode  │  │           │  │ Receives remote_write   │ │
│  │26657│ │9200│ │5050    │  │           │  │ from all validator nodes│ │
│  └──┬──┘ └─┬──┘ └───┬───┘  │           │  └─────────────────────────┘ │
│     │      │        │       │           │                              │
│  ┌──▼──────▼────────▼────┐  │           │  Grafana (dashboards)        │
│  │ gonka-exporter (:9401)│  │           │  ┌─────────────────────────┐ │
│  │ scrapes local APIs    │  │           │  │ Fleet Overview          │ │
│  └──────────┬────────────┘  │   push    │  │ Node Deep Dive         │ │
│             │               │  ──────►  │  └─────────────────────────┘ │
│  ┌──────────▼────────────┐  │   HTTP    │                              │
│  │ prometheus (:9090)    │  │  outbound │  Alertmanager                │
│  │ remote_write to       │  │   only    │  ┌─────────────────────────┐ │
│  │ central server        │  │           │  │ Telegram/Discord alerts │ │
│  └───────────────────────┘  │           │  └─────────────────────────┘ │
│                             │           │                              │
└─────────────────────────────┘           └──────────────────────────────┘
```

**No inbound ports opened on validator nodes.** Prometheus pushes metrics outbound via HTTP to the central server.

---

## Metrics Collected

| Metric | Source | Description |
|--------|--------|-------------|
| `gonka_block_height` | Tendermint RPC | Current block height |
| `gonka_chain_catching_up` | Tendermint RPC | 1 if syncing, 0 if synced |
| `gonka_node_status` | Admin API | INFERENCE/POC/STOPPED/FAILED |
| `gonka_node_poc_weight` | Admin API | PoC weight per model |
| `gonka_node_gpu_avg_utilization_percent` | GPU API | Average GPU utilization |
| `gonka_node_gpu_device_count` | GPU API | Number of GPUs |
| `gonka_participant_inference_count` | Network API | Inferences this epoch |
| `gonka_participant_missed_requests` | Network API | Missed requests this epoch |
| `gonka_participant_earned_coins` | Network API | Earnings this epoch |
| `gonka_participant_epochs_completed` | Network API | Total completed epochs |
| `gonka_participant_coin_balance` | Network API | Current balance |
| `gonka_node_poc_timeslot_assigned` | Admin API | PoC timeslot assignment |

## Alert Rules (on central server)

| Alert | Severity | Trigger |
|-------|----------|---------|
| GonkaMissRateHigh | critical | Miss rate > 20% for 5m |
| GonkaNodeFailed | critical | ML node FAILED for 2m |
| GonkaNodeStopped | critical | ML node STOPPED for 5m |
| GonkaBlockLag | warning | > 50 blocks behind for 5m |
| GonkaBlockLagCritical | critical | > 200 blocks behind for 3m |
| GonkaCatchingUp | warning | Node syncing for 10m |
| GonkaGPUUtilizationHigh | warning | GPU > 95% for 10m |
| GonkaZeroWeight | warning | Zero PoC weight for 30m |
| GonkaZeroInferences | warning | No inferences for 30m |
| GonkaStatusMismatch | warning | Intended != current status for 5m |
| GonkaExporterDown | critical | Exporter unreachable for 5m |

## Grafana Dashboards

Two dashboards are auto-provisioned:

**Fleet Overview** — multi-node view:
- Node status, block lag, miss rate, PoC weight per node
- GPU utilization and device count
- Epoch earnings and inference counts
- Top 20 participants by weight

**Node Deep Dive** — single node detail (select via `$instance` dropdown):
- Block height, sync status, node status
- Miss rate and inference count over time
- GPU utilization graph
- PoC weight and epoch history
- Validator set membership

## Security

- **No inbound ports** — metrics are pushed outbound, no new ports opened on your server
- **No node access** — the exporter only reads from local APIs (same as `curl localhost:9200`)
- **No credentials sent** — only performance metrics (block height, GPU stats, miss rate). No keys, passwords, or IP addresses
- **Your node alias** — you choose what name appears in Grafana. Use a pseudonym if you prefer
- **Opt-out anytime** — run the teardown playbook to remove everything. Your Gonka node is not affected
- **Inventory gitignored** — `inventory/hosts.yml` and `inventory/client.yml` are in `.gitignore` to prevent accidental credential commits
