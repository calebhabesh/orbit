package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
)

func TestQueueFairSchedulingAndLargeFileProgressWithAging(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := repository.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var folder history.ID
	folder[0] = 0xAA
	localDev := history.ID{1}
	if err := db.EnsureFolder(ctx, folder, localDev, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active:   []protocol.ActiveMember{{Device: localDev, KeyPin: history.Digest{1}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	queue := scheduler.NewQueue(db, 1024)

	// Enqueue 1 large file task: size = 200 KiB
	largeID, err := queue.Enqueue(ctx, repository.DurableTask{
		Folder:     folder,
		Kind:       "scan",
		TargetPath: "large_archive.zip",
		FileSize:   200 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Enqueue 1 small file task: size = 1 KiB
	smallID1, err := queue.Enqueue(ctx, repository.DurableTask{
		Folder:     folder,
		Kind:       "scan",
		TargetPath: "small_note_1.txt",
		FileSize:   1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	// First scheduling round: small file should be chosen first!
	t1, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if t1.ID != smallID1 {
		t.Fatalf("expected small file task %s first, got %s", smallID1, t1.ID)
	}

	// Mark smallID1 completed
	_ = queue.UpdateState(ctx, smallID1, "completed", 0, "", "", 0)

	// While large file is still waiting, enqueue another small file (1 KiB)
	smallID2, err := queue.Enqueue(ctx, repository.DurableTask{
		Folder:     folder,
		Kind:       "scan",
		TargetPath: "small_note_2.txt",
		FileSize:   1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Notice: in the previous round, large file was skipped, so its AgeCounter incremented to 1!
	// Effective size for large file is now 200 KiB - 64 KiB = 136 KiB. Still larger than smallID2 (1 KiB).
	t2, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if t2.ID != smallID2 {
		t.Fatalf("expected small file 2 first, got %s", t2.ID)
	}
	_ = queue.UpdateState(ctx, smallID2, "completed", 0, "", "", 0)

	// Now enqueue small file 3 (1 KiB)
	smallID3, err := queue.Enqueue(ctx, repository.DurableTask{
		Folder:     folder,
		Kind:       "scan",
		TargetPath: "small_note_3.txt",
		FileSize:   1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Large file was skipped again! AgeCounter is now 2!
	// Effective size: 200 KiB - 128 KiB = 72 KiB.
	t3, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if t3.ID != smallID3 {
		t.Fatalf("expected small file 3, got %s", t3.ID)
	}
	_ = queue.UpdateState(ctx, smallID3, "completed", 0, "", "", 0)

	// Enqueue small file 4 (1 KiB)
	smallID4, err := queue.Enqueue(ctx, repository.DurableTask{
		Folder:     folder,
		Kind:       "scan",
		TargetPath: "small_note_4.txt",
		FileSize:   1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Large file was skipped again! AgeCounter is now 3!
	// Effective size: 200 KiB - 192 KiB = 8 KiB. Still slightly above 1 KiB.
	t4, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if t4.ID != smallID4 {
		t.Fatalf("expected small file 4, got %s", t4.ID)
	}
	_ = queue.UpdateState(ctx, smallID4, "completed", 0, "", "", 0)

	// Enqueue small file 5 (1 KiB)
	_, err = queue.Enqueue(ctx, repository.DurableTask{
		Folder:     folder,
		Kind:       "scan",
		TargetPath: "small_note_5.txt",
		FileSize:   1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Large file was skipped 4 times! AgeCounter is now 4!
	// Effective score: 200 KiB - 256 KiB = -56 KiB!
	// Small file 5 has score 1 KiB.
	// Since -56 KiB < 1 KiB, the large file WINS and is scheduled! (Invariant I13 proven!)
	t5, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if t5.ID != largeID {
		t.Fatalf("expected large file %s to advance due to aging, got %s", largeID, t5.ID)
	}
}
