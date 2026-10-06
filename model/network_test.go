package model

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestWANW01AdmissionReplayIdentityAndRestart(t *testing.T) {
	m := NewWANAdmission("epoch-1")
	a := WANIdentity{"device", "pin-a"}
	b := WANIdentity{"device", "pin-b"}
	if m.Challenge("nonce", a, 0) != "accepted" {
		t.Fatal("challenge")
	}
	if got := m.Announce(a, "nonce", "op", "payload", 1, 600, 0, false, true); got != "unauthorized" {
		t.Fatal(got)
	}
	if got := m.Announce(b, "nonce", "op", "payload", 1, 600, 0, true, true); got != "replay" {
		t.Fatal("challenge identity", got)
	}
	if got := m.Announce(a, "nonce", "op", "payload", 1, 600, 0, true, true); got != "accepted" {
		t.Fatal(got)
	}
	if got := m.Announce(a, "nonce", "op", "payload", 1, 600, 0, true, true); got != "replay" {
		t.Fatal(got)
	}
	m.Challenge("retry", a, 1)
	if got := m.Announce(a, "retry", "op", "payload", 1, 600, 1, true, true); got != "replayed" {
		t.Fatal(got)
	}
	m.Challenge("conflict", a, 1)
	if got := m.Announce(a, "conflict", "op", "changed", 2, 600, 1, true, true); got != "conflict" {
		t.Fatal(got)
	}
	m.Challenge("competitor", b, 1)
	if got := m.Announce(b, "competitor", "op", "competitor-payload", 1, 600, 1, true, true); got != "accepted" {
		t.Fatal(got)
	}
	if v, ok := m.Lookup(a, true, 1); !ok || v.Payload != "payload" {
		t.Fatal("pin overwrite")
	}
	if _, ok := m.Lookup(a, false, 1); ok {
		t.Fatal("unauthenticated lookup")
	}
	m.Challenge("stale", a, 2)
	if got := m.Announce(a, "stale", "new", "new", 1, 600, 2, true, true); got != "stale" {
		t.Fatal(got)
	}
	m.Challenge("expired", a, 2)
	if got := m.Announce(a, "expired", "new", "new", 2, 600, 62, true, true); got != "replay" {
		t.Fatal(got)
	}
	if _, ok := m.Lookup(a, true, 600); ok {
		t.Fatal("lease survived expiry")
	}
	m.Restart("epoch-2")
	if len(m.Routes) != 0 || len(m.Challenges) != 0 || len(m.Operations) != 0 {
		t.Fatal("restart retained authority")
	}
	if got := m.Attach("session", "initiator", "epoch-1", "enrollment", a, b, 1, 31, true, true); got != "unauthorized" {
		t.Fatal(got)
	}
	if got := m.Attach("session", "initiator", "epoch-2", "enrollment", a, b, 1, 31, true, true); got != "attached" {
		t.Fatal(got)
	}
	if got := m.Attach("session", "initiator", "epoch-2", "enrollment", a, b, 1, 31, true, true); got != "replay" {
		t.Fatal(got)
	}
	if got := m.Attach("session", "responder", "epoch-2", "control", a, b, 1, 31, true, true); got != "unauthorized" {
		t.Fatal(got)
	}
	m.Release("session")
	if len(m.Attachments) != 0 {
		t.Fatal("release leaked")
	}
}
func TestWANW01AdmissionCapacity(t *testing.T) {
	m := NewWANAdmission("epoch")
	for i := 0; i < 128; i++ {
		if got := m.Challenge(fmt.Sprint(i), WANIdentity{fmt.Sprint(i), "pin"}, 0); got != "accepted" {
			t.Fatal(got)
		}
	}
	if got := m.Challenge("extra", WANIdentity{"extra", "pin"}, 0); got != "capacity" {
		t.Fatal(got)
	}
	m.Expire(60)
	if len(m.Challenges) != 0 {
		t.Fatal("expiry leaked")
	}
	for i := 0; i < 128; i++ {
		who := WANIdentity{fmt.Sprint(i), "pin"}
		nonce := fmt.Sprint(i)
		m.Challenge(nonce, who, 60)
		if got := m.Announce(who, nonce, "op", "payload", 1, 660, 60, true, true); got != "accepted" {
			t.Fatal(got)
		}
	}
	who := WANIdentity{"extra", "pin"}
	m.Challenge("extra", who, 60)
	if got := m.Announce(who, "extra", "op", "payload", 1, 660, 60, true, true); got != "capacity" {
		t.Fatal(got)
	}
	if len(m.Routes) != 128 {
		t.Fatal("route cap")
	}
}
func TestWANW01EnrollmentExactApproval(t *testing.T) {
	m := NewWANEnrollment()
	j := WANJoin{"folder-a", "attempt", "inviter", "requester", "pin", "prior", "reviewed-digest", 600}
	if got := m.Submit(j, "cap", 0, false, true); got != "unauthorized" {
		t.Fatal(got)
	}
	if got := m.Submit(j, "cap", 0, true, true); got != "pending" {
		t.Fatal(got)
	}
	if m.FolderAccess(j) {
		t.Fatal("network possession approved folder")
	}
	if got := m.Submit(j, "cap", 1, true, true); got != "replayed" {
		t.Fatal("lost response changed attempt", got)
	}
	// Same-key second folder has separate admission and authority.
	second := j
	second.Folder = "folder-b"
	second.Digest = "other"
	if got := m.Submit(second, "other-cap", 1, true, true); got != "pending" {
		t.Fatal(got)
	}
	if got := m.Approve(j.ID(), "folder-b", j.Requester, j.Pin, j.Prior, j.Digest, "cap", 2); got != "stale" {
		t.Fatal(got)
	}
	if got := m.Approve(j.ID(), j.Folder, j.Requester, "changed-pin", j.Prior, j.Digest, "cap", 2); got != "stale" {
		t.Fatal(got)
	}
	snapshot := *m
	m = &snapshot // durable intent persists a modeled daemon restart
	if got := m.Approve(j.ID(), j.Folder, j.Requester, j.Pin, j.Prior, j.Digest, "cap", 2); got != "approved" {
		t.Fatal(got)
	}
	if !m.FolderAccess(j) || m.FolderAccess(second) {
		t.Fatal("folder isolation")
	}
	m.Revoked["other-cap"] = true
	if got := m.Approve(second.ID(), second.Folder, second.Requester, second.Pin, second.Prior, second.Digest, "other-cap", 2); got != "unauthorized" {
		t.Fatal(got)
	}
	m.Retired[j.Requester] = true
	if m.FolderAccess(j) {
		t.Fatal("retired key still admitted")
	}
	if got := m.Submit(j, "cap", 3, true, true); got != "unauthorized" {
		t.Fatal(got)
	}
	m.Retired[j.Requester] = false
	if got := m.Submit(second, "cap", 600, true, true); got != "expired" {
		t.Fatal(got)
	}
}
func TestWANW01ConnectionGenerationAndBounds(t *testing.T) {
	m := NewWANConnection()
	if !m.Start("a", "direct") || !m.Start("b", "direct") || m.Start("c", "direct") || !m.Start("r", "relay") || m.Start("r2", "relay") {
		t.Fatal("attempt caps")
	}
	if m.Complete("a", 1, false) {
		t.Fatal("pin bypass")
	}
	if !m.Complete("r", 1, true) || m.Route != "relay" {
		t.Fatal("relay")
	}
	if !m.Complete("b", 1, true) || m.Route != "direct" {
		t.Fatal("direct preference")
	}
	m.Start("old", "direct")
	m.Change()
	if m.Complete("old", 1, true) || m.Route != "" {
		t.Fatal("stale completion")
	}
	rng := rand.New(rand.NewSource(101))
	for i := 0; i < 10000; i++ {
		switch rng.Intn(4) {
		case 0:
			m.Change()
		case 1:
			m.Start(fmt.Sprint(i), "direct")
		case 2:
			m.Start(fmt.Sprint(i), "relay")
		case 3:
			for id := range m.Attempts {
				m.Complete(id, m.Generation-1, true)
			}
		}
		if len(m.Attempts) > 3 {
			t.Fatal("unbounded flapping")
		}
	}
	m.Cancel()
	if len(m.Attempts) != 0 || m.Start("new", "direct") || m.Complete("new", m.Generation, true) {
		t.Fatal("cancel leaked")
	}
}

