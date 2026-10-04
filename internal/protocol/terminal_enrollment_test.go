package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestTerminalT01EnrollmentWire(t *testing.T) {
	id := func(s string) string { return strings.Repeat(s, 64) }
	w := TerminalEnrollmentWire{Version: "2", Folder: id("1"), Inviter: id("2"), InviterPin: id("3"), Token: id("4"), Attempt: id("5"), Challenge: id("6"), Requester: id("7"), RequesterPin: id("8"), PublicKey: id("9"), CertificateDER: "validated-by-T03", PriorMembership: id("a"), ExpiresUnix: "1791072000", EnrollmentEndpoint: "https://host:7444", PeerEndpoint: "https://host:7443", Label: "Laptop", Signature: strings.Repeat("b", 128)}
	b, _ := json.Marshal(w)
	var got TerminalEnrollmentWire
	if err := DecodeStrict(b, &got); err != nil {
		t.Fatal(err)
	}
	tr, err := got.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	token, _ := hex.DecodeString(w.Token)
	if tr.TokenDigest != sha256.Sum256(token) {
		t.Fatal("token digest convention")
	}
	for _, change := range []func(*TerminalEnrollmentWire){func(v *TerminalEnrollmentWire) { v.Version = "1" }, func(v *TerminalEnrollmentWire) { v.ExpiresUnix = "01791072000" }, func(v *TerminalEnrollmentWire) { v.Signature = "ab" }, func(v *TerminalEnrollmentWire) { v.PeerEndpoint = "http://host" }, func(v *TerminalEnrollmentWire) { v.Requester = strings.ToUpper(id("a")) }} {
		bad := w
		change(&bad)
		if _, err := bad.Transcript(); err == nil {
			t.Fatal("invalid transcript accepted")
		}
	}
}

func TestTerminalT01StatusProofDomain(t *testing.T) {
	var request, nonce [32]byte
	request[0] = 1
	nonce[0] = 2
	a, err := TerminalStatusTranscript(request, nonce, 42)
	if err != nil {
		t.Fatal(err)
	}
	request[0] = 3
	b, _ := TerminalStatusTranscript(request, nonce, 42)
	if string(a) == string(b) || !strings.HasPrefix(string(a), "orbit-enrollment-status-v2\x00") {
		t.Fatal("status identity/domain omitted")
	}
	if _, err := TerminalStatusTranscript([32]byte{}, nonce, 42); err == nil {
		t.Fatal("zero request accepted")
	}
}
