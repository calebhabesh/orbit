package controlclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

// UploadSessionResult hashes and streams the same private admitted file descriptor.
// A changed file is refused by the controller's digest/size staging verification.
func (c *Client) UploadSessionResult(ctx context.Context, session tc.EditorSession, operation string, limit uint64) (tc.Result, error) {
	fd, err := unix.Open(session.ResultPath, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return tc.Result{}, err
	}
	f := os.NewFile(uintptr(fd), session.ResultPath)
	defer f.Close()
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return tc.Result{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || st.Uid != uint32(os.Geteuid()) || st.Size < 0 || uint64(st.Size) > limit {
		return tc.Result{}, errors.New("INVALID_REQUEST: private bounded editor result required")
	}
	h := sha256.New()
	n, err := io.CopyBuffer(h, io.LimitReader(f, int64(limit)+1), make([]byte, 64<<10))
	if err != nil {
		return tc.Result{}, err
	}
	if uint64(n) > limit {
		return tc.Result{}, errors.New("STORAGE_BLOCKED: editor result limit")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return tc.Result{}, err
	}
	return c.Upload(ctx, tc.UploadIntent{OperationID: operation, Session: session.ID, Bytes: tc.Uint(n), Digest: hex.EncodeToString(h.Sum(nil))}, f)
}
