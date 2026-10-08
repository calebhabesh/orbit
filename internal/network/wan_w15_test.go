package network

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/coder/websocket"
)

type w15ServiceTransport struct {
	base        http.RoundTripper
	destination *url.URL
}

func (r w15ServiceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	destination := *request.URL
	destination.Scheme, destination.Host = r.destination.Scheme, r.destination.Host
	copy.URL = &destination
	return r.base.RoundTrip(copy)
}

// A rejected renewal must not change the generation used in authenticated offers.
// The service oracle independently remembers only accepted announcements.
// W16: a transient (quota) refusal keeps the still-live accepted record ready;
// a semantic refusal withdraws readiness so no service work follows.
func TestWANW15RejectedAnnouncementKeepsAcceptedOfferGeneration(t *testing.T) {
	client := clientFixture(t, &testResolver{})
	client.selection.Profile.Origins = append(client.selection.Profile.Origins, "wss://directory.orbit.invalid")
	canonical, err := client.selection.Profile.Canonical(false)
	if err != nil {
		t.Fatal(err)
	}
	client.selection.Profile.Signature = hex.EncodeToString(ed25519.Sign(client.key, canonical))
	client.digest, _ = client.selection.Digest()
	var accepted atomic.Uint64
	var refuse atomic.Bool
	var refusal atomic.Value
	refusal.Store(p.NetworkQuota)
	rejected := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/network/v1/challenge":
			_ = json.NewEncoder(w).Encode(p.NetworkChallengeResult{Version: "1", Profile: client.digest, Origin: client.origin, Challenge: randomNetworkID(), Expires: p.NetworkUint(time.Now().Unix() + 60)})
		case "/network/v1/control":
			conn, e := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if e != nil {
				return
			}
			defer conn.CloseNow()
			_, body, e := conn.Read(r.Context())
			if e != nil {
				return
			}
			var request p.NetworkLookupRequest
			if json.Unmarshal(body, &request) != nil {
				return
			}
			ack, _ := json.Marshal(p.NetworkResult{Version: "1", Operation: request.Proof.Operation, State: "accepted"})
			if conn.Write(r.Context(), websocket.MessageText, ack) != nil {
				return
			}
			_, _, _ = conn.Read(r.Context())
		case "/network/v1/announce":
			var request p.NetworkAnnounceRequest
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("invalid announcement")
				return
			}
			if request.Proof.Purpose == "peer_data" && refuse.Load() {
				code := refusal.Load().(string)
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(p.NetworkFailure{Version: "1", Code: code, Retryable: code == p.NetworkQuota, RetryAfter: 1})
				select {
				case rejected <- struct{}{}:
				default:
				}
				return
			}
			if request.Proof.Purpose == "peer_data" {
				accepted.Store(uint64(request.Announcement.Generation))
			}
			_ = json.NewEncoder(w).Encode(p.NetworkResult{Version: "1", Operation: request.Proof.Operation, State: "accepted"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	destination, _ := url.Parse(server.URL)
	client.http = &http.Client{Transport: w15ServiceTransport{server.Client().Transport, destination}}
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	runtime, err := NewRelayRuntime(context.Background(), client, manager, 1, client.digest)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = runtime.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	previous := accepted.Load()
	refuse.Store(true)
	runtime.NetworkChanged()
	select {
	case <-rejected:
	case <-ctx.Done():
		t.Fatal("renewal not attempted")
	}
	// The refusal is signalled before its response reaches the runtime.
	for runtime.serviceCooldown() == nil && ctx.Err() == nil {
		time.Sleep(time.Millisecond)
	}
	runtime.mu.Lock()
	endpoint := runtime.endpoints[PeerData]
	runtime.mu.Unlock()
	endpoint.mu.Lock()
	actual := endpoint.generation
	endpoint.mu.Unlock()
	if actual != previous || accepted.Load() != previous {
		t.Fatalf("rejected renewal changed offer generation: offered=%d service=%d previously=%d", actual, accepted.Load(), previous)
	}
	if !runtime.purposeReady(PeerData) {
		t.Fatal("quota-refused renewal withdrew a still-live accepted announcement")
	}
	// A semantic refusal withdraws readiness: dial/lookup then do no service work.
	refusal.Store(p.NetworkIdentityMismatch)
	// Change signals coalesce while a renewal is running; keep requesting one
	// with the quota cooldown cleared until the refusal is observed.
	for runtime.purposeReady(PeerData) && ctx.Err() == nil {
		runtime.mu.Lock()
		runtime.retryAfter = time.Time{}
		runtime.mu.Unlock()
		runtime.NetworkChanged()
		// Each signal also renews enrollment, spending the paced budget.
		time.Sleep(500 * time.Millisecond)
	}
	if runtime.purposeReady(PeerData) {
		t.Fatal("semantic refusal kept readiness")
	}
	var target Target
	device, _ := hex.DecodeString(client.device)
	copy(target.Device[:], device)
	pin, _ := hex.DecodeString(client.pin)
	copy(target.Pin[:], pin)
	profile, _ := hex.DecodeString(client.digest)
	copy(target.Profile[:], profile)
	target.Purpose = PeerData
	_, dialErr := runtime.Dial(ctx, target)
	_, lookupErr := runtime.LookupCandidates(ctx, target, 0)
	for _, err := range []error{dialErr, lookupErr} {
		var service *ServiceError
		if !errors.As(err, &service) || (service.Code != p.NetworkServiceUnavailable && service.Code != p.NetworkQuota) {
			t.Fatal("unannounced endpoint sent service work", err)
		}
	}
	t.Logf("quota-rejected renewal retains exact service-accepted offer generation=%d and readiness; semantic refusal withdraws it and unready dial/lookup do no service work", actual)
}
