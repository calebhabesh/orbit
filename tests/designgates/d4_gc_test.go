package designgates

import (
	"fmt"
	"sort"
	"testing"
)

type gcModel struct {
	object       bool
	verifiedCopy bool
	intent       bool
	refs         map[string]bool
	leases       map[string]bool
	trace        []string
}

func newGCModel() *gcModel {
	return &gcModel{object: true, verifiedCopy: true, refs: map[string]bool{}, leases: map[string]bool{}}
}

func (m *gcModel) beginGC() bool {
	if len(m.refs) != 0 || len(m.leases) != 0 {
		m.trace = append(m.trace, "gc-blocked-by-pin")
		return false
	}
	m.intent = true
	m.trace = append(m.trace, "gc-intent")
	return true
}

func (m *gcModel) unlink() bool {
	if !m.intent || len(m.refs) != 0 || len(m.leases) != 0 {
		m.trace = append(m.trace, "unlink-blocked")
		return false
	}
	m.object = false
	m.trace = append(m.trace, "object-unlinked")
	return true
}

func (m *gcModel) acquireReference(name string) bool {
	// Reference creation and GC intent mutation share repository
	// serialization. A new reference cancels an intent; after unlink it must
	// reinstall verified bytes before the reference may commit.
	if m.intent {
		m.intent = false
		m.trace = append(m.trace, "intent-cancelled")
	}
	if !m.object {
		if !m.verifiedCopy {
			m.trace = append(m.trace, "reference-blocked-no-content")
			return false
		}
		m.object = true
		m.trace = append(m.trace, "verified-object-reinstalled")
	}
	m.refs[name] = true
	m.trace = append(m.trace, "reference-committed:"+name)
	return true
}

func (m *gcModel) acquireLease(name string) bool {
	if !m.object || m.intent {
		m.trace = append(m.trace, "lease-blocked:"+name)
		return false
	}
	m.leases[name] = true
	m.trace = append(m.trace, "lease-acquired:"+name)
	return true
}

func (m *gcModel) recover() {
	if !m.intent {
		m.trace = append(m.trace, "recover-no-intent")
		return
	}
	if len(m.refs) != 0 || len(m.leases) != 0 {
		m.intent = false
		m.trace = append(m.trace, "recover-cancel-intent")
		return
	}
	m.trace = append(m.trace, "recover-unreferenced-intent")
}

func TestD4ReferenceCreationInterleavingsNeverCommitMissingProtectedContent(t *testing.T) {
	actions := []string{"begin", "unlink", "reference"}
	for _, reference := range []string{"head", "in-flight-fetch", "publication", "restore", "enrollment"} {
		for _, schedule := range permutations(actions) {
			beginIndex, unlinkIndex := indexOf(schedule, "begin"), indexOf(schedule, "unlink")
			if beginIndex > unlinkIndex {
				continue
			}
			t.Run(reference+"/"+fmt.Sprint(schedule), func(t *testing.T) {
				model := newGCModel()
				committed := false
				for _, action := range schedule {
					switch action {
					case "begin":
						model.beginGC()
					case "unlink":
						model.unlink()
					case "reference":
						committed = model.acquireReference(reference)
					}
					if committed && !model.object {
						t.Fatalf("protected reference has missing object after %s: %v", action, model.trace)
					}
				}
				if !committed {
					t.Fatalf("reference should commit with a verified recovery copy: %v", model.trace)
				}
				t.Log(model.trace)
			})
		}
	}
}

func TestD4ActiveServeLeaseExcludesDeletionIntent(t *testing.T) {
	model := newGCModel()
	if !model.acquireLease("serve-peer-A") {
		t.Fatal("could not acquire lease on present content")
	}
	if model.beginGC() {
		t.Fatal("GC intent began while serve lease was active")
	}
	if model.unlink() {
		t.Fatal("object unlinked while serve lease was active")
	}
}

func TestD4CrashRecoveryAllowsOnlyExtraUnreferencedBytesOrRecoverableAbsence(t *testing.T) {
	t.Run("after-intent", func(t *testing.T) {
		model := newGCModel()
		model.beginGC()
		model.recover()
		if !model.object {
			t.Fatal("intent-only crash lost object")
		}
	})
	t.Run("after-unlink", func(t *testing.T) {
		model := newGCModel()
		model.beginGC()
		model.unlink()
		model.recover()
		if model.object || len(model.refs) != 0 || len(model.leases) != 0 {
			t.Fatalf("unexpected protected state: %+v", model)
		}
	})
}

func TestD4ExpiryAndOfflinePeersDoNotOverrideSafetyPins(t *testing.T) {
	type contentState struct {
		expired            bool
		currentHead        bool
		publicationPending bool
		restoreActive      bool
		offlinePeerMayNeed bool
	}
	eligible := func(state contentState) bool {
		if !state.expired {
			return false
		}
		return !state.currentHead && !state.publicationPending && !state.restoreActive
	}
	for name, scenario := range map[string]struct {
		state contentState
		want  bool
	}{
		"expired-head":        {state: contentState{expired: true, currentHead: true}, want: false},
		"pending-publication": {state: contentState{expired: true, publicationPending: true}, want: false},
		"active-restore":      {state: contentState{expired: true, restoreActive: true}, want: false},
		"offline-peer-only":   {state: contentState{expired: true, offlinePeerMayNeed: true}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := eligible(scenario.state); got != scenario.want {
				t.Fatalf("eligible = %v, want %v", got, scenario.want)
			}
		})
	}
}

func permutations(values []string) [][]string {
	var output [][]string
	var visit func([]string, int)
	visit = func(items []string, start int) {
		if start == len(items) {
			output = append(output, append([]string(nil), items...))
			return
		}
		for index := start; index < len(items); index++ {
			items[start], items[index] = items[index], items[start]
			visit(items, start+1)
			items[start], items[index] = items[index], items[start]
		}
	}
	visit(append([]string(nil), values...), 0)
	sort.Slice(output, func(i, j int) bool { return fmt.Sprint(output[i]) < fmt.Sprint(output[j]) })
	return output
}

func indexOf(values []string, value string) int {
	for index, candidate := range values {
		if candidate == value {
			return index
		}
	}
	return -1
}
