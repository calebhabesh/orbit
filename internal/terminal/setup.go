package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

// Workflow operations are the same shared-client controls used by the CLI.
// Only forms and selection live here; durable phases and authorization do not.
type Workflows interface {
	Queries
	Mutate(context.Context, tc.Mutation) (tc.Result, error)
	Setup(context.Context, tc.Mutation) (tc.Result, error)
	ManageFolder(context.Context, string, string, string, string) error
	RetirementPreview(context.Context, string, string) (control.RetireDevicePreviewResult, error)
	SaveInvitation(context.Context, string, tc.Invitation) error
}
type field struct {
	label string
	input textinput.Model
}
type workflow struct {
	screen, kind, folder, device, operation, cursor string
	fields                                          []field
	focus, selected, scroll                         int
	advanced, reveal                                bool
	settings                                        tc.Settings
	plan                                            tc.SetupIntent
	invitation                                      tc.Invitation
	mutation                                        tc.Mutation
	request                                         tc.EnrollmentRequest
	result                                          tc.Result
	items                                           []tc.NamedItem
	retirement                                      *control.RetireDevicePreviewResult
	err, notice                                     string
	work                                            func(context.Context) (tc.Result, error)
	task                                            string
	busy                                            bool
}

func newField(label, value string, secret bool) field {
	in := textinput.New()
	in.Prompt = ""
	in.SetValue(value)
	in.SetWidth(48)
	in.CharLimit = 256
	in.KeyMap.Paste.SetEnabled(false)
	if secret {
		in.CharLimit = 16384
		in.EchoMode = textinput.EchoPassword
		in.EchoCharacter = '*'
	}
	return field{label, in}
}
func newID() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func (m *model) workflows() (Workflows, bool) { w, ok := m.client.(Workflows); return w, ok }
func (m *model) openFlow(w *workflow) tea.Cmd {
	if _, ok := m.workflows(); !ok {
		m.notice = "Workflow controls unavailable."
		return nil
	}
	m.search.Blur()
	m.flow = w
	m.notice = ""
	return m.invalidate()
}
func (m *model) closeFlow() tea.Cmd { m.flow = nil; return m.invalidate() }
func (m *model) setupForm(kind string) tea.Cmd {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	if host == "" {
		host = "Orbit Device"
	}
	return m.openFlow(&workflow{screen: "load_settings", kind: kind, fields: []field{
		newField("Device name", host, false), newField("Folder name", "Orbit", false), newField("Local root", filepath.Join(home, "Orbit"), false),
	}})
}
func (m *model) flowCommand(ctx context.Context) (string, func() (tc.Result, error)) {
	f := m.flow
	w, _ := m.workflows()
	if f.work != nil {
		work, task := f.work, f.task
		f.work = nil
		return task, func() (tc.Result, error) { return work(ctx) }
	}
	q := tc.Query{Version: tc.Version, Limit: pageSize, Cursor: f.cursor}
	switch f.screen {
	case "load_settings":
		q.Kind = "settings"
	case "preview":
		q.Kind = "root_preview"
		q.Path = f.plan.Root
		q.Name = f.kind
		p := f.plan
		q.RootPlan = &p
	case "progress":
		q.Kind = "operation"
		q.ID = f.operation
		return "progress", func() (tc.Result, error) {
			r, err := w.Query(ctx, q)
			if err == nil && r.Error == nil && r.Operation != nil && r.Operation.State == "completed" && r.Join != nil {
				current, e := w.Query(ctx, tc.Query{Version: tc.Version, Kind: "status", Folder: r.Join.Folder, Limit: pageSize})
				if e != nil {
					return r, e
				}
				if current.Error != nil {
					return current, nil
				}
				r.Readiness = current.Readiness
				r.Attention = current.Attention
			}
			return r, err
		}
	case "requests", "approval":
		q.Kind = "requests"
		q.Folder = f.folder
	case "pick_folder", "setups":
		q.Kind = "folders"
		if f.screen == "setups" {
			q.Kind = "setups"
		}
	case "pick_device":
		q.Kind = "devices"
	case "folder", "invite_review", "retire":
		q.Kind = "folder_management"
		q.Folder = f.folder
	default:
		return "", nil
	}
	task := f.screen
	return task, func() (tc.Result, error) { return w.Query(ctx, q) }
}

var errExpiredInvitation = errors.New("INVITATION_EXPIRED")

