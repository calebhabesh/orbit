package replication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/pion/logging"
	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/vnet"
)

func TestWANW10AuthenticatedNATPeerMatrix(t *testing.T) {
	cases := []struct {
		name                             string
		mapping, filtering               vnet.EndpointDependencyType
		noNAT, blocked, double, fallback bool
	}{
		{name: "no NAT", noNAT: true},
		{name: "endpoint independent"},
		{name: "address dependent filtering", filtering: vnet.EndpointAddrDependent},
		{name: "address port dependent filtering", filtering: vnet.EndpointAddrPortDependent},
		{name: "incompatible mappings", mapping: vnet.EndpointAddrPortDependent, filtering: vnet.EndpointAddrPortDependent, fallback: true},
		{name: "incompatible double NAT", mapping: vnet.EndpointAddrPortDependent, filtering: vnet.EndpointAddrPortDependent, double: true, fallback: true},
		{name: "blocked peer UDP", blocked: true, fallback: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if e := os.WriteFile(filepath.Join(root, testkit.Marker), []byte("W10 in-process disposable NAT emulator only\n"), 0600); e != nil {
				t.Fatal(e)
			}
			logger := logging.NewDefaultLoggerFactory()
			wan, e := vnet.NewRouter(&vnet.RouterConfig{CIDR: "0.0.0.0/0", LoggerFactory: logger})
			if e != nil {
				t.Fatal(e)
			}
			serverNet, e := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{"11.1.1.1"}})
			if e != nil {
				t.Fatal(e)
			}
			if e = wan.AddNet(serverNet); e != nil {
				t.Fatal(e)
			}
			nets := make([]transport.Net, 2)
			for n := range 2 {
				public := fmt.Sprintf("11.2.%d.1", n+1)
				local := public
				if !c.noNAT {
					local = fmt.Sprintf("10.%d.0.2", n+1)
				}
				stack, err := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{local}})
				if err != nil {
					t.Fatal(err)
				}
				nets[n] = stack
				if c.noNAT {
					if err = wan.AddNet(stack); err != nil {
						t.Fatal(err)
					}
					continue
				}
				nat := &vnet.NATType{MappingBehavior: c.mapping, FilteringBehavior: c.filtering}
				outer, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: fmt.Sprintf("10.%d.0.0/24", n+1), StaticIPs: []string{public}, NATType: nat, LoggerFactory: logger})
				if err != nil {
					t.Fatal(err)
				}
				if c.double {
					outer, err = vnet.NewRouter(&vnet.RouterConfig{CIDR: fmt.Sprintf("172.16.%d.0/24", n+1), StaticIPs: []string{public}, NATType: nat, LoggerFactory: logger})
					if err != nil {
						t.Fatal(err)
					}
					inner, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: fmt.Sprintf("10.%d.0.0/24", n+1), StaticIPs: []string{fmt.Sprintf("172.16.%d.2", n+1)}, NATType: &vnet.NATType{MappingBehavior: vnet.EndpointIndependent, FilteringBehavior: vnet.EndpointIndependent}, LoggerFactory: logger})
					if err != nil {
						t.Fatal(err)
					}
					if err = inner.AddNet(stack); err != nil {
						t.Fatal(err)
					}
					if err = outer.AddRouter(inner); err != nil {
						t.Fatal(err)
					}
				} else if err = outer.AddNet(stack); err != nil {
					t.Fatal(err)
				}
				if err = wan.AddRouter(outer); err != nil {
					t.Fatal(err)
				}
			}
			var outage atomic.Bool
			outage.Store(c.blocked)
			wan.AddChunkFilter(func(packet vnet.Chunk) bool {
				if !outage.Load() {
					return true
				}
				return strings.HasPrefix(packet.SourceAddr().String(), "11.1.1.1:") || strings.HasPrefix(packet.DestinationAddr().String(), "11.1.1.1:")
			})
			if e = wan.Start(); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = wan.Stop() })
			socket, e := serverNet.ListenPacket("udp", "11.1.1.1:3478")
			if e != nil {
				t.Fatal(e)
			}
			server, e := network.NewSTUNServer(socket)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = server.Close() })
			t.Logf("simulated NAT mapping=%d filtering=%d noNAT=%v double=%v blocked=%v; public 11.2.{1,2}.1 private 10.{1,2}.0.2; inner double 172.16.{1,2}.2; real HTTPS/WSS service uses native private interface", c.mapping, c.filtering, c.noNAT, c.double, c.blocked)
			fixture := verifyICEPeerSync(t, c.fallback, nets...)
			if c.name == "endpoint independent" {
				old := make([]*network.QUICEndpoint, 2)
				before, e := fixture.f.senderRepo.Heads(fixture.f.ctx, fixture.f.folder, "resume.bin")
				if e != nil {
					t.Fatal(e)
				}
				oracle, _ := json.Marshal(before)
				for n, r := range fixture.runtimes {
					old[n], _, e = r.ICE(fixture.f.ctx, fixture.peers[n])
					if e != nil {
						t.Fatal(e)
					}
				}
				start := time.Now()
				outage.Store(true)
				for _, endpoint := range old {
					select {
					case <-endpoint.Done():
					case <-time.After(12 * time.Second):
						t.Fatal("ICE consent loss did not retire QUIC")
					}
				}
				outage.Store(false)
				var wg sync.WaitGroup
				results := make(chan error, 2)
				for n, r := range fixture.runtimes {
					wg.Go(func() {
						replacement, _, err := r.ICE(fixture.f.ctx, fixture.peers[n])
						if err == nil && replacement == old[n] {
							err = fmt.Errorf("reused retired pair")
						}
						results <- err
					})
				}
				wg.Wait()
				close(results)
				for err := range results {
					if err != nil {
						t.Fatal("ICE pair rebuild", err)
					}
				}
				if _, e = fixture.f.newSyncer(TransferOptions{}).Sync(fixture.f.ctx); e != nil {
					t.Fatal("sync after pair rebuild", e)
				}
				after, e := fixture.f.receiverRepo.Heads(fixture.f.ctx, fixture.f.folder, "resume.bin")
				if e != nil {
					t.Fatal(e)
				}
				actual, _ := json.Marshal(after)
				if !bytes.Equal(oracle, actual) {
					t.Fatal("route replacement changed heads")
				}
				t.Log("actual ICE consent-loss closure and fresh-pair rebuild; elapsed", time.Since(start), "unchanged heads/hash/author")
				_ = fixture.service.Close()
				deadline := time.Now().Add(2 * time.Second)
				for (fixture.runtimes[0].Ready() || fixture.runtimes[1].Ready()) && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if fixture.runtimes[0].Ready() || fixture.runtimes[1].Ready() {
					t.Fatal("directory outage not observed")
				}
				outageBytes := bytes.Repeat([]byte("captured with directory offline\n"), 4000)
				if e = os.WriteFile(filepath.Join(fixture.f.senderRoot, "directory-offline.bin"), outageBytes, 0600); e != nil {
					t.Fatal(e)
				}
				if _, e = fixture.f.senderWork.Scan(fixture.f.ctx, fixture.f.folder); e != nil {
					t.Fatal(e)
				}
				if _, e = fixture.f.newSyncer(TransferOptions{}).Sync(fixture.f.ctx); e != nil {
					t.Fatal("directory outage broke authenticated ICE", e)
				}
				actualBytes, e := os.ReadFile(filepath.Join(fixture.f.receiverRoot, "directory-offline.bin"))
				if e != nil || !bytes.Equal(actualBytes, outageBytes) {
					t.Fatal("directory outage bytes", e)
				}
				if fixture.managers[1].Observe(fixture.peers[1]).Route != "quic" {
					t.Fatal("directory outage changed direct route")
				}
				t.Log("actual directory/control shutdown preserves existing authenticated ICE peer sync")
			}
		})
	}
}
