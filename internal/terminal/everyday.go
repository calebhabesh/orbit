package terminal

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"path/filepath"
	"slices"
	"strings"
)

type Everyday interface {
	Workflows
	UploadSessionResult(context.Context, tc.EditorSession, string, uint64) (tc.Result, error)
}
type dailyState struct {
	path, destination, action, selectedKey, uploadID, toolKind string
	review                                                     *tc.ContentReview
	versions                                                   []tc.VersionSummary
	source                                                     tc.VersionID
	session                                                    *tc.EditorSession
	upload                                                     *tc.UploadResult
	copies                                                     []tc.CopyPlan
	mutation                                                   tc.Mutation
	limit                                                      uint64
}

func (m *model) everyday(folder, screen, path string) tea.Cmd {
	if _, ok := m.client.(Everyday); !ok {
		m.notice = "Everyday controls unavailable."
		return nil
	}
	f := &workflow{screen: "day_" + screen, folder: folder, daily: &dailyState{path: path, action: "select", limit: 1 << 20}}
	if screen == "paths" {
		f.fields = []field{newField("Find known path", "", false)}
	}
	return m.openFlow(f)
}
func (m *model) dailyCommand(ctx context.Context) (string, func() (tc.Result, error)) {
	f := m.flow
	d := f.daily
	q := tc.Query{Version: tc.Version, Folder: f.folder, Limit: pageSize, Cursor: f.cursor}
	switch f.screen {
	case "day_status":
		q.Kind = "status"
	case "day_storage":
		q.Kind = "storage"
	case "day_maintenance":
		q.Kind = "maintenance"
	case "day_paths":
		q.Kind = "paths"
		q.Name = f.fields[0].input.Value()
		if q.Name == "" {
			return "", nil
		}
	case "day_conflicts":
		q.Kind = "conflicts"
	case "day_history":
		q.Kind = "history"
		q.Path = d.path
	case "day_deleted":
		q.Kind = "deleted"
	case "day_load_review":
		q.Kind = "content_review"
		q.Path = d.path
		q.Destination = d.destination
		if d.source.Counter != 0 {
			source := d.source
			q.Source = &source
		}
	case "day_load_session":
		q.Kind = "session"
		q.ID = f.operation
	case "day_operation":
		q.Kind = "operation"
		q.ID = f.operation
	default:
		return "", nil
	}
	task := f.screen
	return task, func() (tc.Result, error) { return m.client.Query(ctx, q) }
}
func versionKey(v tc.VersionID) string { return fmt.Sprintf("%s:%d", v.Author, v.Counter) }
func (m *model) acceptDaily(task string, r tc.Result, err error) tea.Cmd {
	f := m.flow
	d := f.daily
	f.busy = false
	if err != nil || r.Error != nil {
		f.err = workflowError(r, err)
		if r.Operation != nil {
			f.result = r
			f.operation = r.Operation.ID
		}
		if strings.Contains(f.err, "STALE_VIEW") {
			f.err = "STALE_VIEW: review changed. Press r for a fresh review; retained editor bytes are never committed automatically."
		}
		return nil
	}
	// Reviewed screens are immutable between explicit user actions.
	f.err = ""
	switch task {
	case "day_load_review":
		if r.ContentReview == nil {
			f.err = "Review unavailable; r retry."
			return nil
		}
		d.review = r.ContentReview
		d.versions = r.Versions
		d.mutation = tc.Mutation{}
		f.selected = 0
		f.cursor = ""
		f.scroll = 0
		if d.source.Counter == 0 && len(d.versions) > 0 {
			d.source = d.versions[0].Version
		}
		f.screen = "day_review"
		f.result = r
		if d.session != nil {
			f.screen = "day_recovery"
		}
	case "day_create_session":
		if r.Session == nil {
			f.err = "Session unavailable; retry the exact operation."
			return nil
		}
		d.session = r.Session
		d.limit = 1 << 20
		for _, v := range d.versions {
			d.limit = max(d.limit, uint64(v.Bytes))
		}
		f.screen = "day_editor"
		d.mutation = tc.Mutation{}
		return m.launchSessionTool(d.toolKind)
	case "day_copies":
		d.copies = r.CopyPlans
		d.action = "keep_copies"
		f.screen = "day_confirm"
		d.mutation = tc.Mutation{}
	case "day_upload":
		if r.Upload == nil {
			f.err = "Staged upload unavailable; retry."
			return nil
		}
		d.upload = r.Upload
		d.action = "merge"
		f.screen = "day_confirm"
		f.result = r
		d.mutation = tc.Mutation{}
	case "day_commit":
		f.result = r
		f.screen = "day_operation"
		if r.Operation != nil {
			f.operation = r.Operation.ID
			if r.Operation.State != "completed" && r.Operation.State != "blocked" && r.Operation.State != "failed" {
				return m.startQuery()
			}
		}
	case "day_load_session":
		if r.Session == nil {
			f.err = "Session unavailable."
			return nil
		}
		d.session = r.Session
		d.path = r.Session.Context.Path
		f.folder = r.Session.Context.Folder
		d.limit = 1 << 20
		for _, v := range r.Versions {
			d.limit = max(d.limit, uint64(v.Bytes))
		}
		f.screen = "day_load_review"
		return m.invalidate()
	case "day_renew":
		d.session = r.Session
		d.upload = nil
		d.uploadID = ""
		d.mutation = tc.Mutation{}
		f.screen = "day_editor"
	case "day_discard", "day_operation":
		f.result = r
		f.screen = "day_operation"
		if r.Operation != nil {
			f.operation = r.Operation.ID
		}
	default:
		f.result = r
		count := len(r.Items)
		if f.screen == "day_history" {
			count = len(r.Versions)
		}
		if f.screen == "day_conflicts" {
			count = len(r.Attention)
		}
		// Preserve selected identity across refreshes, then bound the index.
		for i := 0; i < count; i++ {
			if dailyItemKey(f, i) == d.selectedKey {
				f.selected = i
				break
			}
		}
		f.selected = min(f.selected, max(0, count-1))
		if count > 0 {
			d.selectedKey = dailyItemKey(f, f.selected)
		}
	}
	return nil
}
func dailyItemKey(f *workflow, i int) string {
	switch f.screen {
	case "day_history":
		return versionKey(f.result.Versions[i].Version)
	case "day_conflicts":
		return f.result.Attention[i].ID
	default:
		return f.result.Items[i].ID
	}
}
func (m *model) loadReview(action string) tea.Cmd {
	f := m.flow
	d := f.daily
	d.action = action
	d.review = nil
	d.mutation = tc.Mutation{}
	f.screen = "day_load_review"
	f.cursor = ""
	f.err = ""
	return m.invalidate()
}
func (m *model) dailyMutation(task string, mutation tc.Mutation) tea.Cmd {
	f := m.flow
	d := f.daily
	if d.mutation.OperationID == "" {
		id, err := newID()
		if err != nil {
			f.err = "Operation identity unavailable."
			return nil
		}
		mutation.Version = tc.Version
		mutation.OperationID = id
		d.mutation = mutation
	}
	w, _ := m.workflows()
	exact := d.mutation
	f.task = task
	f.work = func(ctx context.Context) (tc.Result, error) { return w.Mutate(ctx, exact) }
	return m.invalidate()
}
func (m *model) createSession(kind string) tea.Cmd {
	d := m.flow.daily
	if d.review == nil {
		return nil
	}
	sources := []tc.VersionID{}
	for _, v := range d.versions {
		if v.Kind == "file" {
			if !available(v) {
				m.flow.err = "CONTENT_UNAVAILABLE: all file sources must be available; peer fetch is unsupported. Inspect orbit doctor."
				return nil
			}
			sources = append(sources, v.Version)
		}
	}
	if len(sources) == 0 {
		m.flow.err = "No file sources; use exact select or the structural-conflict runbook."
		return nil
	}
	if (kind == "edit" && m.opts.Editor == "") || (kind == "diff" && m.opts.Diff == "") {
		m.flow.err = "Tool not configured; set EDITOR or ORBIT_DIFF, or pass --editor / --diff."
		return nil
	}
	d.toolKind = kind
	return m.dailyMutation("day_create_session", tc.Mutation{Kind: "session", Session: &tc.SessionIntent{Action: "create", Context: d.review.Context, Review: d.review.Review, Heads: d.review.Heads, Sources: sources}})
}
func (m *model) launchSessionTool(kind string) tea.Cmd {
	f := m.flow
	d := f.daily
	if d.session == nil || d.session.State != "active" {
		f.err = "Retained session needs a fresh review and renewal before editing."
		return nil
	}
	command := m.opts.Editor
	paths := []string{d.session.ResultPath}
	if kind == "diff" {
		command = m.opts.Diff
		paths = nil
		for i := range d.session.Sources {
			paths = append(paths, filepath.Join(filepath.Dir(d.session.ResultPath), fmt.Sprintf("source-%02d", i)))
		}
	}
	cmd, err := controlclient.LimitedToolCommand(m.ctx, command, d.limit, paths...)
	if err != nil {
		f.err = "Tool unavailable; check configuration and prlimit. Retained session can be reopened."
		return nil
	}
	m.toolRunning = true
	m.generation++
	generation := m.generation
	if m.cancel != nil {
		m.cancel()
		m.dirty = true
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return toolReply{generation: generation, err: err, session: true} })
}
func (m *model) uploadResult() tea.Cmd {
	f := m.flow
	d := f.daily
	if d.session == nil || d.session.State != "active" {
		f.err = "Renew the retained session against a fresh review first."
		return nil
	}
	if d.uploadID == "" {
		id, err := newID()
		if err != nil {
			return nil
		}
		d.uploadID = id
	}
	w := m.client.(Everyday)
	session, id, limit := *d.session, d.uploadID, d.limit
	f.task = "day_upload"
	f.work = func(ctx context.Context) (tc.Result, error) { return w.UploadSessionResult(ctx, session, id, limit) }
	return m.invalidate()
}
func (m *model) commitDaily() tea.Cmd {
	d := m.flow.daily
	if d.review == nil {
		return nil
	}
	p := tc.ContentIntent{Context: d.review.Context, Review: d.review.Review, Heads: d.review.Heads, Action: d.action, Source: d.source, Destination: d.destination, Copies: d.copies}
	if d.review.DestinationReview != nil {
		p.DestinationReview = *d.review.DestinationReview
	}
	if d.action == "merge" && d.upload != nil && d.session != nil {
		p.Session = d.session.ID
		p.Upload = d.upload.ID
		p.Digest = d.upload.Digest
		p.Bytes = d.upload.Bytes
	}
	return m.dailyMutation("day_commit", tc.Mutation{Kind: "content", Content: &p})
}
func (m *model) prepareCopies() tea.Cmd {
	f := m.flow
	d := f.daily
	review := *d.review
	versions := slices.Clone(d.versions)
	destinations := make([]string, len(f.fields))
	for i := range f.fields {
		destinations[i] = f.fields[i].input.Value()
	}
	client := m.client
	f.task = "day_copies"
	f.work = func(ctx context.Context) (tc.Result, error) {
		copies := []tc.CopyPlan{}
		i := 0
		for _, v := range versions {
			if v.Kind != "file" {
				continue
			}
			dst := destinations[i]
			i++
			r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "content_review", Folder: review.Context.Folder, Path: review.Context.Path, Source: &v.Version, Destination: dst})
			if err != nil || r.Error != nil {
				return r, err
			}
			if r.ContentReview == nil || r.ContentReview.DestinationReview == nil {
				return tc.Result{Error: &tc.Error{Code: "STALE_VIEW"}}, nil
			}
			copies = append(copies, tc.CopyPlan{Source: v.Version, Destination: dst, Review: *r.ContentReview.DestinationReview})
		}
		// Return plans as a private intent in the correlated reply, not shared state.
		return tc.Result{CopyPlans: copies}, nil
	}
	return m.invalidate()
}
func (m *model) dailyKey(msg tea.KeyPressMsg) tea.Cmd {
	f := m.flow
	d := f.daily
	k := msg.String()
	if k == "ctrl+c" {
		return m.quit()
	}
	text := f.screen == "day_destination" || f.screen == "day_copies" || (f.screen == "day_paths" && len(f.fields) > 0 && f.fields[0].input.Focused())
	if text {
		if f.busy {
			return nil
		}
		switch k {
		case "esc":
			for i := range f.fields {
				f.fields[i].input.Blur()
			}
			if f.screen == "day_paths" {
				return nil
			}
			f.screen = "day_review"
			return nil
		case "tab", "shift+tab", "down", "up":
			delta := 1
			if k == "shift+tab" || k == "up" {
				delta = -1
			}
			return m.focusField((f.focus + delta + len(f.fields)) % len(f.fields))
		case "enter":
			// Enter advances through fields and confirms on the last (E03).
			if f.screen == "day_copies" && f.focus < len(f.fields)-1 {
				return m.focusField(f.focus + 1)
			}
			if f.screen == "day_destination" {
				d.destination = f.fields[0].input.Value()
				return m.loadReview("separate_copy")
			}
			if f.screen == "day_copies" {
				return m.prepareCopies()
			}
			f.fields[0].input.Blur()
			f.cursor = ""
			f.selected = 0
			d.selectedKey = ""
			return m.invalidate()
		}
		var cmd tea.Cmd
		f.fields[f.focus].input, cmd = f.fields[f.focus].input.Update(msg)
		return cmd
	}
	if k == "esc" {
		return m.closeFlow()
	}
	if k == "q" {
		return m.quit()
	}
	if k == "?" {
		f.advanced = !f.advanced
		return nil
	}
	if f.busy || m.toolRunning {
		return nil
	}
	switch f.screen {
	case "day_status", "day_storage", "day_maintenance", "day_operation":
		if k == "r" {
			if f.screen == "day_operation" && f.result.Operation != nil && f.result.Operation.State == "pending" && d.mutation.Kind == "content" && d.mutation.OperationID == f.operation {
				return m.dailyMutation("day_commit", d.mutation)
			}
			return m.startQuery()
		}
		if k == "j" || k == "down" {
			f.scroll++
		}
		if k == "k" || k == "up" {
			f.scroll = max(0, f.scroll-1)
		}
		if f.screen == "day_storage" && k == "m" {
			f.screen = "day_maintenance"
			return m.invalidate()
		}
	case "day_paths", "day_history", "day_deleted", "day_conflicts":
		count := len(f.result.Items)
		if f.screen == "day_history" {
			count = len(f.result.Versions)
		}
		if f.screen == "day_conflicts" {
			count = len(f.result.Attention)
		}
		if k == "j" || k == "down" {
			f.selected = min(f.selected+1, max(0, count-1))
		}
		if k == "k" || k == "up" {
			f.selected = max(0, f.selected-1)
		}
		if count > 0 {
			d.selectedKey = dailyItemKey(f, f.selected)
		}
		if k == "/" && f.screen == "day_paths" {
			return m.focusField(0)
		}
		if k == "r" {
			return m.startQuery()
		}
		if k == "]" && f.result.Cursor != "" {
			f.cursor = f.result.Cursor
			f.selected = 0
			d.selectedKey = ""
			return m.invalidate()
		}
		if k == "[" {
			f.cursor = ""
			return m.invalidate()
		}
		if k == "enter" && count > 0 {
			switch f.screen {
			case "day_paths", "day_deleted":
				d.path = f.result.Items[f.selected].ID
				f.screen = "day_history"
				f.cursor = ""
				f.selected = 0
				d.selectedKey = ""
				return m.invalidate()
			case "day_history":
				v := f.result.Versions[f.selected]
				if v.Kind != "file" || !available(v) {
					f.err = "CONTENT_UNAVAILABLE: " + safe(v.Availability) + "; restore requires retained verified file bytes. Peer fetch unsupported."
					return nil
				}
				d.source = v.Version
				return m.loadReview("restore")
			case "day_conflicts":
				d.path = f.result.Attention[f.selected].Path
				return m.loadReview("select")
			}
		}
	case "day_load_review":
		if k == "r" {
			return m.startQuery()
		}
	case "day_review":
		if k == "r" {
			d.source = tc.VersionID{}
			return m.loadReview(d.action)
		}
		if k == "j" || k == "down" {
			f.selected = min(f.selected+1, max(0, len(d.versions)-1))
		}
		if k == "k" || k == "up" {
			f.selected = max(0, f.selected-1)
		}
		if d.action == "select" && len(d.versions) > 0 {
			d.source = d.versions[f.selected].Version
		}
		if k == "e" || k == "d" {
			kind := "edit"
			if k == "d" {
				kind = "diff"
			}
			return m.createSession(kind)
		}
		if k == "h" {
			f.screen = "day_history"
			f.cursor = ""
			return m.invalidate()
		}
		if k == "c" && d.source.Counter != 0 {
			f.screen = "day_destination"
			f.fields = []field{newField("Separate synced copy path", d.path+".recovered", false)}
			return m.focusField(0)
		}
		if k == "K" {
			f.fields = nil
			for i, v := range d.versions {
				if v.Kind == "file" {
					if !available(v) {
						f.err = "CONTENT_UNAVAILABLE: keep copies needs verified file sources."
						return nil
					}
					f.fields = append(f.fields, newField("Copy for "+v.DeviceName, fmt.Sprintf("%s.copy-%d", d.path, i+1), false))
				}
			}
			if len(f.fields) == 0 {
				f.err = "No file sources to copy."
				return nil
			}
			f.screen = "day_copies"
			return m.focusField(0)
		}
		if k == "enter" {
			v := d.review.Source
			if d.action == "select" {
				for i := range d.versions {
					if d.versions[i].Version == d.source {
						v = &d.versions[i]
						break
					}
				}
			}
			if v == nil || (v.Kind == "file" && !available(*v)) {
				f.err = "CONTENT_UNAVAILABLE: selected bytes unavailable; peer fetch unsupported."
				return nil
			}
			f.screen = "day_confirm"
			f.scroll = 0
		}
	case "day_confirm":
		if k == "enter" {
			return m.commitDaily()
		}
		if k == "r" {
			d.upload = nil
			return m.loadReview(d.action)
		}
		if k == "j" || k == "down" {
			f.scroll++
		}
		if k == "k" || k == "up" {
			f.scroll = max(0, f.scroll-1)
		}
	case "day_editor":
		if k == "e" || k == "d" {
			kind := "edit"
			if k == "d" {
				kind = "diff"
			}
			return m.launchSessionTool(kind)
		}
		if k == "u" {
			return m.uploadResult()
		}
		if k == "r" {
			return m.loadReview("merge")
		}
	case "day_recovery":
		if k == "e" {
			if d.session != nil {
				f.notice = "Former result retained at " + safe(d.session.ResultPath) + "; new editor session uses fresh reviewed sources."
			}
			d.session = nil
			d.upload = nil
			d.uploadID = ""
			d.mutation = tc.Mutation{}
			return m.createSession("edit")
		}
		if (k == "n" || k == "x") && d.session != nil && d.review != nil {
			action := "renew"
			if k == "x" {
				action = "discard"
			}
			p := &tc.SessionIntent{Action: action, ID: d.session.ID, Context: d.review.Context, Review: d.review.Review, Heads: d.review.Heads, Sources: d.session.Sources}
			if action == "renew" {
				p.Context = d.session.Context
				p.Review = d.session.Review
				p.Heads = d.session.Heads
			}
			return m.dailyMutation("day_"+action, tc.Mutation{Kind: "session", Session: p})
		}
		if k == "r" {
			return m.loadReview("merge")
		}
	}
	return nil
}
