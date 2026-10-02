package model

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
)

func generateTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	return pub, priv
}

func TestOrbitMembershipSequentialRollout(t *testing.T) {
	ownerPub, ownerPriv := generateTestKeyPair(t)
	dev1Pub, _ := generateTestKeyPair(t)
	dev2Pub, _ := generateTestKeyPair(t)
	dev3Pub, _ := generateTestKeyPair(t)

	dev1ID := hex.EncodeToString(dev1Pub)
	dev2ID := hex.EncodeToString(dev2Pub)
	dev3ID := hex.EncodeToString(dev3Pub)

	chain, err := NewMembershipChain("ws-100", []MemberRecord{
		{DeviceID: dev1ID, KeyPin: "pin-dev1"},
	}, ownerPub, ownerPriv)
	if err != nil {
		t.Fatalf("unexpected error creating chain: %v", err)
	}

	if !chain.IsMember(dev1ID) {
		t.Fatal("expected dev1 to be active member")
	}
	if chain.IsMember(dev2ID) {
		t.Fatal("expected dev2 not to be active member yet")
	}

	// Revision 2: add dev2
	rev2 := &MembershipRevision{
		WorkspaceID: "ws-100",
		Revision:    2,
		PriorDigest: chain.Head.Digest(),
		Active: []MemberRecord{
			{DeviceID: dev1ID, KeyPin: "pin-dev1"},
			{DeviceID: dev2ID, KeyPin: "pin-dev2"},
		},
		AuthorID: hex.EncodeToString(ownerPub),
	}
	rev2.Sign(ownerPriv)

	if err := chain.ApplyRevision(rev2); err != nil {
		t.Fatalf("failed to apply rev2: %v", err)
	}
	if !chain.IsMember(dev2ID) {
		t.Fatal("expected dev2 to be active member after rev2")
	}

	// Revision 3: add dev3 and retire dev1
	rev3 := &MembershipRevision{
		WorkspaceID: "ws-100",
		Revision:    3,
		PriorDigest: chain.Head.Digest(),
		Active: []MemberRecord{
			{DeviceID: dev2ID, KeyPin: "pin-dev2"},
			{DeviceID: dev3ID, KeyPin: "pin-dev3"},
		},
		Retired:  []string{dev1ID},
		AuthorID: hex.EncodeToString(ownerPub),
	}
	rev3.Sign(ownerPriv)

	if err := chain.ApplyRevision(rev3); err != nil {
		t.Fatalf("failed to apply rev3: %v", err)
	}
	if chain.IsMember(dev1ID) {
		t.Fatal("expected dev1 to be retired")
	}
	if !chain.IsMember(dev3ID) {
		t.Fatal("expected dev3 to be active")
	}

	// Invariant: Retired device cannot be revived
	rev4Revival := &MembershipRevision{
		WorkspaceID: "ws-100",
		Revision:    4,
		PriorDigest: chain.Head.Digest(),
		Active: []MemberRecord{
			{DeviceID: dev1ID, KeyPin: "pin-dev1"}, // dev1 was retired in rev3!
			{DeviceID: dev2ID, KeyPin: "pin-dev2"},
			{DeviceID: dev3ID, KeyPin: "pin-dev3"},
		},
		AuthorID: hex.EncodeToString(ownerPub),
	}
	rev4Revival.Sign(ownerPriv)

	err = chain.ApplyRevision(rev4Revival)
	if !errors.Is(err, ErrRetiredMemberRevival) {
		t.Fatalf("expected ErrRetiredMemberRevival, got: %v", err)
	}
}

