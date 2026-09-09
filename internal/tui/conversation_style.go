package tui

import (
	"fmt"
	"github.com/A-islander/islander-cli/internal/local"
	"hash/fnv"
	"image/color"
	"path/filepath"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m model) modeButton() string {
	if m.chatStyle {
		return badge("F6 Agent模拟切换", ocean, teal)
	}
	return badge("F6 Agent模拟切换", teal, selectedBG)
}
func (m model) modeButtonX() int { return m.width - 1 - ansi.StringWidth(m.modeButton()) }
func (m model) browsePanel(content string, width, height int, focused bool) string {
	if !m.chatStyle {
		return panel(content, width, height, focused)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(foam)).Background(lipgloss.Color(ocean)).Padding(1, 3).Render(rectangle(content, width-6, height-2))
}
func (m *model) toggleChatStyle() {
	if (m.modal != "" && m.modal != "compose") || m.busy {
		return
	}
	key, delta := "", 0
	for _, item := range m.readerItems {
		if item.line <= m.reader.YOffset() {
			key, delta = item.key, m.reader.YOffset()-item.line
		}
	}
	m.chatStyle = !m.chatStyle
	m.rememberAppearance()
	m.lastListClick = listClick{}
	m.resize(m.width, m.height)
	for _, item := range m.readerItems {
		if item.key == key {
			m.reader.SetYOffset(min(item.line+delta, max(item.line, item.end-1)))
			break
		}
	}
	m.notice = "已切换论坛布局 · F6 Agent模拟切换"
	if m.chatStyle {
		m.notice = "Agent 模拟显示 · F6 返回论坛 · 下方回复框回复当前串"
	}
}

func (m model) replyBoxHeight() int {
	if m.chatStyle {
		return 5
	}
	return 0
}
func (m model) inlineAgentCompose() bool {
	return m.chatStyle && m.reading && m.modal == "compose" && m.current() != nil && m.draft.ThreadID == m.current().id
}
func (m *model) resizeEditor() {
	styles := textarea.DefaultDarkStyles()
	if m.chatStyle || m.theme.name == "el" {
		for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
			state.CursorLine = lipgloss.NewStyle()
			state.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(foam))
			state.Text = lipgloss.NewStyle().Foreground(lipgloss.Color(foam))
			state.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(muted))
			state.Selection = lipgloss.NewStyle().Background(lipgloss.Color(selectedBG))
			if m.chatStyle {
				state.Selection = lipgloss.NewStyle().Reverse(true)
			}
		}
	}
	if m.theme.name == "el" {
		styles.Cursor.Color = lipgloss.Color(foam)
		for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
			state.LineNumber = lipgloss.NewStyle().Foreground(lipgloss.Color(muted))
			state.CursorLineNumber = state.LineNumber
			state.EndOfBuffer = state.LineNumber
		}
	}
	m.styleThemeInputs()
	m.editor.SetStyles(styles)
	m.editor.Prompt = "┃ "
	if m.inlineAgentCompose() {
		m.editor.Prompt = "› "
	}
	if m.inlineAgentCompose() {
		m.editor.SetWidth(max(8, m.width-6))
		m.editor.SetHeight(max(1, m.replyBoxHeight()-2))
	} else {
		m.editor.SetWidth(max(10, min(70, m.width-14)))
		m.editor.SetHeight(max(3, min(12, m.height-16)))
	}
}

// The simulated status is decorative; it does not inspect local repositories
// or represent an actual model session.
const agentSession = "gpt-6-astra high · ~/Develope/islander · Main [default]"
const agentContentTop = 6

func (m model) agentBackButton() string { return strong("exit", foam) }

func (m model) agentBackButtonX() int { return m.width - 2 - ansi.StringWidth(m.agentBackButton()) }

func (m model) agentBackButtonAt(x, y int) bool {
	return m.chatStyle && m.reading && y == 1 && x >= m.agentBackButtonX() &&
		x < m.agentBackButtonX()+ansi.StringWidth(m.agentBackButton())
}

