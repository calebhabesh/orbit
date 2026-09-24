// Package model is a deliberately simple test oracle. It must not import
// the production history or repository implementations.
package model

import (
	"errors"
	"sort"
	"time"
)

var (
	ErrObjectMissing   = errors.New("oracle object missing")
	ErrGCIntentActive  = errors.New("oracle GC intent is active on object")
	ErrObjectHasPin    = errors.New("oracle object has active pin or lease")
	ErrObjectProtected = errors.New("oracle object is protected root")
)

type ObjectState int

const (
	ObjectPresent ObjectState = iota
	ObjectIntent
	ObjectUnlinked
	ObjectFinalized
	ObjectCorrupt
	ObjectQuarantined
)

type ContentStatus string

const (
	StatusReady            ContentStatus = "ready"
	StatusPending          ContentStatus = "pending"
	StatusExpired          ContentStatus = "expired"
	StatusMissingProtected ContentStatus = "missing_protected"
	StatusCorrupt          ContentStatus = "corrupt"
	StatusPeerUnavailable  ContentStatus = "peer_unavailable"
	StatusUnrecoverable    ContentStatus = "unrecoverable"
)

type VersionRecord struct {
	ID          string
	Path        string
	Chunks      []string
	Acquired    time.Time
	ExplicitPin bool
}

type RetentionPolicy struct {
	RetentionDays int
	MinSuperseded int
}

// ReferenceSetOracle tracks objects, versions, pins, leases, and GC transitions
// independently of the production repository implementation.
type ReferenceSetOracle struct {
	objects       map[string]ObjectState
	verifiedBytes map[string]bool // whether backup/verified bytes exist for reinstall
	versions      map[string]VersionRecord
	heads         map[string][]string        // path -> head version IDs
	fallback      map[string]string          // path -> fallback version ID (active while successor pending)
	pendingPaths  map[string]bool            // path -> true if publication/successor is pending
	pins          map[string]map[string]bool // chunk -> "kind:key" -> true
	leases        map[string]map[string]bool // chunk -> leaseID -> true
	intents       map[string]int             // chunk -> generation
	quarantined   map[string]string          // chunk -> reason
	generation    int
	trace         []string
}

func NewReferenceSetOracle() *ReferenceSetOracle {
	return &ReferenceSetOracle{
		objects:       make(map[string]ObjectState),
		verifiedBytes: make(map[string]bool),
		versions:      make(map[string]VersionRecord),
		heads:         make(map[string][]string),
		fallback:      make(map[string]string),
		pendingPaths:  make(map[string]bool),
		pins:          make(map[string]map[string]bool),
		leases:        make(map[string]map[string]bool),
		intents:       make(map[string]int),
		quarantined:   make(map[string]string),
	}
}

func (o *ReferenceSetOracle) Trace() []string {
	return append([]string(nil), o.trace...)
}

func (o *ReferenceSetOracle) InstallObject(chunk string, hasVerifiedBytes bool) {
	o.objects[chunk] = ObjectPresent
	o.verifiedBytes[chunk] = hasVerifiedBytes
	delete(o.intents, chunk)
	o.trace = append(o.trace, "install-object:"+chunk)
}

func (o *ReferenceSetOracle) AddVersion(vr VersionRecord) {
	o.versions[vr.ID] = vr
	for _, chunk := range vr.Chunks {
		if _, ok := o.objects[chunk]; !ok {
			o.objects[chunk] = ObjectPresent
			o.verifiedBytes[chunk] = true
		}
	}
}

func (o *ReferenceSetOracle) SetHeads(path string, headIDs []string) {
	sorted := append([]string(nil), headIDs...)
	sort.Strings(sorted)
	o.heads[path] = sorted
}

func (o *ReferenceSetOracle) SetPending(path string, pending bool, fallbackVersionID string) {
	o.pendingPaths[path] = pending
	if pending && fallbackVersionID != "" {
		o.fallback[path] = fallbackVersionID
	} else if !pending {
		delete(o.fallback, path)
	}
}

