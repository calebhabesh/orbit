package replication

import (
	"context"
	"testing"

	"github.com/calebhabesh/orbit/internal/repository"
)

func nameOf(t *testing.T, get func() (repository.NameRecord, error)) string {
	t.Helper()
	r, err := get()
	if err != nil {
		t.Fatal(err)
	}
	return r.Name
}

// Owner request: an Orbit's name and its devices' names are the same on every
// device, and a later rename on any device wins everywhere.
func TestSyncSharesOrbitAndDeviceNames(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()
	sender, receiver := fix.senderID.DeviceID, fix.receiverID.DeviceID
	// Created on the sender; each device names itself; the receiver only has
	// the invitation's (unshared) name and an enrollment label.
	if err := fix.senderRepo.RenameFolder(ctx, fix.folder, "Demo", sender); err != nil {
		t.Fatal(err)
	}
	if err := fix.senderRepo.RenameDevice(ctx, sender, "CalebPC", sender); err != nil {
		t.Fatal(err)
	}
	if err := fix.receiverRepo.RenameDevice(ctx, receiver, "CalebLaptop", receiver); err != nil {
		t.Fatal(err)
	}
	if err := fix.receiverRepo.SetFolderDisplayName(ctx, fix.folder, "Old invitation name"); err != nil {
		t.Fatal(err)
	}
	if err := fix.senderRepo.SetDeviceDisplayName(ctx, receiver, "label from request"); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := nameOf(t, func() (repository.NameRecord, error) { return fix.receiverRepo.FolderNameRecord(ctx, fix.folder) }); got != "Demo" {
		t.Fatalf("receiver folder name %q", got)
	}
	if got := nameOf(t, func() (repository.NameRecord, error) { return fix.receiverRepo.DeviceNameRecord(ctx, sender) }); got != "CalebPC" {
		t.Fatalf("receiver sees sender as %q", got)
	}
	if got := nameOf(t, func() (repository.NameRecord, error) { return fix.senderRepo.DeviceNameRecord(ctx, receiver) }); got != "CalebLaptop" {
		t.Fatalf("sender sees receiver as %q", got)
	}
	// A rename on the receiver reaches the sender in the same exchange.
	if err := fix.receiverRepo.RenameFolder(ctx, fix.folder, "Projects", receiver); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := nameOf(t, func() (repository.NameRecord, error) { return fix.senderRepo.FolderNameRecord(ctx, fix.folder) }); got != "Projects" {
		t.Fatalf("sender folder name after rename %q", got)
	}
	// A local label never replaces a shared name.
	if err := fix.senderRepo.SetFolderDisplayName(ctx, fix.folder, "local"); err != nil {
		t.Fatal(err)
	}
	if got := nameOf(t, func() (repository.NameRecord, error) { return fix.senderRepo.FolderNameRecord(ctx, fix.folder) }); got != "Projects" {
		t.Fatalf("local label replaced the shared name: %q", got)
	}
}

func TestNameRecordsRejectBadInput(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()
	for _, bad := range []string{"", " padded", "line\nbreak", "esc\x1b[31m", string(make([]byte, 200))} {
		if err := fix.senderRepo.RenameFolder(ctx, fix.folder, bad, fix.senderID.DeviceID); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	// Concurrent renames with equal clocks resolve by author, the same way on both sides.
	a := repository.NameRecord{ID: fix.folder, Name: "A", Clock: 5, Author: fix.senderID.DeviceID}
	b := repository.NameRecord{ID: fix.folder, Name: "B", Clock: 5, Author: fix.receiverID.DeviceID}
	if a.Newer(b) == b.Newer(a) {
		t.Fatal("tie is not broken deterministically")
	}
}

// A name the owner gives while approving a device outranks the name that
// device chose for itself at join, on both devices.
func TestApprovalNameOutranksJoinerName(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()
	sender, receiver := fix.senderID.DeviceID, fix.receiverID.DeviceID
	if err := fix.receiverRepo.RenameDevice(ctx, receiver, "raspberrypi", receiver); err != nil {
		t.Fatal(err)
	}
	if err := fix.senderRepo.RenameDeviceAtLeast(ctx, receiver, "Pi", sender, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	for _, repo := range []*repository.DB{fix.senderRepo, fix.receiverRepo} {
		if got := nameOf(t, func() (repository.NameRecord, error) { return repo.DeviceNameRecord(ctx, receiver) }); got != "Pi" {
			t.Fatalf("device name %q, want the owner's approval name", got)
		}
	}
}