func (m model) readerTop() int {
	if m.chatStyle {
		return agentContentTop
	}
	return browseTop + 2
}
func (m model) listContentTop() int {
	if m.chatStyle {
		return agentContentTop
	}
	return browseTop + 3
}
func (m model) agentReplyTop() int { return m.height - 6 }
func (m model) agentReplyBox() string {
	w := m.width - 4
	body := ink("› Ask anything", muted)
	if t := m.current(); t != nil && m.draft.ThreadID == t.id && m.draft.Cookie == m.identity.Alias && strings.TrimSpace(m.draft.Body) != "" {
		body = ink("› "+compactPostBody(m.draft.Body), foam)
	}
	if m.inlineAgentCompose() {
		body = m.editor.View()
	}
	lines := []string{highlightLine("", w)}
	for _, line := range strings.Split(rectangle(body, w-2, 3), "\n") {
		lines = append(lines, highlightLine(" "+line+" ", w))
	}
	return strings.Join(append(lines, highlightLine("", w)), "\n")
}
func (m *model) clickAgentReply(c tea.MouseClickMsg) (tea.Cmd, bool) {
	if !m.chatStyle || c.Button != tea.MouseLeft || c.X < 2 || c.X >= m.width-2 || c.Y < m.agentReplyTop() || c.Y >= m.agentReplyTop()+m.replyBoxHeight() {
		return nil, false
	}
	if m.inlineAgentCompose() {
		return m.editor.Focus(), true
	}
	if m.modal != "" {
		return nil, true
	}
	if !m.focusMouseReader() {
		return nil, true
	}
	return m.beginCompose(true, false), true
}

func (m model) agentView() tea.View {
	w := m.width - 4
	operation := "Read internal/tui/model.go, internal/tui/view.go"
	heading := strong(">_ Codex", foam) + "\n\n" + strong("• Explored", foam) + "\n" + ink("  └ "+operation, muted)
	body := m.reader.View()
	if !m.reading {
		lines := strings.Split(m.listContent(), "\n")
		body = strings.Join(lines[min(2, len(lines)):], "\n")
	}
	feedback := ""
	if m.stateError != "" {
		feedback = m.stateError
	} else if m.opts.StateWarning != "" {
		feedback = m.opts.StateWarning
	} else if !m.busy && m.notice != "" && !agentRoutineNotice(m.notice) {
		feedback = m.notice
	}
	activity := feedback
	if m.agentWorkingVisible() {
		activity = m.agentWorkingLine()
		if feedback != "" {
			// Keep Working visible; action feedback replaces the decorative read line.
			lines := strings.Split(heading, "\n")
			lines[len(lines)-1] = ink(clip(agentDisplayText(feedback), w), muted)
			heading = strings.Join(lines, "\n")
		}
	}
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(rectangle("", m.width, m.height)),
		lipgloss.NewLayer(rectangle(heading, w, 4)).X(2).Y(1),
		lipgloss.NewLayer(rectangle(body, m.reader.Width(), m.reader.Height())).X(4).Y(agentContentTop),
		lipgloss.NewLayer(ink(clip(activity, w), muted)).X(2).Y(m.height - 7),
		lipgloss.NewLayer(m.agentReplyBox()).X(2).Y(m.agentReplyTop()),
		lipgloss.NewLayer(ink(clip(agentSession, w), muted)).X(2).Y(m.height - 1),
	}
	if m.reading {
		layers = append(layers, lipgloss.NewLayer(m.agentBackButton()).X(m.agentBackButtonX()).Y(1))
	}
	if m.modal != "" && !m.inlineAgentCompose() {
		dialog := m.dialog()
		layers = append(layers, lipgloss.NewLayer(dialog).X((m.width-lipgloss.Width(dialog))/2).Y((m.height-lipgloss.Height(dialog))/2).Z(1))
	}
	v := m.themedView(true, layers...)
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = !m.opts.Demo
	return v
}

