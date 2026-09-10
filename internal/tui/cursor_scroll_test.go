package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHalfPageAndWheelFollowSelectedItem(t *testing.T) {
	for _, reading := range []bool{false, true} {
		for _, agent := range []bool{false, true} {
			t.Run(fmt.Sprintf("reader=%v/agent=%v", reading, agent), func(t *testing.T) {
				m, _ := aheadModel(t, reading, agent)
				// This case exercises short items. Wrapped Agent decorations can
				// exceed a small viewport and correctly consume wheel input as lines.
				m.resize(120, 36)
				if reading {
					for _, item := range m.readerItems {
						if item.end-item.line > m.reader.Height() {
							t.Fatal("short-item fixture exceeds viewport", item.key)
						}
					}
				}
				aheadSelect(&m, 10)
				if reading && m.activePost != 10 || !reading && m.selected != 10 {
					t.Fatal("fixture did not start at the requested item")
				}
				before := m.current().id
				if reading {
					before = m.current().posts[m.activePost].id
				}
				m, _ = updateKey(m, 'd', tea.ModCtrl)
				index := m.selected
				if reading {
					index = m.activePost
				}
				if index <= 10 {
					t.Fatal("half-page did not move cursor forward", index)
				}
				if reading {
					item := selectedReaderItem(t, m)
					if item.line < m.reader.YOffset() {
						t.Fatal("selected item scrolled offscreen")
					}
				} else {
					center := m.listRow(index) + (m.listItemHeight(index)-1)/2 - m.listOffset()
					if center != (m.panelHeight()-4)/2 {
						t.Fatal("list half-page did not center cursor")
					}
				}
				x, y := m.listContentTop(), m.listContentTop()
				if reading {
					x, y = m.readerX()+6, m.readerTop()+1
				} else {
					x = 5
				}
				next, _ := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})
				m = next.(model)
				after := m.selected
				if reading {
					after = m.activePost
				}
				if after != index+1 {
					t.Fatal("one wheel event did not select next item", index, after)
				}
				next, _ = m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelUp})
				m = next.(model)
				after = m.selected
				if reading {
					after = m.activePost
				}
				if after != index {
					t.Fatal("wheel up did not reverse selection")
				}
				m, _ = updateKey(m, 'u', tea.ModCtrl)
				after = m.current().id
				if reading {
					after = m.current().posts[m.activePost].id
				}
				if after >= before+index-10 {
					t.Fatal("half-page up did not move backwards")
				}
			})
		}
	}
}

func TestHalfPagePreservesLongReplyAndNestedQuote(t *testing.T) {
	for _, nested := range []bool{false, true} {
		m, _ := aheadModel(t, true, false)
		aheadSelect(&m, 10)
		long := strings.Repeat("长正文不能跳过\n", 100)
		if nested {
			m.inlineQuotes["1010"] = []inlineQuote{{post: post{id: 999, body: long}}}
			m.activeQuote = "1010/999"
		} else {
			m.current().posts[m.activePost].body = long
		}
		m.refreshReader(false)
		item := selectedReaderItem(t, m)
		m.reader.SetYOffset(item.line + 10)
		key, offset := m.selectedKey(), m.reader.YOffset()
		half := max(1, m.reader.Height()/2)
		m, _ = updateKey(m, 'd', tea.ModCtrl)
		if m.selectedKey() != key || m.reader.YOffset() != offset+half {
			t.Fatal("half-page skipped long content")
		}
		m, _ = updateKey(m, 'u', tea.ModCtrl)
		if m.selectedKey() != key || m.reader.YOffset() != offset {
			t.Fatal("half-page up lost long-content position")
		}
		next, _ := m.Update(tea.MouseWheelMsg{X: m.readerX() + 6, Y: m.readerTop() + 1, Button: tea.MouseWheelDown})
		m = next.(model)
		if m.selectedKey() != key || m.reader.YOffset() != offset+1 {
			t.Fatal("wheel skipped unread long content")
		}
	}
}

func TestHalfPageDoesNotPagePastUnselectedFloors(t *testing.T) {
	m, c := pagingModel(t, true, 1)
	// The entire short page fits onscreen, but cursor is still on its first floor.
	m, _ = updateKey(m, 'd', tea.ModCtrl)
	if m.busy || m.selectedKey() != "111" || len(c.calls) != 0 {
		t.Fatal("viewport boundary skipped unselected floor")
	}
}
