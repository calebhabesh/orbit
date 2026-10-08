package terminal

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) workflowView() tea.View {
	f := m.flow
	title := "Orbit | " + f.screen
	footer := "Esc back  q quit (daemon continues)"
	lines := []string{}
	focusLine := -1
	if f.err != "" {
		lines = append(lines, "Error: "+f.err)
	}
	if f.notice != "" {
		lines = append(lines, f.notice)
	}
	if f.daily != nil {
		var body []string
		title, body, footer, focusLine = m.dailyLines()
		if focusLine >= 0 {
			focusLine += len(lines)
		}
		lines = append(lines, body...)
	}
	switch f.screen {
	case "welcome":
		title = "Orbit | Create or join"
		if m.width >= 50 && m.height >= 20 {
			lines = append(lines, m.theme().banner()...)
		}
		lines = append(lines, "Create your Orbit [c / Enter]", "Join an existing Orbit [j]", "Existing folder contents are reviewed before adoption.", "Closing this interface leaves background sync running.")
	case "load_settings":
		title = "Orbit | Setup"
		lines = append(lines, "Loading actual finite/network/startup settings…")
	case "invitation":
		title = "Orbit | Join invitation"
		lines = append(lines, "Paste a private v2/v3 invitation or enter an absolute private file path.", "The invitation is hidden and never included in status/logs.")
		focusLine = len(lines)
		lines = append(lines, m.theme().field("Private invitation", f.fields[f.focus].input.View(), "", true))
		lines = append(lines, receivedLine(f.fields[f.focus].input.Value()))
		footer = "Enter verify  Ctrl-U clear  Esc back  Ctrl-C close"
	case "form":
		title = "Orbit | Review setup inputs"
		lines = append(lines, "Supported existing contents will become shared.", "Connection choices: Automatic or Local network only; Ctrl-N changes mode.",
			"Files received from other devices are saved owner-only (0600); the executable bit is kept.")
		lines = append(lines, hostStartupLines(f.host)...)
		for _, i := range m.formIndices() {
			value := safe(f.fields[i].input.Value())
			if i == f.focus {
				value = f.fields[i].input.View()
				focusLine = len(lines)
			}
			if f.fields[i].choices != nil {
				value = "‹ " + safe(f.fields[i].input.Value()) + " ›"
			}
			lines = append(lines, m.theme().field(f.fields[i].label, value, f.fields[i].input.Value(), i == f.focus))
		}
		lines = append(lines, "Metadata budget="+hb(f.settings.MetadataBudget)+" reserve="+hb(f.settings.ReserveBytes)+"; retention uses per-folder controls.")
		footer = "↑/↓ or Tab move  ←/→ change choice  Enter next/preview  Ctrl-R re-check startup  Ctrl-A advanced  Esc back"
	case "preview":
		title = "Orbit | Measuring root"
		lines = append(lines, "Bounded enumeration continues; adoption has not been confirmed.")
		if f.result.Preview != nil {
			lines = append(lines, previewLines(f.result.Preview)...)
		}
	case "review":
		title = "Orbit | Confirm adoption"
		p := f.plan
		s := p.Settings
		lines = append(lines, "Device: "+safe(p.DeviceName), "Folder: "+safe(p.FolderName), "Root: "+safe(p.Root), "Existing supported local contents become shared; nothing is erased.")
		if f.kind == "join" {
			lines = append(lines, "Inviter: "+safe(f.invitation.Inviter), "Key pin: "+safe(f.invitation.KeyPin), "Invitation expires: "+safe(f.invitation.ExpiresAt), "Invitation folder: "+safe(f.invitation.Folder))
		}
		if p.Network != nil {
			lines = append(lines, policyLines(*p.Network)...)
			if b := f.builtin; b != nil && p.Network.Profile == b.Digest {
				lines = append(lines, "Operator: "+safe(b.Operator)+" (packaged profile, expires "+safe(b.Expires)+")", "Privacy: "+safe(b.Privacy))
			}
		}
		if f.invitation.Profile != nil {
			same := f.network.Profile == f.invitation.Route.Profile || (f.builtin != nil && f.builtin.Digest == f.invitation.Route.Profile)
			if p.Network != nil && f.invitation.Route != nil && p.Network.Profile == f.invitation.Route.Profile && same {
				lines = append(lines, "Inviter operator: "+safe(f.invitation.Profile.Operator)+" (same operator and profile as this device)")
			} else if p.Network != nil && f.invitation.Route != nil && p.Network.Profile == f.invitation.Route.Profile {
				// Reviewing this screen accepts the inviter's operator (F08).
				lines = append(lines, "Inviter operator: "+safe(f.invitation.Profile.Operator)+" — confirming uses this operator on this device too", "Privacy: "+safe(f.invitation.Profile.Privacy))
			} else {
				lines = append(lines, "Inviter operator: "+safe(f.invitation.Profile.Operator), "Profile: "+safe(f.invitation.Route.Profile), "Select this operator independently with orbit network set before joining.")
			}
		}
		lines = append(lines, previewLines(f.result.Preview)...)
		lines = append(lines, fmt.Sprintf("Startup=%s; data=%s metadata=%s reserve=%s; concurrency=%d bandwidth=%s", safe(s.Startup), hb(s.DataBudget), hb(s.MetadataBudget), hb(s.ReserveBytes), s.Concurrency, bandwidth(s.BandwidthBytesPerSecond)))
		if f.advanced {
			lines = append(lines, "Peer listen: "+safe(s.PeerListen)+"; advertise: "+safe(s.AdvertisedPeer), "Enrollment listen: "+safe(s.EnrollmentListen)+"; advertise: "+safe(s.AdvertisedEnrollment))
		}
		footer = "Enter confirm exact review  Esc edit  arrows scroll  q quit"
	case "progress":
		title = "Orbit | Setup progress"
		r := f.result
		if r.Operation == nil {
			lines = append(lines, "Loading durable operation…")
		} else {
			lines = append(lines, "State: "+safe(r.Operation.State)+" | "+phaseLabel(r.Operation.Phase))
			if r.Operation.Phase == "awaiting_approval" && r.Join != nil {
				// Say what is awaited and where (E04): the inviting device approves.
				who := "this device"
				if f.plan.DeviceName != "" {
					who += " (" + safe(f.plan.DeviceName) + ")"
				}
				lines = append(lines, "Waiting for the inviting device to approve "+who+".",
					"On the inviting device: open Attention (or press w), choose the request and compare this code: "+joinVerification(r))
			}

			if r.Readiness != nil {
				rd := r.Readiness
				lines = append(lines, fmt.Sprintf("Approved=%t membership current=%t root available=%t scan complete=%t", rd.Approved, rd.MembershipCurrent, rd.RootAvailable, rd.ScanComplete), fmt.Sprintf("Uncaptured=%d download pending=%d publication pending=%d conflicts=%d storage blocked=%t", rd.Uncaptured, rd.MissingContent, rd.PendingPublication, rd.Conflicts, rd.StorageBlocked))
				if rd.Ready() && r.Operation.State == "completed" {
					lines = append(lines, "Locally ready (observed). Other offline devices may remain pending.")
				} else {
					lines = append(lines, "Local readiness incomplete. Work remains pending.")
				}
			}
			lines = append(lines, "Operation: "+safe(r.Operation.ID))
			if r.Join != nil {
				lines = append(lines, "Root: "+safe(r.Join.Root))
				if r.Join.Request != "" {
					lines = append(lines, "Request: "+safe(r.Join.Request))
				}
				if r.Join.Inviter != "" {
					lines = append(lines, "Inviter: "+safe(r.Join.Inviter), "Key pin: "+safe(r.Join.KeyPin), "Compare verification code on inviter: "+joinVerification(r))
				}
			}
			if r.Error != nil {
				lines = append(lines, workflowError(r, nil))
			}
		}
		lines = append(lines, networkLines(r.Network)...)
		footer = "r refresh  a add device  N connection  J new join  Esc overview  q quit"
	case "pick_folder", "pick_device", "setups":
		title = "Orbit | Select " + strings.TrimPrefix(f.screen, "pick_")
		if f.device != "" {
			lines = append(lines, "Selected device: "+safe(f.device), "Select a folder to inspect its sharing/contact state.")
		}
		for i, it := range f.items {
			prefix := "  "
			if i == f.selected {
				prefix = "> "
				focusLine = len(lines)
			}
			lines = append(lines, prefix+safe(it.Name)+"  "+safe(it.Root))
		}
		if len(f.items) == 0 {
			lines = append(lines, "No items. Create a folder [Esc, c] or check existing device approval.")
		}
		footer = "j/k select  Enter choose  ] next  [ first  Esc back  q quit"
	case "requests":
		title = "Orbit | Enrollment requests"
		for i, p := range f.result.Requests {
			prefix := "  "
			if i == f.selected {
				prefix = "> "
				focusLine = len(lines)
			}
			lines = append(lines, prefix+safe(p.Label)+" | "+safe(p.State)+" | code "+safe(p.VerificationCode))
		}
		if len(f.result.Requests) == 0 {
			lines = append(lines, "No enrollment requests on this page.")
		}
		footer = "j/k select  Enter exact review  r refresh  ] next  [ first  Esc back"
	case "approval":
		title = "Orbit | Exact request approval"
		p := f.request
		lines = append(lines, "Device: "+safe(p.Label), "Folder: "+safe(p.Folder), "Request: "+safe(p.ID), "Requester: "+safe(p.Requester), "Key pin: "+safe(p.KeyPin), "Verification code: "+safe(p.VerificationCode), "State: "+safe(p.State), "Compare with the joining device. Approval grants this folder only.", "Offline devices may still need membership updates.")
		footer = "a approve exact request  x decline  Esc back  q quit"
	case "approval_done":
		title = "Orbit | Request decision"
		lines = append(lines, "Decision durably recorded; receiver still observes its own readiness.")
	case "invite_review":
		title = "Orbit | Add device / share folder"
		lines = append(lines, "Selected folder: "+safe(f.folder))
		if f.device != "" {
			lines = append(lines, "Known device: "+safe(f.device), "Reuse its existing identity; separate local-root consent and approval required.")
		}
		if info := f.result.FolderManagement; info != nil {
			lines = append(lines, fmt.Sprintf("Reviewed membership revision: %d", info.Revision))
		}
		lines = append(lines, "Invitation expires in one hour; it grants no file access before approval.")
		footer = "Enter create scoped invitation  Esc back  q quit"
	case "invitation_out":
		title = "Orbit | Private invitation"
		lines = append(lines, "Invitation created for selected folder only.", "Paste on receiver, review its root, then approve the exact request here.", "Expires: "+safe(f.invitation.ExpiresAt), "s: save to private transfer file  v: reveal/hide invitation")
		if f.reveal {
			code, _ := tc.InvitationCode(f.invitation)
			lines = append(lines, code)
		} else {
			lines = append(lines, "Capability hidden. Reveal only for deliberate transfer.")
		}
		footer = "s save private file  v reveal  x revoke  arrows scroll  Esc back  q quit"
	case "revoke_invitation_review":
		title = "Orbit | Revoke invitation"
		lines = append(lines, "Revoke this exact invitation for folder: "+safe(f.invitation.Folder), "Pending requests using it cannot be approved. Existing approved membership is preserved.")
		footer = "Enter revoke exact invitation  Esc back  q quit"
	case "invitation_revoked":
		title = "Orbit | Invitation revoked"
	case "network":
		title = "Orbit | Connection details"
		if f.err != "" {
			lines = append(lines, "Last successful observations retained; current connection state unavailable.")
		}
		lines = append(lines, networkLines(f.result.Network)...)
		lines = append(lines, "Observed connections are separate from saved/stored/applied/conflict state.", "Advanced policy/profile/timing review: orbit network preview; orbit network apply.")
		footer = "r refresh cached observations  d explicit doctor (20s)  arrows scroll  Esc cancel/back  q quit"
	case "save_invitation", "relocate_form":
		title = "Orbit | " + f.screen
		focusLine = len(lines)
		lines = append(lines, m.theme().field(f.fields[0].label, f.fields[0].input.View(), "", true))
		footer = "Enter continue  Esc back  Ctrl-C close"
	case "folder", "retire":
		title = "Orbit | Inspect folder and devices"
		info := f.result.FolderManagement
		if info == nil {
			lines = append(lines, "Loading folder state…")
		} else {
			lines = append(lines, "Root: "+safe(info.Root), fmt.Sprintf("Local pause=%t; membership revision=%d", info.Paused, info.Revision))
			for _, member := range info.Members {
				lines = append(lines, "Shares with: "+safe(member.Name))
			}
			found := false
			for _, obs := range f.result.Observations {
				if f.device == "" || f.device == obs.Device {
					found = true
					lines = append(lines, "Last contact: "+safe(obs.LastContact)+" | "+safe(obs.Availability), fmt.Sprintf("Reported stored=%t applied=%t direct=%t; observed=%s", obs.Stored, obs.Applied, obs.Direct, safe(obs.ObservedAt)))
				}
			}
			if !found {
				lines = append(lines, "Last contact/copy observation: unknown.")
			}
			for _, a := range f.result.Attention {
				lines = append(lines, safe(a.Code)+": "+safe(a.Action))
			}
		}
		footer = "v copy status  h history  D deleted  C conflicts  b storage  p pause  l relocate  a add device  s share  x unregister  t retire  N connection  r refresh  Esc back  q quit"
	case "folder_action":
		title = "Orbit | Confirm local " + f.task
		lines = append(lines, "This action changes local synchronization for the selected folder.", "It does not erase remote files or change remote device membership.")
		footer = "Enter confirm  Esc back  q quit"
	case "retry_review":
		title = "Orbit | Retry work"
		var item tc.Attention
		for _, a := range m.result.Attention {
			if a.OperationID == f.operation && a.Code == "EXHAUSTED_WORK" {
				item = a
			}
		}
		lines = append(lines, "Task "+safe(f.operation)+" ran out of attempts.")
		if item.Action != "" {
			lines = append(lines, safe(item.Action))
		}
		lines = append(lines, "Retrying queues it again in the running daemon; nothing else changes.")
		footer = "Enter retry  Esc back  q quit"
	case "relocate_review":
		title = "Orbit | Confirm relocation"
		if info := f.result.FolderManagement; info != nil {
			lines = append(lines, "Current: "+safe(info.Root), "Destination: "+safe(f.fields[0].input.Value()), "Uses the existing resumable relocation journal. Keep original/staging folders until recovery finishes.")
		}
		footer = "Enter relocate  Esc edit  q quit"
	case "unregister_preview":
		title = "Orbit | Unregister preview"
		lines = append(lines, "Scope: remove this device's local root registration only.", "Working files are preserved; remote copies and membership are unchanged.", "This does not erase a remote device or revoke its keys.", "Use the existing conservative procedure: orbit folders remove --folder "+safe(f.folder))
		footer = "arrows scroll  Esc back  q quit"
	case "retirement_preview":
		title = "Orbit | Retirement preview"
		for _, it := range f.result.Items {
			lines = append(lines, safe(it.Name), safe(it.Root))
		}
		lines = append(lines, "Preview only. Follow the conservative reviewed retirement procedure.", "orbit engine peers retire --help; docs/runbooks/membership-fork.md")
		footer = "arrows scroll  Esc back  q quit"
	}
	if f.busy {
		lines = append(lines, "Submitting exact operation; closing the client does not cancel admitted work.")
	}
	return m.frame(title, lines, footer, focusLine, f.scroll)
}

