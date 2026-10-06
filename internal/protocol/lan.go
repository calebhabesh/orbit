package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// LANAnnouncement is a separate local-only domain; its raw public key proves
// signature possession, while the reviewed SPKI pin remains the trust anchor.
type LANAnnouncement struct {
	Version      string             `json:"version"`
	Device       string             `json:"device"`
	Pin          string             `json:"pin"`
	Key          string             `json:"key"`
	Generation   NetworkUint        `json:"generation"`
	Expires      NetworkUint        `json:"expires"`
	Capabilities []string           `json:"capabilities"`
	Candidates   []NetworkCandidate `json:"candidates"`
	Signature    string             `json:"signature"`
}

func (a LANAnnouncement) Canonical() ([]byte, error) {
	if a.Version != "1" || NetworkHex(a.Device, 32) != nil || NetworkHex(a.Pin, 32) != nil || NetworkHex(a.Key, 32) != nil || a.Generation == 0 || a.Expires == 0 || len(a.Candidates) == 0 || len(a.Candidates) > 4 || len(a.Capabilities) < 1 || len(a.Capabilities) > 2 || a.Capabilities[0] != "direct_https_v1" || (len(a.Capabilities) == 2 && a.Capabilities[1] != "quic_http3_v1") {
		return nil, errors.New("INVALID_LAN_ANNOUNCEMENT")
	}
	seen := map[string]bool{}
	for _, c := range a.Candidates {
		if c.Validate(false) != nil || c.Scope != "lan" || (c.Transport != "tcp" && c.Transport != "udp") || seen[c.Transport+"/"+c.Address] {
			return nil, errors.New("INVALID_CANDIDATE")
		}
		if c.Transport == "udp" && len(a.Capabilities) != 2 {
			return nil, errors.New("INVALID_LAN_ANNOUNCEMENT")
		}
		seen[c.Transport+"/"+c.Address] = true
	}
	a.Signature = ""
	b, err := json.Marshal(a)
	return append([]byte("orbit-lan-v1\n"), b...), err
}
func (a LANAnnouncement) Verify(now uint64) error {
	if uint64(a.Expires) <= now || uint64(a.Expires) > now+600 {
		return errors.New("EXPIRED_LAN_ANNOUNCEMENT")
	}
	b, err := a.Canonical()
	if err != nil {
		return err
	}
	key, _ := hex.DecodeString(a.Key)
	der, err := x509.MarshalPKIXPublicKey(ed25519.PublicKey(key))
	if err != nil {
		return err
	}
	pin := sha256.Sum256(der)
	sig, err := hex.DecodeString(a.Signature)
	if err != nil || NetworkHex(a.Signature, 64) != nil || hex.EncodeToString(pin[:]) != a.Pin || !ed25519.Verify(key, b, sig) {
		return errors.New("IDENTITY_MISMATCH")
	}
	return nil
}
