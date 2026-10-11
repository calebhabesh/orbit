package rendezvous

import (
	"context"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
)

// A device whose clock differs from the service's by a few seconds is still
// served: its announcements, challenges and control channel are accepted in
// both directions of skew.
func TestRuntimeToleratesServiceClockSkew(t *testing.T) {
	for _, skew := range []time.Duration{-10 * time.Second, 10 * time.Second} {
		t.Run("service"+skew.String(), func(t *testing.T) {
			f := newFixture(t, true)
			stop := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(50 * time.Millisecond)
				defer ticker.Stop()
				for {
					f.now.Store(time.Now().Add(skew).Unix())
					select {
					case <-stop:
						return
					case <-ticker.C:
					}
				}
			}()
			t.Cleanup(func() { close(stop); <-done })
			d := identity(t, 1, nil)
			c, err := network.NewServiceClient(network.ServiceClientOptions{Selection: f.selection, Origin: f.origin, Device: d.id, Certificate: d.cert, Roots: f.roots, Now: time.Now})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = c.Close() })
			m := network.NewManager(network.ManagerOptions{})
			t.Cleanup(func() { _ = m.Close() })
			r, err := network.NewRelayRuntime(context.Background(), c, m, 1, f.s.digest)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), readyDeadline)
			defer cancel()
			if err := r.WaitReady(ctx); err != nil {
				t.Fatal("device with a skewed clock was not served:", err)
			}
		})
	}
}
