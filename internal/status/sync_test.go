package status

import (
	"context"
	"strings"
	"testing"
	"time"
)

const (
	stripeAppliedChunk = `5:51PM INF Applied snapshot chunk to ABCI app chunk=309 format=3 height=4102000 module=statesync total=1168`
	stripeDiscovered   = `5:42PM INF Discovered new snapshot height=4102000 format=3 hash=abc123`
	stripeBlockSync    = `6:02PM INF received block height=4103250 module=blockchain peer=xyz`
)

func TestDeriveSyncStatus_Synced(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "4102500"
	rpc.Result.SyncInfo.CatchingUp = false

	s := DeriveSyncStatus(rpc, 12, nil, 0)
	if s.Phase != SyncPhaseSynced {
		t.Errorf("Phase = %s, want Synced", s.Phase)
	}
	if !strings.Contains(s.Reason, "4102500") {
		t.Errorf("Reason should cite height: %q", s.Reason)
	}
}

func TestDeriveSyncStatus_BlockSync(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "4102000"
	rpc.Result.SyncInfo.CatchingUp = true

	s := DeriveSyncStatus(rpc, 9, nil, 30*time.Second)
	if s.Phase != SyncPhaseBlockSync {
		t.Errorf("Phase = %s, want BlockSync", s.Phase)
	}
}

func TestDeriveSyncStatus_HeightZeroNoPeers(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "0"
	rpc.Result.SyncInfo.CatchingUp = true

	s := DeriveSyncStatus(rpc, 0, nil, 0)
	if s.Phase != SyncPhaseInitializing {
		t.Errorf("Phase = %s, want Initializing", s.Phase)
	}
	if !strings.Contains(s.Reason, "peers") {
		t.Errorf("Reason should mention peers: %q", s.Reason)
	}
}

func TestDeriveSyncStatus_HeightZeroWithPeersNoLogs(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "0"
	rpc.Result.SyncInfo.CatchingUp = true

	s := DeriveSyncStatus(rpc, 9, nil, 5*time.Minute)
	if s.Phase != SyncPhaseInitializing {
		t.Errorf("Phase = %s, want Initializing", s.Phase)
	}
	if !strings.Contains(s.Reason, "Negotiating") {
		t.Errorf("Reason should mention negotiating: %q", s.Reason)
	}
}

func TestDeriveSyncStatus_FromLogsSnapshotApplying(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "0"
	rpc.Result.SyncInfo.CatchingUp = true

	logs := func(_ context.Context, _ string, _ int) (string, error) {
		// Simulate growing log: include the chunk applied lines
		return `INF Fetching snapshot chunk chunk=309 format=3 height=4102000 module=statesync total=1168
INF Applied snapshot chunk to ABCI app chunk=309 format=3 height=4102000 module=statesync total=1168
INF Fetching snapshot chunk chunk=310 format=3 height=4102000 module=statesync total=1168`, nil
	}

	s := DeriveSyncStatus(rpc, 9, logs, 0)
	if s.Phase != SyncPhaseSnapshotApplying {
		t.Errorf("Phase = %s, want SnapshotApplying", s.Phase)
	}
	if s.SnapshotHeight != 4102000 {
		t.Errorf("SnapshotHeight = %d, want 4102000", s.SnapshotHeight)
	}
	if s.SnapshotChunkIdx != 309 {
		t.Errorf("SnapshotChunkIdx = %d, want 309", s.SnapshotChunkIdx)
	}
	if s.SnapshotChunkTotal != 1168 {
		t.Errorf("SnapshotChunkTotal = %d, want 1168", s.SnapshotChunkTotal)
	}
	expectedProgress := 309.0 / 1168.0
	if s.SnapshotProgress < expectedProgress-0.01 || s.SnapshotProgress > expectedProgress+0.01 {
		t.Errorf("SnapshotProgress = %.4f, want %.4f", s.SnapshotProgress, expectedProgress)
	}
	if !strings.Contains(s.Reason, "26%") && !strings.Contains(s.Reason, "27%") {
		t.Errorf("Reason should include percentage near 26%%: %q", s.Reason)
	}
}

func TestDeriveSyncStatus_LogsSnapshotDiscovery(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "0"
	rpc.Result.SyncInfo.CatchingUp = true

	logs := func(_ context.Context, _ string, _ int) (string, error) {
		return stripeDiscovered, nil
	}
	s := DeriveSyncStatus(rpc, 9, logs, 0)
	if s.Phase != SyncPhaseSnapshotDiscover {
		t.Errorf("Phase = %s, want SnapshotDiscover", s.Phase)
	}
	if s.SnapshotHeight != 4102000 {
		t.Errorf("SnapshotHeight = %d", s.SnapshotHeight)
	}
}

func TestEstimateETA(t *testing.T) {
	s := &SyncStatus{
		Phase:              SyncPhaseSnapshotApplying,
		SnapshotChunkIdx:   309,
		SnapshotChunkTotal: 1168,
	}
	eta := s.EstimateETA()
	if eta < 2*time.Minute || eta > 4*time.Minute {
		t.Errorf("EstimateETA = %s, expected ~2m45s (~ (1168-309)/5 sec)", eta)
	}

	s.Phase = SyncPhaseSynced
	if s.EstimateETA() != 0 {
		t.Error("EstimateETA should be 0 for SyncPhaseSynced")
	}
}

func TestParseAppliedChunkRegex(t *testing.T) {
	m := appliedChunkRe.FindStringSubmatch(stripeAppliedChunk)
	if m == nil {
		t.Fatal("regex did not match")
	}
	if m[1] != "309" || m[2] != "4102000" || m[3] != "1168" {
		t.Errorf("groups = %v", m)
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mERR\x1b[0m something \x1b[1;32m"
	want := "ERR something "
	if got := stripANSI(in); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDeriveSyncStatus_LogsBlockSyncTarget(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "4102000"
	rpc.Result.SyncInfo.CatchingUp = true

	logs := func(_ context.Context, _ string, _ int) (string, error) {
		return stripeBlockSync, nil
	}
	s := DeriveSyncStatus(rpc, 9, logs, 0)
	if s.Phase != SyncPhaseBlockSync {
		t.Errorf("Phase = %s, want BlockSync", s.Phase)
	}
	if s.TargetHeight != 4103250 {
		t.Errorf("TargetHeight = %d, want 4103250", s.TargetHeight)
	}
}

func TestDeriveSyncStatus_LogReadError(t *testing.T) {
	rpc := &TendermintStatus{}
	rpc.Result.SyncInfo.LatestBlockHeight = "0"
	rpc.Result.SyncInfo.CatchingUp = true

	logs := func(_ context.Context, _ string, _ int) (string, error) {
		return "", context.DeadlineExceeded
	}
	s := DeriveSyncStatus(rpc, 9, logs, 0)
	if s.Phase != SyncPhaseInitializing {
		t.Errorf("Phase = %s, want Initializing (fallback)", s.Phase)
	}
	if !strings.Contains(s.Reason, "Could not read node logs") {
		t.Errorf("Reason should explain log failure: %q", s.Reason)
	}
}
