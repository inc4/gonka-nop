import os
import time
import json
import requests
import random
import subprocess
from prometheus_client import start_http_server, Gauge
from typing import List, Dict, Any, Tuple, Optional
from datetime import datetime, timezone

# =============================================================================
# CONFIGURATION
# =============================================================================

BASE_URL = os.getenv("GONKA_BASE_URL", "http://localhost:26657").rstrip("/")
NODE_BASE_URL = os.getenv("NODE_BASE_URL", "http://localhost:9200/admin/v1").rstrip("/")
NETWORK_API_URL = "http://localhost:8000"

BLOCK_HEIGHT_NODES = [
    "http://node1.gonka.ai:8000",
    "http://node2.gonka.ai:8000",
    "http://node3.gonka.ai:8000",
    "http://185.216.21.98:8000",
    "http://36.189.234.197:18026",
    "http://36.189.234.237:17241",
    "http://47.236.26.199:8000",
    "http://47.236.19.22:18000",
    "http://gonka.spv.re:8000",
]

EXPORT_NETWORK_METRICS = os.getenv("EXPORT_NETWORK_METRICS", "false").lower() in ("1", "true", "yes")
ENABLE_NODE_FETCH = os.getenv("ENABLE_NODE_FETCH", "true").lower() in ("1", "true", "yes")
PARTICIPANT_ADDRESS = os.getenv("PARTICIPANT_ADDRESS", "").strip()
EXPORTER_PORT = int(os.getenv("EXPORTER_PORT", "9401"))
REFRESH_INTERVAL = int(os.getenv("REFRESH_INTERVAL", "30"))

TENDERMINT_STATUS_ENDPOINT = "/status"
PARTICIPANTS_ENDPOINT = "/v1/epochs/current/participants"
PRICING_ENDPOINT = "/v1/pricing"
MODELS_ENDPOINT = "/v1/models"
CHAIN_STATUS_ENDPOINT = "/chain-rpc/status"
PARTICIPANT_STATS_ENDPOINT = "/chain-api/productscience/inference/inference/participant"

HARDWARE_NODE_STATUS_MAP = {
    "UNKNOWN": 0, "INFERENCE": 1, "POC": 2,
    "TRAINING": 3, "STOPPED": 4, "FAILED": 5,
}
POC_STATUS_MAP = {"IDLE": 0, "GENERATING": 1, "VALIDATING": 2}

# =============================================================================
# PROMETHEUS METRICS
# =============================================================================

