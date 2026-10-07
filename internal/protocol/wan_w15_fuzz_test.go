package protocol

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"testing"
)

// Uses the independently frozen signed profile, never generates a fresh
// signature for a fuzzer mutation. A successful verification must authenticate
// exactly the original canonical bytes under the independently fixed authority.
func FuzzWANW15SignedProfile(f *testing.F) {
	seed, err := os.ReadFile("../../schemas/fixtures/network-v1/profile.json")
	if err != nil {
		f.Fatal(err)
	}
	var original NetworkProfile
	if err = NetworkDecode(seed, &original); err != nil {
		f.Fatal(err)
	}
	canonical, err := original.Canonical(false)
	if err != nil {
		f.Fatal(err)
	}
	authority := wanKey().Public().(ed25519.PublicKey)
	f.Add(seed)
	f.Add([]byte(`{"version":"1","version":"1"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var p NetworkProfile
		if NetworkDecode(data, &p) != nil {
			return
		}
		if p.Verify(authority, wanFixtureNow, 1, false) == nil {
			got, e := p.Canonical(false)
			if e != nil || !bytes.Equal(got, canonical) {
				t.Fatal("authenticated changed signed profile", e)
			}
		}
	})
}

// Accepted closed network records round-trip semantically; duplicate keys,
// unknown fields and over-limit input must be rejected before validation.
func FuzzWANW15NetworkCodec(f *testing.F) {
	names := []string{"profile", "proof", "announcement", "attachment", "enrollment-request", "enrollment-status", "invitation"}
	for i, name := range names {
		seed, e := os.ReadFile("../../schemas/fixtures/network-v1/" + name + ".json")
		if e != nil {
			f.Fatal(e)
		}
		f.Add(uint8(i), seed)
	}
	f.Fuzz(func(t *testing.T, selector uint8, data []byte) {
		if len(data) > NetworkMaxBytes+1 {
			return
		}
		factory := func() any {
			switch int(selector) % len(names) {
			case 0:
				return &NetworkProfile{}
			case 1:
				return &NetworkProof{}
			case 2:
				return &NetworkAnnouncement{}
			case 3:
				return &RelayAttachment{}
			case 4:
				return &RoutedEnrollmentRequest{}
			case 5:
				return &RoutedEnrollmentStatus{}
			default:
				return &RoutedInvitation{}
			}
		}
		a := factory()
		if NetworkDecode(data, a) != nil {
			return
		}
		encoded, e := json.Marshal(a)
		if e != nil {
			t.Fatal(e)
		}
		b := factory()
		if e = NetworkDecode(encoded, b); e != nil {
			t.Fatal("accepted record failed round-trip", e)
		}
		again, _ := json.Marshal(b)
		if !bytes.Equal(encoded, again) {
			t.Fatal("changed accepted record")
		}
		bad := append([]byte(`{"w15_unknown":true,`), bytes.TrimSpace(encoded)[1:]...)
		if NetworkDecode(bad, factory()) == nil {
			t.Fatal("unknown-field accepted")
		}
	})
}
