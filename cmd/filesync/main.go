package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "filesync: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("a command is required")
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "filesync %s (commit=%s, built=%s, %s/%s, go=%s)\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return nil
	case "stop":
		flags := flag.NewFlagSet("stop", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		timeout := flags.Duration("timeout", 10*time.Second, "timeout waiting for agent to stop")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("stop accepts no positional arguments")
		}
		if err := app.StopAgent(*stateDir, *timeout); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "filesync agent in %s stopped successfully\n", *stateDir)
		return nil
	case "config":
		if len(args) < 2 {
			return errors.New("config requires subcommand: validate")
		}
		switch args[1] {
		case "validate":
			return handleConfigValidate(args[2:], stdout, stderr)
		default:
			return fmt.Errorf("unknown config subcommand %q", args[1])
		}
	case "init":
		flags := flag.NewFlagSet("init", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("init accepts no positional arguments")
		}
		cfg, err := app.Initialize(context.Background(), *stateDir, app.SystemDependencies())
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "initialized device %s in %s\n", cfg.DeviceID, *stateDir)
		return nil
	case "serve":
		flags := flag.NewFlagSet("serve", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		peerListen := flags.String("peer-listen", "", "explicit peer listener address, for example 127.0.0.1:8443")
		controlListen := flags.String("control-listen", "", "explicit loopback control listener address, for example 127.0.0.1:8080")
		profile := flags.String("profile", "laptop", "hardware resource profile: laptop or pi")
		bandwidthLimit := flags.Int64("bandwidth-limit", 0, "token-bucket bandwidth limit in bytes/sec (0 = unlimited)")
		syncInterval := flags.Duration("sync-interval", 5*time.Minute, "bounded periodic reconciliation scan interval")
		fullScanInterval := flags.Duration("full-scan-interval", 24*time.Hour, "bounded full-content verification scan interval")
		noWatch := flags.Bool("no-watch", false, "disable filesystem inotify watcher hints")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("serve accepts no positional arguments")
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return app.ServeWithOptions(ctx, *stateDir, app.ServeOptions{
			PeerAddress:       *peerListen,
			ControlAddress:    *controlListen,
			Ready:             stdout,
			Profile:           *profile,
			BandwidthLimitBps: *bandwidthLimit,
			SyncInterval:      *syncInterval,
			FullScanInterval:  *fullScanInterval,
			NoWatch:           *noWatch,
		})
	case "identity":
		flags := flag.NewFlagSet("identity", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		certificate := flags.Bool("certificate", false, "include the public certificate PEM for out-of-band pairing")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("identity accepts no positional arguments")
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(cfg config.Config, _ *repository.DB, _ *workspace.Workspace) error {
			device, err := parseID(cfg.DeviceID)
			if err != nil {
				return err
			}
			identity, err := replication.LoadOrCreateIdentity(*stateDir, device, time.Now())
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "device=%s key-pin=%x\n", cfg.DeviceID, identity.KeyPin)
			if *certificate {
				_, _ = stdout.Write(identity.CertificatePEM())
			}
			return nil
		})
	case "pair-approve":
		flags := flag.NewFlagSet("pair-approve", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		peerText := flags.String("peer-device", "", "approved peer device ID")
		peerPinText := flags.String("peer-key-pin", "", "approved peer SHA-256 public-key pin")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("pair-approve accepts no positional arguments")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		peer, err := parseID(*peerText)
		if err != nil {
			return err
		}
		peerPin, err := parseDigest(*peerPinText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(cfg config.Config, db *repository.DB, _ *workspace.Workspace) error {
			local, err := parseID(cfg.DeviceID)
			if err != nil {
				return err
			}
			identity, err := replication.LoadOrCreateIdentity(*stateDir, local, time.Now())
			if err != nil {
				return err
			}
			approved, err := db.ApproveMembership(context.Background(), protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: local, KeyPin: identity.KeyPin}, {Device: peer, KeyPin: peerPin}}})
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "approved membership revision=%d digest=%x\n", approved.Revision, approved.Digest)
			return nil
		})
	case "register":
		flags := flag.NewFlagSet("register", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		root := flags.String("root", "", "absolute workspace root")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *root == "" {
			return errors.New("register requires --folder and --root")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(cfg config.Config, db *repository.DB, work *workspace.Workspace) error {
			author, err := parseID(cfg.DeviceID)
			if err != nil {
				return err
			}
			if err := db.EnsureFolder(context.Background(), folder, author, 1); err != nil {
				return err
			}
			registration, err := work.Register(context.Background(), folder, *root)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "registered folder %s at %s (device=%d inode=%d)\n", *folderText, registration.Path, registration.Device, registration.Inode)
			return nil
		})
	case "scan":
		flags := flag.NewFlagSet("scan", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		full := flags.Bool("full", false, "force full content scan")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("scan accepts no positional arguments")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, _ *repository.DB, work *workspace.Workspace) error {
			result, err := work.ScanWithOptions(context.Background(), folder, workspace.ScanOptions{FullContent: *full})
			if err != nil {
				return err
			}
			for _, envelope := range result.Captured {
				fmt.Fprintf(stdout, "captured %s counter=%d kind=%d\n", envelope.Path, envelope.ID.Counter, envelope.Kind)
			}
			for _, issue := range result.Issues {
				fmt.Fprintf(stdout, "issue %s %s: %v\n", issue.Path, issue.Code, issue.Err)
			}
			if result.Deletion != nil {
				fmt.Fprintf(stdout, "deletion preview token=%s generation=%d paths=%v\n", result.Deletion.Token, result.Deletion.Generation, result.Deletion.Paths)
			}
			return nil
		})
	case "sync":
		flags := flag.NewFlagSet("sync", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		peerURL := flags.String("peer-url", "", "peer HTTPS base URL")
		peerText := flags.String("peer-device", "", "approved peer device ID")
		peerCertificate := flags.String("peer-certificate", "", "path to the peer public certificate PEM")
		jsonOutput := flags.Bool("json", false, "write structured JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *peerURL == "" || *peerCertificate == "" {
			return errors.New("sync requires --folder, --peer-url, --peer-device and --peer-certificate")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		peer, err := parseID(*peerText)
		if err != nil {
			return err
		}
		certificatePEM, err := os.ReadFile(*peerCertificate)
		if err != nil {
			return fmt.Errorf("read peer certificate: %w", err)
		}
		certificate, err := replication.ParsePeerCertificate(certificatePEM)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(cfg config.Config, db *repository.DB, work *workspace.Workspace) error {
			local, err := parseID(cfg.DeviceID)
			if err != nil {
				return err
			}
			membership, err := db.Membership(context.Background(), folder)
			if err != nil {
				return err
			}
			peerPin := replication.PublicKeyPin(certificate)
			if err := db.AuthorizePeer(context.Background(), folder, peer, peerPin, membership.Revision, membership.Digest); err != nil {
				return fmt.Errorf("peer certificate is not approved for this folder: %w", err)
			}
			identity, err := replication.LoadOrCreateIdentity(*stateDir, local, time.Now())
			if err != nil {
				return err
			}
			client, err := replication.NewClient(*peerURL, identity, certificate, peerPin)
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			result, err := replication.NewSyncer(db, work, client, local, peer, folder, membership, replication.TransferOptions{}).Sync(context.Background())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(stdout).Encode(result)
			}
			fmt.Fprintf(stdout, "sync complete: inventoried=%d metadata=%d fetched=%d reused=%d stored=%d receipts=%d applied=%d\n", result.Inventoried, result.MetadataAdded, result.ChunksFetched, result.ChunksReused, result.VersionsStored, result.ReceiptsSent, result.VersionsApplied)
			return nil
		})
	case "status":
		flags := flag.NewFlagSet("status", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		jsonOutput := flags.Bool("json", false, "write structured JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("status accepts no positional arguments")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, _ *workspace.Workspace) error {
			ids, err := db.VersionIDs(context.Background(), folder)
			if err != nil {
				return err
			}
			statuses := make([]repository.VersionStatus, 0, len(ids))
			for _, id := range ids {
				status, err := db.VersionStatus(context.Background(), id)
				if err != nil {
					return err
				}
				statuses = append(statuses, status)
			}
			peers, err := db.PeerProgress(context.Background(), folder)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(stdout).Encode(struct {
					Versions []repository.VersionStatus `json:"versions"`
					Peers    []repository.PeerProgress  `json:"peers"`
				}{statuses, peers})
			}
			for _, status := range statuses {
				fmt.Fprintf(stdout, "version author=%x counter=%d path=%s stored=%t applied=%t conflict=%t blocked=%t state=%s\n", status.ID.Author, status.ID.Counter, status.Path, status.Stored, status.Applied, status.Conflict, status.Blocked, status.ContentState)
			}
			for _, progress := range peers {
				fmt.Fprintf(stdout, "peer=%x version=%x:%d receipt=%t direct=%t remote=%s last-contact=%s\n", progress.Peer, progress.Version.Author, progress.Version.Counter, progress.Receipt, progress.Direct, progress.RemoteState, progress.LastContact.UTC().Format(time.RFC3339Nano))
			}
			return nil
		})
	case "inspect":
		flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("inspect accepts no positional arguments")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, _ *workspace.Workspace) error {
			projections, err := db.Projections(context.Background(), folder)
			if err != nil {
				return err
			}
			for _, projection := range projections {
				fmt.Fprintf(stdout, "%s kind=%d basis=%d generation=%d block=%s\n", projection.Path, projection.Kind, len(projection.Basis), projection.PublicationGeneration, projection.BlockReason)
			}
			return nil
		})
	case "approve-deletions":
		return handleApproveDeletions(args[1:], stdout, stderr)
	case "apply":
		flags := flag.NewFlagSet("apply", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		authorText := flags.String("author", "", "32-byte author ID in hex")
		counter := flags.Uint64("counter", 0, "version counter")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *counter == 0 {
			return errors.New("apply requires --folder, --author and --counter")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		author, err := parseID(*authorText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, _ *repository.DB, work *workspace.Workspace) error {
			if err := work.Apply(context.Background(), history.VersionID{Folder: folder, Author: author, Counter: *counter}); err != nil {
				return err
			}
			fmt.Fprintln(stdout, "applied")
			return nil
		})
	case "conflicts":
		if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
			switch args[1] {
			case "select":
				return handleSelect(args[2:], stdout, stderr)
			case "merge":
				return handleMerge(args[2:], stdout, stderr)
			case "keep-copies":
				return handleKeepCopies(args[2:], stdout, stderr)
			default:
				return fmt.Errorf("unknown conflicts subcommand %q", args[1])
			}
		}
		flags := flag.NewFlagSet("conflicts", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		jsonOutput := flags.Bool("json", false, "write structured JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *folderText == "" {
			return errors.New("conflicts requires --folder")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, _ *workspace.Workspace) error {
			conflicts, err := db.Conflicts(context.Background(), folder)
			if err != nil {
				return err
			}
			structural, err := db.StructuralConflicts(context.Background(), folder)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(stdout).Encode(struct {
					Conflicts  []repository.ConflictSet     `json:"conflicts"`
					Structural []history.StructuralConflict `json:"structural"`
				}{conflicts, structural})
			}
			for _, c := range conflicts {
				appliedStr := "none"
				if c.Applied != nil {
					appliedStr = fmt.Sprintf("%x:%d", c.Applied.Author, c.Applied.Counter)
				}
				fmt.Fprintf(stdout, "conflict path=%s kind=%s heads=%d applied=%s head_token=%x\n", c.Path, c.ConflictKind, len(c.Heads), appliedStr, c.HeadToken)
				for _, h := range c.Heads {
					digestStr := ""
					size := uint64(0)
					exec := false
					if h.Manifest != nil {
						digestStr = hex.EncodeToString(h.Manifest.Digest[:])
						size = h.Manifest.Size
						exec = h.Manifest.Executable
					}
					fmt.Fprintf(stdout, "  head author=%x counter=%d kind=%d digest=%s size=%d executable=%t applied=%t state=%s\n", h.ID.Author, h.ID.Counter, h.Kind, digestStr, size, exec, h.Applied, h.ContentState)
				}
			}
			for _, sc := range structural {
				fmt.Fprintf(stdout, "structural ancestor=%s (%x:%d) descendant=%s (%x:%d)\n", sc.AncestorPath, sc.Ancestor.Author, sc.Ancestor.Counter, sc.DescendantPath, sc.Descendant.Author, sc.Descendant.Counter)
			}
			return nil
		})
	case "resolve":
		if len(args) < 2 {
			return errors.New("resolve requires a subcommand: select, merge, or keep-copies")
		}
		switch args[1] {
		case "select":
			return handleSelect(args[2:], stdout, stderr)
		case "merge":
			return handleMerge(args[2:], stdout, stderr)
		case "keep-copies":
			return handleKeepCopies(args[2:], stdout, stderr)
		default:
			return fmt.Errorf("unknown resolve subcommand %q", args[1])
		}
	case "peers":
		if len(args) < 2 {
			return errors.New("peers requires a subcommand: list or retire")
		}
		switch args[1] {
		case "list":
			return handlePeersList(args[2:], stdout, stderr)
		case "retire":
			return handlePeersRetire(args[2:], stdout, stderr)
		default:
			return fmt.Errorf("unknown peers subcommand %q", args[1])
		}
	case "membership":
		if len(args) < 2 {
			return errors.New("membership requires a subcommand: export, import, or preview")
		}
		switch args[1] {
		case "export":
			return handleMembershipExport(args[2:], stdout, stderr)
		case "import":
			return handleMembershipImport(args[2:], stdout, stderr)
		case "preview":
			return handleMembershipPreview(args[2:], stdout, stderr)
		default:
			return fmt.Errorf("unknown membership subcommand %q", args[1])
		}
	case "enroll":
		if len(args) < 2 {
			return errors.New("enroll requires a subcommand: preview or bootstrap")
		}
		switch args[1] {
		case "preview":
			return handleEnrollPreview(args[2:], stdout, stderr)
		case "bootstrap":
			return handleEnrollBootstrap(args[2:], stdout, stderr)
		default:
			return fmt.Errorf("unknown enroll subcommand %q", args[1])
		}
	case "storage":
		if len(args) < 2 {
			return errors.New("storage requires a subcommand: usage, retention, gc, recovery, check, or repair")
		}
		switch args[1] {
		case "usage":
			return handleStorageUsage(args[2:], stdout, stderr)
		case "retention":
			if len(args) < 3 {
				return errors.New("storage retention requires a subcommand: preview or change")
			}
			switch args[2] {
			case "preview":
				return handleStorageRetentionPreview(args[3:], stdout, stderr)
			case "change":
				return handleStorageRetentionChange(args[3:], stdout, stderr)
			default:
				return fmt.Errorf("unknown storage retention subcommand %q", args[2])
			}
		case "gc":
			if len(args) < 3 {
				return errors.New("storage gc requires a subcommand: preview or run")
			}
			switch args[2] {
			case "preview":
				return handleStorageGCPreview(args[3:], stdout, stderr)
			case "run":
				return handleStorageGCRun(args[3:], stdout, stderr)
			default:
				return fmt.Errorf("unknown storage gc subcommand %q", args[2])
			}
		case "recovery":
			if len(args) < 3 {
				return errors.New("storage recovery requires a subcommand: reclaim")
			}
			switch args[2] {
			case "reclaim":
				return handleStorageRecoveryReclaim(args[3:], stdout, stderr)
			default:
				return fmt.Errorf("unknown storage recovery subcommand %q", args[2])
			}
		case "check":
			return handleStorageCheck(args[2:], stdout, stderr)
		case "repair":
			return handleStorageRepair(args[2:], stdout, stderr)
		default:
			return fmt.Errorf("unknown storage subcommand %q", args[1])
		}
	case "check":
		return handleStorageCheck(args[1:], stdout, stderr)
	case "repair":
		return handleStorageRepair(args[1:], stdout, stderr)
	case "restore":
		return handleRestore(args[1:], stdout, stderr)
	case "export":
		return handleExport(args[1:], stdout, stderr)
	case "history":
		return handleHistory(args[1:], stdout, stderr)
	case "work":
		if len(args) < 2 {
			return errors.New("work requires a subcommand: scan, sync, status, retry, cancel, or list")
		}
		switch args[1] {
		case "scan":
			return handleWorkScan(args[2:], stdout, stderr)
		case "sync":
			return handleWorkSync(args[2:], stdout, stderr)
		case "status":
			return handleWorkStatus(args[2:], stdout, stderr)
		case "retry":
			return handleWorkRetry(args[2:], stdout, stderr)
		case "cancel":
			return handleWorkCancel(args[2:], stdout, stderr)
		case "list":
			return handleWorkList(args[2:], stdout, stderr)
		default:
			return fmt.Errorf("unknown work subcommand %q", args[1])
		}
	case "doctor":
		return handleDoctor(args[1:], stdout, stderr)
	case "support-export":
		return handleSupportExport(args[1:], stdout, stderr)
	case "logs":
		return handleLogs(args[1:], stdout, stderr)
	case "metrics":
		return handleMetrics(args[1:], stdout, stderr)
	case "folders":
		return handleFolders(args[1:], stdout, stderr)
	case "safety":
		return handleSafety(args[1:], stdout, stderr)
	case "maintenance":
		return handleMaintenance(args[1:], stdout, stderr)
	case "control":
		return handleControl(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: filesync <init|serve|stop|config|identity|pair-approve|register|scan|sync|status|peers|membership|enroll|conflicts|resolve|restore|export|history|storage|check|repair|work|inspect|approve-deletions|apply|doctor|support-export|logs|metrics|folders|safety|maintenance|control|version> [options]")
}

