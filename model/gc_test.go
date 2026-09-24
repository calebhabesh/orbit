package model

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

func permutations(values []string) [][]string {
	var output [][]string
	var visit func([]string, int)
	visit = func(items []string, start int) {
		if start == len(items) {
			output = append(output, append([]string(nil), items...))
			return
		}
		for index := start; index < len(items); index++ {
			items[start], items[index] = items[index], items[start]
			visit(items, start+1)
			items[start], items[index] = items[index], items[start]
		}
	}
	visit(append([]string(nil), values...), 0)
	sort.Slice(output, func(i, j int) bool { return fmt.Sprint(output[i]) < fmt.Sprint(output[j]) })
	return output
}

func indexOf(values []string, value string) int {
	for index, candidate := range values {
		if candidate == value {
			return index
		}
	}
	return -1
}

func TestReferenceSetOracleInterleavings(t *testing.T) {
	actions := []string{"begin", "unlink", "reference"}
	for _, schedule := range permutations(actions) {
		beginIndex := indexOf(schedule, "begin")
		unlinkIndex := indexOf(schedule, "unlink")
		if beginIndex > unlinkIndex {
			continue // Unlink can only happen after begin
		}

		t.Run(fmt.Sprint(schedule), func(t *testing.T) {
			oracle := NewReferenceSetOracle()
			oracle.InstallObject("chunk-1", true)

			for _, action := range schedule {
				switch action {
				case "begin":
					_, err := oracle.BeginGC([]string{"chunk-1"})
					if err != nil {
						t.Fatal(err)
					}
				case "unlink":
					oracle.Unlink("chunk-1")
				case "reference":
					committed := oracle.AcquireReference("chunk-1", "v-new")
					if !committed {
						t.Fatalf("reference failed to commit with verified bytes: %v", oracle.Trace())
					}
					if oracle.ObjectState("chunk-1") != ObjectPresent {
						t.Fatalf("referenced object is not present: %v", oracle.Trace())
					}
				}
			}
		})
	}
}

