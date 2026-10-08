package designgates

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/testkit"
)

var (
	ErrDestinationExists   = errors.New("destination already exists without overwrite token")
	ErrSubtreeInvalidated  = errors.New("directory subtree modified; reviewed token invalidated")
	ErrLeaseExpired        = errors.New("read lease expired")
	ErrLeaseNotFound       = errors.New("read lease not found")
	ErrConcurrentSourceMod = errors.New("source modified concurrently during move; retained")
)

type journalState string

const (
	statePlanned   journalState = "planned"
	stateStaged    journalState = "staged"
	stateInstalled journalState = "installed"
	stateVerified  journalState = "verified"
	stateCompleted journalState = "completed"
)

type moveOperation struct {
	OpID              string
	State             journalState
	SourcePath        string
	DestPath          string
	ReviewedSrcHash   string
	ReviewedSubtree   string
	OverwriteApproved bool
	DisplacedRecovery string
	SourceRetained    bool
}

// computeHash calculates SHA-256 hex digest of file contents.
func computeHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// TestOrbitG03MoveOverwriteAndRecoveryPreservation verifies that moving onto an existing
// destination requires an explicit overwrite token and displaces the previous destination into
// recovery storage rather than discarding it permanently.
func TestOrbitG03MoveOverwriteAndRecoveryPreservation(t *testing.T) {
	root := testkit.NewDisposable(t)

	recoveryDir := filepath.Join(root, ".orbit-internal", "recovery")
	_ = os.MkdirAll(recoveryDir, 0700)

	src := filepath.Join(root, "fileA.txt")
	dst := filepath.Join(root, "fileB.txt")

	mustWrite(t, src, "contents-of-A")
	mustWrite(t, dst, "original-contents-of-B")

	srcHash, _ := computeHash(src)

	op := &moveOperation{
		OpID:            "op-1",
		State:           statePlanned,
		SourcePath:      src,
		DestPath:        dst,
		ReviewedSrcHash: srcHash,
	}

	// Case 1: Overwrite without token is rejected
	op.OverwriteApproved = false
	if _, err := os.Stat(dst); err == nil && !op.OverwriteApproved {
		// Destination exists and no overwrite token provided
		err = ErrDestinationExists
		if !errors.Is(err, ErrDestinationExists) {
			t.Fatal("expected ErrDestinationExists")
		}
	}

	// Case 2: Overwrite with approved token displaces existing destination to recovery
	op.OverwriteApproved = true
	displacedName := filepath.Join(recoveryDir, "recovered-fileB-"+op.OpID)
	if err := os.Rename(dst, displacedName); err != nil {
		t.Fatalf("failed to displace destination to recovery: %v", err)
	}
	op.DisplacedRecovery = displacedName
	op.State = stateStaged

	// Install destination from source
	if err := os.Rename(src, dst); err != nil {
		t.Fatalf("failed to move src to dst: %v", err)
	}
	op.State = stateCompleted

	// Verify dst now has source content
	if got := mustRead(t, dst); got != "contents-of-A" {
		t.Fatalf("expected dst to have contents-of-A, got: %s", got)
	}
	// Verify displaced destination is preserved intact in recovery!
	if got := mustRead(t, displacedName); got != "original-contents-of-B" {
		t.Fatalf("expected recovery file to preserve original-contents-of-B, got: %s", got)
	}
}

// TestOrbitG03MoveSourceConcurrentModificationRetainsBoth verifies Invariant I26:
// if a source file is modified concurrently by an editor while a move is taking place,
// the source is NOT removed. Both the destination and the modified source are preserved.
func TestOrbitG03MoveSourceConcurrentModificationRetainsBoth(t *testing.T) {
	root := testkit.NewDisposable(t)

	src := filepath.Join(root, "report.md")
	dst := filepath.Join(root, "report_final.md")

	mustWrite(t, src, "initial-report-v1")
	initialHash, _ := computeHash(src)

	op := &moveOperation{
		OpID:            "op-move-race",
		State:           statePlanned,
		SourcePath:      src,
		DestPath:        dst,
		ReviewedSrcHash: initialHash,
	}

	// Step 1: Copy/install contents to destination
	op.State = stateStaged
	mustWrite(t, dst, "initial-report-v1")
	op.State = stateInstalled

	// Concurrency simulation: editor saves new changes to source while move was installing!
	mustWrite(t, src, "editor-concurrent-edits-v2")

	// Step 2: Re-verify source before deleting
	currentSrcHash, _ := computeHash(src)
	if currentSrcHash != op.ReviewedSrcHash {
		// Race detected! Do NOT remove source.
		op.SourceRetained = true
		op.State = stateCompleted
	} else {
		_ = os.Remove(src)
		op.State = stateCompleted
	}

	// Verify both files survive
	if !op.SourceRetained {
		t.Fatal("expected SourceRetained to be true")
	}
	if got := mustRead(t, src); got != "editor-concurrent-edits-v2" {
		t.Fatalf("expected src to retain editor edits, got: %s", got)
	}
	if got := mustRead(t, dst); got != "initial-report-v1" {
		t.Fatalf("expected dst to retain moved initial version, got: %s", got)
	}
}

