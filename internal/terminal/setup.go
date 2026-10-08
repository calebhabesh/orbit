package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
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
	RetryWork(context.Context, control.WorkRetryRequest) (*control.WorkRetryResult, error)
}
type field struct {
	label string
	input textinput.Model
	// choices makes the field a selector: ←/→ change it, typing is ignored.
	choices []string
}
type workflow struct {
	daily                                           *dailyState
	screen, kind, folder, device, operation, cursor string
	fields                                          []field
	focus, selected, scroll                         int
	advanced, reveal                                bool
	settings                                        tc.Settings
	network                                         tc.NetworkPolicy
	builtin                                         *tc.BuiltinProfile
	host                                            *tc.HostStartup
	replaceOnType                                   bool
	saved                                           string
	// joinPolicy is the connection the invitation proposes (F08).
	joinPolicy  *tc.NetworkPolicy
	plan        tc.SetupIntent
	invitation  tc.Invitation
	mutation    tc.Mutation
	request     tc.EnrollmentRequest
	result      tc.Result
	items       []tc.NamedItem
	retirement  *control.RetireDevicePreviewResult
	err, notice string
	work        func(context.Context) (tc.Result, error)
	task        string
	busy        bool
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
	return field{label: label, input: in}
}

// newSelector is a fixed-choice field shown as ‹ value › (E03). The value
// stays in input so existing readers keep working.
func newSelector(label, value string, choices []string) field {
	f := newField(label, value, false)
	f.choices = choices
	return f
}

