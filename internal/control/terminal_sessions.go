package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"golang.org/x/sys/unix"
)

type contentSessionRecord struct {
	Building    bool             `json:"building"`
	Fingerprint string           `json:"fingerprint"`
	Upload      string           `json:"upload"`
	Session     tc.EditorSession `json:"session"`
	Exports     []string         `json:"exports"`
	ResultLimit uint64           `json:"result_limit"`
	Reservation string           `json:"reservation"`
}
type contentUploadRecord struct {
	Intent tc.UploadIntent `json:"intent"`
	Result tc.UploadResult `json:"result"`
	Path   string          `json:"path"`
	State  string          `json:"state"`
}

func (c *Controller) sessionDirectory(id string) string {
	return filepath.Join(c.db.StateDir(), "operations", "editor-"+id)
}
func (c *Controller) loadContentSession(ctx context.Context, id string, active bool) (contentSessionRecord, error) {
	var rec contentSessionRecord
	if len(id) != 64 {
		return rec, terminalError("INVALID_REQUEST")
	}
	if err := c.db.TerminalRecord(ctx, "session/"+id, &rec); err != nil {
		return rec, err
	}
	expiry, err := time.Parse(time.RFC3339Nano, rec.Session.ExpiresAt)
	if err != nil {
		return rec, err
	}
	if rec.Session.State == "active" && !c.options.Now().Before(expiry) {
		rec.Building = false
		rec.Session.State = "recovery"
		if err = c.db.ReleaseContentOwner(ctx, "editor", id); err != nil {
			return rec, err
		}
		if err = c.db.SaveTerminalRecord(ctx, "session/"+id, rec, false); err != nil {
			return rec, err
		}
	}
	if active && rec.Session.State != "active" {
		return rec, terminalError("EXPIRED_REPLAY")
	}
	return rec, nil
}
func (c *Controller) terminalSessionQuery(ctx context.Context, q tc.Query) (tc.Result, error) {
	c.contentMu.Lock()
	defer c.contentMu.Unlock()
	r := terminalResult()
	rec, err := c.loadContentSession(ctx, q.ID, false)
	if err != nil {
		return r, err
	}
	r.Session = &rec.Session
	for _, v := range rec.Session.Sources {
		source, e := contentVersion(v)
		if e != nil {
			return r, e
		}
		env, e := c.db.Envelope(ctx, source)
		if e != nil {
			return r, e
		}
		state, e := c.db.ContentAvailability(ctx, source)
		if e != nil {
			return r, e
		}
		r.Versions = append(r.Versions, summary(env, string(state)))
	}
	if rec.Session.State != "active" {
		r.State = "partial"
		r.Attention = append(r.Attention, tc.Attention{ID: rec.Session.ID, Folder: rec.Session.Context.Folder, Path: rec.Session.Context.Path, Code: "EDITOR_RECOVERY", Action: "inspect retained editor result; obtain a new review before committing"})
	}
	return r, nil
}
func (c *Controller) terminalSessionMutation(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	c.contentMu.Lock()
	defer c.contentMu.Unlock()
	r := terminalResult()
	fingerprint, err := m.Fingerprint()
	if err != nil {
		return r, err
	}
	var prior repository.TerminalRecord
	if err = c.db.TerminalRecord(ctx, "operation/"+m.OperationID, &prior); err == nil {
		return c.replayTerminal(prior, fingerprint)
	} else if !errors.Is(err, repository.ErrOperationNotFound) {
		return r, err
	}
	p := m.Session

	if p.Action == "create" {
		var pending contentSessionRecord
		if e := c.db.TerminalRecord(ctx, "session/"+m.OperationID, &pending); e == nil {
			if pending.Fingerprint != fingerprint {
				return r, terminalError("IDEMPOTENCY_CONFLICT")
			}
			pending.Building = false
			pending.Session.State = "recovery"
			if e = c.db.SaveTerminalRecord(ctx, "session/"+m.OperationID, pending, false); e != nil {
				return r, e
			}
			_ = c.db.ReleaseContentOwner(ctx, "editor", m.OperationID)
			r.Session = &pending.Session
			r.State = "partial"
			return r, nil
		} else if !errors.Is(e, repository.ErrOperationNotFound) {
			return r, e
		}
	}
	_, folder, _, err := c.verifyContentReview(ctx, tc.ContentIntent{Context: p.Context, Review: p.Review, Heads: p.Heads, Session: p.ID})
	if err != nil {
		return r, err
	}
	if p.Action == "discard" {
		rec, err := c.loadContentSession(ctx, p.ID, false)
		if err != nil {
			return r, err
		}
		if rec.Session.Context.Folder != p.Context.Folder || rec.Session.Context.Path != p.Context.Path || generation(rec.Session.Sources) != generation(p.Sources) {
			return r, terminalError("STALE_VIEW")
		}
		rec.Building = false
		rec.Session.State = "recovery"
		if err = c.db.SaveTerminalRecord(ctx, "session/"+p.ID, rec, false); err != nil {
			return r, err
		}
		if err = c.db.ReleaseContentOwner(ctx, "editor", p.ID); err != nil {
			return r, err
		}
		dir := c.sessionDirectory(p.ID)
		fd, err := unix.Open(dir, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil && !errors.Is(err, unix.ENOENT) {
			return r, err
		}
		if err == nil {
			names := []string{"result"}
			for i := range rec.Session.Sources {
				names = append(names, fmt.Sprintf("source-%02d", i))
			}
			if rec.Upload != "" {
				names = append(names, "upload-"+rec.Upload)
			}
			for _, name := range names {
				if err = unix.Unlinkat(fd, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
					unix.Close(fd)
					return r, err
				}
			}
			err = unix.Fsync(fd)
			unix.Close(fd)
			if err != nil {
				return r, err
			}
			if err = os.Remove(dir); err != nil {
				return r, err
			}
			parent, err := os.Open(filepath.Dir(dir))
			if err != nil {
				return r, err
			}
			err = parent.Sync()
			parent.Close()
			if err != nil {
				return r, err
			}
		}
		rec.Session.State = "closed"
		if err = c.db.SaveTerminalRecord(ctx, "session/"+p.ID, rec, false); err != nil {
			return r, err
		}
		if err = c.db.ReleaseReservation(ctx, rec.Reservation); err != nil {
			return r, err
		}
		r.Session = &rec.Session
	} else if p.Action == "renew" {
		rec, err := c.loadContentSession(ctx, p.ID, true)
		if err != nil {
			return r, err
		}
		if rec.Session.Context != p.Context || rec.Session.Review != p.Review || generation(rec.Session.Sources) != generation(p.Sources) || generation(rec.Session.Heads) != generation(p.Heads) {
			return r, terminalError("STALE_VIEW")
		}
		rec.Session.ExpiresAt = c.options.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)
		if err = c.db.SaveTerminalRecord(ctx, "session/"+p.ID, rec, false); err != nil {
			return r, err
		}
		r.Session = &rec.Session
	} else {
		id := m.OperationID
		rec := contentSessionRecord{Building: true, Fingerprint: fingerprint, Session: tc.EditorSession{ID: id, Context: p.Context, Review: p.Review, Heads: p.Heads, Sources: p.Sources, ResultPath: filepath.Join(c.sessionDirectory(id), "result"), ExpiresAt: c.options.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano), State: "recovery"}, Exports: []string{}, Reservation: "editor-" + id}
		var total uint64
		// Admission inspects bounded manifest metadata; exports reopen one exact
		// source at a time so multiple versions never retain multiple chunk buffers.
		for _, v := range p.Sources {
			source, e := contentVersion(v)
			if e != nil {
				return r, e
			}
			env, e := c.db.Envelope(ctx, source)
			if e != nil {
				return r, e
			}
			if env.Path != p.Context.Path || env.Kind != history.KindFile || env.Manifest == nil {
				return r, terminalError("INVALID_REQUEST")
			}
			if e = c.verifyContentAvailability(ctx, source); e != nil {
				return r, e
			}
			total += env.Manifest.Size
			if env.Manifest.Size > rec.ResultLimit {
				rec.ResultLimit = env.Manifest.Size
			}
		}

		if rec.ResultLimit < 1<<20 {
			rec.ResultLimit = 1 << 20
		}
		// Charge exports, editable result and immutable upload before disk writes.
		if err = c.db.Reserve(ctx, rec.Reservation, total+2*rec.ResultLimit, "reviewed editor exports/result/upload"); err != nil {
			return r, err
		}
		if err = c.db.SaveTerminalRecord(ctx, "session/"+id, rec, true); err != nil {
			c.db.ReleaseReservation(context.Background(), rec.Reservation)
			return r, err
		}
		dir := c.sessionDirectory(id)
		if err = os.Mkdir(dir, 0700); err != nil {
			return r, err
		}
		for i, v := range p.Sources {
			source, _ := contentVersion(v)
			read, e := c.OpenContent(ctx, folder, source)
			if e != nil {
				return r, e
			}
			path := filepath.Join(dir, fmt.Sprintf("source-%02d", i))
			e = func() error {
				defer read.Close()
				file, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if e != nil {
					return e
				}
				defer file.Close()
				if _, e = io.CopyBuffer(file, read, make([]byte, 64<<10)); e != nil {
					return e
				}
				if e = file.Sync(); e != nil {
					return e
				}
				for _, chunk := range read.Envelope.Manifest.Chunks {
					if e = c.db.Pin(ctx, chunk.Digest, "editor", id); e != nil {
						return e
					}
				}
				return file.Chmod(0400)
			}()
			if e != nil {
				return r, e
			}
			rec.Exports = append(rec.Exports, path)
		}

		file, err := os.OpenFile(rec.Session.ResultPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return r, err
		}
		err = file.Sync()
		file.Close()
		if err != nil {
			return r, err
		}
		d, err := os.Open(dir)
		if err != nil {
			return r, err
		}
		err = d.Sync()
		d.Close()
		if err != nil {
			return r, err
		}
		parent, err := os.Open(filepath.Dir(dir))
		if err != nil {
			return r, err
		}
		err = parent.Sync()
		parent.Close()
		if err != nil {
			return r, err
		}
		rec.Building = false
		rec.Session.State = "active"
		if err = c.db.SaveTerminalRecord(ctx, "session/"+id, rec, false); err != nil {
			return r, err
		}
		r.Session = &rec.Session
	}
	r.Operation = &tc.Operation{ID: m.OperationID, Fingerprint: fingerprint, Kind: "session", State: "completed", Phase: "reviewed_exports", CommittedEffects: []tc.Effect{}}
	cfg, e := config.Load(c.db.StateDir())
	if e != nil {
		return r, e
	}
	prior = repository.TerminalRecord{Owner: cfg.DeviceID, Mutation: m, Result: r, CreatedAt: c.options.Now().UTC().Format(time.RFC3339Nano), CompletedAt: c.options.Now().UTC().Format(time.RFC3339Nano)}
	if err = c.db.SaveTerminalRecord(ctx, "operation/"+m.OperationID, prior, true); err != nil {
		return r, err
	}
	return r, nil
}

