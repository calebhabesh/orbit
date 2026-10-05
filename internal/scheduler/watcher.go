package scheduler

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/calebhabesh/file-sync/internal/history"
	"golang.org/x/sys/unix"
)

type WatchEvent struct {
	Folder   history.ID `json:"folder"`
	Path     string     `json:"path,omitempty"`
	Overflow bool       `json:"overflow"`
}

type watchEntry struct {
	folder  history.ID
	dirPath string
	relDir  string
}

type Watcher struct {
	mu             sync.Mutex
	inotifyFd      int
	epollFd        int
	pipeR          int
	pipeW          int
	debounceWindow time.Duration
	events         chan WatchEvent
	closed         bool
	wg             sync.WaitGroup

	// folder -> root path
	folderRoots map[history.ID]string
	// wd -> watchEntry
	watches map[int]watchEntry
	// path -> wd
	pathToWd map[string]int

	// debouncing
	pendingDebounce map[string]*time.Timer
}

func NewWatcher(debounceWindow time.Duration) (*Watcher, error) {
	if debounceWindow <= 0 {
		debounceWindow = 200 * time.Millisecond
	}

	ifd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("inotify_init1: %w", err)
	}

	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		unix.Close(ifd)
		return nil, fmt.Errorf("epoll_create1: %w", err)
	}

	var pipeFds [2]int
	if err := unix.Pipe2(pipeFds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		unix.Close(ifd)
		unix.Close(epfd)
		return nil, fmt.Errorf("pipe2: %w", err)
	}

	// Add inotifyFd to epoll
	inEvent := unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(ifd)}
	if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, ifd, &inEvent); err != nil {
		unix.Close(ifd)
		unix.Close(epfd)
		unix.Close(pipeFds[0])
		unix.Close(pipeFds[1])
		return nil, fmt.Errorf("epoll_ctl inotify: %w", err)
	}

	// Add pipeR to epoll for wake/shutdown
	pipeEvent := unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(pipeFds[0])}
	if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, pipeFds[0], &pipeEvent); err != nil {
		unix.Close(ifd)
		unix.Close(epfd)
		unix.Close(pipeFds[0])
		unix.Close(pipeFds[1])
		return nil, fmt.Errorf("epoll_ctl pipe: %w", err)
	}

	w := &Watcher{
		inotifyFd:       ifd,
		epollFd:         epfd,
		pipeR:           pipeFds[0],
		pipeW:           pipeFds[1],
		debounceWindow:  debounceWindow,
		events:          make(chan WatchEvent, 1024),
		folderRoots:     make(map[history.ID]string),
		watches:         make(map[int]watchEntry),
		pathToWd:        make(map[string]int),
		pendingDebounce: make(map[string]*time.Timer),
	}

	w.wg.Add(1)
	go w.readLoop()

	return w, nil
}

func (w *Watcher) Events() <-chan WatchEvent {
	return w.events
}

func (w *Watcher) WatchFolder(folder history.ID, rootPath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return errors.New("watcher is closed")
	}

	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}
	if info, err := os.Stat(absRoot); err != nil || !info.IsDir() {
		return fmt.Errorf("root directory unavailable: %w", err)
	}

	w.folderRoots[folder] = absRoot

	// Recursively add watches for all existing directories
	return filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable components safely
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".filesync-internal" {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil || rel == "." {
			rel = ""
		}
		return w.addWatchLocked(folder, path, rel)
	})
}

func (w *Watcher) addWatchLocked(folder history.ID, dirPath, relDir string) error {
	if _, exists := w.pathToWd[dirPath]; exists {
		return nil
	}
	mask := uint32(unix.IN_CREATE | unix.IN_DELETE | unix.IN_MODIFY |
		unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_ATTRIB |
		unix.IN_DELETE_SELF | unix.IN_MOVE_SELF | unix.IN_ONLYDIR)
	wd, err := unix.InotifyAddWatch(w.inotifyFd, dirPath, mask)
	if err != nil {
		return err
	}
	w.watches[wd] = watchEntry{
		folder:  folder,
		dirPath: dirPath,
		relDir:  relDir,
	}
	w.pathToWd[dirPath] = wd
	return nil
}

func (w *Watcher) UnwatchFolder(folder history.ID) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.folderRoots, folder)
	for wd, entry := range w.watches {
		if entry.folder == folder {
			unix.InotifyRmWatch(w.inotifyFd, uint32(wd))
			delete(w.pathToWd, entry.dirPath)
			delete(w.watches, wd)
		}
	}
	return nil
}

func (w *Watcher) IsWatching(folder history.ID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	root, ok := w.folderRoots[folder]
	if !ok {
		return false
	}
	_, watching := w.pathToWd[root]
	return watching
}

