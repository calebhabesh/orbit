package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const wanFixtureNow uint64 = 1800000000

func wanFixture(t *testing.T, name string, out any) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("../../schemas/fixtures/network-v1", name+".json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = NetworkDecode(b, out); e != nil {
		t.Fatal(e)
	}
	h, e := os.ReadFile(filepath.Join("../../schemas/fixtures/network-v1", name+".hex"))
	if e != nil {
		t.Fatal(e)
	}
	gold, e := hex.DecodeString(strings.TrimSpace(string(h)))
	if e != nil {
		t.Fatal(e)
	}
	return gold
}
func wanKey() ed25519.PrivateKey { return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32)) }
func wanID(c string) string      { return strings.Repeat(c, 64) }
func TestWANW01CanonicalGoldens(t *testing.T) {
	key := wanKey().Public().(ed25519.PublicKey)
	var p NetworkProfile
	gold := wanFixture(t, "profile", &p)
	got, e := p.Canonical(false)
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("profile golden %v", e)
	}
	if e = p.Verify(key, wanFixtureNow, 1, false); e != nil {
		t.Fatal(e)
	}
	var q NetworkProof
	gold = wanFixture(t, "proof", &q)
	got, e = q.Canonical(false)
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("proof golden %v", e)
	}
	if e = q.Verify(false, wanFixtureNow); e != nil {
		t.Fatal(e)
	}
	var a NetworkAnnouncement
	gold = wanFixture(t, "announcement", &a)
	got, e = a.Canonical(wanFixtureNow, true)
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("announcement golden %v", e)
	}
	if NetworkDigest(got) != q.Payload {
		t.Fatal("payload binding")
	}
	var token RelayAttachment
	gold = wanFixture(t, "attachment", &token)
	got, e = token.Canonical(false)
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("attachment golden %v", e)
	}
	if e = token.Verify(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{16}, 32)).Public().(ed25519.PublicKey), wanFixtureNow, false); e != nil {
		t.Fatal(e)
	}
	var w RoutedEnrollmentRequest
	gold = wanFixture(t, "enrollment-request", &w)
	got, e = w.Transcript.Canonical()
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("v3 golden %v", e)
	}
	if e = w.Verify(wanFixtureNow); e != nil {
		t.Fatal(e)
	}
	var s RoutedEnrollmentStatus
	gold = wanFixture(t, "enrollment-status", &s)
	got, e = s.Canonical()
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("status golden %v", e)
	}
	id, e := w.Transcript.RequestID()
	if e != nil || id != s.Request {
		t.Fatal("request identity")
	}
	var approval RoutedEnrollmentApproval
	gold = wanFixture(t, "enrollment-approval", &approval)
	got, e = approval.Canonical()
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("approval golden %v", e)
	}
	var i RoutedInvitation
	b, e := os.ReadFile("../../schemas/fixtures/network-v1/invitation.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = NetworkDecode(b, &i); e != nil {
		t.Fatal(e)
	}
	if e = i.Validate(wanFixtureNow, false); e != nil {
		t.Fatal(e)
	}
}
func TestWANW01StrictNetworkCodec(t *testing.T) {
	for _, b := range []string{`{"generation":"01"}`, `{"generation":1}`, `{"generation":"18446744073709551616"}`, `{"generation":"-1"}`, `{"generation":"1","generation":"1"}`, `{"Generation":"1","expires":"1800000600","candidates":[],"capabilities":[],"relay":false}`, `{"generation":"1","expires":"1800000600","candidates":null,"capabilities":[],"relay":false}`, `{"generation":"1","expires":"1800000600","candidates":[],"capabilities":[],"relay":false,"extra":0}`} {
		var a NetworkAnnouncement
		if e := NetworkDecode([]byte(b), &a); e == nil {
			t.Fatalf("accepted %s", b)
		}
	}
	var n NetworkUint
	if e := json.Unmarshal([]byte(`"18446744073709551615"`), &n); e != nil || uint64(n) != ^uint64(0) {
		t.Fatal("maximum uint64", e)
	}
	// Exactly 64 KiB with whitespace is valid; one additional byte is refused.
	var a NetworkAnnouncement
	b := []byte(`{"generation":"1","expires":"1800000600","candidates":[],"capabilities":[],"relay":false}`)
	b = append(b, bytes.Repeat([]byte(" "), NetworkMaxBytes-len(b))...)
	if e := NetworkDecode(b, &a); e != nil {
		t.Fatal(e)
	}
	if e := NetworkDecode(append(b, ' '), &a); e == nil {
		t.Fatal("body over limit")
	}
}
func TestWANW01MaximumMessages(t *testing.T) {
	var p NetworkProfile
	wanFixture(t, "profile", &p)
	p.Operator = strings.Repeat("o", 128)
	p.Privacy = strings.Repeat("p", 4096)
	p.Epoch = NetworkUint(^uint64(0))
	p.Expires = NetworkUint(^uint64(0))
	p.Origins = []string{"https://a.example.org", "https://b.example.org", "wss://a.example.org", "wss://b.example.org"}
	p.STUN = []string{"8.8.8.8:1", "8.8.8.8:2", "8.8.8.8:3", "8.8.8.8:4"}
	if _, e := p.Canonical(false); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*NetworkProfile){func(p *NetworkProfile) { p.Privacy += "x" }, func(p *NetworkProfile) { p.Operator += "x" }, func(p *NetworkProfile) { p.Origins = append(p.Origins, "https://c.example.org") }, func(p *NetworkProfile) { p.STUN = append(p.STUN, "8.8.8.8:5") }} {
		bad := p
		change(&bad)
		if _, e := bad.Canonical(false); e == nil {
			t.Fatal("over-limit profile accepted")
		}
	}
	a := NetworkAnnouncement{Generation: NetworkUint(^uint64(0)), Expires: NetworkUint(wanFixtureNow + 600), Candidates: []NetworkCandidate{}, Capabilities: []string{"direct_https_v1", "relay_inner_tls_v1", "enrollment_v3", "quic_ice_v1"}}
	for i := 0; i < 16; i++ {
		a.Candidates = append(a.Candidates, NetworkCandidate{Transport: "tcp", Address: "8.8.8.8:" + string(rune('1'+i%9)) + string(rune('1'+i/9)), Scope: "public"})
	}
	if _, e := a.Canonical(wanFixtureNow, true); e != nil {
		t.Fatal(e)
	}
	a.Candidates = append(a.Candidates, NetworkCandidate{Transport: "tcp", Address: "8.8.8.8:99", Scope: "public"})
	if _, e := a.Canonical(wanFixtureNow, true); e == nil {
		t.Fatal("candidate cap")
	}
	var w RoutedEnrollmentRequest
	wanFixture(t, "enrollment-request", &w)
	w.Transcript.Label = strings.Repeat("x", 256)
	w.Transcript.Expires = NetworkUint(^uint64(0))
	if _, e := w.Transcript.Canonical(); e != nil {
		t.Fatal(e)
	}
	w.Transcript.Label += "x"
	if _, e := w.Transcript.Canonical(); e == nil {
		t.Fatal("label cap")
	}
}
func TestWANW01ProofSubstitution(t *testing.T) {
	var q NetworkProof
	wanFixture(t, "proof", &q)
	for _, change := range []func(*NetworkProof){func(p *NetworkProof) { p.Sender = wanID("f") }, func(p *NetworkProof) { p.SenderPin = wanID("f") }, func(p *NetworkProof) { p.Profile = wanID("f") }, func(p *NetworkProof) { p.Origin = "https://other.example.org" }, func(p *NetworkProof) { p.Purpose = "enrollment" }, func(p *NetworkProof) { p.Payload = wanID("f") }, func(p *NetworkProof) { p.Expires++ }, func(p *NetworkProof) { p.Challenge = wanID("f") }, func(p *NetworkProof) { p.Operation = wanID("f") }, func(p *NetworkProof) { p.Generation++ }} {
		bad := q
		change(&bad)
		if e := bad.Verify(false, wanFixtureNow); e == nil {
			t.Fatal("substitution accepted")
		}
	}
	if e := q.Verify(false, uint64(q.Expires)); e == nil {
		t.Fatal("expired proof")
	}
	var token RelayAttachment
	wanFixture(t, "attachment", &token)
	// Every role/identity/purpose/epoch binding must affect the signed bytes.
	v := reflect.ValueOf(token)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if typ.Field(i).Name == "Signature" {
			continue
		}
		bad := token
		f := reflect.ValueOf(&bad).Elem().Field(i)
		if f.Kind() == reflect.String {
			f.SetString(f.String() + "x")
		} else {
			f.SetUint(f.Uint() + 1)
		}
		if e := bad.Verify(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{16}, 32)).Public().(ed25519.PublicKey), wanFixtureNow, false); e == nil {
			t.Fatalf("token %s not bound", typ.Field(i).Name)
		}
	}
}
func TestWANW01URLScope(t *testing.T) {
	for _, origin := range []string{"http://example.org", "https://127.0.0.1", "https://[::1]", "https://localhost", "https://a.localhost", "https://169.254.169.254", "https://10.0.0.1", "https://100.64.0.1", "https://192.0.2.1", "https://[::ffff:8.8.8.8]", "https://example.org/api/v1/status", "https://user@example.org", "https://example.org?x=1", "https://example.org#x", "https://EXAMPLE.org", "https://example.org:443", "https://example.org.", "https://example.org:00080"} {
		if e := ServiceOrigin(origin, false); e == nil {
			t.Fatalf("origin accepted %s", origin)
		}
	}
	if e := ServiceOrigin("https://10.0.0.1:7443", true); e != nil {
		t.Fatal(e)
	}
	c := NetworkCandidate{Transport: "tcp", Address: "10.0.0.1:7443", Scope: "lan", Interface: "eth0"}
	if e := c.Validate(false); e != nil {
		t.Fatal(e)
	}
	if e := c.Validate(true); e == nil {
		t.Fatal("published LAN candidate")
	}
	for _, address := range []string{"127.0.0.1:7443", "[::]:7443", "8.8.8.8:0", "[::ffff:8.8.8.8]:7443", "224.0.0.1:7443", "255.255.255.255:7443", "[fe80::1%eth0]:7443"} {
		c.Address = address
		c.Scope = "public"
		c.Interface = ""
		if e := c.Validate(false); e == nil {
			t.Fatalf("candidate accepted %s", address)
		}
	}
}
func TestWANW01RoutedBindingsAndV2Coexistence(t *testing.T) {
	var w RoutedEnrollmentRequest
	wanFixture(t, "enrollment-request", &w)
	oldID, _ := w.Transcript.RequestID()
	for _, change := range []func(*RoutedEnrollmentRequest){func(w *RoutedEnrollmentRequest) { w.Transcript.Folder = wanID("f") }, func(w *RoutedEnrollmentRequest) { w.Transcript.Inviter.Pin = wanID("f") }, func(w *RoutedEnrollmentRequest) { w.Transcript.Requester.Profile = wanID("f") }, func(w *RoutedEnrollmentRequest) { w.Transcript.PriorMembership = wanID("f") }, func(w *RoutedEnrollmentRequest) { w.Capability = wanID("f") }, func(w *RoutedEnrollmentRequest) { w.Transcript.Attempt = wanID("f") }, func(w *RoutedEnrollmentRequest) { w.Transcript.Label = "different review" }, func(w *RoutedEnrollmentRequest) { w.Transcript.Expires++ }} {
		bad := w
		change(&bad)
		if e := bad.Verify(wanFixtureNow); e == nil {
			t.Fatal("v3 substitution")
		}
	}
	w.Transcript.Folder = wanID("e")
	newID, _ := w.Transcript.RequestID()
	if oldID == newID {
		t.Fatal("same-key folders collided")
	}
	if e := w.Verify(uint64(w.Transcript.Expires)); e == nil {
		t.Fatal("expired request")
	}
	// V2 decoder cannot silently interpret a v3 request as endpoint-bound v2.
	b, _ := json.Marshal(w)
	var v2 TerminalEnrollmentWire
	if e := DecodeStrict(b, &v2); e == nil {
		t.Fatal("implicit downgrade")
	}
}

