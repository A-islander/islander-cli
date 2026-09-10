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
	for end < len(m.visible) && used < m.panelHeight()-4+m.listInset {
		used += m.listItemHeight(end)
		end++
	}
	return end
}

func (m *model) ensureListVisible() {
	if len(m.visible) == 0 {
		m.listTop = 0
		m.listInset = 0
		return
	}
	m.selected = max(0, min(m.selected, len(m.visible)-1))
	// Center the selected card's content, excluding its trailing gap. Only
	// the list head/tail clamp the cursor away from the middle.
	center := m.listRow(m.selected) + (m.listItemHeight(m.selected)-1)/2
	m.setListOffset(center - max(1, m.panelHeight()-4)/2)
}

func (m model) listRow(index int) int {
	rows := 0
	for i := 0; i < min(index, len(m.visible)); i++ {
		rows += m.listItemHeight(i)
	}
	return rows
}

func (m model) listOffset() int { return m.listRow(m.listTop) + m.listInset }

func (m *model) setListOffset(offset int) {
	offset = max(0, min(offset, max(0, m.listRow(len(m.visible))-(m.panelHeight()-4))))
	m.listTop, m.listInset = 0, 0
	for m.listTop < len(m.visible) && offset >= m.listItemHeight(m.listTop) {
		offset -= m.listItemHeight(m.listTop)
		m.listTop++
	}
	m.listInset = offset
}

func (m model) listPageStep(direction int) int {
	return m.listRowStep(direction, m.panelHeight()-4)
}

func (m model) listRowStep(direction, rows int) int {
	index := m.selected
	if direction < 0 {
		index--
	}
	used, count := 0, 0
	for index >= 0 && index < len(m.visible) {
		height := m.listItemHeight(index)
		if count > 0 && used+height > rows {
			break
		}
		used += height
		count++
		index += direction
	}
	return direction * max(1, count)
}