func TestOrbitMembershipCompetingAdministrationForks(t *testing.T) {
	ownerPub, ownerPriv := generateTestKeyPair(t)
	dev1Pub, _ := generateTestKeyPair(t)
	dev2Pub, _ := generateTestKeyPair(t)
	dev3Pub, _ := generateTestKeyPair(t)

	dev1ID := hex.EncodeToString(dev1Pub)
	dev2ID := hex.EncodeToString(dev2Pub)
	dev3ID := hex.EncodeToString(dev3Pub)

	chain, err := NewMembershipChain("ws-fork", []MemberRecord{
		{DeviceID: dev1ID, KeyPin: "pin-1"},
	}, ownerPub, ownerPriv)
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	baseDigest := chain.Head.Digest()

	// Branch A: partitioned admin approves dev2
	rev2A := &MembershipRevision{
		WorkspaceID: "ws-fork",
		Revision:    2,
		PriorDigest: baseDigest,
		Active: []MemberRecord{
			{DeviceID: dev1ID, KeyPin: "pin-1"},
			{DeviceID: dev2ID, KeyPin: "pin-2"},
		},
		AuthorID: hex.EncodeToString(ownerPub),
	}
	rev2A.Sign(ownerPriv)

	// Branch B: another partitioned admin approves dev3 concurrently
	rev2B := &MembershipRevision{
		WorkspaceID: "ws-fork",
		Revision:    2,
		PriorDigest: baseDigest,
		Active: []MemberRecord{
			{DeviceID: dev1ID, KeyPin: "pin-1"},
			{DeviceID: dev3ID, KeyPin: "pin-3"},
		},
		AuthorID: hex.EncodeToString(ownerPub),
	}
	rev2B.Sign(ownerPriv)

	// Oracle fork detection
	forkErr := chain.DetectFork(rev2A, rev2B)
	if !errors.Is(forkErr, ErrMembershipFork) {
		t.Fatalf("expected ErrMembershipFork, got: %v", forkErr)
	}

	// Apply Branch A
	if err := chain.ApplyRevision(rev2A); err != nil {
		t.Fatalf("failed to apply rev2A: %v", err)
	}

	// Attempting to apply rev2B directly now fails because revision is not monotonic / prior digest mismatch
	err = chain.ApplyRevision(rev2B)
	if !errors.Is(err, ErrRevisionNotMonotonic) && !errors.Is(err, ErrPriorDigestMismatch) {
		t.Fatalf("expected ErrRevisionNotMonotonic or ErrPriorDigestMismatch when applying competing rev, got: %v", err)
	}

	// Explicit reconciliation by owner: creates linear Rev 3 incorporating both additions
	rev3Reconciliation := &MembershipRevision{
		WorkspaceID: "ws-fork",
		Revision:    3,
		PriorDigest: chain.Head.Digest(),
		Active: []MemberRecord{
			{DeviceID: dev1ID, KeyPin: "pin-1"},
			{DeviceID: dev2ID, KeyPin: "pin-2"},
			{DeviceID: dev3ID, KeyPin: "pin-3"},
		},
		AuthorID: hex.EncodeToString(ownerPub),
	}
	rev3Reconciliation.Sign(ownerPriv)

	if err := chain.ApplyRevision(rev3Reconciliation); err != nil {
		t.Fatalf("expected reconciliation revision 3 to succeed, got: %v", err)
	}
	if !chain.IsMember(dev2ID) || !chain.IsMember(dev3ID) {
		t.Fatal("expected both dev2 and dev3 to be members after explicit reconciliation")
	}
}

func TestOrbitMembershipUnauthorizedOrForgedSignatures(t *testing.T) {
	ownerPub, ownerPriv := generateTestKeyPair(t)
	_, attackerPriv := generateTestKeyPair(t)

	chain, err := NewMembershipChain("ws-auth", []MemberRecord{
		{DeviceID: "dev1", KeyPin: "pin1"},
	}, ownerPub, ownerPriv)
	if err != nil {
		t.Fatal(err)
	}

	forgedRev := &MembershipRevision{
		WorkspaceID: "ws-auth",
		Revision:    2,
		PriorDigest: chain.Head.Digest(),
		Active: []MemberRecord{
			{DeviceID: "dev1", KeyPin: "pin1"},
			{DeviceID: "attacker", KeyPin: "attacker-pin"},
		},
		AuthorID: "attacker-id",
	}
	// Signed by attacker, not owner
	forgedRev.Sign(attackerPriv)

	err = chain.ApplyRevision(forgedRev)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for forged revision, got: %v", err)
	}
}
