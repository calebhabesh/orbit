package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var errTerminalVersion = errors.New("unsupported enrollment version")
var errTerminalSignature = errors.New("invalid enrollment signature encoding")

func terminalTokenDigest(b []byte) [32]byte { return sha256.Sum256(b) }

// TerminalEnrollmentWire is the closed enrollment_v2 HTTP request object.
// DecodeStrict rejects unknown/duplicate fields; Transcript validates bindings.
// Secrets and signatures are lowercase hex. Counter/expiry fields are strings.
type TerminalEnrollmentWire struct {
	Version            string `json:"version"`
	Folder             string `json:"folder"`
	Inviter            string `json:"inviter"`
	InviterPin         string `json:"inviter_pin"`
	Token              string `json:"token"`
	Attempt            string `json:"attempt"`
	Challenge          string `json:"challenge"`
	Requester          string `json:"requester"`
	RequesterPin       string `json:"requester_pin"`
	PublicKey          string `json:"public_key"`
	CertificateDER     string `json:"certificate_der"`
	PriorMembership    string `json:"prior_membership"`
	ExpiresUnix        string `json:"expires_unix"`
	EnrollmentEndpoint string `json:"enrollment_endpoint"`
	PeerEndpoint       string `json:"peer_endpoint"`
	RequesterEndpoint  string `json:"requester_endpoint"`
	Label              string `json:"label"`
	Signature          string `json:"signature"`
}
type TerminalChallengeRequest struct {
	Version   string `json:"version"`
	Folder    string `json:"folder"`
	Token     string `json:"token"`
	Attempt   string `json:"attempt"`
	Requester string `json:"requester"`
}
type TerminalChallengeResult struct {
	Version         string `json:"version"`
	Challenge       string `json:"challenge"`
	ExpiresUnix     string `json:"expires_unix"`
	PriorMembership string `json:"prior_membership"`
}
type TerminalEnrollmentStatusRequest struct {
	Version     string `json:"version"`
	Request     string `json:"request"`
	Nonce       string `json:"nonce"`
	ExpiresUnix string `json:"expires_unix"`
	Signature   string `json:"signature"`
}
type TerminalEnrollmentResult struct {
	Version          string `json:"version"`
	Request          string `json:"request"`
	State            string `json:"state"`
	TranscriptDigest string `json:"transcript_digest"`
	VerificationCode string `json:"verification_code"`
	// Approved membership is encoded by existing canonical v1 membership rules.
	MembershipHex string `json:"membership_hex,omitempty"`
	PeerEndpoint  string `json:"peer_endpoint"`
}

func (w TerminalEnrollmentWire) Transcript() (TerminalEnrollmentTranscript, error) {
	var t TerminalEnrollmentTranscript
	if w.Version != "2" {
		return t, errTerminalVersion
	}
	fields := []string{w.Folder, w.Inviter, w.InviterPin, w.Attempt, w.Challenge, w.Requester, w.RequesterPin, w.PublicKey, w.PriorMembership}
	dest := []*[32]byte{&t.Folder, &t.Inviter, &t.InviterPin, &t.Attempt, &t.Challenge, &t.Requester, &t.RequesterPin, &t.PublicKey, &t.PriorMembership}
	for i, s := range fields {
		v, err := parseID(s)
		if err != nil {
			return t, err
		}
		copy(dest[i][:], v[:])
	}
	token, err := parseID(w.Token)
	if err != nil {
		return t, err
	}
	t.TokenDigest = terminalTokenDigest(token[:])
	t.ExpiresUnix, err = parseUint(w.ExpiresUnix, false)
	if err != nil {
		return t, err
	}
	t.EnrollmentEndpoint = w.EnrollmentEndpoint
	t.PeerEndpoint = w.PeerEndpoint
	t.RequesterEndpoint = w.RequesterEndpoint
	t.Label = w.Label
	if _, err := t.Canonical(); err != nil {
		return t, err
	}
	sig, err := hex.DecodeString(w.Signature)
	if err != nil || len(sig) != 64 || hex.EncodeToString(sig) != w.Signature {
		return t, errTerminalSignature
	}
	return t, nil
}