BLOCK_HEIGHT_MAX = Gauge("gonka_block_height_max", "Maximum block height from public Gonka nodes")
BLOCK_HEIGHT = Gauge("gonka_block_height", "Latest block height from Tendermint RPC")
BLOCK_TIME = Gauge("gonka_block_time_seconds", "Timestamp of latest block (seconds since epoch)")
NODE_STATUS = Gauge("gonka_node_status", "Node status (0=UNKNOWN,1=INFERENCE,2=POC,3=TRAINING,4=STOPPED,5=FAILED)", ["node_id", "host"])
NODE_POC_WEIGHT = Gauge("gonka_node_poc_weight", "POC weight per node", ["node_id", "host", "model"])
NETWORK_PARTICIPANT_WEIGHT = Gauge("gonka_network_participant_weight", "Weight of each participant", ["participant"])
NETWORK_NODE_POC_WEIGHT = Gauge("gonka_network_node_poc_weight", "PoC weight per network node", ["participant", "node_id"])
PRICING_UNIT_OF_COMPUTE_PRICE = Gauge("gonka_pricing_unit_of_compute_price", "Unit of compute price")
PRICING_DYNAMIC_ENABLED = Gauge("gonka_pricing_dynamic_enabled", "Dynamic pricing enabled (1/0)")
PRICING_MODEL_PRICE = Gauge("gonka_pricing_model_price_per_token", "Price per token per model", ["model_id"])
PRICING_MODEL_UNITS = Gauge("gonka_pricing_model_units_per_token", "Compute units per token per model", ["model_id"])
MODEL_V_RAM = Gauge("gonka_model_v_ram", "VRAM requirement (GB) per model", ["model_id"])
MODEL_THROUGHPUT = Gauge("gonka_model_throughput_per_nonce", "Throughput per nonce per model", ["model_id"])
MODEL_VALIDATION_THRESHOLD = Gauge("gonka_model_validation_threshold", "Validation threshold", ["model_id"])
PARTICIPANT_EPOCHS_COMPLETED = Gauge("gonka_participant_epochs_completed", "Epochs completed", ["participant"])
PARTICIPANT_COIN_BALANCE = Gauge("gonka_participant_coin_balance", "Coin balance", ["participant"])
PARTICIPANT_INFERENCE_COUNT = Gauge("gonka_participant_inference_count", "Inference count this epoch", ["participant"])
PARTICIPANT_MISSED_REQUESTS = Gauge("gonka_participant_missed_requests", "Missed requests this epoch", ["participant"])
PARTICIPANT_EARNED_COINS = Gauge("gonka_participant_earned_coins", "Earned coins this epoch", ["participant"])
PARTICIPANT_VALIDATED_INFERENCES = Gauge("gonka_participant_validated_inferences", "Validated inferences", ["participant"])
PARTICIPANT_INVALIDATED_INFERENCES = Gauge("gonka_participant_invalidated_inferences", "Invalidated inferences", ["participant"])
NODE_INTENDED_STATUS = Gauge("gonka_node_intended_status", "Intended status", ["node_id", "host"])
NODE_POC_CURRENT_STATUS = Gauge("gonka_node_poc_current_status", "Current POC status", ["node_id", "host"])
NODE_POC_INTENDED_STATUS = Gauge("gonka_node_poc_intended_status", "Intended POC status", ["node_id", "host"])
NODE_GPU_DEVICE_COUNT = Gauge("gonka_node_gpu_device_count", "GPU device count", ["node_id", "host"])
NODE_GPU_AVG_UTILIZATION = Gauge("gonka_node_gpu_avg_utilization_percent", "Average GPU utilization %", ["node_id", "host"])
NODE_POC_TIMESLOT_ASSIGNED = Gauge("gonka_node_poc_timeslot_assigned", "PoC timeslot assigned (1/0)", ["node_id", "host", "model"])
EARLIEST_BLOCK_HEIGHT = Gauge("gonka_chain_earliest_block_height", "Earliest block height")
EARLIEST_BLOCK_TIME = Gauge("gonka_chain_earliest_block_time", "Earliest block timestamp")
CATCHING_UP = Gauge("gonka_chain_catching_up", "Whether node is catching up (1/0)")

# =============================================================================
# FETCH FUNCTIONS
# =============================================================================

def fetch_tendermint_status() -> Optional[Dict[str, Any]]:
    url = f"{BASE_URL}{TENDERMINT_STATUS_ENDPOINT}"
    try:
        result = subprocess.run(
            ["docker", "exec", "node", "wget", "-qO-", url],
            capture_output=True, text=True, timeout=10
        )
        if result.returncode != 0:
            print(f"[ERROR] wget failed: {result.stderr}")
            return None
        data = json.loads(result.stdout)
        return data.get("result", {})
    except Exception as exc:
        print(f"[ERROR] Failed to fetch Tendermint status: {exc}")
        return None

def fetch_chain_status_from_node(node_url: str) -> Optional[Dict[str, Any]]:
    url = f"{node_url}{CHAIN_STATUS_ENDPOINT}"
    try:
        response = requests.get(url, timeout=5)
        response.raise_for_status()
        return response.json().get("result", {})
    except Exception as exc:
        print(f"[ERROR] Failed to fetch chain status from {url}: {exc}")
        return None

def fetch_max_block_height_from_nodes() -> Optional[Tuple[int, str]]:
    max_height = None
    latest_time = None
    nodes_to_check = ["http://localhost:8000"]
    nodes_to_check.extend(random.sample(BLOCK_HEIGHT_NODES, min(5, len(BLOCK_HEIGHT_NODES))))
    for node_url in nodes_to_check:
        status = fetch_chain_status_from_node(node_url)
        if not status:
            continue
        sync_info = status.get("sync_info", {})
        height_str = sync_info.get("latest_block_height")
        time_str = sync_info.get("latest_block_time")
        if height_str:
            try:
                height = int(height_str)
                if max_height is None or height > max_height:
                    max_height = height
                    latest_time = time_str
            except Exception:
                pass
    if max_height is not None:
        return max_height, latest_time
    return None

