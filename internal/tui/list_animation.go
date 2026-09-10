package tui

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

type listScrollState struct {
	pending                                   uint64
	started                                   time.Time
	from, target, visible                     int
	selected, generation, width, height, rows int
	agent                                     bool
}
type listScrollTick struct {
	id uint64
	at time.Time
}

func (m model) listScrollValid() bool {
	s := m.listScroll
	t := m.current()
	return s.pending != 0 && !m.reading && !m.busy && m.modal == "" && t != nil && t.id == s.selected &&
		m.requestID == s.generation && m.width == s.width && m.height == s.height && m.chatStyle == s.agent &&
		m.listRow(len(m.visible)) == s.rows && m.listOffset() == s.target
}
func (m model) listVisualOffset() int {
	if m.listScrollValid() {
		return m.listScroll.visible
	}
	return m.listOffset()
}
func (m model) listScrollInput(msg tea.Msg) bool {
	if m.busy || m.modal != "" {
		return false
	}
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		return !m.reading && (v.String() == "j" || v.String() == "k" || v.String() == "up" || v.String() == "down" || v.String() == "ctrl+u" || v.String() == "ctrl+d")
	case tea.MouseWheelMsg:
		return v.Mod == 0 && m.paneAt(v.X, v.Y) == listMousePane && (v.Button == tea.MouseWheelUp || v.Button == tea.MouseWheelDown)
	}
	return false
}
func (m *model) startListScroll(from int) tea.Cmd {
	m.listScroll = listScrollState{}
	t := m.current()
	if m.reading || m.busy || m.modal != "" || t == nil || m.width < 44 || m.height < 16 {
		return nil
	}
	target := m.listOffset()
	if abs := math.Abs(float64(target - from)); abs <= 1 {
		return nil
	}
	m.listScroll = listScrollState{pending: readerFrameSequence.Add(1), started: time.Now(), from: from, target: target, visible: from,
		selected: t.id, generation: m.requestID, width: m.width, height: m.height, rows: m.listRow(len(m.visible)), agent: m.chatStyle}
	return m.nextListFrame()
}
func (m model) nextListFrame() tea.Cmd {
	id := m.listScroll.pending
	return tea.Tick(16*time.Millisecond, func(at time.Time) tea.Msg { return listScrollTick{id, at} })
}
func (m *model) advanceListScroll(tick listScrollTick) tea.Cmd {
	if tick.id == 0 || tick.id != m.listScroll.pending {
		return nil
	}
	if !m.listScrollValid() {
		m.listScroll = listScrollState{}
		return nil
	}
	s := &m.listScroll
	p := min(1.0, max(0.0, float64(tick.at.Sub(s.started))/float64(readerScrollDuration)))
	s.visible = s.from + int(math.Round(float64(s.target-s.from)*(1-math.Pow(1-p, 3))))
	if p >= 1 || s.visible == s.target {
		m.listScroll = listScrollState{}
		return nil
	}
	return m.nextListFrame()
}
