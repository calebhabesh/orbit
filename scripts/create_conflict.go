//go:build ignore

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <state-dir> <folder-hex>\n", os.Args[0])
		os.Exit(1)
	}
	stateDir := os.Args[1]
	folderHex := os.Args[2]
	var folder history.ID
	b, err := hex.DecodeString(folderHex)
	if err != nil {
		panic(err)
	}
	copy(folder[:], b)

	ctx := context.Background()
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var remoteDevice history.ID
	remoteDevice[0] = 0xFE
	remoteDevice[1] = 0xDC
	remoteDevice[2] = 0xBA

	content := []byte("# Architecture Overview\n\nConcurrently updated architecture proposal from remote laptop peer.\n")
	h := history.Digest(sha256.Sum256(content))
	manifest := &history.Manifest{
		Size:       uint64(len(content)),
		Digest:     h,
		Executable: false,
		Chunks: []history.Chunk{
			{Digest: h, Length: uint64(len(content))},
		},
	}
	if err := db.InstallChunk(ctx, h, uint64(len(content)), bytes.NewReader(content)); err != nil {
		panic(err)
	}
	env := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: remoteDevice, Counter: 1},
		Path:             "architecture.md",
		Parents:          nil,
		Vector:           []history.ClockEntry{{Author: remoteDevice, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := db.ImportMetadata(ctx, env); err != nil {
		panic(err)
	}
	if err := db.MarkContentReady(ctx, env.ID); err != nil {
		panic(err)
	}
	fmt.Println("Concurrent conflict created successfully for architecture.md")
}
