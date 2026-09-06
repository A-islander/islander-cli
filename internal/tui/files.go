package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

type fileEntry struct {
	name string
	dir  bool
}

type fileBrowser struct {
	dir             string
	entries         []fileEntry
	selected        int
	hidden, loading bool
	request         int
	err             string
}

type directoryMsg struct {
	request int
	entries []fileEntry
	err     error
}

func (m *model) browseDirectory(path string) tea.Cmd {
	p := &m.filePicker
	p.dir = filepath.Clean(path)
	p.entries, p.selected, p.err, p.loading = nil, 0, "", true
	p.request++
	id, hidden, dir := p.request, p.hidden, p.dir
	return func() tea.Msg {
		entries, err := os.ReadDir(dir)
		result := directoryMsg{request: id, err: err}
		if err != nil {
			return result
		}
		for _, entry := range entries {
			if !hidden && strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			isDir := entry.IsDir()
			if entry.Type()&os.ModeSymlink != 0 {
				if info, err := os.Stat(filepath.Join(dir, entry.Name())); err == nil {
					isDir = info.IsDir()
				}
			}
			result.entries = append(result.entries, fileEntry{entry.Name(), isDir})
		}
		sort.Slice(result.entries, func(i, j int) bool {
			a, b := result.entries[i], result.entries[j]
			if a.dir != b.dir {
				return a.dir
			}
			return a.name < b.name
		})
		return result
	}
}

func (m *model) openFileBrowser() tea.Cmd {
	m.modal = "filepicker"
	m.editor.Blur()
	m.titleInput.Blur()
	dir := m.filePicker.dir
	if dir == "" {
		dir, _ = os.Getwd()
		if dir == "" {
			dir, _ = os.UserHomeDir()
		}
	}
	return m.browseDirectory(dir)
}

func (m *model) addAttachment(path string) error {
	if strings.HasPrefix(path, "~/") {
		dir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(dir, path[2:])
	} else if !filepath.IsAbs(path) && m.filePicker.dir != "" {
		path = filepath.Join(m.filePicker.dir, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := forum.FileInfo(path); err != nil {
		return err
	}
	for _, existing := range m.draft.Files {
		if existing == path {
			return fmt.Errorf("此文件已在附件中")
		}
	}
	m.draft.Files = append(m.draft.Files, path)
	m.notice = "已添加附件：" + forum.Clean(filepath.Base(path))
	return nil
}

func (m *model) updateFileBrowser(key string) tea.Cmd {
	p := &m.filePicker
	switch key {
	case "esc", "q":
		return m.editDraft()
	case ":":
		return m.inputDialog("attach", "手动输入文件路径；Esc 返回文件浏览器")
	case "left", "h", "backspace":
		return m.browseDirectory(filepath.Dir(p.dir))
	case "~":
		dir, err := os.UserHomeDir()
		if err != nil {
			p.err = err.Error()
			return nil
		}
		return m.browseDirectory(dir)
	case ".":
		p.hidden = !p.hidden
		return m.browseDirectory(p.dir)
	}
	if p.loading {
		return nil
	}
	switch key {
	case "down", "j":
		p.selected = min(len(p.entries)-1, p.selected+1)
	case "up", "k":
		p.selected = max(0, p.selected-1)
	case "pgdown":
		p.selected = min(len(p.entries)-1, p.selected+m.fileBrowserRows())
	case "pgup":
		p.selected = max(0, p.selected-m.fileBrowserRows())
	case "enter", "right", "l":
		if p.selected < 0 || p.selected >= len(p.entries) {
			return nil
		}
		e := p.entries[p.selected]
		path := filepath.Join(p.dir, e.name)
		if e.dir {
			return m.browseDirectory(path)
		}
		if key != "enter" {
			return nil
		}
		if err := m.addAttachment(path); err != nil {
			p.err = err.Error()
			return nil
		}
		return m.editDraft()
	}
	return nil
}

func (m model) fileBrowserRows() int { return max(1, min(14, m.height-16)) }

func (m model) fileBrowserContent(width int) string {
	p := m.filePicker
	lines := []string{strong("选择附件", teal), ink(clip(forum.Clean(p.dir), width), muted), ""}
	rows := m.fileBrowserRows()
	start := max(0, p.selected-rows+1)
	for i := start; i < min(len(p.entries), start+rows); i++ {
		e := p.entries[i]
		label := e.name
		if e.dir {
			label += "/"
		}
		if i == p.selected {
			lines = append(lines, strong(clip("› "+forum.Clean(label), width), teal))
		} else {
			lines = append(lines, ink(clip("  "+forum.Clean(label), width), foam))
		}
	}
	if p.loading {
		lines = append(lines, ink("正在读取目录…", muted))
	} else if len(p.entries) == 0 && p.err == "" {
		lines = append(lines, ink("目录为空 · ← 返回上级", muted))
	}
	if p.err != "" {
		lines = append(lines, ink(clip(forum.Clean(p.err), width), sand))
	}
	lines = append(lines, "", ink("↑↓ 选择 · Enter 进入目录 / 添加文件", teal), ink("← 上级 · ~ 主目录 · . 隐藏文件", muted), ink(": 输入路径 · Esc 返回编辑", muted))
	return strings.Join(lines, "\n")
}
