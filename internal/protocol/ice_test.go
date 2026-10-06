package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestWANW10ICEContractBoundsAndLegacyEncoding(t *testing.T) {
	legacy := NetworkOffer{SenderGeneration: 1, TargetGeneration: 2, Candidates: []NetworkCandidate{}}
	b, e := legacy.Canonical()
	if e != nil || string(b) != string(NetworkCanonical("offer", "1", "2", "0")) {
		t.Fatal("legacy encoding", e)
	}
	wire, _ := json.Marshal(legacy)
	var out NetworkOffer
	if e = NetworkDecode(wire, &out); e != nil {
		t.Fatal("legacy decode", e)
	}
	if NetworkDecode([]byte(`{"sender_generation":"1","target_generation":"2","candidates":[],"ice":null}`), &out) == nil {
		t.Fatal("null extension")
	}
	d := NetworkICE{Mode: "offer", Ufrag: "abcd", Password: strings.Repeat("a", 32), Candidates: []string{"1 1 udp 2130706431 11.23.45.1 12345 typ host"}}
	legacy.ICE = &d
	b, e = legacy.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	wire, _ = json.Marshal(legacy)
	if NetworkDecode(bytes.Replace(wire, []byte(`"ice":`), []byte(`"ICE":`), 1), &out) == nil {
		t.Fatal("case-alias ICE extension accepted")
	}
	if e = NetworkDecode(wire, &out); e != nil {
		t.Fatal(e)
	}
	bad := []string{"1 1 udp 1 127.0.0.1 1234 typ host", "1 1 udp 1 10.0.0.1 1234 typ host", "1 1 udp 1 169.254.169.254 80 typ host", "1 1 udp 1 example.com 1234 typ host", "1 1 tcp 1 11.23.45.1 1234 typ host tcptype passive", "1 1 udp 1 11.23.45.1 1234 typ relay raddr 0.0.0.0 rport 0", "1 1 udp 1 11.23.45.1 1234 typ srflx raddr 10.0.0.1 rport 2222", "1 2 udp 1 11.23.45.1 1234 typ host"}
	for _, s := range bad {
		if ValidateICECandidate(s) == nil {
			t.Fatal("accepted prohibited", s)
		}
	}
	for _, key := range []string{"sender_generation", "target_generation", "candidates"} {
		var obj map[string]any
		_ = json.Unmarshal(wire, &obj)
		delete(obj, key)
		missing, _ := json.Marshal(obj)
		if NetworkDecode(missing, &out) == nil {
			t.Fatal("missing key", key)
		}
	}
	d.Candidates = append(d.Candidates, d.Candidates[0])
	if _, e = d.Canonical(); e == nil {
		t.Fatal("duplicate")
	}
	d.Candidates = []string{}
	if _, e = d.Canonical(); e == nil {
		t.Fatal("empty")
	}
	d.Candidates = []string{}
	for n := range 8 {
		d.Candidates = append(d.Candidates, fmt.Sprintf("1 1 udp 2130706431 11.23.45.1 %d typ host", 12345+n))
	}
	if _, e = d.Canonical(); e != nil {
		t.Fatal("maximum candidate set", e)
	}
	d.Candidates = append(d.Candidates, "1 1 udp 2130706431 11.23.45.1 12399 typ host")
	if _, e = d.Canonical(); e == nil {
		t.Fatal("candidate overflow")
	}
	d.Candidates = []string{}
	d.Mode = "request"
	d.Ufrag = ""
	d.Password = ""
	if _, e = d.Canonical(); e != nil {
		t.Fatal(e)
	}
}
func TestWANW10ICEDeterministicRoleAndPurpose(t *testing.T) {
	q := NetworkOfferRequest{Proof: NetworkProof{Sender: strings.Repeat("1", 64), Target: strings.Repeat("2", 64), SenderPin: strings.Repeat("3", 64), TargetPin: strings.Repeat("4", 64), Purpose: "peer_data", Kind: "offer", Role: "initiator"}, Offer: NetworkOffer{ICE: &NetworkICE{Mode: "offer", Ufrag: "abcd", Password: strings.Repeat("a", 32), Candidates: []string{"1 1 udp 2130706431 11.23.45.1 12345 typ host"}}}}
	if e := q.VerifyICE(); e != nil {
		t.Fatal(e)
	}
	q.Proof.Sender, q.Proof.Target = q.Proof.Target, q.Proof.Sender
	if q.VerifyICE() == nil {
		t.Fatal("wrong controlling role")
	}
	q.Offer.ICE.Mode = "accept"
	q.Proof.Kind = "accept"
	q.Proof.Role = "responder"
	if e := q.VerifyICE(); e != nil {
		t.Fatal(e)
	}
	q.Proof.Purpose = "enrollment"
	if q.VerifyICE() == nil {
		t.Fatal("enrollment ICE")
	}
}

func TestWANW10IndependentSignedICEGolden(t *testing.T) {
	data, e := os.ReadFile("../../schemas/fixtures/ice-v1/offer.json")
	if e != nil {
		t.Fatal(e)
	}
	var request NetworkOfferRequest
	if e = NetworkDecode(data, &request); e != nil {
		t.Fatal(e)
	}
	if e = request.Verify(false, wanFixtureNow); e != nil {
		t.Fatal(e)
	}
	for _, fixture := range []struct {
		name  string
		bytes []byte
	}{{"offer", func() []byte {
		b, e := request.Proof.Canonical(false)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}()}, {"payload", func() []byte {
		b, e := request.Offer.Canonical()
		if e != nil {
			t.Fatal(e)
		}
		return b
	}()}} {
		hexBytes, e := os.ReadFile("../../schemas/fixtures/ice-v1/" + fixture.name + ".hex")
		if e != nil {
			t.Fatal(e)
		}
		want, e := hex.DecodeString(strings.TrimSpace(string(hexBytes)))
		if e != nil || !bytes.Equal(want, fixture.bytes) {
			t.Fatal("independent golden", fixture.name, e)
		}
	}
}
