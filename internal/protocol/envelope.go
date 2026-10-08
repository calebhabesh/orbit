// Package protocol owns bounded wire encodings. It converts wire data into the
// pure history domain but does not admit ancestry or perform transport IO.
package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

type wireVersionID struct {
	FolderID string `json:"folder_id"`
	AuthorID string `json:"author_id"`
	Counter  string `json:"counter"`
}
type wireClock struct {
	AuthorID string `json:"author_id"`
	Counter  string `json:"counter"`
}
type wireChunk struct {
	Digest string `json:"digest"`
	Length string `json:"length"`
}
type wireManifest struct {
	Size       string      `json:"size"`
	Digest     string      `json:"digest"`
	Chunks     []wireChunk `json:"chunks"`
	Executable bool        `json:"executable"`
}
type wireEnvelope struct {
	ProtocolVersion  string          `json:"protocol_version"`
	FolderID         string          `json:"folder_id"`
	Path             string          `json:"path"`
	AuthorID         string          `json:"author_id"`
	Counter          string          `json:"counter"`
	Parents          []wireVersionID `json:"parents"`
	Vector           []wireClock     `json:"vector"`
	Kind             string          `json:"kind"`
	Manifest         *wireManifest   `json:"manifest,omitempty"`
	AuthoredRevision string          `json:"authored_revision"`
	DisplayTime      string          `json:"display_time,omitempty"`
}