// frame renders a workflow screen: header, one scrolling panel and key hints.
// The focused line stays visible and the title/footer never scroll away.
func (m *model) frame(title string, lines []string, footer string, focusLine, scroll int) tea.View {
	t := m.theme()
	title = strings.TrimPrefix(title, "Orbit | ")
	right := ""
	if s := m.result.Service; s != nil && !t.plain {
		right = t.pill(daemonLabel(s), startupLabel(s))
	}
	heading := []string{t.header(m.width, t.bold(title), right)}
	if t.plain {
		heading = strings.Split(ansi.Wrap("Orbit | "+title, max(1, m.width), ""), "\n")
	}
	tail := t.footer(footer, m.width)
	// A revealed invitation is copied from the terminal, so no border may
	// interleave with its wrapped lines.
	copyable := m.flow != nil && m.flow.screen == "invitation_out" && m.flow.reveal
	boxed := !copyable && m.width >= 60 && m.height >= len(heading)+len(tail)+4
	inner := m.width
	if boxed {
		inner = m.width - 4
	}
	body, focusWrapped := t.body(lines, inner, focusLine)
	available := max(1, m.height-len(heading)-len(tail))
	if boxed {
		available = max(1, available-2)
	}
	start := min(scroll, max(0, len(body)-available))
	if focusWrapped >= 0 {
		start = max(start, focusWrapped-available+2)
		start = min(start, focusWrapped)
	}
	start = max(0, min(start, max(0, len(body)-available)))
	visible := body[start:min(len(body), start+available)]
	output := heading
	if boxed {
		status := ""
		if len(body) > available {
			status = fmt.Sprintf("%d-%d of %d", start+1, start+len(visible), len(body))
		}
		output = append(output, t.panel("", status, visible, m.width, available+2, true)...)
	} else {
		output = append(output, visible...)
	}
	output = append(output, tail...)
	if len(output) > m.height {
		output = output[:m.height]
	}
	v := tea.NewView(strings.Join(output, "\n"))
	v.AltScreen = true
	return v
}

