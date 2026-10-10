package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// JoiningRecheckDelay spaces checks of a task deferred behind a folder's join.
const JoiningRecheckDelay = time.Second

type ClientFactory func(folder, peer history.ID) (replication.PeerClient, error)

type PeerTarget struct{ Folder, Peer history.ID }

type SchedulerOptions struct {
	TransferHook  func(string) error // deterministic marked-fixture boundary injection
	PeerTargets   func() ([]PeerTarget, error)
	Profile       ResourceProfile
	Limiter       *BandwidthLimiter
	NoWatch       bool
	ClientFactory ClientFactory
	LocalDevice   history.ID
	Peers         []PeerTarget
	// Joining names folders whose working tree an unfinished join publishes.
	// Their scans and syncs wait, so the join remains the only publisher.
	Joining func(context.Context) (map[history.ID]bool, error)
}

type FolderStatus struct {
	Folder         history.ID `json:"folder"`
	Paused         bool       `json:"paused"`
	PauseReason    string     `json:"pause_reason,omitempty"`
	QueuedTasks    int        `json:"queued_tasks"`
	RunningTasks   int        `json:"running_tasks"`
	RetryTasks     int        `json:"retry_tasks"`
	ExhaustedTasks int        `json:"exhausted_tasks"`
}

type Scheduler struct {
	transferHook  func(string) error
	db            *repository.DB
	ws            *workspace.Workspace
	clientFactory ClientFactory
	localDevice   history.ID
	peers         []PeerTarget
	peerTargets   func() ([]PeerTarget, error)
	joining       func(context.Context) (map[history.ID]bool, error)
	folderWork    map[history.ID]chan struct{}
	profile       ResourceProfile
	limiter       *BandwidthLimiter
	noWatch       bool
	watcher       *Watcher
	watchedRoots  map[history.ID]string
	queue         *Queue
	classifier    RetryClassifier
	hashSem       chan struct{}
	transferSem   chan struct{}
	workSignal    chan struct{}

	mu                 sync.Mutex
	pausedFolders      map[history.ID]string
	runningTaskCancels map[string]context.CancelFunc

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	stopped bool
}

func NewScheduler(db *repository.DB, ws *workspace.Workspace, opts SchedulerOptions) (*Scheduler, error) {
	if err := ValidateProfile(opts.Profile); err != nil {
		opts.Profile = GetProfile(ProfileLaptop)
	}

	queue := NewQueue(db, opts.Profile.MaxQueuedTasks)

	var watcher *Watcher
	if !opts.NoWatch {
		w, err := NewWatcher(opts.Profile.DebounceWindow)
		if err != nil {
			// If inotify fails (e.g. unsupported OS or container limits), fallback to no-watch mode gracefully
			opts.NoWatch = true
		} else {
			watcher = w
		}
	}

	return &Scheduler{
		transferHook:       opts.TransferHook,
		db:                 db,
		ws:                 ws,
		clientFactory:      opts.ClientFactory,
		localDevice:        opts.LocalDevice,
		peers:              append([]PeerTarget(nil), opts.Peers...),
		peerTargets:        opts.PeerTargets,
		joining:            opts.Joining,
		folderWork:         map[history.ID]chan struct{}{},
		profile:            opts.Profile,
		limiter:            opts.Limiter,
		noWatch:            opts.NoWatch,
		watcher:            watcher,
		watchedRoots:       map[history.ID]string{},
		queue:              queue,
		hashSem:            make(chan struct{}, opts.Profile.HashWorkers),
		transferSem:        make(chan struct{}, opts.Profile.TransferWorkers),
		workSignal:         make(chan struct{}, 1),
		pausedFolders:      make(map[history.ID]string),
		runningTaskCancels: make(map[string]context.CancelFunc),
	}, nil
}

