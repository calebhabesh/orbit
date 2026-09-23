package designgates

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type fixedID [32]byte

type member struct {
	device fixedID
	keyPin fixedID
}

type retirementRef struct {
	device          fixedID
	retiredRevision uint64
	snapshotDigest  fixedID
}

type membershipRevision struct {
	folder      fixedID
	revision    uint64
	priorDigest fixedID
	active      []member
	retired     []retirementRef
}

type retiredVersion struct {
	counter        uint64
	envelopeDigest fixedID
}

type retirementSnapshot struct {
	folder            fixedID
	configurationRev  uint64
	retiredDevice     fixedID
	acceptedByRetiree []retiredVersion
}

func canonicalMembership(revision membershipRevision) ([]byte, error) {
	active := append([]member(nil), revision.active...)
	sort.Slice(active, func(i, j int) bool { return bytes.Compare(active[i].device[:], active[j].device[:]) < 0 })
	retired := append([]retirementRef(nil), revision.retired...)
	sort.Slice(retired, func(i, j int) bool { return bytes.Compare(retired[i].device[:], retired[j].device[:]) < 0 })
	if len(active) > 16 || len(retired) > 64 {
		return nil, errors.New("membership exceeds v1 limits")
	}
	for i := 1; i < len(active); i++ {
		if active[i-1].device == active[i].device {
			return nil, errors.New("duplicate active device")
		}
	}
	for i := 1; i < len(retired); i++ {
		if retired[i-1].device == retired[i].device {
			return nil, errors.New("duplicate retired device")
		}
	}
	for _, activeMember := range active {
		for _, retiredMember := range retired {
			if activeMember.device == retiredMember.device {
				return nil, errors.New("device is both active and retired")
			}
		}
	}

	var output bytes.Buffer
	output.WriteString("filesync-membership-v1\x00")
	output.Write(revision.folder[:])
	writeU64(&output, revision.revision)
	output.Write(revision.priorDigest[:])
	writeU16(&output, uint16(len(active)))
	for _, activeMember := range active {
		output.Write(activeMember.device[:])
		output.Write(activeMember.keyPin[:])
	}
	writeU16(&output, uint16(len(retired)))
	for _, retiredMember := range retired {
		output.Write(retiredMember.device[:])
		writeU64(&output, retiredMember.retiredRevision)
		output.Write(retiredMember.snapshotDigest[:])
	}
	return output.Bytes(), nil
}

func canonicalRetirementSnapshot(snapshot retirementSnapshot) ([]byte, error) {
	versions := append([]retiredVersion(nil), snapshot.acceptedByRetiree...)
	sort.Slice(versions, func(i, j int) bool { return versions[i].counter < versions[j].counter })
	for i := 1; i < len(versions); i++ {
		if versions[i-1].counter == versions[i].counter {
			return nil, errors.New("duplicate retired author counter")
		}
	}
	var output bytes.Buffer
	output.WriteString("filesync-retirement-v1\x00")
	output.Write(snapshot.folder[:])
	writeU64(&output, snapshot.configurationRev)
	output.Write(snapshot.retiredDevice[:])
	writeU64(&output, uint64(len(versions)))
	for _, version := range versions {
		writeU64(&output, version.counter)
		output.Write(version.envelopeDigest[:])
	}
	return output.Bytes(), nil
}

func writeU16(output *bytes.Buffer, value uint16) {
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], value)
	output.Write(encoded[:])
}

func writeU64(output *bytes.Buffer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	output.Write(encoded[:])
}

func testID(label byte) fixedID {
	var id fixedID
	for index := range id {
		id[index] = label
	}
	return id
}

func fixtureMembership(t *testing.T) (membershipRevision, retirementSnapshot) {
	t.Helper()
	snapshot := retirementSnapshot{
		folder:           testID('F'),
		configurationRev: 7,
		retiredDevice:    testID('B'),
		acceptedByRetiree: []retiredVersion{
			{counter: 9, envelopeDigest: testID('y')},
			{counter: 2, envelopeDigest: testID('x')},
		},
	}
	snapshotBytes, err := canonicalRetirementSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotHash := sha256.Sum256(snapshotBytes)
	revision := membershipRevision{
		folder:      testID('F'),
		revision:    8,
		priorDigest: testID('P'),
		active: []member{
			{device: testID('C'), keyPin: testID('c')},
			{device: testID('A'), keyPin: testID('a')},
		},
		retired: []retirementRef{
			{device: testID('B'), retiredRevision: 8, snapshotDigest: snapshotHash},
		},
	}
	return revision, snapshot
}