def fetch_participants() -> Optional[Dict[str, Any]]:
    url = f"{NETWORK_API_URL}{PARTICIPANTS_ENDPOINT}"
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        return response.json()
    except Exception as exc:
        print(f"[ERROR] Failed to fetch participants: {exc}")
        return None

def fetch_pricing() -> Optional[Dict[str, Any]]:
    url = f"{NETWORK_API_URL}{PRICING_ENDPOINT}"
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        return response.json()
    except Exception as exc:
        print(f"[ERROR] Failed to fetch pricing: {exc}")
        return None

def fetch_models() -> Optional[Dict[str, Any]]:
    url = f"{NETWORK_API_URL}{MODELS_ENDPOINT}"
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        return response.json()
    except Exception as exc:
        print(f"[ERROR] Failed to fetch models: {exc}")
        return None

def fetch_participant_stats(address: str) -> Optional[Dict[str, Any]]:
    url = f"{NETWORK_API_URL}{PARTICIPANT_STATS_ENDPOINT}/{address}"
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        return response.json()
    except Exception as exc:
        print(f"[ERROR] Failed to fetch participant stats: {exc}")
        return None

def fetch_nodes() -> List[Dict[str, Any]]:
    url = f"{NODE_BASE_URL}/nodes"
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        return response.json()
    except Exception as exc:
        print(f"[ERROR] Failed to fetch nodes: {exc}")
        return []

def fetch_gpu_stats(host: str, port: int) -> Tuple[int, float]:
    url = f"http://{host}:{port}/v3.0.8/api/v1/gpu/devices"
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        devices = response.json().get("devices", [])
        count = len(devices)
        if count == 0:
            return 0, 0.0
        total_util = sum(d.get("utilization_percent", 0) for d in devices if isinstance(d, dict))
        return count, total_util / count
    except Exception as exc:
        print(f"[ERROR] Failed to fetch GPU stats from {url}: {exc}")
        return 0, 0.0

# =============================================================================
# UPDATE FUNCTIONS
# =============================================================================

def update_tendermint_metrics():
    if EXPORT_NETWORK_METRICS:
        result = fetch_max_block_height_from_nodes()
        if result:
            max_height, latest_time = result
            BLOCK_HEIGHT_MAX.set(max_height)
            if latest_time:
                try:
                    dt = datetime.fromisoformat(latest_time.rstrip("Z")).replace(tzinfo=timezone.utc)
                    BLOCK_TIME.set(dt.timestamp())
                except Exception:
                    pass

        local_status = fetch_tendermint_status()
        if local_status:
            sync_info = local_status.get("sync_info", {})
            local_height = sync_info.get("latest_block_height")
            if local_height:
                try:
                    BLOCK_HEIGHT.set(int(local_height))
                except Exception:
                    pass
            CATCHING_UP.set(1 if sync_info.get("catching_up", False) else 0)

        for node_url in BLOCK_HEIGHT_NODES:
            status = fetch_chain_status_from_node(node_url)
            if status:
                sync_info = status.get("sync_info", {})
                eh = sync_info.get("earliest_block_height")
                if eh:
                    try:
                        EARLIEST_BLOCK_HEIGHT.set(int(eh))
                    except Exception:
                        pass
                et = sync_info.get("earliest_block_time")
                if et:
                    try:
                        dt = datetime.fromisoformat(et.rstrip("Z")).replace(tzinfo=timezone.utc)
                        EARLIEST_BLOCK_TIME.set(dt.timestamp())
                    except Exception:
                        pass
                break
    else:
        status = fetch_tendermint_status()
        if not status:
            return
        sync_info = status.get("sync_info", {})
        lh = sync_info.get("latest_block_height")
        if lh:
            try:
                BLOCK_HEIGHT.set(int(lh))
            except Exception:
                pass
        lt = sync_info.get("latest_block_time")
        if lt:
            try:
                dt = datetime.fromisoformat(lt.rstrip("Z")).replace(tzinfo=timezone.utc)
                BLOCK_TIME.set(dt.timestamp())
            except Exception:
                pass
        eh = sync_info.get("earliest_block_height")
        if eh:
            try:
                EARLIEST_BLOCK_HEIGHT.set(int(eh))
            except Exception:
                pass
        et = sync_info.get("earliest_block_time")
        if et:
            try:
                dt = datetime.fromisoformat(et.rstrip("Z")).replace(tzinfo=timezone.utc)
                EARLIEST_BLOCK_TIME.set(dt.timestamp())
            except Exception:
                pass
        CATCHING_UP.set(1 if sync_info.get("catching_up", False) else 0)

