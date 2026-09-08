package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/ansi"
)

func TestMixedTitleListFitsRowsAndKeepsSelectionVisible(t *testing.T) {
	m := newModel()
	m.opts.Demo = false
	posts := make([]forum.Post, 12)
	for i := range posts {
		posts[i] = forum.Post{ID: 1000 + i, Body: fmt.Sprintf("正文%d", i)}
		if i%2 == 1 {
			posts[i].Title = fmt.Sprintf("标题%d", i)
		}
	}
	m.applyPage(forum.Page{Page: 1, List: posts})
	lines := strings.Split(ansi.Strip(m.listPanel()), "\n")
	find := func(text string) int {
		for i, line := range lines {
			if strings.Contains(line, text) {
				return i
			}
		}
		return -1
	}
	if find("标题1")-find("正文0") != 3 || find("正文2")-find("标题1") != 4 {
		t.Fatal("untitled and titled cards should occupy three and four rows")
	}
	if find("正文6") < 0 || find("标题7") >= 0 {
		t.Fatal("mixed rows did not fill the viewport with seven complete cards")
	}
	m, _ = updateKey(m, tea.KeyPgDown, 0)
	if m.selected != 7 || !strings.Contains(ansi.Strip(m.listPanel()), "› 标题7") {
		t.Fatal("page down lost the selected card")
	}
	m, _ = updateKey(m, tea.KeyPgUp, 0)
	if m.selected != 0 || !strings.Contains(ansi.Strip(m.listPanel()), "› 正文0") {
		t.Fatal("page up did not return to the first card")
	}
	m.moveSelection(100)
	m.resize(60, 24)
	if !strings.Contains(ansi.Strip(m.listPanel()), "› 标题11") {
		t.Fatal("resize hid the selected final card")
	}
	// Without titles the same 120x36 terminal now fits eight cards.
	for i := range posts {
		posts[i].Title = ""
	}
	m.resize(120, 36)
	m.applyPage(forum.Page{Page: 1, List: posts})
	view := ansi.Strip(m.listPanel())
	if !strings.Contains(view, "正文7") || strings.Contains(view, "正文8") {
		t.Fatal("untitled cards did not use the extra space")
	}
}
