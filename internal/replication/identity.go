// Package replication owns authenticated peer sessions and bounded wire
// handlers. It does not decide causal reconciliation or workspace projection.
package replication

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

const certificateServerName = "peer.filesync.invalid"

type Identity struct {
	DeviceID    history.ID
	Certificate tls.Certificate
	KeyPin      history.Digest
	Leaf        *x509.Certificate
}

func LoadOrCreateIdentity(stateDir string, deviceID history.ID, now time.Time) (Identity, error) {
	if deviceID == (history.ID{}) {
		return Identity{}, errors.New("device identity cannot be zero")
	}
	dir := filepath.Join(stateDir, "identity")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Identity{}, fmt.Errorf("create identity directory: %w", err)
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || dirInfo.Mode().Perm()&0o077 != 0 {
		return Identity{}, errors.New("identity directory must be a private nonsymlink directory (mode 0700)")
	}
	identityPath := filepath.Join(dir, "peer-identity.pem")
	identityInfo, statErr := os.Lstat(identityPath)
	if statErr == nil {
		if !identityInfo.Mode().IsRegular() || identityInfo.Mode().Perm()&0o077 != 0 {
			return Identity{}, errors.New("peer identity must be a private regular file (mode 0600)")
		}
		identityPEM, readErr := os.ReadFile(identityPath)
		if readErr != nil {
			return Identity{}, errors.New("peer identity is unreadable; do not silently replace it")
		}
		return parseIdentity(deviceID, identityPEM, identityPEM)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return Identity{}, errors.New("peer identity is unreadable; do not silently replace it")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("generate peer key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return Identity{}, fmt.Errorf("generate certificate serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "filesync-device-" + hex.EncodeToString(deviceID[:8])},
		DNSNames:              []string{certificateServerName},
		NotBefore:             now.UTC().Add(-5 * time.Minute),
		NotAfter:              now.UTC().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		return Identity{}, fmt.Errorf("create peer certificate: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return Identity{}, fmt.Errorf("encode peer key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	identityPEM := append(append([]byte(nil), certPEM...), keyPEM...)
	if err := writePrivateExclusive(identityPath, identityPEM); err != nil {
		return Identity{}, err
	}
	return parseIdentity(deviceID, identityPEM, identityPEM)
}

func writePrivateExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create identity file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func parseIdentity(deviceID history.ID, certPEM, keyPEM []byte) (Identity, error) {
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return Identity{}, fmt.Errorf("load peer identity: %w", err)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return Identity{}, fmt.Errorf("parse peer certificate: %w", err)
	}
	certificate.Leaf = leaf
	return Identity{DeviceID: deviceID, Certificate: certificate, KeyPin: sha256.Sum256(leaf.RawSubjectPublicKeyInfo), Leaf: leaf}, nil
}

func PublicKeyPin(certificate *x509.Certificate) history.Digest {
	if certificate == nil {
		return history.Digest{}
	}
	return sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
}

func ParsePeerCertificate(data []byte) (*x509.Certificate, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 {
		return nil, errors.New("peer certificate must contain exactly one PEM certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse peer certificate: %w", err)
	}
	return certificate, nil
}

func (identity Identity) CertificatePEM() []byte {
	if identity.Leaf == nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: identity.Leaf.Raw})
}

func (identity Identity) ServerTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{identity.Certificate},
		ClientAuth:   tls.RequireAnyClientCert,
		NextProtos:   []string{"h2", "http/1.1"},
	}
}

func (identity Identity) ClientTLSConfig(peerCertificate *x509.Certificate, expectedPin history.Digest) (*tls.Config, error) {
	if peerCertificate == nil || PublicKeyPin(peerCertificate) != expectedPin {
		return nil, errors.New("peer certificate does not match approved key pin")
	}
	roots := x509.NewCertPool()
	roots.AddCert(peerCertificate)
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{identity.Certificate},
		RootCAs:      roots,
		ServerName:   certificateServerName,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 || PublicKeyPin(state.PeerCertificates[0]) != expectedPin {
				return errors.New("peer public key pin mismatch")
			}
			return nil
		},
	}, nil
}
