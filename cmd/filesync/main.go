package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

var version = "dev"

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
		fmt.Fprintf(stdout, "filesync %s\n", version)
		return nil
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
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("serve accepts no positional arguments")
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return app.Serve(ctx, *stateDir, *peerListen, stdout)
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
			result, err := work.Scan(context.Background(), folder)
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
				fmt.Fprintf(stdout, "peer=%x version=%x:%d receipt=%t remote=%s last-contact=%s\n", progress.Peer, progress.Version.Author, progress.Version.Counter, progress.Receipt, progress.RemoteState, progress.LastContact.UTC().Format(time.RFC3339Nano))
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
		flags := flag.NewFlagSet("approve-deletions", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
		folderText := flags.String("folder", "", "32-byte folder ID in hex")
		token := flags.String("token", "", "deletion preview token")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *token == "" {
			return errors.New("approve-deletions requires --folder and --token")
		}
		folder, err := parseID(*folderText)
		if err != nil {
			return err
		}
		return app.WithWorkspace(context.Background(), *stateDir, func(_ config.Config, _ *repository.DB, work *workspace.Workspace) error {
			created, err := work.ApproveDeletions(context.Background(), folder, *token)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "approved %d deletion versions\n", len(created))
			return nil
		})
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
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: filesync <init|serve|identity|pair-approve|register|scan|sync|status|inspect|approve-deletions|apply|version> [options]")
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
