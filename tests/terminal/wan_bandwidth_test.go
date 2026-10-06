package terminal_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/scheduler"
)

func TestWANW11ActualLargeSmallPeerBandwidthAcrossRoutes(t *testing.T) {
	_, selection, origin, roots, _ := w05Service(t)
	a, b, c := newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots)
	enroll := func(from *w05Node, name string) (history.ID, string, string) {
		folder := w05Create(t, from, name)
		inv := w05Invite(t, from, folder, "")
		join := w05Join(t, b, inv, "join-"+name)
		pending, err := w05MutateRetry(t, b, join)
		if err != nil || pending.Error != nil {
			t.Fatal(pending, err)
		}
		w05Approve(t, from, pending.Join.Request)
		time.Sleep(26 * time.Second)
		result, err := w05MutateRetry(t, b, join)
		if err != nil || result.Error != nil || !result.Readiness.Ready() {
			t.Fatal(result, err)
		}
		return folder, filepath.Join(from.f.root, name), join.Join.Root
	}
	largeFolder, largeRoot, largeDestination := enroll(a, "large-source")
	smallFolder, smallRoot, smallDestination := enroll(c, "small-source")
	interfaces, err := network.SelectedInterfaces(nil)
	if err != nil {
		t.Fatal(err)
	}
	host, iface := "", ""
	for _, i := range interfaces {
		for _, p := range i.Prefixes {
			if p.Addr().Is4() && p.Addr().IsPrivate() {
				host, iface = p.Addr().String(), i.Name
				break
			}
		}
		if host != "" {
			break
		}
	}
	if host == "" {
		t.Fatal("private native interface required")
	}
	tcpAddresses := map[*w05Node]string{}
	udpAddresses := map[*w05Node]string{}
	for _, n := range []*w05Node{a, b, c} {
		socket, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(host)})
		if e != nil {
			t.Fatal(e)
		}
		endpoint, e := n.manager.NewPeerQUICEndpoint(socket, n.id.ServerTLSConfig(), replication.NewServer(n.f.db, n.id))
		if e != nil {
			t.Fatal(e)
		}
		if e = n.manager.EnableQUIC(endpoint); e != nil {
			t.Fatal(e)
		}
		udpAddresses[n] = endpoint.LocalAddr().String()
		listener, e := net.Listen("tcp", net.JoinHostPort(host, "0"))
		if e != nil {
			t.Fatal(e)
		}
		server := replication.NewServer(n.f.db, n.id).HTTPServer()
		done := make(chan struct{})
		go func() { defer close(done); _ = server.ServeTLS(listener, "", "") }()
		t.Cleanup(func() { _ = server.Close(); <-done })
		tcpAddresses[n] = listener.Addr().String()
	}
	digest, _ := selection.Digest()
	target := func(n *w05Node) network.Target {
		return network.Target{Device: n.id.DeviceID, Pin: n.id.KeyPin, Profile: history.Digest(mustID(t, digest)), Purpose: network.PeerData}
	}
	for turn, route := range []string{"relay", "direct", "quic"} {
		t.Run(route, func(t *testing.T) {
			b.manager.NetworkChanged()
			for _, n := range []*w05Node{a, c} {
				candidates := []protocol.NetworkCandidate{}
				if route == "direct" {
					candidates = append(candidates, protocol.NetworkCandidate{Transport: "tcp", Address: tcpAddresses[n], Scope: "lan", Interface: iface})
				}
				if route == "quic" {
					candidates = append(candidates, protocol.NetworkCandidate{Transport: "udp", Address: udpAddresses[n], Scope: "lan", Interface: iface})
				}
				if e := b.manager.SetCandidates(target(n), candidates, uint64(turn+2), time.Now().Add(5*time.Minute), false); e != nil {
					t.Fatal(e)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			limiter := scheduler.NewBandwidthLimiter(2 << 20)
			clients := map[*w05Node]replication.PeerClient{}
			for _, n := range []*w05Node{a, c} {
				client, e := replication.NewRoutedClient(ctx, network.LogicalOrigin(target(n)), b.id, n.id.Leaf, target(n), b.manager)
				if e != nil {
					t.Fatal(e)
				}
				defer client.CloseIdleConnections()
				clients[n] = client
			}
			syncWithRetry := func(syncer *replication.Syncer) error {
				for attempt := 0; attempt < 6; attempt++ {
					_, err := syncer.Sync(ctx)
					if err == nil {
						return nil
					}
					if !(scheduler.RetryClassifier{}).IsTransient(err) || attempt == 5 {
						return err
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(11 * time.Second):
					}
				}
				return nil
			}
			// Establish each authenticated tunnel before timing data competition. This
			// retains the real service bucket and its typed quiet-refill retries.
			for _, item := range []struct {
				n      *w05Node
				folder history.ID
			}{{a, largeFolder}, {c, smallFolder}} {
				membership, e := b.f.db.Membership(ctx, item.folder)
				if e != nil {
					t.Fatal(e)
				}
				if e = syncWithRetry(replication.NewSyncer(b.f.db, b.f.ws, clients[item.n], b.id.DeviceID, item.n.id.DeviceID, item.folder, membership, replication.TransferOptions{Workers: 1})); e != nil {
					t.Fatal("authenticated warmup", e)
				}
			}
			payload := make([]byte, 16*int(history.ChunkSize))
			for i := range payload {
				payload[i] = byte((i + i/int(history.ChunkSize) + turn*17) % 251)
			}
			largeName := fmt.Sprintf("large-%d.bin", turn)
			if e := os.WriteFile(filepath.Join(largeRoot, largeName), payload, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := a.f.ws.Scan(ctx, largeFolder); e != nil {
				t.Fatal(e)
			}
			membershipLarge, e := b.f.db.Membership(ctx, largeFolder)
			if e != nil {
				t.Fatal(e)
			}
			membershipSmall, e := b.f.db.Membership(ctx, smallFolder)
			if e != nil {
				t.Fatal(e)
			}
			// Both authenticated peers compete for one global limiter. Tiny versions
			// continue arriving while the other peer downloads four distinct chunks.
			var firstChunk atomic.Int64
			started := time.Now()
			sampleStop, sampleDone := make(chan struct{}), make(chan struct{})
			var peakHeap, peakGoroutines, peakFD, peakSockets, peakRSS atomic.Uint64
			var cpuStart, cpuEnd syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuStart)
			go func() {
				defer close(sampleDone)
				ticker := time.NewTicker(25 * time.Millisecond)
				defer ticker.Stop()
				for {
					var mem runtime.MemStats
					runtime.ReadMemStats(&mem)
					fds, _ := os.ReadDir("/proc/self/fd")
					sockets := map[string]bool{}
					for _, fd := range fds {
						target, e := os.Readlink(filepath.Join("/proc/self/fd", fd.Name()))
						if e == nil && strings.HasPrefix(target, "socket:[") {
							sockets[target] = true
						}
					}
					if count := uint64(len(sockets)); count > peakSockets.Load() {
						peakSockets.Store(count)
					}
					status, _ := os.ReadFile("/proc/self/status")
					for _, line := range strings.Split(string(status), "\n") {
						fields := strings.Fields(line)
						if len(fields) > 1 && fields[0] == "VmRSS:" {
							rss, _ := strconv.ParseUint(fields[1], 10, 64)
							rss *= 1024
							if rss > peakRSS.Load() {
								peakRSS.Store(rss)
							}
						}
					}
					if mem.HeapAlloc > peakHeap.Load() {
						peakHeap.Store(mem.HeapAlloc)
					}
					g := uint64(runtime.NumGoroutine())
					if g > peakGoroutines.Load() {
						peakGoroutines.Store(g)
					}
					fd := uint64(len(fds))
					if fd > peakFD.Load() {
						peakFD.Store(fd)
					}
					select {
					case <-sampleStop:
						return
					case <-ticker.C:
					}
				}
			}()
			defer func() { close(sampleStop); <-sampleDone }()
			largeDone := make(chan error, 1)
			go func() {
				_, e := replication.NewSyncer(b.f.db, b.f.ws, clients[a], b.id.DeviceID, a.id.DeviceID, largeFolder, membershipLarge, replication.TransferOptions{Workers: 1, Limiter: limiter, Hook: func(name string) error {
					if name == replication.HookChunkVerified {
						firstChunk.CompareAndSwap(0, time.Now().UnixNano())
					}
					return nil
				}}).Sync(ctx)
				largeDone <- e
			}()
			smallCount := 0
			var smallFirst time.Duration
			var largeErr error
			finished := false
			for !finished {
				name := fmt.Sprintf("small-%d-%d.txt", turn, smallCount)
				want := []byte(name)
				if e := os.WriteFile(filepath.Join(smallRoot, name), want, 0600); e != nil {
					t.Fatal(e)
				}
				if _, e := c.f.ws.Scan(ctx, smallFolder); e != nil {
					t.Fatal(e)
				}
				_, e := replication.NewSyncer(b.f.db, b.f.ws, clients[c], b.id.DeviceID, c.id.DeviceID, smallFolder, membershipSmall, replication.TransferOptions{Workers: 1, Limiter: limiter}).Sync(ctx)
				if e != nil {
					t.Fatal(e)
				}
				got, e := os.ReadFile(filepath.Join(smallDestination, name))
				if e != nil || !bytes.Equal(got, want) {
					t.Fatal("small bytes", e)
				}
				smallCount++
				if smallCount == 1 {
					smallFirst = time.Since(started)
				}
				select {
				case largeErr = <-largeDone:
					finished = true
				case <-time.After(100 * time.Millisecond):
				}
			}
			if largeErr != nil {
				t.Fatal(largeErr)
			}
			elapsed := time.Since(started)
			if elapsed < time.Duration(len(payload))*time.Second/(2<<20)-time.Second {
				t.Fatal("payload bypassed global rate plus one-second initial burst", elapsed)
			}
			if firstChunk.Load() == 0 || smallCount < 2 || smallFirst >= elapsed {
				t.Fatal("competing peers did not progress", smallCount, smallFirst, elapsed)
			}
			got, e := os.ReadFile(filepath.Join(largeDestination, largeName))
			if e != nil || !bytes.Equal(got, payload) {
				t.Fatal("large bytes", e)
			}
			left, e := a.f.db.Heads(ctx, largeFolder, largeName)
			if e != nil {
				t.Fatal(e)
			}
			right, e := b.f.db.Heads(ctx, largeFolder, largeName)
			if e != nil {
				t.Fatal(e)
			}
			l, _ := json.Marshal(left)
			r, _ := json.Marshal(right)
			if !bytes.Equal(l, r) || len(left) != 1 || left[0].ID.Author != a.id.DeviceID || b.f.db.VerifyManifest(right[0].Manifest) != nil {
				t.Fatal("large version/head/hash")
			}
			for _, n := range []*w05Node{a, c} {
				if obs := b.manager.Observe(target(n)); obs.Route != route {
					t.Fatal("transport oracle", route, obs)
				}
			}
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuEnd)
			cpuUS := (cpuEnd.Utime.Sec-cpuStart.Utime.Sec+cpuEnd.Stime.Sec-cpuStart.Stime.Sec)*1000000 + cpuEnd.Utime.Usec - cpuStart.Utime.Usec + cpuEnd.Stime.Usec - cpuStart.Stime.Usec
			t.Logf("actual %s: large=%d bytes completed=%s first verified chunk=%s first small=%s small versions=%d; shared cap=2097152 B/s; peak sampled heap=%d goroutines=%d FD=%d unique sockets=%d RSS=%d process CPU=%dus; original author=%s", route, len(payload), elapsed, time.Duration(firstChunk.Load()-started.UnixNano()), smallFirst, smallCount, peakHeap.Load(), peakGoroutines.Load(), peakFD.Load(), peakSockets.Load(), peakRSS.Load(), cpuUS, hex.EncodeToString(a.id.DeviceID[:]))
		})
	}
}