func parseDigest(value string) (history.Digest, error) {
	var digest history.Digest
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != len(digest) || hex.EncodeToString(raw) != value {
		return digest, errors.New("digest must be 64 lowercase hexadecimal characters")
	}
	copy(digest[:], raw)
	if digest == (history.Digest{}) {
		return digest, errors.New("digest cannot be zero")
	}
	return digest, nil
}

func parseID(value string) (history.ID, error) {
	var id history.ID
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != len(id) {
		return id, errors.New("ID must be 64 hexadecimal characters")
	}
	copy(id[:], raw)
	if id == (history.ID{}) {
		return id, errors.New("ID cannot be zero")
	}
	return id, nil
}

func handleSelect(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("select", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	path := flags.String("path", "", "workspace relative path")
	reviewedText := flags.String("reviewed", "", "comma-separated reviewed version IDs (author:counter)")
	headTokenText := flags.String("head-token", "", "expected head token in 64-char hex")
	selectedText := flags.String("selected", "", "selected version ID (author:counter)")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folderText == "" || *path == "" || *selectedText == "" {
		return errors.New("select requires --folder, --path, and --selected")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	selected, err := parseVersionID(folder, *selectedText)
	if err != nil {
		return fmt.Errorf("invalid --selected: %w", err)
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, work *workspace.Workspace) error {
		var reviewed []history.VersionID
		if *reviewedText != "" {
			var err error
			reviewed, err = parseVersionIDList(folder, *reviewedText)
			if err != nil {
				return fmt.Errorf("invalid --reviewed: %w", err)
			}
		} else {
			heads, err := db.Heads(context.Background(), folder, *path)
			if err != nil {
				return err
			}
			for _, h := range heads {
				reviewed = append(reviewed, h.ID)
			}
		}

		var headToken history.Digest
		if *headTokenText != "" {
			var err error
			headToken, err = parseDigest(*headTokenText)
			if err != nil {
				return fmt.Errorf("invalid --head-token: %w", err)
			}
		} else {
			headToken = history.HeadToken(reviewed)
		}

		ctrl := control.New(db, work)
		res, err := ctrl.ResolveSelect(context.Background(), control.ResolveSelectRequest{
			Folder:            folder,
			Path:              *path,
			Reviewed:          reviewed,
			ExpectedHeadToken: headToken,
			Selected:          selected,
			IdempotencyKey:    *idempotencyKey,
		})
		if err != nil {
			return err
		}

		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		replayStr := ""
		if res.Replay {
			replayStr = " (replay)"
		}
		fmt.Fprintf(stdout, "resolved path=%s action=%s version=%x:%d applied=%t%s\n",
			res.Path, res.Action, res.ResolvedID.Author, res.ResolvedID.Counter, res.Applied, replayStr)
		return nil
	})
}

func handleDoctor(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		report, err := ctrl.Doctor(context.Background())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(report)
		}
		fmt.Fprintf(stdout, "doctor report: overall=%s\n", report.OverallStatus)
		for _, c := range report.Checks {
			fmt.Fprintf(stdout, "  [%s] %s: %s\n", c.Status, c.Name, c.Message)
			if c.Remediation != "" {
				fmt.Fprintf(stdout, "    remediation: %s\n", c.Remediation)
			}
		}
		if report.OverallStatus == control.StatusFail {
			return errors.New("doctor checks reported failure")
		}
		return nil
	})
}