var agentPalette = []struct{ from, to color.Color }{
	{lipgloss.Color(ocean), lipgloss.Color("#181818")},
	{lipgloss.Color(foam), lipgloss.Color("#e6e6e6")},
	{lipgloss.Color(muted), lipgloss.Color("#858585")},
	{lipgloss.Color(teal), lipgloss.Color("#dadada")},
	{lipgloss.Color(sand), lipgloss.Color("#a4a4a4")},
	{lipgloss.Color(lineColor), lipgloss.Color("#414141")},
	{lipgloss.Color(selectedBG), lipgloss.Color("#242424")},
}

func agentColor(c color.Color) color.Color {
	if c == nil {
		return c
	}
	r, g, b, a := c.RGBA()
	for _, entry := range agentPalette {
		er, eg, eb, ea := entry.from.RGBA()
		if r == er && g == eg && b == eb && a == ea {
			return entry.to
		}
	}
	// Preserve image placeholder IDs and other semantic colors.
	return c
}

func agentRoutineNotice(s string) bool {
	for _, part := range []string{"Agent 模拟显示", " · b 板块 / g 切站", " · [ ] 翻页 · v 引用", "我的内容 · 饼干", "引用已原位展开 ·", " 条引用 · ↓ 选中"} {
		if strings.Contains(s, part) {
			return true
		}
	}
	return false
}

var agentReferencePattern = regexp.MustCompile(`(?i)(?:No\.|>>|＞＞)\s*[0-9]+`)

func agentDisplayText(s string) string {
	return agentReferencePattern.ReplaceAllString(s, "[reference]")
}

// Each post gets a stable pseudo-random choice within the session. Never run
// these commands: this is decorative text, not a command or API request.
func (m model) agentToolLines(key string, primary bool, width int) []string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d/%s/%s", m.agentSeed, m.opts.Site, key)
	choice := h.Sum64()
	if !primary && choice%3 != 0 {
		return nil
	}
	samples := []string{
		`• Ran cat internal/tui/model.go
  └ package tui

    import (
      "fmt"
      "strings"
    )`,
		`• Ran env GOCACHE=/tmp/islander-tui-go-build GOMODCACHE=/tmp/islander-tui-go-mod go vet ./...
  └ (no output)`,
		`• Ran set -e
  │ env GOCACHE=/tmp/islander-tui-go-build GOMODCACHE=/tmp/islander-tui-go-mod go build -o bin/.islander-next ./cmd/islander
  │ cp bin/.islander-next bin/.islander-dev-next
  │ … +2 lines
  └ (no output)`,
		`• Ran python3 - <<'PY'
  │ from pathlib import Path
  │ p=Path('.local/mouse-agent-smoke.py'); s=p.read_text()
  │ … +5 lines
  └ {"mouseToggle": true, "inlineReplyBox": true, "cookieCheck": true,
    "oneThreadRequestAcrossModeAndFocusChanges": true}
    e3644f991b47e803362c3471330978dd09798171b4e353a761ece38b46d37a2e  bin/islander
    e3644f991b47e803362c3471330978dd09798171b4e353a761ece38b46d37a2e  bin/islander-dev`,
	}
	return agentRenderToolBlock(samples[(choice/3)%uint64(len(samples))], width)
}

func (m model) preferencesRoot() string {
	if m.opts.DataDir != "" {
		return m.opts.DataDir
	}
	if m.store != nil {
		return filepath.Dir(m.store.Dir)
	}
	return ""
}
func (m *model) loadUIPreferences() {
	if m.opts.Demo {
		return
	}
	p, err := local.ReadPreferences(m.preferencesRoot())
	if err != nil {
		m.opts.StateWarning = err.Error()
		return
	}
	m.siteConfigs, m.chatStyle = p.Sites, p.AgentSimulation
	if m.opts.Theme == "" && ValidTheme(p.Theme) {
		m.theme.name = p.Theme
	}
	m.resize(m.width, m.height)
}
func (m *model) rememberAppearance() {
	if m.opts.Demo || m.store == nil {
		return
	}
	if err := local.RememberAgentSimulation(m.preferencesRoot(), m.chatStyle); err != nil {
		m.opts.StateWarning = "界面偏好保存失败：" + err.Error()
	}
}
