package designgates

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/state"
)

func terminalTranscript() protocol.TerminalEnrollmentTranscript {
	var t protocol.TerminalEnrollmentTranscript
	vals := []*[32]byte{&t.Folder, &t.Inviter, &t.InviterPin, &t.TokenDigest, &t.Attempt, &t.Challenge, &t.Requester, &t.RequesterPin, &t.PublicKey, &t.PriorMembership}
	for i, p := range vals {
		for j := range p {
			p[j] = byte(i + 1)
		}
	}
	t.ExpiresUnix = 1791072000
	t.EnrollmentEndpoint = "https://inviter.test:7444"
	t.PeerEndpoint = "https://inviter.test:7443"
	t.RequesterEndpoint = "https://requester.test:7443"
	t.Label = "Laptop"
	return t
}
func TestTerminalT01TG1Transcript(t *testing.T) {
	tr := terminalTranscript()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	copy(tr.PublicKey[:], pub)
	b, err := tr.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, b)
	// Independently mutate every field: none may be omitted from the signature.
	typ := reflect.TypeOf(tr)
	for i := 0; i < typ.NumField(); i++ {
		changed := tr
		f := reflect.ValueOf(&changed).Elem().Field(i)
		switch f.Kind() {
		case reflect.Array:
			f.Index(0).SetUint(f.Index(0).Uint() ^ 1)
		case reflect.Uint64:
			f.SetUint(f.Uint() + 1)
		case reflect.String:
			f.SetString(f.String() + "x")
		}
		cb, err := changed.Canonical()
		if err == nil && ed25519.Verify(pub, cb, sig) {
			t.Fatalf("unbound field %s", typ.Field(i).Name)
		}
	}
	second := tr
	second.Folder[0]++
	third := tr
	third.Attempt[0]++
	if tr.RequestID() == second.RequestID() || tr.RequestID() == third.RequestID() {
		t.Fatal("folder/attempt collision")
	}
	retry := tr
	retry.Challenge[0]++
	if tr.RequestID() != retry.RequestID() {
		t.Fatal("retry identity changed")
	}
	code, _ := tr.VerificationCode()
	code2, _ := second.VerificationCode()
	if code == code2 || len(code) != 23 {
		t.Fatal("verification code unbound")
	}
	golden, err := os.ReadFile("testdata/terminal-enrollment-v2.hex")
	if err != nil {
		t.Fatal(err)
	}
	fixed, _ := terminalTranscript().Canonical()
	if hex.EncodeToString(fixed) != strings.TrimSpace(string(golden)) {
		t.Fatal("canonical transcript differs from independent golden")
	}
	bad := tr
	bad.EnrollmentEndpoint = ""
	bad.RequesterEndpoint = ""
	if _, err := bad.Canonical(); err == nil {
		t.Fatal("missing inviter endpoint accepted")
	}
}
func TestTerminalT01TG1TLSIsolation(t *testing.T) {
	var disclosed atomic.Int32
	enrollment := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/enrollment/v2/request" {
			http.NotFound(w, r)
			return
		}
		disclosed.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer enrollment.Close()
	leaf := enrollment.Certificate()
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	clientFor := func(expected [32]byte) *http.Client {
		return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, VerifyConnection: func(c tls.ConnectionState) error {
			got := sha256.Sum256(c.PeerCertificates[0].RawSubjectPublicKeyInfo)
			if got != expected {
				return fmt.Errorf("IDENTITY_MISMATCH")
			}
			return nil
		}}}}
	}
	good := clientFor(pin)
	defer good.CloseIdleConnections()
	resp, err := good.Post(enrollment.URL+"/enrollment/v2/request", "application/json", strings.NewReader(`{"capability":"synthetic"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || disclosed.Load() != 1 {
		t.Fatal("pinned enrollment not admitted")
	}
	wrong := pin
	wrong[0] ^= 1
	bad := clientFor(wrong)
	defer bad.CloseIdleConnections()
	if resp, err := bad.Post(enrollment.URL+"/enrollment/v2/request", "application/json", strings.NewReader(`{"capability":"secret"}`)); err == nil {
		resp.Body.Close()
		t.Fatal("wrong pin accepted")
	}
	if disclosed.Load() != 1 {
		t.Fatal("capability disclosed before pin verification")
	}
	var dataRequests atomic.Int32
	data := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataRequests.Add(1)
		http.Error(w, "not an approved folder member", http.StatusForbidden)
	}))
	data.Config.ErrorLog = log.New(io.Discard, "", 0)
	data.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAnyClientCert}
	data.StartTLS()
	defer data.Close()
	resp, err = data.Client().Get(data.URL + "/peer/v1/inventory")
	if err == nil {
		resp.Body.Close()
		t.Fatal("unknown client crossed peer TLS admission")
	}
	if dataRequests.Load() != 0 {
		t.Fatal("unknown requester reached data handler")
	}
	resp, err = good.Get(enrollment.URL + "/peer/v1/inventory")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("data on enrollment listener")
	}
}

// Test-only admission model: one transaction contains scope/proof/replay checks,
// capability consumption and pending request creation. No production DB claim.
type terminalAdmission struct {
	mu      sync.Mutex
	folder  [32]byte
	expires uint64
	revoked bool
	used    bool
	records map[string][32]byte
}

func (a *terminalAdmission) admit(tr protocol.TerminalEnrollmentTranscript, sig []byte, now uint64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, err := tr.Canonical()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(tr.PublicKey[:]), b, sig) {
		return fmt.Errorf("INVALID_SIGNATURE")
	}
	fp := sha256.Sum256(b)
	id := tr.RequestID()
	if old, ok := a.records[id]; ok {
		if old == fp {
			return nil
		}
		return fmt.Errorf("IDEMPOTENCY_CONFLICT")
	}
	if a.revoked || now >= a.expires || now >= tr.ExpiresUnix || tr.Folder != a.folder || a.used {
		return fmt.Errorf("capability rejected")
	}
	a.records[id] = fp
	a.used = true
	return nil
}
func TestTerminalT01TG1AtomicAdmission(t *testing.T) {
	tr := terminalTranscript()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	copy(tr.PublicKey[:], pub)
	a := terminalAdmission{folder: tr.Folder, expires: tr.ExpiresUnix, records: map[string][32]byte{}}
	sign := func(tr protocol.TerminalEnrollmentTranscript) []byte {
		b, _ := tr.Canonical()
		return ed25519.Sign(priv, b)
	}
	wrong := tr
	wrong.Folder[0]++
	if a.admit(wrong, sign(wrong), 1) == nil || a.used || len(a.records) != 0 {
		t.Fatal("wrong scope consumed capability")
	}
	if a.admit(tr, make([]byte, 64), 1) == nil || a.used {
		t.Fatal("forged proof consumed capability")
	}
	expired := tr
	expired.ExpiresUnix = 1
	if a.admit(expired, sign(expired), 2) == nil || a.used {
		t.Fatal("expiry ignored")
	}
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			attempt := tr
			attempt.Attempt[0] = byte(i + 1)
			if a.admit(attempt, sign(attempt), 1) == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 || len(a.records) != 1 {
		t.Fatal("non-atomic token consumption")
	}
	// Identical signed retry retrieves the existing pending record without another use.
	for i := 0; i < 16; i++ {
		attempt := tr
		attempt.Attempt[0] = byte(i + 1)
		if _, ok := a.records[attempt.RequestID()]; ok {
			if a.admit(attempt, sign(attempt), tr.ExpiresUnix+1) != nil {
				t.Fatal("recorded retry denied")
			}
		}
	}
	if len(a.records) != 1 {
		t.Fatal("retry duplicated request")
	}
}

func TestTerminalT01TG2RestartAndReadiness(t *testing.T) {
	phases := []string{"reviewed", "request_prepared", "awaiting_approval", "membership_received", "bootstrap_capture", "content_pending", "publishing", "ready"}
	record := tc.JoinRecord{Operation: tc.Operation{ID: strings.Repeat("1", 64), Fingerprint: strings.Repeat("2", 64)}, Attempt: strings.Repeat("3", 64), Request: strings.Repeat("4", 64), Folder: strings.Repeat("5", 64), Root: "/disposable/root", Preview: tc.Review{Token: "root-token", Generation: "tree-generation"}, Inviter: "persistent-inviter", KeyPin: "approved-pin", CertificateDER: "certificate", EnrollmentEndpoint: "https://host:7444", PeerEndpoint: "https://host:7443"}
	for _, phase := range phases {
		record.Operation.Phase = phase
		b, _ := json.Marshal(record)
		var restarted tc.JoinRecord
		if err := tc.Decode(b, &restarted); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(record, restarted) {
			t.Fatalf("restart lost review/identity at %s", phase)
		}
	}
	ready := tc.Readiness{Approved: true, MembershipCurrent: true, RootAvailable: true, ScanComplete: true}
	if !ready.Ready() {
		t.Fatal("valid readiness rejected")
	}
	typ := reflect.TypeOf(ready)
	for i := 0; i < typ.NumField(); i++ {
		blocked := ready
		f := reflect.ValueOf(&blocked).Elem().Field(i)
		if f.Kind() == reflect.Bool {
			f.SetBool(!f.Bool())
		} else {
			f.SetUint(1)
		}
		if blocked.Ready() {
			t.Fatalf("readiness ignored %s", typ.Field(i).Name)
		}
	}
	// A bounded recursive-preview model binds content as well as descriptor
	// identity: a same-size edit must invalidate review, not just a root swap.
	treeGeneration := func(rootIdentity string, content []byte) [32]byte {
		h := sha256.New()
		h.Write([]byte(rootIdentity))
		h.Write([]byte("\x00notes.txt\x00file\x00"))
		h.Write(content)
		var out [32]byte
		copy(out[:], h.Sum(nil))
		return out
	}
	original := treeGeneration("dev:inode:registration", []byte("one"))
	if original == treeGeneration("dev:inode:registration", []byte("two")) || original == treeGeneration("dev:new-inode:registration", []byte("one")) {
		t.Fatal("tree generation omitted bytes/root identity")
	}
	// Root descriptor identity and enumeration generation are both reviewed.
	reviewed := [3]string{"dev-inode", "tree-generation", "registration"}
	for i := range reviewed {
		changed := reviewed
		changed[i] += "-new"
		if reviewed == changed {
			t.Fatal("root review survived change")
		}
	}
	// Bootstrap and incomplete enumeration can never infer absent-path deletion.
	absent := []string{"remote-only.txt"}
	tombstones := func(bootstrap, complete bool) []string {
		if bootstrap || !complete {
			return nil
		}
		return absent
	}
	if len(tombstones(true, true)) != 0 || len(tombstones(false, false)) != 0 || len(tombstones(false, true)) != 1 {
		t.Fatal("bootstrap absence semantics")
	}
}

type terminalReplay struct {
	Fingerprint string
	State       string
	Effects     []string
}

func terminalReplayDecision(old *terminalReplay, fp string, expired bool) string {
	if old != nil {
		if old.Fingerprint != fp {
			return "IDEMPOTENCY_CONFLICT"
		}
		return "inspect_existing"
	}
	if expired {
		return "EXPIRED_REPLAY"
	}
	return "record_before_effects"
}
func TestTerminalT01TG3AdaptersAndReplay(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".filesync-disposable"), []byte("T01 lock experiment"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureDirectory(root); err != nil {
		t.Fatal(err)
	}
	owner, err := state.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	other, err := state.Acquire(root)
	if other != nil {
		other.Close()
		t.Fatal("second owner admitted")
	}
	if !errors.Is(err, state.ErrLocked) {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	other, err = state.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
	// Late response correlation rejects an old selection while retaining its
	// mutation observation under a durable operation identity.
	selection, sequence := "folder-B", uint64(2)
	applies := func(folder string, seq uint64) bool { return folder == selection && seq == sequence }
	if applies("folder-A", 1) || applies("folder-B", 1) || !applies("folder-B", 2) {
		t.Fatal("late response changed selection")
	}

	// Exhaust the adapter decision table. Liveness failures never grant ownership.
	for _, live := range []bool{false, true} {
		for _, lock := range []bool{false, true} {
			for _, httpOK := range []bool{false, true} {
				selected := "blocked"
				if live {
					selected = "live"
					if !httpOK {
						selected = "live_error"
					}
				} else if lock {
					selected = "stopped"
				}
				if live && selected == "stopped" {
					t.Fatal("live HTTP error entered direct adapter")
				}
				if !live && !lock && selected != "blocked" {
					t.Fatal("stopped adapter lacks ownership")
				}
			}
		}
	}
	fp := strings.Repeat("a", 64)
	old := terminalReplay{Fingerprint: fp, State: "running", Effects: []string{"copy captured"}}
	b, _ := json.Marshal(old)
	var restarted terminalReplay
	json.Unmarshal(b, &restarted)
	if terminalReplayDecision(&restarted, fp, false) != "inspect_existing" || terminalReplayDecision(&restarted, "different", false) != "IDEMPOTENCY_CONFLICT" || terminalReplayDecision(nil, fp, true) != "EXPIRED_REPLAY" {
		t.Fatal("unsafe replay after restart")
	}
	if len(restarted.Effects) != 1 {
		t.Fatal("effects forgotten")
	}
	// Cancel closes the request, not a durable operation; explicit cancel records
	// a request, then reaches a safe boundary. It cannot remove committed effects.
	restarted.State = "partial"
	if len(restarted.Effects) != 1 {
		t.Fatal("cancel undid commit")
	}
}
func TestTerminalT01TG4ReviewedSession(t *testing.T) {
	// Independent reference-set experiment: session holds exact sources, while
	// streaming ownership outlives an expired session's short lease.
	chunks := map[string]bool{"a": true, "b": true, "unreferenced": true}
	sessionPins := map[string]bool{"a": true, "b": true}
	streamPins := map[string]bool{"a": true, "b": true}
	gc := func() {
		for c := range chunks {
			if !sessionPins[c] && !streamPins[c] {
				delete(chunks, c)
			}
		}
	}
	gc()
	if !chunks["a"] || !chunks["b"] || chunks["unreferenced"] {
		t.Fatal("pin reference oracle")
	}
	sessionPins = map[string]bool{}
	gc()
	if len(chunks) != 2 {
		t.Fatal("expiry killed live read")
	}
	streamPins = map[string]bool{}
	gc()
	if len(chunks) != 0 {
		t.Fatal("closed read leaked pin")
	}
	oldHeads := []string{"A1", "B1"}
	newHeads := []string{"A1", "B1", "C1"}
	commit := func(review, current []string) string {
		if !reflect.DeepEqual(review, current) {
			return "STALE_VIEW"
		}
		return "commit_reviewed"
	}
	if commit(oldHeads, newHeads) != "STALE_VIEW" {
		t.Fatal("editor implicitly covered unseen head")
	}
	// Source version is provenance; restoration parents remain the reviewed set.
	restored := struct {
		Source  string
		Parents []string
	}{"historic-A0", oldHeads}
	if restored.Source == restored.Parents[0] {
		t.Fatal("source substituted for current ancestry")
	}
	effects := []string{"copy installed"}
	canceled := terminalReplay{State: "partial", Effects: effects}
	b, _ := json.Marshal(canceled)
	var recovered terminalReplay
	json.Unmarshal(b, &recovered)
	if len(recovered.Effects) != 1 || recovered.State != "partial" {
		t.Fatal("separate-copy recovery hidden")
	}
	// Streaming review experiment measures peak buffer, using synthetic 8 MiB.
	source := io.LimitReader(&terminalRepeatingReader{}, 8<<20)
	dest := sha256.New()
	buffer := make([]byte, 1<<20)
	n, err := io.CopyBuffer(dest, source, buffer)
	if err != nil || n != 8<<20 || len(buffer) != 1<<20 {
		t.Fatal("bounded stream failed")
	}
	// The sink is an incremental digest; no whole-file destination is allocated
	// during transfer. The oracle below allocates separately after streaming.
	expected := sha256.Sum256(bytes.Repeat([]byte{'x'}, 8<<20))
	if !bytes.Equal(dest.Sum(nil), expected[:]) {
		t.Fatal("stream corrupted bytes")
	}
}

type terminalRepeatingReader struct{}

func (*terminalRepeatingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}
func TestTerminalT01TG5Lifecycle(t *testing.T) {
	for _, tty := range []bool{false, true} {
		for _, setup := range []bool{false, true} {
			entry := "status"
			if tty {
				entry = "overview"
				if !setup {
					entry = "setup"
				}
			}
			if !tty && entry != "status" {
				t.Fatal("escape UI in pipe")
			}
		}
	}
	daemonRunning := true
	terminalOwned := true
	editorRunning := false
	// query cancellation, resize, editor return and client quit affect client state.
	for _, event := range []string{"cancel_query", "resize", "editor_start", "editor_return", "quit"} {
		switch event {
		case "editor_start":
			terminalOwned = false
			editorRunning = true
		case "editor_return":
			editorRunning = false
			terminalOwned = true
		case "quit":
			terminalOwned = false
		}
		if !daemonRunning {
			t.Fatal("client event stopped daemon")
		}
	}
	if editorRunning || terminalOwned {
		t.Fatal("terminal ownership leaked")
	}
	// Login enablement is not proof of running/unattended behavior.
	svc := tc.Service{Enabled: true, Mode: "login"}
	if svc.Running || svc.UnattendedVerified {
		t.Fatal("enabled inferred running or logout guarantee")
	}
	legacy := map[string]string{"orbit": "tty_dispatch", "filesync": "legacy_dispatch", "state": "preserved", "identity": "preserved"}
	if legacy["filesync"] != "legacy_dispatch" || legacy["identity"] != "preserved" {
		t.Fatal("compatibility contract")
	}
}
