package replication

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

const (
	ProtocolVersion  = "1"
	MaxMetadataBytes = int64(8 << 20)
	MaxInventoryPage = 128
	MaxVersionBatch  = 128
)

type FolderHandshake struct {
	FolderID         string `json:"folder_id"`
	Revision         string `json:"membership_revision"`
	MembershipDigest string `json:"membership_digest"`
}

type Limits struct {
	MetadataBytes string `json:"metadata_bytes"`
	InventoryPage string `json:"inventory_page"`
}

type HelloRequest struct {
	ProtocolVersion string            `json:"protocol_version"`
	DeviceID        string            `json:"device_id"`
	Folders         []FolderHandshake `json:"folders"`
	Limits          Limits            `json:"limits"`
}

type HelloResponse struct {
	ProtocolVersion string            `json:"protocol_version"`
	DeviceID        string            `json:"device_id"`
	Folders         []FolderHandshake `json:"folders"`
	Limits          Limits            `json:"limits"`
}

type InventoryRequest struct {
	ProtocolVersion  string `json:"protocol_version"`
	DeviceID         string `json:"device_id"`
	FolderID         string `json:"folder_id"`
	Revision         string `json:"membership_revision"`
	MembershipDigest string `json:"membership_digest"`
	SnapshotToken    string `json:"snapshot_token,omitempty"`
	Cursor           string `json:"cursor"`
	PageSize         string `json:"page_size"`
}

type VersionIDWire struct {
	AuthorID string `json:"author_id"`
	Counter  string `json:"counter"`
}

type InventoryEntry struct {
	VersionIDWire
	Path           string `json:"path"`
	Kind           string `json:"kind"`
	Availability   string `json:"availability"`
	EnvelopeDigest string `json:"envelope_digest"`
}

type InventoryResponse struct {
	ProtocolVersion string           `json:"protocol_version"`
	SnapshotToken   string           `json:"snapshot_token"`
	NextCursor      string           `json:"next_cursor"`
	Done            bool             `json:"done"`
	Entries         []InventoryEntry `json:"entries"`
}

type VersionsRequest struct {
	ProtocolVersion  string          `json:"protocol_version"`
	DeviceID         string          `json:"device_id"`
	FolderID         string          `json:"folder_id"`
	Revision         string          `json:"membership_revision"`
	MembershipDigest string          `json:"membership_digest"`
	Versions         []VersionIDWire `json:"versions"`
}

type VersionsResponse struct {
	ProtocolVersion string            `json:"protocol_version"`
	Envelopes       []json.RawMessage `json:"envelopes"`
}

type ChunkRequest struct {
	ProtocolVersion  string `json:"protocol_version"`
	DeviceID         string `json:"device_id"`
	FolderID         string `json:"folder_id"`
	Revision         string `json:"membership_revision"`
	MembershipDigest string `json:"membership_digest"`
	AuthorID         string `json:"author_id"`
	Counter          string `json:"counter"`
	ChunkIndex       string `json:"chunk_index"`
}

type ReceiptsRequest struct {
	ProtocolVersion  string          `json:"protocol_version"`
	DeviceID         string          `json:"device_id"`
	FolderID         string          `json:"folder_id"`
	Revision         string          `json:"membership_revision"`
	MembershipDigest string          `json:"membership_digest"`
	Versions         []VersionIDWire `json:"versions"`
}

type ReceiptsResponse struct {
	ProtocolVersion string          `json:"protocol_version"`
	Accepted        []VersionIDWire `json:"accepted"`
}

type StatusRequest struct {
	ProtocolVersion  string          `json:"protocol_version"`
	DeviceID         string          `json:"device_id"`
	FolderID         string          `json:"folder_id"`
	Revision         string          `json:"membership_revision"`
	MembershipDigest string          `json:"membership_digest"`
	Versions         []VersionIDWire `json:"versions"`
}

type StatusEntry struct {
	VersionIDWire
	MetadataKnown bool   `json:"metadata_known"`
	ContentState  string `json:"content_state"`
	Stored        bool   `json:"stored"`
	Applied       bool   `json:"applied"`
	Conflict      bool   `json:"conflict"`
	Blocked       bool   `json:"blocked"`
}

type StatusResponse struct {
	ProtocolVersion string        `json:"protocol_version"`
	Entries         []StatusEntry `json:"entries"`
}

type ErrorResponse struct {
	ProtocolVersion string `json:"protocol_version"`
	Code            string `json:"code"`
	Message         string `json:"message"`
	Retryable       bool   `json:"retryable"`
	Action          string `json:"action"`
}

type MembershipGetRequest struct {
	ProtocolVersion string `json:"protocol_version"`
	DeviceID        string `json:"device_id"`
	FolderID        string `json:"folder_id"`
	FromRevision    string `json:"from_revision,omitempty"`
	ExpectedDigest  string `json:"expected_digest,omitempty"`
}

type MembershipGetResponse struct {
	ProtocolVersion string                        `json:"protocol_version"`
	FolderID        string                        `json:"folder_id"`
	Membership      protocol.Membership           `json:"membership"`
	Snapshots       []protocol.RetirementSnapshot `json:"snapshots,omitempty"`
}

func parseID(text string) (history.ID, error) {
	var id history.ID
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != len(id) || hex.EncodeToString(raw) != text {
		return id, errors.New("expected 64 lowercase hexadecimal characters")
	}
	copy(id[:], raw)
	if id == (history.ID{}) {
		return id, errors.New("identity cannot be zero")
	}
	return id, nil
}

func parseDigest(text string) (history.Digest, error) {
	var digest history.Digest
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != len(digest) || hex.EncodeToString(raw) != text {
		return digest, errors.New("expected 64 lowercase hexadecimal characters")
	}
	copy(digest[:], raw)
	return digest, nil
}

func parseDecimal(text string, zero bool) (uint64, error) {
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil || strconv.FormatUint(value, 10) != text || (!zero && value == 0) {
		return 0, errors.New("expected canonical unsigned decimal string")
	}
	return value, nil
}

func kindName(kind history.Kind) string {
	switch kind {
	case history.KindFile:
		return "file"
	case history.KindDirectory:
		return "directory"
	case history.KindTombstone:
		return "tombstone"
	default:
		return "unknown"
	}
}
