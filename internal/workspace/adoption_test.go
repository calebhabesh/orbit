package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Trial finding: a typed root whose parents did not exist failed setup with a
// bare "no such file or directory". Missing parents are now created too.
func TestAdoptionCreatesMissingParents(t *testing.T) {
	ctx := context.Background()
	work, _, _, _ := testWorkspace(t)
	base := t.TempDir()
	root := filepath.Join(base, "a", "b", "Demo")
	walk, err := work.BeginAdoption(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !walk.Missing || !walk.Preview.Missing || len(walk.Create) != 3 {
		t.Fatalf("walk=%+v", walk)
	}
	if err := work.AdoptionSlice(ctx, &walk); err != nil {
		t.Fatal(err)
	}
	if err := work.CreateAdoptionRoot(walk); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("root not created: %v", err)
	}
	// A parent appearing after the review invalidates it.
	other := filepath.Join(base, "x", "Demo")
	walk, err = work.BeginAdoption(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := work.AdoptionSlice(ctx, &walk); err != ErrRootUnavailable {
		t.Fatalf("changed parent accepted: %v", err)
	}
}
