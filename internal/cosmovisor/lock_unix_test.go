//go:build !windows

package cosmovisor

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireLock_HappyPath(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".update.lock")

	release, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if release == nil {
		t.Fatal("release func is nil")
	}
	if err := release(); err != nil {
		t.Errorf("release: %v", err)
	}
}

func TestAcquireLock_RefusesWhenHeld(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".update.lock")

	release1, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer func() { _ = release1() }()

	// Second acquire from the SAME process — flock semantics treat this as
	// a separate fd so the second OpenFile + Flock should be refused.
	_, err = AcquireLock(lockPath)
	if !errors.Is(err, ErrLockHeld) {
		t.Errorf("second AcquireLock should return ErrLockHeld, got %v", err)
	}
}

func TestAcquireLock_ReleaseEnablesReacquire(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".update.lock")

	release1, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := release1(); err != nil {
		t.Fatalf("release: %v", err)
	}

	release2, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	_ = release2()
}

func TestAcquireLock_BadPath(t *testing.T) {
	// Parent directory doesn't exist — OpenFile fails.
	_, err := AcquireLock("/nonexistent/parent/lock")
	if err == nil {
		t.Fatal("expected error for nonexistent parent directory")
	}
}
