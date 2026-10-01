package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type StorageLimits struct {
	FormatVersion         int    `json:"format_version"`
	DataBudgetBytes       uint64 `json:"data_budget_bytes"`
	MetadataBudgetBytes   uint64 `json:"metadata_budget_bytes"`
	FreeSpaceReserveBytes uint64 `json:"free_space_reserve_bytes"`
}

func DefaultStorageLimits() StorageLimits {
	return StorageLimits{FormatVersion: 1, DataBudgetBytes: 10 * 1024 * 1024 * 1024, MetadataBudgetBytes: 256 * 1024 * 1024, FreeSpaceReserveBytes: 512 * 1024 * 1024}
}

func LoadStorageLimits(stateDir string) (StorageLimits, error) {
	path := filepath.Join(stateDir, "limits.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return StorageLimits{}, nil
	}
	if err != nil {
		return StorageLimits{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return StorageLimits{}, errors.New("limits.json must be a private regular file at most 4 KiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return StorageLimits{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var limits StorageLimits
	if err := decoder.Decode(&limits); err != nil {
		return limits, err
	}
	if decoder.Decode(new(any)) != io.EOF || limits.FormatVersion != 1 || limits.DataBudgetBytes == 0 || limits.MetadataBudgetBytes == 0 || limits.FreeSpaceReserveBytes == 0 {
		return limits, errors.New("storage limits require version 1 and positive finite budgets/reserve")
	}
	return limits, nil
}

func InitializeStorageLimits(stateDir string) error {
	limits, err := LoadStorageLimits(stateDir)
	if err != nil || limits.FormatVersion != 0 {
		return err
	}
	data, err := json.MarshalIndent(DefaultStorageLimits(), "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(stateDir, "limits.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	d, err := os.Open(stateDir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
