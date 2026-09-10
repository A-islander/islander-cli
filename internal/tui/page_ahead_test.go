package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

func aheadModel(t *testing.T, reading, agent bool) (model, *paginationBackend) {
	t.Helper()
	m, c := pagingModel(t, reading, 1)
	for page := 1; page <= 3; page++ {
		p := c.pages[page]
		p.List = nil
		for i := 0; i < 20; i++ {
			p.List = append(p.List, forum.Post{ID: page*1000 + i, FollowID: 100, Body: fmt.Sprintf("内容 %d/%d", page, i)})
		}
		c.pages[page] = p
	}
	// Decorative command blocks must have stable sizes in navigation fixtures.
	m.chatStyle, m.agentSeed = agent, 123
	if reading {
		m.applyThread(threadResult{Root: *c.pages[1].Root, Page: c.pages[1]})
	} else {
		m.applyPage(c.pages[1])
	}
	m.resize(90, 24)
	return m, c
}
func aheadSelect(m *model, index int) {
	if m.reading {
		m.moveReaderItem(index - m.activePost)
	} else {
		m.moveSelection(index - m.selected)
	}
	m.syncPagingPosition()
}

func TestPageAheadJoinsAtMidpointWithoutMoving(t *testing.T) {
	for _, reading := range []bool{false, true} {
		for _, agent := range []bool{false, true} {
			t.Run(fmt.Sprintf("reader=%v/agent=%v", reading, agent), func(t *testing.T) {
				m, c := aheadModel(t, reading, agent)
				m = prefetchReady(m)
				aheadSelect(&m, 9)
				m.attachPageAhead()
				if m.activeWindow().last != 1 {
					t.Fatal("page joined before midpoint")
				}
				aheadSelect(&m, 10)
				before, readOffset, listOffset, generation := m.navigation(), m.reader.YOffset(), m.listOffset(), m.requestID
				n, cmd := m.Update(struct{}{})
				m = drainPageCommands(n.(model), cmd)
				if m.activeWindow().last != 2 || m.navigation() != before || m.reader.YOffset() != readOffset || m.listOffset() != listOffset || m.requestID != generation || m.busy {
					t.Fatal("midpoint join moved view or failed")
				}
				for range 5 {
					n, cmd = m.Update(struct{}{})
					m = drainPageCommands(n.(model), cmd)
				}
				if fmt.Sprint(c.calls) != "[2]" {
					t.Fatal("joining cascaded into more requests", c.calls)
				}
				// Walk across the old seam: all ordinary items stay centered.
				for i := 11; i <= 21; i++ {
					m, cmd = updateKey(m, 'j', 0)
					m = drainPageCommands(m, cmd)
					if reading {
						item := selectedReaderItem(t, m)
						want := item.line - (m.reader.Height()-(item.end-item.line))/2
						if item.end-item.line <= m.reader.Height() && (m.reader.YOffset() < want-1 || m.reader.YOffset() > want+1) {
							t.Fatal("reply lost center at page seam", i, m.reader.YOffset(), want)
						}
					} else {
						center := m.listRow(m.selected) + (m.listItemHeight(m.selected)-1)/2 - m.listOffset()
						if center != (m.panelHeight()-4)/2 {
							t.Fatal("list lost center at page seam", i, center)
						}
					}
				}
				if m.currentPage().Page != 2 || m.activeWindow().last != 2 || fmt.Sprint(c.calls) != "[2 3]" {
					t.Fatal("current page follows download instead of cursor", c.calls)
				}
				aheadSelect(&m, 30)
				n, cmd = m.Update(struct{}{})
				m = drainPageCommands(n.(model), cmd)
				if m.activeWindow().last != 3 || m.currentPage().Page != 2 || fmt.Sprint(c.calls) != "[2 3]" {
					t.Fatal("next midpoint failed")
				}
			})
		}
	}
}

func TestLatePageAheadPreservesAnimationAndLongQuote(t *testing.T) {
	for _, reading := range []bool{false, true} {
		m, _ := aheadModel(t, reading, false)
		fetch := m.preparePagePrefetch()
		aheadSelect(&m, 10)
		m, _ = updateKey(m, 'j', 0)
		readVisual, listVisual := m.readerVisualOffset(), m.listVisualOffset()
		readPending, listPending := m.readerScroll.pending, m.listScroll.pending
		before := m.navigation()
		m = drainPageCommands(m, fetch)
		if m.navigation() != before || m.readerVisualOffset() != readVisual || m.listVisualOffset() != listVisual || m.readerScroll.pending != readPending || m.listScroll.pending != listPending {
			t.Fatal("late completion interrupted animation")
		}
		if reading && !m.readerScrollValid() || !reading && !m.listScrollValid() {
			t.Fatal("animation invalidated by appended rows")
		}
		if reading {
			m, _ = func() (model, tea.Cmd) {
				n, c := m.Update(readerScrollTick{id: readPending, at: time.Now().Add(time.Second)})
				return n.(model), c
			}()
			m.inlineQuotes["1011"] = []inlineQuote{{post: post{id: 999, body: strings.Repeat("很长的引用\n", 80)}}}
			m.activeQuote = "1011/999"
			m.refreshReader(false)
			item := selectedReaderItem(t, m)
			m.reader.SetYOffset(item.line + 12)
			offset, key := m.reader.YOffset(), m.selectedKey()
			m, _ = updateKey(m, 'j', 0)
			if m.reader.YOffset() != offset+1 || m.selectedKey() != key {
				t.Fatal("joining skipped long quote")
			}
		}
	}
}

func TestPageAheadDefersInactiveOrFilteredViews(t *testing.T) {
	for _, state := range []string{"filter", "sort", "modal", "back"} {
		t.Run(state, func(t *testing.T) {
			m, c := aheadModel(t, false, false)
			fetch := m.preparePagePrefetch()
			aheadSelect(&m, 12)
			switch state {
			case "filter":
				m.filter = "内容"
			case "sort":
				m.newest = true
			case "modal":
				m.modal = "menu"
			case "back":
				aheadSelect(&m, 1)
			}
			before, offset := m.navigation(), m.listOffset()
			m = drainPageCommands(m, fetch)
			if m.navigation() != before || m.listOffset() != offset || m.activeWindow().last != 1 || fmt.Sprint(c.calls) != "[2]" {
				t.Fatal("background page changed inactive view")
			}
		})
	}
}