func TestWANW01ProfileAuthorityExpiryAndRotation(t *testing.T) {
	var p NetworkProfile
	wanFixture(t, "profile", &p)
	authority := wanKey().Public().(ed25519.PublicKey)
	for _, test := range []struct {
		name     string
		now, min uint64
		key      ed25519.PublicKey
	}{{"expired", uint64(p.Expires), 1, authority}, {"rollback", wanFixtureNow, 2, authority}, {"missing-authority", wanFixtureNow, 1, nil}, {"self-supplied-authority", wanFixtureNow, 1, ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, 32)).Public().(ed25519.PublicKey)}} {
		t.Run(test.name, func(t *testing.T) {
			if e := p.Verify(test.key, test.now, test.min, false); e == nil {
				t.Fatal("untrusted profile accepted")
			}
		})
	}
	for _, change := range []func(*NetworkProfile){func(p *NetworkProfile) { p.ServiceKey = wanID("b") }, func(p *NetworkProfile) { p.Epoch++ }, func(p *NetworkProfile) { p.Privacy = "different operator text" }, func(p *NetworkProfile) { p.Origins = []string{"https://other.example.org"} }} {
		bad := p
		change(&bad)
		if e := bad.Verify(authority, wanFixtureNow, 1, false); e == nil {
			t.Fatal("profile substitution")
		}
	}
}
func TestWANW01OfferMaximum(t *testing.T) {
	o := NetworkOffer{SenderGeneration: NetworkUint(^uint64(0)), TargetGeneration: NetworkUint(^uint64(0)), Candidates: []NetworkCandidate{}}
	for i := 1; i <= 8; i++ {
		o.Candidates = append(o.Candidates, NetworkCandidate{Transport: "tcp", Address: fmt.Sprintf("8.8.8.8:%d", i), Scope: "public"})
	}
	if _, e := o.Canonical(); e != nil {
		t.Fatal(e)
	}
	o.Candidates = append(o.Candidates, NetworkCandidate{Transport: "tcp", Address: "8.8.8.8:9", Scope: "public"})
	if _, e := o.Canonical(); e == nil {
		t.Fatal("offer over limit")
	}
}
func TestWANW01CertificateMaximum(t *testing.T) {
	key := wanKey()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Unix(int64(wanFixtureNow-1), 0), NotAfter: time.Unix(int64(wanFixtureNow+600), 0)}
	var der []byte
	for size := 3600; size < 4100; size++ {
		template.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 3, 4}, Value: make([]byte, size)}}
		candidate, e := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if e != nil {
			t.Fatal(e)
		}
		if len(candidate) == NetworkMaxCertificateBytes {
			der = candidate
			break
		}
	}
	if der == nil {
		t.Fatal("maximum certificate fixture unavailable")
	}
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	pin := NetworkDigest(cert.RawSubjectPublicKeyInfo)
	if _, _, e := NetworkCertificate(base64.StdEncoding.EncodeToString(der), pin, wanFixtureNow); e != nil {
		t.Fatal(e)
	}
	if _, _, e := NetworkCertificate(base64.StdEncoding.EncodeToString(append(der, 0)), pin, wanFixtureNow); e == nil {
		t.Fatal("certificate over limit")
	}
}