func (m *model) acceptFlow(task string, r tc.Result, err error) tea.Cmd {
	f := m.flow
	f.busy = false
	if err != nil || r.Error != nil {
		f.err = workflowError(r, err)
		if r.Operation != nil {
			f.result = r
			f.operation = r.Operation.ID
			f.screen = "progress"
		}
		if task == "preview" {
			f.screen = "form"
		}
		return nil
	}
	if task != "approval" {
		f.err = ""
	}
	switch task {
	case "load_settings":
		if r.Settings == nil {
			f.err = "Settings unavailable; use orbit doctor."
			return nil
		}
		f.settings = *r.Settings
		s := f.settings
		f.fields = append(f.fields, newField("Startup (manual/login/unattended)", s.Startup, false), newField("Data budget (bytes)", fmt.Sprint(s.DataBudget), false),
			newField("Peer listen (IP:port)", s.PeerListen, false), newField("Enrollment listen (IP:port)", s.EnrollmentListen, false),
			newField("Advertised peer (IP:port)", s.AdvertisedPeer, false), newField("Advertised enrollment (IP:port)", s.AdvertisedEnrollment, false),
			newField("Metadata budget (bytes)", fmt.Sprint(s.MetadataBudget), false), newField("Free space reserve (bytes)", fmt.Sprint(s.ReserveBytes), false),
			newField("Concurrency (1-32)", fmt.Sprint(s.Concurrency), false), newField("Bandwidth (bytes/sec; 0 unlimited)", fmt.Sprint(s.BandwidthBytesPerSecond), false))
		if f.kind == "join" {
			f.screen = "invitation"
			f.fields = append(f.fields, newField("Private invitation", "", true))
			f.focus = len(f.fields) - 1
		} else {
			f.screen = "form"
		}
		f.fields[f.focus].input.Focus()
	case "parse_invitation":
		f.invitation = *r.Invitation
		f.screen = "form"
		f.focus = 0
		return m.focusField(0)
	case "retirement_preview":
		f.screen = "retirement_preview"
		f.result = r
	case "preview":
		f.result = r
		if r.Preview == nil {
			f.err = "Root preview unavailable."
			f.screen = "form"
			return nil
		}
		if !r.Preview.Complete {
			f.cursor = r.Cursor
			return m.startQuery()
		}
		f.cursor = ""
		if r.Preview.Unsupported != 0 || r.Preview.Unreadable != 0 {
			f.screen = "form"
			f.err = "ROOT_REVIEW_INCOMPLETE: correct unsupported/unreadable objects, then review again."
			return nil
		}
		if r.Review == nil {
			f.err = "Root review missing."
			return nil
		}
		f.plan.Preview = *r.Review
		f.screen = "review"
	case "submit":
		f.result = r
		if r.Operation == nil {
			f.err = "Submission outcome unavailable; retry the same reviewed request."
			return nil
		}
		f.operation = r.Operation.ID
		f.screen = "progress"
		for i := range f.fields {
			if f.fields[i].label == "Private invitation" {
				f.fields[i].input.SetValue("")
			}
		}
	case "progress":
		f.result = r
	case "pick_folder", "pick_device", "setups":
		f.items = r.Items
		f.result = r
		f.selected = min(f.selected, max(0, len(f.items)-1))
	case "requests":
		f.result = r
		f.selected = min(f.selected, max(0, len(r.Requests)-1))
	case "approval":
		// Never silently replace a reviewed request with a freshly polled one.
	case "approve", "decline":
		f.result = r
		f.screen = "approval_done"
		f.notice = "Exact request " + task + " completed. Other devices may still need membership updates."
	case "invite_review":
		f.result = r
	case "invite":
		if r.Invitation == nil {
			f.err = "Invitation unavailable; retry the same operation."
			return nil
		}
		f.invitation = *r.Invitation
		f.screen = "invitation_out"
		f.result = r
	case "save_invitation":
		f.screen = "invitation_out"
		f.notice = "Private invitation saved. Transfer deliberately to the receiving device."
	case "folder":
		f.result = r
	case "pause", "resume", "relocate":
		f.notice = "Local folder action completed: " + task
		f.screen = "folder"
		return m.startQuery()
	case "retire":
		f.result = r
	}
	return nil
}
func workflowError(r tc.Result, err error) string {
	code := "CONTROL_UNAVAILABLE"
	if r.Error != nil {
		code = r.Error.Code
	} else {
		var e *control.ControlError
		if errors.As(err, &e) {
			code = e.Code
		} else if err != nil {
			// Only known categories are displayed; transport errors may contain secrets.
			for _, c := range []string{"IDENTITY_MISMATCH", "INVALID_REQUEST", "STALE_VIEW", "STORAGE_BLOCKED", "NETWORK_RESTART_REQUIRED", "IDEMPOTENCY_CONFLICT", "INVITATION_EXPIRED"} {
				if strings.Contains(err.Error(), c) {
					code = c
					break
				}
			}
		}
	}
	action := "Retry; use orbit doctor to inspect local control/network reachability."
	switch code {
	case "IDENTITY_MISMATCH":
		action = "Stop and verify the inviter identity; request a new invitation from the intended device."
	case "EXPIRED_ATTEMPT", "INVITATION_EXPIRED", "EXPIRED_OR_DECLINED_ATTEMPT", "EXPIRED_REPLAY", "INVITATION_INVALID":
		action = "Ask the inviter for a new invitation; explicitly review a new join attempt."
	case "STALE_VIEW", "STALE_ROOT", "INVALID_ROOT", "ROOT_REVIEW_INCOMPLETE":
		action = "Edit/correct the root or membership and obtain a fresh review."
	case "STORAGE_BLOCKED", "LIMITS_REVIEW_REQUIRED":
		action = "Review finite budgets, reserve and available space; keep existing files."
	case "MEMBERSHIP_FORK", "MEMBERSHIP_BEHIND":
		action = "Keep work pending; use the membership-fork recovery runbook."
	case "SYSTEMD_UNAVAILABLE", "UNATTENDED_PREREQUISITE", "STARTUP_REVIEW_REQUIRED", "SERVICE_SELECTION_REQUIRED":
		action = "Inspect startup prerequisites with orbit doctor; manual startup remains available."
	case "NETWORK_RESTART_REQUIRED":
		action = "Restart the selected daemon, then reopen the durable operation."
	}
	return safe(code + ": " + action)
}
func (m *model) focusField(i int) tea.Cmd {
	f := m.flow
	for j := range f.fields {
		f.fields[j].input.Blur()
	}
	f.focus = i
	return f.fields[i].input.Focus()
}
func (m *model) previewSetup() tea.Cmd {
	f := m.flow
	get := func(i int) string { return strings.TrimSpace(f.fields[i].input.Value()) }
	s := f.settings
	s.Startup = get(3)
	s.PeerListen = get(5)
	s.EnrollmentListen = get(6)
	s.AdvertisedPeer = get(7)
	s.AdvertisedEnrollment = get(8)
	for _, v := range []struct {
		i int
		n *tc.Uint
	}{{4, &s.DataBudget}, {9, &s.MetadataBudget}, {10, &s.ReserveBytes}, {11, &s.Concurrency}, {12, &s.BandwidthBytesPerSecond}} {
		n, err := strconv.ParseUint(get(v.i), 10, 64)
		if err != nil {
			f.err = "Enter a nonnegative byte/count value: " + f.fields[v.i].label
			return m.focusField(v.i)
		}
		*v.n = tc.Uint(n)
	}
	if get(0) == "" || get(1) == "" || !filepath.IsAbs(get(2)) {
		f.err = "Device/folder names and an absolute local root are required."
		return nil
	}
	if err := config.ValidateRuntimeSettings(s); err != nil {
		f.err = "Review startup, finite positive budgets and numeric network addresses (advertise a reachable LAN/Tailscale IP)."
		return nil
	}
	f.plan = tc.SetupIntent{DeviceName: get(0), FolderName: get(1), Root: filepath.Clean(get(2)), Settings: s}
	q := tc.Query{Version: tc.Version, Kind: "root_preview", Path: f.plan.Root, Name: f.kind, RootPlan: &f.plan}
	if err := q.Validate(); err != nil {
		f.err = "Names must fit 256 bytes and the reviewed settings must be finite."
		return nil
	}
	f.screen = "preview"
	f.cursor = ""
	f.err = ""
	f.mutation = tc.Mutation{}
	return m.invalidate()
}
func (m *model) submitSetup() tea.Cmd {
	f := m.flow
	w, _ := m.workflows()
	if f.mutation.OperationID == "" {
		id, err := newID()
		if err != nil {
			f.err = "Could not allocate operation identity."
			return nil
		}
		f.mutation = tc.Mutation{Version: tc.Version, OperationID: id, Kind: f.kind}
		if f.kind == "join" {
			attempt, err := newID()
			if err != nil {
				f.err = "Could not allocate attempt."
				return nil
			}
			p := f.plan
			f.mutation.Join = &tc.JoinIntent{Invitation: f.invitation, Attempt: attempt, DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings}
		} else {
			p := f.plan
			f.mutation.Setup = &p
		}
	}
	mutation := f.mutation
	f.task = "submit"
	f.work = func(ctx context.Context) (tc.Result, error) { return w.Setup(ctx, mutation) }
	return m.invalidate()
}
func (m *model) formKey(msg tea.KeyPressMsg) tea.Cmd {
	f := m.flow
	k := msg.String()
	switch k {
	case "esc":
		if f.screen == "invitation" {
			return m.closeFlow()
		}
		if f.screen == "save_invitation" {
			f.screen = "invitation_out"
			return nil
		}
		if f.screen == "relocate_form" {
			f.screen = "folder"
			return m.invalidate()
		}
		f.screen = "welcome"
		return nil
	case "ctrl+a":
		f.advanced = !f.advanced
		return nil
	case "tab", "shift+tab":
		n := len(f.fields)
		if f.screen == "invitation" {
			return nil
		}
		if f.screen == "form" {
			n = 9
			if f.advanced {
				n = 13
			}
		}
		delta := 1
		if k == "shift+tab" {
			delta = -1
		}
		return m.focusField((f.focus + delta + n) % n)
	case "enter":
		switch f.screen {
		case "invitation":
			return m.parseInvitation()
		case "form":
			return m.previewSetup()
		case "save_invitation":
			w, _ := m.workflows()
			path := f.fields[0].input.Value()
			inv := f.invitation
			f.task = "save_invitation"
			f.work = func(ctx context.Context) (tc.Result, error) { return tc.Result{}, w.SaveInvitation(ctx, path, inv) }
			return m.invalidate()
		case "relocate_form":
			f.screen = "relocate_review"
			return nil
		}
	}
	var cmd tea.Cmd
	f.fields[f.focus].input, cmd = f.fields[f.focus].input.Update(msg)
	return cmd
}
func (m *model) flowKey(msg tea.KeyPressMsg) tea.Cmd {
	f := m.flow
	k := msg.String()
	if k == "ctrl+c" {
		return m.quit()
	}
	if f.screen == "form" || f.screen == "invitation" || f.screen == "save_invitation" || f.screen == "relocate_form" {
		if f.busy {
			if k == "esc" {
				return m.closeFlow()
			}
			return nil
		}
		return m.formKey(msg)
	}
	if k == "esc" {
		if f.screen == "review" {
			f.screen = "form"
			return m.focusField(f.focus)
		}
		if f.screen == "relocate_review" {
			f.screen = "relocate_form"
			return m.focusField(0)
		}
		return m.closeFlow()
	}
	if k == "q" {
		return m.quit()
	}
	if f.busy || (m.pending != 0 && f.screen == "preview") {
		return nil
	}
	if f.screen != "pick_folder" && f.screen != "pick_device" && f.screen != "setups" && f.screen != "requests" && f.screen != "invitation_out" && f.screen != "welcome" {
		if k == "down" || k == "j" {
			f.scroll++
			return nil
		}
		if k == "up" || k == "k" {
			f.scroll = max(0, f.scroll-1)
			return nil
		}
	}
	switch f.screen {
	case "welcome":
		if k == "c" || k == "enter" {
			return m.setupForm("setup")
		}
		if k == "j" {
			return m.setupForm("join")
		}
	case "review":
		if k == "enter" {
			return m.submitSetup()
		}
	case "progress":
		if k == "r" {
			return m.startQuery()
		}
		if k == "J" {
			return m.setupForm("join")
		}
	case "pick_folder", "pick_device", "setups":
		return m.pickerKey(k)
	case "requests":
		return m.requestsKey(k)
	case "approval":
		if k == "a" || k == "x" {
			return m.decideRequest(k == "a")
		}
	case "invite_review":
		if k == "enter" {
			return m.makeInvitation()
		}
	case "invitation_out":
		if k == "v" {
			f.reveal = !f.reveal
			f.scroll = 0
		}
		if k == "j" || k == "down" {
			f.scroll++
		}
		if k == "k" || k == "up" {
			f.scroll = max(0, f.scroll-1)
		}
		if k == "s" {
			f.fields = []field{newField("Private output file (absolute)", "", false)}
			f.focus = 0
			f.screen = "save_invitation"
			return m.focusField(0)
		}
	case "folder":
		return m.folderKey(k)
	case "folder_action":
		if k == "enter" {
			return m.manageFolder(f.task)
		}
	case "relocate_review":
		if k == "enter" {
			return m.manageFolder("relocate")
		}
	}
	return nil
}

// One serialized lane runs both queries and explicit actions. Replies remain
// matched to request+generation even when Esc abandons a submitted operation.
func (m *model) startFlowQuery() tea.Cmd {
	f := m.flow
	if m.pending != 0 || m.quitting {
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	task, run := m.flowCommand(ctx)
	if run == nil {
		cancel()
		return nil
	}
	m.dirty = false
	m.request++
	m.pending = m.request
	m.cancel = cancel
	request, generation := m.request, m.generation
	// A task is consumed exactly once. Polls never repeat a mutation.
	if f.task == task && f.work == nil {
		f.task = ""
		f.busy = true
	}
	return func() tea.Msg {
		defer cancel()
		r, err := run()
		r.Items = r.Items[:min(len(r.Items), pageSize)]
		r.Requests = r.Requests[:min(len(r.Requests), pageSize)]
		r.Attention = r.Attention[:min(len(r.Attention), pageSize)]
		r.Observations = r.Observations[:min(len(r.Observations), pageSize)]
		return queryReply{request: request, generation: generation, result: r, err: err, task: task}
	}
}
