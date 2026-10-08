package model_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/model"
)

type fixtureFile struct {
	Format          string        `json:"format"`
	CounterEncoding string        `json:"counter_encoding"`
	Cases           []fixtureCase `json:"cases"`
}

type fixtureCase struct {
	ID              string   `json:"id"`
	Author          string   `json:"author"`
	Counter         string   `json:"counter"`
	Path            string   `json:"path"`
	Parents         []string `json:"parents"`
	Kind            string   `json:"kind"`
	SameBytesAs     string   `json:"same_bytes_as"`
	Executable      bool     `json:"executable"`
	TransportSender string   `json:"transport_sender"`
	ExpectedAuthor  string   `json:"expected_author"`
}

func TestGoldenHistoryFixture(t *testing.T) {
	data, err := os.ReadFile("../schemas/fixtures/history-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture fixtureFile
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Format != "orbit-history-fixtures-v1" {
		t.Fatalf("format=%q", fixture.Format)
	}
	production := history.New()
	oracle := model.New()
	built := map[string]generated{}
	contents := map[string]string{}
	for _, item := range fixture.Cases {
		counter, err := strconv.ParseUint(item.Counter, 10, 64)
		if err != nil || strconv.FormatUint(counter, 10) != item.Counter {
			t.Fatalf("case %s has noncanonical counter %q", item.ID, item.Counter)
		}
		path := item.Path
		if path == "" {
			path = "x"
		}
		parents := make([]generated, len(item.Parents))
		for index, parentID := range item.Parents {
			parent, ok := built[parentID]
			if !ok {
				t.Fatalf("case %s references unavailable parent %s", item.ID, parentID)
			}
			parents[index] = parent
		}
		content := item.ID
		if item.SameBytesAs != "" {
			content = contents[item.SameBytesAs]
		}
		event := buildEvent(id('F'), item.Author[0], counter, path, item.Kind, content, parents)
		event.oracle.ID = item.ID
		event.oracle.Parents = append([]string(nil), item.Parents...)
		if item.Kind == "tombstone" {
			event.production.Kind = history.KindTombstone
			event.production.Manifest = nil
		}
		if item.Executable && event.production.Manifest != nil {
			event.production.Manifest.Executable = true
		}
		if err := production.Accept(event.production); err != nil {
			t.Fatalf("case %s: %v", item.ID, err)
		}
		if err := oracle.Accept(event.oracle, item.Kind != "tombstone"); err != nil {
			t.Fatalf("oracle case %s: %v", item.ID, err)
		}
		if item.TransportSender != "" && item.ExpectedAuthor != item.Author {
			t.Fatalf("forwarded fixture changed author: %s", item.ID)
		}
		built[item.ID] = event
		contents[item.ID] = content
	}
	if got := oracle.Heads("x"); len(got) != 2 || got[0] != "A3" || got[1] != "C2" {
		t.Fatalf("fixture heads=%v, want A3/C2", got)
	}
	golden, err := os.ReadFile("../schemas/fixtures/normalized-heads-v1.txt")
	if err != nil {
		t.Fatal(err)
	}
	gotNormalized := strings.Join(history.NormalizeHeads(production.Heads(id('F'), "x")), "\n") + "\n"
	if gotNormalized != string(golden) {
		t.Fatalf("normalized comparison output changed\ngot:\n%s\nwant:\n%s", gotNormalized, golden)
	}
	if conflicts := production.StructuralConflicts(id('F')); len(conflicts) != 1 {
		t.Fatalf("fixture structural conflicts=%v", conflicts)
	}
}

type wireEnvelope struct {
	ProtocolVersion string                                         `json:"protocol_version"`
	FolderID        string                                         `json:"folder_id"`
	Path            string                                         `json:"path"`
	AuthorID        string                                         `json:"author_id"`
	Counter         string                                         `json:"counter"`
	Parents         []struct{ FolderID, AuthorID, Counter string } `json:"parents"`
	Vector          []struct {
		AuthorID string `json:"author_id"`
		Counter  string `json:"counter"`
	} `json:"vector"`
	Kind     string `json:"kind"`
	Manifest *struct {
		Size       string            `json:"size"`
		Digest     string            `json:"digest"`
		Chunks     []json.RawMessage `json:"chunks"`
		Executable bool              `json:"executable"`
	} `json:"manifest"`
	AuthoredRevision string `json:"authored_revision"`
}

func TestGoldenWireEnvelopeMapsToDomain(t *testing.T) {
	data, err := os.ReadFile("../schemas/fixtures/version-envelope-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireEnvelope
	if err := decoder.Decode(&wire); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("trailing wire data: %v", err)
	}
	parseID := func(value string) history.ID {
		raw, err := hex.DecodeString(value)
		if err != nil || len(raw) != 32 {
			t.Fatalf("invalid fixed ID %q", value)
		}
		var result history.ID
		copy(result[:], raw)
		return result
	}
	parseCounter := func(value string) uint64 {
		result, err := strconv.ParseUint(value, 10, 64)
		if err != nil || strconv.FormatUint(result, 10) != value || result == 0 {
			t.Fatalf("invalid canonical counter %q", value)
		}
		return result
	}
	vector := make([]history.ClockEntry, len(wire.Vector))
	for index, entry := range wire.Vector {
		vector[index] = history.ClockEntry{Author: parseID(entry.AuthorID), Counter: parseCounter(entry.Counter)}
	}
	digestBytes, err := hex.DecodeString(wire.Manifest.Digest)
	if err != nil || len(digestBytes) != 32 {
		t.Fatal("invalid manifest digest")
	}
	var digest history.Digest
	copy(digest[:], digestBytes)
	envelope := history.Envelope{ID: history.VersionID{Folder: parseID(wire.FolderID), Author: parseID(wire.AuthorID), Counter: parseCounter(wire.Counter)}, Path: wire.Path, Vector: vector, Kind: history.KindFile, Manifest: &history.Manifest{Size: 0, Digest: digest, Executable: wire.Manifest.Executable}, AuthoredRevision: parseCounter(wire.AuthoredRevision)}
	if wire.ProtocolVersion != "1" || wire.Kind != "file" {
		t.Fatal("unexpected golden wire discriminators")
	}
	h := history.New()
	if err := h.Accept(envelope); err != nil {
		t.Fatalf("golden wire envelope: %v", err)
	}
}
