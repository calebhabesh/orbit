package terminal_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func t08Version(id history.VersionID) tc.VersionID {
	return tc.VersionID{Folder: hex.EncodeToString(id.Folder[:]), Author: hex.EncodeToString(id.Author[:]), Counter: tc.Uint(id.Counter)}
}
func t08Capture(t *testing.T, f *fixture, folder history.ID, name, path, content string) history.Envelope {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.root, name, path), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	scan, err := f.ws.Scan(context.Background(), folder)
	if err != nil || len(scan.Captured) != 1 {
		t.Fatalf("capture: %+v %v", scan, err)
	}
	return scan.Captured[0]
}
func t08Remote(t *testing.T, f *fixture, folder history.ID, path, content string, author byte) history.Envelope {
	t.Helper()
	ctx := context.Background()
	manifest, err := f.db.StoreFile(ctx, strings.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	membership, err := f.ctrl.MembershipExport(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	next := membership.Membership
	next.Revision++
	next.PriorDigest = membership.Digest
	next.Active = append(next.Active, protocol.ActiveMember{Device: history.ID{author}, KeyPin: history.Digest{author}})
	if _, err = f.db.ApproveMembership(ctx, next); err != nil {
		t.Fatal(err)
	}
	id := history.VersionID{Folder: folder, Author: history.ID{author}, Counter: 1}
	env := history.Envelope{ID: id, Path: path, Kind: history.KindFile, Manifest: manifest, Vector: []history.ClockEntry{{Author: id.Author, Counter: 1}}, AuthoredRevision: next.Revision}
	if err = f.db.ImportMetadata(ctx, env); err != nil {
		t.Fatal(err)
	}
	if err = f.db.MarkContentReady(ctx, id); err != nil {
		t.Fatal(err)
	}
	return env
}
func t08Review(t *testing.T, c *controlclient.Client, folder history.ID, path string, source *history.VersionID, destination string) tc.ContentReview {
	t.Helper()
	q := tc.Query{Version: tc.Version, Kind: "content_review", Folder: hex.EncodeToString(folder[:]), Path: path, Destination: destination}
	if source != nil {
		v := t08Version(*source)
		q.Source = &v
	}
	r, err := c.Query(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return *r.ContentReview
}
func t08Mutation(v tc.ContentReview, action string, source history.VersionID, op string) tc.Mutation {
	return tc.Mutation{Version: tc.Version, Kind: "content", OperationID: strings.Repeat(op, 64/len(op)), Content: &tc.ContentIntent{Context: v.Context, Review: v.Review, Heads: v.Heads, Action: action, Source: t08Version(source)}}
}
func t08Code(t *testing.T, err error, code string) {
	t.Helper()
	var ce *control.ControlError
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestTerminalT08ExactReadsLiveStoppedAndGC(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("reads")
	original := strings.Repeat("verified original bytes", 400000)
	v := t08Capture(t, f, folder, "reads", "doc", original)
	c := terminalClient(t, f)
	r, err := c.Read(ctx, tc.ReadIntent{Version: t08Version(v.ID), Offset: 9, Length: 8})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil || string(data) != "original" {
		t.Fatalf("range=%q %v", data, err)
	}
	r.Close()
	r, err = c.Read(ctx, tc.ReadIntent{Version: t08Version(v.ID)})
	if err != nil {
		t.Fatal(err)
	}
	t08Capture(t, f, folder, "reads", "doc", "new captured bytes")
	policy := repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	if err = f.db.SetRetentionPolicy(ctx, folder, policy); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.RunGC(ctx, folder, &policy, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(r)
	r.Close()
	if err != nil || string(data) != original {
		t.Fatalf("GC read length=%d %v", len(data), err)
	}

	// Once the protected response closes, another inspected GC pass may expire
	// the superseded payload; source identity is never substituted on reopen.
	if _, err = f.db.RunGC(ctx, folder, &policy, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.close()
	latest, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "history", Folder: hex.EncodeToString(folder[:]), Path: "doc"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = c.Read(ctx, tc.ReadIntent{Version: latest.Versions[0].Version})
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(r)
	r.Close()
	if err != nil || string(data) != "new captured bytes" {
		t.Fatalf("stopped read=%q %v", data, err)
	}
	_, err = c.Read(ctx, tc.ReadIntent{Version: t08Version(v.ID)})
	t08Code(t, err, "CONTENT_EXPIRED")
	expiredReview := t08Review(t, c, folder, "doc", &v.ID, "")
	_, err = c.Mutate(ctx, t08Mutation(expiredReview, "restore", v.ID, "1f"))
	t08Code(t, err, "CONTENT_EXPIRED")

}
func TestTerminalT08ReviewedSelectReplayAndStale(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("select")
	a := t08Capture(t, f, folder, "select", "doc", "local")
	b := t08Remote(t, f, folder, "doc", "remote", 71)
	c := terminalClient(t, f)
	review := t08Review(t, c, folder, "doc", nil, "")
	if len(review.Heads) != 2 || !review.WorkingCaptured {
		t.Fatalf("review=%+v", review)
	}
	omitted := t08Mutation(review, "select", a.ID, "1")
	omitted.Content.Heads = nil
	if _, err := c.Mutate(ctx, omitted); err == nil {
		t.Fatal("omitted review accepted")
	}
	m := t08Mutation(review, "select", b.ID, "2")
	r, err := c.Mutate(ctx, m)
	if err != nil || r.State != "completed" {
		t.Fatalf("select=%+v %v", r, err)
	}
	first := r.Effects[0].Version
	r, err = c.Mutate(ctx, m)
	if err != nil || *r.Effects[0].Version != *first {
		t.Fatalf("replay=%+v %v", r, err)
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "select", "doc"))
	if string(data) != "remote" {
		t.Fatalf("selected bytes=%q", data)
	}
	id := history.VersionID{Folder: folder, Author: history.ID{}, Counter: uint64(first.Counter)}
	_ = id.Author.UnmarshalText([]byte(first.Author))
	env, err := f.db.Envelope(ctx, id)
	if err != nil || len(env.Parents) != 2 {
		t.Fatalf("parents=%+v %v", env.Parents, err)
	}
	_, err = c.Mutate(ctx, t08Mutation(review, "select", a.ID, "3"))
	t08Code(t, err, "STALE_VIEW")

	currentEnv := t08Capture(t, f, folder, "select", "doc", "later captured bytes")
	replay, err := c.Mutate(ctx, m)
	if err != nil || *replay.Effects[0].Version != *first {
		t.Fatal("late completed replay changed identity")
	}
	data, _ = os.ReadFile(filepath.Join(f.root, "select", "doc"))
	if string(data) != "later captured bytes" {
		t.Fatal("completed replay reapplied older bytes")
	}
	freshReview := t08Review(t, c, folder, "doc", nil, "")
	if err = os.WriteFile(filepath.Join(f.root, "select", "doc"), []byte("uncaptured protected edit"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = c.Mutate(ctx, t08Mutation(freshReview, "select", currentEnv.ID, "4"))
	t08Code(t, err, "STALE_VIEW")
	data, _ = os.ReadFile(filepath.Join(f.root, "select", "doc"))
	if string(data) != "uncaptured protected edit" {
		t.Fatal("working edit changed")
	}
}
func TestTerminalT08RestoreProvenanceCopyAndCollision(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("restore")
	old := t08Capture(t, f, folder, "restore", "doc", "old")
	current := t08Capture(t, f, folder, "restore", "doc", "current")
	c := terminalClient(t, f)
	preview := t08Review(t, c, folder, "doc", &old.ID, "recovered")
	m := t08Mutation(preview, "separate_copy", old.ID, "5")
	m.Content.Destination = "recovered"
	m.Content.DestinationReview = *preview.DestinationReview
	r, err := c.Mutate(ctx, m)
	if err != nil || r.State != "completed" {
		t.Fatalf("copy=%+v %v", r, err)
	}
	b, _ := os.ReadFile(filepath.Join(f.root, "restore", "recovered"))
	if string(b) != "old" {
		t.Fatalf("copy bytes=%q", b)
	}
	b, _ = os.ReadFile(filepath.Join(f.root, "restore", "doc"))
	if string(b) != "current" {
		t.Fatal("copy changed original")
	}
	r2, err := c.Mutate(ctx, m)
	if err != nil || len(r2.Effects) != 1 || *r2.Effects[0].Version != *r.Effects[0].Version {
		t.Fatal("copy replay duplicated")
	}
	_, err = c.Query(ctx, tc.Query{Version: tc.Version, Kind: "content_review", Folder: preview.Context.Folder, Path: "doc", Source: &m.Content.Source, Destination: "recovered"})
	t08Code(t, err, "DESTINATION_COLLISION")
	review := t08Review(t, c, folder, "doc", &old.ID, "")
	r, err = c.Mutate(ctx, t08Mutation(review, "restore", old.ID, "6"))
	if err != nil || r.State != "completed" {
		t.Fatalf("restore=%+v %v", r, err)
	}
	id := *r.Effects[0].Version
	native := history.VersionID{Folder: folder, Counter: uint64(id.Counter)}
	_ = native.Author.UnmarshalText([]byte(id.Author))
	env, err := f.db.Envelope(ctx, native)
	if err != nil || len(env.Parents) != 1 || env.Parents[0] != current.ID || r.Effects[0].Source.Counter != tc.Uint(old.ID.Counter) {
		t.Fatalf("ancestry/provenance=%+v %v", env, err)
	}
}
func TestTerminalT08SessionStreamedMergeAndStaleArrival(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("merge")
	a := t08Capture(t, f, folder, "merge", "doc", strings.Repeat("A", 8<<20))
	t08Remote(t, f, folder, "doc", "remote", 72)
	c := terminalClient(t, f)
	review := t08Review(t, c, folder, "doc", nil, "")
	session, err := c.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "session", OperationID: strings.Repeat("7", 64), Session: &tc.SessionIntent{Action: "create", Context: review.Context, Review: review.Review, Heads: review.Heads, Sources: []tc.VersionID{t08Version(a.ID)}}})
	if err != nil {
		t.Fatal(err)
	}
	// A result larger than metadata bounds streams as bytes rather than JSON.
	data := bytes.Repeat([]byte("merge"), 1200000)
	digest := sha256.Sum256(data)
	u := tc.UploadIntent{OperationID: strings.Repeat("8", 64), Session: session.Session.ID, Bytes: tc.Uint(len(data)), Digest: hex.EncodeToString(digest[:])}
	upload, err := c.Upload(ctx, u, bytes.NewReader(data))
	if err != nil || upload.Upload == nil {
		t.Fatalf("upload=%+v %v", upload, err)
	}
	replay, err := c.Upload(ctx, u, bytes.NewReader(data))
	if err != nil || replay.Upload.ID != upload.Upload.ID {
		t.Fatalf("upload replay %v", err)
	}
	m := t08Mutation(review, "merge", a.ID, "9")
	m.Content.Session = session.Session.ID
	m.Content.Upload = upload.Upload.ID
	m.Content.Bytes = u.Bytes
	m.Content.Digest = u.Digest
	r, err := c.Mutate(ctx, m)
	if err != nil || r.State != "completed" {
		t.Fatalf("merge=%+v %v", r, err)
	}
	disk, _ := os.ReadFile(filepath.Join(f.root, "merge", "doc"))
	if !bytes.Equal(data, disk) {
		t.Fatal("merged bytes differ")
	}
	next := t08Review(t, c, folder, "doc", nil, "")
	session, err = c.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "session", OperationID: strings.Repeat("a", 64), Session: &tc.SessionIntent{Action: "create", Context: next.Context, Review: next.Review, Heads: next.Heads, Sources: next.Heads}})
	if err != nil {
		t.Fatal(err)
	}
	t08Remote(t, f, folder, "doc", "arrived during editor review", 73)
	_, err = c.Upload(ctx, tc.UploadIntent{OperationID: strings.Repeat("b", 64), Session: session.Session.ID, Bytes: u.Bytes, Digest: u.Digest}, bytes.NewReader(data))
	t08Code(t, err, "STALE_VIEW")
	if _, err = os.Stat(session.Session.ResultPath); err != nil {
		t.Fatal("editor recovery result missing")
	}
}
func TestTerminalT08AtomicReplayAfterCommitAndCopyPartial(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("lost")
	a := t08Capture(t, f, folder, "lost", "doc", "local")
	b := t08Remote(t, f, folder, "doc", "remote", 74)
	c := terminalClient(t, f)
	review := t08Review(t, c, folder, "doc", nil, "")
	copyA := t08Review(t, c, folder, "doc", &a.ID, "copy-a")
	copyB := t08Review(t, c, folder, "doc", &b.ID, "copy-b")
	m := t08Mutation(review, "keep_copies", a.ID, "c")
	m.Content.Copies = []tc.CopyPlan{{Source: t08Version(a.ID), Destination: "copy-a", Review: *copyA.DestinationReview}, {Source: t08Version(b.ID), Destination: "copy-b", Review: *copyB.DestinationReview}}
	fault := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
		if name == "terminal.content.copy.committed" {
			return errors.New("response lost after durable copy")
		}
		return nil
	}})
	_, err := fault.TerminalMutate(ctx, m)
	if err == nil {
		t.Fatal("fault not reached")
	}
	op, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "operation", ID: m.OperationID})
	if err != nil || len(op.Effects) != 1 || op.State != "partial" {
		t.Fatalf("partial=%+v %v", op, err)
	}
	first := *op.Effects[0].Version
	r, err := c.Mutate(ctx, m)
	if err != nil || r.State != "completed" || len(r.Effects) != 3 || *r.Effects[0].Version != first {
		t.Fatalf("resume=%+v %v", r, err)
	}
	review = t08Review(t, c, folder, "doc", &b.ID, "")
	m = t08Mutation(review, "restore", b.ID, "d")
	fault = control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
		if name == "terminal.content.committed" {
			return errors.New("response lost after durable resolution")
		}
		return nil
	}})
	_, err = fault.TerminalMutate(ctx, m)
	if err == nil {
		t.Fatal("commit fault not reached")
	}
	op, err = c.Query(ctx, tc.Query{Version: tc.Version, Kind: "operation", ID: m.OperationID})
	if err != nil || len(op.Effects) != 1 {
		t.Fatalf("durable=%+v %v", op, err)
	}
	first = *op.Effects[0].Version
	r, err = c.Mutate(ctx, m)
	if err != nil || *r.Effects[0].Version != first {
		t.Fatalf("lost-response replay=%+v %v", r, err)
	}
	newerReview := t08Review(t, c, folder, "doc", &a.ID, "")
	pending := t08Mutation(newerReview, "restore", a.ID, "3e")
	if _, err = fault.TerminalMutate(ctx, pending); err == nil {
		t.Fatal("pending commit fault not reached")
	}
	t08Remote(t, f, folder, "doc", "later competing head", 82)
	blocked, err := c.Mutate(ctx, pending)
	if err != nil || blocked.Error == nil || blocked.Error.Code != "STALE_VIEW" || len(blocked.Effects) != 1 {
		t.Fatalf("pending stale replay=%+v %v", blocked, err)
	}
	disk, _ := os.ReadFile(filepath.Join(f.root, "lost", "doc"))
	if string(disk) != "remote" {
		t.Fatal("pending replay replaced newer working state")
	}
	heads, err := f.db.Heads(ctx, folder, "doc")
	if err != nil || len(heads) != 2 {
		t.Fatalf("late head/resolution not retained: %v %v", heads, err)
	}

}
func TestTerminalT08CLIHistoryReviewRestoreExportAndTools(t *testing.T) {
	f := fresh(t)
	folder := f.folder("cli")
	old := t08Capture(t, f, folder, "cli", "doc", "old bytes")
	t08Capture(t, f, folder, "cli", "doc", "new bytes")
	t08Capture(t, f, folder, "cli", "large", strings.Repeat("L", 8<<20))
	selected := t08Capture(t, f, folder, "cli", "conflict", "choose local")
	selectedRemote := t08Remote(t, f, folder, "conflict", "choose remote", 80)
	copyA := t08Capture(t, f, folder, "cli", "copies", "copy local")
	copyB := t08Remote(t, f, folder, "copies", "copy remote", 81)
	_ = selected

	_ = terminalClient(t, f)
	binary := buildOrbitBinary(t, f.root)
	f.close()
	daemon := exec.Command(binary, "serve", "--state", f.state, "--control-listen", "127.0.0.1:0", "--no-watch", "--sync-interval", "1s")
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testkit.ValidateDestructiveTarget(f.root, f.state); err != nil {
			t.Fatal(err)
		}
		_ = daemon.Process.Signal(syscall.SIGTERM)
		_ = daemon.Wait()
	}()
	client := &controlclient.Client{StateDir: f.state}
	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		var r tc.Result
		if err := client.Call(context.Background(), "POST", "/control/terminal/v1/query", tc.Query{Version: tc.Version, Kind: "capabilities"}, &r); err == nil {
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatal("daemon control never became ready")
	}

	run := func(args ...string) []byte {
		t.Helper()
		argv := args
		cmd := exec.Command(binary, argv...)
		cmd.Dir = filepath.Join(f.root, "cli")
		out, err := cmd.CombinedOutput()
		if cmd.ProcessState != nil {
			if usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
				t.Logf("CLI %s peak_rss_kib=%d", args[0], usage.Maxrss)
			}
		}

		if err != nil {
			t.Fatalf("CLI %v: %v\n%s", args, err, out)
		}
		return out
	}
	out := run("history", "doc", "--folder", "cli", "--state", f.state, "--json", "--limit", "1")
	var page tc.Result
	if err := json.Unmarshal(out, &page); err != nil || len(page.Versions) != 1 || page.Cursor == "" {
		t.Fatalf("history %s %v", out, err)
	}
	reviewPath := filepath.Join(f.root, "review.json")
	source := hex.EncodeToString(old.ID.Author[:]) + ":1"
	run("restore", "doc", "--folder", "cli", "--state", f.state, "--version", source, "--out", reviewPath, "--json")
	run("restore", "doc", "--folder", "cli", "--state", f.state, "--version", source, "--review-file", reviewPath, "--json")
	b, _ := os.ReadFile(filepath.Join(f.root, "cli", "doc"))
	if string(b) != "old bytes" {
		t.Fatalf("restore bytes %q", b)
	}
	exportPath := filepath.Join(f.root, "exported")
	run("export", "doc", "--folder", "cli", "--state", f.state, "--version", source, "--out", exportPath, "--json")
	b, _ = os.ReadFile(exportPath)
	if string(b) != "old bytes" {
		t.Fatalf("export=%q", b)
	}

	selectReview := filepath.Join(f.root, "select-review.json")
	run("conflicts", "show", "conflict", "--folder", "cli", "--state", f.state, "--out", selectReview, "--json")
	sourceText := func(id history.VersionID) string {
		return hex.EncodeToString(id.Author[:]) + fmt.Sprintf(":%d", id.Counter)
	}
	run("conflicts", "select", "conflict", "--folder", "cli", "--state", f.state, "--selected", sourceText(selectedRemote.ID), "--review-file", selectReview, "--json")
	chosen, _ := os.ReadFile(filepath.Join(f.root, "cli", "conflict"))
	if string(chosen) != "choose remote" {
		t.Fatal("live CLI select bytes differ")
	}
	copiesReview := filepath.Join(f.root, "copies-review.json")
	run("conflicts", "show", "copies", "--folder", "cli", "--state", f.state, "--out", copiesReview, "--json")
	var plans []tc.CopyPlan
	for i, v := range []history.Envelope{copyA, copyB} {
		dest := fmt.Sprintf("saved-copy-%d", i)
		out := run("conflicts", "show", "copies", "--folder", "cli", "--state", f.state, "--version", sourceText(v.ID), "--to", dest, "--json")
		var preview tc.Result
		if err := json.Unmarshal(out, &preview); err != nil {
			t.Fatal(err)
		}
		plans = append(plans, tc.CopyPlan{Source: t08Version(v.ID), Destination: dest, Review: *preview.ContentReview.DestinationReview})
	}
	plansFile := filepath.Join(f.root, "copies-plan.json")
	planBytes, _ := json.Marshal(plans)
	if err := os.WriteFile(plansFile, planBytes, 0600); err != nil {
		t.Fatal(err)
	}
	run("conflicts", "keep-copies", "copies", "--folder", "cli", "--state", f.state, "--selected", sourceText(copyA.ID), "--review-file", copiesReview, "--copies-file", plansFile, "--json")
	for i, expected := range []string{"copy local", "copy remote"} {
		data, _ := os.ReadFile(filepath.Join(f.root, "cli", fmt.Sprintf("saved-copy-%d", i)))
		if string(data) != expected {
			t.Fatal("live CLI kept-copy bytes differ")
		}
	}
	largeReview := filepath.Join(f.root, "large-review.json")
	run("conflicts", "show", "large", "--folder", "cli", "--state", f.state, "--out", largeReview, "--json")
	tool := filepath.Join(f.root, "large editor fixture.py")
	if err := os.WriteFile(tool, []byte("import sys\nwith open(sys.argv[1], 'wb') as f:\n f.write(b'M' * 6000000)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	staged := run("conflicts", "edit", "large", "--folder", "cli", "--state", f.state, "--review-file", largeReview, "--tool", "python3 '"+tool+"'", "--json")
	var uploaded tc.Result
	if err := json.Unmarshal(staged, &uploaded); err != nil || uploaded.Upload == nil {
		t.Fatalf("streamed tool upload=%s %v", staged, err)
	}
	// A committed merge may report durable "pending" (exit 5) when publication
	// does not apply on the first attempt; recovery then completes it.
	merge := exec.Command(binary, "conflicts", "merge", "large", "--folder", "cli", "--state", f.state, "--review-file", largeReview, "--session", uploaded.Upload.Session, "--upload", uploaded.Upload.ID, "--digest", uploaded.Upload.Digest, "--bytes", "6000000", "--json")
	merge.Dir = filepath.Join(f.root, "cli")
	merged, err := merge.Output()
	var mergeResult tc.Result
	if jsonErr := json.Unmarshal(merged, &mergeResult); jsonErr != nil || mergeResult.Operation == nil {
		t.Fatalf("CLI merge: %v %v\n%s", err, jsonErr, merged)
	}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 5 || mergeResult.State != "pending" {
			t.Fatalf("CLI merge: %v\n%s", err, merged)
		}
		w06Wait(t, f.state, mergeResult.Operation.ID)
	}
	large, err := os.Open(filepath.Join(f.root, "cli", "large"))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	n, err := io.Copy(hash, large)
	large.Close()
	if err != nil || n != 6000000 || hex.EncodeToString(hash.Sum(nil)) != uploaded.Upload.Digest {
		t.Fatal("large CLI merge hash/size differs")
	}

	failureReview := filepath.Join(f.root, "failure-review.json")
	run("conflicts", "show", "doc", "--folder", "cli", "--state", f.state, "--out", failureReview, "--json")
	failureID := strings.Repeat("3a", 32)
	failing := exec.Command(binary, "conflicts", "edit", "doc", "--folder", "cli", "--state", f.state, "--review-file", failureReview, "--tool", "/bin/false", "--operation", failureID, "--json")
	failing.Dir = filepath.Join(f.root, "cli")
	if _, err := failing.CombinedOutput(); err == nil {
		t.Fatal("failed configured editor reported success")
	}
	retained, err := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "session", ID: failureID})
	if err != nil || retained.Session == nil {
		t.Fatalf("failed-tool session lost: %v", err)
	}
	if _, err = os.Stat(retained.Session.ResultPath); err != nil {
		t.Fatal("failed-tool result missing")
	}

	crashTool := filepath.Join(f.root, "crashing editor.py")
	if err := os.WriteFile(crashTool, []byte("import os, sys, signal\nwith open(sys.argv[1], 'wb') as f:\n f.write(b'recover this editor output')\n f.flush()\n os.fsync(f.fileno())\nos.kill(os.getpid(), signal.SIGKILL)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := testkit.ValidateDestructiveTarget(f.root, crashTool); err != nil {
		t.Fatal(err)
	}
	crashID := strings.Repeat("3b", 32)
	crashing := exec.Command(binary, "conflicts", "edit", "doc", "--folder", "cli", "--state", f.state, "--review-file", failureReview, "--tool", "python3 '"+crashTool+"'", "--operation", crashID, "--json")
	crashing.Dir = filepath.Join(f.root, "cli")
	if _, err := crashing.CombinedOutput(); err == nil {
		t.Fatal("crashed editor reported success")
	}
	crashed, err := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "session", ID: crashID})
	if err != nil {
		t.Fatal(err)
	}
	candidate, _ := os.ReadFile(crashed.Session.ResultPath)
	if string(candidate) != "recover this editor output" {
		t.Fatal("crashed editor candidate lost")
	}
	unchanged, _ := os.ReadFile(filepath.Join(f.root, "cli", "doc"))
	if string(unchanged) != "old bytes" {
		t.Fatal("failed editor changed captured workspace")
	}
	argv, err := controlclient.ToolArgv(`tool --label 'a b' "$(touch injected)"`)
	if err != nil || len(argv) != 4 || argv[3] != "$(touch injected)" {
		t.Fatalf("argv=%q %v", argv, err)
	}
	if err = controlclient.RunTool(context.Background(), "/bin/false", nil, io.Discard, io.Discard, filepath.Join(f.root, "literal ; file")); err == nil {
		t.Fatal("tool exit ignored")
	}
}