func previewLines(p *tc.RootPreview) []string {
	if p == nil {
		return nil
	}
	lines := []string{fmt.Sprintf("Measured files=%d directories=%d size=%s; complete=%t", p.Files, p.Directories, hb(p.Bytes), p.Complete), fmt.Sprintf("Unsupported=%d unreadable=%d capacity known=%t available=%s", p.Unsupported, p.Unreadable, p.CapacityKnown, hb(p.AvailableBytes))}
	for _, issue := range p.Issues {
		lines = append(lines, safe(issue.Code)+": "+safe(issue.Path))
	}
	return lines
}
func phaseLabel(phase string) string {
	switch phase {
	case "network_restart":
		return "Restarting Orbit to use the reviewed connection"
	case "awaiting_approval":
		return "Waiting for approval"
	case "membership_received":
		return "Approved; updating devices"
	case "bootstrap_capture":
		return "Scanning local files"
	case "content_pending":
		return "Downloading / publishing files"
	case "ready":
		return "Local readiness observed"
	default:
		return safe(phase)
	}
}

func joinVerification(r tc.Result) string {
	if r.Join != nil {
		for _, p := range r.Requests {
			if p.ID == r.Join.Request {
				return safe(p.VerificationCode)
			}
		}
	}
	return "unavailable (waiting for inviter observation)"
}

