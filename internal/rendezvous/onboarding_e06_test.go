package rendezvous

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
)

func TestOnboardingE06PairingAndOpaqueCapture(t *testing.T) {
	var mu sync.Mutex
	var captured bytes.Buffer
	f := newWrappedFixture(t, true, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/network/v1/pairing" {
				b, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(b))
				mu.Lock()
				captured.Write(b)
				mu.Unlock()
			}
			next.ServeHTTP(w, r)
		})
	})
	a, b := identity(t, 20, nil), identity(t, 21, nil)
	ca, cb := f.client(t, a, f.roots), f.client(t, b, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	plain := []byte("orbit-invitation:v3:synthetic-private-folder-and-capability")
	status, e := ca.StartPairing(ctx, strings.Repeat("a", 64), plain, time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	if len(status.Code) != 9 || status.Code[4] != '-' {
		t.Fatal(status)
	}
	got, device, pin, e := cb.JoinPairing(ctx, strings.ToLower(status.Code))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, plain) || device != a.id || pin != a.pin {
		t.Fatal("invitation/identity mismatch")
	}
	if s := ca.PairingStatus(strings.Repeat("a", 64)); s.State != "sent" || s.Code != "" {
		t.Fatal(s)
	}
	mu.Lock()
	wire := captured.String()
	mu.Unlock()
	if strings.Contains(wire, string(plain)) || strings.Contains(wire, hex.EncodeToString(plain)) || strings.Contains(wire, status.Code) {
		t.Fatal("plaintext invitation or password reached service")
	}
	if _, _, _, e = cb.JoinPairing(ctx, status.Code); e == nil {
		t.Fatal("code reused")
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if len(f.s.mailboxes) != 0 {
		t.Fatal("delivered mailbox retained")
	}
}

func TestOnboardingE06WrongCodeBurnsMailbox(t *testing.T) {
	f := newFixture(t, true)
	ca, cb := f.client(t, identity(t, 30, nil), f.roots), f.client(t, identity(t, 31, nil), f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, e := ca.StartPairing(ctx, "wrong", []byte("secret invitation"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	wrong := status.Code[:8] + "0"
	if wrong == status.Code {
		wrong = status.Code[:8] + "1"
	}
	if _, _, _, e = cb.JoinPairing(ctx, wrong); e == nil {
		t.Fatal("wrong password accepted")
	}
	if s := ca.PairingStatus("wrong"); s.State != "failed" || s.Error != p.PairingWrong {
		t.Fatal(s)
	}
	if _, _, _, e = cb.JoinPairing(ctx, status.Code); e == nil {
		t.Fatal("burned mailbox accepted correct password")
	}
}

func TestOnboardingE06MailboxExpirySingleClaimAndBounds(t *testing.T) {
	f := newFixture(t, true)
	ca, cb := f.client(t, identity(t, 40, nil), f.roots), f.client(t, identity(t, 41, nil), f.roots)
	ctx := context.Background()
	q := p.PairingRequest{Action: "create", Mailbox: "ABCD", Session: strings.Repeat("a", 64), Expires: p.NetworkUint(f.now.Load() + 600), Data: strings.Repeat("b", 64)}
	if _, e := ca.PairingExchange(ctx, q); e != nil {
		t.Fatal(e)
	}
	if _, e := ca.PairingExchange(ctx, q); e == nil {
		t.Fatal("squatted name overwritten")
	}
	claim := p.PairingRequest{Action: "claim", Mailbox: "ABCD"}
	if _, e := cb.PairingExchange(ctx, claim); e != nil {
		t.Fatal(e)
	}
	if _, e := cb.PairingExchange(ctx, claim); e == nil {
		t.Fatal("second claim")
	}
	// Advance the service clock only; expiry is tested through its normal sweep.
	f.s.mu.Lock()
	f.s.expire(uint64(f.now.Load() + 601))
	remaining := len(f.s.mailboxes)
	f.s.mu.Unlock()
	if remaining != 0 {
		t.Fatal("expired mailbox retained")
	}
	if _, e := cb.PairingExchange(ctx, claim); e == nil {
		t.Fatal("expired code")
	}
	// The fifth create/claim spends the source burst; next is refused, including
	// nonexistent mailbox claims. Reset only the fixture rate table for key cap.
	f.s.mu.Lock()
	f.s.pairSources = map[string]bucket{}
	f.s.mu.Unlock()
	for i, name := range []string{"AAAA", "AAAB", "AAAC", "AAAD", "AAAE"} {
		q.Mailbox = name
		_, e := ca.PairingExchange(ctx, q)
		if i < 4 && e != nil {
			t.Fatalf("create %d: %v", i, e)
		}
		if i == 4 && e == nil {
			t.Fatal("per-key mailbox cap")
		}
	}
	if _, e := cb.PairingExchange(ctx, p.PairingRequest{Action: "claim", Mailbox: "ZZZZ"}); e == nil || e.Error() != p.NetworkQuota {
		t.Fatalf("source flood: %v", e)
	}
}

func TestOnboardingE06ServiceRestartLosesMailbox(t *testing.T) {
	var mu sync.Mutex
	var restarted http.Handler
	f := newWrappedFixture(t, true, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if restarted != nil {
				restarted.ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	})
	ca, cb := f.client(t, identity(t, 51, nil), f.roots), f.client(t, identity(t, 52, nil), f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := ca.StartPairing(ctx, "restart", []byte("private invitation"), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := New(Options{Selection: f.selection, Origin: f.origin, ServiceKey: f.s.key, Now: f.s.now})
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	mu.Lock()
	restarted = fresh
	mu.Unlock()
	if err = f.s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = cb.JoinPairing(ctx, status.Code); err == nil {
		t.Fatal("mailbox survived restart")
	}
	for ca.PairingStatus("restart").State == "waiting" {
		select {
		case <-ctx.Done():
			t.Fatal("inviter did not learn mailbox was lost")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if got := ca.PairingStatus("restart"); got.Error != p.PairingUnavailable {
		t.Fatal(got)
	}
}

func TestOnboardingE06GlobalMailboxAndSourceBounds(t *testing.T) {
	f := newFixture(t, true)
	ca := f.client(t, identity(t, 60, nil), f.roots)
	f.s.mu.Lock()
	f.s.mailboxes = map[string]*mailbox{}
	f.s.pairSources = map[string]bucket{}
	for i := 0; i < MaxPairingMailboxes; i++ {
		f.s.mailboxes[fmt.Sprint(i)] = &mailbox{expires: uint64(f.now.Load() + 600)}
	}
	f.s.mu.Unlock()
	q := p.PairingRequest{Action: "create", Mailbox: "ABCD", Session: strings.Repeat("a", 64), Expires: p.NetworkUint(f.now.Load() + 600), Data: strings.Repeat("b", 64)}
	if _, err := ca.PairingExchange(context.Background(), q); err == nil || err.Error() != p.NetworkQuota {
		t.Fatalf("global cap: %v", err)
	}
	f.s.mu.Lock()
	f.s.mailboxes = map[string]*mailbox{}
	f.s.pairSources = map[string]bucket{}
	for i := 0; i < 128; i++ {
		f.s.pairSources[fmt.Sprint(i)] = bucket{at: f.s.now()}
	}
	f.s.mu.Unlock()
	if _, err := ca.PairingExchange(context.Background(), q); err == nil || err.Error() != p.NetworkQuota {
		t.Fatalf("source table cap: %v", err)
	}
}

func TestOnboardingE06OlderServiceRefusesShortCode(t *testing.T) {
	f := newWrappedFixture(t, true, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/network/v1/pairing" {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	ca := f.client(t, identity(t, 70, nil), f.roots)
	if status, err := ca.StartPairing(context.Background(), "old", []byte("private invitation"), time.Now().Add(time.Minute)); err == nil || status.Code != "" {
		t.Fatalf("unsupported service issued code: %+v %v", status, err)
	}
}
