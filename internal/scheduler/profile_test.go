package scheduler_test

import (
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/scheduler"
)

func TestHardwareProfiles(t *testing.T) {
	laptop := scheduler.GetProfile(scheduler.ProfileLaptop)
	if laptop.TransferWorkers != 4 || laptop.HashWorkers != 2 {
		t.Fatalf("unexpected laptop profile: %+v", laptop)
	}
	if err := scheduler.ValidateProfile(laptop); err != nil {
		t.Fatalf("laptop profile failed validation: %v", err)
	}

	pi := scheduler.GetProfile(scheduler.ProfilePi)
	if pi.TransferWorkers != 2 || pi.HashWorkers != 1 {
		t.Fatalf("unexpected pi profile: %+v", pi)
	}
	if err := scheduler.ValidateProfile(pi); err != nil {
		t.Fatalf("pi profile failed validation: %v", err)
	}

	invalid := laptop
	invalid.TransferWorkers = 0
	if err := scheduler.ValidateProfile(invalid); err == nil {
		t.Fatal("expected error for 0 transfer workers")
	}

	invalid = laptop
	invalid.ReconcileInterval = -1 * time.Minute
	if err := scheduler.ValidateProfile(invalid); err == nil {
		t.Fatal("expected error for negative reconcile interval")
	}
}
