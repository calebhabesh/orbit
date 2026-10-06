package scheduler

import (
	"fmt"
	p "github.com/calebhabesh/file-sync/internal/protocol"
	"testing"

	"github.com/calebhabesh/file-sync/internal/network"
)

func TestWANW02NetworkBackpressureUsesExistingRetryBudget(t *testing.T) {
	c := RetryClassifier{}
	for _, e := range []error{network.ErrBackpressure, network.ErrStale} {
		if !c.IsTransient(e) || c.ErrorCode(e) == "IO_ERROR" {
			t.Fatal(e)
		}
	}
	if c.IsTransient(network.ErrClosed) {
		t.Fatal("closed manager kept retrying")
	}
}

func TestWANW04TypedRelayFailuresUseExistingRetryBudget(t *testing.T) {
	c := RetryClassifier{}
	for _, code := range []string{p.NetworkQuota, p.NetworkUnavailable, p.NetworkServiceUnavailable, p.NetworkStaleGeneration} {
		err := fmt.Errorf("route: %w", &network.ServiceError{Code: code})
		if !c.IsTransient(err) || c.ErrorCode(err) != code {
			t.Fatal(code, err)
		}
	}
	for _, code := range []string{p.NetworkIdentityMismatch, p.NetworkPurposeMismatch, p.NetworkProfileExpired, p.NetworkProfileUntrusted, p.NetworkReplay} {
		err := &network.ServiceError{Code: code}
		if c.IsTransient(err) || c.ErrorCode(err) != code {
			t.Fatal("unsafe retry", code)
		}
	}
}
