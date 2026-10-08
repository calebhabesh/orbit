package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
)

const pageSize = 20

var sections = []string{"Overview", "Folders", "Attention", "Devices"}

type queryReply struct {
	request, generation uint64
	result              tc.Result
	devices             []tc.NamedItem
	err                 error
	task                string
	setups              []tc.NamedItem
}
type refreshMsg struct{}
type toolReply struct {
	session    bool
	generation uint64
	err        error
}
type row struct{ key, name, subtitle, folder, path, action string }

type model struct {
	ctx                                     context.Context
	client                                  Queries
	opts                                    Options
	width, height, section, selected, focus int
	search                                  textinput.Model
	help, detail, quitting, toolRunning     bool
	result                                  tc.Result
	devices                                 []tc.NamedItem
	errText, notice, selectedKey, cursor    string
	detailRow                               row
	generation, request, pending            uint64
	cancel                                  context.CancelFunc
	dirty                                   bool
	flow                                    *workflow
	firstLoad                               bool
	unfinished                              []tc.NamedItem
}

func newModel(ctx context.Context, client Queries, opts Options) *model {
	if opts.Refresh < time.Second {
		opts.Refresh = 3 * time.Second
	}
	input := textinput.New()
	input.Prompt = "Search page: "
	input.Placeholder = "folder, path or device"
	input.CharLimit = 256
	// Bracketed paste is handled below. Do not spawn a clipboard process that
	// could return an unbounded buffer outside application paste admission.
	input.KeyMap.Paste.SetEnabled(false)
	input.SetWidth(40)
	return &model{ctx: ctx, client: client, opts: opts, width: 80, height: 24, search: input, generation: 1}
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.startQuery(), m.tick()) }
func (m *model) tick() tea.Cmd {
	return tea.Tick(m.opts.Refresh, func(time.Time) tea.Msg { return refreshMsg{} })
}

