package scheduler

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
)

const MaxReadySkips = 8 // force progress independently of file size

const AgeBonusBytes = 64 * 1024 // 64 KiB effective size reduction per age tick

type Queue struct {
	mu          sync.Mutex
	db          *repository.DB
	maxCapacity int
	tasks       map[string]*repository.DurableTask
	folderOrder []history.ID
	folderIndex int
}

func NewQueue(db *repository.DB, maxCapacity int) *Queue {
	if maxCapacity <= 0 || maxCapacity > 4096 {
		maxCapacity = 1024
	}
	return &Queue{
		db:          db,
		maxCapacity: maxCapacity,
		tasks:       make(map[string]*repository.DurableTask),
	}
}

func (q *Queue) LoadFromDB(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	tasks, err := q.db.ListDurableTasks(ctx, repository.TaskFilter{State: "active", Limit: q.maxCapacity})
	if err != nil {
		return err
	}
	q.tasks = make(map[string]*repository.DurableTask)
	for i := range tasks {
		t := tasks[i]
		if t.State == "queued" || t.State == "running" || t.State == "retry" {
			q.tasks[t.ID] = &t
			q.ensureFolderTrackingLocked(t.Folder)
		}
	}
	return nil
}

func (q *Queue) ensureFolderTrackingLocked(folder history.ID) {
	for _, f := range q.folderOrder {
		if f == folder {
			return
		}
	}
	q.folderOrder = append(q.folderOrder, folder)
}

func (q *Queue) Count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, t := range q.tasks {
		if t.State == "queued" || t.State == "running" || t.State == "retry" {
			count++
		}
	}
	return count
}

func (q *Queue) Enqueue(ctx context.Context, task repository.DurableTask) (string, error) {
	// First enqueue in SQLite (which checks capacity and database coalescing)
	taskID, err := q.db.EnqueueDurableTask(ctx, task)
	if err != nil {
		return "", err
	}
	stored, err := q.db.GetDurableTask(ctx, taskID)
	if err != nil {
		return "", err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.tasks[taskID]; !exists && (stored.State == "queued" || stored.State == "running" || stored.State == "retry") {
		q.tasks[taskID] = &stored
	}
	q.ensureFolderTrackingLocked(task.Folder)
	return taskID, nil
}

func (q *Queue) UpdateState(ctx context.Context, taskID string, state string, attempts int, lastError, errorCode string, retryAfterNS int64) error {
	if len(lastError) > 2048 {
		lastError = strings.ToValidUTF8(lastError[:2048], "?")
	}
	if err := q.db.UpdateDurableTaskState(ctx, taskID, state, attempts, lastError, errorCode, retryAfterNS); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if t, ok := q.tasks[taskID]; ok {
		t.State = state
		t.Attempts = attempts
		t.LastError = lastError
		t.ErrorCode = errorCode
		t.RetryAfterNS = retryAfterNS
		t.UpdatedNS = time.Now().UnixNano()
		if state == "completed" || state == "canceled" || state == "exhausted" {
			delete(q.tasks, taskID)
		}
	}
	return nil
}

// NextReadyTask selects the next task according to round-robin folder scheduling
// and small-file preference with aging within the chosen folder.
func (q *Queue) NextReadyTask(ctx context.Context, pausedFolders map[history.ID]string, now time.Time) (*repository.DurableTask, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.folderOrder) == 0 {
		return nil, nil
	}

	nowNS := now.UnixNano()
	numFolders := len(q.folderOrder)

	// Round-robin iteration over folders
	for i := 0; i < numFolders; i++ {
		folderIdx := (q.folderIndex + i) % numFolders
		folder := q.folderOrder[folderIdx]

		if reason, paused := pausedFolders[folder]; paused && reason != "" {
			continue
		}

		// Find ready tasks in this folder
		var ready []*repository.DurableTask
		for _, t := range q.tasks {
			if t.Folder != folder {
				continue
			}
			if t.State == "queued" || (t.State == "retry" && nowNS >= t.RetryAfterNS) {
				ready = append(ready, t)
			}
		}

		if len(ready) == 0 {
			continue
		}

		// Advance folder index for next round
		q.folderIndex = (folderIdx + 1) % numFolders

		// Sort ready tasks by effective priority score:
		// score = FileSize - (AgeCounter * AgeBonusBytes)
		// Smallest score wins (small files first, with aging boost for older tasks)
		sort.Slice(ready, func(a, b int) bool {
			// After eight ready skips, prioritize age over size. This also
			// avoids relying on a huge file's size-derived aging horizon.
			agedA, agedB := ready[a].AgeCounter >= MaxReadySkips, ready[b].AgeCounter >= MaxReadySkips
			if agedA != agedB {
				return agedA
			}
			if agedA && ready[a].AgeCounter != ready[b].AgeCounter {
				return ready[a].AgeCounter > ready[b].AgeCounter
			}
			scoreA := int64(ready[a].FileSize) - int64(ready[a].AgeCounter)*AgeBonusBytes
			scoreB := int64(ready[b].FileSize) - int64(ready[b].AgeCounter)*AgeBonusBytes
			if scoreA != scoreB {
				return scoreA < scoreB
			}
			return ready[a].CreatedNS < ready[b].CreatedNS
		})

		selected := ready[0]

		// Increment age of all skipped ready tasks in this folder
		var skippedIDs []string
		for _, skipped := range ready[1:] {
			skipped.AgeCounter++
			skippedIDs = append(skippedIDs, skipped.ID)
		}
		if len(skippedIDs) > 0 {
			_ = q.db.IncrementDurableTaskAge(ctx, skippedIDs)
		}

		selected.AgeCounter = 0
		selected.State = "running"
		selected.UpdatedNS = nowNS
		_ = q.db.UpdateDurableTaskState(ctx, selected.ID, "running", selected.Attempts, selected.LastError, selected.ErrorCode, selected.RetryAfterNS)

		return selected, nil
	}

	return nil, nil
}

func (q *Queue) Cancel(ctx context.Context, taskID string) error {
	if err := q.db.CancelDurableTask(ctx, taskID); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.tasks, taskID)
	return nil
}

func (q *Queue) Retry(ctx context.Context, taskID string) error {
	if err := q.db.RetryDurableTask(ctx, taskID); err != nil {
		return err
	}
	task, err := q.db.GetDurableTask(ctx, taskID)
	if err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks[taskID] = &task
	q.ensureFolderTrackingLocked(task.Folder)
	return nil
}