def update_network_metrics():
    if not EXPORT_NETWORK_METRICS:
        return
    data = fetch_participants()
    if not data:
        return
    participants = data.get("active_participants", {}).get("participants", [])
    for participant in participants:
        address = participant.get("seed", {}).get("participant")
        weight = participant.get("weight")
        if address and weight is not None:
            NETWORK_PARTICIPANT_WEIGHT.labels(participant=address).set(weight)
        for group in participant.get("ml_nodes", []):
            for node in group.get("ml_nodes", []):
                node_id = node.get("node_id")
                poc_weight = node.get("poc_weight")
                if address and node_id and poc_weight is not None:
                    NETWORK_NODE_POC_WEIGHT.labels(participant=address, node_id=node_id).set(poc_weight)

def update_pricing_metrics():
    if not EXPORT_NETWORK_METRICS:
        return
    pricing = fetch_pricing()
    if not pricing:
        return
    up = pricing.get("unit_of_compute_price")
    if up is not None:
        PRICING_UNIT_OF_COMPUTE_PRICE.set(up)
    de = pricing.get("dynamic_pricing_enabled")
    if de is not None:
        PRICING_DYNAMIC_ENABLED.set(1 if de else 0)
    for model in pricing.get("models", []):
        mid = model.get("id")
        if not mid:
            continue
        ppt = model.get("price_per_token")
        if ppt is not None:
            PRICING_MODEL_PRICE.labels(model_id=mid).set(ppt)
        upt = model.get("units_of_compute_per_token")
        if upt is not None:
            PRICING_MODEL_UNITS.labels(model_id=mid).set(upt)

def update_model_metrics():
    if not EXPORT_NETWORK_METRICS:
        return
    models = fetch_models()
    if not models:
        return
    for model in models.get("models", []):
        mid = model.get("id")
        if not mid:
            continue
        vr = model.get("v_ram")
        if vr is not None:
            MODEL_V_RAM.labels(model_id=mid).set(vr)
        tp = model.get("throughput_per_nonce")
        if tp is not None:
            MODEL_THROUGHPUT.labels(model_id=mid).set(tp)
        vt = model.get("validation_threshold", {})
        val_v = vt.get("value")
        val_e = vt.get("exponent")
        if val_v is not None and val_e is not None:
            try:
                MODEL_VALIDATION_THRESHOLD.labels(model_id=mid).set(float(val_v) * (10 ** int(val_e)))
            except Exception:
                pass

def update_participant_metrics():
    if not PARTICIPANT_ADDRESS:
        return
    p_data = fetch_participant_stats(PARTICIPANT_ADDRESS)
    if not p_data or not isinstance(p_data, dict):
        return
    participant = p_data.get("participant", {})
    epochs = participant.get("epochs_completed")
    if epochs is not None:
        try:
            PARTICIPANT_EPOCHS_COMPLETED.labels(participant=PARTICIPANT_ADDRESS).set(int(epochs))
        except Exception:
            pass
    cb = participant.get("coin_balance")
    if cb is not None:
        try:
            PARTICIPANT_COIN_BALANCE.labels(participant=PARTICIPANT_ADDRESS).set(int(cb))
        except Exception:
            pass
    es = participant.get("current_epoch_stats", {})
    for metric, key in [
        (PARTICIPANT_INFERENCE_COUNT, "inference_count"),
        (PARTICIPANT_MISSED_REQUESTS, "missed_requests"),
        (PARTICIPANT_EARNED_COINS, "earned_coins"),
        (PARTICIPANT_VALIDATED_INFERENCES, "validated_inferences"),
        (PARTICIPANT_INVALIDATED_INFERENCES, "invalidated_inferences"),
    ]:
        val = es.get(key)
        if val is not None:
            try:
                metric.labels(participant=PARTICIPANT_ADDRESS).set(int(val))
            except Exception:
                pass

