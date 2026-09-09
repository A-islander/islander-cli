package tui

import (
	"image/color"
	"math"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/A-islander/islander-cli/internal/local"
)

// Terminal colors are session state, never persisted: another terminal may use
// a different palette. Queries are asynchronous and cannot delay forum loading.
type terminalTheme struct {
	name   string
	fg, bg color.Color
}

func ValidTheme(name string) bool { return name == "" || name == "islander" || name == "el" }

func (m model) requestThemeColors() tea.Cmd {
	if m.theme.name != "el" {
		return nil
	}
	return tea.Batch(tea.RequestForegroundColor, tea.RequestBackgroundColor)
}

func (m *model) toggleTheme() tea.Cmd {
	if m.theme.name == "el" {
		m.theme.name = "islander"
	} else {
		m.theme.name = "el"
	}
	m.opts.Theme = m.theme.name
	m.resizeEditor()
	m.notice = "主题：原主题 · F7 el"
	if m.theme.name == "el" {
		m.notice = "主题：el（柔和跟随终端）· F7 原主题"
	}
	if !m.opts.Demo && m.store != nil {
		if err := local.RememberTheme(m.preferencesRoot(), m.theme.name); err != nil {
			m.opts.StateWarning = "主题保存失败：" + err.Error()
		}
	}
	return m.requestThemeColors()
}

func rgb(c color.Color) color.NRGBA {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

func blend(a, b color.Color, amount float64) color.Color {
	x, y := rgb(a), rgb(b)
	channel := func(v, w uint8) uint8 { return uint8(math.Round(float64(v)*(1-amount) + float64(w)*amount)) }
	return color.NRGBA{channel(x.R, y.R), channel(x.G, y.G), channel(x.B, y.B), 255}
}

func luminance(c color.Color) float64 {
	x := rgb(c)
	linear := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= .04045 {
			return f / 12.92
		}
		return math.Pow((f+.055)/1.055, 2.4)
	}
	return .2126*linear(x.R) + .7152*linear(x.G) + .0722*linear(x.B)
}

func contrast(a, b color.Color) float64 {
	x, y := luminance(a), luminance(b)
	return (max(x, y) + .05) / (min(x, y) + .05)
}

func readable(fg, bg color.Color, minimum float64) color.Color {
	if contrast(fg, bg) >= minimum {
		return fg
	}
	var target color.Color = color.White
	if contrast(color.Black, bg) > contrast(color.White, bg) {
		target = color.Black
	}
	for step := 1; step <= 100; step++ {
		candidate := blend(fg, target, float64(step)/100)
		if contrast(candidate, bg) >= minimum {
			return candidate
		}
	}
	return target
}

func soften(c color.Color) color.Color {
	x := rgb(c)
	if int(max(x.R, x.G, x.B))-int(min(x.R, x.G, x.B)) < 80 {
		return c
	}
	gray := uint8(math.Round(.2126*float64(x.R) + .7152*float64(x.G) + .0722*float64(x.B)))
	return blend(c, color.NRGBA{gray, gray, gray, 255}, .45)
}

type themePalette map[color.NRGBA]color.Color

func (t terminalTheme) palette() themePalette {
	if t.name != "el" || t.fg == nil || t.bg == nil {
		return nil
	}
	bg := t.bg
	body := readable(soften(t.fg), bg, 7)
	selection := blend(bg, body, .14)
	// Near middle gray, shading toward text can make even black/white text
	// unreadable. Shade away from text instead, keeping one text polarity.
	if contrast(body, selection) < 4.5 {
		var away color.Color = color.White
		if luminance(body) > luminance(bg) {
			away = color.Black
		}
		selection = blend(bg, away, .14)
	}
	secondary := readable(readable(blend(bg, body, .65), bg, 4.5), selection, 4.5)
	accent := readable(blend(body, t.fg, .3), bg, 4.5)
	p := themePalette{}
	for from, to := range map[string]color.Color{
		ocean: bg, foam: body, muted: secondary, teal: accent, sand: accent,
		lineColor: blend(bg, body, .4), selectedBG: selection,
	} {
		p[rgb(lipgloss.Color(from))] = to
	}
	// The working shimmer and syntax samples also adapt; image colors never do.
	for from, to := range map[string]color.Color{
		"#858585": secondary, "#aaaaaa": blend(secondary, body, .4),
		"#dddddd": body, "#ffffff": readable(t.fg, bg, 7),
	} {
		p[rgb(lipgloss.Color(from))] = to
	}
	for _, from := range []string{agentCommandColor, agentKeywordColor, agentPathColor, agentStringColor, agentNumberColor} {
		p[rgb(lipgloss.Color(from))] = readable(blend(lipgloss.Color(from), body, .55), bg, 4.5)
	}
	return p
}

func (p themePalette) mapColor(c color.Color) color.Color {
	if c != nil {
		if mapped, ok := p[rgb(c)]; ok {
			return mapped
		}
	}
	return c
}

func (m model) themedView(agent bool, layers ...*lipgloss.Layer) tea.View {
	return paletteScreenView(m.width, m.height, agent, m.theme.palette(), layers...)
}

func (m *model) styleThemeInputs() {
	styles := textinput.DefaultDarkStyles()
	if m.theme.name == "el" {
		for _, state := range []*textinput.StyleState{&styles.Focused, &styles.Blurred} {
			state.Text = lipgloss.NewStyle().Foreground(lipgloss.Color(foam))
			state.Prompt = state.Text
			state.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(muted))
			state.Suggestion = state.Placeholder
		}
		styles.Cursor.Color = lipgloss.Color(foam)
	}
	m.input.SetStyles(styles)
	m.titleInput.SetStyles(styles)
}
