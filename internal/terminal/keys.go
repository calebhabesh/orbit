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
	if k == "ctrl+c" {
		return m.quit()
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
	switch k {
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
	case "tab", "shift+tab":
		m.focus = 1
		return m.search.Focus()
	case "j", "down":
		m.selectRow(1)
	case "k", "up":
		m.selectRow(-1)
	case "left":
		return m.changeSection((m.section + len(sections) - 1) % len(sections))
	case "right":
		return m.changeSection((m.section + 1) % len(sections))
	case "o":
		return m.changeSection(0)
	case "f":
		return m.changeSection(1)
	case "n":
		return m.changeSection(2)
	case "d":
		return m.changeSection(3)
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
