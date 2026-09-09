package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/ansi"
)

func TestHelpNavigationKeepsReaderAndDoesNotExecuteCommands(t *testing.T) {
	for _, agent := range []bool{false, true} {
		m := press(newModel(), "enter")
		m.chatStyle = agent
		m.resize(60, 20)
		m = press(m, "n")
		key, offset, id := m.selectedKey(), m.reader.YOffset(), m.current().id
		m = press(m, "?")
		m, _ = updateKey(m, tea.KeyEnd, 0)
		if m.help.selected != len(m.helpEntries())-1 || m.help.top == 0 {
			t.Fatal("last command not reachable")
		}
		selected, top := m.help.selected, m.help.top
		m = press(m, "enter")
		if !m.help.detail {
			t.Fatal("Enter did not open description")
		}
		for _, k := range []string{"r", "c", "x", "s"} {
			m = press(m, k)
		}
		m, _ = updateKey(m, tea.KeyF6, 0)
		m, _ = updateKey(m, tea.KeyF7, 0)
		m, _ = updateKey(m, 'p', tea.ModCtrl)
		if !m.isHelp() || !m.help.detail || m.chatStyle != agent || m.theme.name != "" || m.draft.Body != "" {
			t.Fatal("help keys executed an action")
		}
		m = press(m, "esc")
		if m.help.detail || !m.isHelp() || m.help.selected != selected || m.help.top != top {
			t.Fatal("back lost list position")
		}
		m = press(m, "esc")
		if m.modal != "" || !m.reading || m.selectedKey() != key || m.reader.YOffset() != offset || m.current().id != id {
			t.Fatal("help changed reading state")
		}
	}
}

func TestHelpFitsWindowAndDetailsScroll(t *testing.T) {
	for _, size := range [][2]int{{44, 16}, {60, 20}, {120, 36}} {
		m := newModel()
		m.opts.Demo = false
		m.resize(size[0], size[1])
		m = press(m, "?")
		for range len(m.helpEntries()) {
			dialog := m.dialog()
			if lipgloss.Width(dialog) > m.width || lipgloss.Height(dialog) > m.height-4 {
				t.Fatal("help overflows terminal")
			}
			plain := ansi.Strip(dialog)
			if strings.Count(plain, "> ") != 1 {
				t.Fatal("missing or duplicate selection arrow")
			}
			m = press(m, "j")
		}
		for i, e := range m.helpEntries() {
			if e.key == "P" {
				m.help.selected = i
			}
		}
		m = press(m, "enter")
		m, _ = updateKey(m, tea.KeyEnd, 0)
		if size[1] == 16 && m.help.offset == 0 {
			t.Fatal("long detail cannot scroll")
		}
		m, _ = updateKey(m, tea.KeyHome, 0)
		if m.help.offset != 0 {
			t.Fatal("Home did not reach start of detail")
		}
		m, _ = updateKey(m, tea.KeyEnd, 0)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
		m = next.(model)
		if !strings.Contains(ansi.Strip(m.dialog()), "按键：P") {
			t.Fatal("resize left a blank scrolled detail")
		}
	}
}

func TestHelpMouseSelectionAndBackDoNotClickThrough(t *testing.T) {
	m := press(newModel(), "enter")
	m = press(m, "?")
	w, h, _ := m.helpSize()
	x, y := (m.width-w)/2, (m.height-h)/2
	next, _ := m.Update(tea.MouseWheelMsg{X: x + 4, Y: y + 4, Button: tea.MouseWheelDown})
	m = next.(model)
	if m.help.selected != 1 {
		t.Fatal("wheel did not move help selection")
	}
	m, _ = clickAt(m, x+4, y+5, tea.MouseLeft)
	if !m.help.detail || m.help.selected != 2 {
		t.Fatal("clicked row did not open its detail")
	}
	m, _ = clickAt(m, x+4, y+5, tea.MouseRight)
	if m.help.detail || !m.isHelp() {
		t.Fatal("right click did not return to list")
	}
	key := m.selectedKey()
	m, _ = clickAt(m, 1, 1, tea.MouseLeft)
	if m.modal != "" || m.selectedKey() != key || !m.reading {
		t.Fatal("outside click reached content behind help")
	}
}

func TestHelpEntriesMatchSiteCapabilities(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		m := newModel()
		m.opts.Demo = false
		var err error
		m.client, err = forum.NewBackend(site, "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, e := range m.helpEntries() {
			if found[e.key] || strings.ContainsAny(e.key+e.title, "\n\r") || e.detail == "" {
				t.Fatal("invalid or duplicated help row", e.key)
			}
			found[e.key] = true
		}
		caps := m.capabilities()
		if found["c"] != caps.Publish || found["r"] != (caps.Publish || caps.Reply) || found["m"] != caps.Mine || found["s"] != caps.Sage || found["x"] != caps.Manage {
			t.Fatal("help advertises unsupported operations", site)
		}
	}
}
