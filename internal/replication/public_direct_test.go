package replication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/rendezvous"
)

func TestWANW08IsolatedPublicTCPAndIPv6(t *testing.T) {
	if os.Getenv("ORBIT_W08_PUBLIC_FIXTURE") != "isolated-marked-namespace" {
		t.Skip("requires scripts/wan_direct_namespace_test.py; public addresses are simulated only inside an isolated namespace")
	}
	f, _ := routedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interfaces, err := network.SelectedInterfaces([]string{"orbit-w08"})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := network.OptionalDirectListener(network.DirectSettings{})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := NewServer(f.senderRepo, f.senderID)
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	defer func() { cancel(); <-done }()
	candidates := network.GatherCandidates(interfaces, listener.Addr(), true)
	if len(candidates) != 2 {
		t.Fatal("actual public IPv4/IPv6 gathering", candidates)
	}
	// Production authenticated directory publishes the actual bound TCP ports;
	// the service's observed source address/port is not an invented route.
	serviceListener, err := net.Listen("tcp4", "11.23.45.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serviceListener.Close()
	origin := "https://" + serviceListener.Addr().String()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "W08 isolated fixture service"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("11.23.45.1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	profile := p.NetworkProfile{Version: "1", Operator: "Isolated simulated public fixture", Authority: hex.EncodeToString(pub), ServiceKey: hex.EncodeToString(pub), Epoch: 1, Expires: p.NetworkUint(now.Unix() + 3600), Origins: []string{origin}, STUN: []string{}, Privacy: "Disposable isolated namespace only; no retained metadata or public network."}
	canonical, err := profile.Canonical(false)
	if err != nil {
		t.Fatal(err)
	}
	profile.Signature = hex.EncodeToString(ed25519.Sign(key, canonical))
	selection := network.ProfileSelection{Profile: profile, Authority: profile.Authority, HighestEpoch: 1, Environment: "development"}
	service, err := rendezvous.New(rendezvous.Options{Selection: selection, Origin: origin, ServiceKey: key})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	httpServer := service.Server()
	httpServer.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	sd := make(chan error, 1)
	go func() { sd <- httpServer.ServeTLS(serviceListener, "", "") }()
	defer func() { httpServer.Close(); <-sd }()
	announcer, err := network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: hex.EncodeToString(f.senderID.DeviceID[:]), Certificate: f.senderID.Certificate, Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	defer announcer.Close()
	lookup, err := network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: hex.EncodeToString(f.receiverID.DeviceID[:]), Certificate: f.receiverID.Certificate, Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	defer lookup.Close()
	for i, candidate := range candidates {
		t.Run(candidate.Address, func(t *testing.T) {
			// Exercise each actual address family separately, using the real production
			// signed lookup and pinned transport rather than remapping a dial adapter.
			a := p.NetworkAnnouncement{Generation: p.NetworkUint(i + 1), Expires: p.NetworkUint(now.Unix() + 600), Candidates: []p.NetworkCandidate{candidate}, Capabilities: []string{"direct_https_v1"}, Relay: false}
			operation, err := enrollmentRandom()
			if err != nil {
				t.Fatal(err)
			}
			if err := announcer.Announce(ctx, "peer_data", operation, a); err != nil {
				t.Fatal(err)
			}
			manager := network.NewManager(network.ManagerOptions{})
			defer manager.Close()
			target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
			if err := manager.RegisterDirect(target, func(work context.Context, t network.Target, minimum uint64) (p.NetworkAnnouncement, error) {
				out, found, e := lookup.Lookup(work, hex.EncodeToString(t.Device[:]), hex.EncodeToString(t.Pin[:]), "peer_data", minimum)
				if e == nil && !found {
					e = network.ErrNoRoute
				}
				return out, e
			}); err != nil {
				t.Fatal(err)
			}
			client, e := NewRoutedClient(ctx, network.LogicalOrigin(target), f.receiverID, f.senderID.Leaf, target, manager)
			if e != nil {
				t.Fatal(e)
			}
			f.client = client
			name := []string{"public-v4.txt", "public-v6.txt"}[i]
			want := []byte("actual TCP family / signed directory / pinned peer / verified engine: " + name)
			if e = os.WriteFile(filepath.Join(f.senderRoot, name), want, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e = f.senderWork.Scan(ctx, f.folder); e != nil {
				t.Fatal(e)
			}
			if _, e = f.newSyncer(TransferOptions{}).Sync(ctx); e != nil {
				t.Fatal(e)
			}
			got, e := os.ReadFile(filepath.Join(f.receiverRoot, name))
			if e != nil || !bytes.Equal(got, want) {
				t.Fatal(e)
			}
			heads, e := f.receiverRepo.Heads(ctx, f.folder, name)
			if e != nil || len(heads) != 1 || heads[0].ID.Author != f.senderID.DeviceID || f.receiverRepo.VerifyManifest(heads[0].Manifest) != nil || manager.Observe(target).Route != "direct" {
				t.Fatal("public fixture route/version/hash", e)
			}
			// Stop only the disposable service after the IPv6 transfer; a valid direct
			// pool and captured edit continue without lookup or service sockets.
			if i == 1 {
				service.Close()
				httpServer.Close()
				want = []byte("existing pinned public fixture pool survives actual directory outage")
				if e = os.WriteFile(filepath.Join(f.senderRoot, name), want, 0600); e != nil {
					t.Fatal(e)
				}
				if _, e = f.senderWork.Scan(ctx, f.folder); e != nil {
					t.Fatal(e)
				}
				if _, e = f.newSyncer(TransferOptions{}).Sync(ctx); e != nil {
					t.Fatal(e)
				}
				got, e = os.ReadFile(filepath.Join(f.receiverRoot, name))
				if e != nil || !bytes.Equal(got, want) {
					t.Fatal(e)
				}
			}
		})
	}
	t.Log("Actual public-scope numeric IPv4/IPv6 TCP, signed production directory and pinned TLS/engine transfer in a new isolated network namespace. Simulated addresses, no internet/native public reachability claim.")
}
