package network

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
)

// The deployed release profile (W13 deployment record) is the packaged default.
const w14ReleaseDigest = "356f0ced2c898e7e6bcdd4913713c797737809ffa27a54e40362168f577ec165"

func TestWANW14PackagedReleaseProfile(t *testing.T) {
	t.Setenv(DisablePackagedProfileEnv, "")
	s, err := DecodeBuiltinProfile(releaseProfileJSON)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := s.Digest()
	if digest != w14ReleaseDigest || s.Authority != ReleaseAuthority || s.Environment != "release" || s.Private() {
		t.Fatal(digest, s.Authority, s.Environment)
	}
	expires := uint64(s.Profile.Expires)
	if err = s.Validate(expires - 1); err != nil {
		t.Fatal("packaged profile invalid before expiry", err)
	}
	if err = s.Validate(expires); err == nil {
		t.Fatal("packaged profile accepted at expiry")
	}
	got, ok := BuiltinProfile()
	if gd, _ := got.Digest(); !ok || gd != digest {
		t.Fatal("BuiltinProfile does not return the embedded selection")
	}

	// Tampered text or a different signer is never a packaged default.
	var tampered ProfileSelection
	_ = json.Unmarshal(releaseProfileJSON, &tampered)
	tampered.Profile.Privacy += " Edited."
	b, _ := json.Marshal(tampered)
	if _, err = DecodeBuiltinProfile(b); err == nil {
		t.Fatal("tampered packaged profile accepted")
	}
	pub, signer, _ := ed25519.GenerateKey(rand.Reader)
	foreign := tampered
	foreign.Authority = hex.EncodeToString(pub)
	foreign.Profile.Authority = foreign.Authority
	canonical, _ := foreign.Profile.Canonical(false)
	foreign.Profile.Signature = hex.EncodeToString(ed25519.Sign(signer, canonical))
	b, _ = json.Marshal(foreign)
	if _, err = DecodeBuiltinProfile(b); err == nil || err.Error() != "PROFILE_UNTRUSTED" {
		t.Fatal("foreign authority accepted as packaged default", err)
	}
	foreign.Environment = "development"
	b, _ = json.Marshal(foreign)
	if _, err = DecodeBuiltinProfile(b); err == nil {
		t.Fatal("development selection accepted as packaged default")
	}

	t.Setenv(DisablePackagedProfileEnv, "1")
	if _, ok = BuiltinProfile(); ok {
		t.Fatal("disable switch ignored")
	}
}

func TestWANW14ReviewOperatorChange(t *testing.T) {
	release, err := DecodeBuiltinProfile(releaseProfileJSON)
	if err != nil {
		t.Fatal(err)
	}
	_, signer, _ := ed25519.GenerateKey(rand.Reader)
	self := release.Profile
	self.Authority = hex.EncodeToString(signer.Public().(ed25519.PublicKey))
	canonical, _ := self.Canonical(true)
	self.Signature = hex.EncodeToString(ed25519.Sign(signer, canonical))
	now := uint64(release.Profile.Expires) - 10
	if _, err = ReviewProfile(&release, self, self.Authority, "self_hosted", now); !errors.Is(err, ErrOperatorChange) {
		t.Fatal("operator change not distinguished", err)
	}
	if _, err = ReviewProfileChange(&release, nil, self, self.Authority, "self_hosted", now, true); err != nil {
		t.Fatal(err)
	}
	floors := map[string]uint64{release.Authority: uint64(release.HighestEpoch) + 1}
	if _, err = ReviewProfileChange(nil, floors, release.Profile, release.Authority, "release", now, true); err == nil {
		t.Fatal("floor ignored")
	}
}
