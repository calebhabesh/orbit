package network

import (
	"context"
	"encoding/hex"
	p "github.com/calebhabesh/orbit/internal/protocol"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWANW11CompetingPeersSlowDNSRelayAndCancellation(t *testing.T) {
	for _, phase := range []string{"dns", "relay"} {
		t.Run(phase, func(t *testing.T) {
			resolver := &testResolver{block: true}
			service := clientFixture(t, resolver)
			_, trust, base := managerServer(t, func(http.ResponseWriter, *http.Request) { t.Error("failed route received HTTP") })
			manager := NewManager(ManagerOptions{})
			defer manager.Close()
			if err := manager.ConfigureTiming(p.RouteTiming{HeadStartMS: 250}); err != nil {
				t.Fatal(err)
			}
			var active, peak atomic.Int64
			entered := make(chan struct{}, 64)
			var workers sync.WaitGroup
			work, cancel := context.WithCancel(context.Background())
			defer cancel()
			for i := range 32 {
				target := base
				target.Device[0] = byte(i + 1)
				target.Profile[0] = 1
				if err := manager.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) {
					n := active.Add(1)
					defer active.Add(-1)
					for {
						old := peak.Load()
						if n <= old || peak.CompareAndSwap(old, n) {
							break
						}
					}
					entered <- struct{}{}
					<-ctx.Done()
					return nil, ctx.Err()
				}); err != nil {
					t.Fatal(err)
				}
				var source candidateSource
				if phase == "dns" {
					source = func(ctx context.Context, target Target, min uint64) (p.NetworkAnnouncement, error) {
						a, _, err := service.Lookup(ctx, hex.EncodeToString(target.Device[:]), hex.EncodeToString(target.Pin[:]), string(target.Purpose), min)
						return a, err
					}
				}
				if err := manager.RegisterDirect(target, source); err != nil {
					t.Fatal(err)
				}
				rt, err := manager.Transport(work, target, trust)
				if err != nil {
					t.Fatal(err)
				}
				workers.Go(func() {
					req, _ := http.NewRequestWithContext(work, "GET", LogicalOrigin(target), nil)
					response, err := rt.RoundTrip(req)
					if err == nil {
						if response != nil {
							response.Body.Close()
						}
						t.Error("failed routes reported success")
					}
				})
			}
			until := time.Now().Add(2 * time.Second)
			for time.Now().Before(until) {
				if phase == "dns" && resolver.peak.Load() > 0 {
					break
				}
				if phase == "relay" && peak.Load() > 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			start := time.Now()
			cancel()
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}
			if err := service.Close(); err != nil {
				t.Fatal(err)
			}
			workers.Wait()
			if time.Since(start) > time.Second || peak.Load() > MaxActivePeerSlots || resolver.peak.Load() > 4 || active.Load() != 0 || resolver.active.Load() != 0 || len(manager.requests) != 0 || len(manager.pools) != 0 || manager.activeDials != 0 {
				t.Fatal("cancel bounds", peak.Load(), resolver.peak.Load(), active.Load(), resolver.active.Load(), time.Since(start))
			}
			t.Logf("32 competing peers phase=%s DNS peak=%d relay attempts peak=%d; joined cancellation=%s; active requests/pools/dials/resolvers/relay=0", phase, resolver.peak.Load(), peak.Load(), time.Since(start))
		})
	}
}