func handleSupportExport(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("support-export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	outPath := flags.String("out", "", "target output file (.tar.gz)")
	unredacted := flags.Bool("unredacted", false, "disable path pseudonymization/redaction")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		res, err := ctrl.ExportSupportBundle(context.Background(), control.SupportExportRequest{
			DestinationPath: *outPath,
			RedactPaths:     !*unredacted,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "support bundle written to %s (size=%d bytes, redacted=%v)\n", res.ArchivePath, res.TotalBytes, res.RedactedPaths)
		return nil
	})
}

func handleLogs(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	limit := flags.Int("limit", 50, "maximum number of events to display")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		events, err := ctrl.ListEvents(context.Background(), *limit)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(events)
		}
		fmt.Fprintf(stdout, "events count=%d\n", len(events))
		for _, e := range events {
			fmt.Fprintf(stdout, "  [ts=%d] id=%d op=%s phase=%s error=%s duration=%dns\n",
				e.TimestampNS, e.ID, e.OperationID, e.Phase, e.ErrorCode, e.DurationNS)
		}
		return nil
	})
}

func handleMetrics(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("metrics", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		m, err := ctrl.Metrics(context.Background())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(m)
		}
		fmt.Fprintf(stdout, "metrics:\n")
		fmt.Fprintf(stdout, "  captured_files=%d captured_bytes=%d\n", m.CapturedFiles, m.CapturedBytes)
		fmt.Fprintf(stdout, "  queue_depth=%d max_queue_age_ns=%d\n", m.QueueDepth, m.MaxQueueAgeNS)
		fmt.Fprintf(stdout, "  active_conflicts=%d\n", m.ActiveConflicts)
		fmt.Fprintf(stdout, "  staging_bytes=%d metadata_bytes=%d\n", m.StagingBytes, m.MetadataBytes)
		return nil
	})
}

func handleFolders(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return errors.New("folders requires a subcommand: list, add, pause, resume, remove, or revalidate")
	}
	switch args[0] {
	case "list":
		return handleFoldersList(args[1:], stdout, stderr)
	case "add":
		return handleFoldersAdd(args[1:], stdout, stderr)
	case "pause":
		return handleFoldersPause(args[1:], stdout, stderr)
	case "resume":
		return handleFoldersResume(args[1:], stdout, stderr)
	case "remove":
		return handleFoldersRemove(args[1:], stdout, stderr)
	case "revalidate":
		return handleFoldersRevalidate(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown folders subcommand %q", args[0])
	}
}

func handleFoldersList(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("folders list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		folders, err := ctrl.Folders(context.Background())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(folders)
		}
		fmt.Fprintf(stdout, "folders count=%d\n", len(folders))
		for _, f := range folders {
			status := "active"
			if f.Paused {
				status = fmt.Sprintf("paused (%s)", f.PauseReason)
			}
			fmt.Fprintf(stdout, "  folder %x status=%s root=%s\n", f.Folder, status, f.RootPath)
		}
		return nil
	})
}

func handleFoldersAdd(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("folders add", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	rootPath := flags.String("root", "", "absolute workspace root directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *folderText == "" || *rootPath == "" {
		return errors.New("folders add requires --folder and --root")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		reg, err := ctrl.RegisterFolder(context.Background(), folder, *rootPath)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(reg)
		}
		fmt.Fprintf(stdout, "registered folder %x at root %s\n", folder, reg.Path)
		return nil
	})
}

func handleFoldersPause(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("folders pause", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	reason := flags.String("reason", "operator paused", "reason for pausing folder")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *folderText == "" {
		return errors.New("folders pause requires --folder")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		if err := ctrl.PauseFolder(context.Background(), folder, *reason); err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(map[string]any{"status": "paused", "folder": folder})
		}
		fmt.Fprintf(stdout, "paused folder %x (reason: %s)\n", folder, *reason)
		return nil
	})
}

func handleFoldersResume(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("folders resume", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *folderText == "" {
		return errors.New("folders resume requires --folder")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		if err := ctrl.ResumeFolder(context.Background(), folder); err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(map[string]any{"status": "resumed", "folder": folder})
		}
		fmt.Fprintf(stdout, "resumed folder %x\n", folder)
		return nil
	})
}

func handleFoldersRemove(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("folders remove", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *folderText == "" {
		return errors.New("folders remove requires --folder")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		if err := ctrl.UnregisterFolder(context.Background(), folder); err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(map[string]any{
				"status":  "removed",
				"folder":  folder,
				"message": "registration removed preserving working files",
			})
		}
		fmt.Fprintf(stdout, "removed registration for folder %x (working files preserved on disk, zero tombstones emitted)\n", folder)
		return nil
	})
}

func handleFoldersRevalidate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("folders revalidate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *folderText == "" {
		return errors.New("folders revalidate requires --folder")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		if err := ctrl.RevalidateRoot(context.Background(), folder); err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(map[string]any{"status": "valid", "folder": folder})
		}
		fmt.Fprintf(stdout, "revalidated root for folder %x: valid\n", folder)
		return nil
	})
}

func handleSafety(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return errors.New("safety requires a subcommand: root-revalidate or approve-deletions")
	}
	switch args[0] {
	case "root-revalidate":
		return handleFoldersRevalidate(args[1:], stdout, stderr)
	case "approve-deletions":
		return handleApproveDeletions(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown safety subcommand %q", args[0])
	}
}

func handleApproveDeletions(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("approve-deletions", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	token := flags.String("token", "", "deletion preview token")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if flags.NArg() != 0 || *token == "" || *folderText == "" {
		return errors.New("approve-deletions requires --folder and --token")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(_ config.Config, _ *repository.DB, work *workspace.Workspace) error {
		created, err := work.ApproveDeletions(context.Background(), folder, *token)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(map[string]any{
				"approved_count": len(created),
				"folder":         folder,
				"token":          *token,
			})
		}
		fmt.Fprintf(stdout, "approved %d deletion versions\n", len(created))
		return nil
	})
}