func (o *ReferenceSetOracle) Pin(chunk, kind, key string) {
	if o.pins[chunk] == nil {
		o.pins[chunk] = make(map[string]bool)
	}
	pinID := kind + ":" + key
	o.pins[chunk][pinID] = true
	// D4: Pinning / referencing cancels an active intent
	if _, ok := o.intents[chunk]; ok {
		delete(o.intents, chunk)
		if o.objects[chunk] == ObjectIntent {
			o.objects[chunk] = ObjectPresent
		}
		o.trace = append(o.trace, "intent-cancelled-by-pin:"+chunk)
	}
	o.trace = append(o.trace, "pinned:"+chunk+":"+pinID)
}

func (o *ReferenceSetOracle) Unpin(chunk, kind, key string) {
	pinID := kind + ":" + key
	if m := o.pins[chunk]; m != nil {
		delete(m, pinID)
		if len(m) == 0 {
			delete(o.pins, chunk)
		}
	}
	o.trace = append(o.trace, "unpinned:"+chunk+":"+pinID)
}

func (o *ReferenceSetOracle) HasPinsOrLeases(chunk string) bool {
	return len(o.pins[chunk]) > 0 || len(o.leases[chunk]) > 0
}

func (o *ReferenceSetOracle) AcquireLease(chunk, leaseID string) error {
	state, exists := o.objects[chunk]
	if !exists || state == ObjectUnlinked || state == ObjectFinalized {
		return ErrObjectMissing
	}
	if _, hasIntent := o.intents[chunk]; hasIntent || state == ObjectIntent {
		o.trace = append(o.trace, "lease-blocked-by-intent:"+chunk+":"+leaseID)
		return ErrGCIntentActive
	}
	if o.leases[chunk] == nil {
		o.leases[chunk] = make(map[string]bool)
	}
	o.leases[chunk][leaseID] = true
	o.trace = append(o.trace, "lease-acquired:"+chunk+":"+leaseID)
	return nil
}

func (o *ReferenceSetOracle) ReleaseLease(chunk, leaseID string) {
	if m := o.leases[chunk]; m != nil {
		delete(m, leaseID)
		if len(m) == 0 {
			delete(o.leases, chunk)
		}
	}
	o.trace = append(o.trace, "lease-released:"+chunk+":"+leaseID)
}

// ComputeProtectedChunks returns the set of all chunks that are currently protected
// by heads, fallback content, retention policy, explicit pins, active pins, and active leases.
func (o *ReferenceSetOracle) ComputeProtectedChunks(now time.Time, policy RetentionPolicy) map[string]bool {
	protected := make(map[string]bool)

	// 1. Current heads
	for _, headIDs := range o.heads {
		for _, hid := range headIDs {
			if v, ok := o.versions[hid]; ok {
				for _, ch := range v.Chunks {
					protected[ch] = true
				}
			}
		}
	}

	// 2. Pending-publication fallback
	for path, pending := range o.pendingPaths {
		if pending {
			if fbID, ok := o.fallback[path]; ok {
				if v, ok := o.versions[fbID]; ok {
					for _, ch := range v.Chunks {
						protected[ch] = true
					}
				}
			}
		}
	}

	// 3. Historical retention policy per path
	// Group versions by path
	byPath := make(map[string][]VersionRecord)
	for _, v := range o.versions {
		byPath[v.Path] = append(byPath[v.Path], v)
	}

	retentionDur := time.Duration(policy.RetentionDays) * 24 * time.Hour
	for _, vers := range byPath {
		// Sort superseded versions by Acquired descending
		sort.Slice(vers, func(i, j int) bool {
			return vers[i].Acquired.After(vers[j].Acquired)
		})

		for idx, v := range vers {
			// Explicit pin always protects
			if v.ExplicitPin {
				for _, ch := range v.Chunks {
					protected[ch] = true
				}
				continue
			}

			// Arm 1: Newer than RetentionDays since local durable acquisition.
			// Clock jump backwards: if now < v.Acquired, elapsed is negative, so protected!
			elapsed := now.Sub(v.Acquired)
			if elapsed < retentionDur {
				for _, ch := range v.Chunks {
					protected[ch] = true
				}
				continue
			}

			// Arm 2: Among the MinSuperseded newest superseded versions on path
			if idx < policy.MinSuperseded {
				for _, ch := range v.Chunks {
					protected[ch] = true
				}
				continue
			}
		}
	}

	// 4. Active pins and serve leases
	for chunk, pmap := range o.pins {
		if len(pmap) > 0 {
			protected[chunk] = true
		}
	}
	for chunk, lmap := range o.leases {
		if len(lmap) > 0 {
			protected[chunk] = true
		}
	}

	return protected
}

