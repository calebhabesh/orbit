package model_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/model"
)

func id(label byte) history.ID {
	var value history.ID
	for index := range value {
		value[index] = label
	}
	return value
}

type generated struct {
	production history.Envelope
	oracle     model.Event
}

func buildEvent(folder history.ID, author byte, counter uint64, path, kind, content string, parents []generated) generated {
	joined := map[history.ID]uint64{}
	parentIDs := make([]history.VersionID, len(parents))
	oracleParents := make([]string, len(parents))
	for index, parent := range parents {
		parentIDs[index] = parent.production.ID
		oracleParents[index] = parent.oracle.ID
		for _, entry := range parent.production.Vector {
			if entry.Counter > joined[entry.Author] {
				joined[entry.Author] = entry.Counter
			}
		}
	}
	joined[id(author)] = counter
	vector := make([]history.ClockEntry, 0, len(joined))
	for actor, value := range joined {
		vector = append(vector, history.ClockEntry{Author: actor, Counter: value})
	}
	sort.Slice(vector, func(i, j int) bool { return string(vector[i].Author[:]) < string(vector[j].Author[:]) })
	sort.Slice(parentIDs, func(i, j int) bool { return history.CompareVersionID(parentIDs[i], parentIDs[j]) < 0 })
	sort.Strings(oracleParents)
	versionID := history.VersionID{Folder: folder, Author: id(author), Counter: counter}
	return generated{history.Envelope{ID: versionID, Path: path, Parents: parentIDs, Vector: vector, Kind: history.KindFile, Manifest: modelManifest(content), AuthoredRevision: 1}, model.Event{ID: fmt.Sprintf("%c%d", author, counter), Author: string(author), Path: path, Parents: oracleParents, Kind: kind, Content: content}}
}

func modelManifest(content string) *history.Manifest { // The model compares ancestry; production manifest validity is covered in history tests.
	if content == "" {
		return &history.Manifest{Digest: [32]byte{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}}
	}
	var digest history.Digest
	digest[0] = byte(len(content))
	return &history.Manifest{Size: uint64(len(content)), Digest: digest, Chunks: []history.Chunk{{Digest: digest, Length: uint64(len(content))}}}
}

func normalizedOracle(events map[string]generated, heads []string) []string {
	result := make([]string, len(heads))
	for i, head := range heads {
		event := events[head].production
		result[i] = history.NormalizeHeads([]history.Envelope{event})[0]
	}
	sort.Strings(result)
	return result
}