func (s *Scheduler) notifyWork() {
	select {
	case s.workSignal <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Start(parentCtx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ctx, s.cancel = context.WithCancel(parentCtx)

	// 1. Recover in-flight tasks from SQLite
	if _, err := s.db.RecoverInFlightDurableTasks(s.ctx); err != nil {
		return fmt.Errorf("recover in-flight tasks: %w", err)
	}

	// 2. Load pending tasks into queue
	if err := s.queue.LoadFromDB(s.ctx); err != nil {
		return fmt.Errorf("load work queue: %w", err)
	}

	// 3. Inspect registered folders
	folders, err := s.db.RegisteredFolders(s.ctx)
	if err != nil {
		return fmt.Errorf("list registered folders: %w", err)
	}

	for _, reg := range folders {
		// Recheck root
		if s.watcher != nil {
			if err := s.watcher.WatchFolder(reg.Folder, reg.Path); err != nil {
				s.pausedFolders[reg.Folder] = "ROOT_UNAVAILABLE"
				continue
			}
		}
		s.watchedRoots[reg.Folder] = reg.Path
		// Enqueue initial quick reconciliation scan
		_, _ = s.queue.Enqueue(s.ctx, repository.DurableTask{
			Folder: reg.Folder,
			Kind:   "scan",
		})
	}

	// 4. Start watcher event consumer
	if s.watcher != nil {
		s.wg.Add(1)
		go s.watchLoop()
	}

	// 5. Start dual-scan cadence loop
	s.wg.Add(1)
	go s.cadenceLoop()

	// 6. Start dispatcher loop
	s.wg.Add(1)
	go s.dispatchLoop()

	s.notifyWork()
	s.enqueuePeerSyncs()
	return nil
}

func (s *Scheduler) enqueuePeerSyncs() {
	targets := s.peers
	if s.peerTargets != nil {
		if current, err := s.peerTargets(); err == nil {
			targets = current
		}
	}
	for _, target := range targets {
		peer := target.Peer
		_, _ = s.queue.Enqueue(s.ctx, repository.DurableTask{Folder: target.Folder, Peer: &peer, Kind: "sync"})
	}
	s.notifyWork()
}

func (s *Scheduler) watchLoop() {
	defer s.wg.Done()
	events := s.watcher.Events()

	for {
		select {
		case <-s.ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			// Enqueue scan task
			s.mu.Lock()
			_, paused := s.pausedFolders[evt.Folder]
			s.mu.Unlock()
			if paused {
				continue
			}

			fullScan := evt.Overflow
			_, _ = s.queue.Enqueue(s.ctx, repository.DurableTask{
				Folder:     evt.Folder,
				Kind:       "scan",
				TargetPath: evt.Path,
				// If overflow occurred, scan whole folder
				AgeCounter: 0,
			})
			_ = fullScan
			s.notifyWork()
		}
	}
}

func (s *Scheduler) cadenceLoop() {
	defer s.wg.Done()

	reconcileTicker := time.NewTicker(s.profile.ReconcileInterval)
	defer reconcileTicker.Stop()

	fullScanTicker := time.NewTicker(s.profile.FullScanInterval)
	defer fullScanTicker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-reconcileTicker.C:
			s.enqueuePeerSyncs()
			// Enqueue periodic reconciliation scans for all active folders
			folders, err := s.db.RegisteredFolders(s.ctx)
			if err != nil {
				continue
			}
			for _, reg := range folders {
				s.mu.Lock()
				_, paused := s.pausedFolders[reg.Folder]
				s.mu.Unlock()
				if !paused {
					_, _ = s.queue.Enqueue(s.ctx, repository.DurableTask{
						Folder: reg.Folder,
						Kind:   "scan",
					})
				}
			}
			s.notifyWork()
		case <-fullScanTicker.C:
			// Enqueue periodic full-content verification scans
			folders, err := s.db.RegisteredFolders(s.ctx)
			if err != nil {
				continue
			}
			for _, reg := range folders {
				s.mu.Lock()
				_, paused := s.pausedFolders[reg.Folder]
				s.mu.Unlock()
				if !paused {
					_, _ = s.queue.Enqueue(s.ctx, repository.DurableTask{
						Folder: reg.Folder,
						Kind:   "scan",
					})
				}
			}
			// Periodic bounded pruning of finished lifecycle records (Invariant I28)
			_, _ = s.db.PruneLifecycleRecords(s.ctx, time.Now().Add(-24*time.Hour))
			s.notifyWork()
		}
	}
}

