package faults

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

var p06Folder, p06SenderDev, p06ReceiverDev = faultID('F'), faultID('S'), faultID('R')

func TestP06TransferKillRestartBoundaries(t *testing.T) {
	for _, hook := range []string{
		replication.HookChunkVerified,
		replication.HookBeforeReady,
		replication.HookAfterReady,
		replication.HookBeforeReceipt,
		replication.HookAfterReceipt,
	} {
		t.Run(hook, func(t *testing.T) {
			root := testkit.NewDisposable(t)
			senderDir := filepath.Join(root, "sender")
			receiverDir := filepath.Join(root, "receiver")
			if err := os.Mkdir(senderDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(receiverDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(root, receiverDir); err != nil {
				t.Fatal(err)
			}

			now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
			receiverID, err := replication.LoadOrCreateIdentity(filepath.Join(receiverDir, "identity"), p06ReceiverDev, now)
			if err != nil {
				t.Fatal(err)
			}

			// Setup sender server and seed a multi-chunk file
			serverURL, serverCertPath, cancelServer := setupP06Sender(t, senderDir, receiverID.KeyPin)
			defer cancelServer()

			// Prepare receiver state
			setupP06Receiver(t, receiverDir, receiverID, serverCertPath)

			cmd := exec.Command(os.Args[0], "-test.run=^TestP06BoundaryHelper$")
			cmd.Env = append(
				os.Environ(),
				"FILESYNC_P06_HELPER=1",
				"FILESYNC_P06_RECEIVER="+receiverDir,
				"FILESYNC_P06_SERVER_URL="+serverURL,
				"FILESYNC_P06_SERVER_CERT="+serverCertPath,
				"FILESYNC_P06_HOOK="+hook,
			)
			output, err := cmd.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("helper failed to execute: %v\n%s", err, output)
			}
			status, ok := exitErr.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatalf("helper was not killed by SIGKILL at %s: exitErr=%v status=%+v\n%s", hook, exitErr, status, output)
			}

			// Verify post-crash state in receiver repository
			db, err := repository.Open(context.Background(), filepath.Join(receiverDir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			versionID := history.VersionID{Folder: p06Folder, Author: p06SenderDev, Counter: 1}
			ready, err := db.ContentReady(context.Background(), versionID)
			if err != nil {
				t.Fatal(err)
			}

			switch hook {
			case replication.HookChunkVerified, replication.HookBeforeReady:
				if ready {
					t.Fatalf("version should not be ready when killed at %s", hook)
				}
				positions, err := db.TransferVerifiedPositions(context.Background(), stableTransferID(p06SenderDev, versionID))
				if err != nil {
					t.Fatal(err)
				}
				if len(positions) == 0 {
					t.Fatalf("expected at least one verified chunk position when killed at %s", hook)
				}
			case replication.HookAfterReady, replication.HookBeforeReceipt, replication.HookAfterReceipt:
				if !ready {
					t.Fatalf("version should be ready when killed at %s", hook)
				}
			}

			// Resume transfer without fault hook; must complete safely and reuse verified chunks
			resumeReceiver(t, receiverDir, serverURL, serverCertPath)
		})
	}
}

func setupP06Sender(t *testing.T, dir string, receiverPin history.Digest) (string, string, func()) {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	identity, err := replication.LoadOrCreateIdentity(filepath.Join(dir, "identity"), p06SenderDev, now)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.Open(context.Background(), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.EnsureFolder(context.Background(), p06Folder, p06SenderDev, 1); err != nil {
		t.Fatal(err)
	}

	membership := protocol.Membership{
		Folder:   p06Folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: p06SenderDev, KeyPin: identity.KeyPin},
			{Device: p06ReceiverDev, KeyPin: receiverPin},
		},
	}
	if _, err := repo.ApproveMembership(context.Background(), membership); err != nil {
		t.Fatal(err)
	}

	// Create a 2-chunk file (2 * 1 MiB)
	content := bytes.Repeat([]byte("F"), int(2*history.ChunkSize))
	manifest, err := repo.StoreFile(context.Background(), bytes.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{
		Folder:           p06Folder,
		Path:             "fault-test.bin",
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := replication.NewServer(repo, identity)
	go func() { _ = server.Serve(ctx, listener) }()

	certPath := filepath.Join(dir, "sender.pem")
	if err := os.WriteFile(certPath, identity.CertificatePEM(), 0o600); err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		cancel()
		_ = listener.Close()
		_ = repo.Close()
	}
	return "https://" + listener.Addr().String(), certPath, cleanup
}

func setupP06Receiver(t *testing.T, dir string, receiverID replication.Identity, senderCertPath string) {
	t.Helper()
	repo, err := repository.Open(context.Background(), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.EnsureFolder(context.Background(), p06Folder, p06ReceiverDev, 1); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	work := workspace.New(repo, workspace.Options{})
	if _, err := work.Register(context.Background(), p06Folder, root); err != nil {
		t.Fatal(err)
	}

	senderCertPEM, err := os.ReadFile(senderCertPath)
	if err != nil {
		t.Fatal(err)
	}
	senderCert, err := replication.ParsePeerCertificate(senderCertPEM)
	if err != nil {
		t.Fatal(err)
	}
	membership := protocol.Membership{
		Folder:   p06Folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: p06SenderDev, KeyPin: replication.PublicKeyPin(senderCert)},
			{Device: p06ReceiverDev, KeyPin: receiverID.KeyPin},
		},
	}
	if _, err := repo.ApproveMembership(context.Background(), membership); err != nil {
		t.Fatal(err)
	}
}

func resumeReceiver(t *testing.T, dir, serverURL, senderCertPath string) {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	receiverID, err := replication.LoadOrCreateIdentity(filepath.Join(dir, "identity"), p06ReceiverDev, now)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.Open(context.Background(), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	senderCertPEM, err := os.ReadFile(senderCertPath)
	if err != nil {
		t.Fatal(err)
	}
	senderCert, err := replication.ParsePeerCertificate(senderCertPEM)
	if err != nil {
		t.Fatal(err)
	}
	client, err := replication.NewClient(serverURL, receiverID, senderCert, replication.PublicKeyPin(senderCert))
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	membership, err := repo.Membership(context.Background(), p06Folder)
	if err != nil {
		t.Fatal(err)
	}
	work := workspace.New(repo, workspace.Options{})
	syncer := replication.NewSyncer(repo, work, client, p06ReceiverDev, p06SenderDev, p06Folder, membership, replication.TransferOptions{})
	res, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("resume sync failed: %v", err)
	}
	if res.VersionsApplied != 1 {
		t.Fatalf("expected 1 version applied on resume, got %+v", res)
	}
}

func TestP06BoundaryHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P06_HELPER") != "1" {
		return
	}
	receiverDir := os.Getenv("FILESYNC_P06_RECEIVER")
	serverURL := os.Getenv("FILESYNC_P06_SERVER_URL")
	serverCertPath := os.Getenv("FILESYNC_P06_SERVER_CERT")
	hook := os.Getenv("FILESYNC_P06_HOOK")

	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	receiverID, err := replication.LoadOrCreateIdentity(filepath.Join(receiverDir, "identity"), p06ReceiverDev, now)
	if err != nil {
		os.Exit(90)
	}
	repo, err := repository.Open(context.Background(), filepath.Join(receiverDir, "state"))
	if err != nil {
		os.Exit(91)
	}
	defer repo.Close()

	senderCertPEM, err := os.ReadFile(serverCertPath)
	if err != nil {
		os.Exit(92)
	}
	senderCert, err := replication.ParsePeerCertificate(senderCertPEM)
	if err != nil {
		os.Exit(93)
	}
	client, err := replication.NewClient(serverURL, receiverID, senderCert, replication.PublicKeyPin(senderCert))
	if err != nil {
		os.Exit(94)
	}
	defer client.CloseIdleConnections()

	membership, err := repo.Membership(context.Background(), p06Folder)
	if err != nil {
		os.Exit(95)
	}
	work := workspace.New(repo, workspace.Options{})

	syncer := replication.NewSyncer(repo, work, client, p06ReceiverDev, p06SenderDev, p06Folder, membership, replication.TransferOptions{
		Hook: func(name string) error {
			if name == hook {
				_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			return nil
		},
	})

	_, _ = syncer.Sync(context.Background())
	os.Exit(99) // Should not reach here if killed
}

func stableTransferID(peer history.ID, id history.VersionID) string {
	h := sha256.New()
	h.Write([]byte("filesync-transfer-v1\x00"))
	h.Write(peer[:])
	h.Write(id.Folder[:])
	h.Write(id.Author[:])
	h.Write([]byte(strconv.FormatUint(id.Counter, 10)))
	return "transfer-" + hex.EncodeToString(h.Sum(nil))
}
