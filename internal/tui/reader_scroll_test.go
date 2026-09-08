package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func longReplyModel(width, height int, nested, last bool) model {
	m := newModel()
	m.current().title = ""
	root := post{id: 100, body: "主楼"}
	long := post{id: 101, body: strings.Repeat("逐行阅读，不要漏掉正文。\n", 60) + "正文尾巴"}
	tail := post{id: 102, body: "下一楼"}
	m.current().posts = []post{root, long, tail}
	if last {
		m.current().posts = []post{root, long}
	}
	m.reading = true
	if nested {
		m.current().posts = []post{root, tail}
		m.inlineQuotes["100"] = []inlineQuote{{post: long}}
		m.activePost, m.activeQuote = 0, "100/101"
	} else {
		m.activePost = 1
	}
	m.resize(width, height)
	m.refreshReader(false)
	for _, item := range m.readerItems {
		if item.key == m.selectedKey() {
			m.reader.SetYOffset(item.line)
		}
	}
	return m
}
func selectedReaderItem(t *testing.T, m model) readerItem {
	t.Helper()
	for _, item := range m.readerItems {
		if item.key == m.selectedKey() {
			return item
		}
	}
	t.Fatal("selected item missing")
	return readerItem{}
}

func TestLongReplyAndQuoteAreReadBeforeMovingOn(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {60, 24}, {44, 16}} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprint(size, nested), func(t *testing.T) {
				m := longReplyModel(size[0], size[1], nested, false)
				item := selectedReaderItem(t, m)
				for steps := 0; m.reader.YOffset()+m.reader.Height() < item.end; steps++ {
					if steps > 1000 {
						t.Fatal("reader stuck before end of long post")
					}
					before := m.reader.YOffset()
					key := "j"
					if steps%2 == 0 {
						key = "down"
					}
					m = press(m, key)
					if m.selectedKey() != item.key || m.reader.YOffset() != before+1 {
						t.Fatal("j/down skipped unread content")
					}
				}
				if !strings.Contains(ansi.Strip(m.reader.View()), "正文尾巴") {
					t.Fatal("tail never visible before moving on")
				}
				m = press(m, "j")
				if m.selectedKey() != "102" {
					t.Fatal("cannot leave completely visible post")
				}
				m = press(m, "k")
				if m.selectedKey() != item.key || !strings.Contains(ansi.Strip(m.reader.View()), "正文尾巴") {
					t.Fatal("k skipped previous long post's tail")
				}
				for m.reader.YOffset() > item.line {
					before := m.reader.YOffset()
					m = press(m, "up")
					if m.selectedKey() != item.key || m.reader.YOffset() != before-1 {
						t.Fatal("up skipped unread content")
					}
				}
				m = press(m, "k")
				if m.selectedKey() != "100" {
					t.Fatal("cannot leave top of long reply")
				}
			})
		}
	}
}

func TestLastLongReplyDoesNotJumpBackAndExplicitSkipWorks(t *testing.T) {
	m := longReplyModel(60, 24, false, true)
	item := selectedReaderItem(t, m)
	for m.reader.YOffset()+m.reader.Height() < item.end {
		m = press(m, "j")
	}
	offset := m.reader.YOffset()
	m = press(m, "j")
	if m.reader.YOffset() != offset || m.selectedKey() != item.key {
		t.Fatal("j at final post's bottom jumped to its beginning")
	}
	m = longReplyModel(60, 24, false, false)
	m = press(m, "n")
	if m.selectedKey() != "102" {
		t.Fatal("explicit n no longer skips long post")
	}
	m = press(m, "p")
	if m.selectedKey() != "101" || m.reader.YOffset() != selectedReaderItem(t, m).line {
		t.Fatal("explicit p no longer jumps to post start")
	}
}

func TestLongReaderResizeKeepsScrollInsteadOfSkipping(t *testing.T) {
	m := longReplyModel(120, 36, true, false)
	for range 5 {
		m = press(m, "j")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 44, Height: 16})
	m = next.(model)
	before := m.reader.YOffset()
	m = press(m, "j")
	if m.selectedKey() != "100/101" || m.reader.YOffset() != before+1 {
		t.Fatal("narrowing window lost long quote scroll")
	}
}

func TestReaderMovesSelectionBeforeScrolling(t *testing.T) {
	m := newModel()
	m.reading = true
	m.current().title = ""
	m.current().posts = []post{
		{id: 100, body: "主楼"}, {id: 101, body: "第一条"},
		{id: 102, body: "第二条"}, {id: 103, body: "第三条"},
		{id: 104, body: "第四条"}, {id: 105, body: strings.Repeat("长回复正文\n", 40)},
	}
	m.resize(120, 28)
	m.reader.GotoTop()
	m = press(m, "j")
	if m.activePost != 1 || m.reader.YOffset() != 0 {
		t.Fatal("selecting a visible reply scrolled it to the top")
	}
	m = press(m, "k")
	if m.activePost != 0 || m.reader.YOffset() != 0 {
		t.Fatal("selecting the visible previous post moved the viewport")
	}
	for m.activePost < 3 {
		m = press(m, "j")
	}
	if m.reader.YOffset() != 0 {
		t.Fatal("viewport moved before the cursor reached its bottom")
	}
	m = press(m, "j")
	item := selectedReaderItem(t, m)
	if m.activePost != 4 || item.line < m.reader.YOffset() || item.end != m.reader.YOffset()+m.reader.Height() {
		t.Fatal("offscreen reply was not fully revealed with the smallest scroll")
	}
	m = press(m, "j")
	item = selectedReaderItem(t, m)
	if m.activePost != 5 || m.reader.YOffset() != item.line {
		t.Fatal("long reply did not start at its header")
	}
	before := m.reader.YOffset()
	m = press(m, "j")
	if m.activePost != 5 || m.reader.YOffset() != before+1 {
		t.Fatal("long reply was skipped instead of scrolling one line")
	}
}
