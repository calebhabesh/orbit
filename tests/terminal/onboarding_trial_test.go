package terminal_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// Trial finding (2026-10-08): the laptop's join showed SETUP_BLOCKED with a
// generic "enrollment connection or TLS identity verification failed" while
// it could not reach the PC. Over a local routed service and two real
// daemons: with the inviter stopped, the joiner waits and says why; once the
// inviter is back, the request reaches it without the owner resuming anything.
// Never run against the deployed VPS.
func TestOnboardingTrialJoinWaitsForUnreachableInviter(t *testing.T) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	_, selection, origin, roots, _ := w05Service(t, func() {})
	conn, err := tls.Dial("tcp", strings.TrimPrefix(origin, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(base, "service-ca.pem")
	err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: conn.ConnectionState().PeerCertificates[0].Raw}), 0600)
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	cli := w06CLI{t: t, binary: buildOrbitBinary(t, base), ca: ca}
	profile := filepath.Join(base, "profile.json")
	data, _ := json.Marshal(selection)
	if err = os.WriteFile(profile, data, 0600); err != nil {
		t.Fatal(err)
	}
	stateA, stateB := filepath.Join(base, "a"), filepath.Join(base, "b")
	for _, dir := range []string{stateA, stateB} {
		e00StopOnCleanup(t, base, dir)
	}
	review := stateA + "-network.json"
	cli.ok("network", "preview", "--state", stateA, "--mode", "self_hosted", "--profile-file", profile, "--review-file", review, "--json")
	cli.ok("network", "apply", "--state", stateA, "--review-file", review, "--json")
	rootA, rootB := filepath.Join(base, "orbit-a"), filepath.Join(base, "orbit-b")
	if err = os.Mkdir(rootA, 0700); err != nil {
		t.Fatal(err)
	}
	setupFile := filepath.Join(base, "create.json")
	cli.ok("setup", "--state", stateA, "--root", rootA, "--label", "PC", "--name", "Trial", "--preview", "--review-file", setupFile, "--json")
	created := cli.ok("setup", "--state", stateA, "--request-file", setupFile, "--timeout", "0", "--json")
	w06Wait(t, stateA, created.Operation.ID)
	inviteReview, invFile := filepath.Join(base, "invite-review.json"), filepath.Join(base, "invitation.json")
	cli.ok("devices", "invite", "--state", stateA, "--folder", "Trial", "--preview", "--review-file", inviteReview, "--json")
	if _, out, e := cli.call("", "devices", "invite", "--state", stateA, "--request-file", inviteReview, "--out", invFile, "--json"); e != nil {
		t.Fatal(e, out)
	}

	// The inviting device goes away before the joiner submits.
	if err = app.StopAgent(stateA, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	joinFile := filepath.Join(base, "join.json")
	cli.ok("join", "--state", stateB, "--root", rootB, "--label", "Laptop", "--name", "Trial", "--invitation-file", invFile, "--preview", "--review-file", joinFile, "--json")
	joined, out, e := cli.call("", "join", "--state", stateB, "--request-file", joinFile, "--timeout", "0", "--json")
	if e != nil || joined.Operation == nil {
		t.Fatalf("submit: %v %s", e, out)
	}
	var waiting tc.Result
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		waiting, err = (&controlclient.Client{StateDir: stateB}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: joined.Operation.ID})
		if err != nil {
			t.Fatal(err)
		}
		if waiting.Error != nil {
			t.Fatalf("join blocked while the inviter is away: %+v", waiting.Error)
		}
		if reconnecting(waiting) {
			break
		}
	}
	if !reconnecting(waiting) {
		t.Fatalf("join never said it is reconnecting: state=%s effects=%+v", waiting.State, waiting.Effects)
	}
	t.Logf("while the inviter is stopped: state=%s phase=%s effects=%+v", waiting.State, waiting.Operation.Phase, waiting.Effects)

	// The inviter comes back; the request arrives without resuming the join
	// by hand.
	serve := exec.Command(cli.binary, "serve", "--state="+stateA, "--control-listen=127.0.0.1:0")
	serve.Env = append(os.Environ(), "SSL_CERT_FILE="+ca)
	serveLog, _ := os.Create(filepath.Join(base, "serve-a.log"))
	serve.Stdout, serve.Stderr = serveLog, serveLog
	if err = serve.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = serve.Wait() }()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if _, e := os.Stat(filepath.Join(stateA, "control.addr")); e == nil {
			break
		}
	}
	if st, e := (&controlclient.Client{StateDir: stateA}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"}); e != nil {
		t.Logf("inviter not answering after restart: %v", e)
	} else {
		t.Logf("inviter back: network code=%s ready=%v", st.Network.Code, st.Network.Ready)
	}
	var pending tc.Result
	for deadline := time.Now().Add(120 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
		if pending = cli.ok("devices", "requests", "--state", stateA, "--json"); len(pending.Requests) == 1 {
			break
		}
	}
	if len(pending.Requests) != 1 {
		b, _ := os.ReadFile(filepath.Join(base, "serve-a.log"))
		final, _ := (&controlclient.Client{StateDir: stateB}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: joined.Operation.ID})
		t.Logf("inviter serve log:\n%s", b)
		t.Logf("joiner operation: state=%s phase=%s err=%+v effects=%+v", final.State, final.Operation.Phase, final.Error, final.Effects)
		t.Fatalf("request never reached the returning inviter: %+v", pending.Requests)
	}
}

func reconnecting(r tc.Result) bool {
	for _, e := range r.Effects {
		if strings.HasPrefix(e.State, "reconnecting:") {
			return true
		}
	}
	return false
}
