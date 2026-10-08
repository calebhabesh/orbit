package control

import (
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
)

func TestAllFifteenStableErrorCategories(t *testing.T) {
	categories := []*ControlError{
		UnauthorizedError(""),
		MembershipMismatchError(""),
		IncompatibleVersionError(""),
		InvalidManifestError(""),
		InvalidPathError("test/path", "too long"),
		StaleViewError([]history.VersionID{}, history.Digest{}),
		RootUnavailableError("/mnt/data", "not mounted"),
		StructuralConflictError("dir/file", "collision"),
		ContentPendingError(""),
		ContentExpiredError(""),
		ContentUnavailableError(""),
		DiskBudgetError(""),
		IOError(""),
		UnstableFileError("foo.txt", "mtime changed"),
		RetryExhaustedError("task-1", "connection refused"),
	}

	expectedCodes := map[string]bool{
		"UNAUTHORIZED":         true,
		"MEMBERSHIP_MISMATCH":  true,
		"INCOMPATIBLE_VERSION": true,
		"INVALID_MANIFEST":     true,
		"INVALID_PATH":         true,
		"STALE_VIEW":           true,
		"ROOT_UNAVAILABLE":     true,
		"STRUCTURAL_CONFLICT":  true,
		"CONTENT_PENDING":      true,
		"CONTENT_EXPIRED":      true,
		"CONTENT_UNAVAILABLE":  true,
		"DISK_BUDGET":          true,
		"IO_ERROR":             true,
		"UNSTABLE_FILE":        true,
		"RETRY_EXHAUSTED":      true,
	}

	seenCodes := make(map[string]bool)

	for _, ce := range categories {
		if ce.Code == "" {
			t.Errorf("empty error code for %+v", ce)
		}
		if ce.Message == "" {
			t.Errorf("empty message for code %s", ce.Code)
		}
		if ce.Action == "" {
			t.Errorf("empty action for code %s", ce.Code)
		}
		if !expectedCodes[ce.Code] {
			t.Errorf("unexpected error code: %s", ce.Code)
		}
		if seenCodes[ce.Code] {
			t.Errorf("duplicate error category tested: %s", ce.Code)
		}
		seenCodes[ce.Code] = true
	}

	if len(seenCodes) != 15 {
		t.Fatalf("expected 15 stable error categories, got %d", len(seenCodes))
	}
}
