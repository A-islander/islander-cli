package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/ansi"
)

func TestFileBrowserNavigationAndAttachment(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "照片")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(child, "海边 (ﾟДﾟ).png")
	if err := os.WriteFile(file, []byte("attachment fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.modal = "compose"
	m.editor.SetValue("No.101\n正文 (ﾟДﾟ)")
	m.titleInput.SetValue("标题")
	m.filePicker.dir = dir
	m, cmd := updateKey(m, 'a', tea.ModCtrl)
	m = applyCommand(m, cmd)
	if m.modal != "filepicker" || len(m.filePicker.entries) != 1 || !m.filePicker.entries[0].dir {
		t.Fatal("file browser should open with directories and hide dotfiles")
	}
	m, cmd = updateKey(m, tea.KeyEnter, 0)
	m = applyCommand(m, cmd)
	if m.filePicker.dir != child || len(m.filePicker.entries) != 1 {
		t.Fatal("directory navigation failed")
	}
	for _, width := range []int{120, 60, 44} {
		m.resize(width, 24)
		for _, line := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidthWc(line) > width {
				t.Fatal("file browser overflowed with a kaomoji filename")
			}
		}
	}
	m = press(m, "enter")
	if m.modal != "compose" || len(m.draft.Files) != 1 || m.draft.Files[0] != file {
		t.Fatal("file was not added to draft")
	}
	if m.editor.Value() != "No.101\n正文 (ﾟДﾟ)" || m.titleInput.Value() != "标题" {
		t.Fatal("attachment browsing changed the draft")
	}
	m, cmd = updateKey(m, 'a', tea.ModCtrl)
	m = applyCommand(m, cmd)
	if m.filePicker.dir != child {
		t.Fatal("last directory was not remembered")
	}
	m = press(m, "enter")
	if m.modal != "filepicker" || m.filePicker.err == "" || len(m.draft.Files) != 1 {
		t.Fatal("duplicate attachment should be rejected")
	}
	m, cmd = updateKey(m, tea.KeyLeft, 0)
	m = applyCommand(m, cmd)
	m, cmd = updateKey(m, '.', 0)
	m = applyCommand(m, cmd)
	if len(m.filePicker.entries) != 2 {
		t.Fatal("hidden-file toggle failed")
	}
	m = press(m, ":")
	if m.modal != "attach" {
		t.Fatal("manual path entry is missing")
	}
	m, cmd = updateKey(m, tea.KeyEscape, 0)
	m = applyCommand(m, cmd)
	if m.modal != "filepicker" {
		t.Fatal("canceling manual path should return to browser")
	}
	m = press(m, "esc")
	if m.modal != "compose" || len(m.draft.Files) != 1 {
		t.Fatal("canceling browser lost draft")
	}
}

func TestFileBrowserErrorsAndStaleDirectory(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	m.modal = "filepicker"
	stale := m.browseDirectory(dir)
	cmd := m.browseDirectory(filepath.Join(dir, "missing"))
	m = applyCommand(m, stale)
	if !m.filePicker.loading {
		t.Fatal("stale directory result replaced current navigation")
	}
	m = applyCommand(m, cmd)
	if m.filePicker.err == "" || m.filePicker.loading {
		t.Fatal("directory error was not shown")
	}
	large := filepath.Join(dir, "large.bin")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(forum.MaxFile + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := m.addAttachment(large); err == nil || len(m.draft.Files) != 0 {
		t.Fatal("oversized attachment accepted")
	}
	if err := m.addAttachment(dir); err == nil {
		t.Fatal("directory accepted as attachment")
	}
}
