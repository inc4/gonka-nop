package status

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SyncPhase enumerates the visible sync states. CometBFT's /status reports
// `catching_up=true` with `height=0` during state-sync until ALL snapshot
// chunks have been applied AND committed — operators see no progress for
// minutes-to-hours despite the node working hard. The phases below let the
// status command surface what's actually happening.
type SyncPhase string

const (
	SyncPhaseUnknown          SyncPhase = "Unknown"
	SyncPhaseInitializing     SyncPhase = "Initializing"     // peers being discovered, no snapshot yet
	SyncPhaseSnapshotDiscover SyncPhase = "SnapshotDiscover" // negotiating snapshot offers
	SyncPhaseSnapshotApplying SyncPhase = "SnapshotApplying" // chunks streaming
	SyncPhaseBlockSync        SyncPhase = "BlockSync"        // snapshot done, fast-syncing blocks
	SyncPhaseSynced           SyncPhase = "Synced"
)

// SyncStatus describes what the node is doing right now. Populated by
// FetchStatusWithConfig when CometBFT /status alone doesn't tell the full
// story (height=0 + catching_up=true → look in node container logs).
type SyncStatus struct {
	Phase SyncPhase

	// Snapshot fields (populated during SnapshotApplying)
	SnapshotHeight     int64
	SnapshotChunkIdx   int     // last "Applied snapshot chunk to ABCI app" number
	SnapshotChunkTotal int     // total chunks for this snapshot
	SnapshotProgress   float64 // 0.0 .. 1.0

	// BlockSync fields (populated when snapshot complete and fast-syncing)
	TargetHeight int64 // best peer's reported height

	// Peers
	PeerCount int

	// Diagnostics
	StuckAtZeroFor time.Duration // how long height has been 0 (0 = not stuck)
	LastLogLine    string        // most recent statesync-related log line for the debug section
	Reason         string        // human-readable summary
}

// Apply snapshot chunk regex: `Applied snapshot chunk to ABCI app chunk=300 format=3 height=4102000 module=statesync total=1168`
var appliedChunkRe = regexp.MustCompile(`Applied snapshot chunk to ABCI app.*?chunk=(\d+).*?height=(\d+).*?total=(\d+)`)

// Discovered/offered snapshot regex: `Discovered new snapshot height=4102000 format=3 ...`
var discoveredSnapRe = regexp.MustCompile(`Discovered new snapshot.*?height=(\d+).*?format=\d+`)

// Block sync target height regex: `Fast Sync Verifying Block ... height=12345`
// or `received block ... height=12345`
var blockSyncHeightRe = regexp.MustCompile(`(?:Verifying Block|received block).*?height=(\d+)`)

// DockerLogsTail returns the last `n` lines of the named container's logs
// via `docker logs --tail n <name>`. Returns (output, error). Always merges
// stderr+stdout since docker writes log content to both depending on driver.
type DockerLogsTail func(ctx context.Context, container string, n int) (string, error)