func TestTerminalT08UnavailableRestoreDeletedAndStructuralPages(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("availability")
	old := t08Capture(t, f, folder, "availability", "doc", "old retained bytes")
	t08Capture(t, f, folder, "availability", "doc", "current bytes")
	c := terminalClient(t, f)
	review := t08Review(t, c, folder, "doc", &old.ID, "")
	digest := old.Manifest.Chunks[0].Digest
	hexDigest := hex.EncodeToString(digest[:])
	object := filepath.Join(f.state, "objects", "sha256", hexDigest[:2], hexDigest[2:])
	if err := testkit.ValidateDestructiveTarget(f.root, object); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(object, []byte("bad retained data"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := c.Read(ctx, tc.ReadIntent{Version: t08Version(old.ID)})
	t08Code(t, err, "CONTENT_UNAVAILABLE")
	_, err = c.Mutate(ctx, t08Mutation(review, "restore", old.ID, "e"))
	t08Code(t, err, "CONTENT_UNAVAILABLE")
	current, _ := os.ReadFile(filepath.Join(f.root, "availability", "doc"))
	if string(current) != "current bytes" {
		t.Fatal("corrupt restore changed working bytes")
	}
	// A real local deletion exposes history candidates and their actual state.
	if err = os.Remove(filepath.Join(f.root, "availability", "doc")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.ws.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	deleted, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "deleted", Folder: review.Context.Folder})
	if err != nil || len(deleted.Items) != 1 || len(deleted.Versions) != 1 || deleted.Versions[0].Availability != "ready" {
		t.Fatalf("deleted=%+v %v", deleted, err)
	}
	pending := t08Remote(t, f, folder, "doc", "remote pending", 75)
	// Author a second accepted envelope with a manifest whose chunks are absent.
	unknown := sha256.Sum256([]byte("pending bytes"))
	id := pending.ID
	id.Counter = 2
	env := history.Envelope{ID: id, Path: "doc", Parents: []history.VersionID{pending.ID}, Vector: []history.ClockEntry{{Author: id.Author, Counter: 2}}, Kind: history.KindFile, Manifest: &history.Manifest{Size: 13, Digest: unknown, Chunks: []history.Chunk{{Digest: unknown, Length: 13}}}, AuthoredRevision: pending.AuthoredRevision}
	if err = f.db.ImportMetadata(ctx, env); err != nil {
		t.Fatal(err)
	}
	v := t08Review(t, c, folder, "doc", &id, "")
	_, err = c.Mutate(ctx, t08Mutation(v, "restore", id, "f"))
	t08Code(t, err, "CONTENT_PENDING")
	// Concurrent ancestor-file versus descendant-file remains structural attention.
	t08Remote(t, f, folder, "parent", "ancestor", 76)
	t08Remote(t, f, folder, "parent/child", "child", 77)
	page, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "conflicts", Folder: review.Context.Folder, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	count := 0
	for {
		count += len(page.Attention)
		for _, a := range page.Attention {
			if a.Code == "STRUCTURAL_CONFLICT" && a.Path == "parent" {
				seen = true
			}
		}
		if page.Cursor == "" {
			break
		}
		page, err = c.Query(ctx, tc.Query{Version: tc.Version, Kind: "conflicts", Folder: review.Context.Folder, Limit: 1, Cursor: page.Cursor})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !seen || count < 2 {
		t.Fatalf("structural/conflict pages seen=%t count=%d", seen, count)
	}
}

func TestTerminalT08InterruptedUploadSessionRestartAndToolLimit(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("interrupted")
	source := t08Capture(t, f, folder, "interrupted", "doc", "saved original")
	c := terminalClient(t, f)
	review := t08Review(t, c, folder, "doc", nil, "")
	session, err := c.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "session", OperationID: strings.Repeat("1a", 32), Session: &tc.SessionIntent{Action: "create", Context: review.Context, Review: review.Review, Heads: review.Heads, Sources: []tc.VersionID{t08Version(source.ID)}}})
	if err != nil {
		t.Fatal(err)
	}
	result := session.Session.ResultPath
	if err = os.WriteFile(result, []byte("protected editor candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	badDigest := sha256.Sum256([]byte("full expected upload"))
	_, err = c.Upload(ctx, tc.UploadIntent{OperationID: strings.Repeat("1b", 32), Session: session.Session.ID, Bytes: 20, Digest: hex.EncodeToString(badDigest[:])}, strings.NewReader("interrupted"))
	if err == nil {
		t.Fatal("incomplete upload accepted")
	}
	r, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "session", ID: session.Session.ID})
	if err != nil || r.Session.State != "recovery" {
		t.Fatalf("partial=%+v %v", r, err)
	}
	f.close()
	f.open()
	c = terminalClient(t, f)
	r, err = c.Query(ctx, tc.Query{Version: tc.Version, Kind: "session", ID: session.Session.ID})
	if err != nil || r.Session.State != "recovery" {
		t.Fatalf("restart=%+v %v", r, err)
	}
	attention, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "attention", Folder: review.Context.Folder})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range attention.Attention {
		if a.Code == "EDITOR_RECOVERY" && a.ID == session.Session.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("recovery attention lost")
	}
	data, _ := os.ReadFile(result)
	if string(data) != "protected editor candidate" {
		t.Fatal("result lost")
	}
	// Deterministic real tool paths include spaces and shell metacharacters.
	tool := filepath.Join(f.root, "editor ; fixture.py")
	if err = os.WriteFile(tool, []byte("import sys\nwith open(sys.argv[1], 'wb') as f:\n f.write(b'x' * 10000)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err = controlclient.RunLimitedTool(ctx, "python3 '"+tool+"'", 1024, nil, io.Discard, io.Discard, result)
	if err == nil {
		t.Fatal("tool exceeded admitted result size")
	}
	info, err := os.Stat(result)
	if err != nil || info.Size() > 1024 {
		t.Fatalf("bounded result: %v %v", info, err)
	}
	_, err = c.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "cancel", OperationID: strings.Repeat("1c", 32), Cancel: &tc.CancelIntent{Target: session.Session.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(result); err != nil {
		t.Fatal("cancel removed candidate")
	}
}

func TestTerminalT08RenewExpiryAndReviewedDiscard(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("expiry")
	source := t08Capture(t, f, folder, "expiry", "doc", "source")
	c := terminalClient(t, f)
	review := t08Review(t, c, folder, "doc", nil, "")
	session, err := c.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "session", OperationID: strings.Repeat("2a", 32), Session: &tc.SessionIntent{Action: "create", Context: review.Context, Review: review.Review, Heads: review.Heads, Sources: []tc.VersionID{t08Version(source.ID)}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	clock := now.Add(4 * time.Minute)
	ctrl := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, Now: func() time.Time { return clock }})
	renew := tc.Mutation{Version: tc.Version, Kind: "session", OperationID: strings.Repeat("2b", 32), Session: &tc.SessionIntent{Action: "renew", ID: session.Session.ID, Context: review.Context, Review: review.Review, Heads: review.Heads, Sources: session.Session.Sources}}
	r, err := ctrl.TerminalMutate(ctx, renew)
	if err != nil {
		t.Fatal(err)
	}
	if r.Session.ExpiresAt == session.Session.ExpiresAt {
		t.Fatal("renewal did not extend lease")
	}
	clock = now.Add(6 * time.Minute)
	data := []byte("merged after renewal")
	digest := sha256.Sum256(data)
	_, err = ctrl.TerminalUpload(ctx, tc.UploadIntent{OperationID: strings.Repeat("2c", 32), Session: session.Session.ID, Bytes: tc.Uint(len(data)), Digest: hex.EncodeToString(digest[:])}, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("active renewed review: %v", err)
	}
	clock = now.Add(10 * time.Minute)
	r, err = ctrl.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "session", ID: session.Session.ID})
	if err != nil || r.Session.State != "recovery" {
		t.Fatalf("expiry: %+v %v", r, err)
	}
	fresh, err := ctrl.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "content_review", Folder: review.Context.Folder, Path: "doc"})
	if err != nil {
		t.Fatal(err)
	}
	discard := tc.Mutation{Version: tc.Version, Kind: "session", OperationID: strings.Repeat("2d", 32), Session: &tc.SessionIntent{Action: "discard", ID: session.Session.ID, Context: fresh.ContentReview.Context, Review: fresh.ContentReview.Review, Heads: fresh.ContentReview.Heads, Sources: session.Session.Sources}}
	if err = testkit.ValidateDestructiveTarget(f.root, filepath.Dir(session.Session.ResultPath)); err != nil {
		t.Fatal(err)
	}
	r, err = ctrl.TerminalMutate(ctx, discard)
	if err != nil || r.Session.State != "closed" {
		t.Fatalf("discard: %+v %v", r, err)
	}
	if _, err = os.Stat(session.Session.ResultPath); !os.IsNotExist(err) {
		t.Fatal("reviewed discard retained result")
	}
	r, err = ctrl.TerminalMutate(ctx, discard)
	if err != nil || r.Session.State != "closed" {
		t.Fatal("discard replay failed")
	}
}
