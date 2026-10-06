package protocol

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

const RoutedInvitationMaxBytes = 16 << 10

// Route references bind identities/profile/purpose only; mutable candidates are
// deliberately absent. V2 endpoint strings keep their original interpretation.
type EnrollmentRoute struct {
	Device  string `json:"device"`
	Pin     string `json:"pin"`
	Profile string `json:"profile"`
	Purpose string `json:"purpose"`
}

func (r EnrollmentRoute) Validate() error {
	for _, s := range []string{r.Device, r.Pin, r.Profile} {
		if e := NetworkHex(s, 32); e != nil {
			return e
		}
	}
	if r.Purpose != "enrollment" {
		return errors.New("PURPOSE_MISMATCH")
	}
	return nil
}
func (r EnrollmentRoute) fields() []string { return []string{r.Device, r.Pin, r.Profile, r.Purpose} }

type RoutedInvitation struct {
	Version        string          `json:"version"`
	Folder         string          `json:"folder"`
	InviterRoute   EnrollmentRoute `json:"inviter_route"`
	CertificateDER string          `json:"certificate_der"`
	Capability     string          `json:"capability"`
	Expires        NetworkUint     `json:"expires"`
	Profile        NetworkProfile  `json:"profile"`
}

// Profile trust is supplied by the review/distributed authority, not the profile
// itself. Validate alone checks shape and transferred inviter pin, never consent.
func (i RoutedInvitation) Validate(now uint64, private bool) error {
	encoded, e := json.Marshal(i)
	if e != nil || len(encoded) > RoutedInvitationMaxBytes {
		return errors.New("PAYLOAD_TOO_LARGE")
	}
	if i.Version != "3" || uint64(i.Expires) <= now {
		return errors.New("INVITATION_INVALID")
	}
	for _, s := range []string{i.Folder, i.Capability} {
		if e := NetworkHex(s, 32); e != nil {
			return e
		}
	}
	if e := i.InviterRoute.Validate(); e != nil {
		return e
	}
	if _, _, e := NetworkCertificate(i.CertificateDER, i.InviterRoute.Pin, now); e != nil {
		return e
	}
	d, e := i.Profile.Digest(private)
	if e != nil {
		return e
	}
	if d != i.InviterRoute.Profile {
		return errors.New("PROFILE_MISMATCH")
	}
	return nil
}

type RoutedEnrollmentTranscript struct {
	Version          string          `json:"version"`
	Folder           string          `json:"folder"`
	Inviter          EnrollmentRoute `json:"inviter"`
	Requester        EnrollmentRoute `json:"requester"`
	CapabilityDigest string          `json:"capability_digest"`
	Attempt          string          `json:"attempt"`
	Challenge        string          `json:"challenge"`
	PublicKey        string          `json:"public_key"`
	PriorMembership  string          `json:"prior_membership"`
	Expires          NetworkUint     `json:"expires"`
	Label            string          `json:"label"`
}

func (t RoutedEnrollmentTranscript) Canonical() ([]byte, error) {
	if t.Version != "3" || t.Expires == 0 || !networkText(t.Label, 256) {
		return nil, errors.New("INVALID_REQUEST")
	}
	if e := t.Inviter.Validate(); e != nil {
		return nil, e
	}
	if e := t.Requester.Validate(); e != nil {
		return nil, e
	}
	if t.Inviter.Profile != t.Requester.Profile {
		return nil, errors.New("PROFILE_MISMATCH")
	}
	for _, s := range []string{t.Folder, t.CapabilityDigest, t.Attempt, t.Challenge, t.PublicKey, t.PriorMembership} {
		if e := NetworkHex(s, 32); e != nil {
			return nil, e
		}
	}
	f := []string{t.Version, t.Folder}
	f = append(f, t.Inviter.fields()...)
	f = append(f, t.Requester.fields()...)
	f = append(f, t.CapabilityDigest, t.Attempt, t.Challenge, t.PublicKey, t.PriorMembership, strconv.FormatUint(uint64(t.Expires), 10), t.Label)
	return NetworkCanonical("enrollment-request-v3", f...), nil
}
func (t RoutedEnrollmentTranscript) RequestID() (string, error) {
	if _, e := t.Canonical(); e != nil {
		return "", e
	}
	return NetworkDigest(NetworkCanonical("enrollment-attempt-v3", t.Folder, t.Inviter.Device, t.Inviter.Pin, t.Requester.Device, t.Requester.Pin, t.Attempt, t.Inviter.Profile)), nil
}
func (t RoutedEnrollmentTranscript) VerificationCode() (string, error) {
	b, e := t.Canonical()
	if e != nil {
		return "", e
	}
	s := NetworkDigest(b)[:20]
	return s[:5] + "-" + s[5:10] + "-" + s[10:15] + "-" + s[15:], nil
}

type RoutedEnrollmentRequest struct {
	Transcript     RoutedEnrollmentTranscript `json:"transcript"`
	Capability     string                     `json:"capability"`
	CertificateDER string                     `json:"certificate_der"`
	Signature      string                     `json:"signature"`
}