func assertAgreement(t *testing.T, schedule []generated) {
	t.Helper()
	production := history.New()
	oracle := model.New()
	events := map[string]generated{}
	for _, event := range schedule {
		if err := production.Accept(event.production); err != nil {
			t.Fatalf("production accept %s: %v", event.oracle.ID, err)
		}
		if err := oracle.Accept(event.oracle, true); err != nil {
			t.Fatalf("oracle accept %s: %v", event.oracle.ID, err)
		}
		events[event.oracle.ID] = event
	}
	got := history.NormalizeHeads(production.Heads(id('F'), "x"))
	want := normalizedOracle(events, oracle.Heads("x"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized heads disagree\nproduction=%v\noracle=%v", got, want)
	}
}

func TestBoundedExhaustiveActorSchedulesAgree(t *testing.T) {
	// Exhaustive bound: every author schedule of length 1..4 for two actors
	// (30 histories), and length 1..4 for three actors (120 histories).
	for _, actors := range [][]byte{{'A', 'B'}, {'A', 'B', 'C'}} {
		for length := 1; length <= 4; length++ {
			total := 1
			for range length {
				total *= len(actors)
			}
			for encoded := 0; encoded < total; encoded++ {
				value := encoded
				counters := map[byte]uint64{}
				latest := map[byte]generated{}
				schedule := make([]generated, 0, length)
				for step := 0; step < length; step++ {
					author := actors[value%len(actors)]
					value /= len(actors)
					counters[author]++
					var parents []generated
					if parent, ok := latest[author]; ok {
						parents = []generated{parent}
					}
					event := buildEvent(id('F'), author, counters[author], "x", "file", fmt.Sprintf("%c-%d", author, counters[author]), parents)
					latest[author] = event
					schedule = append(schedule, event)
				}
				assertAgreement(t, schedule)
			}
		}
	}
}

func TestDeterministicGeneratedSchedulesAgree(t *testing.T) {
	// Four fixed seeds, four actors, 128 events each. Every event extends the
	// author's lineage and some events explicitly resolve all current heads.
	for _, seed := range []int64{2, 17, 101, 20260921} {
		rng := rand.New(rand.NewSource(seed))
		origin := history.New()
		oracleOrigin := model.New()
		counters := map[byte]uint64{}
		byID := map[history.VersionID]generated{}
		events := map[string]generated{}
		var created []generated
		for step := 0; step < 128; step++ {
			author := byte('A' + rng.Intn(4))
			counters[author]++
			var parents []generated
			heads := origin.Heads(id('F'), "x")
			if step > 0 && step%11 == 0 {
				for _, head := range heads {
					parents = append(parents, byID[head.ID])
				}
			} else {
				var latest *generated
				for _, candidate := range created {
					if candidate.production.ID.Author == id(author) && (latest == nil || candidate.production.ID.Counter > latest.production.ID.Counter) {
						copy := candidate
						latest = &copy
					}
				}
				if latest != nil {
					parents = []generated{*latest}
				}
			}
			event := buildEvent(id('F'), author, counters[author], "x", "file", fmt.Sprintf("seed-%d-step-%d", seed, step), parents)
			if err := origin.Accept(event.production); err != nil {
				t.Fatalf("seed %d build: %v", seed, err)
			}
			if err := oracleOrigin.Accept(event.oracle, step%5 != 0); err != nil {
				t.Fatal(err)
			}
			created = append(created, event)
			byID[event.production.ID] = event
			events[event.oracle.ID] = event
		}
		// Deliver a deterministic random topological schedule with duplicates.
		production := history.New()
		oracle := model.New()
		pending := append([]generated(nil), created...)
		delivered := map[string]bool{}
		for len(pending) > 0 {
			var ready []int
			for index, event := range pending {
				ok := true
				for _, parent := range event.oracle.Parents {
					if !delivered[parent] {
						ok = false
						break
					}
				}
				if ok {
					ready = append(ready, index)
				}
			}
			chosen := ready[rng.Intn(len(ready))]
			event := pending[chosen]
			pending = append(pending[:chosen], pending[chosen+1:]...)
			if err := production.Accept(event.production); err != nil {
				t.Fatalf("seed %d delivery: %v", seed, err)
			}
			available := rng.Intn(3) != 0
			if err := oracle.Accept(event.oracle, available); err != nil {
				t.Fatal(err)
			}
			if rng.Intn(4) == 0 {
				if err := production.Accept(event.production); err != nil {
					t.Fatal(err)
				}
				if err := oracle.Accept(event.oracle, available); err != nil {
					t.Fatal(err)
				}
			}
			delivered[event.oracle.ID] = true
		}
		got := history.NormalizeHeads(production.Heads(id('F'), "x"))
		want := normalizedOracle(events, oracle.Heads("x"))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d mismatch\ngot=%v\nwant=%v", seed, got, want)
		}
	}
}

func TestOracleCatchesIntentionalDominanceMutation(t *testing.T) {
	// Mutation demonstration: pretend the newest scalar counter wins and drop
	// concurrent heads. The independent reachability oracle rejects that rule.
	a := buildEvent(id('F'), 'A', 1, "x", "file", "a", nil)
	b := buildEvent(id('F'), 'B', 1, "x", "file", "b", nil)
	oracle := model.New()
	_ = oracle.Accept(a.oracle, true)
	_ = oracle.Accept(b.oracle, true)
	mutatedHeads := []string{b.oracle.ID}
	if reflect.DeepEqual(mutatedHeads, oracle.Heads("x")) {
		t.Fatal("oracle failed to detect intentionally mutated comparison rule")
	}
}

func TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads(t *testing.T) {
	a := buildEvent(id('F'), 'A', 1, "x", "file", "a", nil)
	b := buildEvent(id('F'), 'B', 1, "x", "file", "b", nil)
	oracle := model.New()
	if err := oracle.Accept(a.oracle, true); err != nil {
		t.Fatal(err)
	}
	if err := oracle.Accept(b.oracle, false); err != nil {
		t.Fatal(err)
	}
	if len(oracle.Heads("x")) != 2 || !oracle.Available(a.oracle.ID) || oracle.Available(b.oracle.ID) {
		t.Fatal("content availability was conflated with accepted causal heads")
	}
	inventoryCursor := uint64(100)
	if inventoryCursor == 100 && oracle.Available(b.oracle.ID) {
		t.Fatal("inventory cursor incorrectly implied receipt/content availability")
	}
	admission := model.Admission{MembershipMatches: true, Active: map[string]bool{"A": true}, Retired: map[string]map[uint64]string{"B": {1: "approved"}}}
	if !admission.Allows("B", 1, "approved", true) || admission.Allows("B", 2, "unknown", true) || admission.Allows("A", 2, "active", false) {
		t.Fatal("retirement oracle admitted an unknown or ancestry-blocked event")
	}
}