func (w *Watcher) readLoop() {
	defer w.wg.Done()
	epollEvents := make([]unix.EpollEvent, 16)
	buf := make([]byte, 4096)

	for {
		n, err := unix.EpollWait(w.epollFd, epollEvents, -1)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return
		}

		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()

		for i := 0; i < n; i++ {
			fd := int(epollEvents[i].Fd)
			if fd == w.pipeR {
				// Wake pipe received signal -> shutdown
				return
			}
			if fd == w.inotifyFd {
				w.processInotifyEvents(buf)
			}
		}
	}
}

func (w *Watcher) processInotifyEvents(buf []byte) {
	for {
		bytesRead, err := unix.Read(w.inotifyFd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				break
			}
			return
		}
		if bytesRead <= 0 {
			break
		}

		offset := 0
		for offset <= bytesRead-unix.SizeofInotifyEvent {
			raw := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
			mask := raw.Mask
			wd := int(raw.Wd)
			nameLen := int(raw.Len)

			offset += unix.SizeofInotifyEvent
			var name string
			if nameLen > 0 && offset+nameLen <= bytesRead {
				rawName := buf[offset : offset+nameLen]
				if idx := bytes.IndexByte(rawName, 0); idx != -1 {
					rawName = rawName[:idx]
				}
				name = string(rawName)
				offset += nameLen
			}

			// Handle IN_Q_OVERFLOW: system queue overflowed, events were lost!
			if mask&unix.IN_Q_OVERFLOW != 0 {
				w.emitOverflowAll()
				continue
			}

			w.mu.Lock()
			entry, ok := w.watches[wd]
			w.mu.Unlock()
			if !ok {
				continue
			}

			// Skip internal scratch files
			if name == ".filesync-internal" || strings.HasPrefix(name, ".filesync-internal/") {
				continue
			}

			relPath := name
			if entry.relDir != "" {
				if name != "" {
					relPath = filepath.Join(entry.relDir, name)
				} else {
					relPath = entry.relDir
				}
			}

			// If directory created or moved in, add watch recursively
			if mask&unix.IN_ISDIR != 0 && (mask&unix.IN_CREATE != 0 || mask&unix.IN_MOVED_TO != 0) {
				fullSubDir := filepath.Join(entry.dirPath, name)
				w.mu.Lock()
				_ = w.addWatchLocked(entry.folder, fullSubDir, relPath)
				w.mu.Unlock()
			}

			// Clean up watches on delete
			if mask&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_IGNORED) != 0 {
				w.mu.Lock()
				delete(w.pathToWd, entry.dirPath)
				delete(w.watches, wd)
				w.mu.Unlock()
			}

			w.debounceEvent(entry.folder, relPath)
		}
	}
}

func (w *Watcher) debounceEvent(folder history.ID, path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	key := fmt.Sprintf("%x:%s", folder, path)
	if timer, ok := w.pendingDebounce[key]; ok {
		timer.Stop()
	}

	w.pendingDebounce[key] = time.AfterFunc(w.debounceWindow, func() {
		w.mu.Lock()
		delete(w.pendingDebounce, key)
		if w.closed {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()

		evt := WatchEvent{Folder: folder, Path: path}
		select {
		case w.events <- evt:
		default:
			// Queue full -> flag overflow
			w.emitOverflow(folder)
		}
	})
}

func (w *Watcher) emitOverflow(folder history.ID) {
	evt := WatchEvent{Folder: folder, Overflow: true}
	select {
	case w.events <- evt:
	default:
		// already congested, drop without blocking
	}
}

func (w *Watcher) emitOverflowAll() {
	w.mu.Lock()
	folders := make([]history.ID, 0, len(w.folderRoots))
	for f := range w.folderRoots {
		folders = append(folders, f)
	}
	w.mu.Unlock()

	for _, f := range folders {
		w.emitOverflow(f)
	}
}

func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true

	// Stop pending debounce timers
	for _, timer := range w.pendingDebounce {
		timer.Stop()
	}
	w.pendingDebounce = nil

	// Wake epoll loop
	var dummy [1]byte
	_, _ = unix.Write(w.pipeW, dummy[:])
	w.mu.Unlock()

	w.wg.Wait()

	w.mu.Lock()
	defer w.mu.Unlock()

	for wd := range w.watches {
		unix.InotifyRmWatch(w.inotifyFd, uint32(wd))
	}
	w.watches = nil
	w.pathToWd = nil

	unix.Close(w.inotifyFd)
	unix.Close(w.epollFd)
	unix.Close(w.pipeR)
	unix.Close(w.pipeW)

	close(w.events)
	return nil
}
