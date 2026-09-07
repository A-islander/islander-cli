package tui

import (
	"strings"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
)

func TestExternalQuoteReplyComposerAndPreview(t *testing.T) {
	for _, site := range []string{"x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m := newModel()
			m.opts = Options{Site: site, ForumURL: "https://fixture.test/"}
			m.store, _ = local.NewSite(t.TempDir(), site, m.opts.ForumURL, "", "file")
			m.identity = local.Cookie{Alias: "daily"}
			token := "cookie"
			if site == "bog" {
				token = "bog_master=master; bog_sel=shadow"
			}
			m.client, _ = forum.NewBackend(site, m.opts.ForumURL, "", token)
			root := forum.Post{ID: 100, Body: "main"}
			reply := forum.Post{ID: 101, FollowID: 100, Body: "reply"}
			m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, List: []forum.Post{root, reply}}})
			m.movePost(1)
			m = press(m, "enter")
			found := false
			for i, item := range m.menu {
				if item.Value == "s" || item.Value == "S" || item.Value == "x" || item.Value == "X" {
					t.Fatal("enabling new threads exposed unsupported management actions")
				}
				if item.Value == "R" {
					m.menuIndex = i
					found = true
				}
			}
			if !found {
				t.Fatal("external quote reply missing from post menu")
			}
			m = press(m, "enter")
			if m.modal != "compose" || m.draft.ThreadID != 100 || m.editor.Value() != forum.Quote(site, 101)+"\n" {
				t.Fatal("external quote or reply target wrong")
			}
			m.editor.SetValue(m.editor.Value() + "本地测试正文")
			m.preview()
			if m.modal != "publish" || !strings.Contains(m.popup.GetContent(), "站点：") || !strings.Contains(m.popup.GetContent(), "100") {
				t.Fatal("reply has no explicit preview")
			}
			drafts, err := m.store.Drafts("daily")
			if err != nil || len(drafts) != 1 || drafts[0].Body != forum.Quote(site, 101)+"\n本地测试正文" {
				t.Fatal("preview did not persist external reply")
			}
			m.modal = ""
			m.apiBoards = []forum.Board{{ID: 4, Name: "综合版"}}
			m.board = 1
			m.reading = false
			m = press(m, "c")
			if m.modal != "compose" || m.draft.ThreadID != 0 || m.draft.BoardID != 4 {
				t.Fatal("c did not open new-thread composer in selected board")
			}
			m.editor.SetValue("新串草稿")
			m.preview()
			if m.modal != "publish" || !strings.Contains(m.popup.GetContent(), "综合版") {
				t.Fatal("new-thread preview missing target board")
			}
			m.identity = local.Cookie{}
			m.beginCompose(true, false)
			if m.modal != "menu" {
				t.Fatal("anonymous external reply bypassed cookie selection")
			}
		})
	}
}
