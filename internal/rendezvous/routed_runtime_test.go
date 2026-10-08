package rendezvous

import (
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

func TestWANW05QuotaProofDoesNotStrandChallengeSlots(t *testing.T) {
	f := newFixture(t, false)
	d := identity(t, 41, nil)
	for i := 0; i < network.MetadataBurst; i++ {
		q := f.proof(t, d, "lookup", "enrollment", "", nil)
		if _, code := f.s.lookup(q, uint64(f.now.Load())); code != "" {
			t.Fatal(code)
		}
	}
	// Repeated valid proofs at quota consume their challenges, without changing
	// records. They cannot occupy both challenge slots until expiry.
	for i := 0; i < 5; i++ {
		q := f.proof(t, d, "lookup", "enrollment", "", nil)
		if _, code := f.s.lookup(q, uint64(f.now.Load())); code != p.NetworkQuota {
			t.Fatal("quota missing", code)
		}
		if len(f.s.challenges) != 0 {
			t.Fatal("quota refusal stranded challenge")
		}
	}
	f.now.Add(int64((2 * time.Second) / time.Second))
	q := f.proof(t, d, "lookup", "enrollment", "", nil)
	if _, code := f.s.lookup(q, uint64(f.now.Load())); code != "" {
		t.Fatal("fresh proof after quota cannot recover", code)
	}
}