// cycle moves a selector to the next (delta 1) or previous choice.
func (f *field) cycle(delta int) {
	i := slices.Index(f.choices, f.input.Value())
	if i < 0 {
		i = 0
		delta = 0
	}
	f.input.SetValue(f.choices[(i+delta+len(f.choices))%len(f.choices)])
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
	if f.daily != nil {
		return m.dailyCommand(ctx)
	}
	q := tc.Query{Version: tc.Version, Limit: pageSize, Cursor: f.cursor}
	switch f.screen {
	case "load_settings":
		return "load_settings", func() (tc.Result, error) {
			r, err := w.Query(ctx, tc.Query{Version: tc.Version, Kind: "settings"})
			if err != nil || r.Error != nil {
				return r, err
			}
			n, err := w.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
			if err != nil || n.Error != nil {
				return n, err
			}
			r.Network = n.Network
			return r, nil
		}
	case "network":
		q.Kind = "network_status"
		q.ID = f.device
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
			if err == nil && r.Error == nil {
				n, e := w.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
				if e == nil && n.Error == nil {
					r.Network = n.Network
				}
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
	if f.daily != nil {
		return m.acceptDaily(task, r, err)
	}
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
			// A rejected root puts the cursor on the root field (E03).
			if code := strings.SplitN(f.err, ":", 2)[0]; strings.Contains(code, "ROOT") && len(f.fields) > 2 {
				return m.focusField(2)
			}
		}
		if task == "parse_invitation" && f.screen == "invitation" {
			// The failed value stays (W07), but the next typed character starts
			// a fresh value, so an unbracketed re-paste replaces it (F06).
			f.replaceOnType = true
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
		f.host = r.Host
		f.network = tc.NetworkPolicy{Mode: "manual", Generation: 1}
		if r.Network != nil {
			f.network = r.Network.Policy
			if b := r.Network.Builtin; b != nil && !b.Expired {
				f.builtin = b
			}
		}
		mode := f.network.Mode
		// Fresh devices, and devices with no folders yet when a packaged
		// profile exists, are offered Automatic (F08).
		if mode == "manual" && (m.opts.FreshInstall || (len(m.result.Items) == 0 && f.builtin != nil)) {
			mode = "automatic"
		}
		f.result.Network = r.Network
		s := f.settings
		f.fields = append(f.fields, newSelector("Startup", s.Startup, []string{"manual", "login", "unattended"}), newField("Data budget (bytes)", fmt.Sprint(s.DataBudget), false),
			newField("Peer listen (IP:port)", s.PeerListen, false), newField("Enrollment listen (IP:port)", s.EnrollmentListen, false),
			newField("Advertised peer (IP:port)", s.AdvertisedPeer, false), newField("Advertised enrollment (IP:port)", s.AdvertisedEnrollment, false),
			newField("Metadata budget (bytes)", fmt.Sprint(s.MetadataBudget), false), newField("Free space reserve (bytes)", fmt.Sprint(s.ReserveBytes), false),
			newField("Concurrency (1-32)", fmt.Sprint(s.Concurrency), false), newField("Bandwidth (bytes/sec; 0 unlimited)", fmt.Sprint(s.BandwidthBytesPerSecond), false))
		f.fields = append(f.fields, newSelector("Connection", mode, []string{"automatic", "local_only", "manual", "self_hosted"}))
		if f.kind == "join" {
			f.screen = "invitation"
			f.fields = append(f.fields, newField("Private invitation", "", true))
			f.focus = len(f.fields) - 1
		} else {
			f.screen = "form"
		}
		f.fields[f.focus].input.Focus()
	case "retry_work":
		m.notice = "Work re-queued; attention clears when it completes."
		return m.closeFlow()
	case "recheck_host":
		// Re-check after the owner ran the linger command; Orbit never runs it.
		f.host = r.Host
		if r.Host != nil && r.Host.Suggested == "unattended" && f.fields[3].input.Value() == "login" {
			f.fields[3].input.SetValue("unattended")
		}
	case "parse_invitation":
		f.invitation = *r.Invitation
		// The invitation proposes the connection: the inviter's operator for a
		// routed invitation (F08). The owner can still change it in the form.
		p := tc.JoinPolicy(f.invitation, f.network, f.builtin, m.opts.FreshInstall || len(m.result.Items) == 0)
		f.joinPolicy = &p
		f.fields[13].input.SetValue(p.Mode)
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
			return m.focusField(2)
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
		m.opts.FreshInstall = false
		f.screen = "progress"
		for i := range f.fields {
			if f.fields[i].label == "Private invitation" {
				f.fields[i].input.SetValue("")
			}
		}
	case "progress", "network":
		f.result = r
	case "pick_folder", "pick_device", "setups":
		selected := ""
		if f.selected < len(f.items) {
			selected = f.items[f.selected].ID
		}
		f.items = r.Items
		for i, it := range f.items {
			if it.ID == selected {
				f.selected = i
				break
			}
		}
		f.result = r
		f.selected = min(f.selected, max(0, len(f.items)-1))
	case "requests":
		selected := ""
		if f.selected < len(f.result.Requests) {
			selected = f.result.Requests[f.selected].ID
		}
		for i, it := range r.Requests {
			if it.ID == selected {
				f.selected = i
				break
			}
		}
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
	case "revoke_invitation":
		f.reveal = false
		f.invitation.Capability = ""
		f.screen = "invitation_revoked"
		f.notice = "Invitation revoked; request a fresh invitation for a new attempt."
	case "save_invitation":
		f.screen = "invitation_out"
		f.saved = f.fields[0].input.Value()
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
	// The daemon's own next step for codes this table does not know (F13).
	daemonAction := ""
	if r.Error != nil {
		code = r.Error.Code
		daemonAction = r.Error.Action
	} else {
		var e *control.ControlError
		if errors.As(err, &e) {
			code = e.Code
			daemonAction = e.Action
		} else if err != nil {
			// Only known categories are displayed; transport errors may contain secrets.
			for _, c := range []string{"INVITATION_INCOMPLETE", "UNSUPPORTED_INVITATION_VERSION", "PROFILE_NOT_PACKAGED", "IDENTITY_MISMATCH", "INVALID_REQUEST", "STALE_VIEW", "STORAGE_BLOCKED", "NETWORK_RESTART_REQUIRED", "IDEMPOTENCY_CONFLICT", "INVITATION_EXPIRED"} {
				if strings.Contains(err.Error(), c) {
					code = c
					break
				}
			}
		}
	}
	// No generic "retry" advice (F13): each code names its next step; unknown
	// codes use the daemon's action, else say where the details are.
	action := daemonAction
	if action == "" {
		action = "This step failed; details are in Orbit's log (journalctl --user -u orbit, or daemon.log in the state directory) and orbit doctor."
	}
	switch code {
	case "CONTROL_UNAVAILABLE":
		action = "Orbit's background daemon did not answer; check it with orbit service status (log: journalctl --user -u orbit, or daemon.log in the state directory)."
	case "NOT_SET_UP":
		action = "Orbit is not set up on this device yet; create or join an Orbit first."
	case "INVALID_REQUEST":
		action = "Orbit rejected this input as malformed; check the values on this screen (an invitation must be pasted whole) and submit again."
	case "UNAUTHORIZED":
		action = "The local control credential was refused; quit and reopen orbit so it reads the current credential."
	case "PAYLOAD_TOO_LARGE":
		action = "The input is larger than Orbit accepts; check that only one invitation or file path was pasted."
	case "INTERNAL_ERROR", "IO_ERROR":
		action = "Orbit hit an internal or disk error during this step; details are in its log (journalctl --user -u orbit, or daemon.log in the state directory). Your reviewed input is kept."
	case "SETUP_BLOCKED":
		action = "Setup stopped before finishing; open the setup's progress for the blocking step. Your reviewed input is kept."
	case "QUOTA_EXCEEDED":
		action = "The Orbit service's limit for this device is used up for now; wait and resume the same operation later."
	case "IDEMPOTENCY_CONFLICT":
		action = "This operation ID was already used for different input; start the step again from its screen."
	case "UNSUPPORTED_CAPABILITY", "INCOMPATIBLE_VERSION":
		action = "The running daemon is a different Orbit version; restart it (orbit service restart) and try again."
	case "HANDOVER_FAILED":
		action = "The background daemon could not be handed to the service; run orbit stop, then orbit service start."
	case "SERVICE_REVIEW_REQUIRED":
		action = "A service command was interrupted; check orbit service status before trying it again."
	case "INVITATION_INCOMPLETE":
		action = "The invitation code is incomplete or damaged; copy the whole line again (or transfer the invitation file instead)."
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
	case "MANUAL_DAEMON_RUNNING":
		action = "Sync keeps running; run orbit service start from a terminal to hand the daemon over to the service."
	case "SERVICE_UNAVAILABLE", "UNAVAILABLE", "DEVICE_OFFLINE":
		action = "Waiting for a connection; saved local work and the reviewed attempt are retained. Retry later or open Connection details."
	case "PROFILE_MISSING_OR_EXPIRED", "PROFILE_MISMATCH", "NETWORK_REVIEW_REQUIRED":
		action = "Review connection configuration with orbit network automatic (or orbit network set); keep names, root and existing identity."
	case "PROFILE_EPOCH_MISMATCH":
		action = "The two devices carry different service profile versions; update Orbit on the older device, then resume."
	case "PROFILE_OPERATOR_MISMATCH", "PROFILE_NOT_PACKAGED":
		action = "The devices use different Orbit service operators or versions; use the same operator on both, or transfer the invitation file."
	case "UNSUPPORTED_INVITATION_VERSION":
		action = "The invitation came from a newer Orbit; update Orbit on this device."
	case "RATE_LIMITED", "CAPACITY", "BUSY":
		action = "Connection service is busy or at its limit; retry the same operation later."
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
	for i := range 3 {
		if get(i) == "" || (i == 2 && !filepath.IsAbs(get(2))) {
			f.err = "Device/folder names and an absolute local root are required."
			// Focus the field with the problem (E03).
			return m.focusField(i)
		}
	}
	if err := config.ValidateRuntimeSettings(s); err != nil {
		f.err = "Review startup, finite positive budgets and numeric network addresses (advertise a reachable LAN/Tailscale IP)."
		return nil
	}
	policy := f.network
	if f.joinPolicy != nil && get(13) == f.joinPolicy.Mode {
		policy = *f.joinPolicy
	} else if len(f.fields) > 13 {
		policy.Mode = get(13)
		if policy.Mode != f.network.Mode || m.opts.FreshInstall {
			policy.LANAdvertising = policy.Mode != "manual"
		}
		if policy.Mode == "manual" || policy.Mode == "local_only" {
			policy.Profile = ""
		}
		if policy.Mode == "automatic" && policy.Profile == "" && f.builtin != nil {
			policy.Profile = f.builtin.Digest // packaged release profile, shown in the review
		}
		policy.AwaitingProfile = policy.Mode == "automatic" && policy.Profile == ""
		if err := policy.Validate(); err != nil {
			f.err = "Choose a connection mode; automatic/self-hosted profiles require independent network review (orbit network preview)."
			return m.focusField(13)
		}
		if policy != f.network {
			policy.Generation = f.network.Generation + 1
		}
	}
	f.plan = tc.SetupIntent{Network: &policy, DeviceName: get(0), FolderName: get(1), Root: filepath.Clean(get(2)), Settings: s}
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
			f.mutation.Join = &tc.JoinIntent{Invitation: f.invitation, Attempt: attempt, DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings, Network: p.Network}
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
		if f.screen == "form" && f.kind == "join" {
			f.screen = "invitation"
			return m.focusField(len(f.fields) - 1)
		}
		f.screen = "welcome"
		return nil
	case "ctrl+a":
		f.advanced = !f.advanced
		if !f.advanced && f.focus >= 5 && f.focus <= 12 {
			return m.focusField(0)
		}
		return nil
	case "ctrl+n":
		if f.screen == "form" {
			return m.cycleConnection()
		}
		return nil
	case "ctrl+r":
		if f.screen == "form" {
			w, _ := m.workflows()
			f.task = "recheck_host"
			f.work = func(ctx context.Context) (tc.Result, error) {
				return w.Query(ctx, tc.Query{Version: tc.Version, Kind: "settings"})
			}
			return m.invalidate()
		}
		return nil
	case "tab", "down":
		return m.moveField(1)
	case "shift+tab", "up":
		return m.moveField(-1)
	case "ctrl+u":
		// Clears the whole field, including a hidden invitation (F06).
		if f.fields[f.focus].choices == nil {
			f.fields[f.focus].input.SetValue("")
		}
		return nil
	case "enter":
		// Enter advances to the next field and confirms on the last (E03).
		if indices := m.formIndices(); f.screen != "invitation" && len(indices) > 1 && f.focus != indices[len(indices)-1] {
			return m.moveField(1)
		}
		switch f.screen {
		case "invitation":
			if strings.TrimSpace(f.fields[f.focus].input.Value()) == "" {
				f.err = "Paste the invitation code (or type a private file path) first."
				return nil
			}
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
	if fld := &f.fields[f.focus]; fld.choices != nil {
		// Selectors ignore typing; j/k still move between fields.
		switch k {
		case "left", "h":
			fld.cycle(-1)
		case "right", "l", "space":
			fld.cycle(1)
		case "j":
			return m.moveField(1)
		case "k":
			return m.moveField(-1)
		}
		return nil
	}
	if f.replaceOnType && msg.Text != "" {
		f.fields[f.focus].input.SetValue("")
	}
	f.replaceOnType = false
	var cmd tea.Cmd
	f.fields[f.focus].input, cmd = f.fields[f.focus].input.Update(msg)
	return cmd
}

// moveField focuses the next (delta 1) or previous visible field, wrapping.
func (m *model) moveField(delta int) tea.Cmd {
	indices := m.formIndices()
	for i, index := range indices {
		if index == m.flow.focus {
			return m.focusField(indices[(i+delta+len(indices))%len(indices)])
		}
	}
	return m.focusField(indices[0])
}
func (m *model) flowKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.flow.daily != nil {
		return m.dailyKey(msg)
	}
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
		if f.screen == "revoke_invitation_review" {
			f.screen = "invitation_out"
			return nil
		}
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
		if k == "r" && f.err != "" && f.mutation.OperationID != "" && (f.mutation.Kind == "setup" || f.mutation.Kind == "adopt" || f.mutation.Kind == "join") {
			return m.submitSetup()
		}
		if k == "a" {
			return m.openFlow(&workflow{screen: "pick_folder", kind: "invite"})
		}
		if k == "N" {
			return m.openFlow(&workflow{screen: "network"})
		}
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
			return m.revealInvitation()
		}
		if k == "c" {
			return m.copyInvitation()
		}
		if k == "j" || k == "down" {
			f.scroll++
		}
		if k == "k" || k == "up" {
			f.scroll = max(0, f.scroll-1)
		}
		if k == "x" {
			f.screen = "revoke_invitation_review"
			return nil
		}
		if k == "s" {
			f.fields = []field{newField("Private output file (absolute)", m.defaultInvitationPath(), false)}
			f.focus = 0
			f.screen = "save_invitation"
			return m.focusField(0)
		}
	case "revoke_invitation_review":
		if k == "enter" {
			return m.revokeInvitation()
		}
	case "network":
		if k == "d" {
			f.task = "network"
			device := f.device
			f.work = func(ctx context.Context) (tc.Result, error) {
				w, _ := m.workflows()
				return w.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_doctor", ID: device})
			}
			return m.startQuery()
		}
		if k == "r" {
			return m.startQuery()
		}
	case "folder":
		if k == "N" {
			return m.openFlow(&workflow{screen: "network", device: f.device})
		}
		return m.folderKey(k)
	case "folder_action":
		if k == "enter" {
			return m.manageFolder(f.task)
		}
	case "relocate_review":
		if k == "enter" {
			return m.manageFolder("relocate")
		}
	case "retry_review":
		if k == "enter" {
			return m.retryWork()
		}
	}
	return nil
}

// One serialized lane runs both queries and explicit actions. Replies remain
// matched to request+generation even when Esc abandons a submitted operation.
func (m *model) startFlowQuery() tea.Cmd {
	f := m.flow
	if m.pending != 0 || m.quitting || m.toolRunning {
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
		versionLimit := pageSize
		if task == "day_load_review" || task == "day_load_session" || task == "day_create_session" {
			versionLimit = 64
		}
		r.Versions = r.Versions[:min(len(r.Versions), versionLimit)]
		r.Items = r.Items[:min(len(r.Items), pageSize)]
		r.Requests = r.Requests[:min(len(r.Requests), pageSize)]
		r.Attention = r.Attention[:min(len(r.Attention), pageSize)]
		r.Observations = r.Observations[:min(len(r.Observations), pageSize)]
		return queryReply{request: request, generation: generation, result: r, err: err, task: task}
	}
}

// Ordinary fields never require numeric addresses. Advanced preserves all legacy settings.
func (m *model) formIndices() []int {
	f := m.flow
	if f.screen != "form" {
		indices := make([]int, len(f.fields))
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	indices := []int{0, 1, 2, 3, 4, 13}
	if f.advanced {
		indices = append(indices, 5, 6, 7, 8, 9, 10, 11, 12)
	}
	return indices
}
func (m *model) cycleConnection() tea.Cmd {
	m.flow.fields[13].cycle(1)
	return nil
}
