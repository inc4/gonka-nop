# Gonka NOP — End-to-End Operator Guide

This guide walks an operator through a full deploy using `gonka-nop` across the
three supported topologies, plus the post-deploy actions every operator must
perform after the chain's collateral grace period ends (epoch ≥ 181).

It is the canonical companion to the [README](../README.md), which lists
commands and flags. This guide is task-oriented: pick the topology that
matches your setup and follow the section start to finish.

- [Topologies at a glance](#topologies-at-a-glance)
- [Prerequisites](#prerequisites)
- [Picking a model](#picking-a-model)
- [Topology 1 — Full (single server)](#topology-1--full-single-server)
- [Topology 2 — Multi-server (1 validator + N ML slaves)](#topology-2--multi-server-1-validator--n-ml-slaves)
- [Topology 3 — Network-only validator (no local GPU)](#topology-3--network-only-validator-no-local-gpu)
- [Post-deploy: registration, collateral, governance opt-in](#post-deploy-registration-collateral-governance-opt-in)
- [Day-2 operations](#day-2-operations)
- [Recovery](#recovery)

---

## Topologies at a glance

| Topology | When to use | What runs locally |
|---|---|---|
| **Full** | One server has both 16-core CPU + 40GB+ VRAM GPUs and you want a self-contained validator | tmkms, node, api, proxy, bridge, versiond + mlnode + nginx |
| **Multi-server** | You have a dedicated chain server and one or more dedicated GPU servers | Validator services on one host; mlnode + nginx on each GPU host |
| **Network-only** | Your only host is CPU-only and ML will live elsewhere | Validator services only; ML nodes attach later from remote hosts |

All three start with `gonka-nop setup` — only `--type` and a few flags differ.

---

## Prerequisites

Run these checks before any setup. NOP refuses to start if Docker or the NVIDIA
runtime is missing on a host that needs them.

```bash
# All hosts
docker --version                       # 24+
docker compose version                 # v2 plugin

# GPU hosts (full + mlnode)
nvidia-smi                             # at least 40GB total VRAM
docker run --rm --gpus all nvidia/cuda:12.6.0-base-ubuntu22.04 nvidia-smi
```

If `nvidia-ctk` is missing, NOP will offer to install the NVIDIA Container
Toolkit during setup (one confirmation prompt).

Disk: 250+ GB free on the chain server (Cosmovisor keeps upgrade backups).
HuggingFace cache (`HF_HOME`) needs ~220 GB for Qwen3-235B FP8.

---

## Picking a model

NOP auto-recommends a model from the chain's governance-approved list
based on the detected hardware, and you can override with
`--model <id>`. NOP rejects mismatched architectures before writing any
config — no half-deployed state.

| Model ID | Min VRAM/GPU | Min Total VRAM | GPU arch | Notes |
|---|---|---|---|---|
| `Qwen/Qwen3-235B-A22B-Instruct-2507-FP8` | 40 GB | 320 GB | any (A100+, H100, H200, Blackwell) | Default for most fleets; ~220 GB HF cache |
| `Qwen/Qwen3-32B-FP8` | 20 GB | 40 GB | any | Smaller model, lower VRAM bar |
| `Qwen/QwQ-32B` | 20 GB | 80 GB | any | Reasoning variant |
| `moonshotai/Kimi-K2.6` | 80 GB | 320 GB | **Blackwell only** (sm_100, sm_103, sm_120) | Refuses to deploy on Ampere/Hopper — NOP will hard-error |

**Hardware ↔ model decision matrix**

| Your hardware | Recommended model |
|---|---|
| 8×A100 80GB | Qwen3-235B-A22B-Instruct-2507-FP8 |
| 8×H100 / H200 80GB | Qwen3-235B-A22B-Instruct-2507-FP8 |
| 4–8× B200 / B300 | Kimi-K2.6 (best weight); Qwen3-235B also works |
| 2× B200 / 4× consumer 24GB | Qwen3-32B-FP8 or QwQ-32B |

For models you **cannot** run (e.g. Kimi on A100), see
[Per-model governance opt-in](#per-model-governance-opt-in) — delegating
to a Blackwell host that runs Kimi costs ~5% weight vs ~10% for refuse,
~15% for doing nothing.

---

## Topology 1 — Full (single server)

Single host runs everything. Simplest path.

```bash
# Interactive
gonka-nop setup -o /GONKA/deploy

# Non-interactive
gonka-nop setup -y -o /GONKA/deploy \
  --type full \
  --network mainnet \
  --key-workflow quick \
  --key-name gonka-node \
  --keyring-password '<your-strong-password>' \
  --public-ip <your-public-ip> \
  --hf-home /GONKA/hf \
  --ports default \
  --model Qwen/Qwen3-235B-A22B-Instruct-2507-FP8
```

**What happens (7 phases)**

1. Prerequisites check (Docker, nvidia-ctk, distro, kernel headers)
2. GPU detection + TP/PP/model recommendation
3. NVIDIA Container Toolkit install (if missing, prompts)
4. Key management (cold + warm keys generated on this host; mnemonics printed
   once — **save them offline immediately**)
5. Config generation (`docker-compose.yml`, `docker-compose.mlnode.yml`,
   `node-config.json`, `config.env`, `nginx.conf`, `devshards/{bin,data}/`)
6. Deployment (`docker compose pull && up -d`, syncs chain in background)
7. Registration phase placeholder — actual on-chain registration happens
   after sync completes (next section)

**Verify**

```bash
gonka-nop status -o /GONKA/deploy
```

Health should show `12/13 PASS` once registered. The remaining FAIL is
`active_in_epoch` (clears after the first PoC cycle in the next epoch).

---

## Topology 2 — Multi-server (1 validator + N ML slaves)

Pattern: one **validator host** (CPU is fine; GPUs optional) runs the chain
+ api + proxy; one or more **GPU hosts** run only mlnode + nginx, attached
to the validator via the Admin API.

### Step 2.1 — Validator host

If the validator host has GPUs you also want to use, set up as Full
(Topology 1). If it is CPU-only, follow Topology 3 first and come back here.

### Step 2.2 — Each GPU host

```bash
gonka-nop setup -y -o /data/gonka-node \
  --type mlnode \
  --network mainnet \
  --network-node-url http://<validator-private-ip>:9200 \
  --public-ip <gpu-host-public-ip> \
  --hf-home /data/hf \
  --ports default \
  --model Qwen/Qwen3-235B-A22B-Instruct-2507-FP8
```

This generates only `docker-compose.mlnode.yml` + `nginx.conf` +
`mlnode-registration.json` (used in the next step). It does NOT generate
keys — keys live on the validator. It does NOT register on chain — that
also happens from the validator side.

### Step 2.3 — Register the GPU host with the validator

Copy the registration JSON from the GPU host to the validator host:

```bash
# On the GPU host
sudo cat /data/gonka-node/mlnode-registration.json
```

Save the output as `/tmp/mlnode-N.json` on the **validator host**.

> **⚠ ID collision warning.** The generated JSON defaults to `"id": "node1"`.
> If the validator already has a `node1` (e.g. it runs Full mode locally),
> edit the JSON and bump to `"id": "node2"` (or any unique ID) before adding,
> otherwise `ml-node add` will silently overwrite the existing entry.

Then on the validator host:

```bash
gonka-nop ml-node add --config /tmp/mlnode-N.json
gonka-nop ml-node list
```

A common race: when a fresh mlnode is added while the api is polling, the
api may interrupt vLLM mid-init with a stop signal, causing a restart loop.
Safe sequence to avoid this:

```bash
# On validator host
gonka-nop ml-node disable nodeN          # before/right after add
# Wait ~2 minutes (let vLLM finish loading on the GPU host)
gonka-nop ml-node enable nodeN
```

---

## Topology 3 — Network-only validator (no local GPU)

Use when your only chain server is CPU-only. Mlnode + nginx are NOT
deployed locally; ML capacity will be attached from remote hosts later
(Topology 2 step 2.2).

```bash
gonka-nop setup -y -o /GONKA/deploy \
  --type network \
  --network mainnet \
  --key-workflow quick \
  --key-name gonka-node \
  --keyring-password '<your-strong-password>' \
  --public-ip <your-public-ip> \
  --ports default
```

After sync + registration (next section), each GPU host you provision
follows Topology 2 step 2.2 onward, pointing
`--network-node-url http://<this-host-ip>:9200`.

---

## Post-deploy: registration, collateral, governance opt-in

These steps apply to all topologies and **must** be completed for the node
to earn full PoC weight.

### Wait for chain sync

```bash
gonka-nop status -o /GONKA/deploy
# Sync: ✓ Synced at height N
```

State-sync typically finishes in 5–15 min; block replay from snapshot to
head can take another 30–90 min depending on how stale the published
snapshots are.

### Register on chain

```bash
gonka-nop register -o /GONKA/deploy
```

This signs and broadcasts `register-new-participant` from the cold key,
then `grant-ml-ops-permissions` for the warm key. The cosmetics
sometimes say "Timed out waiting for transaction" even when the tx is
included — re-run `gonka-nop status` and check that
`permissions_granted: All 20 required permissions granted` shows PASS.

### Deposit collateral (epoch ≥ 181 only)

After the chain's grace period (default 180 epochs), participants need
collateral to back the non-base portion of their PoC weight. Without it
you are capped at 20% of earned weight (`base_weight_ratio`).

**Formula** (live chain params):

```
required_collateral = weight × (1 − base_weight_ratio) × collateral_per_weight_unit
                    = weight × 0.8 × 4.2     (ngonka)
```

Deposit (cold key must hold the funds; gas is paid via feegrant from the
warm key):

```bash
GONKA_KEYRING_PASSWORD='<your-password>' \
  gonka-nop collateral deposit 5000000ngonka -o /GONKA/deploy

gonka-nop collateral show -o /GONKA/deploy
# Collateral
#   ✓ 5000000 ngonka (0.005000 GONKA)
```

**Recommended initial size**: ~2× the formula minimum for your expected
weight, to absorb between-epoch weight fluctuations. Deposits are
cumulative — top up later as your weight grows.

**Withdrawal**: 1-epoch unbonding queue; collateral is still slashable
during unbonding.

```bash
gonka-nop collateral withdraw 1000000ngonka -o /GONKA/deploy
```

### Per-model governance opt-in

The chain runs multi-model PoC: each governance-approved model has its
own delegation/refusal/intent state per participant. Operators must
declare their stance for each.

**Run the model locally** (recommended): no on-chain action required.
Your stack submits the PoC v2 store commit automatically — keep running.

**Cannot run the model** (hardware mismatch — e.g. you have A100s and
the model needs Blackwell): delegate to a host that does run it. The
official docs put the penalty hierarchy at:

| Action | Weight loss |
|---|---|
| Run the model | 0% |
| Delegate to a running host | ~5% |
| Refuse explicitly | ~10% |
| Do nothing | ~15% (if a quorum forms without you) |

**Discover candidate delegates:**

```bash
gonka-nop govmodel candidates moonshotai/Kimi-K2.6 --healthy-only --limit 10 -o /GONKA/deploy
```

Output is sorted best-first (more INFERENCE nodes wins, fewer FAILED
wins as tiebreaker, larger fleet wins last). Pick an address you trust.

**Delegate:**

```bash
GONKA_KEYRING_PASSWORD='<your-password>' \
  gonka-nop govmodel delegate moonshotai/Kimi-K2.6 <picked-address> -o /GONKA/deploy
```

**Refuse** (last resort, ~10% loss):

```bash
GONKA_KEYRING_PASSWORD='<your-password>' \
  gonka-nop govmodel refuse moonshotai/Kimi-K2.6 -o /GONKA/deploy
```

**Declare intent** (non-binding signal, no weight effect; bootstrap-only):

```bash
GONKA_KEYRING_PASSWORD='<your-password>' \
  gonka-nop govmodel declare-intent future/Model-v3 -o /GONKA/deploy
```

Verify the state:

```bash
gonka-nop govmodel list -o /GONKA/deploy
```

---

## Day-2 operations

### Status

```bash
gonka-nop status -o /GONKA/deploy
```

Sections: Overview, Sync, Blockchain, Epoch, ML Node, Node Config,
Security, Collateral, Governance Models.

### ML node management

```bash
gonka-nop ml-node list                         # all attached nodes
gonka-nop ml-node status nodeN                 # one node, detailed
gonka-nop ml-node disable nodeN                # safe pause (no PoC)
gonka-nop ml-node enable nodeN                 # resume
gonka-nop ml-node update --id nodeN --model X  # switch model (PoC-gated)
gonka-nop ml-node set-image <full-image-ref>   # safe image swap rollout
```

### Update chain binaries

```bash
gonka-nop update --check                          # show available updates
gonka-nop update                                  # apply (chain handles
                                                  # Cosmovisor swaps; mlnode
                                                  # rolls via disable→pull→
                                                  # recreate→wait→enable)
gonka-nop update --service api --image <ref>      # api-only swap
gonka-nop update --api-only --version <v>         # Cosmovisor api hotfix
```

### Re-grant missing permissions (without re-register)

```bash
gonka-nop repair-permissions -o /GONKA/deploy
```

Useful when a chain upgrade introduces new permission types and your
warm key's grant set is now incomplete.

---

## Recovery

### Reset blockchain data, keep keys + configs

```bash
gonka-nop reset -o /GONKA/deploy
```

### Fix stuck node (missing upgrade binary)

```bash
gonka-nop repair -o /GONKA/deploy
```

NOP parses `upgrade-info.json` (the on-chain plan), downloads the
correct `inferenced` + `decentralized-api` binaries with SHA256
verification, places them in the right Cosmovisor directory, and
restarts the node. Works for both mainnet and testnet (different
release repos per network).

### Clean up disk

```bash
gonka-nop cleanup
```

Removes old Cosmovisor backups and reports freed space.
