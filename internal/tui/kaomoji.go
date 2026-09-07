package tui

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

const kaomojiColumns = 4

func (m model) kaomojis() []string {
	if m.opts.Site == "bog" {
		return bogKaomojiList
	}
	return kaomojiList
}

func (m *model) openKaomoji() tea.Cmd {
	if err := m.saveDraft(); err != nil {
		m.stateError = err.Error()
		return nil
	}
	m.modal, m.kaomojiError = "kaomoji", ""
	m.editor.Blur()
	m.titleInput.Blur()
	return nil
}

// Resume without SetValue: it would reset the user's cursor and selection.
func (m *model) closeKaomoji() tea.Cmd {
	m.modal = "compose"
	if m.editTitle {
		return m.titleInput.Focus()
	}
	return m.editor.Focus()
}

func (m *model) insertKaomoji() tea.Cmd {
	face := m.kaomojis()[m.kaomojiSelected]
	if m.editTitle {
		text := m.titleInput.Value()
		if len(text)+len(face) > 128 || (m.opts.Site == "bog" && len(utf16.Encode([]rune(text+face))) > 50) || (m.opts.Site == "x" && len(utf16.Encode([]rune(text+face))) > 100) {
			m.kaomojiError = "标题放不下这个颜文字了"
			return nil
		}
		focus := m.closeKaomoji()
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(tea.PasteMsg{Content: face})
		return tea.Batch(focus, cmd)
	}
	text, selected := m.editor.Value(), m.editor.SelectedText()
	if len(text)-len(selected)+len(face) > 8192 || (m.editor.CharLimit > 0 && utf8.RuneCountInString(text)-utf8.RuneCountInString(selected)+utf8.RuneCountInString(face) > m.editor.CharLimit) {
		m.kaomojiError = "正文放不下这个颜文字了"
		return nil
	}
	focus := m.closeKaomoji()
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.PasteMsg{Content: face})
	return tea.Batch(focus, cmd)
}

func (m *model) updateKaomoji(key string) tea.Cmd {
	switch key {
	case "esc", "f3":
		return m.closeKaomoji()
	case "ctrl+c":
		if err := m.saveDraft(); err != nil {
			m.kaomojiError = err.Error()
			return nil
		}
		return tea.Quit
	case "enter":
		return m.insertKaomoji()
	case "left", "h", "shift+tab":
		m.kaomojiSelected = max(0, m.kaomojiSelected-1)
	case "right", "l", "tab":
		m.kaomojiSelected = min(len(m.kaomojis())-1, m.kaomojiSelected+1)
	case "up", "k":
		m.kaomojiSelected = max(0, m.kaomojiSelected-kaomojiColumns)
	case "down", "j":
		m.kaomojiSelected = min(len(m.kaomojis())-1, m.kaomojiSelected+kaomojiColumns)
	case "home":
		m.kaomojiSelected = 0
	case "end":
		m.kaomojiSelected = len(m.kaomojis()) - 1
	}
	return nil
}

func (m model) kaomojiContent(width int) string {
	faces := m.kaomojis()
	lines := []string{strong(fmt.Sprintf("颜文字 (%d/%d)", m.kaomojiSelected+1, len(faces)), teal), ink("方向键/hjkl 选择 · Tab 下一个", muted), ""}
	capacity := max(1, m.height-14)
	if m.kaomojiError != "" {
		capacity = max(1, capacity-1)
	}
	start := max(0, m.kaomojiSelected/kaomojiColumns-capacity+1)
	cellWidth := max(1, (width-(kaomojiColumns-1))/kaomojiColumns)
	for r := start; r < min((len(faces)+kaomojiColumns-1)/kaomojiColumns, start+capacity); r++ {
		var cells []string
		for c := 0; c < kaomojiColumns; c++ {
			i := r*kaomojiColumns + c
			face, prefix := "", " "
			if i < len(faces) {
				face = faces[i]
			}
			if i == m.kaomojiSelected {
				prefix = "›"
			}
			cell := rectangle(clip(prefix+face, cellWidth), cellWidth, 1)
			if i == m.kaomojiSelected {
				cells = append(cells, strong(cell, teal))
			} else {
				cells = append(cells, ink(cell, foam))
			}
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	// A full-width preview keeps long faces readable in a narrow four-column grid.
	lines = append(lines, strong(clip(faces[m.kaomojiSelected], width), teal))
	if m.kaomojiError != "" {
		lines = append(lines, ink(clip(m.kaomojiError, width), sand))
	}
	lines = append(lines, "", ink("Enter 插入 · Esc 返回编辑", sand))
	return strings.Join(lines, "\n")
}
