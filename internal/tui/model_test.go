package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func press(m model, key string) model {
	code := map[string]rune{"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "down": tea.KeyDown, "up": tea.KeyUp}[key]
	msg := tea.KeyPressMsg{Code: code}
	if code == 0 {
		msg.Code = []rune(key)[0]
		msg.Text = key
	}
	next, _ := m.Update(msg)
	return next.(model)
}

func TestBrowseAndQuotePreservePosition(t *testing.T) {
	m := press(newModel(), "enter")
	for range 3 {
		m = press(m, "n")
	}
	if m.activePost != 3 {
		t.Fatal("expected third reply")
	}
	offset := m.reader.YOffset()
	m = press(m, "v")
	if m.modal != "" || len(m.inlineQuotes["10433"]) != 1 || m.inlineQuotes["10433"][0].post.id != 10431 {
		t.Fatal("wrong quoted post")
	}
	m = press(m, "down")
	m = press(m, "esc")
	if m.reader.YOffset() != offset || m.activePost != 3 {
		t.Fatal("quote changed reading position")
	}
	m = press(m, "esc")
	if m.reading {
		t.Fatal("expected list focus")
	}
	m = press(m, "down")
	m = press(m, "up")
	m = press(m, "enter")
	if m.current().id != 10428 || m.reader.YOffset() != offset {
		t.Fatal("reopening lost position")
	}
}

func TestFilterJumpAndCancel(t *testing.T) {
	m := press(newModel(), "/")
	if m.modal != "filter" || !m.input.Focused() {
		t.Fatal("filter did not open")
	}
	m = press(m, "薄")
	m = press(m, "荷")
	m = press(m, "enter")
	if len(m.visible) != 1 || m.current().id != 10428 {
		t.Fatal("filter failed")
	}
	m = press(m, "/")
	m.input.SetValue("nothing matches")
	m = press(m, "esc")
	if m.filter != "薄荷" {
		t.Fatal("cancel changed active filter")
	}
	m = press(m, "/")
	m.input.SetValue("nothing matches")
	m = press(m, "enter")
	if m.current() != nil || !strings.Contains(ansi.Strip(m.View().Content), "暂时没有匹配") {
		t.Fatal("empty state missing")
	}
	m = press(m, ":")
	m.input.SetValue("No.10433")
	m = press(m, "enter")
	if !m.reading || m.current().id != 10428 || m.activePost != 3 || m.modal != "" {
		t.Fatal("reply jump failed")
	}
	before := m.reader.YOffset()
	m = press(m, ":")
	m.input.SetValue("999999")
	m = press(m, "enter")
	if m.reader.YOffset() != before || !strings.Contains(m.notice, "未找到") {
		t.Fatal("failed jump changed position")
	}
}

func TestBoardAndNarrowNavigation(t *testing.T) {
	m := newModel()
	m.resize(60, 24)
	m = press(m, "4")
	if len(m.visible) != 2 || m.current().board != "技术" {
		t.Fatal("board filter failed")
	}
	if m.split() {
		t.Fatal("narrow screen should be single column")
	}
	m = press(m, "enter")
	if !m.reading || !strings.Contains(ansi.Strip(m.View().Content), "主楼") {
		t.Fatal("reader missing")
	}
	m = press(m, "n")
	position := m.reader.YOffset()
	m = press(m, "esc")
	if !strings.Contains(ansi.Strip(m.View().Content), "岛上此刻") {
		t.Fatal("list missing")
	}
	m = press(m, "enter")
	if m.reader.YOffset() != position {
		t.Fatal("narrow return lost position")
	}
}

func TestViewsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{140, 42}, {120, 36}, {100, 24}, {99, 24}, {80, 24}, {60, 20}, {44, 16}, {32, 12}} {
		for _, mode := range []string{"list", "reader", "help", "quote", "filter", "jump", "empty"} {
			m := newModel()
			m.resize(size[0], size[1])
			switch mode {
			case "reader":
				m.reading = true
			case "help":
				m.modal = "help"
			case "quote":
				m.reading = true
				m.activePost = 3
				m.toggleInlineQuotes()
			case "filter", "jump":
				m.startInput(mode)
			case "empty":
				m.filter = "no-match"
				m.refilter()
			}
			view := m.View().Content
			if lipgloss.Height(view) != size[1] {
				t.Errorf("%v %s height = %d", size, mode, lipgloss.Height(view))
			}
			for n, line := range strings.Split(view, "\n") {
				if w := ansi.StringWidth(line); w > size[0] {
					t.Errorf("%v %s line %d width = %d", size, mode, n, w)
				}
			}
		}
	}
}

func TestViewHasConsistentBackground(t *testing.T) {
	for _, mode := range []string{"list", "quote", "help", "filter", "narrow", "small"} {
		m := newModel()
		switch mode {
		case "quote":
			m.reading = true
			m.activePost = 3
			m.toggleInlineQuotes()
		case "help":
			m.modal = "help"
		case "filter":
			m.startInput("filter")
		case "narrow":
			m.resize(60, 24)
			m.reading = true
		case "small":
			m.resize(32, 12)
		}
		canvas := lipgloss.NewCanvas(m.width, m.height).Compose(lipgloss.NewLayer(m.View().Content))
		missing := 0
		for y := 0; y < m.height; y++ {
			for x := 0; x < m.width; x++ {
				c := canvas.CellAt(x, y)
				if c.Width == 0 {
					continue
				} // continuation cell of a wide grapheme
				if c.Style.Bg == nil {
					missing++
				}
			}
		}
		if missing != 0 {
			t.Errorf("%s: %d cells fall back to the terminal background", mode, missing)
		}
		if mode == "list" {
			r, g, b, _ := canvas.CellAt(4, 8).Style.Bg.RGBA()
			wr, wg, wb, _ := lipgloss.Color(selectedBG).RGBA()
			if r != wr || g != wg || b != wb {
				t.Error("selection background was lost")
			}
		}
	}
}
