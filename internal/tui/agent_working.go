package tui

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

const agentFrameInterval = 80 * time.Millisecond

var agentFrameSequence atomic.Uint64

type agentWorkingState struct {
	thread         int
	site, identity string
	started, now   time.Time
	pending        uint64
}
type agentWorkingTick struct {
	id uint64
	at time.Time
}

// Track the reading session in both layouts, but animate only while the
// simulated reader is visible. Animation ticks never enter API/persistence or
// image reconciliation paths.
func (m *model) syncAgentWorking(now time.Time) tea.Cmd {
	t := m.current()
	if !m.reading || t == nil {
		m.agentWorking = agentWorkingState{}
		return nil
	}
	s := &m.agentWorking
	if s.started.IsZero() || s.thread != t.id || s.site != m.opts.Site || s.identity != m.identity.Alias {
		*s = agentWorkingState{thread: t.id, site: m.opts.Site, identity: m.identity.Alias, started: now}
	}
	s.now = now
	if !m.agentWorkingVisible() {
		s.pending = 0
		return nil
	}
	if s.pending != 0 {
		return nil
	}
	s.pending = agentFrameSequence.Add(1)
	id := s.pending
	return tea.Tick(agentFrameInterval, func(at time.Time) tea.Msg { return agentWorkingTick{id, at} })
}
func (m model) agentWorkingVisible() bool {
	return m.chatStyle && m.reading && m.modal == "" && m.width >= 44 && m.height >= 16
}
func (m *model) advanceAgentWorking(tick agentWorkingTick) tea.Cmd {
	if tick.id == 0 || tick.id != m.agentWorking.pending {
		return nil
	}
	m.agentWorking.pending = 0
	return m.syncAgentWorking(tick.at)
}
func (m model) agentWorkingLine() string {
	elapsed := max(time.Duration(0), m.agentWorking.now.Sub(m.agentWorking.started))
	seconds := int(elapsed / time.Second)
	// A narrow white highlight travels left to right across the word, with a
	// softer shoulder on either side and a short dark gap between sweeps.
	word := "Working"
	center := int(elapsed/agentFrameInterval)%(len(word)+6) - 3
	var glow strings.Builder
	for i, c := range word {
		distance := i - center
		if distance < 0 {
			distance = -distance
		}
		shade := "#858585"
		switch distance {
		case 0:
			shade = "#ffffff"
		case 1:
			shade = "#dddddd"
		case 2:
			shade = "#aaaaaa"
		}
		glow.WriteString(ink(string(c), shade))
	}
	return ink("• ", muted) + glow.String() + ink(fmt.Sprintf(" (%dm %ds • esc to interrupt)", seconds/60, seconds%60), muted)
}
