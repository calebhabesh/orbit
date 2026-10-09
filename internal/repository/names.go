package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/calebhabesh/orbit/internal/history"
)

// MaxSharedName bounds a shared Orbit or device name in bytes.
const MaxSharedName = 128

// ErrInvalidName rejects empty, oversized or control-character names.
var ErrInvalidName = errors.New("name must be 1 to 128 bytes of printable UTF-8")

// NameRecord is a shared name: the last writer by (Clock, Author) wins on
// every device. Clock 0 marks a local name that is never sent.
type NameRecord struct {
	ID     history.ID
	Name   string
	Clock  uint64
	Author history.ID
}

// Newer reports whether r supersedes other.
func (r NameRecord) Newer(other NameRecord) bool {
	if r.Clock != other.Clock {
		return r.Clock > other.Clock
	}
	return bytes.Compare(r.Author[:], other.Author[:]) > 0
}

// ValidName reports whether name may be stored or shared.
func ValidName(name string) error {
	if name == "" || len(name) > MaxSharedName || !utf8.ValidString(name) || strings.TrimSpace(name) != name {
		return ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return ErrInvalidName
		}
	}
	return nil
}

func scanRecord(row *sql.Row, id history.ID) (NameRecord, error) {
	r := NameRecord{ID: id}
	var name sql.NullString
	var clock int64
	var author []byte
	if err := row.Scan(&name, &clock, &author); err != nil {
		return r, err
	}
	r.Name, r.Clock = name.String, uint64(clock)
	copy(r.Author[:], author)
	return r, nil
}

// FolderNameRecord returns the folder's shared name record.
func (db *DB) FolderNameRecord(ctx context.Context, folder history.ID) (NameRecord, error) {
	return scanRecord(db.db.QueryRowContext(ctx, `SELECT display_name,name_clock,name_author FROM folders WHERE folder_id=?`, folder[:]), folder)
}

// DeviceNameRecord returns a device's shared name record (empty when unknown).
func (db *DB) DeviceNameRecord(ctx context.Context, device history.ID) (NameRecord, error) {
	r, err := scanRecord(db.db.QueryRowContext(ctx, `SELECT display_name,name_clock,name_author FROM devices WHERE device_id=?`, device[:]), device)
	if errors.Is(err, sql.ErrNoRows) {
		return NameRecord{ID: device}, nil
	}
	return r, err
}

// RenameFolder sets the folder's name as the owner's latest choice, to be
// shared with its members. An unchanged, already shared name is left alone.
func (db *DB) RenameFolder(ctx context.Context, folder history.ID, name string, author history.ID) error {
	if err := ValidName(name); err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	current, err := db.FolderNameRecord(ctx, folder)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("folder %s not found", folder)
		}
		return err
	}
	if current.Name == name && current.Clock > 0 {
		return nil
	}
	_, err = db.db.ExecContext(ctx, `UPDATE folders SET display_name=?,name_clock=?,name_author=? WHERE folder_id=?`, name, int64(current.Clock+1), author[:], folder[:])
	return err
}

// RenameDevice sets a device's name as the owner's latest choice.
func (db *DB) RenameDevice(ctx context.Context, device history.ID, name string, author history.ID) error {
	return db.RenameDeviceAtLeast(ctx, device, name, author, 0)
}

// RenameDeviceAtLeast is RenameDevice with a minimum clock. An owner's name
// given while approving a device uses 2, so it outranks the name that device
// gave itself at join (clock 1) once the two meet.
func (db *DB) RenameDeviceAtLeast(ctx context.Context, device history.ID, name string, author history.ID, minClock uint64) error {
	if err := ValidName(name); err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	current, err := db.DeviceNameRecord(ctx, device)
	if err != nil {
		return err
	}
	if current.Name == name && current.Clock >= max(minClock, 1) {
		return nil
	}
	_, err = db.db.ExecContext(ctx, `INSERT INTO devices(device_id,display_name,is_local,name_clock,name_author) VALUES(?,?,0,?,?)
ON CONFLICT(device_id) DO UPDATE SET display_name=excluded.display_name,name_clock=excluded.name_clock,name_author=excluded.name_author`, device[:], name, int64(max(current.Clock+1, minClock)), author[:])
	return err
}

// MergeFolderName installs a peer's record when it is newer.
func (db *DB) MergeFolderName(ctx context.Context, r NameRecord) (bool, error) {
	if r.Clock == 0 || ValidName(r.Name) != nil || r.Clock > 1<<62 {
		return false, ErrInvalidName
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	current, err := db.FolderNameRecord(ctx, r.ID)
	if err != nil || !r.Newer(current) {
		return false, err
	}
	_, err = db.db.ExecContext(ctx, `UPDATE folders SET display_name=?,name_clock=?,name_author=? WHERE folder_id=?`, r.Name, int64(r.Clock), r.Author[:], r.ID[:])
	return err == nil, err
}

// MergeDeviceName installs a peer's record for a device when it is newer.
func (db *DB) MergeDeviceName(ctx context.Context, r NameRecord) (bool, error) {
	if r.Clock == 0 || ValidName(r.Name) != nil || r.Clock > 1<<62 {
		return false, ErrInvalidName
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	current, err := db.DeviceNameRecord(ctx, r.ID)
	if err != nil || !r.Newer(current) {
		return false, err
	}
	_, err = db.db.ExecContext(ctx, `INSERT INTO devices(device_id,display_name,is_local,name_clock,name_author) VALUES(?,?,0,?,?)
ON CONFLICT(device_id) DO UPDATE SET display_name=excluded.display_name,name_clock=excluded.name_clock,name_author=excluded.name_author`, r.ID[:], r.Name, int64(r.Clock), r.Author[:])
	return err == nil, err
}
