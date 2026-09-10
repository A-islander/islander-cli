package tui

// Move roughly half a viewport from the selected item, keeping selection and
// centering coupled. Long replies consume the budget in lines rather than
// being skipped; entering a long reply stops at its readable edge.
func (m *model) moveReaderHalf(direction int) {
	remaining := max(1, m.reader.Height()/2)
	for remaining > 0 {
		key, offset, line := m.selectedKey(), m.reader.YOffset(), 0
		for _, item := range m.readerItems {
			if item.key == key {
				line = item.line
				break
			}
		}
		m.moveReaderItem(direction)
		if m.selectedKey() == key {
			distance := m.reader.YOffset() - offset
			if distance < 0 {
				distance = -distance
			}
			if distance == 0 {
				return
			}
			remaining -= distance
			continue
		}
		for _, item := range m.readerItems {
			if item.key == m.selectedKey() {
				distance := item.line - line
				if distance < 0 {
					distance = -distance
				}
				remaining -= max(1, distance)
				if item.end-item.line > m.reader.Height() {
					return
				}
				break
			}
		}
	}
}