func TestWANW01ReplayCapacityCannotEvictLiveProtection(t *testing.T) {
	m := NewWANAdmission("epoch")
	who := WANIdentity{"device", "pin"}
	for i := 0; i < 1024; i++ {
		nonce := fmt.Sprint(i)
		m.Challenge(nonce, who, 0)
		if got := m.Announce(who, nonce, nonce, nonce, uint64(i+1), 600, 0, true, true); got != "accepted" {
			t.Fatal(got)
		}
	}
	m.Challenge("over", who, 0)
	if got := m.Announce(who, "over", "over", "over", 1025, 600, 0, true, true); got != "capacity" {
		t.Fatal(got)
	}
	if len(m.Operations) != 1024 || m.Routes[who].Generation != 1024 {
		t.Fatal("capacity refusal changed admitted state")
	}
	m.Challenge("retry", who, 0)
	if got := m.Announce(who, "retry", "0", "0", 1, 600, 0, true, true); got != "replayed" {
		t.Fatal("live protection evicted", got)
	}
}

func TestWANW01AttachmentExpiryIsNotTunnelExpiry(t *testing.T) {
	m := NewWANAdmission("epoch")
	a, b := WANIdentity{"a", "a-pin"}, WANIdentity{"b", "b-pin"}
	if got := m.Attach("session", "initiator", "epoch", "peer_data", a, b, 0, 30, true, true); got != "attached" {
		t.Fatal(got)
	}
	m.Expire(30)
	if len(m.Attachments) != 1 {
		t.Fatal("token expiry killed active tunnel")
	}
	if got := m.Attach("session", "responder", "epoch", "peer_data", b, a, 30, 30, true, true); got != "expired" {
		t.Fatal(got)
	}
	m.Expire(3600)
	if len(m.Attachments) != 0 {
		t.Fatal("tunnel lifetime leaked")
	}
	m.Attach("another", "initiator", "epoch", "peer_data", a, b, 4000, 4030, true, true)
	m.Release("another")
	if got := m.Attach("another", "initiator", "epoch", "peer_data", a, b, 4001, 4030, true, true); got != "replay" {
		t.Fatal("released credential reused", got)
	}
}
