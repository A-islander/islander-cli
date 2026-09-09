package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/ansi"
)

func scrollingListModel(agent bool) model {
	m := newModel()
	m.chatStyle = agent
	posts := make([]forum.Post, 30)
	for i := range posts {
		posts[i] = forum.Post{ID: 100 + i, Body: fmt.Sprintf("正文%d", i)}
		if i%2 == 0 {
			posts[i].Title = fmt.Sprintf("标题%d", i)
		}
	}
	m.applyPage(forum.Page{Page: 1, List: posts})
	m.resize(120, 36)
	return m
}

func TestListSelectionCentersAndClampsAtEdges(t *testing.T) {
	for _, agent := range []bool{false, true} {
		m := scrollingListModel(agent)
		for i := 0; i < len(m.visible); i++ {
			m.moveSelection(i - m.selected)
			start := m.listRow(i) - m.listOffset()
			height := m.listItemHeight(i) - 1
			capacity := m.panelHeight() - 4
			if start < 0 || start+height > capacity {
				t.Fatal("selected card clipped")
			}
			atEdge := m.listOffset() == 0 || m.listOffset() == m.listRow(len(m.visible))-capacity
			if !atEdge {
				distance := start + (height / 2) - capacity/2
				if distance < -1 || distance > 1 {
					t.Fatal("interior card is not centered", i, distance)
				}
			}
		}
		m.moveSelection(-100)
		if m.listOffset() != 0 {
			t.Fatal("head did not clamp")
		}
	}
}

func TestListAnimationFramesAndContinuousInput(t *testing.T) {
	for _, agent := range []bool{false, true} {
		m := scrollingListModel(agent)
		m.moveSelection(10)
		from := m.listOffset()
		m, _ = updateKey(m, 'j', 0)
		if !m.listScrollValid() || m.listVisualOffset() != from {
			t.Fatal("scroll did not start from previous row")
		}
		generation := m.requestID
		s := m.listScroll
		next, _ := m.Update(listScrollTick{id: s.pending, at: s.started.Add(30 * time.Millisecond)})
		m = next.(model)
		if m.listVisualOffset() <= s.from || m.listVisualOffset() >= s.target || m.requestID != generation {
			t.Fatal("missing intermediate frame or frame triggered API work")
		}
		visible := m.listVisualOffset()
		m, _ = updateKey(m, 'j', 0)
		if m.listScroll.from != visible {
			t.Fatal("repeat input jumped to old animation target")
		}
		s = m.listScroll
		next, _ = m.Update(listScrollTick{id: s.pending, at: s.started.Add(readerScrollDuration)})
		m = next.(model)
		if m.listScroll.pending != 0 || m.listVisualOffset() != m.listOffset() {
			t.Fatal("animation did not settle")
		}
		if !strings.Contains(ansi.Strip(m.listContent()), "› ") {
			t.Fatal("selected card missing after animation")
		}
	}
}

func TestListMouseUsesVisibleAnimationAndModalCancelsIt(t *testing.T) {
	m := scrollingListModel(false)
	m.moveSelection(10)
	m, _ = updateKey(m, 'j', 0)
	y := m.listContentTop() + 1
	index := m.listAt(y)
	if index < 0 {
		y++
		index = m.listAt(y)
	}
	if index < 0 {
		t.Fatal("fixture points at a gap")
	}
	want := m.threads[m.visible[index]].id
	m, _ = clickAt(m, 7, y, tea.MouseLeft)
	if m.current().id != want || m.listScroll.pending != 0 {
		t.Fatal("clicked future animation position")
	}
	m = scrollingListModel(false)
	m.moveSelection(10)
	m, _ = updateKey(m, 'j', 0)
	m = press(m, "?")
	if m.listScroll.pending != 0 || !m.isHelp() {
		t.Fatal("opening help left list animation active")
	}
}
