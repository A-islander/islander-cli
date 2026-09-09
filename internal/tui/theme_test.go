package tui

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/A-islander/islander-cli/internal/local"
)

func TestTerminalThemeColorsRemainReadable(t *testing.T) {
	for _, pair := range [][2]string{{"#ff9c00", "#120800"}, {"#333333", "#ffffff"}, {"#888888", "#777777"}, {"#eeeeee", "#151515"}} {
		p := (terminalTheme{name: "el", fg: lipgloss.Color(pair[0]), bg: lipgloss.Color(pair[1])}).palette()
		mapped := func(s string) color.Color { return p.mapColor(lipgloss.Color(s)) }
		if rgb(mapped(ocean)) != rgb(lipgloss.Color(pair[1])) {
			t.Fatal("background does not follow terminal")
		}
		for _, background := range []string{ocean, selectedBG} {
			for _, text := range []string{foam, muted} {
				if ratio := contrast(mapped(text), mapped(background)); ratio < 4.5 {
					t.Fatalf("%v: %s on %s has insufficient contrast %.2f", pair, text, background, ratio)
				}
			}
		}
		if pair[0] == "#ff9c00" {
			c := rgb(mapped(foam))
			if c.R <= c.G || c.G <= c.B || c.R-c.B >= 200 {
				t.Fatalf("lost soft amber tone: %#v", c)
			}
		}
	}
}

func TestTerminalThemeComposesTextSelectionAndImages(t *testing.T) {
	theme := terminalTheme{name: "el", fg: lipgloss.Color("#ff9c00"), bg: lipgloss.Color("#120800")}
	for _, agent := range []bool{false, true} {
		m := newModel()
		m.theme, m.chatStyle = theme, agent
		m.reading = true
		m.resize(120, 36)
		view := m.View()
		canvas := lipgloss.NewCanvas(m.width, m.height).Compose(lipgloss.NewLayer(view.Content))
		background := rgb(theme.bg)
		selection := rgb(theme.palette().mapColor(lipgloss.Color(selectedBG)))
		selected := 0
		for y := 0; y < m.height; y++ {
			for x := 0; x < m.width; x++ {
				c := canvas.CellAt(x, y)
				if c == nil || c.Width == 0 {
					continue
				}
				bg := rgb(c.Style.Bg)
				if bg == rgb(lipgloss.Color(ocean)) || bg == rgb(lipgloss.Color("#181818")) {
					t.Fatal("original background leaked through theme")
				}
				if bg == selection {
					selected++
				}
			}
		}
		if rgb(canvas.CellAt(0, 0).Style.Bg) != background || selected < 20 {
			t.Fatal("lost full background or selection")
		}
	}
	// Deliberately make image pixels/IDs equal theme source colors.
	content := lipgloss.NewStyle().Foreground(lipgloss.Color(foam)).Background(lipgloss.Color(selectedBG)).Render("▀") +
		lipgloss.NewStyle().Foreground(lipgloss.Color(teal)).Background(lipgloss.Color(ocean)).Render("\U0010eeee")
	v := paletteScreenView(2, 1, true, theme.palette(), lipgloss.NewLayer(content))
	c := lipgloss.NewCanvas(2, 1).Compose(lipgloss.NewLayer(v.Content))
	if rgb(c.CellAt(0, 0).Style.Fg) != rgb(lipgloss.Color(foam)) || rgb(c.CellAt(0, 0).Style.Bg) != rgb(lipgloss.Color(selectedBG)) {
		t.Fatal("image pixels recolored")
	}
	if rgb(c.CellAt(1, 0).Style.Fg) != rgb(lipgloss.Color(teal)) {
		t.Fatal("Kitty image ID recolored")
	}
}

func TestThemeFallbackAndUpdatesPreserveReader(t *testing.T) {
	m := press(newModel(), "enter")
	offset, key, request := m.reader.YOffset(), m.selectedKey(), m.requestID
	original := m.View().Content
	m, cmd := updateKey(m, tea.KeyF7, 0)
	if cmd == nil || m.theme.name != "el" {
		t.Fatal("theme switch did not request colors")
	}
	// No terminal reply: render synchronously with the original colors.
	if m.theme.palette() != nil {
		t.Fatal("missing color reply should retain original palette")
	}
	fallback := lipgloss.NewCanvas(m.width, m.height).Compose(lipgloss.NewLayer(m.View().Content))
	if rgb(fallback.CellAt(0, 0).Style.Bg) != rgb(lipgloss.Color(ocean)) {
		t.Fatal("fallback changed background")
	}
	for _, msg := range []tea.Msg{tea.ForegroundColorMsg{Color: lipgloss.Color("#ff9c00")}, tea.BackgroundColorMsg{Color: lipgloss.Color("#120800")}} {
		next, c := m.Update(msg)
		m = next.(model)
		if c != nil {
			t.Fatal("color reply triggered work")
		}
	}
	if m.reader.YOffset() != offset || m.selectedKey() != key || m.requestID != request || !m.reading {
		t.Fatal("theme update changed reading state")
	}
	if m.View().Content == original {
		t.Fatal("terminal replies did not affect rendering")
	}
	m, cmd = updateKey(m, tea.KeyF7, 0)
	if cmd != nil || m.theme.name != "islander" || m.theme.palette() != nil {
		t.Fatal("cannot return to original theme")
	}
}

func TestThemePreferencePreservesSitesAndAgentMode(t *testing.T) {
	dir := t.TempDir()
	m, _ := persistenceModel(t, dir, "x")
	m.toggleChatStyle()
	m.toggleTheme()
	p, err := local.ReadPreferences(dir)
	if err != nil || p.Theme != "el" || !p.AgentSimulation {
		t.Fatal("theme/Agent choice did not persist", err)
	}
	next, _ := persistenceModel(t, dir, "x")
	next.loadUIPreferences()
	if next.theme.name != "el" || !next.chatStyle {
		t.Fatal("restart lost appearance")
	}
	next.toggleTheme()
	p, err = local.ReadPreferences(dir)
	if err != nil || p.Theme != "islander" || !p.AgentSimulation {
		t.Fatal("toggle overwrote another preference")
	}
	// An explicit command-line theme takes precedence over stored preferences.
	next.opts.Theme = "el"
	next.theme.name = "el"
	next.loadUIPreferences()
	if next.theme.name != "el" {
		t.Fatal("stored theme replaced explicit flag")
	}
}