func (s *Scheduler) dispatchLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.workSignal:
		case <-time.After(500 * time.Millisecond):
		}

		s.refreshRootWatches()
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			s.mu.Lock()
			pausedCopy := make(map[history.ID]string)
			for k, v := range s.pausedFolders {
				pausedCopy[k] = v
			}
			s.mu.Unlock()

			task, err := s.queue.NextReadyTask(s.ctx, pausedCopy, time.Now())
			if err != nil || task == nil {
				break
			}

			// Launch task execution
			s.wg.Add(1)
			go s.executeTask(task)
		}
	}
}

func (s *Scheduler) executeTask(task *repository.DurableTask) {
	defer s.wg.Done()
	defer s.notifyWork()
	// Work queued before this device left the folder is dropped, not failed.
	if left, err := s.db.FolderLeft(s.ctx, task.Folder); err == nil && left {
		_ = s.queue.Cancel(context.Background(), task.ID)
		return
	}

	taskCtx, taskCancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	gate := s.folderWork[task.Folder]
	if gate == nil {
		gate = make(chan struct{}, 1)
		s.folderWork[task.Folder] = gate
	}
	s.runningTaskCancels[task.ID] = taskCancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.runningTaskCancels, task.ID)
		s.mu.Unlock()
		taskCancel()
	}()
	// Scanning and publication share one working tree. Serialize scheduled work
	// per folder, while unrelated folders retain worker concurrency.
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-taskCtx.Done():
		_ = s.queue.UpdateState(context.Background(), task.ID, "queued", task.Attempts, "", "", 0)
		return
	}
	if left, err := s.db.FolderLeft(taskCtx, task.Folder); err != nil || left {
		_ = s.queue.Cancel(context.Background(), task.ID)
		return
	}
	if task.Kind == "sync" && task.Peer != nil {
		if retired, err := s.db.IsDeviceRetired(taskCtx, task.Folder, *task.Peer); err != nil || retired {
			_ = s.queue.Cancel(context.Background(), task.ID)
			return
		}
	}
	// Checked under the folder gate: a join that started after dispatch still
	// keeps this task off its working tree. The attempt is not charged.
	if s.joining != nil {
		if joining, err := s.joining(taskCtx); err == nil && joining[task.Folder] {
			_ = s.queue.UpdateState(context.Background(), task.ID, "retry", task.Attempts, "", "", time.Now().Add(JoiningRecheckDelay).UnixNano())
			return
		}
	}

	var execErr error
	switch task.Kind {
	case "scan":
		// Acquire hash worker permit
		select {
		case s.hashSem <- struct{}{}:
			defer func() { <-s.hashSem }()
		case <-taskCtx.Done():
			_ = s.queue.UpdateState(context.Background(), task.ID, "queued", task.Attempts, "", "", 0)
			return
		}

		fullContent := (task.Attempts > 0) // if retried or explicit
		scanOpts := workspace.ScanOptions{FullContent: fullContent}
		if task.TargetPath != "" {
			scanOpts.Paths = []string{task.TargetPath}
		}
		_, execErr = s.ws.ScanWithOptions(taskCtx, task.Folder, scanOpts)

	case "sync":
		// Acquire transfer worker permit
		select {
		case s.transferSem <- struct{}{}:
			defer func() { <-s.transferSem }()
		case <-taskCtx.Done():
			_ = s.queue.UpdateState(context.Background(), task.ID, "queued", task.Attempts, "", "", 0)
			return
		}

		if s.clientFactory == nil || task.Peer == nil {
			execErr = errors.New("client factory or peer not configured")
		} else {
			client, err := s.clientFactory(task.Folder, *task.Peer)
			if err == nil {
				if closer, ok := client.(interface{ CloseIdleConnections() }); ok {
					defer closer.CloseIdleConnections()
				}
			}
			if err != nil {
				execErr = err
			} else {
				membership, mErr := s.db.Membership(taskCtx, task.Folder)
				if mErr != nil {
					execErr = mErr
				} else {
					local := s.localDevice
					if local == (history.ID{}) {
						if reg, err := s.db.Folders(taskCtx); err == nil {
							for _, f := range reg {
								if f.Folder == task.Folder {
									local = f.LocalAuthor
									break
								}
							}
						}
					}
					options := replication.TransferOptions{Workers: s.profile.TransferWorkers, Hook: s.transferHook}
					if s.limiter != nil {
						options.Limiter = s.limiter
					}
					reg, rootErr := s.db.Root(taskCtx, task.Folder)
					if rootErr != nil || !reg.BootstrapComplete {
						execErr = workspace.ErrRootUnavailable
					} else {
						syncer := replication.NewSyncer(s.db, s.ws, client, local, *task.Peer, task.Folder, membership, options)
						_, execErr = syncer.Sync(taskCtx)
					}
				}
			}
		}
	default:
		execErr = fmt.Errorf("unknown task kind: %s", task.Kind)
	}
	// Membership can change while the task runs. A retired peer's old
	// response cannot revive its work or end our own participation.
	if task.Kind == "sync" && task.Peer != nil {
		if retired, err := s.db.IsDeviceRetired(context.Background(), task.Folder, *task.Peer); err == nil && retired {
			_ = s.queue.Cancel(context.Background(), task.ID)
			return
		}
	}

	if execErr == nil {
		_ = s.queue.UpdateState(context.Background(), task.ID, "completed", task.Attempts, "", "", 0)
		if task.Kind == "sync" && task.Peer != nil {
			_ = s.db.ResolveExhaustedSyncTasks(context.Background(), task.Folder, *task.Peer)
		}
		// A completed full scan covers every earlier scan of the folder, so a
		// scan exhausted on a passing condition (a briefly missing root, F02)
		// stops being attention without the owner retrying it.
		if task.Kind == "scan" && task.TargetPath == "" {
			_, _ = s.db.SupersedeExhaustedScans(context.Background(), task.Folder, task.ID, task.CreatedNS)
		}
		return
	}

	// A pinned peer's explicit removal refusal ends activity for this Orbit.
	var ending *replication.WireError
	if task.Peer != nil && errors.As(execErr, &ending) && ending.Body.Code == replication.DeviceRemovedCode {
		_ = replication.RecordDeviceRemoval(context.Background(), s.db, task.Folder, *task.Peer, execErr)
		_ = s.queue.Cancel(context.Background(), task.ID)
		return
	}
	if left, _ := s.db.FolderLeft(context.Background(), task.Folder); left {
		_ = s.queue.Cancel(context.Background(), task.ID)
		return
	}
	// Error handling
	if errors.Is(execErr, context.Canceled) {
		// Was canceled or stopped
		_ = s.queue.UpdateState(context.Background(), task.ID, "queued", task.Attempts, "", "", 0)
		return
	}

	if errors.Is(execErr, workspace.ErrRootUnavailable) {
		// Pause relevant folder operations
		s.PauseFolder(task.Folder, "ROOT_UNAVAILABLE")
		_ = s.queue.UpdateState(context.Background(), task.ID, "exhausted", task.Attempts+1, execErr.Error(), "ROOT_UNAVAILABLE", 0)
		return
	}

	isTransient := s.classifier.IsTransient(execErr)
	errorCode := s.classifier.ErrorCode(execErr)
	attempts := task.Attempts + 1

	if isTransient && attempts < task.MaxAttempts {
		delay := s.classifier.BackoffFor(execErr, attempts)
		retryAfterNS := time.Now().Add(delay).UnixNano()
		_ = s.queue.UpdateState(context.Background(), task.ID, "retry", attempts, execErr.Error(), errorCode, retryAfterNS)
	} else {
		// Exhausted
		_ = s.queue.UpdateState(context.Background(), task.ID, "exhausted", attempts, execErr.Error(), errorCode, 0)
	}
}

