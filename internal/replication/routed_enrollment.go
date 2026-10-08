package replication

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

// RoutedRecordWire projects review fields only. It is never signed or decoded as
// a v2 transcript. Exact v3 bytes are retained separately in EnrollmentRecord.
func RoutedRecordWire(w p.RoutedEnrollmentRequest) p.TerminalEnrollmentWire {
	t := w.Transcript
	return p.TerminalEnrollmentWire{Version: "3", Folder: t.Folder, Inviter: t.Inviter.Device, InviterPin: t.Inviter.Pin, Requester: t.Requester.Device, RequesterPin: t.Requester.Pin, PublicKey: t.PublicKey, CertificateDER: w.CertificateDER, Attempt: t.Attempt, Challenge: t.Challenge, PriorMembership: t.PriorMembership, ExpiresUnix: strconv.FormatUint(uint64(t.Expires), 10), Label: t.Label, Signature: w.Signature}
}
func routedResult(r EnrollmentRecord) p.RoutedEnrollmentResult {
	return p.RoutedEnrollmentResult{Version: "3", Request: r.Result.Request, State: r.Result.State, TranscriptDigest: r.Digest, VerificationCode: r.Result.VerificationCode, MembershipHex: r.Result.MembershipHex, RetirementSnapshots: []string{}, InviterRoute: r.Routed.Transcript.Inviter, Approval: r.Approval, ApprovalSignature: r.ApprovalSignature}
}
func (s *EnrollmentServer) routedChallenge(ctx context.Context, q p.RoutedChallengeRequest) (p.RoutedChallengeResult, error) {
	var out p.RoutedChallengeResult
	if q.Version != "3" || q.Inviter.Validate() != nil || q.Requester.Validate() != nil || q.Inviter.Profile != q.Requester.Profile {
		return out, enrollmentError("INVALID_REQUEST")
	}
	for _, v := range []string{q.Folder, q.Attempt} {
		if _, e := enrollmentID(v); e != nil {
			return out, e
		}
	}
	verifier, err := tokenVerifier(q.Capability)
	if err != nil {
		return out, err
	}
	q.Capability = verifier
	err = s.repo.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
		inv, e := invitationActive(tx, verifier, q.Folder, s.now(), false)
		if e != nil {
			return e
		}
		if inv.Invitation.Version != "3" || inv.Invitation.Route == nil || *inv.Invitation.Route != q.Inviter || q.Inviter.Device != hex.EncodeToString(s.identity.DeviceID[:]) || q.Inviter.Pin != hex.EncodeToString(s.identity.KeyPin[:]) {
			return enrollmentError("UNAUTHORIZED")
		}
		if inv.TargetDevice != "" && (inv.TargetDevice != q.Requester.Device || inv.TargetPin != q.Requester.Pin) {
			return enrollmentError("UNAUTHORIZED")
		}
		count, e := enrollmentNonceCount(tx, s.now())
		if e != nil {
			return e
		}
		if count >= 128 {
			return enrollmentError("RATE_LIMITED")
		}
		folder, _ := enrollmentID(q.Folder)
		_, prior, e := tx.Membership(folder)
		if e != nil {
			return e
		}
		nonce, e := enrollmentRandom()
		if e != nil {
			return e
		}
		expiry := s.now().Add(time.Minute)
		ie, _ := time.Parse(time.RFC3339Nano, inv.Invitation.ExpiresAt)
		if ie.Before(expiry) {
			expiry = ie
		}
		out = p.RoutedChallengeResult{Version: "3", Challenge: nonce, Expires: p.NetworkUint(expiry.Unix()), PriorMembership: hex.EncodeToString(prior[:])}
		return tx.Put("challenge/"+nonce, enrollmentNonce{Routed: &q, Result: p.TerminalChallengeResult{Version: "3", Challenge: nonce, ExpiresUnix: strconv.FormatInt(expiry.Unix(), 10), PriorMembership: out.PriorMembership}})
	})
	return out, err
}
func (s *EnrollmentServer) routedSubmit(ctx context.Context, w p.RoutedEnrollmentRequest) (p.RoutedEnrollmentResult, error) {
	var out p.RoutedEnrollmentResult
	// Cryptographic proof is required even for replay; accepted records outlive
	// their minute-long submission challenge, without extending capability use.
	b, err := w.Transcript.Canonical()
	if err != nil {
		return out, err
	}
	verifyAt := uint64(s.now().Unix())
	if uint64(w.Transcript.Expires) <= verifyAt {
		verifyAt = uint64(w.Transcript.Expires) - 1
	}
	if err = w.Verify(verifyAt); err != nil {
		return out, err
	}
	request, err := w.Transcript.RequestID()
	if err != nil {
		return out, err
	}
	digest := p.NetworkDigest(b)
	verifier, _ := tokenVerifier(w.Capability)
	err = s.repo.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
		var old EnrollmentRecord
		if e := tx.Get("request/"+request, &old); e == nil {
			safe := w
			safe.Capability = ""
			if old.Routed == nil || old.Digest != digest || *old.Routed != safe || old.TokenDigest != verifier {
				return enrollmentError("IDEMPOTENCY_CONFLICT")
			}
			if s.now().Unix() >= old.Expires {
				return enrollmentError("EXPIRED_REPLAY")
			}
			var inv EnrollmentInvitation
			if e = tx.Get("invite/"+verifier, &inv); e != nil || inv.Revoked {
				return enrollmentError("INVITATION_INVALID")
			}
			out = routedResult(old)
			return nil
		} else if !errors.Is(e, repository.ErrOperationNotFound) {
			return e
		}
		if uint64(w.Transcript.Expires) <= uint64(s.now().Unix()) {
			return enrollmentError("EXPIRED_REPLAY")
		}
		t := w.Transcript
		inv, e := invitationActive(tx, verifier, t.Folder, s.now(), false)
		if e != nil {
			return e
		}
		if inv.Invitation.Version != "3" || inv.Invitation.Route == nil || *inv.Invitation.Route != t.Inviter {
			return enrollmentError("UNAUTHORIZED")
		}
		if inv.TargetDevice != "" && (inv.TargetDevice != t.Requester.Device || inv.TargetPin != t.Requester.Pin) {
			return enrollmentError("UNAUTHORIZED")
		}
		var nonce enrollmentNonce
		if tx.Get("challenge/"+t.Challenge, &nonce) != nil {
			return enrollmentError("UNAUTHORIZED")
		}
		expected := p.RoutedChallengeRequest{Version: "3", Folder: t.Folder, Inviter: t.Inviter, Requester: t.Requester, Capability: verifier, Attempt: t.Attempt}
		if nonce.Routed == nil || *nonce.Routed != expected || nonce.Result.PriorMembership != t.PriorMembership || nonce.Result.ExpiresUnix != strconv.FormatUint(uint64(t.Expires), 10) {
			return enrollmentError("UNAUTHORIZED")
		}
		folder, _ := enrollmentID(t.Folder)
		membership, prior, e := tx.Membership(folder)
		if e != nil {
			return e
		}
		if hex.EncodeToString(prior[:]) != t.PriorMembership {
			return enrollmentError("STALE_VIEW")
		}
		for _, r := range membership.Retired {
			if hex.EncodeToString(r.Device[:]) == t.Requester.Device {
				return enrollmentError("UNAUTHORIZED")
			}
		}
		pin, keyErr := tx.DeviceKeyPin(history.ID(mustNetworkID(t.Requester.Device)))
		if keyErr == nil && hex.EncodeToString(pin[:]) != t.Requester.Pin {
			return enrollmentError("UNAUTHORIZED")
		}
		if keyErr != nil && !errors.Is(keyErr, sql.ErrNoRows) {
			return keyErr
		}
		for _, member := range membership.Active {
			if hex.EncodeToString(member.Device[:]) == t.Requester.Device {
				return enrollmentError("UNAUTHORIZED")
			}
		}
		count, e := pendingCount(tx, s.now())
		if e != nil {
			return e
		}
		if count >= 128 {
			return enrollmentError("RATE_LIMITED")
		}
		code, e := t.VerificationCode()
		if e != nil {
			return e
		}
		expiry, _ := time.Parse(time.RFC3339Nano, inv.Invitation.ExpiresAt)
		w.Capability = ""
		record := EnrollmentRecord{CreatedNS: s.now().UnixNano(), Wire: RoutedRecordWire(w), Routed: &w, Digest: digest, TokenDigest: verifier, Expires: expiry.Unix(), Result: p.TerminalEnrollmentResult{Version: "3", Request: request, State: "pending_approval", TranscriptDigest: digest, VerificationCode: code}}
		if e = tx.Put("request/"+request, record); e != nil {
			return e
		}
		inv.Uses++
		if e = tx.Put("invite/"+verifier, inv); e != nil {
			return e
		}
		out = routedResult(record)
		return tx.Delete("challenge/" + t.Challenge)
	})
	return out, err
}
func mustNetworkID(s string) [32]byte {
	var out [32]byte
	b, _ := hex.DecodeString(s)
	copy(out[:], b)
	return out
}
func (s *EnrollmentServer) routedStatus(ctx context.Context, q p.RoutedEnrollmentStatus) (any, error) {
	if q.Version != "3" || q.Requester.Validate() != nil || p.NetworkHex(q.Request, 32) != nil || p.NetworkHex(q.TranscriptDigest, 32) != nil {
		return nil, enrollmentError("INVALID_REQUEST")
	}
	var out any
	err := s.repo.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
		if q.Nonce == "" && q.Signature == "" && q.Expires == 0 {
			count, e := enrollmentNonceCount(tx, s.now())
			if e != nil {
				return e
			}
			if count >= 128 {
				return enrollmentError("RATE_LIMITED")
			}
			nonce, e := enrollmentRandom()
			if e != nil {
				return e
			}
			out = p.RoutedChallengeResult{Version: "3", Challenge: nonce, Expires: p.NetworkUint(s.now().Unix() + 60)}
			return tx.Put("status/"+nonce, enrollmentNonce{RoutedStatus: &q, Result: p.TerminalChallengeResult{Version: "3", Challenge: nonce, ExpiresUnix: strconv.FormatInt(s.now().Unix()+60, 10)}})
		}
		var nonce enrollmentNonce
		if tx.Get("status/"+q.Nonce, &nonce) != nil {
			return enrollmentError("UNAUTHORIZED")
		}
		base := q
		base.Nonce = ""
		base.Signature = ""
		base.Expires = 0
		if nonce.RoutedStatus == nil || *nonce.RoutedStatus != base || nonce.Result.ExpiresUnix != strconv.FormatUint(uint64(q.Expires), 10) {
			return enrollmentError("UNAUTHORIZED")
		}
		var record EnrollmentRecord
		if tx.Get("request/"+q.Request, &record) != nil || record.Routed == nil {
			return enrollmentError("UNAUTHORIZED")
		}
		t := record.Routed.Transcript
		if record.Digest != q.TranscriptDigest || t.Requester != q.Requester {
			return enrollmentError("UNAUTHORIZED")
		}
		key, _ := hex.DecodeString(t.PublicKey)
		if q.Verify(ed25519.PublicKey(key), uint64(s.now().Unix())) != nil {
			return enrollmentError("UNAUTHORIZED")
		}
		var inv EnrollmentInvitation
		if tx.Get("invite/"+record.TokenDigest, &inv) != nil || inv.Revoked {
			return enrollmentError("INVITATION_INVALID")
		}
		if s.now().Unix() >= record.Expires && record.Result.State == "pending_approval" {
			record.Result.State = "expired"
			if e := tx.Put("request/"+q.Request, record); e != nil {
				return e
			}
		}
		out = routedResult(record)
		return tx.Delete("status/" + q.Nonce)
	})
	return out, err
}

