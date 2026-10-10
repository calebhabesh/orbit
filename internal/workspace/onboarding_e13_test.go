package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/repository"
)

func TestOnboardingE13ParticipationEndPreservesPendingRecoveryWithoutApplying(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(map[bool]string{false: "left", true: "removed"}[removed], func(t *testing.T) {
			ctx := context.Background()
			w, db, folder, root := testWorkspace(t)
			path := filepath.Join(root, "note")
			if err := os.WriteFile(path, []byte("keep current"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Scan(ctx, folder); err != nil {
				t.Fatal(err)
			}
			remote := remoteFile(t, db, folder, "note", []byte("pending remote"))
			w.hook = func(name string) error {
				if name == HookStageFlushed {
					return syscall.ENOSPC
				}
				return nil
			}
			if err := w.Apply(ctx, remote); !errors.Is(err, syscall.ENOSPC) {
				t.Fatal(err)
			}
			before, err := db.Publications(ctx, folder)
			if err != nil || len(before) == 0 {
				t.Fatal("fault did not retain recovery", before, err)
			}
			if removed {
				err = db.MarkDeviceRemoved(ctx, folder, testID('B'))
			} else {
				err = w.Leave(ctx, folder, time.Now())
			}
			if err != nil {
				t.Fatal(err)
			}
			// A new workspace represents daemon restart; it must not replay an
			// interrupted publication after participation has ended.
			fresh := New(db, Options{})
			if err = fresh.Recover(ctx, folder); !errors.Is(err, repository.ErrFolderLeft) {
				t.Fatal("recovery still active", err)
			}
			if err = fresh.Apply(ctx, remote); !errors.Is(err, repository.ErrFolderLeft) {
				t.Fatal("publication still active", err)
			}
			after, err := db.Publications(ctx, folder)
			if err != nil || len(after) != len(before) {
				t.Fatal("recovery records lost", after, err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "keep current" {
				t.Fatal("working bytes changed", string(data), err)
			}
		})
	}
}
