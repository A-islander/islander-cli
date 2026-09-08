package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/charmbracelet/x/ansi"
)

func readerItemHighlighted(m model, item readerItem) bool {
	lines := strings.Split(m.reader.GetContent(), "\n")
	canvas := lipgloss.NewCanvas(m.reader.Width(), 1).Compose(lipgloss.NewLayer(lines[item.line]))
	cell := canvas.CellAt(min(item.depth, 4)*2, 0)
	if cell.Style.Bg == nil {
		return false
	}
	r, g, b, _ := cell.Style.Bg.RGBA()
	wr, wg, wb, _ := lipgloss.Color(selectedBG).RGBA()
	return r == wr && g == wg && b == wb
}

func assertSelectedPost(t *testing.T, m model, id int) {
	t.Helper()
	p, ok := m.selectedPost()
	if !ok || p.id != id {
		t.Fatalf("wrong selected post: %+v", p)
	}
	selected := 0
	for _, item := range m.readerItems {
		if readerItemHighlighted(m, item) {
			selected++
			if item.key != m.selectedKey() {
				t.Fatal("highlight belongs to another post")
			}
		}
	}
	if selected != 1 {
		t.Fatalf("expected one highlighted post, got %d", selected)
	}
	if !strings.Contains(ansi.Strip(m.reader.View()), fmt.Sprintf("No.%d", id)) {
		t.Fatal("selected header is outside the viewport")
	}
}

func TestVisibleFloorSelection(t *testing.T) {
	for _, width := range []int{120, 60, 44} {
		m := newModel()
		m.resize(width, 24)
		// This test covers quick selection of posts that fit on one screen.
		m.current().title = ""
		for i := range m.current().posts {
			m.current().posts[i].body = "短回复"
			m.current().posts[i].attachment = ""
			m.current().posts[i].quote = 0
		}
		m = press(m, "enter")
		assertSelectedPost(t, m, m.current().posts[0].id)
		for _, key := range []string{"down", "j", "n"} {
			previous := m.activePost
			m = press(m, key)
			if m.activePost != previous+1 {
				t.Fatalf("%s did not select next floor", key)
			}
			assertSelectedPost(t, m, m.current().posts[m.activePost].id)
		}
		m = press(m, "up")
		assertSelectedPost(t, m, m.current().posts[2].id)
		m = press(m, "esc")
		for _, item := range m.readerItems {
			if readerItemHighlighted(m, item) {
				t.Fatal("reader kept highlight after returning to list")
			}
		}
		m = press(m, "enter")
		assertSelectedPost(t, m, m.current().posts[2].id)
	}
}

func TestScrollKeepsVisiblePostSelected(t *testing.T) {
	m := press(newModel(), "enter")
	m.current().posts[0].body = strings.Repeat("长正文 (ﾟДﾟ)\n", 80)
	m.refreshReader(false)
	m, _ = updateKey(m, tea.KeyPgDown, 0)
	if m.activePost != 0 || m.reader.YOffset() == 0 || !strings.Contains(ansi.Strip(m.reader.View()), "长正文") {
		t.Fatal("long selected post lost its visible gutter while scrolling")
	}
	for range 12 {
		m, _ = updateKey(m, tea.KeyPgDown, 0)
	}
	if m.activePost == 0 || !strings.Contains(ansi.Strip(m.reader.GetContent()), fmt.Sprintf("No.%d", m.current().posts[m.activePost].id)) {
		t.Fatal("scrolling did not update selection")
	}
}

func TestSelectedPostActions(t *testing.T) {
	for _, action := range []string{"R", "s", "a"} {
		for _, menu := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/menu=%t", action, menu), func(t *testing.T) {
				m := newModel()
				m.opts.Demo = false
				m.identity = local.Cookie{Alias: "daily", ID: 1}
				root := forum.Post{ID: 100, Body: "主楼", BoardID: 1}
				reply := forum.Post{ID: 101, FollowID: 100, Body: "选中这条回复", UserID: 1, MediaURL: `["https://example.com/reply.png"]`}
				m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, Count: 2, List: []forum.Post{root, reply}}})
				m = press(m, "down")
				assertSelectedPost(t, m, 101)
				if menu {
					m = press(m, "enter")
					if m.modal != "menu" || !strings.Contains(m.returnModal, "No.101") {
						t.Fatal("post menu has wrong target")
					}
					for i, item := range m.menu {
						if item.Value == action {
							m.menuIndex = i
						}
					}
					m = press(m, "enter")
				} else {
					m = press(m, action)
				}
				switch action {
				case "R":
					if m.modal != "compose" || m.draft.ThreadID != 100 || m.draft.Body != "No.101\n" {
						t.Fatal("quote did not target selected reply")
					}
				case "s":
					if m.modal != "confirm" || m.confirmID != 101 || m.confirmAction != "sage" {
						t.Fatal("SAGE did not confirm selected reply")
					}
				case "a":
					defer m.attachment.cancel()
					if m.modal != "attachment" || !m.attachmentDirect || len(m.menu) != 1 || m.menu[0].Value != "https://example.com/reply.png" {
						t.Fatal("attachments did not belong to selected reply")
					}
				}
			})
		}
	}
}

func TestReloadAndJumpRestoreSelection(t *testing.T) {
	m := newModel()
	m.opts.Demo = false
	root := forum.Post{ID: 100, Body: "主楼", BoardID: 1}
	r := threadResult{Root: root, Page: forum.Page{Page: 1, Count: 3, List: []forum.Post{
		root, {ID: 101, FollowID: 100, Body: "第一条回复"}, {ID: 102, FollowID: 100, Body: "第二条回复"},
	}}}
	m.applyThread(r)
	m = press(m, "down")
	m.loadThread(100, 1, 0) // Creating the command saves position; no request is run.
	m.applyThread(r)
	assertSelectedPost(t, m, 101)
	r.Target = 102
	m.applyThread(r)
	assertSelectedPost(t, m, 102)
}

func TestSelectedPostBackgroundCoversBodyAndPadding(t *testing.T) {
	m := press(newModel(), "enter")
	m.current().posts[0].body = "第一行 (ﾟДﾟ)\n\n" + strings.Repeat("长回复正文\n", 60)
	m.refreshReader(false)
	item := m.readerItems[0]
	canvas := lipgloss.NewCanvas(m.reader.Width(), item.end).Compose(lipgloss.NewLayer(m.reader.GetContent()))
	wantR, wantG, wantB, _ := lipgloss.Color(selectedBG).RGBA()
	for y := item.line; y < item.end; y++ {
		for x := 0; x < m.reader.Width(); x++ {
			cell := canvas.CellAt(x, y)
			if cell.Width == 0 {
				continue
			}
			if cell.Style.Bg == nil {
				t.Fatalf("selected post lost background at %d,%d", x, y)
			}
			r, g, b, _ := cell.Style.Bg.RGBA()
			if r != wantR || g != wantG || b != wantB {
				t.Fatalf("selected post has wrong background at %d,%d", x, y)
			}
		}
	}
	m.reader.SetYOffset(10)
	canvas = lipgloss.NewCanvas(m.reader.Width(), m.reader.Height()).Compose(lipgloss.NewLayer(m.reader.View()))
	for y := 0; y < m.reader.Height(); y++ {
		cell := canvas.CellAt(m.reader.Width()-1, y)
		if cell.Style.Bg == nil {
			t.Fatal("scrolling removed the body background")
		}
		r, g, b, _ := cell.Style.Bg.RGBA()
		if r != wantR || g != wantG || b != wantB {
			t.Fatal("scrolled body has a different background")
		}
	}
}
