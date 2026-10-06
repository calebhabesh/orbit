package protocol

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"

	"github.com/pion/ice/v4"
)

// NetworkICE is an additive signed extension. Legacy offers retain exact bytes.
// Candidate addresses are public numeric targets, never DNS, TURN or mDNS.
type NetworkICE struct {
	Mode       string   `json:"mode"` // request, offer, accept
	Ufrag      string   `json:"ufrag"`
	Password   string   `json:"password"`
	Candidates []string `json:"candidates"`
}

func iceToken(s string, min int) bool {
	if len(s) < min || len(s) > 256 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return false
		}
	}
	return true
}
func ValidateICECandidate(s string) error {
	if len(s) > 512 || strings.ContainsAny(s, "\r\n\x00") {
		return errors.New("INVALID_ICE_CANDIDATE")
	}
	c, err := ice.UnmarshalCandidate(s)
	if err != nil {
		return errors.New("INVALID_ICE_CANDIDATE")
	}
	ip, err := netip.ParseAddr(c.Address())
	if err != nil || !AllowedAddress(ip, false) || c.Component() != 1 || c.Port() <= 0 || c.Port() > 65535 || (c.NetworkType() != ice.NetworkTypeUDP4 && c.NetworkType() != ice.NetworkTypeUDP6) || (c.Type() != ice.CandidateTypeHost && c.Type() != ice.CandidateTypeServerReflexive) {
		return errors.New("INVALID_ICE_CANDIDATE")
	}
	// Related private addresses leak topology and are not needed for checking.
	if related := c.RelatedAddress(); related != nil && ((related.Address != "0.0.0.0" && related.Address != "") || related.Port != 0) {
		return errors.New("INVALID_ICE_CANDIDATE")
	}
	if c.Marshal() != s {
		return errors.New("INVALID_ICE_CANDIDATE")
	}
	return nil
}
func (i NetworkICE) Canonical() ([]byte, error) {
	if i.Candidates == nil || len(i.Candidates) > NetworkMaxOfferCandidates {
		return nil, errors.New("INVALID_ICE")
	}
	if i.Mode == "request" {
		if i.Ufrag != "" || i.Password != "" || len(i.Candidates) != 0 {
			return nil, errors.New("INVALID_ICE")
		}
	} else if (i.Mode != "offer" && i.Mode != "accept") || !iceToken(i.Ufrag, 4) || !iceToken(i.Password, 22) || len(i.Candidates) == 0 {
		return nil, errors.New("INVALID_ICE")
	}
	f := []string{i.Mode, i.Ufrag, i.Password, strconv.Itoa(len(i.Candidates))}
	seen := map[string]bool{}
	for _, c := range i.Candidates {
		if ValidateICECandidate(c) != nil || seen[c] {
			return nil, errors.New("INVALID_ICE_CANDIDATE")
		}
		seen[c] = true
		f = append(f, c)
	}
	return NetworkCanonical("ice-v1", f...), nil
}

// VerifyICE binds deterministic controlling role to the existing signed proof.
func (r NetworkOfferRequest) VerifyICE() error {
	i := r.Offer.ICE
	if i == nil {
		return nil
	}
	smaller := r.Proof.Sender+"/"+r.Proof.SenderPin < r.Proof.Target+"/"+r.Proof.TargetPin
	if r.Proof.Purpose != "peer_data" || (i.Mode == "offer" && (!smaller || r.Proof.Kind != "offer" || r.Proof.Role != "initiator")) || (i.Mode == "request" && (smaller || r.Proof.Kind != "offer" || r.Proof.Role != "initiator")) || (i.Mode == "accept" && (smaller || r.Proof.Kind != "accept" || r.Proof.Role != "responder")) {
		return errors.New("INVALID_ICE_ROLE")
	}
	_, err := i.Canonical()
	return err
}
