package protocol

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestTerminalT03MembershipDecoder(t *testing.T) {
	raw, err := os.ReadFile("../../tests/designgates/testdata/membership-revision-v1.hex")
	if err != nil {
		t.Fatal(err)
	}
	golden, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := DecodeMembership(golden)
	if err != nil || m.Revision != 8 || len(m.Active) != 2 || len(m.Retired) != 1 {
		t.Fatal("frozen membership could not be decoded")
	}
	encoded, err := EncodeMembership(m)
	if err != nil || !bytes.Equal(encoded, golden) {
		t.Fatal("decoder changed canonical bytes")
	}
	for i := 0; i < len(golden); i++ {
		if _, err := DecodeMembership(golden[:i]); err == nil {
			t.Fatal("truncated membership accepted")
		}
	}
	if _, err := DecodeMembership(append(append([]byte{}, golden...), 0)); err == nil {
		t.Fatal("trailing membership data accepted")
	}
	unsorted := append([]byte{}, golden...)
	offset := len("filesync-membership-v1\x00") + 32 + 8 + 32 + 2
	first := append([]byte{}, unsorted[offset:offset+64]...)
	copy(unsorted[offset:offset+64], unsorted[offset+64:offset+128])
	copy(unsorted[offset+64:offset+128], first)
	if _, err := DecodeMembership(unsorted); err == nil {
		t.Fatal("noncanonical member order accepted")
	}
}