func TestReferenceSetOracleServeLeaseBlocksGCIntent(t *testing.T) {
	oracle := NewReferenceSetOracle()
	oracle.InstallObject("chunk-1", true)

	if err := oracle.AcquireLease("chunk-1", "lease-1"); err != nil {
		t.Fatalf("could not acquire lease: %v", err)
	}

	acquired, err := oracle.BeginGC([]string{"chunk-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(acquired) != 0 {
		t.Fatalf("GC intent acquired while serve lease was active: %v", acquired)
	}

	// Unlinking without intent must fail
	if oracle.Unlink("chunk-1") {
		t.Fatal("unlinked chunk without intent")
	}

	oracle.ReleaseLease("chunk-1", "lease-1")

	// Now GC intent should succeed
	acquired, err = oracle.BeginGC([]string{"chunk-1"})
	if err != nil || len(acquired) != 1 {
		t.Fatalf("GC intent failed after lease release: %v err=%v", acquired, err)
	}

	// While intent is active, new lease must be blocked
	if err := oracle.AcquireLease("chunk-1", "lease-2"); err != ErrGCIntentActive {
		t.Fatalf("lease on active GC intent got %v, want ErrGCIntentActive", err)
	}
}

func TestReferenceSetOracleSharedChunks(t *testing.T) {
	oracle := NewReferenceSetOracle()
	now := time.Now()
	policy := RetentionPolicy{RetentionDays: 30, MinSuperseded: 20}

	// Chunk shared by an old expired version and a current head
	oracle.InstallObject("shared-chunk", true)
	oracle.InstallObject("head-only-chunk", true)
	oracle.InstallObject("old-only-chunk", true)

	oldTime := now.Add(-60 * 24 * time.Hour) // 60 days old
	// Create 25 superseded versions so old-v is outside MinSuperseded (20)
	for i := 1; i <= 25; i++ {
		vID := fmt.Sprintf("v-old-%d", i)
		oracle.AddVersion(VersionRecord{
			ID:       vID,
			Path:     "file.txt",
			Chunks:   []string{"old-only-chunk"},
			Acquired: oldTime.Add(time.Duration(i) * time.Hour),
		})
	}

	// Old version with shared-chunk
	oracle.AddVersion(VersionRecord{
		ID:       "v-old-shared",
		Path:     "file.txt",
		Chunks:   []string{"shared-chunk"},
		Acquired: oldTime,
	})

	// Current head with shared-chunk and head-only-chunk
	oracle.AddVersion(VersionRecord{
		ID:       "v-head",
		Path:     "file.txt",
		Chunks:   []string{"shared-chunk", "head-only-chunk"},
		Acquired: now,
	})
	oracle.SetHeads("file.txt", []string{"v-head"})

	protected := oracle.ComputeProtectedChunks(now, policy)
	if !protected["shared-chunk"] {
		t.Fatal("shared-chunk must be protected because head references it")
	}
	if !protected["head-only-chunk"] {
		t.Fatal("head-only-chunk must be protected because head references it")
	}

	candidates := oracle.ComputeCandidates(now, policy)
	// old-only-chunk should not be candidate if among the 20 newest superseded,
	// but v-old-shared is oldest (index 25) so shared-chunk would be expired IF not for head!
	for _, c := range candidates {
		if c == "shared-chunk" {
			t.Fatal("shared-chunk was erroneously marked as GC candidate")
		}
	}
}

func TestReferenceSetOraclePendingFallbackPreservation(t *testing.T) {
	oracle := NewReferenceSetOracle()
	now := time.Now()
	policy := RetentionPolicy{RetentionDays: 1, MinSuperseded: 0} // aggressive expiry

	oracle.InstallObject("chunk-v1", true)
	oracle.InstallObject("chunk-v2", true)

	oldTime := now.Add(-10 * 24 * time.Hour)
	oracle.AddVersion(VersionRecord{
		ID:       "v1",
		Path:     "doc.txt",
		Chunks:   []string{"chunk-v1"},
		Acquired: oldTime,
	})
	oracle.AddVersion(VersionRecord{
		ID:       "v2",
		Path:     "doc.txt",
		Chunks:   []string{"chunk-v2"},
		Acquired: now,
	})

	// v2 is head, but publication of v2 is pending, so v1 is fallback!
	oracle.SetHeads("doc.txt", []string{"v2"})
	oracle.SetPending("doc.txt", true, "v1")

	protected := oracle.ComputeProtectedChunks(now, policy)
	if !protected["chunk-v1"] {
		t.Fatal("chunk-v1 must be protected as pending publication fallback content")
	}
	if !protected["chunk-v2"] {
		t.Fatal("chunk-v2 must be protected as current head")
	}

	// Once publication finishes (pending=false), v1 is no longer fallback and can expire
	oracle.SetPending("doc.txt", false, "")
	candidates := oracle.ComputeCandidates(now, policy)
	foundV1 := false
	for _, c := range candidates {
		if c == "chunk-v1" {
			foundV1 = true
		}
		if c == "chunk-v2" {
			t.Fatal("chunk-v2 must remain protected")
		}
	}
	if !foundV1 {
		t.Fatal("chunk-v1 should be candidate once no longer fallback")
	}
}

func TestReferenceSetOracleClockJumps(t *testing.T) {
	oracle := NewReferenceSetOracle()
	baseTime := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	policy := RetentionPolicy{RetentionDays: 30, MinSuperseded: 20}

	oracle.InstallObject("head-chunk", true)
	oracle.AddVersion(VersionRecord{
		ID:       "head",
		Path:     "doc.txt",
		Chunks:   []string{"head-chunk"},
		Acquired: baseTime,
	})
	oracle.SetHeads("doc.txt", []string{"head"})

	// Create 10 superseded versions (all within MinSuperseded = 20)
	for i := 1; i <= 10; i++ {
		chunkID := fmt.Sprintf("hist-chunk-%d", i)
		vID := fmt.Sprintf("hist-%d", i)
		oracle.InstallObject(chunkID, true)
		oracle.AddVersion(VersionRecord{
			ID:       vID,
			Path:     "doc.txt",
			Chunks:   []string{chunkID},
			Acquired: baseTime.Add(-time.Duration(i) * time.Hour),
		})
	}

	// Test 1: Clock jumps 50 years into the FUTURE
	futureTime := baseTime.Add(50 * 365 * 24 * time.Hour)
	protectedFuture := oracle.ComputeProtectedChunks(futureTime, policy)
	if !protectedFuture["head-chunk"] {
		t.Fatal("head chunk must remain protected even after 50 years future clock jump")
	}
	for i := 1; i <= 10; i++ {
		chunkID := fmt.Sprintf("hist-chunk-%d", i)
		if !protectedFuture[chunkID] {
			t.Fatalf("%s must remain protected under MinSuperseded arm even with clock jump into future", chunkID)
		}
	}

	// Test 2: Clock jumps 10 years into the PAST (clock rollback)
	pastTime := baseTime.Add(-10 * 365 * 24 * time.Hour)
	protectedPast := oracle.ComputeProtectedChunks(pastTime, policy)
	if !protectedPast["head-chunk"] {
		t.Fatal("head chunk must remain protected after clock rollback")
	}
	for i := 1; i <= 10; i++ {
		chunkID := fmt.Sprintf("hist-chunk-%d", i)
		if !protectedPast[chunkID] {
			t.Fatalf("%s must remain protected after clock rollback", chunkID)
		}
	}
}

func TestReferenceSetOracleCrashRecovery(t *testing.T) {
	now := time.Now()
	policy := RetentionPolicy{RetentionDays: 30, MinSuperseded: 20}

	t.Run("crash-after-intent", func(t *testing.T) {
		oracle := NewReferenceSetOracle()
		oracle.InstallObject("unreferenced", true)
		_, _ = oracle.BeginGC([]string{"unreferenced"})

		// Crash before unlink: recover retains extra bytes safely
		oracle.Recover(now, policy)
		if oracle.ObjectState("unreferenced") != ObjectPresent {
			t.Fatalf("crash after intent did not retain object: state=%v", oracle.ObjectState("unreferenced"))
		}
	})

	t.Run("crash-after-unlink", func(t *testing.T) {
		oracle := NewReferenceSetOracle()
		oracle.InstallObject("unreferenced", true)
		_, _ = oracle.BeginGC([]string{"unreferenced"})
		oracle.Unlink("unreferenced")

		// Crash before finalize: recover finalizes unreferenced unlinked object
		oracle.Recover(now, policy)
		if oracle.ObjectState("unreferenced") != ObjectFinalized {
			t.Fatalf("crash after unlink did not finalize unreferenced object: state=%v", oracle.ObjectState("unreferenced"))
		}
	})
}

func TestReferenceSetOracleCorruptionAndRepair(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	policy := RetentionPolicy{RetentionDays: 30, MinSuperseded: 0}

	t.Run("shared-chunk-corruption-diagnoses-all-versions", func(t *testing.T) {
		oracle := NewReferenceSetOracle()
		oracle.InstallObject("shared-chunk", true)
		oracle.InstallObject("v1-chunk", true)
		oracle.InstallObject("v2-chunk", true)

		oracle.AddVersion(VersionRecord{
			ID:       "v1",
			Path:     "file1.txt",
			Chunks:   []string{"shared-chunk", "v1-chunk"},
			Acquired: now,
		})
		oracle.AddVersion(VersionRecord{
			ID:       "v2",
			Path:     "file2.txt",
			Chunks:   []string{"shared-chunk", "v2-chunk"},
			Acquired: now,
		})
		oracle.SetHeads("file1.txt", []string{"v1"})
		oracle.SetHeads("file2.txt", []string{"v2"})

		// Initial state: both ready
		if st := oracle.VersionAvailability("v1", now, policy); st != StatusReady {
			t.Fatalf("expected v1 ready, got %v", st)
		}
		if st := oracle.VersionAvailability("v2", now, policy); st != StatusReady {
			t.Fatalf("expected v2 ready, got %v", st)
		}

		// Injected corruption of the shared chunk
		affected := oracle.CorruptChunk("shared-chunk", "bit-flip")
		if len(affected) != 2 || affected[0] != "v1" || affected[1] != "v2" {
			t.Fatalf("expected [v1, v2] affected, got %v", affected)
		}

		// Move to quarantine
		qAffected := oracle.QuarantineChunk("shared-chunk", "bit-flip")
		if len(qAffected) != 2 {
			t.Fatalf("expected [v1, v2] quarantined, got %v", qAffected)
		}

		// Both versions now report corrupt
		if st := oracle.VersionAvailability("v1", now, policy); st != StatusCorrupt {
			t.Fatalf("expected v1 corrupt, got %v", st)
		}
		if st := oracle.VersionAvailability("v2", now, policy); st != StatusCorrupt {
			t.Fatalf("expected v2 corrupt, got %v", st)
		}

		// Attempt repair when no peer has valid bytes: fails unrecoverable
		err := oracle.RepairChunk("shared-chunk", false)
		if err == nil {
			t.Fatal("expected repair to fail when no replica has valid bytes")
		}
		if st := oracle.VersionAvailability("v1", now, policy); st != StatusCorrupt {
			t.Fatalf("expected v1 to remain corrupt after failed repair, got %v", st)
		}

		// Authorized repair from a peer with verified bytes
		if err := oracle.RepairChunk("shared-chunk", true); err != nil {
			t.Fatalf("repair with verified bytes failed: %v", err)
		}

		// Both versions restored to ready
		if st := oracle.VersionAvailability("v1", now, policy); st != StatusReady {
			t.Fatalf("expected v1 restored to ready, got %v", st)
		}
		if st := oracle.VersionAvailability("v2", now, policy); st != StatusReady {
			t.Fatalf("expected v2 restored to ready, got %v", st)
		}
	})

	t.Run("pins-during-repair-protect-from-gc", func(t *testing.T) {
		oracle := NewReferenceSetOracle()
		oldTime := now.Add(-60 * 24 * time.Hour) // beyond retention policy
		oracle.InstallObject("repairing-chunk", true)
		oracle.AddVersion(VersionRecord{
			ID:       "old-v",
			Path:     "old.txt",
			Chunks:   []string{"repairing-chunk"},
			Acquired: oldTime,
		})
		// Not a head, not fallback, beyond retention days: ordinarily eligible for GC
		candidates := oracle.ComputeCandidates(now, policy)
		found := false
		for _, c := range candidates {
			if c == "repairing-chunk" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected old chunk to be candidate for GC before pin")
		}

		// Pin during repair/checking
		oracle.Pin("repairing-chunk", "repair", "repair-op-42")

		// Recompute candidates: pinned chunk is PROTECTED from GC
		candidatesAfterPin := oracle.ComputeCandidates(now, policy)
		for _, c := range candidatesAfterPin {
			if c == "repairing-chunk" {
				t.Fatal("pinned chunk must NOT be a GC candidate during repair")
			}
		}

		// Attempting GC on pinned chunk acquires no intent
		acquired, _ := oracle.BeginGC([]string{"repairing-chunk"})
		if len(acquired) != 0 {
			t.Fatalf("expected 0 acquired intents on pinned chunk, got %v", acquired)
		}
		if oracle.Unlink("repairing-chunk") {
			t.Fatal("unlink must be blocked by active pin")
		}

		// Unpin after repair completes
		oracle.Unpin("repairing-chunk", "repair", "repair-op-42")
		candidatesAfterUnpin := oracle.ComputeCandidates(now, policy)
		foundAfter := false
		for _, c := range candidatesAfterUnpin {
			if c == "repairing-chunk" {
				foundAfter = true
				break
			}
		}
		if !foundAfter {
			t.Fatal("chunk should become candidate again after unpin")
		}
	})

	t.Run("distinguish-all-availability-states", func(t *testing.T) {
		oracle := NewReferenceSetOracle()
		oldTime := now.Add(-60 * 24 * time.Hour)

		// 1. Ready version
		oracle.InstallObject("ch-ready", true)
		oracle.AddVersion(VersionRecord{ID: "v-ready", Path: "ready.txt", Chunks: []string{"ch-ready"}, Acquired: now})
		oracle.SetHeads("ready.txt", []string{"v-ready"})
		if st := oracle.VersionAvailability("v-ready", now, policy); st != StatusReady {
			t.Fatalf("expected StatusReady, got %v", st)
		}

		// 2. Corrupt version
		oracle.InstallObject("ch-corrupt", true)
		oracle.AddVersion(VersionRecord{ID: "v-corrupt", Path: "corrupt.txt", Chunks: []string{"ch-corrupt"}, Acquired: now})
		oracle.QuarantineChunk("ch-corrupt", "hash-mismatch")
		if st := oracle.VersionAvailability("v-corrupt", now, policy); st != StatusCorrupt {
			t.Fatalf("expected StatusCorrupt, got %v", st)
		}

		// 3. Expired historical version (missing chunk, beyond retention, not head)
		oracle.InstallObject("ch-expired", true)
		oracle.AddVersion(VersionRecord{ID: "v-expired", Path: "expired.txt", Chunks: []string{"ch-expired"}, Acquired: oldTime})
		// simulate GC unlink
		_, _ = oracle.BeginGC([]string{"ch-expired"})
		oracle.Unlink("ch-expired")
		if st := oracle.VersionAvailability("v-expired", now, policy); st != StatusExpired {
			t.Fatalf("expected StatusExpired, got %v", st)
		}

		// 4. Missing protected version (missing chunk, but is a current head!)
		oracle.InstallObject("ch-missing-prot", true)
		oracle.AddVersion(VersionRecord{ID: "v-missing-prot", Path: "missing.txt", Chunks: []string{"ch-missing-prot"}, Acquired: now})
		oracle.SetHeads("missing.txt", []string{"v-missing-prot"})
		// simulate unexpected loss of object file
		delete(oracle.objects, "ch-missing-prot")
		if st := oracle.VersionAvailability("v-missing-prot", now, policy); st != StatusMissingProtected {
			t.Fatalf("expected StatusMissingProtected, got %v", st)
		}
	})
}
