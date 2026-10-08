// Package pairing implements Orbit's single-attempt invitation exchange.
// CPaceRistretto255/SHA-512 is pinned to draft-irtf-cfrg-cpace-21, sections
// 7.2 and 8.3, initiator/responder ordering. Curve arithmetic is delegated to
// github.com/gtank/ristretto255; this package implements no curve primitives.
package pairing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"strings"

	"github.com/gtank/ristretto255"
)

const Suite = "orbit-pairing-cpace-ristretto255-sha512-draft21-v1"
const Alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var ErrCode = errors.New("PAIRING_CODE_INVALID")
var ErrProof = errors.New("PAIRING_WRONG_CODE")

// Code returns independently uniform mailbox and password halves (20 bits
// each). The mailbox is public. Only the password half authenticates CPace.
func Code() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = Alphabet[b[i]&31]
	}
	return string(b[:4]) + "-" + string(b[4:])
}

func Normalize(code string) (string, error) {
	if len(code) > 64 {
		return "", ErrCode
	}
	code = strings.ToUpper(code)
	code = strings.Map(func(r rune) rune {
		switch r {
		case '-', ' ', '\t', '\r', '\n':
			return -1
		case 'O':
			return '0'
		case 'I', 'L':
			return '1'
		}
		return r
	}, code)
	if len(code) != 8 {
		return "", ErrCode
	}
	for _, r := range code {
		if !strings.ContainsRune(Alphabet, r) {
			return "", ErrCode
		}
	}
	return code, nil
}

func lv(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = binary.AppendUvarint(out, uint64(len(p)))
		out = append(out, p...)
	}
	return out
}

func generator(password, ci, sid []byte) *ristretto255.Element {
	dsi := []byte("CPaceRistretto255")
	pad := max(0, sha512.BlockSize-len(lv(password))-len(lv(dsi))-1)
	h := sha512.Sum512(lv(dsi, password, make([]byte, pad), ci, sid))
	g, _ := ristretto255.NewElement().SetUniformBytes(h[:])
	return g
}

// Exchange owns a single private scalar. Finish consumes it even on failure;
// callers cannot reuse it to test another password guess.
type Exchange struct {
	secret      *ristretto255.Scalar
	public, sid []byte
}

func New(password, ci, sid []byte) *Exchange {
	var b [32]byte
	_, _ = rand.Read(b[:])
	b[31] &= 15 // draft21 recommended 252-bit scalar sampling
	s, _ := ristretto255.NewScalar().SetCanonicalBytes(b[:])
	return exchange(s, password, ci, sid)
}

func exchange(s *ristretto255.Scalar, password, ci, sid []byte) *Exchange {
	return &Exchange{s, ristretto255.NewElement().ScalarMult(s, generator(password, ci, sid)).Bytes(), append([]byte(nil), sid...)}
}

func (x *Exchange) Public() []byte { return append([]byte(nil), x.public...) }

func (x *Exchange) Finish(peer, ada, adb []byte, initiator bool) ([]byte, error) {
	if x.secret == nil {
		return nil, ErrProof
	}
	s := x.secret
	x.secret = nil
	defer s.Zero()
	point, err := ristretto255.NewElement().SetCanonicalBytes(peer)
	if err != nil {
		return nil, ErrProof
	}
	k := ristretto255.NewElement().ScalarMult(s, point)
	if k.Equal(ristretto255.NewIdentityElement()) == 1 {
		return nil, ErrProof
	}
	a, b := x.public, peer
	if !initiator {
		a, b = peer, x.public
	}
	input := append(lv([]byte("CPaceRistretto255_ISK"), x.sid, k.Bytes()), lv(a, ada, b, adb)...)
	isk := sha512.Sum512(input)
	return isk[:], nil
}

// Separate labels bind key confirmation direction and invitation encryption.
func derive(key []byte, label string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(lv([]byte(Suite), []byte(label)))
	return m.Sum(nil)
}
func Confirmation(key []byte) []byte { return derive(key, "joiner key confirmation") }
func Confirm(key, tag []byte) bool   { return hmac.Equal(Confirmation(key), tag) }
func aead(key []byte) cipher.AEAD {
	b, _ := aes.NewCipher(derive(key, "inviter invitation encryption"))
	a, _ := cipher.NewGCM(b)
	return a
}
func Seal(key, invitation []byte) []byte {
	a := aead(key)
	n := make([]byte, a.NonceSize())
	_, _ = rand.Read(n)
	return a.Seal(n, n, invitation, []byte(Suite))
}
func Open(key, encrypted []byte) ([]byte, error) {
	a := aead(key)
	if len(encrypted) < a.NonceSize() {
		return nil, ErrProof
	}
	b, e := a.Open(nil, encrypted[:a.NonceSize()], encrypted[a.NonceSize():], []byte(Suite))
	if e != nil {
		return nil, ErrProof
	}
	return b, nil
}

// Context binds the mailbox to the operator and the unique incarnation. Device
// identities/pins are bound as ordered associated data at key derivation.
func Context(origin, mailbox string) []byte {
	return lv([]byte(Suite), []byte(origin), []byte(mailbox))
}
func Identity(device, pin string) []byte { return lv([]byte(device), []byte(pin)) }
