// Package model provides independent test oracles for causal DAGs, garbage collection,
// and linear membership rollout.
package model

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrRevisionNotMonotonic = errors.New("membership revision not monotonic")
	ErrPriorDigestMismatch  = errors.New("membership prior digest mismatch")
	ErrDuplicateMember      = errors.New("duplicate member in membership revision")
	ErrRetiredMemberRevival = errors.New("cannot revive retired member")
	ErrInvalidSignature     = errors.New("membership revision signature invalid")
	ErrMembershipFork       = errors.New("membership fork detected: concurrent conflicting revisions")
	ErrUnauthorizedAuthor   = errors.New("membership revision author not authorized")
)

// MemberRecord represents an active participant device and its key pin.
type MemberRecord struct {
	DeviceID string
	KeyPin   string
}

// MembershipRevision represents an explicit linear revision of workspace membership.
type MembershipRevision struct {
	WorkspaceID string
	Revision    uint64
	PriorDigest string
	Active      []MemberRecord
	Retired     []string
	AuthorID    string
	Signature   []byte
}

// Digest computes the canonical SHA-256 digest of the membership revision.
func (r *MembershipRevision) Digest() string {
	buf := new(bytes.Buffer)
	buf.WriteString("orbit-membership-v1\x00")
	buf.WriteString(r.WorkspaceID)
	buf.WriteByte(0)
	var revBytes [8]byte
	binary.BigEndian.PutUint64(revBytes[:], r.Revision)
	buf.Write(revBytes[:])
	buf.WriteString(r.PriorDigest)

	active := append([]MemberRecord(nil), r.Active...)
	sort.Slice(active, func(i, j int) bool {
		return active[i].DeviceID < active[j].DeviceID
	})
	for _, m := range active {
		buf.WriteString(m.DeviceID)
		buf.WriteString(m.KeyPin)
	}

	retired := append([]string(nil), r.Retired...)
	sort.Strings(retired)
	for _, ret := range retired {
		buf.WriteString(ret)
	}

	hash := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(hash[:])
}

// Sign signs the revision using the authority private key.
func (r *MembershipRevision) Sign(privKey ed25519.PrivateKey) {
	d := r.Digest()
	r.Signature = ed25519.Sign(privKey, []byte(d))
}

// VerifySignature verifies the signature against the authority public key.
func (r *MembershipRevision) VerifySignature(pubKey ed25519.PublicKey) bool {
	if len(r.Signature) == 0 {
		return false
	}
	d := r.Digest()
	return ed25519.Verify(pubKey, []byte(d), r.Signature)
}

// MembershipChain models an independent test oracle for linear membership state.
type MembershipChain struct {
	WorkspaceID string
	Head        *MembershipRevision
	History     map[uint64]*MembershipRevision
	Retired     map[string]bool
	OwnerPubKey ed25519.PublicKey
}

// NewMembershipChain initializes a new chain with a root revision signed by the owner.
func NewMembershipChain(workspaceID string, initialMembers []MemberRecord, ownerPubKey ed25519.PublicKey, ownerPrivKey ed25519.PrivateKey) (*MembershipChain, error) {
	root := &MembershipRevision{
		WorkspaceID: workspaceID,
		Revision:    1,
		PriorDigest: stringsRepeat("0", 64),
		Active:      initialMembers,
		Retired:     nil,
		AuthorID:    hex.EncodeToString(ownerPubKey[:]),
	}
	root.Sign(ownerPrivKey)

	c := &MembershipChain{
		WorkspaceID: workspaceID,
		Head:        root,
		History:     map[uint64]*MembershipRevision{1: root},
		Retired:     make(map[string]bool),
		OwnerPubKey: ownerPubKey,
	}
	return c, nil
}

func stringsRepeat(s string, count int) string {
	var res string
	for i := 0; i < count; i++ {
		res += s
	}
	return res
}

// ApplyRevision applies a candidate revision to the chain, checking monotonicity,
// prior digest, signatures, duplicate members, and retirement immutability.
func (c *MembershipChain) ApplyRevision(rev *MembershipRevision) error {
	if rev.WorkspaceID != c.WorkspaceID {
		return errors.New("workspace ID mismatch")
	}
	if rev.Revision != c.Head.Revision+1 {
		return ErrRevisionNotMonotonic
	}
	expectedPrior := c.Head.Digest()
	if rev.PriorDigest != expectedPrior {
		// If revision numbers match existing history or another branch with different digest, it's a fork
		return ErrPriorDigestMismatch
	}
	if !rev.VerifySignature(c.OwnerPubKey) {
		return ErrInvalidSignature
	}

	// Check duplicates within active
	seenActive := make(map[string]bool)
	for _, m := range rev.Active {
		if seenActive[m.DeviceID] {
			return ErrDuplicateMember
		}
		seenActive[m.DeviceID] = true

		// Check if member was previously retired
		if c.Retired[m.DeviceID] {
			return ErrRetiredMemberRevival
		}
	}

	// Check retired set
	for _, r := range rev.Retired {
		if seenActive[r] {
			return errors.New("device cannot be active and retired in same revision")
		}
		c.Retired[r] = true
	}

	c.Head = rev
	c.History[rev.Revision] = rev
	return nil
}

// DetectFork checks if two competing revisions from the same base revision constitute a fork.
func (c *MembershipChain) DetectFork(revA, revB *MembershipRevision) error {
	if revA.Revision == revB.Revision && revA.PriorDigest == revB.PriorDigest {
		if revA.Digest() != revB.Digest() {
			return fmt.Errorf("%w: competing revisions at rev %d with same prior %s", ErrMembershipFork, revA.Revision, revA.PriorDigest)
		}
	}
	return nil
}

// IsMember returns true if deviceID is an active member in the current head revision.
func (c *MembershipChain) IsMember(deviceID string) bool {
	for _, m := range c.Head.Active {
		if m.DeviceID == deviceID {
			return true
		}
	}
	return false
}
