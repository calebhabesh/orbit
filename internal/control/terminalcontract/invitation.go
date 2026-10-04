package terminalcontract

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"
)

func validEndpoint(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/")
}

// Validate checks a transferred invitation's structure and certificate pin.
// Trust still requires deliberate transfer; expiry and current membership are
// rechecked by the inviter's durable admission transaction, not this codec.
func (i Invitation) Validate() error {
	if i.Version != Version || !validID(i.Folder) || !validID(i.Inviter) || !validID(i.KeyPin) || !validID(i.Capability) || !validEndpoint(i.EnrollmentEndpoint) || !validEndpoint(i.PeerEndpoint) {
		return fmt.Errorf("INVALID_REQUEST: invitation")
	}
	if _, err := time.Parse(time.RFC3339Nano, i.ExpiresAt); err != nil {
		return err
	}
	b, err := base64.StdEncoding.DecodeString(i.CertificateDER)
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(b)
	if err != nil {
		return err
	}
	pin := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	if hex.EncodeToString(pin[:]) != i.KeyPin {
		return fmt.Errorf("IDENTITY_MISMATCH")
	}
	return nil
}