func TestWANW01CrossEnvelopeBindings(t *testing.T) {
	var proof NetworkProof
	wanFixture(t, "proof", &proof)
	var announcement NetworkAnnouncement
	wanFixture(t, "announcement", &announcement)
	r := NetworkAnnounceRequest{Proof: proof, Announcement: announcement}
	if e := r.Verify(false, wanFixtureNow); e != nil {
		t.Fatal(e)
	}
	r.Announcement.Generation++
	if e := r.Verify(false, wanFixtureNow); e == nil {
		t.Fatal("unsigned payload substitution")
	}
	var token RelayAttachment
	wanFixture(t, "attachment", &token)
	attached := NetworkProof{Kind: "attach", Profile: token.Profile, Origin: token.Origin, Session: token.Session, Sender: token.Device, SenderPin: token.Pin, Target: token.Partner, TargetPin: token.PartnerPin, Purpose: token.Purpose, Role: token.Role}
	if e := token.Bind(attached, token.Profile, token.Origin, token.Epoch); e != nil {
		t.Fatal(e)
	}
	attached.Role = "responder"
	if e := token.Bind(attached, token.Profile, token.Origin, token.Epoch); e == nil {
		t.Fatal("other role attached")
	}
	var status RoutedEnrollmentStatus
	wanFixture(t, "enrollment-status", &status)
	if e := status.Verify(wanKey().Public().(ed25519.PublicKey), wanFixtureNow); e != nil {
		t.Fatal(e)
	}
	status.Nonce = wanID("f")
	if e := status.Verify(wanKey().Public().(ed25519.PublicKey), wanFixtureNow); e == nil {
		t.Fatal("status substitution")
	}
}
func TestWANW01InvitationSizeBound(t *testing.T) {
	raw, e := os.ReadFile("../../schemas/fixtures/network-v1/invitation.json")
	if e != nil {
		t.Fatal(e)
	}
	var invitation RoutedInvitation
	raw = append(raw, bytes.Repeat([]byte(" "), RoutedInvitationMaxBytes-len(raw))...)
	if e := DecodeRoutedInvitation(raw, &invitation); e != nil {
		t.Fatal(e)
	}
	if e := invitation.Validate(wanFixtureNow, false); e != nil {
		t.Fatal(e)
	}
	if e := DecodeRoutedInvitation(append(raw, ' '), &invitation); e == nil {
		t.Fatal("invitation over limit")
	}
}

