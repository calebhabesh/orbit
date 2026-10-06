package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func w13Certificate(t *testing.T, ca bool, notAfter time.Time, parent *x509.Certificate, parentKey ed25519.PrivateKey) ([]byte, *x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "w13"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, IsCA: ca, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, pub, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert, key
}

func TestWANW13ServiceRootsValidationAndReplacement(t *testing.T) {
	now := time.Now()
	ca, caCert, caKey := w13Certificate(t, true, now.Add(time.Hour), nil, nil)
	leaf, _, _ := w13Certificate(t, false, now.Add(time.Hour), caCert, caKey)
	expired, _, _ := w13Certificate(t, true, now.Add(-time.Minute), nil, nil)
	if _, fingerprint, err := ValidateServiceRoots(ca, now); err != nil || len(fingerprint) != 64 {
		t.Fatal("valid CA refused", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("not trust")})
	for name, data := range map[string][]byte{
		"empty":         nil,
		"non-CA leaf":   leaf,
		"expired":       expired,
		"private key":   append(append([]byte{}, ca...), keyPEM...),
		"trailing junk": append(append([]byte{}, ca...), []byte("junk")...),
		"too many":      []byte(strings.Repeat(string(ca), MaxServiceRoots+1)),
		"oversized":     make([]byte, MaxServiceRootsBytes+1),
	} {
		if _, _, err := ValidateServiceRoots(data, now); err == nil {
			t.Fatalf("%s accepted as service trust", name)
		}
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if pool, fingerprint, err := LoadServiceRoots(dir, now); pool != nil || fingerprint != "" || err != nil {
		t.Fatal("absent trust must mean system roots", err)
	}
	if err := SaveServiceRoots(dir, leaf, now); err == nil {
		t.Fatal("invalid trust saved")
	}
	if err := SaveServiceRoots(dir, ca, now); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dir, ServiceRootsFile)); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("trust is not owner-only", err)
	}
	if pool, _, err := LoadServiceRoots(dir, now); pool == nil || err != nil {
		t.Fatal("saved trust not loaded", err)
	}
	if _, _, err := LoadServiceRoots(dir, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired stored trust loaded")
	}
	if err := SaveServiceRoots(dir, nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ServiceRootsFile)); !os.IsNotExist(err) {
		t.Fatal("profile review without trust retained old roots")
	}
}
