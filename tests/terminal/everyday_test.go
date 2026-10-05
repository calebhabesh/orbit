package terminal_test

import (
	"context"
	"encoding/hex"
	"fmt"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTerminalT11BoundedSearchStorageAndLiveStopped(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("everyday")
	for i := 0; i < 43; i++ {
		if err := os.WriteFile(filepath.Join(f.root, "everyday", fmt.Sprintf("doc-%02d", i)), []byte("safe bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.ws.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	client := terminalClient(t, f)
	id := hex.EncodeToString(folder[:])
	q := tc.Query{Version: tc.Version, Kind: "paths", Folder: id, Name: "doc", Limit: 7}
	seen := map[string]bool{}
	for {
		r, err := client.Query(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Items) > 7 || len(r.Items) == 0 {
			t.Fatal("bounds")
		}
		for _, it := range r.Items {
			if seen[it.ID] {
				t.Fatal("duplicate")
			}
			seen[it.ID] = true
		}
		if r.Cursor == "" {
			break
		}
		q.Cursor = r.Cursor
	}
	if len(seen) != 43 {
		t.Fatal("omitted paths")
	}
	before, err := f.db.DetailedStorageUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "maintenance", Folder: id})
	if err != nil || r.Storage == nil {
		t.Fatal(err)
	}
	if uint64(r.Storage.Objects) != before.ObjectBytes || uint64(r.Storage.MetadataBudget) != before.MetadataBudgetBytes {
		t.Fatal("usage mismatch")
	}
	after, _ := f.db.DetailedStorageUsage(ctx)
	if after.ObjectBytes != before.ObjectBytes {
		t.Fatal("preview cleaned payload")
	}
	f.close()
	r, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "paths", Folder: id, Name: "doc", Limit: 7})
	if err != nil || len(r.Items) != 7 {
		t.Fatal("stopped parity", err)
	}
}
func TestTerminalT11SessionResultStreamAndSymlinkRefusal(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("editor")
	a := t08Capture(t, f, folder, "editor", "doc", "local")
	t08Remote(t, f, folder, "doc", "remote", 91)
	c := terminalClient(t, f)
	review := t08Review(t, c, folder, "doc", nil, "")
	r, err := c.Mutate(ctx, tc.Mutation{Version: tc.Version, OperationID: fmt.Sprintf("%064x", 51), Kind: "session", Session: &tc.SessionIntent{Action: "create", Context: review.Context, Review: review.Review, Heads: review.Heads, Sources: []tc.VersionID{t08Version(a.ID)}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(r.Session.ResultPath, []byte("merged exact"), 0600); err != nil {
		t.Fatal(err)
	}
	upload, err := c.UploadSessionResult(ctx, *r.Session, fmt.Sprintf("%064x", 52), 1<<20)
	if err != nil || upload.Upload == nil || upload.Upload.Bytes != 12 {
		t.Fatal("stream upload", err)
	}
	m := tc.Mutation{Version: tc.Version, OperationID: fmt.Sprintf("%064x", 53), Kind: "content", Content: &tc.ContentIntent{Context: review.Context, Review: review.Review, Heads: review.Heads, Action: "merge", Session: r.Session.ID, Upload: upload.Upload.ID, Digest: upload.Upload.Digest, Bytes: upload.Upload.Bytes}}
	result, err := c.Mutate(ctx, m)
	if err != nil || result.Operation.State != "completed" {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "editor", "doc"))
	if string(data) != "merged exact" {
		t.Fatal("not published")
	}
	heads, _ := f.db.Heads(ctx, folder, "doc")
	if len(heads) != 1 {
		t.Fatal("not exact resolution")
	}
	if err = os.Remove(r.Session.ResultPath); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(f.root, "editor", "doc"), r.Session.ResultPath); err != nil {
		t.Fatal(err)
	}
	if _, err = c.UploadSessionResult(ctx, *r.Session, fmt.Sprintf("%064x", 54), 1<<20); err == nil {
		t.Fatal("symlink result admitted")
	}
	_, err = f.db.ContentAvailability(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
}
func TestTerminalT11RealPTYEveryday(t *testing.T) {
	root := testkit.NewDisposable(t)
	binary := filepath.Join(root, "filesync")
	build := exec.Command("go", "build", "-o", binary, "./cmd/filesync")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %v %s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "../../scripts/terminal_everyday_pty_test.py", "--binary", binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("everyday PTY %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