func (s *Scheduler) Submit(task repository.DurableTask) (string, error) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return "", errors.New("scheduler is stopped")
	}
	s.mu.Unlock()

	id, err := s.queue.Enqueue(s.ctx, task)
	if err == nil {
		s.notifyWork()
	}
	return id, err
}

func (s *Scheduler) Cancel(taskID string) error {
	s.mu.Lock()
	cancel, running := s.runningTaskCancels[taskID]
	s.mu.Unlock()

	if running && cancel != nil {
		cancel()
	}
	return s.queue.Cancel(context.Background(), taskID)
}

func (s *Scheduler) Retry(taskID string) error {
	err := s.queue.Retry(context.Background(), taskID)
	if err == nil {
		s.notifyWork()
	}
	return err
}

func (s *Scheduler) RetryAll(folder *history.ID) (int, error) {
	count, err := s.db.RetryAllExhaustedTasks(context.Background(), folder)
	if err == nil && count > 0 {
		_ = s.queue.LoadFromDB(context.Background())
		s.notifyWork()
	}
	return count, err
}

// ReloadWork picks up tasks re-queued in the database by another component,
// such as a control retry while the daemon runs (F03).
func (s *Scheduler) ReloadWork() {
	if err := s.queue.LoadFromDB(context.Background()); err == nil {
		s.notifyWork()
	}
}

