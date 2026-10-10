package terminal

import (
	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	if m.flow != nil {
		return m.flowKey(msg)
	}
	k := msg.String()
	m.toast = ""
	// An explicit view selection wins over the asynchronous initial landing.
	if k == "1" || k == "2" || k == "3" || k == "4" || k == "5" || k == "tab" || k == "shift+tab" || k == "left" || k == "right" {
		if !m.search.Focused() {
			m.firstLoad = true
		}
	}
	if k == "ctrl+c" {
		return m.quit()
	}
	if m.search.Focused() && m.section == filesSection {
		switch k {
		case "enter":
			m.search.Blur()
			m.focus = 0
			return m.filesSearch()
		case "esc":
			m.search.Blur()
			m.search.SetValue("")
			m.focus = 0
			return nil
		}
	}
	if m.search.Focused() {
		switch k {
		case "esc", "enter", "tab":
			m.search.Blur()
			m.focus = 0
			return nil
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.restoreSelection()
			return cmd
		}
	}
	if m.help {
		if k == "esc" || k == "?" || k == "enter" {
			m.help = false
		}
		if k == "q" {
			return m.quit()
		}
		return nil
	}
	if m.section == filesSection {
		if cmd, handled := m.filesKey(k); handled {
			return cmd
		}
	}
	switch k {
	case "L", "X":
		rows := m.rows()
		kind := "leave"
		if k == "X" {
			kind = "remove"
		}
		if len(rows) > 0 {
			row := rows[m.selected]
			if row.folder != "" {
				return m.openFlow(&workflow{screen: "folder", folder: row.folder, folderName: row.name, kind: kind})
			}
			if k == "X" && m.section == 3 {
				return m.openFlow(&workflow{screen: "pick_folder", kind: kind, device: row.key})
			}
		}
		return m.openFlow(&workflow{screen: "pick_folder", kind: kind})
	case "N":
		return m.openFlow(&workflow{screen: "network"})
	case "b":
		return m.openFlow(&workflow{screen: "pick_folder", kind: "storage"})
	case "c":
		kind := "setup"
		if len(m.result.Items) > 0 {
			kind = "adopt"
		}
		return m.setupForm(kind)
	case "J":
		return m.setupForm("join")
	case "a":
		return m.openFlow(&workflow{screen: "pick_folder", kind: "invite"})
	case "s":
		return m.openFlow(&workflow{screen: "pick_folder", kind: "share"})
	case "w":
		return m.openFlow(&workflow{screen: "requests"})
	case "u":
		return m.openFlow(&workflow{screen: "setups"})
	case "q":
		return m.quit()
	case "?":
		m.help = true
	case "/":
		m.focus = 1
		return m.search.Focus()
	case "tab":
		// Tab moves between views, not into search; / searches (E03).
		return m.changeSection((m.section + 1) % len(sections))
	case "shift+tab":
		return m.changeSection((m.section + len(sections) - 1) % len(sections))
	case "1", "2", "3", "4", "5":
		return m.changeSection(int(k[0] - '1'))
	case "j", "down":
		return m.selectRow(1)
	case "k", "up":
		return m.selectRow(-1)
	case "left":
		return m.changeSection((m.section + len(sections) - 1) % len(sections))
	case "right":
		return m.changeSection((m.section + 1) % len(sections))
	case "R":
		return m.renameSelected()
	case "enter":
		if !m.detail {
			return m.inspect()
		}
	case "esc":
		if m.detail {
			m.detail = false
			m.result = tc.Result{}
			return m.invalidate()
		}
		m.search.SetValue("")
		m.restoreSelection()
	case "r":
		return m.startQuery()
	case "]":
		if !m.detail && m.section != 0 && m.result.Cursor != "" {
			m.cursor = m.result.Cursor
			m.selected = 0
			m.selectedKey = ""
			return m.invalidate()
		}
	case "[":
		if !m.detail {
			m.cursor = ""
			return m.invalidate()
		}
	case "e":
		return m.launchTool()
	}
	return nil
}