// ComputeCandidates returns chunks installed in the oracle that are not protected.
func (o *ReferenceSetOracle) ComputeCandidates(now time.Time, policy RetentionPolicy) []string {
	protected := o.ComputeProtectedChunks(now, policy)
	var candidates []string
	for chunk, state := range o.objects {
		if (state == ObjectPresent || state == ObjectIntent) && !protected[chunk] {
			candidates = append(candidates, chunk)
		}
	}
	sort.Strings(candidates)
	return candidates
}

// BeginGC acquires durable deletion intents on eligible candidates.
func (o *ReferenceSetOracle) BeginGC(candidates []string) ([]string, error) {
	o.generation++
	var acquired []string
	for _, chunk := range candidates {
		if o.HasPinsOrLeases(chunk) {
			o.trace = append(o.trace, "gc-intent-blocked-by-pin:"+chunk)
			continue
		}
		state, ok := o.objects[chunk]
		if !ok || state == ObjectUnlinked || state == ObjectFinalized {
			continue
		}
		o.objects[chunk] = ObjectIntent
		o.intents[chunk] = o.generation
		acquired = append(acquired, chunk)
		o.trace = append(o.trace, "gc-intent:"+chunk)
	}
	return acquired, nil
}

// Unlink unlinks an object that has an active intent and no concurrent pins/leases.
func (o *ReferenceSetOracle) Unlink(chunk string) bool {
	if _, hasIntent := o.intents[chunk]; !hasIntent {
		o.trace = append(o.trace, "unlink-blocked-no-intent:"+chunk)
		return false
	}
	if o.HasPinsOrLeases(chunk) {
		o.trace = append(o.trace, "unlink-blocked-by-pin:"+chunk)
		return false
	}
	o.objects[chunk] = ObjectUnlinked
	o.trace = append(o.trace, "object-unlinked:"+chunk)
	return true
}

// Finalize commits the GC completion: removes intent and marks finalized.
func (o *ReferenceSetOracle) Finalize(chunk string) bool {
	if o.objects[chunk] != ObjectUnlinked {
		return false
	}
	delete(o.intents, chunk)
	o.objects[chunk] = ObjectFinalized
	o.trace = append(o.trace, "object-finalized:"+chunk)
	return true
}

// AcquireReference models adding a new reference to chunk (e.g. from new version or restore).
// D4 rules:
// - cancels intent
// - if already unlinked, must reinstall verified bytes before commit.
func (o *ReferenceSetOracle) AcquireReference(chunk, refID string) bool {
	if _, hasIntent := o.intents[chunk]; hasIntent {
		delete(o.intents, chunk)
		if o.objects[chunk] == ObjectIntent {
			o.objects[chunk] = ObjectPresent
		}
		o.trace = append(o.trace, "intent-cancelled-by-ref:"+chunk+":"+refID)
	}

	state := o.objects[chunk]
	if state == ObjectUnlinked || state == ObjectFinalized {
		if !o.verifiedBytes[chunk] {
			o.trace = append(o.trace, "reference-blocked-no-verified-bytes:"+chunk)
			return false
		}
		o.objects[chunk] = ObjectPresent
		o.trace = append(o.trace, "verified-object-reinstalled:"+chunk)
	}

	o.trace = append(o.trace, "reference-committed:"+chunk+":"+refID)
	return true
}

