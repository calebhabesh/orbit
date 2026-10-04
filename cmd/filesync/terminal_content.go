package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/launcher"
	"golang.org/x/sys/unix"
)

func contentOperationID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func readContentReview(path string) (tc.ContentReview, error) {
	var r tc.Result
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return tc.ContentReview{}, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) || stat.Size > tc.MaxMetadata {
		return tc.ContentReview{}, errors.New("INVALID_REQUEST: owner-only bounded review file required")
	}
	b, err := io.ReadAll(io.LimitReader(f, tc.MaxMetadata+1))
	if err != nil {
		return tc.ContentReview{}, err
	}
	if err = tc.Decode(b, &r); err != nil {
		return tc.ContentReview{}, err
	}
	if r.ContentReview == nil {
		return tc.ContentReview{}, errors.New("INVALID_REQUEST: content review required")
	}
	return *r.ContentReview, nil
}

// handleTerminalContent provides preview-first named recovery commands. Commit
// always requires the exact private review produced by a separate invocation.
func handleTerminalContent(action string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateFlag := flags.String("state", "", "agent state directory")
	folderFlag := flags.String("folder", "", "folder name or ID")
	sourceFlag := flags.String("version", "", "exact source author:counter")
	flags.StringVar(sourceFlag, "source", "", "alias for --version")
	previewFlag := flags.Bool("preview", false, "preview without committing")
	selectedFlag := flags.String("selected", "", "exact source author:counter")
	reviewFlag := flags.String("review-file", "", "private content review JSON to commit")
	outFlag := flags.String("out", "", "private review output or exact-version export destination")
	destFlag := flags.String("to", "", "separate copy destination (synced root relative)")
	sessionFlag := flags.String("session", "", "durable editor session ID")
	uploadFlag := flags.String("upload", "", "immutable uploaded content ID")
	operationFlag := flags.String("operation", "", "stable replay operation ID")
	fileFlag := flags.String("file", "", "stream this file as editor result")
	toolFlag := flags.String("tool", "", "direct editor/diff argv with shell-style quoting")
	copiesFlag := flags.String("copies-file", "", "private JSON array of exact source/destination/review copy plans")
	digestFlag := flags.String("digest", "", "reviewed upload SHA256")
	bytesFlag := flags.Uint64("bytes", 0, "reviewed upload size")
	limitFlag := flags.Uint("limit", 50, "bounded page size")
	cursorFlag := flags.String("cursor", "", "page cursor")
	jsonFlag := flags.Bool("json", false, "structured JSON")
	path, flagArgs := parsePositionalFolder(args, "state", "folder", "version", "selected", "review-file", "out", "to", "session", "upload", "operation", "file", "tool", "digest", "bytes", "limit", "cursor", "copies-file", "source")
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	if *previewFlag {
		*reviewFlag = ""
	}
	dir, err := launcher.DiscoverState(*stateFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	fail := func(err error) error { return &CLIExitError{Code: RenderError(stderr, err, *jsonFlag)} }
	emit := func(r tc.Result) error {
		if *outFlag != "" && action != "export" {
			b, err := json.Marshal(r)
			if err != nil {
				return err
			}
			if err = writeContentReviewFile(ctx, client, *outFlag, b); err != nil {
				return err
			}
		}
		if *jsonFlag {
			return json.NewEncoder(stdout).Encode(r)
		}
		for _, v := range r.Versions {
			fmt.Fprintf(stdout, "%s  %s:%d kind=%s bytes=%d state=%s digest=%s\n", EscapeTerminal(v.Path), v.Version.Author, v.Version.Counter, v.Kind, v.Bytes, v.Availability, v.Digest)
		}
		for _, att := range r.Attention {
			fmt.Fprintf(stdout, "%s %s: %s\n", EscapeTerminal(att.Path), att.Code, EscapeTerminal(att.Action))
		}
		for _, v := range r.Items {
			fmt.Fprintln(stdout, EscapeTerminal(v.Name))
		}
		if r.ContentReview != nil {
			v := r.ContentReview
			fmt.Fprintf(stdout, "Review %s: %d heads; working bytes captured=%t; replacement=%s\n", v.Review.Token, len(v.Heads), v.WorkingCaptured, EscapeTerminal(strings.Join(v.ReplacementPaths, ", ")))
			if *reviewFlag == "" {
				fmt.Fprintln(stdout, "Commit requires --review-file from this preview; unavailable content must be recovered first.")
			}
		}
		if r.Session != nil {
			fmt.Fprintf(stdout, "Session %s state=%s result=%s expires=%s\n", r.Session.ID, r.Session.State, EscapeTerminal(r.Session.ResultPath), r.Session.ExpiresAt)
		}
		if r.Upload != nil {
			fmt.Fprintf(stdout, "Upload %s bytes=%d digest=%s (staged; requires merge commit preview)\n", r.Upload.ID, r.Upload.Bytes, r.Upload.Digest)
		}
		if r.Operation != nil {
			fmt.Fprintf(stdout, "Operation %s state=%s phase=%s\n", r.Operation.ID, r.Operation.State, r.Operation.Phase)
		}
		if r.Cursor != "" {
			fmt.Fprintf(stdout, "Next cursor: %s\n", r.Cursor)
		}
		return nil
	}
	if action == "cancel" {
		r, err := client.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "cancel", OperationID: contentOperationID(), Cancel: &tc.CancelIntent{Target: *sessionFlag}})
		if err != nil {
			return fail(err)
		}
		return emit(r)
	}
	if action == "session" {
		r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "session", ID: *sessionFlag})
		if err != nil {
			return fail(err)
		}
		return emit(r)
	}
	resolved, err := ResolveContext(ctx, client, ContextOptions{StateDir: dir, Folder: *folderFlag, Path: path, AllowEmptyPath: action == "deleted" || action == "conflicts"})
	if err != nil {
		return fail(err)
	}
	if resolved.Result.Error != nil {
		return &CLIExitError{Code: RenderResult(stdout, stderr, resolved.Result, *jsonFlag)}
	}
	sourceText := *sourceFlag
	if sourceText == "" {
		sourceText = *selectedFlag
	}
	var source tc.VersionID
	if sourceText != "" {
		folder, err := parseID(resolved.Folder)
		if err != nil {
			return fail(err)
		}
		id, err := parseVersionID(folder, sourceText)
		if err != nil {
			return fail(err)
		}
		source = tc.VersionID{Folder: resolved.Folder, Author: hex.EncodeToString(id.Author[:]), Counter: tc.Uint(id.Counter)}
	}
	if action == "export" {
		if *outFlag == "" || sourceText == "" {
			return fail(errors.New("INVALID_REQUEST: export requires --version and --out"))
		}
		// External recovery must not write into any registered synced root.
		absolute, err := filepath.Abs(*outFlag)
		if err != nil {
			return fail(err)
		}
		folders, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "folders"})
		if err != nil {
			return fail(err)
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
		if err != nil {
			return fail(err)
		}
		absolute = filepath.Join(parent, filepath.Base(absolute))
		for _, f := range folders.Items {
			rel, e := filepath.Rel(f.Root, absolute)
			if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return fail(errors.New("INVALID_REQUEST: use reviewed separate-copy recovery inside synced roots"))
			}
		}
		read, err := client.Read(ctx, tc.ReadIntent{Version: source})
		if err != nil {
			return fail(err)
		}
		defer read.Close()
		// Publish only a fully verified stream, never an incomplete destination.
		temp, err := os.CreateTemp(parent, ".orbit-export-*")
		if err != nil {
			return fail(err)
		}
		defer os.Remove(temp.Name())
		defer temp.Close()
		_, err = io.CopyBuffer(temp, read, make([]byte, 64<<10))
		if err == nil {
			err = temp.Sync()
		}
		if err != nil {
			return fail(err)
		}
		if err = os.Link(temp.Name(), absolute); err != nil {
			return fail(err)
		}
		d, err := os.Open(parent)
		if err != nil {
			return fail(err)
		}
		err = d.Sync()
		d.Close()
		if err != nil {
			return fail(err)
		}
		if *jsonFlag {
			return json.NewEncoder(stdout).Encode(map[string]string{"state": "completed", "destination": absolute})
		}
		fmt.Fprintf(stdout, "Exported exact version to %s\n", EscapeTerminal(absolute))
		return nil
	}
	if action == "history" || action == "deleted" || action == "conflicts" {
		r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: action, Folder: resolved.Folder, Path: resolved.Path, Limit: tc.Uint(*limitFlag), Cursor: *cursorFlag})
		if err != nil {
			return fail(err)
		}
		return emit(r)
	}
	if *reviewFlag == "" {
		q := tc.Query{Version: tc.Version, Kind: "content_review", Folder: resolved.Folder, Path: resolved.Path, Destination: *destFlag}
		if sourceText != "" {
			q.Source = &source
		}
		r, err := client.Query(ctx, q)
		if err != nil {
			return fail(err)
		}
		return emit(r)
	}
	reviewed, err := readContentReview(*reviewFlag)
	if err != nil {
		return fail(err)
	}
	if reviewed.Context.Folder != resolved.Folder || reviewed.Context.Path != resolved.Path {
		return fail(errors.New("STALE_VIEW: review belongs to another target"))
	}
	op := *operationFlag
	if op == "" {
		op = contentOperationID()
	}
	if action == "discard" || action == "renew" {
		session, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "session", ID: *sessionFlag})
		if err != nil {
			return fail(err)
		}
		r, err := client.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "session", OperationID: op, Session: &tc.SessionIntent{Action: action, ID: *sessionFlag, Context: reviewed.Context, Review: reviewed.Review, Heads: reviewed.Heads, Sources: session.Session.Sources}})
		if err != nil {
			return fail(err)
		}
		return emit(r)
	}
	if action == "edit" || action == "diff" || action == "upload" {
		if *sessionFlag == "" {
			sources := reviewed.Heads
			if sourceText != "" {
				sources = []tc.VersionID{source}
			}
			// Tombstones/directories have no byte stream; explicitly choose file sources.
			r, err := client.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "session", OperationID: op, Session: &tc.SessionIntent{Action: "create", Context: reviewed.Context, Review: reviewed.Review, Heads: reviewed.Heads, Sources: sources}})
			if err != nil {
				return fail(err)
			}
			*sessionFlag = r.Session.ID
		}
		r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "session", ID: *sessionFlag})
		if err != nil {
			return fail(err)
		}
		if r.Session.State != "active" {
			return emit(r)
		}
		resultPath := r.Session.ResultPath
		if *toolFlag != "" {
			limit := uint64(1 << 20)
			for _, v := range r.Versions {
				if uint64(v.Bytes) > limit {
					limit = uint64(v.Bytes)
				}
			}
			paths := []string{resultPath}
			if action == "diff" {
				paths = nil
				for i := range r.Session.Sources {
					paths = append(paths, filepath.Join(filepath.Dir(resultPath), fmt.Sprintf("source-%02d", i)))
				}
			}
			toolOutput := stdout
			if *jsonFlag {
				toolOutput = stderr
			}
			if err = controlclient.RunLimitedTool(ctx, *toolFlag, limit, os.Stdin, toolOutput, stderr, paths...); err != nil {
				_ = emit(r)
				return fail(err)
			}
		}
		if action == "diff" {
			return emit(r)
		}
		input := *fileFlag
		if input == "" {
			input = resultPath
		}
		fd, err := unix.Open(input, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		var file *os.File
		if err == nil {
			file = os.NewFile(uintptr(fd), input)
		}
		if err != nil {
			return fail(err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return fail(errors.New("INVALID_REQUEST: regular merge file required"))
		}
		hash := sha256.New()
		n, err := io.CopyBuffer(hash, file, make([]byte, 64<<10))
		if err != nil {
			return fail(err)
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return fail(err)
		}
		upload, err := client.Upload(ctx, tc.UploadIntent{OperationID: contentOperationID(), Session: *sessionFlag, Bytes: tc.Uint(n), Digest: hex.EncodeToString(hash.Sum(nil))}, file)
		if err != nil {
			return fail(err)
		}
		return emit(upload)
	}
	content := tc.ContentIntent{Context: reviewed.Context, Review: reviewed.Review, Heads: reviewed.Heads, Action: action, Source: source, Session: *sessionFlag, Upload: *uploadFlag, Digest: *digestFlag, Bytes: tc.Uint(*bytesFlag)}
	if action == "keep_copies" {
		if *copiesFlag == "" {
			return fail(errors.New("INVALID_REQUEST: keep-copies requires --copies-file"))
		}
		file, err := os.Open(*copiesFlag)
		if err != nil {
			return fail(err)
		}
		defer file.Close()
		b, err := io.ReadAll(io.LimitReader(file, tc.MaxMetadata+1))
		if err != nil {
			return fail(err)
		}
		if len(b) > tc.MaxMetadata {
			return fail(errors.New("PAYLOAD_TOO_LARGE"))
		}
		if err = tc.Decode(b, &content.Copies); err != nil {
			return fail(err)
		}
	}
	if action == "show" {
		return fail(errors.New("INVALID_REQUEST: show is a preview; omit --review-file"))
	}
	if action == "restore" && *destFlag != "" {
		content.Action = "separate_copy"
		content.Destination = *destFlag
		if reviewed.DestinationReview != nil {
			content.DestinationReview = *reviewed.DestinationReview
		}
	}
	r, err := client.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "content", OperationID: op, Content: &content})
	if err != nil {
		return fail(err)
	}
	if err = emit(r); err != nil {
		return err
	}
	if code := tc.ExitCode(r); code != 0 {
		return &CLIExitError{Code: code}
	}
	return nil
}

// Review output never overwrites a file or becomes an ordinary synced edit.
func writeContentReviewFile(ctx context.Context, client *controlclient.Client, path string, data []byte) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return err
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	folders, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "folders"})
	if err != nil {
		return err
	}
	for _, f := range folders.Items {
		rel, e := filepath.Rel(f.Root, absolute)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return &control.ControlError{Code: "INVALID_PATH", Message: "save review metadata outside synced folders", Action: "choose a private external review file"}
		}
	}
	file, err := os.CreateTemp(parent, ".orbit-review-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = os.Link(file.Name(), absolute); err != nil {
		return &control.ControlError{Code: "DESTINATION_COLLISION", Message: "review destination exists or cannot be installed", Action: "choose a new private review filename", Err: err}
	}
	if err = os.Remove(file.Name()); err != nil {
		return err
	}
	dir, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