func DecodeEnvelope(data []byte) (history.Envelope, error) {
	if err := RejectDuplicateKeys(data); err != nil {
		return history.Envelope{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireEnvelope
	if err := decoder.Decode(&wire); err != nil {
		return history.Envelope{}, fmt.Errorf("decode version envelope: %w", err)
	}
	if err := expectEOF(decoder); err != nil {
		return history.Envelope{}, err
	}
	if wire.ProtocolVersion != "1" {
		return history.Envelope{}, errors.New("unsupported protocol version")
	}
	folder, err := parseID(wire.FolderID)
	if err != nil {
		return history.Envelope{}, fmt.Errorf("folder_id: %w", err)
	}
	author, err := parseID(wire.AuthorID)
	if err != nil {
		return history.Envelope{}, fmt.Errorf("author_id: %w", err)
	}
	counter, err := parseUint(wire.Counter, false)
	if err != nil {
		return history.Envelope{}, fmt.Errorf("counter: %w", err)
	}
	revision, err := parseUint(wire.AuthoredRevision, false)
	if err != nil {
		return history.Envelope{}, fmt.Errorf("authored_revision: %w", err)
	}
	result := history.Envelope{ID: history.VersionID{Folder: folder, Author: author, Counter: counter}, Path: wire.Path, AuthoredRevision: revision, DisplayTime: wire.DisplayTime}
	if wire.DisplayTime != "" {
		if _, err := time.Parse(time.RFC3339Nano, wire.DisplayTime); err != nil {
			return history.Envelope{}, fmt.Errorf("display_time: %w", err)
		}
	}
	result.Parents = make([]history.VersionID, len(wire.Parents))
	for i, parent := range wire.Parents {
		parentFolder, err := parseID(parent.FolderID)
		if err != nil {
			return history.Envelope{}, err
		}
		parentAuthor, err := parseID(parent.AuthorID)
		if err != nil {
			return history.Envelope{}, err
		}
		parentCounter, err := parseUint(parent.Counter, false)
		if err != nil {
			return history.Envelope{}, err
		}
		result.Parents[i] = history.VersionID{Folder: parentFolder, Author: parentAuthor, Counter: parentCounter}
	}
	result.Vector = make([]history.ClockEntry, len(wire.Vector))
	for i, entry := range wire.Vector {
		entryAuthor, err := parseID(entry.AuthorID)
		if err != nil {
			return history.Envelope{}, err
		}
		entryCounter, err := parseUint(entry.Counter, false)
		if err != nil {
			return history.Envelope{}, err
		}
		result.Vector[i] = history.ClockEntry{Author: entryAuthor, Counter: entryCounter}
	}
	switch wire.Kind {
	case "file":
		result.Kind = history.KindFile
	case "directory":
		result.Kind = history.KindDirectory
	case "tombstone":
		result.Kind = history.KindTombstone
	default:
		return history.Envelope{}, errors.New("unknown version kind")
	}
	if wire.Manifest != nil {
		size, err := parseUint(wire.Manifest.Size, true)
		if err != nil {
			return history.Envelope{}, err
		}
		digest, err := parseDigest(wire.Manifest.Digest)
		if err != nil {
			return history.Envelope{}, err
		}
		manifest := &history.Manifest{Size: size, Digest: digest, Executable: wire.Manifest.Executable, Chunks: make([]history.Chunk, len(wire.Manifest.Chunks))}
		for i, chunk := range wire.Manifest.Chunks {
			chunkDigest, err := parseDigest(chunk.Digest)
			if err != nil {
				return history.Envelope{}, err
			}
			length, err := parseUint(chunk.Length, false)
			if err != nil {
				return history.Envelope{}, err
			}
			manifest.Chunks[i] = history.Chunk{Digest: chunkDigest, Length: length}
		}
		result.Manifest = manifest
	}
	return result, nil
}

// EncodeEnvelope emits the frozen v1 wire representation. Integer values are
// strings so JavaScript consumers cannot truncate uint64 counters or sizes.
func EncodeEnvelope(envelope history.Envelope) ([]byte, error) {
	wire := wireEnvelope{
		ProtocolVersion:  "1",
		FolderID:         hex.EncodeToString(envelope.ID.Folder[:]),
		Path:             envelope.Path,
		AuthorID:         hex.EncodeToString(envelope.ID.Author[:]),
		Counter:          strconv.FormatUint(envelope.ID.Counter, 10),
		Parents:          make([]wireVersionID, len(envelope.Parents)),
		Vector:           make([]wireClock, len(envelope.Vector)),
		AuthoredRevision: strconv.FormatUint(envelope.AuthoredRevision, 10),
		DisplayTime:      envelope.DisplayTime,
	}
	for i, parent := range envelope.Parents {
		wire.Parents[i] = wireVersionID{FolderID: hex.EncodeToString(parent.Folder[:]), AuthorID: hex.EncodeToString(parent.Author[:]), Counter: strconv.FormatUint(parent.Counter, 10)}
	}
	for i, entry := range envelope.Vector {
		wire.Vector[i] = wireClock{AuthorID: hex.EncodeToString(entry.Author[:]), Counter: strconv.FormatUint(entry.Counter, 10)}
	}
	switch envelope.Kind {
	case history.KindFile:
		wire.Kind = "file"
	case history.KindDirectory:
		wire.Kind = "directory"
	case history.KindTombstone:
		wire.Kind = "tombstone"
	default:
		return nil, errors.New("unknown version kind")
	}
	if envelope.Manifest != nil {
		wire.Manifest = &wireManifest{Size: strconv.FormatUint(envelope.Manifest.Size, 10), Digest: hex.EncodeToString(envelope.Manifest.Digest[:]), Executable: envelope.Manifest.Executable, Chunks: make([]wireChunk, len(envelope.Manifest.Chunks))}
		for i, chunk := range envelope.Manifest.Chunks {
			wire.Manifest.Chunks[i] = wireChunk{Digest: hex.EncodeToString(chunk.Digest[:]), Length: strconv.FormatUint(chunk.Length, 10)}
		}
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	if _, err := DecodeEnvelope(data); err != nil {
		return nil, fmt.Errorf("encode invalid envelope: %w", err)
	}
	return data, nil
}

func parseID(value string) (history.ID, error) {
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != value {
		return history.ID{}, errors.New("expected 64 lowercase hexadecimal characters")
	}
	var result history.ID
	copy(result[:], raw)
	return result, nil
}
func parseDigest(value string) (history.Digest, error) {
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != value {
		return history.Digest{}, errors.New("expected 64 lowercase hexadecimal characters")
	}
	var result history.Digest
	copy(result[:], raw)
	return result, nil
}
func parseUint(value string, zeroAllowed bool) (uint64, error) {
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || strconv.FormatUint(parsed, 10) != value || (!zeroAllowed && parsed == 0) {
		return 0, errors.New("expected canonical unsigned decimal string")
	}
	return parsed, nil
}
func expectEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("version envelope contains trailing JSON data")
	}
	return nil
}

// RejectDuplicateKeys recursively rejects ambiguous JSON objects before they
// are decoded into Go structs.
func RejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkJSON(decoder, first); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("version envelope contains trailing JSON data")
	}
	return nil
}

// DecodeStrict applies the common wire rules used by endpoint wrappers.
func DecodeStrict(data []byte, value any) error {
	if err := RejectDuplicateKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return expectEOF(decoder)
}
func walkJSON(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = true
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSON(decoder, value); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSON(decoder, value); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	_, err := decoder.Token()
	return err
}
