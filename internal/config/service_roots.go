package config

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/calebhabesh/orbit/internal/state"
)

// ServiceRootsFile holds reviewed custom trust for a self-hosted or development
// connection service. Release profiles always use the system roots.
const ServiceRootsFile = "network-roots.pem"

const MaxServiceRootsBytes = 16 << 10
const MaxServiceRoots = 4

// ValidateServiceRoots accepts one to four currently valid CA certificates (or
// self-signed service certificates) and returns a stable SHA-256 fingerprint of
// the canonical DER sequence. It never accepts keys or other PEM blocks.
func ValidateServiceRoots(data []byte, now time.Time) (*x509.CertPool, string, error) {
	if len(data) == 0 || len(data) > MaxServiceRootsBytes {
		return nil, "", errors.New("SERVICE_TRUST_INVALID")
	}
	pool := x509.NewCertPool()
	digest := sha256.New()
	count := 0
	for rest := data; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			if len(trimSpace(rest)) != 0 {
				return nil, "", errors.New("SERVICE_TRUST_INVALID")
			}
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, "", errors.New("SERVICE_TRUST_INVALID")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
			return nil, "", errors.New("SERVICE_TRUST_INVALID")
		}
		selfSigned := cert.CheckSignatureFrom(cert) == nil
		if !(cert.BasicConstraintsValid && cert.IsCA) && !selfSigned {
			return nil, "", errors.New("SERVICE_TRUST_INVALID")
		}
		count++
		if count > MaxServiceRoots {
			return nil, "", errors.New("SERVICE_TRUST_INVALID")
		}
		pool.AddCert(cert)
		sum := sha256.Sum256(cert.Raw)
		digest.Write(sum[:])
	}
	if count == 0 {
		return nil, "", errors.New("SERVICE_TRUST_INVALID")
	}
	return pool, hex.EncodeToString(digest.Sum(nil)), nil
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t') {
		b = b[1:]
	}
	return b
}

// LoadServiceRoots returns nil without error when no custom trust was reviewed.
func LoadServiceRoots(dir string, now time.Time) (*x509.CertPool, string, error) {
	data, err := state.ReadPrivate(dir, ServiceRootsFile, MaxServiceRootsBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return ValidateServiceRoots(data, now)
}

// SaveServiceRoots replaces or removes reviewed trust under the caller's
// exclusive state ownership. Empty data removes custom trust.
func SaveServiceRoots(dir string, data []byte, now time.Time) error {
	if len(data) == 0 {
		err := os.Remove(filepath.Join(dir, ServiceRootsFile))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if _, _, err := ValidateServiceRoots(data, now); err != nil {
		return err
	}
	return WritePrivate(dir, ServiceRootsFile, data)
}