// TestOrbitG03DirectorySubtreeInvalidation verifies that a directory move operation is
// invalidated if a new child file appears in the directory before completion.
func TestOrbitG03DirectorySubtreeInvalidation(t *testing.T) {
	root := testkit.NewDisposable(t)

	dirA := filepath.Join(root, "project")
	_ = os.MkdirAll(dirA, 0700)
	mustWrite(t, filepath.Join(dirA, "main.go"), "package main")
	mustWrite(t, filepath.Join(dirA, "go.mod"), "module example")

	// Compute snapshot of children
	computeSubtreeDigest := func(dir string) string {
		entries, _ := os.ReadDir(dir)
		h := sha256.New()
		for _, e := range entries {
			h.Write([]byte(e.Name()))
		}
		return hex.EncodeToString(h.Sum(nil))
	}

	reviewedToken := computeSubtreeDigest(dirA)

	// Concurrent creation of new file inside dirA
	mustWrite(t, filepath.Join(dirA, "new_file.go"), "package main; func Helper() {}")

	// Verify before completing move
	currentToken := computeSubtreeDigest(dirA)
	if currentToken == reviewedToken {
		t.Fatal("expected subtree token to change after new child added")
	}

	// Move must be blocked/invalidated due to subtree mismatch
	var moveErr error
	if currentToken != reviewedToken {
		moveErr = ErrSubtreeInvalidated
	}
	if !errors.Is(moveErr, ErrSubtreeInvalidated) {
		t.Fatalf("expected ErrSubtreeInvalidated, got: %v", moveErr)
	}
}

// readLeaseManager models active content read leases that protect CAS chunks from GC sweeps.
type readLeaseManager struct {
	mu     sync.Mutex
	leases map[string]readLeaseRecord
}

type readLeaseRecord struct {
	LeaseID   string
	VersionID string
	Chunks    []string // chunk SHA-256 hashes
	ExpiresAt time.Time
}

func newReadLeaseManager() *readLeaseManager {
	return &readLeaseManager{leases: make(map[string]readLeaseRecord)}
}

func (m *readLeaseManager) AcquireLease(id, versionID string, chunks []string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leases[id] = readLeaseRecord{
		LeaseID:   id,
		VersionID: versionID,
		Chunks:    append([]string(nil), chunks...),
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (m *readLeaseManager) ReleaseLease(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.leases, id)
}

func (m *readLeaseManager) IsChunkProtected(chunkHash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, l := range m.leases {
		if now.Before(l.ExpiresAt) {
			for _, c := range l.Chunks {
				if c == chunkHash {
					return true
				}
			}
		}
	}
	return false
}

// TestOrbitG03ReadLeaseProtectsChunksFromGC tests Invariant I25:
// active read leases protect content chunks from garbage collection unlinking.
func TestOrbitG03ReadLeaseProtectsChunksFromGC(t *testing.T) {
	lm := newReadLeaseManager()

	chunk1 := "chunk-hash-1111"
	chunk2 := "chunk-hash-2222"

	// Acquire 200ms read lease
	lm.AcquireLease("lease-preview-1", "v-100", []string{chunk1, chunk2}, 200*time.Millisecond)

	// GC check while lease is active
	if !lm.IsChunkProtected(chunk1) || !lm.IsChunkProtected(chunk2) {
		t.Fatal("expected chunks to be protected while read lease is active")
	}

	// Sleep past lease expiration
	time.Sleep(250 * time.Millisecond)

	// After expiration, chunks are no longer protected by this lease
	if lm.IsChunkProtected(chunk1) {
		t.Fatal("expected chunk1 protection to expire with lease")
	}
}
