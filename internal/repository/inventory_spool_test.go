package repository

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestInventorySpoolBudgetAndCleanup(t *testing.T) {
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.metadataBudgetBytes = 8
	spool, err := db.NewInventorySpool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spool.Write([]byte("12345678")); err != nil {
		t.Fatal(err)
	}
	if _, err := spool.Write([]byte("9")); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("unbounded spool: %v", err)
	}
	usage, err := db.StorageUsage(context.Background())
	if err != nil || usage.Incoming != 8 {
		t.Fatalf("incoming accounting: %+v %v", usage, err)
	}
	if err := spool.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(spool.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spool remained: %v", err)
	}
}