func policyLines(p tc.NetworkPolicy) []string {
	lines := []string{"Connection: " + connectionMode(p.Mode)}
	if p.Mode != "manual" {
		lines = append(lines, fmt.Sprintf("LAN advertising: %t; broadcasts signed device identity and listener addresses.", p.LANAdvertising))
	}
	if p.Mode == "automatic" || p.Mode == "self_hosted" {
		lines = append(lines, "Orbit services help your devices connect. They see device addresses and connection metadata; file contents stay encrypted in transit.")
		if p.AwaitingProfile {
			lines = append(lines, "Connection service configuration needs update; local capture remains available.")
		}
	}
	if p.Mode == "local_only" {
		lines = append(lines, "No public announcements or relay use. Only known pinned devices on permitted local interfaces can connect.")
	}
	return lines
}
func connectionMode(mode string) string {
	switch mode {
	case "automatic":
		return "Automatic"
	case "local_only":
		return "Local network only"
	case "manual":
		return "Manual/private network"
	case "self_hosted":
		return "Self-hosted automatic"
	}
	return safe(mode)
}
func networkLines(n *tc.NetworkStatus) []string {
	if n == nil {
		return []string{"Connection observations unavailable; local readiness is reported separately."}
	}
	lines := []string{"Connection: " + connectionMode(n.Policy.Mode), "Service configuration: " + safe(n.Code) + fmt.Sprintf("; ready=%t", n.Ready)}
	if n.RestartRequired {
		lines = append(lines, "Active connection: "+connectionMode(n.ActivePolicy.Mode)+"; daemon restart required.")
	}
	if n.Operator != "" {
		lines = append(lines, "Operator: "+safe(n.Operator), "Metadata: "+safe(n.Privacy))
	}
	if n.ProfileState != "" {
		profile := "Profile: " + safe(n.ProfileState)
		if n.ProfileExpires != "" {
			profile += "; expires=" + safe(n.ProfileExpires)
		}
		lines = append(lines, profile)
	}
	lines = append(lines, "Next action: "+safe(n.Action))
	for _, probe := range n.Probes {
		lines = append(lines, "Probe "+safe(probe.Kind)+": "+safe(probe.Code)+"; observed="+safe(probe.ObservedAt))
	}
	for _, o := range n.Observations {
		label := "Finding a connection"
		switch o.Route {
		case "relay":
			label = "Connected via relay"
		case "direct", "quic":
			label = "Direct connection"
		case "unavailable":
			label = "Connection blocked"
		case "not_tested":
			label = "Connection not tested"
		}
		if o.Freshness == "stale" {
			label = "Last observed " + label + " (stale)"
		}
		lines = append(lines, safe(o.Device)+": "+label+"; observed="+safe(o.ObservedAt)+"; "+safe(o.Code))
		lines = append(lines, "Freshness: "+safe(o.Freshness)+fmt.Sprintf("; candidates LAN=%d public=%d expired=%d; UDP=%s", o.LANCandidates, o.PublicCandidates, o.ExpiredCandidates, safe(o.UDPCode)), "Next action: "+safe(o.Action))
	}
	return lines
}

// hostStartupLines explain the proposed startup for this host (EG3), including
// the exact linger command the owner may run; Orbit never runs it.
func hostStartupLines(h *tc.HostStartup) []string {
	if h == nil {
		return []string{"Startup: login needs user systemd; unattended also needs lingering."}
	}
	var lines []string
	switch h.Class {
	case "desktop":
		lines = append(lines, "Startup: this looks like a desktop, so Orbit starts when you log in.")
	case "headless":
		if h.Lingering {
			lines = append(lines, "Startup: this looks like a headless host with lingering on, so Orbit runs unattended.")
		} else {
			lines = append(lines, "Startup: this looks like a headless host.")
		}
	}
	if h.Note != "" {
		lines = append(lines, safe(h.Note))
	}
	return lines
}

// receivedLine reports how much of a hidden value arrived without showing it.
func receivedLine(value string) string {
	n := utf8.RuneCountInString(value)
	if n == 0 {
		return "Nothing received yet. Paste the whole code; a new paste replaces it."
	}
	return humanCount(n) + " characters received. A new paste replaces it; Ctrl-U clears."
}

// humanCount formats n with thousands separators (1,666).
func humanCount(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
