package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func centeredReaderModel(agent bool) model {
	m := newModel()
	m.chatStyle, m.reading, m.agentSeed = agent, true, 123
	m.current().title = ""
	m.current().posts = nil
	for i := 0; i < 20; i++ {
		m.current().posts = append(m.current().posts, post{id: 100 + i, body: fmt.Sprintf("正文 %d", i)})
	}
	m.resize(120, 36)
	m.reader.GotoTop()
	return m
}

func readerWheel(m model, button tea.MouseButton) model {
	n, _ := m.Update(tea.MouseWheelMsg{X: m.readerX() + 6, Y: m.readerTop() + 1, Button: button})
	return n.(model)
}

func TestWheelMovesOneFloorAndCentersBetweenThreadEdges(t *testing.T) {
	for _, agent := range []bool{false, true} {
		m := centeredReaderModel(agent)
		for i := 1; i < 20; i++ {
			m = readerWheel(m, tea.MouseWheelDown)
			if m.activePost != i {
				t.Fatalf("agent=%v: wheel skipped floor %d", agent, i)
			}
			item := selectedReaderItem(t, m)
			above, below := item.line-m.reader.YOffset(), m.reader.YOffset()+m.reader.Height()-item.end
			if above < 0 || below < 0 {
				t.Fatal("short reply clipped")
			}
			atTail := m.reader.YOffset()+m.reader.Height() >= m.readerItems[len(m.readerItems)-1].end
			if !m.reader.AtTop() && !atTail && (above-below > 1 || below-above > 1) {
				t.Fatal("interior reply not centered")
			}
		}
		if m.reader.YOffset()+m.reader.Height() != m.readerItems[len(m.readerItems)-1].end {
			t.Fatal("thread tail should clamp to bottom")
		}
		for i := 18; i >= 0; i-- {
			m = readerWheel(m, tea.MouseWheelUp)
			if m.activePost != i {
				t.Fatal("reverse wheel skipped a floor")
			}
		}
		if !m.reader.AtTop() {
			t.Fatal("thread head should clamp to top")
		}
	}
}

func TestWheelReadsEntireLongReplyBeforeChangingFloor(t *testing.T) {
	m := longReplyModel(60, 24, true, false)
	item := selectedReaderItem(t, m)
	for m.reader.YOffset()+m.reader.Height() < item.end {
		before := m.reader.YOffset()
		m = readerWheel(m, tea.MouseWheelDown)
		if m.selectedKey() != item.key || m.reader.YOffset() != before+1 {
			t.Fatal("wheel skipped unread long quote")
		}
	}
	m = readerWheel(m, tea.MouseWheelDown)
	if m.selectedKey() != "102" {
		t.Fatal("wheel could not leave completed quote")
	}
}

func TestReaderAnimationOnlyChangesDisplayAndRejectsOldFrames(t *testing.T) {
	m := centeredReaderModel(false)
	for m.readerScroll.pending == 0 {
		m = readerWheel(m, tea.MouseWheelDown)
	}
	s := m.readerScroll
	offset, key, request, stateTick, body := m.reader.YOffset(), m.selectedKey(), m.requestID, m.stateTickID, m.reader.GetContent()
	n, cmd := m.Update(readerScrollTick{s.pending, s.started.Add(30 * time.Millisecond)})
	m = n.(model)
	if cmd == nil || m.readerVisualOffset() <= s.from || m.readerVisualOffset() >= s.target {
		t.Fatal("missing intermediate animation frame")
	}
	if m.reader.YOffset() != offset || m.selectedKey() != key || m.requestID != request || m.stateTickID != stateTick || m.reader.GetContent() != body {
		t.Fatal("animation frame changed navigation, requests or content")
	}
	visible := m.readerVisualOffset()
	m = readerWheel(m, tea.MouseWheelDown)
	if m.readerScroll.from != visible {
		t.Fatal("rapid scrolling snapped to old target")
	}
	newID := m.readerScroll.pending
	n, cmd = m.Update(readerScrollTick{s.pending, s.started.Add(readerScrollDuration)})
	m = n.(model)
	if cmd != nil || m.readerScroll.pending != newID {
		t.Fatal("old frame affected newer animation")
	}
	n, _ = m.Update(readerScrollTick{newID, m.readerScroll.started.Add(readerScrollDuration)})
	m = n.(model)
	if m.readerScroll.pending != 0 || m.readerVisualOffset() != m.reader.YOffset() {
		t.Fatal("animation did not settle")
	}
}

func TestMouseSelectsVisibleFloorDuringAnimation(t *testing.T) {
	m := centeredReaderModel(false)
	for m.readerScroll.pending == 0 {
		m = readerWheel(m, tea.MouseWheelDown)
	}
	visual := m
	visual.reader.SetYOffset(m.readerVisualOffset())
	item, ok := visual.readerAt(m.readerTop())
	if !ok {
		t.Fatal("missing visible click target")
	}
	m, _ = clickAt(m, m.readerX()+6, m.readerTop(), tea.MouseLeft)
	if m.selectedKey() != item.key || m.readerScroll.pending != 0 {
		t.Fatal("click targeted the future animation position")
	}
}

func TestWheelPagesOnlyAfterSelectingLastFloor(t *testing.T) {
	m, backend := pagingModel(t, true, 1)
	// Both replies fit on screen: merely seeing the bottom must not skip
	// selecting the second reply and fetch the next page early.
	m = readerWheel(m, tea.MouseWheelDown)
	if m.selectedKey() != "111" || m.busy || len(backend.calls) != 0 {
		t.Fatal("wheel requested a page before selecting the last reply")
	}
	n, cmd := m.Update(tea.MouseWheelMsg{X: m.readerX() + 6, Y: m.readerTop() + 1, Button: tea.MouseWheelDown})
	m = n.(model)
	if !m.busy || cmd == nil {
		t.Fatal("wheel did not request the next page")
	}
	m = applyCommand(m, cmd)
	if m.selectedKey() != "120" || fmt.Sprint(backend.calls) != "[2]" {
		t.Fatal("wheel did not continue onto the next page")
	}
}

func TestReaderAnimationStopsOnNavigationAndResize(t *testing.T) {
	for _, msg := range []tea.Msg{tea.KeyPressMsg{Code: 'h', Text: "h"}, tea.KeyPressMsg{Code: tea.KeyF6}, tea.WindowSizeMsg{Width: 80, Height: 24}, tea.KeyPressMsg{Code: '?', Text: "?"}} {
		m := centeredReaderModel(false)
		for m.readerScroll.pending == 0 {
			m = readerWheel(m, tea.MouseWheelDown)
		}
		s := m.readerScroll
		n, _ := m.Update(msg)
		m = n.(model)
		if m.readerScroll.pending != 0 {
			t.Fatal("animation survived a navigation or layout change")
		}
		offset := m.reader.YOffset()
		n, cmd := m.Update(readerScrollTick{s.pending, s.started.Add(readerScrollDuration)})
		m = n.(model)
		if cmd != nil || m.reader.YOffset() != offset {
			t.Fatal("late frame moved the new view")
		}
	}
}
