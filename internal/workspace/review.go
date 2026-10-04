package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/calebhabesh/file-sync/internal/history"
	"golang.org/x/sys/unix"
)

// ReviewPath observes descriptor-rooted bytes without capture or publication.
// Its fingerprint binds both contents and the named inode to a later review.
func (w *Workspace) ReviewPath(ctx context.Context, folder history.ID, path string) (string, bool, error) {
	if err := history.ValidatePath(path); err != nil {
		return "", false, err
	}
	ctx, release := w.enterIO(ctx)
	defer release()
	root, err := w.openRootRegistered(ctx, folder)
	if err != nil {
		return "", false, err
	}
	defer root.close()
	fd, err := safeOpen(root.fd, path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return "absent", true, nil
	}
	if err != nil {
		return "", false, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var before, after, named unix.Stat_t
	if err = unix.Fstat(fd, &before); err != nil {
		return "", false, err
	}
	kind := before.Mode & unix.S_IFMT
	if kind != unix.S_IFREG && kind != unix.S_IFDIR {
		return "", false, ErrUnsupportedEntry
	}
	hash := sha256.New()
	if kind == unix.S_IFREG {
		if before.Nlink != 1 {
			return "", false, ErrUnsupportedEntry
		}
		buf := make([]byte, 64<<10)
		for {
			if err = ctx.Err(); err != nil {
				return "", false, err
			}
			n, e := f.Read(buf)
			hash.Write(buf[:n])
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return "", false, e
			}
		}
	}
	if err = unix.Fstat(fd, &after); err != nil {
		return "", false, err
	}
	if err = statPath(root.fd, path, &named); err != nil || !sameObservation(before, after) || !sameObservation(after, named) {
		return "", false, ErrUnstableFile
	}
	digest := history.Digest{}
	copy(digest[:], hash.Sum(nil))
	// Include metadata so a replacement with equal bytes still invalidates review.
	stamp := sha256.Sum256([]byte(hex.EncodeToString(digest[:]) + fmt.Sprintf("%d:%d:%d:%d:%d:%d", named.Dev, named.Ino, named.Size, named.Mode, named.Mtim.Nano(), named.Ctim.Nano())))
	p, e := w.repo.Projection(ctx, folder, path)
	captured := e == nil && ((kind == unix.S_IFREG && p.Kind == history.KindFile && p.Digest == digest && p.Executable == (named.Mode&0111 != 0)) || (kind == unix.S_IFDIR && p.Kind == history.KindDirectory))
	return hex.EncodeToString(stamp[:]), captured, nil
}