func handleMaintenance(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return errors.New("maintenance requires a subcommand: backup, check, preflight, recovery, reset-identity, or restore-backup")
	}
	switch args[0] {
	case "backup":
		return handleMaintenanceBackup(args[1:], stdout, stderr)
	case "check":
		return handleMaintenanceCheck(args[1:], stdout, stderr)
	case "preflight":
		return handleMaintenancePreflight(args[1:], stdout, stderr)
	case "recovery":
		return handleMaintenanceRecovery(args[1:], stdout, stderr)
	case "reset-identity":
		return handleMaintenanceResetIdentity(args[1:], stdout, stderr)
	case "restore-backup":
		return handleMaintenanceRestoreBackup(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown maintenance subcommand %q", args[0])
	}
}

func handleMaintenancePreflight(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("maintenance preflight", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}

	res, err := control.RunPreflight(context.Background(), actualStateDir)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(res)
	}

	fmt.Fprintf(stdout, "upgrade preflight: status=%s database_schema=%d binary_schema=%d agent_running=%v integrity_clean=%v free_space_mb=%d\n",
		res.Status, res.DatabaseSchema, res.BinarySchema, res.AgentRunning, res.IntegrityClean, res.FreeSpaceBytes/(1024*1024))
	if len(res.Issues) > 0 {
		fmt.Fprintf(stdout, "issues:\n")
		for _, issue := range res.Issues {
			fmt.Fprintf(stdout, "  - %s\n", issue)
		}
	}
	if len(res.NextSteps) > 0 {
		fmt.Fprintf(stdout, "next steps:\n")
		for _, step := range res.NextSteps {
			fmt.Fprintf(stdout, "  - %s\n", step)
		}
	}
	if res.Status == "blocked" {
		return fmt.Errorf("preflight check failed: upgrade is blocked")
	}
	return nil
}

func handleMaintenanceRestoreBackup(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("maintenance restore-backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	backupPath := flags.String("backup", "", "path to consistent SQLite backup file (.sqlite)")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *backupPath == "" {
		return errors.New("--backup <path> is required")
	}

	res, err := control.RestoreBackup(context.Background(), actualStateDir, *backupPath)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(res)
	}

	fmt.Fprintf(stdout, "backup restored from %s\n", res.BackupPath)
	fmt.Fprintf(stdout, "causal identity safely reset: old_device=%x new_device=%x key_pin=%x\n",
		res.OldDeviceID, res.NewDeviceID, res.NewKeyPin)
	fmt.Fprintf(stdout, "CRITICAL (Invariant I08): Rolled-back causal author counters have been retired.\n")
	fmt.Fprintf(stdout, "Action required: %s\n", res.Action)
	return nil
}

func handleConfigValidate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}

	// 1. Directory permissions
	if err := state.ValidateDirectory(actualStateDir); err != nil {
		return fmt.Errorf("state directory validation failed: %w", err)
	}

	// 2. config.json
	cfg, err := config.Load(actualStateDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DeviceID == "" || len(cfg.DeviceID) != 64 {
		return fmt.Errorf("invalid device_id in config.json: %q", cfg.DeviceID)
	}
	if _, err := config.LoadPeerEndpoints(actualStateDir); err != nil {
		return fmt.Errorf("peer endpoints: %w", err)
	}
	if _, err := config.LoadStorageLimits(actualStateDir); err != nil {
		return fmt.Errorf("storage limits: %w", err)
	}
	deviceID, err := parseID(cfg.DeviceID)
	if err != nil {
		return fmt.Errorf("parse device_id: %w", err)
	}

	// 3. TLS files
	ident, err := replication.LoadOrCreateIdentity(actualStateDir, deviceID, time.Now())
	if err != nil {
		return fmt.Errorf("load TLS identity: %w", err)
	}

	if *jsonOutput {
		res := map[string]any{
			"valid":     true,
			"state_dir": actualStateDir,
			"device_id": cfg.DeviceID,
			"key_pin":   hex.EncodeToString(ident.KeyPin[:]),
		}
		return json.NewEncoder(stdout).Encode(res)
	}

	fmt.Fprintf(stdout, "configuration is valid: state_dir=%s device_id=%s key_pin=%x\n",
		actualStateDir, cfg.DeviceID, ident.KeyPin)
	return nil
}

func handleMaintenanceBackup(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("maintenance backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	outPath := flags.String("out", "", "target output backup file path (.sqlite)")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		res, err := ctrl.Backup(context.Background(), *outPath)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "backup created at %s (size=%d bytes)\n", res.BackupPath, res.SizeBytes)
		return nil
	})
}

func handleMaintenanceCheck(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("maintenance check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		res, err := ctrl.CheckMigration(context.Background())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "schema check: database=%d binary=%d status=%s action=%s\n",
			res.CurrentDatabaseSchema, res.BinarySchema, res.Status, res.Action)
		return nil
	})
}

func handleMaintenanceRecovery(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("maintenance recovery", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		res, err := ctrl.RecoveryInspection(context.Background())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "recovery inspection: pending_publications=%d quarantined_chunks=%d exhausted_tasks=%d reclaimable=%d\n",
			res.PendingPublications, res.QuarantinedChunks, res.ExhaustedTasks, res.ReclaimableRecovery)
		return nil
	})
}

func handleMaintenanceResetIdentity(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("maintenance reset-identity", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	deviceID := flags.String("device-id", "", "optional specific 32-byte hex device ID to assign")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		res, err := ctrl.ResetIdentity(context.Background(), *deviceID)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "reset identity: old=%x new=%x key_pin=%x (%s)\n", res.OldDeviceID, res.NewDeviceID, res.NewKeyPin, res.Message)
		fmt.Fprintf(stdout, "note: prior memberships have been cleared; reenrolling folders is required\n")
		return nil
	})
}

func handleControl(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return errors.New("control requires a subcommand: bootstrap-token")
	}
	switch args[0] {
	case "bootstrap-token":
		return handleControlBootstrapToken(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown control subcommand %q", args[0])
	}
}

