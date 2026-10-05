package terminal

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

type dailyFixture struct {
	workflowFixture
	uploads int
}

func (f *dailyFixture) UploadSessionResult(context.Context, tc.EditorSession, string, uint64) (tc.Result, error) {
	f.uploads++
	return tc.Result{}, nil
}
func dailyModel() (*model, *dailyFixture) {
	w := &dailyFixture{}
	return newModel(context.Background(), w, Options{Colorless: true}), w
}

func TestTerminalT13PendingContentUsesTickAndExactExplicitRetry(t *testing.T) {
	m, w := dailyModel()
	intent := tc.Mutation{Version: tc.Version, Kind: "content", OperationID: strings.Repeat("a", 64)}
	m.flow = &workflow{screen: "day_operation", operation: intent.OperationID, daily: &dailyState{mutation: intent}}
	pending := tc.Result{Operation: &tc.Operation{ID: intent.OperationID, State: "pending", Phase: "publication"}}
	if cmd := m.acceptDaily("day_operation", pending, nil); cmd != nil {
		t.Fatal("pending observation started an immediate polling loop")
	}
	if w.calls != 0 {
		t.Fatal("poll replayed mutation")
	}
	runReply(m, press(m, "r"))
	if w.calls != 1 || w.mutation.OperationID != intent.OperationID {
		t.Fatal("explicit retry did not reuse the durable operation")
	}
}
func reviewFixture() tc.Result {
	a := tc.VersionID{Folder: strings.Repeat("a", 64), Author: strings.Repeat("b", 64), Counter: 1}
	b := a
	b.Author = strings.Repeat("c", 64)
	v := tc.ContentReview{Context: tc.Context{Folder: a.Folder, FolderName: "Notes", Root: "/private/root", Path: "doc", Generation: strings.Repeat("d", 64)}, Review: tc.Review{Token: strings.Repeat("e", 64)}, Heads: []tc.VersionID{a, b}, WorkingCaptured: true}
	return tc.Result{ContentReview: &v, Versions: []tc.VersionSummary{{Version: a, Path: "doc", DeviceName: "Laptop", Kind: "file", Availability: "ready"}, {Version: b, Path: "doc", DeviceName: "Pi", Kind: "file", Availability: "ready"}}}
}
func TestTerminalT11FrozenReviewAndExactRetry(t *testing.T) {
	m, w := dailyModel()
	r := reviewFixture()
	w.query = func(context.Context, tc.Query) (tc.Result, error) { return r, nil }
	runReply(m, m.everyday(r.ContentReview.Context.Folder, "load_review", "doc"))
	f := m.flow
	token := f.daily.review.Review
	if cmd := m.startQuery(); cmd != nil {
		t.Fatal("poll refreshed immutable review")
	}
	press(m, "j")
	keyCode(m, tea.KeyEnter)
	if f.screen != "day_confirm" || w.calls != 0 {
		t.Fatal("preview missing")
	}
	runReply(m, keyCode(m, tea.KeyEnter))
	mutation := w.mutation
	if mutation.Content.Source != r.Versions[1].Version || mutation.Content.Review != token || len(mutation.Content.Heads) != 2 {
		t.Fatal("wrong reviewed intent")
	}
	f.screen = "day_confirm"
	runReply(m, keyCode(m, tea.KeyEnter))
	if w.mutation.OperationID != mutation.OperationID {
		t.Fatal("retry changed operation")
	}
	f.screen = "day_confirm"
	m.acceptDaily("day_commit", tc.Result{}, &control.ControlError{Code: "STALE_VIEW", Message: "SECRET"})
	if !strings.Contains(m.View().Content, "STALE_VIEW") || strings.Contains(m.View().Content, "SECRET") {
		t.Fatal("stale correction/error redaction")
	}
	if f.daily.review.Review != token {
		t.Fatal("failure silently advanced review")
	}
}
func TestTerminalT11PathDraftPollingAndSelection(t *testing.T) {
	m, w := dailyModel()
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		return tc.Result{Items: []tc.NamedItem{{ID: "aa", Name: "aa"}, {ID: "doc", Name: "doc"}}, Cursor: "next"}, nil
	}
	runReply(m, m.everyday(strings.Repeat("a", 64), "paths", ""))
	press(m, "/")
	for _, s := range []string{"j", "k", "q", "?"} {
		press(m, s)
	}
	if m.flow.fields[0].input.Value() != "jkq?" || m.quitting {
		t.Fatal("focused shortcuts consumed")
	}
	runReply(m, keyCode(m, tea.KeyEnter))
	press(m, "j")
	runReply(m, m.startQuery())
	if m.flow.selected != 1 || m.flow.fields[0].input.Value() != "jkq?" {
		t.Fatal("refresh lost selected identity/draft")
	}
	old := m.startQuery()
	m.closeFlow()
	m.Update(old())
	if m.flow != nil {
		t.Fatal("late reply resurrected flow")
	}
}
func TestTerminalT11UnavailableContentAndNarrowHelp(t *testing.T) {
	m, w := dailyModel()
	r := reviewFixture()
	r.Versions[0].Availability = "pending"
	w.query = func(context.Context, tc.Query) (tc.Result, error) { return r, nil }
	runReply(m, m.everyday(r.ContentReview.Context.Folder, "load_review", "doc"))
	keyCode(m, tea.KeyEnter)
	if m.flow.err == "" || m.flow.screen != "day_review" || w.calls != 0 {
		t.Fatal("unavailable action admitted")
	}
	press(m, "e")
	if w.calls != 0 {
		t.Fatal("unconfigured editor created session")
	}
	for _, screen := range []string{"day_status", "day_storage", "day_maintenance", "day_paths", "day_history", "day_deleted", "day_conflicts", "day_review", "day_destination", "day_copies", "day_confirm", "day_editor", "day_recovery", "day_operation"} {
		m.flow.screen = screen
		m.flow.fields = []field{newField("Destination", "doc", false)}
		m.flow.result = r
		m.flow.advanced = true
		for _, size := range [][2]int{{80, 24}, {40, 16}, {20, 10}} {
			m.width, m.height = size[0], size[1]
			view := m.View().Content
			if len(strings.Split(view, "\n")) > m.height {
				t.Fatal(screen, "height")
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > m.width {
					t.Fatal(screen, "width")
				}
			}
			if !strings.Contains(view, "Esc") {
				t.Fatal(screen, "back unavailable")
			}
		}
	}
}
func TestTerminalT11QualifiedObservationsAndPartialEffects(t *testing.T) {
	m, _ := dailyModel()
	m.flow = &workflow{screen: "day_status", daily: &dailyState{}, result: tc.Result{Readiness: &tc.Readiness{Uncaptured: 1, MissingContent: 2}, Observations: []tc.Observation{{DeviceName: "Pi", Saved: true, Stored: true, Applied: false, Online: false, Direct: false, LastContact: "yesterday"}}}}
	m.height = 40
	v := m.View().Content
	for _, s := range []string{"uncaptured=1", "missing=2", "saved=true stored=true applied=false", "online=false direct=false", "yesterday", "historical"} {
		if !strings.Contains(v, s) {
			t.Fatal(s, v)
		}
	}
	m.flow.screen = "day_operation"
	m.flow.result = tc.Result{Operation: &tc.Operation{State: "partial", Phase: "copies", CommittedEffects: []tc.Effect{{Path: "safe-copy", State: "durable"}}}}
	v = m.View().Content
	if !strings.Contains(v, "safe-copy: durable") || !strings.Contains(v, "partial") {
		t.Fatal("partial effects hidden")
	}
}