func (w RoutedEnrollmentRequest) Verify(now uint64) error {
	b, e := w.Transcript.Canonical()
	if e != nil {
		return e
	}
	if uint64(w.Transcript.Expires) <= now {
		return errors.New("INVITATION_INVALID")
	}
	if e := NetworkHex(w.Capability, 32); e != nil {
		return e
	}
	capability, _ := hex.DecodeString(w.Capability)
	if NetworkDigest(capability) != w.Transcript.CapabilityDigest {
		return errors.New("INVITATION_INVALID")
	}
	_, key, e := NetworkCertificate(w.CertificateDER, w.Transcript.Requester.Pin, now)
	if e != nil {
		return e
	}
	if hex.EncodeToString(key) != w.Transcript.PublicKey {
		return errors.New("IDENTITY_MISMATCH")
	}
	sig, e := hex.DecodeString(w.Signature)
	if e != nil || NetworkHex(w.Signature, 64) != nil || !ed25519.Verify(key, b, sig) {
		return errors.New("INVALID_SIGNATURE")
	}
	return nil
}

type RoutedEnrollmentStatus struct {
	Version          string          `json:"version"`
	Request          string          `json:"request"`
	TranscriptDigest string          `json:"transcript_digest"`
	Requester        EnrollmentRoute `json:"requester"`
	Nonce            string          `json:"nonce"`
	Expires          NetworkUint     `json:"expires"`
	Signature        string          `json:"signature"`
}

func (s RoutedEnrollmentStatus) Canonical() ([]byte, error) {
	if s.Version != "3" || s.Expires == 0 {
		return nil, errors.New("INVALID_REQUEST")
	}
	if e := s.Requester.Validate(); e != nil {
		return nil, e
	}
	for _, v := range []string{s.Request, s.TranscriptDigest, s.Nonce} {
		if e := NetworkHex(v, 32); e != nil {
			return nil, e
		}
	}
	f := []string{s.Version, s.Request, s.TranscriptDigest}
	f = append(f, s.Requester.fields()...)
	f = append(f, s.Nonce, strconv.FormatUint(uint64(s.Expires), 10))
	return NetworkCanonical("enrollment-status-v3", f...), nil
}

// Approval review binds the exact request digest and exact v1 membership digest.
// The existing membership transaction remains the only authority to install it.
type RoutedEnrollmentApproval struct {
	Version          string `json:"version"`
	Request          string `json:"request"`
	TranscriptDigest string `json:"transcript_digest"`
	PriorMembership  string `json:"prior_membership"`
	MembershipDigest string `json:"membership_digest"`
}

func (a RoutedEnrollmentApproval) Canonical() ([]byte, error) {
	if a.Version != "3" {
		return nil, errors.New("UNSUPPORTED_CAPABILITY")
	}
	for _, s := range []string{a.Request, a.TranscriptDigest, a.PriorMembership, a.MembershipDigest} {
		if e := NetworkHex(s, 32); e != nil {
			return nil, e
		}
	}
	return NetworkCanonical("enrollment-approval-v3", a.Version, a.Request, a.TranscriptDigest, a.PriorMembership, a.MembershipDigest), nil
}

func DecodeRoutedInvitation(data []byte, out *RoutedInvitation) error {
	if len(data) > RoutedInvitationMaxBytes {
		return errors.New("PAYLOAD_TOO_LARGE")
	}
	return NetworkDecode(data, out)
}

func (s RoutedEnrollmentStatus) Verify(key ed25519.PublicKey, now uint64) error {
	b, e := s.Canonical()
	if e != nil {
		return e
	}
	if uint64(s.Expires) <= now || uint64(s.Expires)-now > 60 {
		return errors.New("CHALLENGE_EXPIRED")
	}
	signature, e := hex.DecodeString(s.Signature)
	if e != nil || NetworkHex(s.Signature, 64) != nil || len(key) != 32 || !ed25519.Verify(key, b, signature) {
		return errors.New("INVALID_SIGNATURE")
	}
	return nil
}

// W05 serves these closed v3 HTTP envelopes. Capability is sent only after
// pinned inviter TLS, and never through directory/relay admission messages.
type RoutedChallengeRequest struct {
	Version    string          `json:"version"`
	Folder     string          `json:"folder"`
	Inviter    EnrollmentRoute `json:"inviter"`
	Requester  EnrollmentRoute `json:"requester"`
	Capability string          `json:"capability"`
	Attempt    string          `json:"attempt"`
}
type RoutedChallengeResult struct {
	Version         string      `json:"version"`
	Challenge       string      `json:"challenge"`
	Expires         NetworkUint `json:"expires"`
	PriorMembership string      `json:"prior_membership"`
}
type RoutedEnrollmentResult struct {
	Approval            *RoutedEnrollmentApproval `json:"approval,omitempty"`
	ApprovalSignature   string                    `json:"approval_signature,omitempty"`
	Version             string                    `json:"version"`
	Request             string                    `json:"request"`
	State               string                    `json:"state"`
	TranscriptDigest    string                    `json:"transcript_digest"`
	VerificationCode    string                    `json:"verification_code"`
	MembershipHex       string                    `json:"membership_hex"` // empty unless approved, canonical v1 artifact
	RetirementSnapshots []string                  `json:"retirement_snapshots"`
	InviterRoute        EnrollmentRoute           `json:"inviter_route"` // data Target uses this pin only after folder approval
}
