package protocol

import "encoding/hex"

const PairingUnavailable = "PAIRING_UNAVAILABLE"
const PairingUsed = "PAIRING_ALREADY_USED"
const PairingWrong = "PAIRING_WRONG_CODE"

// PairingRequest carries only public PAKE messages or encrypted invitation
// bytes. Its canonical payload is bound by the existing device proof.
type PairingRequest struct {
	Proof   NetworkProof `json:"proof"`
	Action  string       `json:"action"`
	Mailbox string       `json:"mailbox"`
	Session string       `json:"session"`
	Expires NetworkUint  `json:"expires"`
	Data    string       `json:"data"`
}

func (q PairingRequest) Canonical() []byte {
	return NetworkCanonical("pairing", q.Action, q.Mailbox, q.Session, stringUintPairing(q.Expires), q.Data)
}
func stringUintPairing(v NetworkUint) string { b, _ := v.MarshalJSON(); return string(b) }
func (q PairingRequest) Valid() bool {
	if len(q.Mailbox) != 4 || len(q.Data) > 32<<10 {
		return false
	}
	for _, c := range q.Mailbox {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' && c != 'I' && c != 'L' && c != 'O' && c != 'U') {
			return false
		}
	}
	b, e := hex.DecodeString(q.Data)
	return e == nil && hex.EncodeToString(b) == q.Data && (q.Session == "" || NetworkHex(q.Session, 32) == nil)
}

type PairingResult struct {
	Version string      `json:"version"`
	State   string      `json:"state"`
	Session string      `json:"session"`
	Expires NetworkUint `json:"expires"`
	Device  string      `json:"device"`
	Pin     string      `json:"pin"`
	Data    string      `json:"data"`
}