def update_node_metrics():
    if not ENABLE_NODE_FETCH:
        return
    nodes = fetch_nodes()
    if not nodes:
        return
    for entry in nodes:
        node_info = entry.get("node", {})
        node_id = node_info.get("id", "unknown")
        node_host = node_info.get("host", "unknown")
        node_port = node_info.get("poc_port")
        state = entry.get("state", {})
        current_status = state.get("current_status", "").upper()
        NODE_STATUS.labels(node_id=node_id, host=node_host).set(HARDWARE_NODE_STATUS_MAP.get(current_status, 0))
        intended_status = state.get("intended_status", "").upper()
        if intended_status:
            NODE_INTENDED_STATUS.labels(node_id=node_id, host=node_host).set(HARDWARE_NODE_STATUS_MAP.get(intended_status, 0))
        poc_status = state.get("poc_current_status", "").upper()
        NODE_POC_CURRENT_STATUS.labels(node_id=node_id, host=node_host).set(POC_STATUS_MAP.get(poc_status, 0))
        poc_intended = state.get("poc_intended_status", "").upper()
        if poc_intended:
            NODE_POC_INTENDED_STATUS.labels(node_id=node_id, host=node_host).set(POC_STATUS_MAP.get(poc_intended, 0))
        epoch_ml_nodes = state.get("epoch_ml_nodes", {})
        for model, model_data in epoch_ml_nodes.items():
            if isinstance(model_data, dict):
                pw = model_data.get("poc_weight")
                if pw is not None:
                    NODE_POC_WEIGHT.labels(node_id=node_id, host=node_host, model=model).set(pw)
                ta = model_data.get("timeslot_allocation", [])
                if isinstance(ta, list) and len(ta) >= 2:
                    NODE_POC_TIMESLOT_ASSIGNED.labels(node_id=node_id, host=node_host, model=model).set(1 if ta[1] else 0)
        if node_port and node_host:
            gpu_count, gpu_avg_util = fetch_gpu_stats(node_host, node_port)
            NODE_GPU_DEVICE_COUNT.labels(node_id=node_id, host=node_host).set(gpu_count)
            NODE_GPU_AVG_UTILIZATION.labels(node_id=node_id, host=node_host).set(gpu_avg_util)

def update_metrics():
    print(f"[INFO] Updating metrics... (Network={EXPORT_NETWORK_METRICS}, Nodes={ENABLE_NODE_FETCH}, Participant={bool(PARTICIPANT_ADDRESS)})")
    update_tendermint_metrics()
    if EXPORT_NETWORK_METRICS:
        update_network_metrics()
        update_pricing_metrics()
        update_model_metrics()
    update_participant_metrics()
    update_node_metrics()

# =============================================================================
# MAIN
# =============================================================================

def main():
    print("=" * 70)
    print("Gonka Prometheus Exporter")
    print("=" * 70)
    print(f"  BASE_URL: {BASE_URL}")
    print(f"  NODE_BASE_URL: {NODE_BASE_URL}")
    print(f"  EXPORTER_PORT: {EXPORTER_PORT}")
    print(f"  REFRESH_INTERVAL: {REFRESH_INTERVAL}s")
    print(f"  EXPORT_NETWORK_METRICS: {EXPORT_NETWORK_METRICS}")
    print(f"  ENABLE_NODE_FETCH: {ENABLE_NODE_FETCH}")
    print(f"  PARTICIPANT_ADDRESS: {'<set>' if PARTICIPANT_ADDRESS else '<not set>'}")
    print("=" * 70)
    start_http_server(EXPORTER_PORT)
    print(f"[INFO] Metrics at http://localhost:{EXPORTER_PORT}/metrics")
    update_metrics()
    while True:
        time.sleep(REFRESH_INTERVAL)
        update_metrics()

if __name__ == "__main__":
    main()
