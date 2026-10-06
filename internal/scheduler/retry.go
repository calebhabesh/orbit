package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

const (
	MaxRetryAttempts = 5
	BaseRetryDelay   = 50 * time.Millisecond
	MaxRetryDelay    = 5 * time.Second
)

type RetryClassifier struct{}

func (rc RetryClassifier) IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var service *network.ServiceError
	if errors.As(err, &service) {
		switch service.Code {
		case p.NetworkQuota, p.NetworkUnavailable, p.NetworkServiceUnavailable, p.NetworkStaleGeneration:
			return true
		default:
			return false
		}
	}
	var wire *replication.WireError
	if errors.As(err, &wire) {
		return wire.Body.Retryable || wire.Body.Code == "MEMBERSHIP_MISMATCH" || wire.Body.Code == "UNAUTHORIZED"
	}
	if errors.Is(err, network.ErrBackpressure) || errors.Is(err, network.ErrStale) {
		return true
	}
	if errors.Is(err, repository.ErrMembershipMismatch) {
		return true
	}
	if errors.Is(err, workspace.ErrRootUnavailable) {
		return false
	}
	if errors.Is(err, workspace.ErrStructuralConflict) {
		return false
	}
	if errors.Is(err, workspace.ErrUnsupportedEntry) {
		return false
	}
	if errors.Is(err, workspace.ErrUnstableFile) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var sysErr syscall.Errno
	if errors.As(err, &sysErr) {
		switch sysErr {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT, syscall.EPIPE, syscall.EAGAIN:
			return true
		}
	}
	return false
}

func (rc RetryClassifier) ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, network.ErrBackpressure) {
		return "NETWORK_BUSY"
	}
	if errors.Is(err, network.ErrStale) {
		return "STALE_NETWORK_GENERATION"
	}
	var service *network.ServiceError
	if errors.As(err, &service) {
		return service.Code
	}
	var wire *replication.WireError
	if errors.As(err, &wire) {
		return wire.Body.Code
	}
	if errors.Is(err, repository.ErrMembershipFork) {
		return "MEMBERSHIP_FORK"
	}
	if errors.Is(err, repository.ErrMembershipMismatch) {
		return "MEMBERSHIP_MISMATCH"
	}
	if errors.Is(err, workspace.ErrRootUnavailable) {
		return "ROOT_UNAVAILABLE"
	}
	if errors.Is(err, workspace.ErrUnstableFile) {
		return "UNSTABLE_FILE"
	}
	if errors.Is(err, workspace.ErrStructuralConflict) {
		return "STRUCTURAL_CONFLICT"
	}
	if errors.Is(err, workspace.ErrUnsupportedEntry) {
		return "INVALID_PATH"
	}
	return "IO_ERROR"
}

func (rc RetryClassifier) Backoff(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := BaseRetryDelay * time.Duration(1<<uint(attempt-1))
	if delay > MaxRetryDelay {
		delay = MaxRetryDelay
	}

	// Add up to 25% jitter
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	rnd := binary.LittleEndian.Uint64(raw[:])
	jitterMax := int64(delay / 4)
	if jitterMax > 0 {
		jitter := time.Duration(rnd % uint64(jitterMax))
		delay += jitter
	}

	return delay
}

// BackoffFor leaves the existing task/operation identity intact while allowing
// the service metadata bucket to refill before another network attempt.
func (rc RetryClassifier) BackoffFor(err error, attempt int) time.Duration {
	delay := rc.Backoff(attempt)
	var service *network.ServiceError
	if errors.As(err, &service) && service.Code == p.NetworkQuota && delay < network.ServiceRetryQuietPeriod {
		return network.ServiceRetryQuietPeriod
	}
	return delay
}