func TestWANW01MaximumGoldenAndOneOver(t *testing.T) {
	var p NetworkProfile
	gold := wanFixture(t, "maximum-profile", &p)
	got, e := p.Canonical(false)
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("maximum profile %v", e)
	}
	if e = p.Verify(wanKey().Public().(ed25519.PublicKey), wanFixtureNow, 1, false); e != nil {
		t.Fatal(e)
	}
	var a NetworkAnnouncement
	gold = wanFixture(t, "maximum-announcement", &a)
	got, e = a.Canonical(wanFixtureNow, true)
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatalf("maximum announcement %v", e)
	}
	raw, e := os.ReadFile("../../schemas/fixtures/network-v1/over-limit-profile.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = NetworkDecode(raw, &p); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Canonical(false); e == nil {
		t.Fatal("over-limit golden profile")
	}
	raw, e = os.ReadFile("../../schemas/fixtures/network-v1/over-limit-announcement.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = NetworkDecode(raw, &a); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Canonical(wanFixtureNow, true); e == nil {
		t.Fatal("over-limit golden announcement")
	}
}

func TestWANW01IntentGoldenAndFreshChallenge(t *testing.T) {
	var q NetworkProof
	gold := wanFixture(t, "lookup-proof", &q)
	got, e := q.Canonical(false)
	if e != nil || !bytes.Equal(gold, got) {
		t.Fatal("lookup proof golden", e)
	}
	if e = q.Verify(false, wanFixtureNow); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../schemas/fixtures/network-v1/lookup-intent.hex")
	if e != nil {
		t.Fatal(e)
	}
	want, e := hex.DecodeString(strings.TrimSpace(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	q.Payload = ""
	intent, e := q.Intent(false)
	if e != nil || !bytes.Equal(intent, want) {
		t.Fatal("intent construction golden", e)
	}
	q.Challenge = wanID("f")
	q.Expires--
	again, e := q.Intent(false)
	if e != nil || !bytes.Equal(intent, again) {
		t.Fatal("fresh authentication changed semantic operation")
	}
	q.Purpose = "enrollment"
	changed, e := q.Intent(false)
	if e != nil || bytes.Equal(changed, intent) {
		t.Fatal("purpose omitted from semantic operation")
	}
}
