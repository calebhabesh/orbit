package pairing

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/gtank/ristretto255"
)

// Published draft21 Appendix B.3, independent of Orbit's exchange fixtures.
func TestOnboardingE06CPacePublishedVectors(t *testing.T) {
	h := func(s string) []byte {
		b, e := hex.DecodeString(s)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	ci := h("0b415f696e69746961746f720b425f726573706f6e646572")
	sid := h("7e4b4791d6a8ef019b936c79fb7f2c57")
	if got := hex.EncodeToString(generator([]byte("Password"), ci, sid).Bytes()); got != "222b6b195fe84b1652badb6f6a3ae3d24341e7306967f0b8115b40d5698c7e56" {
		t.Fatal(got)
	}
	sa, _ := ristretto255.NewScalar().SetCanonicalBytes(h("da3d23700a9e5699258aef94dc060dfda5ebb61f02a5ea77fad53f4ff0976d08"))
	sb, _ := ristretto255.NewScalar().SetCanonicalBytes(h("d2316b454718c35362d83d69df6320f38578ed5984651435e2949762d900b80d"))
	a, b := exchange(sa, []byte("Password"), ci, sid), exchange(sb, []byte("Password"), ci, sid)
	if hex.EncodeToString(a.Public()) != "d6bac480f2c386c394efc7c47adb9925dcd2630b64f240c50f8d0eec482b9157" {
		t.Fatal("Ya")
	}
	if hex.EncodeToString(b.Public()) != "3ea7e0b19560d7c0b0f5734f63b955286dfa8232b5ebe63324e2d9e7433f7258" {
		t.Fatal("Yb")
	}
	ka, e := a.Finish(b.Public(), []byte("ADa"), []byte("ADb"), true)
	if e != nil {
		t.Fatal(e)
	}
	kb, e := b.Finish(a.Public(), []byte("ADa"), []byte("ADb"), false)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(ka, kb) || hex.EncodeToString(ka) != "b69effbf61b51d56401c0f65601abe428de8206feaaf0e32198896dcae7b35cd2b38950a39dfd5d4a79164614c2984f7daa460b588c1e80c3fa2068af7900447" {
		t.Fatalf("ISK: %x", ka)
	}
}

func TestOnboardingE06CPaceRejectsWrongPasswordAndReuse(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		pw := []byte("1234")
		other := pw
		if wrong {
			other = []byte("5678")
		}
		a, b := New(pw, []byte("context"), []byte("unique session")), New(other, []byte("context"), []byte("unique session"))
		ka, e := a.Finish(b.Public(), []byte("a"), []byte("b"), true)
		if e != nil {
			t.Fatal(e)
		}
		kb, e := b.Finish(a.Public(), []byte("a"), []byte("b"), false)
		if e != nil {
			t.Fatal(e)
		}
		if Confirm(ka, Confirmation(kb)) == wrong {
			t.Fatal("confirmation")
		}
		plain, e := Open(kb, Seal(ka, []byte("private invitation")))
		if wrong && e == nil || !wrong && (e != nil || string(plain) != "private invitation") {
			t.Fatal("AEAD")
		}
		if _, e = a.Finish(b.Public(), nil, nil, true); e == nil {
			t.Fatal("reused scalar")
		}
	}
	for _, peer := range [][]byte{nil, make([]byte, 32), bytes.Repeat([]byte{255}, 32)} {
		a := New([]byte("1234"), nil, nil)
		if _, e := a.Finish(peer, nil, nil, true); e == nil {
			t.Fatal("invalid/identity point")
		}
	}
	if got, e := Normalize(" olil- k7qx "); e != nil || got != "0111K7QX" {
		t.Fatalf("%s %v", got, e)
	}
}
