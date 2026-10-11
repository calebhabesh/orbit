package protocol

import "testing"

func TestWithinLifetimeAllowsBoundedIssuerSkew(t *testing.T) {
	const now = 1_000_000
	for _, c := range []struct {
		expires, lifetime uint64
		want              bool
	}{
		{now + 600, 600, true},
		{now + 600 + MaxClockSkew, 600, true},
		{now + 601 + MaxClockSkew, 600, false},
		{now + 1, 60, true},
		{now, 60, false},
		{now - 1, 60, false},
	} {
		if got := WithinLifetime(c.expires, now, c.lifetime); got != c.want {
			t.Errorf("WithinLifetime(now%+d, %d) = %v, want %v", int64(c.expires)-now, c.lifetime, got, c.want)
		}
	}
}
