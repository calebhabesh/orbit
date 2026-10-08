package terminal

import (
	"fmt"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"strings"
)

func available(v tc.VersionSummary) bool {
	return v.Availability == "ready" || v.Availability == "available"
}
func readinessLines(r *tc.Readiness) []string {
	if r == nil {
		return []string{"Local capture/readiness: unknown"}
	}
	return []string{fmt.Sprintf("Local: uncaptured=%d missing=%d publication pending=%d conflicts=%d", r.Uncaptured, r.MissingContent, r.PendingPublication, r.Conflicts), fmt.Sprintf("Root available=%t scan complete=%t membership current=%t storage blocked=%t", r.RootAvailable, r.ScanComplete, r.MembershipCurrent, r.StorageBlocked), "Local observations do not establish all devices are up to date."}
}
func (m *model) dailyLines() (string, []string, string, int) {
	f := m.flow
	d := f.daily
	r := f.result
	title := "Orbit | " + strings.TrimPrefix(f.screen, "day_")
	lines := []string{}
	focus := -1
	footer := "j/k scroll r refresh ? help Esc back q quit"
	if f.advanced {
		lines = append(lines, "Keyboard help: actions below apply to this screen.", "Text fields receive ordinary j/k/q/? input.", "Esc returns; q/Ctrl-C closes only this interface.", "Reviewed actions require preview then Enter confirm.")
	}
	switch f.screen {
	case "day_status":
		title = "Orbit | Qualified copy status"
		for _, it := range r.Items {
			lines = append(lines, "Folder: "+safe(it.Name), "Root: "+safe(it.Root))
		}
		lines = append(lines, readinessLines(r.Readiness)...)
		lines = append(lines, "Saved = local durable capture; stored/applied = reported device copy.", "Offline and indirect receipts establish historical knowledge only.")
		for _, o := range r.Observations {
			name := o.DeviceName
			if name == "" {
				name = "Device " + o.Device[:min(8, len(o.Device))]
			}
			lines = append(lines, safe(name)+fmt.Sprintf(": saved=%t stored=%t applied=%t", o.Saved, o.Stored, o.Applied), fmt.Sprintf("online=%t direct=%t local content=%s", o.Online, o.Direct, safe(o.Availability)), "Last contact: "+safe(o.LastContact), "Observed: "+safe(o.ObservedAt))
		}
		if len(r.Observations) == 0 {
			lines = append(lines, "Device-copy observations: unknown")
		}
		for _, a := range r.Attention {
			lines = append(lines, safe(a.Code+": "+a.Path), safe(a.Action))
		}
	case "day_storage", "day_maintenance":
		title = "Orbit | Storage and retention"
		if s := r.Storage; s != nil {
			lines = append(lines, fmt.Sprintf("Objects=%d / data budget=%d bytes", s.Objects, s.DataBudget), fmt.Sprintf("Metadata=%d / budget=%d bytes", s.Metadata, s.MetadataBudget), fmt.Sprintf("Staging=%d recovery=%d quarantine=%d bytes", s.Staging, s.Recovery, s.Quarantine), fmt.Sprintf("Free space reserve=%d bytes", s.Reserve))
		}
		for _, it := range r.Items {
			lines = append(lines, safe(it.Name), safe(it.Root))
		}
		lines = append(lines, "History availability depends on retained verified bytes, not a deletion timer.", "Preview only: inspection never runs cleanup.", "Maintenance: orbit storage --help", "Root/recovery: docs/runbooks/terminal-recovery.md", "Retirement: docs/runbooks/membership-fork.md")
		footer = "m maintenance preview j/k scroll r refresh ? Esc q"
	case "day_paths":
		title = "Orbit | Find path history"
		lines = append(lines, "Search known paths; ordinary editing stays in your applications.")
		prefix := "  "
		if f.fields[0].input.Focused() {
			prefix = "> "
			focus = len(lines)
		}
		lines = append(lines, prefix+"Find known path: "+f.fields[0].input.View())
		fallthrough
	case "day_history", "day_deleted", "day_conflicts":
		if f.screen == "day_history" {
			title = "Orbit | History: " + safe(d.path)
			lines = append(lines, "Time is informational; restore uses exact source and current reviewed parents.")
		}
		if f.screen == "day_deleted" {
			title = "Orbit | Deleted files"
			lines = append(lines, "Known deleted paths; bytes may be pending, expired, unavailable or corrupt.")
		}
		if f.screen == "day_conflicts" {
			title = "Orbit | Conflicts"
		}
		count := len(r.Items)
		if f.screen == "day_history" {
			count = len(r.Versions)
		}
		if f.screen == "day_conflicts" {
			count = len(r.Attention)
		}
		for i := 0; i < count; i++ {
			prefix := "  "
			if i == f.selected {
				prefix = "> "
				if focus < 0 {
					focus = len(lines)
				}
			}
			switch f.screen {
			case "day_history":
				v := r.Versions[i]
				lines = append(lines, prefix+versionLabel(v), "  Time: "+safe(v.DisplayTime))
			case "day_conflicts":
				a := r.Attention[i]
				lines = append(lines, prefix+safe(a.Path)+" | "+safe(a.Code))
			default:
				lines = append(lines, prefix+safe(r.Items[i].Name))
			}
		}
		if count == 0 {
			lines = append(lines, "No candidates on this page.")
		}
		if r.Cursor != "" {
			lines = append(lines, "More available: ] next page")
		}
		footer = "j/k select Enter review / search ] next [ first r ? Esc q"
		if f.screen == "day_paths" && f.fields[0].input.Focused() {
			footer = "Type path Enter search Esc navigation Tab focus Ctrl-C close"
		}
	case "day_load_review", "day_load_session":
		lines = append(lines, "Loading exact review; no replacement committed.")
	case "day_review":
		title = "Orbit | Review versions: " + safe(d.path)
		if d.review != nil {
			lines = append(lines, "Folder: "+safe(d.review.Context.FolderName), "Root: "+safe(d.review.Context.Root), fmt.Sprintf("Exact heads=%d working captured=%t", len(d.review.Heads), d.review.WorkingCaptured))
		}
		for i, v := range d.versions {
			prefix := "  "
			if i == f.selected {
				prefix = "> "
				focus = len(lines)
			}
			lines = append(lines, prefix+versionLabel(v), "  Time: "+safe(v.DisplayTime))
		}
		if d.review != nil && d.review.Source != nil {
			lines = append(lines, "Historical source: "+versionLabel(*d.review.Source))
		}
		lines = append(lines, "Unavailable bytes disable content actions; peer fetch unsupported.", "New arrivals require fresh review. Time never selects a winner.")
		footer = "Enter preview e editor d diff K keep copies c copy h history r fresh ? Esc q"
	case "day_destination", "day_copies":
		title = "Orbit | Separate copy destinations"
		lines = append(lines, "Root-relative destinations; existing files cause collision refusal.")
		for i, field := range f.fields {
			prefix := "  "
			value := safe(field.input.Value())
			if i == f.focus {
				prefix = "> "
				value = field.input.View()
				focus = len(lines)
			}
			lines = append(lines, prefix+field.label+": "+value)
		}
		footer = "Tab field Enter preview Esc review Ctrl-C close"
	case "day_confirm":
		title = "Orbit | Confirm reviewed " + safe(d.action)
		if d.review != nil {
			lines = append(lines, "Folder: "+safe(d.review.Context.FolderName), "Root: "+safe(d.review.Context.Root), "Original path: "+safe(d.review.Context.Path), fmt.Sprintf("Exact heads=%d working captured=%t", len(d.review.Heads), d.review.WorkingCaptured), "Replace: "+safe(strings.Join(d.review.ReplacementPaths, ", ")))
		}
		if d.destination != "" {
			lines = append(lines, "Separate destination: "+safe(d.destination), "Original path stays unchanged.")
		}
		for _, c := range d.copies {
			lines = append(lines, "Keep exact copy -> "+safe(c.Destination))
		}
		if d.source.Counter != 0 {
			lines = append(lines, "Source: "+safe(versionKey(d.source)))
		}
		if d.upload != nil {
			lines = append(lines, fmt.Sprintf("Staged result=%d bytes", d.upload.Bytes), "SHA256: "+safe(d.upload.Digest))
		}
		lines = append(lines, "Current working bytes are protected by capture/publication checks.", "Each copy is individually durable; publication can remain pending.", "Enter commits only this review. Changed state refuses the action.")
		footer = "Enter confirm exact result r fresh review j/k scroll ? Esc q"
	case "day_editor", "day_recovery":
		title = "Orbit | Retained editor session"
		if d.session != nil {
			lines = append(lines, "State: "+safe(d.session.State), "Result: "+safe(d.session.ResultPath), "Expires: "+safe(d.session.ExpiresAt))
		}
		lines = append(lines, "Private result is retained until reviewed cleanup.", "Closing a client does not commit edited bytes.")
		footer = "e editor d diff u stage result r recovery review ? Esc q"
		if f.screen == "day_recovery" {
			lines = append(lines, "Fresh current-head review obtained. e opens new reviewed sources; former result remains retained.", "n extends only an active original review; expiry/stale heads require a new session.", "Discard removes only this session's private artifacts; captured versions remain.")
			footer = "e new session n renew original x discard reviewed r fresh ? Esc q"
		}
	case "day_operation":
		title = "Orbit | Durable content operation"
		if r.Operation != nil {
			lines = append(lines, "State: "+safe(r.Operation.State)+" | phase: "+safe(r.Operation.Phase))
		}
		effects := r.Effects
		if r.Operation != nil && len(effects) == 0 {
			effects = r.Operation.CommittedEffects
		}
		for _, e := range effects {
			lines = append(lines, safe(e.Path)+": "+safe(e.State))
		}
		lines = append(lines, "Durable effects may precede publication; pending/partial work stays visible.")
		if r.Operation != nil && r.Operation.State == "pending" && d.mutation.Kind == "content" && d.mutation.OperationID == f.operation {
			footer = "r resume exact operation | j/k scroll ? help Esc back q quit"
		}
	}
	return title, lines, footer, focus
}
func versionLabel(v tc.VersionSummary) string {
	return fmt.Sprintf("%s #%d %s %d bytes %s", safe(v.DeviceName), v.Version.Counter, safe(v.Kind), v.Bytes, safe(v.Availability))
}
