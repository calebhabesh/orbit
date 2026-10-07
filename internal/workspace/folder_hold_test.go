package workspace

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A join's syncer and the scheduler's syncer may publish the same version.
// Recovery treats every journaled publication as interrupted, so the second
// publisher must wait rather than recover the first one's live publication.
func TestConcurrentPublishersOfOneFolderSerialize(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	remote := remoteFile(t, db, folder, "large", []byte("remote bytes"))
	paused, resume := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	work.hook = func(name string) error {
		if name == HookBeforeExchange && first.CompareAndSwap(false, true) {
			close(paused)
			<-resume
		}
		return nil
	}
	firstDone := make(chan error, 1)
	go func() { firstDone <- work.Apply(ctx, remote) }()
	<-paused
	secondDone := make(chan error, 1)
	go func() { secondDone <- work.Apply(ctx, remote) }()
	select {
	case err := <-secondDone:
		close(resume)
		<-firstDone
		t.Fatalf("second publisher ran beside a live publication: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	// A bounded wait gives up without touching the live publication.
	waiting, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	if err := work.Recover(waiting, folder); err != context.DeadlineExceeded {
		t.Fatalf("bounded recovery beside a live publication: %v", err)
	}
	cancel()
	close(resume)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "large"))
	if err != nil || string(data) != "remote bytes" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	projection, err := db.Projection(ctx, folder, "large")
	if err != nil || projection.BlockReason != "" {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	publications, err := db.Publications(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, publication := range publications {
		if publication.Phase != "COMMITTED" {
			t.Fatalf("uncommitted publication: %+v", publication)
		}
	}
	if err := work.Recover(ctx, folder); err != nil {
		t.Fatal(err)
	}
}

// Nested calls inherit the hold, and other folders are not serialized behind it.
func TestFolderHoldIsReentrantAndPerFolder(t *testing.T) {
	work, _, folder, _ := testWorkspace(t)
	held, release, err := work.enterFolder(context.Background(), folder)
	if err != nil {
		t.Fatal(err)
	}
	nested, cancel := context.WithTimeout(held, time.Second)
	defer cancel()
	if err := work.Recover(nested, folder); err != nil {
		t.Fatalf("nested call did not inherit the hold: %v", err)
	}
	other, otherRelease, err := work.enterFolder(nested, testID('G'))
	if err != nil || other == nil {
		t.Fatalf("other folder waited: %v", err)
	}
	otherRelease()
	release()
	again, cancelAgain := context.WithTimeout(context.Background(), time.Second)
	defer cancelAgain()
	if _, releaseAgain, err := work.enterFolder(again, folder); err != nil {
		t.Fatalf("hold was not released: %v", err)
	} else {
		releaseAgain()
	}
}
