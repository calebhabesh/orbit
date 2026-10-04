package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"unicode/utf8"
)

// TerminalEnrollmentTranscript is the canonical signed request material for
// enrollment_v2. It does not change base_sync_v1 envelopes or membership bytes.
// Fixed identities/digests are 32 bytes; strings are uint32 length-prefixed UTF-8.
type TerminalEnrollmentTranscript struct {
	Folder             [32]byte
	Inviter            [32]byte
	InviterPin         [32]byte
	TokenDigest        [32]byte
	Attempt            [32]byte
	Challenge          [32]byte
	Requester          [32]byte
	RequesterPin       [32]byte
	PublicKey          [32]byte // Ed25519 raw public key, alongside the TLS SPKI pin
	PriorMembership    [32]byte
	ExpiresUnix        uint64
	EnrollmentEndpoint string
	PeerEndpoint       string
	RequesterEndpoint  string // empty for receive-only initial connectivity
	Label              string
}

func (t TerminalEnrollmentTranscript) Canonical() ([]byte, error) {
	var zero [32]byte
	for _, v := range [][32]byte{t.Folder, t.Inviter, t.InviterPin, t.TokenDigest, t.Attempt, t.Challenge, t.Requester, t.RequesterPin, t.PublicKey, t.PriorMembership} {
		if v == zero {
			return nil, fmt.Errorf("zero enrollment identity/digest")
		}
	}
	if t.ExpiresUnix == 0 || len(t.Label) > 256 || !utf8.ValidString(t.Label) {
		return nil, fmt.Errorf("invalid enrollment expiry/label")
	}
	for index, s := range []string{t.EnrollmentEndpoint, t.PeerEndpoint, t.RequesterEndpoint} {
		if s == "" && index == 2 {
			continue
		}
		u, err := url.Parse(s)
		if err != nil || len(s) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, fmt.Errorf("invalid enrollment endpoint")
		}
	}
	var b bytes.Buffer
	b.WriteString("orbit-enrollment-request-v2\x00")
	for _, v := range [][32]byte{t.Folder, t.Inviter, t.InviterPin, t.TokenDigest, t.Attempt, t.Challenge, t.Requester, t.RequesterPin, t.PublicKey, t.PriorMembership} {
		b.Write(v[:])
	}
	_ = binary.Write(&b, binary.BigEndian, t.ExpiresUnix)
	for _, s := range []string{t.EnrollmentEndpoint, t.PeerEndpoint, t.RequesterEndpoint, t.Label} {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(s)))
		b.WriteString(s)
	}
	return b.Bytes(), nil
}
func (t TerminalEnrollmentTranscript) Digest() ([32]byte, error) {
	b, err := t.Canonical()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(b), nil
}
func (t TerminalEnrollmentTranscript) RequestID() string {
	// Identity distinguishes folders/attempts but not retry challenge refreshes.
	b := append([]byte("orbit-enrollment-attempt-v2\x00"), t.Folder[:]...)
	b = append(b, t.Inviter[:]...)
	b = append(b, t.Requester[:]...)
	b = append(b, t.Attempt[:]...)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (t TerminalEnrollmentTranscript) VerificationCode() (string, error) {
	d, err := t.Digest()
	if err != nil {
		return "", err
	}
	// 80 bits of transcript hash, grouped for deliberate cross-device comparison.
	s := hex.EncodeToString(d[:10])
	return s[:5] + "-" + s[5:10] + "-" + s[10:15] + "-" + s[15:], nil
}

// TerminalStatusTranscript binds possession to one request and one-use server
// nonce. The handler checks nonce storage/expiry before returning any artifact.
func TerminalStatusTranscript(request, nonce [32]byte, expires uint64) ([]byte, error) {
	if request == ([32]byte{}) || nonce == ([32]byte{}) || expires == 0 {
		return nil, fmt.Errorf("invalid status proof")
	}
	var b bytes.Buffer
	b.WriteString("orbit-enrollment-status-v2\x00")
	b.Write(request[:])
	b.Write(nonce[:])
	_ = binary.Write(&b, binary.BigEndian, expires)
	return b.Bytes(), nil
}