func handleControlBootstrapToken(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("control bootstrap-token", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	address := flags.String("address", "", "explicit control listener address (e.g. 127.0.0.1:8080)")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}

	addr := *address
	if addr == "" {
		addrBytes, err := os.ReadFile(filepath.Join(actualStateDir, "control.addr"))
		if err != nil {
			return fmt.Errorf("control server address not found: agent is not running with --control-listen (error: %w)", err)
		}
		addr = strings.TrimSpace(string(addrBytes))
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}

	tokenBytes, err := os.ReadFile(filepath.Join(actualStateDir, "control.token"))
	if err != nil {
		return fmt.Errorf("failed to read control.token from %s: %w", actualStateDir, err)
	}
	cliToken := strings.TrimSpace(string(tokenBytes))

	req, err := http.NewRequestWithContext(context.Background(), "POST", addr+"/api/v1/auth/bootstrap-token", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cliToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect to control server at %s: %w", addr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var ctrlErr control.ControlError
		if err := json.NewDecoder(resp.Body).Decode(&ctrlErr); err == nil && ctrlErr.Code != "" {
			return fmt.Errorf("control server error: %s - %s", ctrlErr.Code, ctrlErr.Message)
		}
		return fmt.Errorf("control server returned HTTP %d", resp.StatusCode)
	}

	var res struct {
		BootstrapToken string `json:"bootstrap_token"`
		ExpiresInSecs  int    `json:"expires_in_secs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(res)
	}
	fmt.Fprintf(stdout, "bootstrap token: %s (valid for %d seconds)\n", res.BootstrapToken, res.ExpiresInSecs)
	fmt.Fprintf(stdout, "submit this one-time token in your browser UI to establish an authenticated session\n")
	return nil
}

func handleMerge(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("merge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	path := flags.String("path", "", "workspace relative path")
	reviewedText := flags.String("reviewed", "", "comma-separated reviewed version IDs (author:counter)")
	headTokenText := flags.String("head-token", "", "expected head token in 64-char hex")
	file := flags.String("file", "", "path to file containing merged content")
	content := flags.String("content", "", "direct merged content string")
	executable := flags.Bool("executable", false, "mark merged file as executable")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folderText == "" || *path == "" {
		return errors.New("merge requires --folder and --path")
	}
	if *file == "" && *content == "" {
		return errors.New("merge requires --file or --content")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}

	var mergeBytes []byte
	if *file != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			return fmt.Errorf("reading merge file: %w", err)
		}
		mergeBytes = data
	} else {
		mergeBytes = []byte(*content)
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, work *workspace.Workspace) error {
		var reviewed []history.VersionID
		if *reviewedText != "" {
			var err error
			reviewed, err = parseVersionIDList(folder, *reviewedText)
			if err != nil {
				return fmt.Errorf("invalid --reviewed: %w", err)
			}
		} else {
			heads, err := db.Heads(context.Background(), folder, *path)
			if err != nil {
				return err
			}
			for _, h := range heads {
				reviewed = append(reviewed, h.ID)
			}
		}

		var headToken history.Digest
		if *headTokenText != "" {
			var err error
			headToken, err = parseDigest(*headTokenText)
			if err != nil {
				return fmt.Errorf("invalid --head-token: %w", err)
			}
		} else {
			headToken = history.HeadToken(reviewed)
		}

		ctrl := control.New(db, work)
		res, err := ctrl.ResolveManualMerge(context.Background(), control.ResolveMergeRequest{
			Folder:            folder,
			Path:              *path,
			Reviewed:          reviewed,
			ExpectedHeadToken: headToken,
			Executable:        *executable,
			Content:           mergeBytes,
			IdempotencyKey:    *idempotencyKey,
		})
		if err != nil {
			return err
		}

		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		replayStr := ""
		if res.Replay {
			replayStr = " (replay)"
		}
		fmt.Fprintf(stdout, "resolved path=%s action=%s version=%x:%d applied=%t%s\n",
			res.Path, res.Action, res.ResolvedID.Author, res.ResolvedID.Counter, res.Applied, replayStr)
		return nil
	})
}

func handleKeepCopies(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("keep-copies", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	path := flags.String("path", "", "workspace relative path")
	reviewedText := flags.String("reviewed", "", "comma-separated reviewed version IDs (author:counter)")
	headTokenText := flags.String("head-token", "", "expected head token in 64-char hex")
	copiesText := flags.String("copies", "", "comma-separated head=dest_path assignments")
	selectedText := flags.String("selected", "", "head to retain at original path (optional)")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folderText == "" || *path == "" {
		return errors.New("keep-copies requires --folder and --path")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, work *workspace.Workspace) error {
		var reviewed []history.VersionID
		if *reviewedText != "" {
			var err error
			reviewed, err = parseVersionIDList(folder, *reviewedText)
			if err != nil {
				return fmt.Errorf("invalid --reviewed: %w", err)
			}
		} else {
			heads, err := db.Heads(context.Background(), folder, *path)
			if err != nil {
				return err
			}
			for _, h := range heads {
				reviewed = append(reviewed, h.ID)
			}
		}

		var headToken history.Digest
		if *headTokenText != "" {
			var err error
			headToken, err = parseDigest(*headTokenText)
			if err != nil {
				return fmt.Errorf("invalid --head-token: %w", err)
			}
		} else {
			headToken = history.HeadToken(reviewed)
		}

		var copies []control.CopyTarget
		if *copiesText != "" {
			var err error
			copies, err = parseCopyTargets(folder, *copiesText)
			if err != nil {
				return fmt.Errorf("invalid --copies: %w", err)
			}
		}

		var origSelected *history.VersionID
		if *selectedText != "" {
			sel, err := parseVersionID(folder, *selectedText)
			if err != nil {
				return fmt.Errorf("invalid --selected: %w", err)
			}
			origSelected = &sel
		}

		ctrl := control.New(db, work)
		res, err := ctrl.ResolveKeepCopies(context.Background(), control.KeepCopiesRequest{
			Folder:            folder,
			Path:              *path,
			Reviewed:          reviewed,
			ExpectedHeadToken: headToken,
			Copies:            copies,
			OriginalSelected:  origSelected,
			IdempotencyKey:    *idempotencyKey,
		})
		if err != nil {
			return err
		}

		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		replayStr := ""
		if res.Replay {
			replayStr = " (replay)"
		}
		fmt.Fprintf(stdout, "resolved path=%s version=%x:%d completed=%t%s\n",
			res.Path, res.ResolvedID.Author, res.ResolvedID.Counter, res.Completed, replayStr)
		for _, cp := range res.Copies {
			fmt.Fprintf(stdout, "  copy head=%x:%d dest=%s version=%x:%d\n",
				cp.HeadID.Author, cp.HeadID.Counter, cp.DestinationPath, cp.VersionID.Author, cp.VersionID.Counter)
		}
		return nil
	})
}

func handleRestore(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	path := flags.String("path", "", "workspace relative path")
	sourceText := flags.String("source", "", "historical version ID to restore (author:counter)")
	reviewedText := flags.String("reviewed", "", "comma-separated reviewed current head IDs (author:counter)")
	headTokenText := flags.String("head-token", "", "expected head token in 64-char hex")
	preview := flags.Bool("preview", false, "inspect historical version without modifying history or workspace")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folderText == "" || *path == "" || *sourceText == "" {
		return errors.New("restore requires --folder, --path, and --source")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	sourceVersion, err := parseVersionID(folder, *sourceText)
	if err != nil {
		return fmt.Errorf("invalid --source: %w", err)
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, work *workspace.Workspace) error {
		ctrl := control.New(db, work)
		if *preview {
			prev, err := ctrl.PreviewRestore(context.Background(), control.RestorePreviewRequest{
				Folder:        folder,
				Path:          *path,
				SourceVersion: sourceVersion,
			})
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(stdout).Encode(prev)
			}
			fmt.Fprintf(stdout, "restore-preview path=%s source=%x:%d kind=%d size=%d digest=%x executable=%t state=%s heads=%d head_token=%x\n",
				prev.Path, prev.SourceVersion.Author, prev.SourceVersion.Counter, prev.SourceKind, prev.SourceSize, prev.SourceDigest, prev.SourceExecutable, prev.ContentState, len(prev.CurrentHeads), prev.ExpectedHeadToken)
			for _, h := range prev.CurrentHeads {
				fmt.Fprintf(stdout, "  current-head %x:%d\n", h.Author, h.Counter)
			}
			return nil
		}

		var reviewed []history.VersionID
		if *reviewedText != "" {
			var err error
			reviewed, err = parseVersionIDList(folder, *reviewedText)
			if err != nil {
				return fmt.Errorf("invalid --reviewed: %w", err)
			}
		} else {
			heads, err := db.Heads(context.Background(), folder, *path)
			if err != nil {
				return err
			}
			for _, h := range heads {
				reviewed = append(reviewed, h.ID)
			}
		}

		var headToken history.Digest
		if *headTokenText != "" {
			var err error
			headToken, err = parseDigest(*headTokenText)
			if err != nil {
				return fmt.Errorf("invalid --head-token: %w", err)
			}
		} else {
			headToken = history.HeadToken(reviewed)
		}

		res, err := ctrl.Restore(context.Background(), control.RestoreRequest{
			Folder:            folder,
			Path:              *path,
			SourceVersion:     sourceVersion,
			Reviewed:          reviewed,
			ExpectedHeadToken: headToken,
			IdempotencyKey:    *idempotencyKey,
		})
		if err != nil {
			return err
		}

		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		replayStr := ""
		if res.Replay {
			replayStr = " (replay)"
		}
		fmt.Fprintf(stdout, "restored path=%s action=%s version=%x:%d applied=%t%s\n",
			res.Path, res.Action, res.ResolvedID.Author, res.ResolvedID.Counter, res.Applied, replayStr)
		return nil
	})
}

func handleExport(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	versionText := flags.String("version", "", "version ID to export (author:counter)")
	outPath := flags.String("out", "", "destination file path (writes to stdout if omitted)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folderText == "" || *versionText == "" {
		return errors.New("export requires --folder and --version")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	verID, err := parseVersionID(folder, *versionText)
	if err != nil {
		return fmt.Errorf("invalid --version: %w", err)
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, work *workspace.Workspace) error {
		ctrl := control.New(db, work)
		var dest io.Writer = stdout
		var f *os.File
		if *outPath != "" {
			var err error
			f, err = os.OpenFile(*outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("creating destination file: %w", err)
			}
			defer f.Close()
			dest = f
		}

		if err := ctrl.Export(context.Background(), folder, verID, dest); err != nil {
			return err
		}
		if *outPath != "" {
			fmt.Fprintf(stdout, "exported %x:%d to %s\n", verID.Author, verID.Counter, *outPath)
		}
		return nil
	})
}

func handleHistory(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("history", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	path := flags.String("path", "", "workspace relative path")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folderText == "" || *path == "" {
		return errors.New("history requires --folder and --path")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, work *workspace.Workspace) error {
		ctrl := control.New(db, work)
		items, err := ctrl.History(context.Background(), folder, *path)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(items)
		}
		for _, it := range items {
			digestStr := ""
			size := uint64(0)
			exec := false
			if it.Manifest != nil {
				digestStr = hex.EncodeToString(it.Manifest.Digest[:])
				size = it.Manifest.Size
				exec = it.Manifest.Executable
			}
			fmt.Fprintf(stdout, "version %x:%d path=%s kind=%d digest=%s size=%d exec=%t state=%s applied=%t head=%t time=%s\n",
				it.ID.Author, it.ID.Counter, it.Path, it.Kind, digestStr, size, exec, it.ContentState, it.Applied, it.IsHead, it.DisplayTime)
		}
		return nil
	})
}

func parseVersionID(folder history.ID, value string) (history.VersionID, error) {
	parts := strings.Split(value, ":")
	if len(parts) == 1 {
		parts = strings.Split(value, "/")
	}
	if len(parts) == 2 {
		author, err := parseID(parts[0])
		if err != nil {
			return history.VersionID{}, err
		}
		counter, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return history.VersionID{}, err
		}
		return history.VersionID{Folder: folder, Author: author, Counter: counter}, nil
	}
	if len(parts) == 3 {
		parsedFolder, err := parseID(parts[0])
		if err != nil {
			return history.VersionID{}, err
		}
		author, err := parseID(parts[1])
		if err != nil {
			return history.VersionID{}, err
		}
		counter, err := strconv.ParseUint(parts[2], 10, 64)
		if err != nil {
			return history.VersionID{}, err
		}
		return history.VersionID{Folder: parsedFolder, Author: author, Counter: counter}, nil
	}
	return history.VersionID{}, fmt.Errorf("expected <author>:<counter>, got %q", value)
}

func parseVersionIDList(folder history.ID, value string) ([]history.VersionID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw := strings.Split(value, ",")
	var list []history.VersionID
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := parseVersionID(folder, s)
		if err != nil {
			return nil, err
		}
		list = append(list, id)
	}
	return list, nil
}

func parseCopyTargets(folder history.ID, value string) ([]control.CopyTarget, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw := strings.Split(value, ",")
	var targets []control.CopyTarget
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		pair := strings.SplitN(item, "=", 2)
		if len(pair) != 2 {
			return nil, fmt.Errorf("expected <version_id>=<dest_path>, got %q", item)
		}
		vid, err := parseVersionID(folder, strings.TrimSpace(pair[0]))
		if err != nil {
			return nil, err
		}
		targets = append(targets, control.CopyTarget{
			HeadID:          vid,
			DestinationPath: strings.TrimSpace(pair[1]),
		})
	}
	return targets, nil
}

func handlePeersList(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("peers list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.PeerList(context.Background(), folder)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "folder=%x revision=%d digest=%x active_count=%d retired_count=%d\n", res.Folder, res.Revision, res.Digest, len(res.Active), len(res.Retired))
		for _, a := range res.Active {
			fmt.Fprintf(stdout, "  active device=%x key-pin=%x\n", a.Device, a.KeyPin)
		}
		for _, r := range res.Retired {
			fmt.Fprintf(stdout, "  retired device=%x retired-at=%d snapshot-digest=%x\n", r.Device, r.RetiredAt, r.SnapshotDigest)
		}
		return nil
	})
}

func handlePeersRetire(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("peers retire", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	targetText := flags.String("peer-device", "", "32-byte peer device ID to retire in hex")
	preview := flags.Bool("preview", false, "preview retirement without executing")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	target, err := parseID(*targetText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		ctx := context.Background()
		if *preview {
			prev, err := ctrl.RetireMemberPreview(ctx, folder, target)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(stdout).Encode(prev)
			}
			fmt.Fprintf(stdout, "retirement preview: folder=%x target=%x current_rev=%d next_rev=%d accepted_versions=%d snapshot_digest=%x next_membership_digest=%x\n",
				prev.Folder, prev.TargetDevice, prev.CurrentRevision, prev.NextRevision, prev.AcceptedVersionsCount, prev.SnapshotDigest, prev.NextMembershipDigest)
			for _, s := range prev.SurvivingMembers {
				fmt.Fprintf(stdout, "  surviving member device=%x\n", s)
			}
			return nil
		}
		res, err := ctrl.RetireMemberExecute(ctx, control.RetireMemberRequest{
			Folder:         folder,
			TargetDevice:   target,
			IdempotencyKey: *idempotencyKey,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "member retired: folder=%x target=%x approved_revision=%d approved_digest=%x replay=%t\n",
			res.Folder, res.TargetDevice, res.ApprovedRevision, res.ApprovedDigest, res.Replay)
		return nil
	})
}

func handleMembershipExport(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("membership export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	revision := flags.Uint64("revision", 0, "specific membership revision (default latest)")
	filePath := flags.String("file", "", "output file path to write exported bundle")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		var revs []uint64
		if *revision > 0 {
			revs = append(revs, *revision)
		}
		res, err := ctrl.MembershipExport(context.Background(), folder, revs...)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		if *filePath != "" {
			if err := os.WriteFile(*filePath, data, 0644); err != nil {
				return fmt.Errorf("write export file: %w", err)
			}
		}
		if *jsonOutput {
			_, err = stdout.Write(append(data, '\n'))
			return err
		}
		if *filePath != "" {
			fmt.Fprintf(stdout, "exported membership revision %d (digest %x) to %s\n", res.Membership.Revision, res.Digest, *filePath)
		} else {
			_, err = stdout.Write(append(data, '\n'))
			return err
		}
		return nil
	})
}

func handleMembershipPreview(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("membership preview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	filePath := flags.String("file", "", "bundle file to preview")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("--file is required")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	var data []byte
	if *filePath == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(*filePath)
	}
	if err != nil {
		return fmt.Errorf("read membership file: %w", err)
	}
	var bundle control.MembershipExportResult
	if err := json.Unmarshal(data, &bundle); err != nil || bundle.Membership.Revision == 0 {
		var m protocol.Membership
		if err2 := json.Unmarshal(data, &m); err2 == nil && m.Revision > 0 {
			bundle.Membership = m
		} else if err != nil {
			return fmt.Errorf("decode membership bundle: %w", err)
		}
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.MembershipPreview(context.Background(), folder, bundle.Membership)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "membership preview: folder=%x current_rev=%d current_digest=%x next_rev=%d next_digest=%x valid=%t prior_digest_matches=%t reason=%s\n",
			res.Folder, res.CurrentRevision, res.CurrentDigest, res.NextRevision, res.NextDigest, res.ValidTransition, res.PriorDigestMatches, res.Reason)
		for _, a := range res.AddedActive {
			fmt.Fprintf(stdout, "  added active: %x\n", a)
		}
		for _, r := range res.RemovedActive {
			fmt.Fprintf(stdout, "  removed active: %x\n", r)
		}
		for _, rm := range res.NewlyRetired {
			fmt.Fprintf(stdout, "  new retired: %x\n", rm)
		}
		return nil
	})
}

func handleMembershipImport(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("membership import", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	filePath := flags.String("file", "", "bundle file to import")
	approve := flags.Bool("approve", false, "approve and commit the imported membership revision")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return errors.New("--file is required")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	var data []byte
	if *filePath == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(*filePath)
	}
	if err != nil {
		return fmt.Errorf("read membership file: %w", err)
	}
	var bundle control.MembershipExportResult
	if err := json.Unmarshal(data, &bundle); err != nil || bundle.Membership.Revision == 0 {
		var m protocol.Membership
		if err2 := json.Unmarshal(data, &m); err2 == nil && m.Revision > 0 {
			bundle.Membership = m
		} else if err != nil {
			return fmt.Errorf("decode membership bundle: %w", err)
		}
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		appM, err := ctrl.MembershipImport(context.Background(), folder, bundle.Membership, bundle.Snapshots, *approve)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(struct {
				Approved bool                          `json:"approved"`
				Result   repository.ApprovedMembership `json:"result"`
			}{*approve, appM})
		}
		if *approve {
			fmt.Fprintf(stdout, "membership approved: folder=%x revision=%d digest=%x\n", folder, appM.Revision, appM.Digest)
		} else {
			fmt.Fprintf(stdout, "membership validated (not approved): folder=%x revision=%d digest=%x\n", folder, appM.Revision, appM.Digest)
		}
		return nil
	})
}

func handleEnrollPreview(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("enroll preview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	rootPath := flags.String("root", "", "local directory root to enroll")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *rootPath == "" {
		return errors.New("--root is required")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.EnrollPreview(context.Background(), folder, *rootPath)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "enrollment preview: folder=%x root=%s local_files=%d identical=%d conflicts=%d remote_only=%d\n",
			res.Folder, res.RootPath, res.LocalFilesCount, len(res.IdenticalPaths), len(res.ConflictingPaths), len(res.RemoteOnlyPaths))
		for _, p := range res.IdenticalPaths {
			fmt.Fprintf(stdout, "  identical: %s\n", p)
		}
		for _, p := range res.ConflictingPaths {
			fmt.Fprintf(stdout, "  conflict: %s\n", p)
		}
		for _, p := range res.RemoteOnlyPaths {
			fmt.Fprintf(stdout, "  remote only: %s\n", p)
		}
		return nil
	})
}

func handleEnrollBootstrap(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("enroll bootstrap", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	rootPath := flags.String("root", "", "local directory root to enroll")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *rootPath == "" {
		return errors.New("--root is required")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.EnrollBootstrap(context.Background(), folder, *rootPath)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "enrollment bootstrap complete: folder=%x root=%s captured=%d\n",
			res.Folder, res.RootPath, res.CapturedCount)
		for _, env := range res.Envelopes {
			fmt.Fprintf(stdout, "  captured path=%s version=%x:%d kind=%d\n",
				env.Path, env.ID.Author, env.ID.Counter, env.Kind)
		}
		return nil
	})
}

func handleStorageUsage(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage usage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.StorageUsage(context.Background())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		u := res.Usage
		fmt.Fprintf(stdout, "state_filesystem: path=%s total=%d free=%d available=%d\n",
			u.StateFilesystem.Path, u.StateFilesystem.TotalBytes, u.StateFilesystem.FreeBytes, u.StateFilesystem.AvailableBytes)
		fmt.Fprintf(stdout, "database: metadata_bytes=%d budget=%d\n", u.Metadata, u.MetadataBudgetBytes)
		fmt.Fprintf(stdout, "objects: bytes=%d data_budget=%d\n", u.Objects, u.DataBudgetBytes)
		fmt.Fprintf(stdout, "quarantine: bytes=%d\n", u.Quarantine)
		fmt.Fprintf(stdout, "reservations: total_bytes=%d free_space_reserve=%d\n", u.Reserved, u.FreeSpaceReserveBytes)
		for _, f := range u.Folders {
			fmt.Fprintf(stdout, "folder=%x root_path=%s stage_bytes=%d recovery_bytes=%d\n",
				f.FolderID, f.RootPath, f.StageBytes, f.RecoveryBytes)
		}
		return nil
	})
}

func handleStorageRetentionPreview(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage retention preview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	retentionDays := flags.Int("retention-days", -1, "superseded version retention duration in days")
	minSuperseded := flags.Int("min-superseded", -1, "minimum superseded versions to retain")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	req := control.RetentionPreviewRequest{Folder: folder}
	if *retentionDays >= 0 {
		req.RetentionDays = retentionDays
	}
	if *minSuperseded >= 0 {
		req.MinSuperseded = minSuperseded
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.RetentionPreview(context.Background(), req)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		p := res.Preview
		fmt.Fprintf(stdout, "folder=%x retention_days=%d min_superseded=%d total_versions=%d head_versions=%d retained_versions=%d expired_versions=%d protected_chunks=%d candidate_chunks=%d reclaimable_bytes=%d suspended=%t\n",
			p.FolderID, p.Policy.RetentionDays, p.Policy.MinSuperseded, p.TotalVersions, p.HeadVersions, p.RetainedVersions, p.ExpiredVersions, p.ProtectedChunks, p.CandidateChunks, p.ReclaimableBytes, p.Suspended)
		if p.Suspended {
			fmt.Fprintf(stdout, "  suspend_reason: %s\n", p.SuspendReason)
		}
		return nil
	})
}

func handleStorageRetentionChange(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage retention change", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	retentionDays := flags.Int("retention-days", -1, "superseded version retention duration in days")
	minSuperseded := flags.Int("min-superseded", -1, "minimum superseded versions to retain")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *retentionDays < 0 || *minSuperseded < 0 {
		return errors.New("--retention-days and --min-superseded must both be >= 0")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	req := control.RetentionChangeRequest{
		Folder:        folder,
		RetentionDays: *retentionDays,
		MinSuperseded: *minSuperseded,
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.RetentionChange(context.Background(), req)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "retention policy updated: folder=%x retention_days=%d min_superseded=%d\n",
			res.Folder, res.Policy.RetentionDays, res.Policy.MinSuperseded)
		return nil
	})
}

func handleStorageGCPreview(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage gc preview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "optional 32-byte folder ID in hex")
	retentionDays := flags.Int("retention-days", -1, "superseded version retention duration in days")
	minSuperseded := flags.Int("min-superseded", -1, "minimum superseded versions to retain")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var folder history.ID
	if *folderText != "" {
		var err error
		folder, err = parseID(*folderText)
		if err != nil {
			return err
		}
	}
	req := control.GCPreviewRequest{Folder: folder}
	if *retentionDays >= 0 {
		req.RetentionDays = retentionDays
	}
	if *minSuperseded >= 0 {
		req.MinSuperseded = minSuperseded
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.GCPreview(context.Background(), req)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		r := res.Report
		fmt.Fprintf(stdout, "gc preview: folder=%x candidates=%d unlinked_objects=%d reclaimed_bytes=%d remaining_objects=%d remaining_bytes=%d suspended=%t\n",
			r.FolderID, r.Candidates, r.UnlinkedObjects, r.ReclaimedBytes, r.RemainingObjects, r.RemainingBytes, r.Suspended)
		if r.Suspended {
			fmt.Fprintf(stdout, "  suspend_reason: %s\n", r.SuspendReason)
		}
		return nil
	})
}

func handleStorageGCRun(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage gc run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "optional 32-byte folder ID in hex")
	retentionDays := flags.Int("retention-days", -1, "superseded version retention duration in days")
	minSuperseded := flags.Int("min-superseded", -1, "minimum superseded versions to retain")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var folder history.ID
	if *folderText != "" {
		var err error
		folder, err = parseID(*folderText)
		if err != nil {
			return err
		}
	}
	req := control.GCRunRequest{
		Folder:         folder,
		IdempotencyKey: *idempotencyKey,
	}
	if *retentionDays >= 0 {
		req.RetentionDays = retentionDays
	}
	if *minSuperseded >= 0 {
		req.MinSuperseded = minSuperseded
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.GCRun(context.Background(), req)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		r := res.Report
		fmt.Fprintf(stdout, "gc run: folder=%x unlinked_objects=%d reclaimed_bytes=%d remaining_objects=%d remaining_bytes=%d replay=%t suspended=%t\n",
			r.FolderID, r.UnlinkedObjects, r.ReclaimedBytes, r.RemainingObjects, r.RemainingBytes, res.Replay, r.Suspended)
		if r.Suspended {
			fmt.Fprintf(stdout, "  suspend_reason: %s\n", r.SuspendReason)
		}
		return nil
	})
}

func handleStorageRecoveryReclaim(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage recovery reclaim", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.ReclaimRecoveryCopies(context.Background(), control.ReclaimRecoveryRequest{Folder: folder})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "recovery copies reclaimed: folder=%x count=%d bytes=%d\n",
			res.Folder, res.ReclaimedCount, res.ReclaimedBytes)
		return nil
	})
}

func handleStorageCheck(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "optional 32-byte folder ID in hex")
	path := flags.String("path", "", "optional relative path to inspect")
	versionText := flags.String("version", "", "optional author:counter version ID")
	quarantine := flags.Bool("quarantine", true, "automatically quarantine detected corrupt chunks")
	limit := flags.Int("limit", 0, "maximum number of chunks to inspect")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	var folder history.ID
	if *folderText != "" {
		var err error
		folder, err = parseID(*folderText)
		if err != nil {
			return err
		}
	}

	var versionID *history.VersionID
	if *versionText != "" {
		parts := strings.Split(*versionText, ":")
		if len(parts) != 2 {
			return errors.New("version must be formatted as author:counter")
		}
		author, err := parseID(parts[0])
		if err != nil {
			return err
		}
		counter, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return err
		}
		versionID = &history.VersionID{
			Folder:  folder,
			Author:  author,
			Counter: counter,
		}
	}

	req := control.StorageCheckRequest{
		Folder:         folder,
		Path:           *path,
		VersionID:      versionID,
		AutoQuarantine: *quarantine,
		Limit:          *limit,
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.StorageIntegrityCheck(context.Background(), req)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "integrity check complete: chunks_checked=%d clean=%d corrupt=%d missing_protected=%d expired_historical=%d (duration=%s)\n",
			res.TotalChunksChecked, res.CleanChunks, len(res.CorruptChunks), len(res.MissingProtected), len(res.ExpiredHistorical), time.Duration(res.DurationNS))
		for _, c := range res.CorruptChunks {
			fmt.Fprintf(stdout, "  CORRUPT chunk %x: %s (quarantined=%s, %d affected versions, paths=%v)\n",
				c.Digest, c.Reason, c.QuarantinePath, len(c.AffectedVersions), c.AffectedPaths)
		}
		for _, m := range res.MissingProtected {
			fmt.Fprintf(stdout, "  MISSING PROTECTED chunk %x: %d affected versions, paths=%v\n",
				m.Digest, len(m.AffectedVersions), m.AffectedPaths)
		}
		for _, e := range res.ExpiredHistorical {
			fmt.Fprintf(stdout, "  EXPIRED HISTORICAL chunk %x: %d affected versions\n",
				e.Digest, len(e.AffectedVersions))
		}
		return nil
	})
}

func handleStorageRepair(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("storage repair", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	versionText := flags.String("version", "", "author:counter version ID to repair")
	peerURL := flags.String("peer-url", "", "optional peer HTTPS base URL")
	peerText := flags.String("peer-device", "", "optional approved peer device ID")
	peerCertificate := flags.String("peer-certificate", "", "optional path to peer public certificate PEM")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folderText == "" || *versionText == "" {
		return errors.New("repair requires --folder and --version")
	}

	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}

	parts := strings.Split(*versionText, ":")
	if len(parts) != 2 {
		return errors.New("version must be formatted as author:counter")
	}
	author, err := parseID(parts[0])
	if err != nil {
		return err
	}
	counter, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return err
	}
	targetVersion := history.VersionID{
		Folder:  folder,
		Author:  author,
		Counter: counter,
	}

	return app.WithWorkspace(context.Background(), *stateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		localDevice, err := parseID(cfg.DeviceID)
		if err != nil {
			return err
		}

		var repairPeers []replication.RepairPeer
		if *peerURL != "" && *peerText != "" && *peerCertificate != "" {
			peerDevice, err := parseID(*peerText)
			if err != nil {
				return err
			}
			certPEM, err := os.ReadFile(*peerCertificate)
			if err != nil {
				return fmt.Errorf("read peer certificate: %w", err)
			}
			cert, err := replication.ParsePeerCertificate(certPEM)
			if err != nil {
				return err
			}
			peerPin := replication.PublicKeyPin(cert)
			identity, err := replication.LoadOrCreateIdentity(*stateDir, localDevice, time.Now())
			if err != nil {
				return err
			}
			client, err := replication.NewClient(*peerURL, identity, cert, peerPin)
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			repairPeers = append(repairPeers, replication.RepairPeer{
				DeviceID: peerDevice,
				Client:   client,
			})
		}

		ctrl := control.New(db, ws, control.Options{
			LocalDevice: localDevice,
			RepairPeers: repairPeers,
		})

		req := control.StorageRepairRequest{
			Folder:         folder,
			VersionID:      targetVersion,
			Peers:          repairPeers,
			IdempotencyKey: *idempotencyKey,
		}

		res, err := ctrl.StorageRepair(context.Background(), req)
		if err != nil {
			if *jsonOutput && res.Status != "" {
				_ = json.NewEncoder(stdout).Encode(res)
			}
			return err
		}

		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}

		fmt.Fprintf(stdout, "repair complete: version=%s path=%s status=%s repaired_chunks=%d total_chunks=%d replay=%t message=%s\n",
			*versionText, res.Path, res.Status, res.RepairedChunks, res.TotalChunks, res.Replay, res.Message)
		return nil
	})
}

func handleWorkScan(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("work scan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "optional 32-byte folder ID in hex")
	full := flags.Bool("full", false, "force full content scan")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}

	var folderPtr *history.ID
	if *folderText != "" {
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		folderPtr = &folder
	}

	return app.WithWorkspace(context.Background(), actualStateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.WorkScan(context.Background(), control.WorkScanRequest{
			Folder:         folderPtr,
			FullContent:    *full,
			IdempotencyKey: *idempotencyKey,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "work scan complete: full=%v captured=%d issues=%d duration=%s replay=%t\n",
			res.FullContent, res.CapturedCount, len(res.Issues), time.Duration(res.DurationNS), res.Replay)
		for _, env := range res.CapturedEnvelopes {
			fmt.Fprintf(stdout, "  captured %s counter=%d kind=%d\n", env.Path, env.ID.Counter, env.Kind)
		}
		return nil
	})
}

func handleWorkSync(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("work sync", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "32-byte folder ID in hex")
	peerText := flags.String("peer-device", "", "optional approved peer device ID")
	peerURL := flags.String("peer-url", "", "optional peer HTTPS base URL")
	peerCertificate := flags.String("peer-certificate", "", "optional path to peer public certificate PEM")
	idempotencyKey := flags.String("idempotency-key", "", "optional idempotency key")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *folderText == "" {
		return errors.New("work sync requires --folder")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	var peerPtr *history.ID
	if *peerText != "" {
		peer, err := parseID(*peerText)
		if err != nil {
			return err
		}
		peerPtr = &peer
	}

	return app.WithWorkspace(context.Background(), actualStateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		if *peerURL != "" && *peerCertificate != "" && peerPtr != nil {
			local, err := parseID(cfg.DeviceID)
			if err != nil {
				return err
			}
			membership, err := db.Membership(context.Background(), folder)
			if err != nil {
				return err
			}
			certificatePEM, err := os.ReadFile(*peerCertificate)
			if err != nil {
				return fmt.Errorf("read peer certificate: %w", err)
			}
			certificate, err := replication.ParsePeerCertificate(certificatePEM)
			if err != nil {
				return err
			}
			peerPin := replication.PublicKeyPin(certificate)
			if err := db.AuthorizePeer(context.Background(), folder, *peerPtr, peerPin, membership.Revision, membership.Digest); err != nil {
				return fmt.Errorf("peer certificate is not approved for this folder: %w", err)
			}
			identity, err := replication.LoadOrCreateIdentity(actualStateDir, local, time.Now())
			if err != nil {
				return err
			}
			client, err := replication.NewClient(*peerURL, identity, certificate, peerPin)
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			res, err := ctrl.WorkSync(context.Background(), control.WorkSyncRequest{
				Folder:         folder,
				PeerDevice:     peerPtr,
				IdempotencyKey: *idempotencyKey,
			}, client, local, membership)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return json.NewEncoder(stdout).Encode(res)
			}
			fmt.Fprintf(stdout, "work sync complete: folder=%x peer=%x inventoried=%d metadata=%d fetched=%d reused=%d stored=%d receipts=%d applied=%d replay=%t\n",
				res.Folder, res.PeerDevice, res.Inventoried, res.MetadataAdded, res.ChunksFetched, res.ChunksReused, res.VersionsStored, res.ReceiptsSent, res.VersionsApplied, res.Replay)
			return nil
		}

		taskID, err := db.EnqueueDurableTask(context.Background(), repository.DurableTask{
			Folder: folder,
			Kind:   "sync",
			Peer:   peerPtr,
			State:  "queued",
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(map[string]any{
				"task_id": taskID,
				"folder":  folder,
				"state":   "queued",
			})
		}
		fmt.Fprintf(stdout, "work sync enqueued: task_id=%s folder=%x\n", taskID, folder)
		return nil
	})
}

func handleWorkStatus(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("work status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "optional 32-byte folder ID in hex")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}

	var folderPtr *history.ID
	if *folderText != "" {
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		folderPtr = &folder
	}

	return app.WithWorkspace(context.Background(), actualStateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.WorkStatus(context.Background(), control.WorkStatusRequest{Folder: folderPtr})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		for _, f := range res.Folders {
			statusStr := "active"
			if f.Paused {
				statusStr = fmt.Sprintf("paused (%s)", f.PauseReason)
			}
			fmt.Fprintf(stdout, "folder %x: %s | queued=%d running=%d retry=%d exhausted=%d\n",
				f.Folder, statusStr, f.QueuedTasks, f.RunningTasks, f.RetryTasks, f.ExhaustedTasks)
		}
		return nil
	})
}

func handleWorkRetry(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("work retry", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "optional 32-byte folder ID in hex")
	taskID := flags.String("task", "", "task ID to retry")
	retryAll := flags.Bool("all", false, "retry all exhausted and retry tasks")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *taskID == "" && !*retryAll {
		return errors.New("work retry requires either --task <id> or --all")
	}

	var folder history.ID
	if *folderText != "" {
		var err error
		folder, err = parseID(*folderText)
		if err != nil {
			return err
		}
	}

	return app.WithWorkspace(context.Background(), actualStateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.WorkRetry(context.Background(), control.WorkRetryRequest{
			Folder: folder,
			TaskID: *taskID,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "%s\n", res.Message)
		return nil
	})
}

func handleWorkCancel(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("work cancel", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	stateDirAlt := flags.String("state-dir", "", "agent state directory")
	taskID := flags.String("task", "", "task ID to cancel")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	actualStateDir := *stateDir
	if *stateDirAlt != "" {
		actualStateDir = *stateDirAlt
	}
	if *taskID == "" {
		return errors.New("work cancel requires --task")
	}

	return app.WithWorkspace(context.Background(), actualStateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.WorkCancel(context.Background(), control.WorkCancelRequest{TaskID: *taskID})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "%s\n", res.Message)
		return nil
	})
}

func handleWorkList(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("work list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state-dir", "", "agent state directory")
	stateFlag := flags.String("state", "", "task state filter or agent state directory")
	taskStateAlt := flags.String("task-state", "", "task state filter")
	folderText := flags.String("folder", "", "optional 32-byte folder ID in hex")
	limit := flags.Int("limit", 0, "maximum number of tasks to list")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	validStates := map[string]bool{
		"queued": true, "running": true, "retry": true,
		"exhausted": true, "completed": true, "canceled": true,
	}

	actualStateDir := config.DefaultStateDir()
	if *stateDir != "" {
		actualStateDir = *stateDir
	}

	var taskState string
	if validStates[*stateFlag] {
		taskState = *stateFlag
	} else if *stateFlag != "" {
		actualStateDir = *stateFlag
	}
	if *taskStateAlt != "" {
		taskState = *taskStateAlt
	}

	var folder history.ID
	if *folderText != "" {
		var err error
		folder, err = parseID(*folderText)
		if err != nil {
			return err
		}
	}

	return app.WithWorkspace(context.Background(), actualStateDir, func(_ config.Config, db *repository.DB, ws *workspace.Workspace) error {
		ctrl := control.New(db, ws)
		res, err := ctrl.WorkList(context.Background(), control.WorkListRequest{
			Folder: folder,
			State:  taskState,
			Limit:  *limit,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return json.NewEncoder(stdout).Encode(res)
		}
		fmt.Fprintf(stdout, "tasks count=%d\n", len(res.Tasks))
		for _, t := range res.Tasks {
			fmt.Fprintf(stdout, "  task %s kind=%s state=%s folder=%x attempts=%d/%d file_size=%d age=%d",
				t.ID, t.Kind, t.State, t.Folder, t.Attempts, t.MaxAttempts, t.FileSize, t.AgeCounter)
			if t.ErrorCode != "" {
				fmt.Fprintf(stdout, " error=%s", t.ErrorCode)
			}
			fmt.Fprintln(stdout)
		}
		return nil
	})
}