// NewV3EnrollmentClient requires independently reviewed profile trust as well as
// the deliberately transferred inviter certificate. No URL is interpreted as v3.
func NewV3EnrollmentClient(ctx context.Context, inv tc.Invitation, id Identity, selection network.ProfileSelection, manager network.Manager) (*EnrollmentClient, error) {
	if inv.Version != "3" || manager == nil {
		return nil, enrollmentError("UNSUPPORTED_CAPABILITY")
	}
	if err := inv.Validate(); err != nil {
		return nil, err
	}
	if err := selection.Validate(uint64(time.Now().Unix())); err != nil {
		return nil, err
	}
	digest, err := selection.Digest()
	if err != nil || digest != inv.Route.Profile {
		return nil, enrollmentError("PROFILE_MISMATCH")
	}
	target := network.Target{Device: history.ID(mustNetworkID(inv.Inviter)), Pin: history.Digest(mustNetworkID(inv.KeyPin)), Profile: history.Digest(mustNetworkID(digest)), Purpose: network.Enrollment}
	cert, _, err := p.NetworkCertificate(inv.CertificateDER, inv.KeyPin, uint64(time.Now().Unix()))
	if err != nil {
		return nil, err
	}
	trust, err := id.ClientTLSConfig(cert, target.Pin)
	if err != nil {
		return nil, err
	}
	rt, err := manager.Transport(ctx, target, trust)
	if err != nil {
		return nil, err
	}
	// Only the client copy receives a logical HTTP origin. The reviewed invitation
	// and all signed/persisted v3 transcripts remain endpoint-free.
	inv.EnrollmentEndpoint = network.LogicalOrigin(target)
	httpClient := network.HTTPClient(rt)
	httpClient.Timeout = 10 * time.Second
	return &EnrollmentClient{invitation: inv, identity: id, http: httpClient, transport: httpClient}, nil
}
func (c *EnrollmentClient) PrepareV3(ctx context.Context, attempt, label string) (p.RoutedEnrollmentRequest, error) {
	var w p.RoutedEnrollmentRequest
	i := c.invitation
	if i.Route == nil {
		return w, enrollmentError("UNSUPPORTED_CAPABILITY")
	}
	expiry, _ := time.Parse(time.RFC3339Nano, i.ExpiresAt)
	if !time.Now().Before(expiry) {
		return w, enrollmentError("INVITATION_INVALID")
	}
	requester := p.EnrollmentRoute{Device: hex.EncodeToString(c.identity.DeviceID[:]), Pin: hex.EncodeToString(c.identity.KeyPin[:]), Profile: i.Route.Profile, Purpose: "enrollment"}
	var challenge p.RoutedChallengeResult
	if err := c.post(ctx, "/enrollment/v3/challenge", p.RoutedChallengeRequest{Version: "3", Folder: i.Folder, Inviter: *i.Route, Requester: requester, Capability: i.Capability, Attempt: attempt}, &challenge); err != nil {
		return w, err
	}
	if challenge.Version != "3" || uint64(challenge.Expires) > uint64(expiry.Unix()) || uint64(challenge.Expires) <= uint64(time.Now().Unix()) {
		return w, enrollmentError("INVALID_REQUEST")
	}
	key := c.identity.Certificate.PrivateKey.(ed25519.PrivateKey)
	w = p.RoutedEnrollmentRequest{Transcript: p.RoutedEnrollmentTranscript{Version: "3", Folder: i.Folder, Inviter: *i.Route, Requester: requester, CapabilityDigest: mustTokenVerifier(i.Capability), Attempt: attempt, Challenge: challenge.Challenge, PublicKey: hex.EncodeToString(key.Public().(ed25519.PublicKey)), PriorMembership: challenge.PriorMembership, Expires: challenge.Expires, Label: label}, Capability: i.Capability, CertificateDER: encodeCertificate(c.identity.Leaf)}
	b, err := w.Transcript.Canonical()
	if err != nil {
		return w, err
	}
	w.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	return w, nil
}
func mustTokenVerifier(s string) string { v, _ := tokenVerifier(s); return v }
func (c *EnrollmentClient) SubmitV3(ctx context.Context, w p.RoutedEnrollmentRequest) (p.RoutedEnrollmentResult, error) {
	var out p.RoutedEnrollmentResult
	err := c.post(ctx, "/enrollment/v3/request", w, &out)
	if err == nil {
		err = c.validateV3Result(w, out)
	}
	return out, err
}
func (c *EnrollmentClient) StatusV3(ctx context.Context, w p.RoutedEnrollmentRequest) (p.RoutedEnrollmentResult, error) {
	var out p.RoutedEnrollmentResult
	b, err := w.Transcript.Canonical()
	if err != nil {
		return out, err
	}
	request, _ := w.Transcript.RequestID()
	q := p.RoutedEnrollmentStatus{Version: "3", Request: request, TranscriptDigest: p.NetworkDigest(b), Requester: w.Transcript.Requester}
	var challenge p.RoutedChallengeResult
	if err = c.post(ctx, "/enrollment/v3/status", q, &challenge); err != nil {
		return out, err
	}
	if challenge.Version != "3" || uint64(challenge.Expires) <= uint64(time.Now().Unix()) || uint64(challenge.Expires)-uint64(time.Now().Unix()) > 60 {
		return out, enrollmentError("INVALID_REQUEST")
	}
	q.Nonce = challenge.Challenge
	q.Expires = challenge.Expires
	b, err = q.Canonical()
	if err != nil {
		return out, err
	}
	q.Signature = hex.EncodeToString(ed25519.Sign(c.identity.Certificate.PrivateKey.(ed25519.PrivateKey), b))
	err = c.post(ctx, "/enrollment/v3/status", q, &out)
	if err == nil {
		err = c.validateV3Result(w, out)
	}
	return out, err
}
func (c *EnrollmentClient) validateV3Result(w p.RoutedEnrollmentRequest, out p.RoutedEnrollmentResult) error {
	b, e := w.Transcript.Canonical()
	if e != nil {
		return e
	}
	request, _ := w.Transcript.RequestID()
	if out.Version != "3" || out.Request != request || out.TranscriptDigest != p.NetworkDigest(b) || out.InviterRoute != w.Transcript.Inviter {
		return enrollmentError("IDENTITY_MISMATCH")
	}
	if out.State != "approved" {
		return nil
	}
	raw, e := hex.DecodeString(out.MembershipHex)
	if e != nil {
		return e
	}
	m, e := p.DecodeMembership(raw)
	if e != nil {
		return e
	}
	d, e := p.MembershipDigest(m)
	if e != nil {
		return e
	}
	expected := p.RoutedEnrollmentApproval{Version: "3", Request: request, TranscriptDigest: out.TranscriptDigest, PriorMembership: w.Transcript.PriorMembership, MembershipDigest: hex.EncodeToString(d[:])}
	if out.Approval == nil || *out.Approval != expected {
		return enrollmentError("IDENTITY_MISMATCH")
	}
	msg, e := expected.Canonical()
	if e != nil {
		return e
	}
	_, key, e := p.NetworkCertificate(c.invitation.CertificateDER, c.invitation.KeyPin, uint64(time.Now().Unix()))
	if e != nil {
		return e
	}
	sig, _ := hex.DecodeString(out.ApprovalSignature)
	if !ed25519.Verify(key, msg, sig) || hex.EncodeToString(m.Folder[:]) != w.Transcript.Folder || hex.EncodeToString(m.PriorDigest[:]) != w.Transcript.PriorMembership {
		return enrollmentError("IDENTITY_MISMATCH")
	}
	inviter, requester := false, false
	for _, a := range m.Active {
		if hex.EncodeToString(a.Device[:]) == w.Transcript.Inviter.Device && hex.EncodeToString(a.KeyPin[:]) == w.Transcript.Inviter.Pin {
			inviter = true
		}
		if a.Device == c.identity.DeviceID && a.KeyPin == c.identity.KeyPin {
			requester = true
		}
	}
	if !inviter || !requester {
		return enrollmentError("IDENTITY_MISMATCH")
	}
	return nil
}
func encodeCertificate(c *x509.Certificate) string { return base64.StdEncoding.EncodeToString(c.Raw) }