// Recover simulates crash recovery at startup.
func (o *ReferenceSetOracle) Recover(now time.Time, policy RetentionPolicy) {
	protected := o.ComputeProtectedChunks(now, policy)
	for chunk, gen := range o.intents {
		_ = gen
		state := o.objects[chunk]
		switch state {
		case ObjectIntent:
			// File is still on disk. Crash after intent retains extra bytes safely.
			delete(o.intents, chunk)
			o.objects[chunk] = ObjectPresent
			o.trace = append(o.trace, "recover-intent-retained:"+chunk)
		case ObjectUnlinked:
			// File was unlinked before crash.
			if protected[chunk] || o.HasPinsOrLeases(chunk) {
				// Protected content missing!
				o.trace = append(o.trace, "recover-unlinked-protected-missing:"+chunk)
			} else {
				// Clean up unlinked object from inventory
				delete(o.intents, chunk)
				o.objects[chunk] = ObjectFinalized
				o.trace = append(o.trace, "recover-unlinked-finalized:"+chunk)
			}
		}
	}
}

func (o *ReferenceSetOracle) ObjectState(chunk string) ObjectState {
	return o.objects[chunk]
}

// CorruptChunk marks a chunk as corrupt and returns all version IDs referencing it.
func (o *ReferenceSetOracle) CorruptChunk(chunk string, reason string) []string {
	o.objects[chunk] = ObjectCorrupt
	o.trace = append(o.trace, "corrupt:"+chunk+":"+reason)
	return o.AffectedVersions(chunk)
}

// QuarantineChunk moves a corrupt chunk into quarantined state and returns all version IDs referencing it.
func (o *ReferenceSetOracle) QuarantineChunk(chunk string, reason string) []string {
	o.objects[chunk] = ObjectQuarantined
	if o.quarantined == nil {
		o.quarantined = make(map[string]string)
	}
	o.quarantined[chunk] = reason
	o.trace = append(o.trace, "quarantine:"+chunk+":"+reason)
	return o.AffectedVersions(chunk)
}

// AffectedVersions returns all version IDs that reference the given chunk.
func (o *ReferenceSetOracle) AffectedVersions(chunk string) []string {
	var affected []string
	for vid, v := range o.versions {
		for _, ch := range v.Chunks {
			if ch == chunk {
				affected = append(affected, vid)
				break
			}
		}
	}
	sort.Strings(affected)
	return affected
}

// RepairChunk restores a chunk using verified bytes from an authorized replica.
func (o *ReferenceSetOracle) RepairChunk(chunk string, hasVerifiedBytes bool) error {
	if !hasVerifiedBytes {
		o.trace = append(o.trace, "repair-failed-unrecoverable:"+chunk)
		return errors.New("no replica has valid bytes: unrecoverable")
	}
	o.objects[chunk] = ObjectPresent
	o.verifiedBytes[chunk] = true
	if o.quarantined != nil {
		delete(o.quarantined, chunk)
	}
	o.trace = append(o.trace, "repaired:"+chunk)
	return nil
}

// VersionAvailability computes the availability status of a version.
func (o *ReferenceSetOracle) VersionAvailability(versionID string, now time.Time, policy RetentionPolicy) ContentStatus {
	v, ok := o.versions[versionID]
	if !ok {
		return StatusUnrecoverable
	}
	if len(v.Chunks) == 0 {
		return StatusReady
	}

	// Check if any chunk is corrupt or quarantined
	for _, ch := range v.Chunks {
		if st, exists := o.objects[ch]; exists {
			if st == ObjectCorrupt || st == ObjectQuarantined {
				return StatusCorrupt
			}
		}
	}

	// Check if all chunks are present
	allPresent := true
	for _, ch := range v.Chunks {
		st, exists := o.objects[ch]
		if !exists || st != ObjectPresent {
			allPresent = false
			break
		}
	}
	if allPresent {
		return StatusReady
	}

	// Chunks are missing / unlinked. Determine if expired or missing protected.
	protected := o.ComputeProtectedChunks(now, policy)
	isProtected := false
	for _, ch := range v.Chunks {
		if protected[ch] {
			isProtected = true
			break
		}
	}
	if isProtected {
		return StatusMissingProtected
	}
	return StatusExpired
}
