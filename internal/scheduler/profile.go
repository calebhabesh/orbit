package scheduler

import (
	"errors"
	"fmt"
	"time"
)

type Profile string

const (
	ProfileLaptop Profile = "laptop"
	ProfilePi     Profile = "pi"
)

type ResourceProfile struct {
	Name              Profile       `json:"name"`
	TransferWorkers   int           `json:"transfer_workers"`
	HashWorkers       int           `json:"hash_workers"`
	MaxQueuedTasks    int           `json:"max_queued_tasks"`
	MaxRetries        int           `json:"max_retries"`
	ReconcileInterval time.Duration `json:"reconcile_interval"`
	FullScanInterval  time.Duration `json:"full_scan_interval"`
	DebounceWindow    time.Duration `json:"debounce_window"`
	FreeSpaceReserve  uint64        `json:"free_space_reserve"`
	MetadataBudget    uint64        `json:"metadata_budget"`
}

func GetProfile(name Profile) ResourceProfile {
	switch name {
	case ProfilePi:
		return ResourceProfile{
			Name:              ProfilePi,
			TransferWorkers:   2,
			HashWorkers:       1,
			MaxQueuedTasks:    1024,
			MaxRetries:        5,
			ReconcileInterval: 5 * time.Minute,
			FullScanInterval:  24 * time.Hour,
			DebounceWindow:    500 * time.Millisecond,
			FreeSpaceReserve:  512 * 1024 * 1024,
			MetadataBudget:    256 * 1024 * 1024,
		}
	default:
		return ResourceProfile{
			Name:              ProfileLaptop,
			TransferWorkers:   4,
			HashWorkers:       2,
			MaxQueuedTasks:    1024,
			MaxRetries:        5,
			ReconcileInterval: 5 * time.Minute,
			FullScanInterval:  24 * time.Hour,
			DebounceWindow:    200 * time.Millisecond,
			FreeSpaceReserve:  512 * 1024 * 1024,
			MetadataBudget:    256 * 1024 * 1024,
		}
	}
}

func ValidateProfile(p ResourceProfile) error {
	if p.TransferWorkers <= 0 {
		return errors.New("transfer workers must be at least 1")
	}
	if p.HashWorkers <= 0 {
		return errors.New("hash workers must be at least 1")
	}
	if p.MaxQueuedTasks <= 0 || p.MaxQueuedTasks > 4096 {
		return fmt.Errorf("max queued tasks must be between 1 and 4096, got %d", p.MaxQueuedTasks)
	}
	if p.MaxRetries <= 0 {
		return errors.New("max retries must be at least 1")
	}
	if p.ReconcileInterval <= 0 {
		return errors.New("reconcile interval must be positive")
	}
	if p.FullScanInterval <= 0 {
		return errors.New("full scan interval must be positive")
	}
	return nil
}
