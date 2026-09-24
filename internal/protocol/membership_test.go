package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
)

func repeatedID(value byte) history.ID {
	var id history.ID
	for i := range id {
		id[i] = value
	}
	return id
}

func repeatedDigest(value byte) history.Digest { return history.Digest(repeatedID(value)) }

func TestMembershipEncodingMatchesP01GoldenFixture(t *testing.T) {
	retirementHex, err := os.ReadFile("../../tests/designgates/testdata/retirement-snapshot-v1.hex")
	if err != nil {
		t.Fatal(err)
	}
	retirement, err := hex.DecodeString(strings.TrimSpace(string(retirementHex)))
	if err != nil {
		t.Fatal(err)
	}
	membership := Membership{
		Folder:      repeatedID('F'),
		Revision:    8,
		PriorDigest: repeatedDigest('P'),
		Active: []ActiveMember{
			{Device: repeatedID('C'), KeyPin: repeatedDigest('c')},
			{Device: repeatedID('A'), KeyPin: repeatedDigest('a')},
		},
		Retired: []RetiredMember{{Device: repeatedID('B'), RetiredAt: 8, SnapshotDigest: sha256.Sum256(retirement)}},
	}
	got, err := EncodeMembership(membership)
	if err != nil {
		t.Fatal(err)
	}
	wantHex, err := os.ReadFile("../../tests/designgates/testdata/membership-revision-v1.hex")
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != strings.TrimSpace(string(wantHex)) {
		t.Fatal("production membership encoding differs from the P01 golden fixture")
	}
}

func TestRetirementSnapshotEncodingMatchesP01GoldenFixture(t *testing.T) {
	snapshot := RetirementSnapshot{
		Folder:           repeatedID('F'),
		ConfigurationRev: 7,
		RetiredDevice:    repeatedID('B'),
		AcceptedByRetiree: []RetiredVersion{
			{Counter: 9, EnvelopeDigest: repeatedDigest('y')},
			{Counter: 2, EnvelopeDigest: repeatedDigest('x')},
		},
	}
	got, err := EncodeRetirementSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	wantHex, err := os.ReadFile("../../tests/designgates/testdata/retirement-snapshot-v1.hex")
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != strings.TrimSpace(string(wantHex)) {
		t.Fatalf("production retirement snapshot differs from golden fixture\ngot:  %s\nwant: %s", hex.EncodeToString(got), strings.TrimSpace(string(wantHex)))
	}

	decoded, err := DecodeRetirementSnapshot(got)
	if err != nil {
		t.Fatalf("decode retirement snapshot: %v", err)
	}
	if decoded.Folder != snapshot.Folder || decoded.ConfigurationRev != snapshot.ConfigurationRev || decoded.RetiredDevice != snapshot.RetiredDevice || len(decoded.AcceptedByRetiree) != 2 {
		t.Fatalf("decoded snapshot mismatch: %+v", decoded)
	}
	if decoded.AcceptedByRetiree[0].Counter != 2 || decoded.AcceptedByRetiree[1].Counter != 9 {
		t.Fatalf("decoded versions not sorted: %+v", decoded.AcceptedByRetiree)
	}

	digest, err := RetirementSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if digest != sha256.Sum256(got) {
		t.Fatal("digest mismatch")
	}
}
