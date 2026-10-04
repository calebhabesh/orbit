package terminalcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func id(n string) string { return strings.Repeat(n, 64) }
func review() Review {
	return Review{Token: id("1"), Generation: id("2"), ExpiresAt: "2026-10-04T00:00:00Z"}
}
func settings() Settings {
	return Settings{DataBudget: 10 << 30, MetadataBudget: 256 << 20, ReserveBytes: 512 << 20, Concurrency: 4, Startup: "login"}
}
func TestTerminalT01IntegersAndStrictCodec(t *testing.T) {
	for _, s := range []string{`"0"`, `"9007199254740993"`, `"18446744073709551615"`} {
		var n Uint
		if err := Decode([]byte(s), &n); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(n)
		if string(b) != s {
			t.Fatalf("lost precision %s -> %s", s, b)
		}
	}
	for _, s := range []string{`0`, `"01"`, `"+1"`, `"-1"`, `"1.0"`, `"18446744073709551616"`, `null`, `""`} {
		var n Uint
		if Decode([]byte(s), &n) == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	for _, s := range []string{`{"version":"1","version":"1"}`, `{"unknown":true}`, `{} {}`} {
		var r Result
		if Decode([]byte(s), &r) == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	var r Result
	if Decode(bytes.Repeat([]byte(" "), MaxMetadata+1), &r) == nil {
		t.Fatal("metadata unbounded")
	}
}
func TestTerminalT01MutationContracts(t *testing.T) {
	v := VersionID{Folder: id("3"), Author: id("4"), Counter: 9007199254740993}
	c := Context{Folder: v.Folder, Path: "notes.txt", Generation: id("5")}
	// All operation families except join (certificate fixture tested separately).
	cases := []Mutation{
		{Kind: "setup", Setup: &SetupIntent{DeviceName: "laptop", FolderName: "notes", Root: "/tmp/Orbit", Preview: review(), Settings: settings()}},
		{Kind: "adopt", Setup: &SetupIntent{DeviceName: "laptop", FolderName: "notes", Root: "/tmp/notes", Preview: review(), Settings: settings()}},
		{Kind: "invite", Invite: &InviteIntent{Folder: v.Folder, ExpectedMembership: id("6"), ExpiresAt: review().ExpiresAt}},
		{Kind: "share", Invite: &InviteIntent{Folder: v.Folder, Device: id("7"), ExpectedMembership: id("6"), ExpiresAt: review().ExpiresAt}},
		{Kind: "approval", Approval: &ApprovalIntent{Request: id("8"), Folder: v.Folder, Requester: id("7"), KeyPin: id("9"), TranscriptDigest: id("a"), ExpectedMembership: id("6"), Decision: "approve"}},
		{Kind: "folder", Folder: &FolderIntent{Folder: v.Folder, Review: review(), Action: "relocate", ExpectedRoot: "/tmp/notes", Destination: "/tmp/new-notes"}},
		{Kind: "content", Content: &ContentIntent{Context: c, Review: review(), Heads: []VersionID{v}, Source: v, Action: "select"}},
		{Kind: "content", Content: &ContentIntent{Context: c, Review: review(), Heads: []VersionID{v}, Source: v, Action: "restore"}},
		{Kind: "content", Content: &ContentIntent{Context: c, Review: review(), Heads: []VersionID{v}, Source: v, Action: "separate_copy", Destination: "recovered.txt", DestinationReview: review()}},
		{Kind: "content", Content: &ContentIntent{Context: c, Review: review(), Heads: []VersionID{v}, Action: "merge", Session: id("b"), Upload: id("c"), Digest: id("d"), Bytes: 9007199254740993}},
		{Kind: "content", Content: &ContentIntent{Context: c, Review: review(), Heads: []VersionID{v}, Action: "keep_copies", Copies: []CopyPlan{{Source: v, Destination: "copy.txt", Review: review()}}}},
		{Kind: "session", Session: &SessionIntent{Action: "create", Context: c, Review: review(), Heads: []VersionID{v}, Sources: []VersionID{v}}},
		{Kind: "settings", Settings: &SettingsIntent{Review: review(), Settings: settings()}},
		{Kind: "service", Service: &ServiceIntent{Review: review(), Action: "start", Mode: "login"}},
		{Kind: "cancel", Cancel: &CancelIntent{Target: id("e")}},
	}
	for _, m := range cases {
		m.Version = Version
		m.OperationID = id("f")
		if err := m.Validate(); err != nil {
			t.Fatalf("%s: %v", m.Kind, err)
		}
		b, _ := json.Marshal(m)
		var decoded Mutation
		if err := Decode(b, &decoded); err != nil {
			t.Fatal(err)
		}
		fp, _ := m.Fingerprint()
		got, _ := decoded.Fingerprint()
		if fp != got {
			t.Fatal("roundtrip changed fingerprint")
		}
		decoded.OperationID = id("a")
		got, _ = decoded.Fingerprint()
		if got != fp {
			t.Fatal("identity entered payload fingerprint")
		}
		decoded.Version = "2"
		if decoded.Validate() == nil {
			t.Fatal("unsupported version")
		}
	}
	m := cases[0]
	m.Version = Version
	m.OperationID = id("f")
	m.Cancel = &CancelIntent{Target: id("e")}
	if m.Validate() == nil {
		t.Fatal("mixed intent accepted")
	}
	m.Cancel = nil
	m.Kind = "service"
	if m.Validate() == nil {
		t.Fatal("wrong tag accepted")
	}
	m = Mutation{Version: Version, OperationID: id("f"), Kind: "content", Content: &ContentIntent{Context: c, Review: review(), Heads: []VersionID{v, v}, Source: v, Action: "restore"}}
	if m.Validate() == nil {
		t.Fatal("duplicate heads accepted")
	}
	m.Content.Heads = []VersionID{v}
	m.Content.Context.Path = "../escape"
	if m.Validate() == nil {
		t.Fatal("unsafe path accepted")
	}
}
func TestTerminalT01Fixtures(t *testing.T) {
	files, err := filepath.Glob("../../../schemas/fixtures/terminal-v1/*.json")
	if err != nil || len(files) != 11 {
		t.Fatalf("fixture discovery: %d %v", len(files), err)
	}
	seen := map[string]bool{}
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var r Result
		if err := Decode(b, &r); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if r.Version != Version || r.Items == nil || r.Attention == nil || r.Observations == nil || r.Effects == nil || r.Requests == nil {
			t.Fatalf("incomplete fixture %s", p)
		}
		if r.Invitation != nil {
			t.Fatal("secret in view fixture")
		}
		seen[r.State] = true
		b2, _ := json.Marshal(r)
		var r2 Result
		if err := Decode(b2, &r2); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"success", "empty", "loading", "awaiting_approval", "offline", "stale", "storage_blocked", "root_unavailable", "partial", "fork", "conflict"} {
		if !seen[s] {
			t.Fatalf("missing %s", s)
		}
	}
}
func TestTerminalT01QueriesAndExits(t *testing.T) {
	for _, k := range []string{"capabilities", "context", "root_preview", "operation", "status", "attention", "devices", "folders", "requests", "history", "deleted", "content_review", "session", "settings", "service", "doctor"} {
		q := Query{Version: Version, Kind: k, Folder: id("1"), Path: "notes.txt", ID: id("2"), Limit: 200}
		if k == "root_preview" {
			q.Path = "/tmp/notes"
		}
		if err := q.Validate(); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		q.Limit = 201
		if q.Validate() == nil {
			t.Fatal("unbounded query")
		}
	}
	for code, want := range map[string]int{"INVALID_REQUEST": 2, "UNAUTHORIZED": 3, "STALE_VIEW": 4, "EXPIRED_REPLAY": 4, "OFFLINE": 5, "DISK_BUDGET": 6, "INCOMPATIBLE_VERSION": 7, "CANCELED": 130, "IO_ERROR": 1} {
		if got := ExitCode(Result{Error: &Error{Code: code}}); got != want {
			t.Fatalf("%s: %d", code, got)
		}
	}
}

func TestTerminalT01FingerprintBindsReview(t *testing.T) {
	m := Mutation{Version: Version, OperationID: id("f"), Kind: "setup", Setup: &SetupIntent{DeviceName: "laptop", FolderName: "notes", Root: "/tmp/notes", Preview: review(), Settings: settings()}}
	first, _ := m.Fingerprint()
	m.Setup.Preview.Generation = id("9")
	second, _ := m.Fingerprint()
	if first == second {
		t.Fatal("review generation not bound")
	}
	m.Setup.Settings.DataBudget++
	third, _ := m.Fingerprint()
	if second == third {
		t.Fatal("settings not bound")
	}
}

func TestTerminalT01JoinInvitation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	cert := server.Certificate()
	pin := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	invitation := Invitation{Version: Version, Folder: id("1"), Inviter: id("2"), KeyPin: hex.EncodeToString(pin[:]), CertificateDER: base64.StdEncoding.EncodeToString(cert.Raw), EnrollmentEndpoint: server.URL, PeerEndpoint: "https://peer.test:7443", Capability: id("3"), ExpiresAt: review().ExpiresAt}
	m := Mutation{Version: Version, OperationID: id("f"), Kind: "join", Join: &JoinIntent{Invitation: invitation, Attempt: id("4"), DeviceName: "Laptop", FolderName: "Notes", Root: "/tmp/notes", Preview: review(), Settings: settings()}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m)
	var got Mutation
	if err := Decode(b, &got); err != nil || got.Validate() != nil {
		t.Fatal("join codec")
	}
	first, _ := m.Fingerprint()
	m.Join.Attempt = id("5")
	second, _ := m.Fingerprint()
	if first == second {
		t.Fatal("attempt fingerprint")
	}
	m.Join.Invitation.KeyPin = id("6")
	if m.Validate() == nil {
		t.Fatal("inviter pin mismatch accepted")
	}
	m.Join.Invitation = invitation
	m.Join.Invitation.EnrollmentEndpoint = "http://peer.test"
	if m.Validate() == nil {
		t.Fatal("plaintext enrollment accepted")
	}
}

func TestTerminalT01UploadFingerprint(t *testing.T) {
	u := UploadIntent{OperationID: id("1"), Session: id("2"), Bytes: 64 << 30, Digest: id("3")}
	a, err := u.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	u.Session = id("4")
	b, _ := u.Fingerprint()
	if a == b {
		t.Fatal("upload session not bound")
	}
	u.Bytes++
	if _, err := u.Fingerprint(); err == nil {
		t.Fatal("oversize upload admitted")
	}
}

func TestTerminalT01StateExitCategories(t *testing.T) {
	for state, want := range map[string]int{"success": 0, "empty": 0, "failed": 1, "partial": 4, "awaiting_approval": 5, "root_unavailable": 5, "storage_blocked": 6, "canceled": 130} {
		if got := ExitCode(Result{State: state}); got != want {
			t.Fatalf("%s: %d", state, got)
		}
	}
}
