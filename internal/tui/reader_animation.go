package tui

import (
	"math"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

const readerScrollDuration = 150 * time.Millisecond

var readerFrameSequence atomic.Uint64

type readerScrollState struct {
	pending                                  uint64
	started                                  time.Time
	from, target, visible                    int
	thread, generation, width, height, lines int
	key                                      string
	agent                                    bool
}

type readerScrollTick struct {
	id uint64
	at time.Time
}

func (m model) readerScrollValid() bool {
	s := m.readerScroll
	t := m.current()
	return s.pending != 0 && m.reading && !m.busy && m.modal == "" && t != nil &&
		t.id == s.thread && m.requestID == s.generation && m.selectedKey() == s.key &&
		m.width == s.width && m.height == s.height && m.chatStyle == s.agent &&
		m.reader.TotalLineCount() == s.lines && m.reader.YOffset() == s.target
}

func (m model) readerVisualOffset() int {
	if m.readerScrollValid() {
		return m.readerScroll.visible
	}
	return m.reader.YOffset()
}

func (m model) readerFloorInput(msg tea.Msg) bool {
	if m.busy || m.modal != "" {
		return false
	}
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		return m.reading && (v.String() == "j" || v.String() == "k" || v.String() == "up" || v.String() == "down")
	case tea.MouseWheelMsg:
		return v.Mod == 0 && m.paneAt(v.X, v.Y) == readerMousePane &&
			(v.Button == tea.MouseWheelUp || v.Button == tea.MouseWheelDown)
	}
	return false
}

func (m *model) startReaderScroll(from int) tea.Cmd {
	m.readerScroll = readerScrollState{}
	t := m.current()
	if !m.reading || m.busy || m.modal != "" || t == nil || m.width < 44 || m.height < 16 {
		return nil
	}
	target := m.reader.YOffset()
	if math.Abs(float64(target-from)) <= 1 {
		return nil
	}
	m.readerScroll = readerScrollState{
		pending: readerFrameSequence.Add(1), started: time.Now(), from: from, target: target, visible: from,
		thread: t.id, generation: m.requestID, width: m.width, height: m.height,
		lines: m.reader.TotalLineCount(), key: m.selectedKey(), agent: m.chatStyle,
	}
	return m.nextReaderFrame()
}

func (m model) nextReaderFrame() tea.Cmd {
	id := m.readerScroll.pending
	return tea.Tick(16*time.Millisecond, func(at time.Time) tea.Msg { return readerScrollTick{id, at} })
}

func (m *model) advanceReaderScroll(tick readerScrollTick) tea.Cmd {
	if tick.id == 0 || tick.id != m.readerScroll.pending {
		return nil
	}
	if !m.readerScrollValid() {
		m.readerScroll = readerScrollState{}
		return nil
	}
	s := &m.readerScroll
	p := min(1.0, max(0.0, float64(tick.at.Sub(s.started))/float64(readerScrollDuration)))
	// Ease out in terminal rows, with no reflow or API work on animation ticks.
	s.visible = s.from + int(math.Round(float64(s.target-s.from)*(1-math.Pow(1-p, 3))))
	if p >= 1 || s.visible == s.target {
		m.readerScroll = readerScrollState{}
		return nil
	}
	return m.nextReaderFrame()
}