func (s *Scheduler) PauseFolder(folder history.ID, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reason == "" {
		reason = "PAUSED"
	}
	s.pausedFolders[folder] = reason
}

func (s *Scheduler) ResumeFolder(folder history.ID) {
	s.mu.Lock()
	delete(s.pausedFolders, folder)
	s.mu.Unlock()
	s.notifyWork()
}

func (s *Scheduler) Status(folder history.ID) (FolderStatus, error) {
	s.mu.Lock()
	reason, paused := s.pausedFolders[folder]
	s.mu.Unlock()

	tasks, err := s.db.ListDurableTasks(context.Background(), repository.TaskFilter{Folder: folder})
	if err != nil {
		return FolderStatus{}, err
	}

	st := FolderStatus{
		Folder:      folder,
		Paused:      paused,
		PauseReason: reason,
	}
	for _, t := range tasks {
		switch t.State {
		case "queued":
			st.QueuedTasks++
		case "running":
			st.RunningTasks++
		case "retry":
			st.RetryTasks++
		case "exhausted":
			st.ExhaustedTasks++
		}
	}
	return st, nil
}

func (s *Scheduler) ListTasks(filter repository.TaskFilter) ([]repository.DurableTask, error) {
	return s.db.ListDurableTasks(context.Background(), filter)
}

func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	if s.cancel != nil {
		s.cancel()
	}
	// Cancel all running tasks immediately
	for _, cancel := range s.runningTaskCancels {
		if cancel != nil {
			cancel()
		}
	}
	s.mu.Unlock()

	if s.watcher != nil {
		_ = s.watcher.Close()
	}

	s.wg.Wait()

	// Ensure any tasks left running in DB are safely reset to queued
	_, _ = s.db.RecoverInFlightDurableTasks(context.Background())
	return nil
}

// A location change preserves relative paths but inotify must watch the new tree.
// Only the dispatch loop owns watchedRoots after Start.
func (s *Scheduler) refreshRootWatches() {
	regs, err := s.db.RegisteredFolders(s.ctx)
	if err != nil {
		return
	}
	active := map[history.ID]bool{}
	for _, reg := range regs {
		active[reg.Folder] = true

		s.mu.Lock()
		pausedReason := s.pausedFolders[reg.Folder]
		s.mu.Unlock()

		if pausedReason == "ROOT_UNAVAILABLE" {
			if s.ws != nil && s.ws.Revalidate(s.ctx, reg.Folder) == nil {
				s.mu.Lock()
				delete(s.pausedFolders, reg.Folder)
				s.mu.Unlock()
				pausedReason = ""
			}
		}

		if reg.Paused || pausedReason != "" {
			continue
		}

		if s.watcher != nil {
			if s.watchedRoots[reg.Folder] == reg.Path && s.watcher.IsWatching(reg.Folder) {
				continue
			}
			_ = s.watcher.UnwatchFolder(reg.Folder)
			if err := s.watcher.WatchFolder(reg.Folder, reg.Path); err == nil {
				s.watchedRoots[reg.Folder] = reg.Path
				_, _ = s.queue.Enqueue(s.ctx, repository.DurableTask{Folder: reg.Folder, Kind: "scan"})
			}
		}
	}

	if s.watcher != nil {
		for folder := range s.watchedRoots {
			if !active[folder] {
				_ = s.watcher.UnwatchFolder(folder)
				delete(s.watchedRoots, folder)
			}
		}
	}
}
