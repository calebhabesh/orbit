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

func TestWANW03DirectoryLossPreservesManualCaptureAndSync(t *testing.T) {
	f, _ := routedFixture(t)
	ctx := context.Background()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var ip net.IP
	for _, a := range addrs {
		host, _, _ := net.ParseCIDR(a.String())
		if host != nil && host.To4() != nil && host.IsPrivate() && !host.IsLoopback() {
			ip = host
			break
		}
	}
	if ip == nil {
		t.Fatal("nonloopback private interface required")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	origin := "https://" + listener.Addr().String()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "disposable directory"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{ip}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	profile := p.NetworkProfile{Version: "1", Operator: "Development directory loss fixture", Authority: hex.EncodeToString(pub), ServiceKey: hex.EncodeToString(pub), Epoch: 1, Expires: p.NetworkUint(now.Unix() + 3600), Origins: []string{origin}, STUN: []string{}, Privacy: "Development only; metadata expires in memory; no logs or folder data."}
	b, _ := profile.Canonical(true)
	profile.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	selection := network.ProfileSelection{Profile: profile, Authority: profile.Authority, HighestEpoch: 1, Environment: "development"}
	service, err := rendezvous.New(rendezvous.Options{Selection: selection, Origin: origin, ServiceKey: key})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := service.Server()
	server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, NextProtos: []string{"http/1.1"}}
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(rendezvous.BoundedListener(listener), "", "") }()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	// The existing persistent device identity/certificate is reused for service
	// proof; directory presence cannot create a new identity or grant membership.
	client, err := network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: hex.EncodeToString(f.receiverID.DeviceID[:]), Certificate: f.receiverID.Certificate, Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, _, err = client.Lookup(ctx, hex.EncodeToString(f.senderID.DeviceID[:]), hex.EncodeToString(f.senderID.KeyPin[:]), "peer_data", 0); err != nil {
		t.Fatal("directory initially reachable", err)
	}
	_ = service.Close()
	_ = server.Close()
	<-done
	if _, _, err = client.Lookup(ctx, hex.EncodeToString(f.senderID.DeviceID[:]), hex.EncodeToString(f.senderID.KeyPin[:]), "peer_data", 0); err == nil {
		t.Fatal("directory outage not observed")
	}
	want := []byte("captured and transferred while the authenticated directory is offline")
	if err = os.WriteFile(filepath.Join(f.senderRoot, "directory-offline.txt"), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.senderWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	if _, err = f.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal("manual sync depended on directory", err)
	}
	got, err := os.ReadFile(filepath.Join(f.receiverRoot, "directory-offline.txt"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("outage bytes", err)
	}
	heads, err := f.receiverRepo.Heads(ctx, f.folder, "directory-offline.txt")
	if err != nil || len(heads) != 1 || f.receiverRepo.VerifyManifest(heads[0].Manifest) != nil {
		t.Fatal("outage version/hash oracle", err)
	}
	if heads[0].ID.Author != f.senderID.DeviceID {
		t.Fatal("directory outage changed causal author")
	}
}
