package tui

import "github.com/A-islander/islander-cli/internal/forum"

// A card has a title/body line, metadata and a gap. Only actual titles get
// an additional body excerpt; generated titles already contain that text.
func (m model) listItemHeight(index int) int {
	t := m.threads[m.visible[index]]
	title := t.title
	if p, ok := m.raw[t.id]; ok {
		title = forum.Clean(p.Title)
	}
	if title == "" {
		return 3
	}
	return 4
}

func (m model) listEnd(top int) int {
	end, used := top, 0
	for end < len(m.visible) {
		height := m.listItemHeight(end)
		if end > top && used+height > m.panelHeight()-4 {
			break
		}
		used += height
		end++
	}
	return end
}

func (m *model) ensureListVisible() {
	if len(m.visible) == 0 {
		m.listTop = 0
		return
	}
	m.listTop = max(0, min(m.listTop, m.selected))
	used := 0
	for i := m.listTop; i <= m.selected; i++ {
		used += m.listItemHeight(i)
	}
	for used > m.panelHeight()-4 && m.listTop < m.selected {
		used -= m.listItemHeight(m.listTop)
		m.listTop++
	}
	// Fill the available rows when resizing or reaching the end of the list.
	if m.listEnd(m.listTop) == len(m.visible) {
		for i := m.selected + 1; i < len(m.visible); i++ {
			used += m.listItemHeight(i)
		}
		for m.listTop > 0 && used+m.listItemHeight(m.listTop-1) <= m.panelHeight()-4 {
			m.listTop--
			used += m.listItemHeight(m.listTop)
		}
	}
}

func (m model) listPageStep(direction int) int {
	index := m.selected
	if direction < 0 {
		index--
	}
	used, count := 0, 0
	for index >= 0 && index < len(m.visible) {
		height := m.listItemHeight(index)
		if count > 0 && used+height > m.panelHeight()-4 {
			break
		}
		used += height
		count++
		index += direction
	}
	return direction * max(1, count)
}
