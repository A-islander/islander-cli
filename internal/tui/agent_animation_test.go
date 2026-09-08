package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAgentWorkingClockLifecycle(t *testing.T) {
	m := newModel()
	m.reading = true
	base := time.Unix(1700000000, 0)
	if m.syncAgentWorking(base) != nil {
		t.Fatal("forum layout scheduled animation")
	}
	m.chatStyle = true
	if m.syncAgentWorking(base.Add(271*time.Second)) == nil {
		t.Fatal("visible agent reader did not animate")
	}
	if got := ansi.Strip(m.agentWorkingLine()); got != "• Working (4m 31s • esc to interrupt)" {
		t.Fatalf("wrong reading time: %s", got)
	}
	pending := m.agentWorking.pending
	if m.syncAgentWorking(base.Add(272*time.Second)) != nil || m.agentWorking.pending != pending {
		t.Fatal("duplicate animation timer")
	}
	// Timer frames do not refresh the reader, change request IDs, or schedule persistence.
	content, request, stateTick := m.reader.GetContent(), m.requestID, m.stateTickID
	next, cmd := m.Update(agentWorkingTick{pending, base.Add(273 * time.Second)})
	m = next.(model)
	if cmd == nil || m.agentWorking.pending == pending || m.requestID != request || m.stateTickID != stateTick || m.reader.GetContent() != content {
		t.Fatal("animation modified browsing state")
	}
	stale := m.agentWorking.pending
	m.modal = "compose"
	if m.syncAgentWorking(base.Add(274*time.Second)) != nil || m.agentWorking.pending != 0 {
		t.Fatal("hidden animation did not stop")
	}
	if m.advanceAgentWorking(agentWorkingTick{stale, base.Add(275 * time.Second)}) != nil {
		t.Fatal("stale frame revived animation")
	}
	m.modal = ""
	m.syncAgentWorking(base.Add(276 * time.Second))
	if !m.agentWorking.started.Equal(base) {
		t.Fatal("modal reset reading time")
	}
	m.reading = false
	m.syncAgentWorking(base.Add(277 * time.Second))
	if !m.agentWorking.started.IsZero() || m.agentWorking.pending != 0 {
		t.Fatal("leaving thread did not stop timer")
	}
	m.reading = true
	m.syncAgentWorking(base.Add(280 * time.Second))
	if !m.agentWorking.started.Equal(base.Add(280 * time.Second)) {
		t.Fatal("new reading session did not restart timer")
	}
}

func TestAgentWorkingHighlightMovesLeftToRight(t *testing.T) {
	m := newModel()
	m.chatStyle, m.reading, m.notice = true, true, ""
	m.agentWorking.started = time.Unix(1700000000, 0)
	for step := 0; step < 7; step++ {
		m.agentWorking.now = m.agentWorking.started.Add(time.Duration(step+3) * agentFrameInterval)
		view := m.View()
		canvas := lipgloss.NewCanvas(m.width, m.height).Compose(lipgloss.NewLayer(view.Content))
		for i := 0; i < 7; i++ {
			r, g, b, _ := canvas.CellAt(4+i, m.height-7).Style.Fg.RGBA()
			white := r == 0xffff && g == 0xffff && b == 0xffff
			if white != (i == step) {
				t.Fatalf("frame %d highlight at wrong character %d", step, i)
			}
		}
	}
}

func TestAgentWorkingEscapeAndModeToggle(t *testing.T) {
	m := press(newModel(), "enter")
	started := m.agentWorking.started
	m, _ = updateKey(m, tea.KeyF6, 0)
	if !m.agentWorking.started.Equal(started) || !strings.Contains(ansi.Strip(m.View().Content), "esc to interrupt") {
		t.Fatal("mode toggle reset or hid timer")
	}
	m, _ = updateKey(m, tea.KeyEscape, 0)
	if m.reading || !m.agentWorking.started.IsZero() || strings.Contains(ansi.Strip(m.View().Content), "Working (") {
		t.Fatal("Escape did not end reading animation")
	}
}

func TestAgentToolSyntaxSurvivesThemeAndWrapping(t *testing.T) {
	line := "• Ran cat --number internal/tui/model.go 'literal'"
	colored := agentHighlightToolLine(line)
	if ansi.Strip(colored) != line {
		t.Fatal("syntax renderer changed command text")
	}
	view := themedScreenView(100, 1, true, lipgloss.NewLayer(colored))
	canvas := lipgloss.NewCanvas(100, 1).Compose(lipgloss.NewLayer(view.Content))
	for _, token := range []struct{ text, color string }{{"cat", agentCommandColor}, {"--number", agentKeywordColor}, {"internal/tui/model.go", agentPathColor}, {"'literal'", agentStringColor}} {
		x := ansi.StringWidth(line[:strings.Index(line, token.text)])
		actual := canvas.CellAt(x, 0).Style.Fg
		r, g, b, a := actual.RGBA()
		er, eg, eb, ea := lipgloss.Color(token.color).RGBA()
		if r != er || g != eg || b != eb || a != ea {
			t.Fatalf("lost %s syntax color in final view", token.text)
		}
	}
	for _, width := range []int{30, 44, 100} {
		lines := agentRenderToolBlock("• Ran env GOCACHE=/tmp/islander-tui-go-build GOMODCACHE=/tmp/islander-tui-go-mod go vet ./...\n  └ (no output)", width)
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatal("syntax highlight overflowed terminal")
			}
		}
		if width < 100 && !strings.HasPrefix(ansi.Strip(lines[1]), "  │ ") {
			t.Fatal("wrapped command lost tree continuation")
		}
	}
}