// TerminalUpload stages immutable verified bytes. Interrupted partial spools
// stay budgeted in operations and cannot be committed as complete content.
func (c *Controller) TerminalUpload(ctx context.Context, intent tc.UploadIntent, source io.Reader) (tc.Result, error) {
	c.contentMu.Lock()
	defer c.contentMu.Unlock()
	r := terminalResult()
	fingerprint, err := intent.Fingerprint()
	if err != nil {
		return r, err
	}
	var upload contentUploadRecord
	err = c.db.TerminalRecord(ctx, "upload/"+intent.OperationID, &upload)
	if err == nil {
		old, _ := upload.Intent.Fingerprint()
		if old != fingerprint {
			return r, terminalError("IDEMPOTENCY_CONFLICT")
		}
		if upload.State != "ready" {
			return r, terminalError("EXPIRED_REPLAY")
		}
		if _, err = c.loadContentSession(ctx, intent.Session, true); err != nil {
			return r, err
		}
		r.Upload = &upload.Result
		return r, nil
	}
	if !errors.Is(err, repository.ErrOperationNotFound) {
		return r, err
	}
	rec, err := c.loadContentSession(ctx, intent.Session, true)
	if err != nil {
		return r, err
	}
	if rec.Upload != "" {
		return r, terminalError("EXPIRED_REPLAY")
	}
	if uint64(intent.Bytes) > rec.ResultLimit {
		return r, terminalError("DISK_BUDGET")
	}
	if _, _, _, err = c.verifyContentReview(ctx, tc.ContentIntent{Context: rec.Session.Context, Review: rec.Session.Review, Heads: rec.Session.Heads, Session: intent.Session}); err != nil {
		return r, err
	}
	rec.Upload = intent.OperationID
	if err = c.db.SaveTerminalRecord(ctx, "session/"+intent.Session, rec, false); err != nil {
		return r, err
	}
	upload = contentUploadRecord{Intent: intent, Path: filepath.Join(c.sessionDirectory(intent.Session), "upload-"+intent.OperationID), State: "partial", Result: tc.UploadResult{ID: intent.OperationID, Session: intent.Session, Bytes: intent.Bytes, Digest: intent.Digest, ExpiresAt: rec.Session.ExpiresAt}}
	if err = c.db.SaveTerminalRecord(ctx, "upload/"+intent.OperationID, upload, true); err != nil {
		return r, err
	}

	staged := false
	defer func() {
		if !staged {
			rec.Building = false
			rec.Session.State = "recovery"
			_ = c.db.SaveTerminalRecord(context.Background(), "session/"+intent.Session, rec, false)
			_ = c.db.ReleaseContentOwner(context.Background(), "editor", intent.Session)
		}
	}()
	f, err := os.OpenFile(upload.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return r, err
	}
	defer f.Close()
	hash := sha256.New()
	n, err := io.CopyBuffer(io.MultiWriter(f, hash), io.LimitReader(&contextReader{ctx, source}, int64(intent.Bytes)+1), make([]byte, 64<<10))
	if syncErr := f.Sync(); err == nil {
		err = syncErr
	}
	if err != nil {
		return r, err
	}
	if n != int64(intent.Bytes) || hex.EncodeToString(hash.Sum(nil)) != intent.Digest {
		return r, readError(repository.ErrContentMismatch)
	}
	if err = f.Chmod(0400); err != nil {
		return r, err
	}
	dir, err := os.Open(filepath.Dir(upload.Path))
	if err != nil {
		return r, err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return r, err
	}
	upload.State = "ready"
	if err = c.db.SaveTerminalRecord(ctx, "upload/"+intent.OperationID, upload, false); err != nil {
		return r, err
	}
	staged = true
	r.Upload = &upload.Result
	return r, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// openUpload refuses tool-created links and rechecks content before authoring.
func (c *Controller) openUpload(ctx context.Context, p tc.ContentIntent) (*os.File, *history.Manifest, error) {
	rec, err := c.loadContentSession(ctx, p.Session, true)
	if err != nil {
		return nil, nil, err
	}
	if rec.Session.Context != p.Context || rec.Session.Review != p.Review || generation(rec.Session.Heads) != generation(p.Heads) {
		return nil, nil, terminalError("STALE_VIEW")
	}
	var upload contentUploadRecord
	if err = c.db.TerminalRecord(ctx, "upload/"+p.Upload, &upload); err != nil {
		return nil, nil, err
	}
	if upload.State != "ready" || upload.Intent.Session != p.Session || upload.Intent.Digest != p.Digest || upload.Intent.Bytes != p.Bytes {
		return nil, nil, terminalError("INVALID_REQUEST")
	}
	fd, err := unix.Open(upload.Path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	f := os.NewFile(uintptr(fd), upload.Path)
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0077 != 0 || stat.Size != int64(p.Bytes) {
		f.Close()
		return nil, nil, terminalError("INVALID_REQUEST")
	}
	// Pin each digest before installation. GC cannot unlink an installed upload
	// chunk between install and the atomic authoring transaction.
	buf := make([]byte, history.ChunkSize)
	whole := sha256.New()
	manifest := &history.Manifest{}
	if len(rec.Session.Sources) > 0 {
		v, e := contentVersion(rec.Session.Sources[0])
		if e != nil {
			f.Close()
			return nil, nil, e
		}
		env, e := c.db.Envelope(ctx, v)
		if e != nil {
			f.Close()
			return nil, nil, e
		}
		if env.Manifest != nil {
			manifest.Executable = env.Manifest.Executable
		}
	}
	for {
		n, e := io.ReadFull(&contextReader{ctx, f}, buf)
		if e != nil && e != io.EOF && e != io.ErrUnexpectedEOF {
			f.Close()
			return nil, nil, e
		}
		if n == 0 {
			break
		}
		digest := sha256.Sum256(buf[:n])
		whole.Write(buf[:n])
		if err = c.db.InstallPinnedChunk(ctx, digest, uint64(n), &byteReader{data: buf[:n]}, "editor", p.Session); err != nil {
			f.Close()
			return nil, nil, err
		}

		manifest.Chunks = append(manifest.Chunks, history.Chunk{Digest: digest, Length: uint64(n)})
		manifest.Size += uint64(n)
		if e == io.ErrUnexpectedEOF {
			break
		}
	}
	copy(manifest.Digest[:], whole.Sum(nil))
	if hex.EncodeToString(manifest.Digest[:]) != p.Digest || manifest.Size != uint64(p.Bytes) {
		f.Close()
		return nil, nil, readError(repository.ErrContentMismatch)
	}
	return f, manifest, nil
}

type byteReader struct{ data []byte }

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// Cancellation closes a session, releases session pins, and preserves every
// editor candidate. It does not cancel a captured publication or delete bytes.
func (c *Controller) terminalSessionCancel(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	c.contentMu.Lock()
	defer c.contentMu.Unlock()
	r := terminalResult()
	fingerprint, err := m.Fingerprint()
	if err != nil {
		return r, err
	}
	var record repository.TerminalRecord
	if err = c.db.TerminalRecord(ctx, "operation/"+m.OperationID, &record); err == nil {
		return c.replayTerminal(record, fingerprint)
	} else if !errors.Is(err, repository.ErrOperationNotFound) {
		return r, err
	}
	rec, err := c.loadContentSession(ctx, m.Cancel.Target, false)
	if err != nil {
		return r, err
	}
	if rec.Session.State != "discarded" {
		rec.Building = false
		rec.Session.State = "recovery"
	}
	if err = c.db.SaveTerminalRecord(ctx, "session/"+m.Cancel.Target, rec, false); err != nil {
		return r, err
	}
	if err = c.db.ReleaseContentOwner(ctx, "editor", m.Cancel.Target); err != nil {
		return r, err
	}
	r.Session = &rec.Session
	r.State = "canceled"
	r.Operation = &tc.Operation{ID: m.OperationID, Kind: "cancel", Fingerprint: fingerprint, State: "canceled", Phase: "editor_result_retained", CommittedEffects: []tc.Effect{}}
	cfg, err := config.Load(c.db.StateDir())
	if err != nil {
		return r, err
	}
	record = repository.TerminalRecord{Owner: cfg.DeviceID, Mutation: m, Result: r, CreatedAt: c.options.Now().UTC().Format(time.RFC3339Nano), CompletedAt: c.options.Now().UTC().Format(time.RFC3339Nano)}
	if err = c.db.SaveTerminalRecord(ctx, "operation/"+m.OperationID, record, true); err != nil {
		return r, err
	}
	return r, nil
}
