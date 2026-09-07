package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/charmbracelet/x/ansi"
)

func assertSelectedPost(t *testing.T, m model, id int) {
	t.Helper()
	content := ansi.Strip(m.reader.GetContent())
	marker := fmt.Sprintf("› No.%d", id)
	if !strings.Contains(content, marker) || strings.Count(content, "› No.") != 1 {
		t.Fatalf("expected one selected post %s", marker)
	}
	if !strings.Contains(ansi.Strip(m.reader.View()), marker) {
		t.Fatalf("selected header %s is outside the viewport", marker)
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
		if strings.Contains(ansi.Strip(m.reader.GetContent()), "› No.") {
			t.Fatal("reader kept focus marker after returning to list")
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
	if m.activePost != 0 || m.reader.YOffset() == 0 || !strings.Contains(ansi.Strip(m.reader.View()), "│ 长正文") {
		t.Fatal("long selected post lost its visible gutter while scrolling")
	}
	for range 12 {
		m, _ = updateKey(m, tea.KeyPgDown, 0)
	}
	if m.activePost == 0 || !strings.Contains(ansi.Strip(m.reader.GetContent()), fmt.Sprintf("› No.%d", m.current().posts[m.activePost].id)) {
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