func TestD3CanonicalMembershipAndRetirementFixtures(t *testing.T) {
	revision, snapshot := fixtureMembership(t)
	membershipBytes, err := canonicalMembership(revision)
	if err != nil {
		t.Fatal(err)
	}
	snapshotBytes, err := canonicalRetirementSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	assertHexFixture(t, "membership-revision-v1.hex", membershipBytes)
	assertHexFixture(t, "retirement-snapshot-v1.hex", snapshotBytes)
	t.Logf("membership sha256=%x", sha256.Sum256(membershipBytes))
	t.Logf("retirement sha256=%x", sha256.Sum256(snapshotBytes))
}

func TestD3CanonicalEncodingIgnoresInputOrderAndRejectsDuplicates(t *testing.T) {
	revision, snapshot := fixtureMembership(t)
	first, err := canonicalMembership(revision)
	if err != nil {
		t.Fatal(err)
	}
	revision.active[0], revision.active[1] = revision.active[1], revision.active[0]
	second, err := canonicalMembership(revision)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("active member input order changed canonical encoding")
	}
	revision.active = append(revision.active, revision.active[0])
	if _, err := canonicalMembership(revision); err == nil {
		t.Fatal("duplicate member was accepted")
	}

	snapshot.acceptedByRetiree[0], snapshot.acceptedByRetiree[1] = snapshot.acceptedByRetiree[1], snapshot.acceptedByRetiree[0]
	if _, err := canonicalRetirementSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.acceptedByRetiree = append(snapshot.acceptedByRetiree, snapshot.acceptedByRetiree[0])
	if _, err := canonicalRetirementSnapshot(snapshot); err == nil {
		t.Fatal("duplicate retired counter was accepted")
	}
}

type retirementPolicy struct {
	active             map[fixedID]bool
	retiredAccepted    map[fixedID]map[uint64]fixedID
	membershipDigestOK bool
}

func (policy retirementPolicy) admit(author fixedID, counter uint64, envelopeDigest fixedID, parentsAdmitted bool) error {
	if !policy.membershipDigestOK {
		return errors.New("membership mismatch: exchange paused")
	}
	if policy.active[author] {
		if !parentsAdmitted {
			return errors.New("unknown or rejected ancestor")
		}
		return nil
	}
	accepted, retired := policy.retiredAccepted[author]
	if !retired {
		return errors.New("unknown author")
	}
	knownDigest, known := accepted[counter]
	if !known || knownDigest != envelopeDigest {
		return errors.New("retired-author version absent from approved snapshot")
	}
	if !parentsAdmitted {
		return errors.New("unknown or rejected ancestor")
	}
	return nil
}

func TestD3RetirementRejectsOldEpochResurrection(t *testing.T) {
	a, b, c := testID('A'), testID('B'), testID('C')
	acceptedDigest := testID('x')
	policy := retirementPolicy{
		active:             map[fixedID]bool{a: true, c: true},
		retiredAccepted:    map[fixedID]map[uint64]fixedID{b: {2: acceptedDigest}},
		membershipDigestOK: true,
	}
	if err := policy.admit(b, 2, acceptedDigest, true); err != nil {
		t.Fatalf("approved retired history was rejected: %v", err)
	}
	if err := policy.admit(b, 3, testID('z'), true); err == nil {
		t.Fatal("previously unseen retired-author history was admitted")
	}
	if err := policy.admit(c, 4, testID('q'), false); err == nil {
		t.Fatal("successor depending on rejected ancestry was admitted")
	}
	policy.membershipDigestOK = false
	if err := policy.admit(a, 4, testID('q'), true); err == nil {
		t.Fatal("stale configuration did not pause exchange")
	}
	t.Log("known retired envelopes remain forwardable; unknown old-epoch versions and their successor histories are blocked")
}

func assertHexFixture(t *testing.T, name string, actual []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	wantText, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v\nactual hex:\n%s", name, err, hex.EncodeToString(actual))
	}
	want, err := hex.DecodeString(strings.TrimSpace(string(wantText)))
	if err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatalf("fixture %s changed\nwant: %s\n got: %s", name, hex.EncodeToString(want), hex.EncodeToString(actual))
	}
}