// defaultDockerLogsTail shells out to `docker logs --tail N name`. Used by
// production; tests inject their own DockerLogsTail.
func defaultDockerLogsTail(ctx context.Context, container string, n int) (string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(dlCtx, "docker", "logs", "--tail", strconv.Itoa(n), container) // #nosec G204 - container name from state
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// DeriveSyncStatus computes a SyncStatus from a CometBFT TendermintStatus
// (RPC /status) plus optional log access. When `logs` is nil or returns an
// error, falls back to RPC-only inference and sets Reason accordingly.
func DeriveSyncStatus(rpc *TendermintStatus, peers int, logs DockerLogsTail, stuckSince time.Duration) *SyncStatus {
	s := &SyncStatus{
		PeerCount:      peers,
		StuckAtZeroFor: stuckSince,
		Phase:          SyncPhaseUnknown,
	}

	// Treat missing/zero RPC as Initializing
	if rpc == nil {
		s.Phase = SyncPhaseInitializing
		s.Reason = "RPC not yet reachable (node still starting)"
		return s
	}

	height, _ := strconv.ParseInt(rpc.Result.SyncInfo.LatestBlockHeight, 10, 64)
	catchingUp := rpc.Result.SyncInfo.CatchingUp

	switch {
	case !catchingUp && height > 0:
		s.Phase = SyncPhaseSynced
		s.TargetHeight = height
		s.Reason = fmt.Sprintf("Synced at height %d", height)
		return s
	case height > 0 && catchingUp:
		// Already past state-sync; just block-syncing. Optionally refine
		// target height from logs (if available) but do NOT fall through
		// to the height==0 branch below.
		s.Phase = SyncPhaseBlockSync
		s.TargetHeight = height
		s.Reason = fmt.Sprintf("Block-syncing — at height %d", height)
		if logs != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if out, err := logs(ctx, "node", 200); err == nil {
				parseSyncFromLogs(s, out)
			}
		}
		return s
	}

	// height == 0 && catching_up=true → likely state-sync; look in logs.
	if logs == nil {
		s.Phase = SyncPhaseInitializing
		if peers > 0 {
			s.Reason = fmt.Sprintf("Negotiating state-sync (%d peers connected, no visible progress yet)", peers)
		} else {
			s.Reason = "Waiting for peers"
		}
		return s
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := logs(ctx, "node", 200)
	if err != nil {
		s.Phase = SyncPhaseInitializing
		s.Reason = fmt.Sprintf("Could not read node logs (%v); chain probably still negotiating", err)
		return s
	}

	parseSyncFromLogs(s, out)

	// If parser found nothing actionable, classify by peer count.
	if s.Phase == SyncPhaseUnknown {
		if peers > 0 {
			s.Phase = SyncPhaseSnapshotDiscover
			s.Reason = fmt.Sprintf("Connected to %d peers, awaiting state-sync snapshot offers", peers)
		} else {
			s.Phase = SyncPhaseInitializing
			s.Reason = "Waiting for peers"
		}
	}
	return s
}

// parseSyncFromLogs scans `out` for the most informative state-sync /
// block-sync line and updates `s` in place.
func parseSyncFromLogs(s *SyncStatus, out string) {
	lines := strings.Split(out, "\n")
	// Walk from newest to oldest, prefer the most specific signal.
	var lastApplied []string
	var lastDiscovered []string
	var lastBlockSync []string
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if lastApplied == nil {
			if m := appliedChunkRe.FindStringSubmatch(line); m != nil {
				lastApplied = m
				s.LastLogLine = strings.TrimSpace(stripANSI(line))
			}
		}
		if lastDiscovered == nil {
			if m := discoveredSnapRe.FindStringSubmatch(line); m != nil {
				lastDiscovered = m
			}
		}
		if lastBlockSync == nil {
			if m := blockSyncHeightRe.FindStringSubmatch(line); m != nil {
				lastBlockSync = m
			}
		}
		if lastApplied != nil && lastBlockSync != nil {
			break
		}
	}

	switch {
	case lastApplied != nil:
		chunkIdx, _ := strconv.Atoi(lastApplied[1])
		snapHeight, _ := strconv.ParseInt(lastApplied[2], 10, 64)
		total, _ := strconv.Atoi(lastApplied[3])
		s.Phase = SyncPhaseSnapshotApplying
		s.SnapshotHeight = snapHeight
		s.SnapshotChunkIdx = chunkIdx
		s.SnapshotChunkTotal = total
		if total > 0 {
			s.SnapshotProgress = float64(chunkIdx) / float64(total)
		}
		s.Reason = fmt.Sprintf("Applying state-sync snapshot @ height %d (chunk %d/%d, %.0f%%)",
			snapHeight, chunkIdx, total, s.SnapshotProgress*100)
	case lastDiscovered != nil:
		snapHeight, _ := strconv.ParseInt(lastDiscovered[1], 10, 64)
		s.Phase = SyncPhaseSnapshotDiscover
		s.SnapshotHeight = snapHeight
		s.Reason = fmt.Sprintf("Discovered snapshot @ height %d, awaiting chunk fetch", snapHeight)
	case lastBlockSync != nil:
		h, _ := strconv.ParseInt(lastBlockSync[1], 10, 64)
		if h > s.TargetHeight {
			s.TargetHeight = h
		}
	}
}

// stripANSI removes color escape codes from a log line for cleaner display.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// EstimateETA returns a rough wall-clock estimate to finish state-sync
// based on the chunk rate observed in the logs window. Conservative — assumes
// constant 5 chunks/sec when no other signal is available.
func (s *SyncStatus) EstimateETA() time.Duration {
	if s.Phase != SyncPhaseSnapshotApplying || s.SnapshotChunkTotal <= 0 {
		return 0
	}
	remaining := s.SnapshotChunkTotal - s.SnapshotChunkIdx
	if remaining <= 0 {
		return 0
	}
	const observedRateChunksPerSec = 5
	return time.Duration(remaining/observedRateChunksPerSec) * time.Second
}
