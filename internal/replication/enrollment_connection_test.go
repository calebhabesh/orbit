package replication

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

// Trial finding: a join showed only "enrollment connection or TLS identity
// verification failed". The cause is now a safe category, and only an
// identity failure stops retrying.
func TestOnboardingTrialEnrollmentConnectionCause(t *testing.T) {
	wrap := func(err error) error { return &url.Error{Op: "Post", URL: "https://peer.orbit.invalid/x", Err: err} }
	for _, c := range []struct {
		err       error
		cause     string
		retryable bool
	}{
		{wrap(&network.ServiceError{Code: p.NetworkQuota}), p.NetworkQuota, true},
		{wrap(fmt.Errorf("dial: %w", context.DeadlineExceeded)), "TIMEOUT", true},
		{wrap(fmt.Errorf("remote error: %w", ErrPeerPinMismatch)), "IDENTITY_MISMATCH", false},
		{wrap(errors.New("connect: connection refused 192.0.2.7:4000")), "UNREACHABLE", true},
	} {
		e := &EnrollmentConnectionError{Cause: connectionCause(c.err)}
		if e.Cause != c.cause || e.Retryable() != c.retryable {
			t.Errorf("%v: cause %q retryable %v", c.err, e.Cause, e.Retryable())
		}
		if strings.Contains(e.Error(), "192.0.2.7") || strings.Contains(e.Error(), "peer.orbit.invalid") {
			t.Errorf("message leaks detail: %s", e.Error())
		}
	}
}
