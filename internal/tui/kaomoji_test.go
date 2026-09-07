package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/ansi"
)

func kaomojiModel(t *testing.T, site string) model {
	t.Helper()
	m, _ := persistenceModel(t, t.TempDir(), site)
	m.identity.Alias = "daily"
	m.draft = forum.Draft{Cookie: "daily", ThreadID: 100, Body: "左边右边"}
	m.editDraft()
	m.editor.SetCursorColumn(2)
	return m
}
func pressF3(m model) model {
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	return next.(model)
}

func TestKaomojiInsertsOriginalAtCursorAndAutosavesAcrossSites(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m := kaomojiModel(t, site)
			m = pressF3(m)
			if m.modal != "kaomoji" || m.editor.Focused() {
				t.Fatal("F3 did not open picker")
			}
			m = press(m, "right")
			m = press(m, "enter")
			want := "左边(´ﾟДﾟ`)右边"
			if site == "bog" {
				want = "左边[´ﾟДﾟ`]右边"
			}
			if m.modal != "compose" || !m.editor.Focused() || m.editor.Value() != want {
				t.Fatalf("insertion changed cursor or original face: %q", m.editor.Value())
			}
			m = press(m, "!")
			if m.editor.Value() != strings.Replace(want, "右边", "!右边", 1) {
				t.Fatal("typing did not resume after inserted face")
			}
			next, _ := m.Update(draftSaveTick{m.draftTickID})
			m = next.(model)
			drafts, err := m.store.Drafts("daily")
			if err != nil || len(drafts) != 1 || drafts[0].Body != m.editor.Value() {
				t.Fatal("inserted face not autosaved")
			}
			m = pressF3(m)
			m = press(m, "tab")
			if m.kaomojiSelected != 2 {
				t.Fatal("Tab did not move to next cell")
			}
			m = press(m, "j")
			chosen := "|-` )"
			if site == "bog" {
				chosen = "|-` ]"
			}
			m = press(m, "enter")
			if !strings.Contains(m.editor.Value(), chosen+"右边") {
				t.Fatal("category face not inserted at current cursor")
			}
		})
	}
}

func TestBOGKaomojiUsesWebCatalogVerbatim(t *testing.T) {
	m := kaomojiModel(t, "bog")
	m = pressF3(m)
	// The web catalog includes extra opening brackets, not a mechanical substitution.
	m.kaomojiSelected = 44
	if !strings.Contains(ansi.Strip(m.kaomojiContent(100)), "[[[　°д°]]]") {
		t.Fatal("BOG preview did not use web catalog")
	}
	m = press(m, "enter")
	if m.editor.Value() != "左边[[[　ﾟдﾟ]]]右边" {
		t.Fatalf("BOG inserted different text: %q", m.editor.Value())
	}
	x := kaomojiModel(t, "x")
	x = pressF3(x)
	x.kaomojiSelected = 44
	x = press(x, "enter")
	if x.editor.Value() != "左边(　ﾟдﾟ)))右边" {
		t.Fatal("BOG picker modified another site's catalog")
	}
}

func TestKaomojiGridNavigation(t *testing.T) {
	m := pressF3(kaomojiModel(t, "islander"))
	for _, step := range []struct {
		key   string
		index int
	}{
		{"left", 0}, {"up", 0}, {"right", 1}, {"down", 5},
		{"j", 9}, {"h", 8}, {"k", 4}, {"l", 5},
		{"tab", 6}, {"shift+tab", 5}, {"end", 98},
		{"right", 98}, {"down", 98}, {"up", 94}, {"home", 0},
	} {
		m = press(m, step.key)
		if m.kaomojiSelected != step.index {
			t.Fatalf("%s: got %d want %d", step.key, m.kaomojiSelected, step.index)
		}
	}
	lines := strings.Split(ansi.Strip(m.kaomojiContent(106)), "\n")
	for i, face := range kaomojiList[:8] {
		if !strings.Contains(lines[3+i/4], displayText(face)) {
			t.Fatalf("entry %d missing from four-column row: %s", i, lines[3+i/4])
		}
	}
}

func TestKaomojiCancelKeepsBodyCursorAndTitleFocus(t *testing.T) {
	m := kaomojiModel(t, "islander")
	m = pressF3(m)
	m = press(m, "j")
	m = press(m, "esc")
	m = press(m, "!")
	if m.editor.Value() != "左边!右边" {
		t.Fatal("cancel moved or changed body cursor")
	}
	m.draft.ID = "" // Start a separate new-thread draft, retaining the editor fixture.
	m.draft.ThreadID, m.draft.BoardID = 0, 1
	m.titleInput.SetValue("前后")
	m = press(m, "tab")
	m.titleInput.SetCursor(1)
	m = pressF3(m)
	m = press(m, "enter")
	if m.titleInput.Value() != "前(=ﾟωﾟ)=后" || !m.titleInput.Focused() || !m.editTitle {
		t.Fatalf("title insertion/focus lost: %s", m.titleInput.Value())
	}
	if m.editor.Value() != "左边!右边" {
		t.Fatal("title insertion changed body")
	}
}

func TestKaomojiNeverInsertsTruncatedFace(t *testing.T) {
	m := kaomojiModel(t, "x")
	m.editor.SetValue(strings.Repeat("a", 8190))
	m = pressF3(m)
	m = press(m, "enter")
	if m.modal != "kaomoji" || m.kaomojiError == "" || m.editor.Value() != strings.Repeat("a", 8190) {
		t.Fatal("near limit inserted partial face")
	}
	m = press(m, "esc")
	m.draft.ID = "" // Start a separate new-thread draft, retaining the editor fixture.
	m.draft.ThreadID, m.draft.BoardID = 0, 1
	m.titleInput.SetValue(strings.Repeat("a", 127))
	m = press(m, "tab")
	m = pressF3(m)
	m = press(m, "enter")
	if m.modal != "kaomoji" || m.kaomojiError == "" || len(m.titleInput.Value()) != 127 {
		t.Fatal("title received truncated face")
	}
}

func TestKaomojiPickerFitsSmallScreensAndShowsControls(t *testing.T) {
	for _, site := range []string{"islander", "bog"} {
		for _, size := range [][2]int{{120, 36}, {60, 24}, {44, 16}} {
			m := kaomojiModel(t, site)
			m.resize(size[0], size[1])
			if !strings.Contains(ansi.Strip(m.View().Content), "F3 颜文字") {
				t.Fatal("composer lacks discoverable F3 control")
			}
			m = pressF3(m)
			for i := range m.kaomojis() {
				m.kaomojiSelected = i
				view := m.View().Content
				for _, line := range strings.Split(view, "\n") {
					if ansi.StringWidthWc(line) > size[0] {
						t.Fatal("face pushed screen frame open")
					}
				}
				visible := ansi.Strip(view)
				if !strings.Contains(visible, "Enter 插入") || !strings.Contains(visible, "Esc 返回") || !strings.Contains(visible, "›") {
					t.Fatalf("picker controls or selection clipped at %v", size)
				}
				if !strings.Contains(visible, displayText(m.kaomojis()[i])) {
					t.Fatalf("selected face preview clipped at %v: %s", size, m.kaomojis()[i])
				}
			}
		}
	}
}