// One query lane: cancellation coalesces navigation until the old command has
// returned. Even a slow adapter cannot accumulate parallel polls or pages.
func (m *model) invalidate() tea.Cmd {
	m.generation++
	m.dirty = true
	if m.cancel != nil {
		m.cancel()
	}
	if m.pending == 0 && !m.toolRunning {
		return m.startQuery()
	}
	return nil
}
func (m *model) startQuery() tea.Cmd {
	if m.flow != nil {
		return m.startFlowQuery()
	}
	if m.pending != 0 || m.quitting || m.toolRunning {
		return nil
	}
	m.dirty = false
	m.request++
	m.pending = m.request
	request, generation := m.request, m.generation
	section, detail, target, cursor := m.section, m.detail, m.detailRow, m.cursor
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	m.cancel = cancel
	client := m.client
	return func() tea.Msg {
		defer cancel()
		q := tc.Query{Version: tc.Version, Kind: "folders", Limit: pageSize, Cursor: cursor}
		if detail {
			q.Kind, q.Folder, q.Cursor = "attention", target.folder, ""
			if target.folder == "" {
				q.Kind, q.ID = "devices", target.key
			}
		} else {
			switch section {
			case 2:
				q.Kind = "attention"
			case 3:
				q.Kind = "devices"
			}
		}
		r, err := client.Query(ctx, q)
		var devices, setups []tc.NamedItem
		if err == nil && r.Error == nil && section == 0 && !detail {
			var a, s tc.Result
			a, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "attention", Limit: pageSize})
			if err == nil && a.Error != nil {
				r = a
			} else {
				r.Attention = a.Attention
			}
			if err == nil && r.Error == nil {
				s, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "service", Limit: pageSize})
				if s.Error != nil {
					r = s
				} else {
					r.Service = s.Service
				}
			}
			if err == nil && r.Error == nil {
				d, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "devices", Limit: pageSize})
				err = e
				if d.Error != nil {
					r = d
				} else {
					devices = d.Items[:min(len(d.Items), pageSize)]
				}
			}
		}
		if err == nil && r.Error == nil && section == 0 && !detail && r.Network == nil {
			// Passive cached connection status (never a probe); optional, so an
			// adapter without it keeps the rest of the overview.
			if n, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status"}); e == nil && n.Error == nil {
				r.Network = n.Network
			}
		}
		if err == nil && section == 0 && !detail {
			if _, ok := client.(Workflows); ok {
				var a tc.Result
				a, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "setups", Limit: pageSize})
				setups = a.Items[:min(len(a.Items), pageSize)]
			}
		}
		// Defensive bounds also apply to injected/future adapters.
		r.Items = r.Items[:min(len(r.Items), pageSize)]
		r.Attention = r.Attention[:min(len(r.Attention), pageSize)]
		r.Observations = r.Observations[:min(len(r.Observations), pageSize)]
		return queryReply{request: request, generation: generation, result: r, devices: devices, err: err, setups: setups}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case revealDone:
		if m.flow != nil && msg.err != nil {
			m.flow.err = "Could not show the invitation in this terminal; press s to save it to a file."
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.search.SetWidth(max(1, m.width-16))
	case queryReply:
		if msg.request != m.pending {
			return m, nil
		}
		m.pending = 0
		m.cancel = nil
		if !m.quitting && msg.generation == m.generation && m.flow != nil {
			cmd := m.acceptFlow(msg.task, msg.result, msg.err)
			if cmd != nil {
				return m, cmd
			}
			if m.dirty {
				return m, m.startQuery()
			}
			return m, nil
		}
		if !m.quitting && msg.generation == m.generation {
			m.errText = ""
			if msg.err != nil {
				// Transport errors can contain credentials or private endpoints.
				m.errText = "Control unavailable. Retrying; orbit doctor can inspect the daemon."
			} else if msg.result.Error != nil {
				m.errText = safe(msg.result.Error.Code + ": " + msg.result.Error.Action)
			} else {
				m.result = msg.result
				m.devices = msg.devices
				m.restoreSelection()
				m.unfinished = msg.setups
				if !m.firstLoad {
					m.firstLoad = true
					if _, ok := m.workflows(); ok {
						if len(m.unfinished) > 0 {
							return m, m.openFlow(&workflow{screen: "progress", operation: m.unfinished[0].ID})
						}
						if len(m.result.Items) == 0 && m.section == 0 {
							return m, m.openFlow(&workflow{screen: "welcome"})
						}
					}
				}
			}
		}
		if m.dirty {
			return m, m.startQuery()
		}
	case refreshMsg:
		return m, tea.Batch(m.startQuery(), m.tick())
	case toolReply:
		m.toolRunning = false
		if msg.session && msg.generation == m.generation && m.flow != nil && m.flow.daily != nil {
			if msg.err != nil {
				m.flow.err = "Tool failed; private result retained. Retry e, inspect with d, or r for recovery review."
				m.flow.notice = ""
			} else {
				m.flow.err = ""
				m.flow.notice = "Tool returned; result retained. Press u to stage, then review before commit."
			}
			return m, nil
		}
		if msg.generation == m.generation {
			m.notice = "Tool returned; terminal resumed."
			if msg.err != nil {
				m.notice = "Tool failed; terminal resumed. Review the result before using it."
			}
		}
		return m, m.startQuery()
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.PasteMsg:
		if m.flow != nil {
			f := m.flow
			if !f.busy && (f.screen == "form" || f.screen == "invitation" || f.screen == "save_invitation" || f.screen == "relocate_form" || (f.daily != nil && len(f.fields) > 0 && f.fields[f.focus].input.Focused())) {
				text := msg.Content
				limit := 4096
				if f.screen == "invitation" {
					limit = 16384
				}
				if len(text) > limit {
					f.err = "Paste exceeds the field limit."
					return m, nil
				}
				fld := &f.fields[f.focus]
				if fld.choices != nil {
					return m, nil
				}
				if f.screen == "invitation" {
					text = strings.NewReplacer("\r", "", "\n", "").Replace(text)
				}
				if fld.input.EchoMode == textinput.EchoPassword {
					// A hidden field cannot be inspected, so each paste replaces
					// it rather than appending to a failed attempt (F06).
					fld.input.SetValue(safe(strings.TrimSpace(text)))
					f.replaceOnType = false
				} else {
					fld.input.SetValue(fld.input.Value() + safe(strings.TrimSpace(text)))
				}
			}
			return m, nil
		}
		if m.search.Focused() {
			// Bound before Bubbles converts an arbitrary paste to runes.
			text := msg.Content
			if len(text) > 4096 {
				text = text[:4096]
			}
			m.search.SetValue(m.search.Value() + safe(text))
			m.restoreSelection()
		}
	default:
		if m.flow != nil {
			f := m.flow
			if !f.busy && len(f.fields) > 0 && f.fields[f.focus].input.Focused() {
				var cmd tea.Cmd
				f.fields[f.focus].input, cmd = f.fields[f.focus].input.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		if m.search.Focused() {
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) launchTool() tea.Cmd {
	if m.opts.Tool == nil {
		m.notice = "No external tool configured."
		return nil
	}
	t := m.opts.Tool
	cmd, err := controlclient.LimitedToolCommand(m.ctx, t.Command, t.MaxBytes, t.Paths...)
	if err != nil {
		m.notice = "Tool unavailable; check its command and prlimit."
		return nil
	}
	m.toolRunning = true
	m.generation++
	if m.cancel != nil {
		m.cancel()
		m.dirty = true
	}
	generation := m.generation
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return toolReply{generation: generation, err: err} })
}

func (m *model) quit() tea.Cmd {
	m.quitting = true
	if m.cancel != nil {
		m.cancel()
	}
	return tea.Quit
}

func (m *model) restoreSelection() {
	rows := m.rows()
	for i, r := range rows {
		if r.key == m.selectedKey {
			m.selected = i
			return
		}
	}
	m.selected = min(m.selected, max(0, len(rows)-1))
	if len(rows) > 0 {
		m.selectedKey = rows[m.selected].key
	} else {
		m.selectedKey = ""
	}
}

func (m *model) selectRow(delta int) {
	rows := m.rows()
	m.selected = min(max(0, m.selected+delta), max(0, len(rows)-1))
	if len(rows) > 0 {
		m.selectedKey = rows[m.selected].key
	}
}

func (m *model) changeSection(section int) tea.Cmd {
	m.section, m.selected, m.focus = section, 0, 0
	m.detail, m.help = false, false
	m.selectedKey, m.cursor = "", ""
	m.result = tc.Result{}
	return m.invalidate()
}

func (m *model) inspect() tea.Cmd {
	rows := m.rows()
	if len(rows) == 0 {
		return nil
	}
	if _, ok := m.workflows(); ok {
		r := rows[m.selected]
		if strings.HasPrefix(r.key, "a:") {
			for _, a := range m.result.Attention {
				if "a:"+a.ID == r.key {
					if cmd := m.openAttention(a); cmd != nil || m.flow != nil {
						return cmd
					}
					// No screen for this code: show the item's details.
					m.detailRow, m.detail = r, true
					return nil
				}
			}
		}
		if r.folder != "" {
			return m.openFlow(&workflow{screen: "folder", folder: r.folder})
		}
		return m.openFlow(&workflow{screen: "pick_folder", kind: "device_detail", device: r.key})
	}
	m.detailRow, m.detail = rows[m.selected], true
	m.result = tc.Result{}
	return m.invalidate()
}

func running(s *tc.Service) string {
	if s == nil {
		return "unknown"
	}
	if s.Running {
		return "running"
	}
	return "stopped"
}

func (m *model) summary() string {
	return fmt.Sprintf("Daemon: %s   Startup: %s", daemonLabel(m.result.Service), startupLabel(m.result.Service))
}

// daemonLabel names the daemon state and who runs it: the service, the
// terminal launcher or a manual start (F04).
func daemonLabel(s *tc.Service) string {
	label := running(s)
	if s != nil && s.Running && s.Owner != "" {
		label += " (" + safe(s.Owner) + ")"
	}
	return label
}

// startupLabel is the configured startup mode, reported separately from the
// owner, with a failing or restarting unit shown as such.
func startupLabel(s *tc.Service) string {
	if s == nil {
		return "unknown"
	}
	startup := safe(s.Mode)
	if !s.Enabled {
		startup += " (not enabled)"
	}
	switch {
	case s.UnitState == "failed":
		startup += "; service failed"
	case s.UnitState == "activating" && s.Owner != "service":
		startup += "; service restarting"
	}
	return startup
}
