package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
)

func updateKey(m model, code rune, mod tea.KeyMod) (model, tea.Cmd) {
	n, c := m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	return n.(model), c
}
func applyCommand(m model, c tea.Cmd) model {
	if c == nil {
		return m
	}
	n, _ := m.Update(c())
	return n.(model)
}
func TestLiveQuoteReplyPublishAndPinnedIdentity(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/forum/reply" {
			writes++
			if r.Header.Get("Authorization") != "token-a" {
				t.Error("identity drift")
			}
			var d struct {
				Value    string
				FollowID int
				ReplyArr []int
			}
			json.NewDecoder(r.Body).Decode(&d)
			if d.FollowID != 100 || len(d.ReplyArr) != 1 || d.ReplyArr[0] != 101 {
				t.Errorf("reply payload %+v", d)
			}
			w.Write([]byte(`{"code":200,"data":null}`))
			return
		}
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()
	store, _ := local.New(t.TempDir(), server.URL, server.URL, "file")
	store.Import("a", "token-a", forum.User{ID: 1, Name: "A"})
	store.Import("b", "token-b", forum.User{ID: 2, Name: "B"})
	store.Use("a")
	m := newModel()
	m.opts = Options{ForumURL: server.URL, UserURL: server.URL}
	m.store = store
	m.setIdentity("")
	m.apiBoards = []forum.Board{{ID: 1, Name: "综合"}}
	m.boardNames = []string{"全部", "综合"}
	m.applyThread(threadResult{Root: forum.Post{ID: 100, BoardID: 1, Body: "主楼"}, Page: forum.Page{Page: 1, Count: 2, List: []forum.Post{{ID: 100, BoardID: 1, Body: "主楼"}, {ID: 101, FollowID: 100, Body: "回复"}}}})
	m.activePost = 1
	m, _ = updateKey(m, 'R', 0)
	if m.modal != "compose" || m.draft.Body != "No.101\n" {
		t.Fatal("quote editor did not open")
	}
	m.editor.SetValue("No.101\n引用回复测试")
	m, _ = updateKey(m, 'p', tea.ModCtrl)
	if m.modal != "publish" || writes != 0 {
		t.Fatal("preview published")
	}
	// A concurrent CLI changing the global selection must not change this editor identity.
	store.Use("b")
	var cmd tea.Cmd
	m, cmd = updateKey(m, tea.KeyEnter, 0)
	m = applyCommand(m, cmd)
	if writes != 1 || m.draft.ID != "" {
		t.Fatal("publication failed")
	}
	drafts, _ := store.Drafts("a")
	if len(drafts) != 0 {
		t.Fatal("sent draft not cleared")
	}
}
func TestStaleResultsAndEditorResize(t *testing.T) {
	m := newModel()
	m.requestID = 3
	m.busy = true
	next, _, _ := m.extendedUpdate(resultMsg{ID: 2, Kind: "list", Value: forum.Page{List: []forum.Post{{ID: 99}}}})
	m = next.(model)
	if m.current().id == 99 {
		t.Fatal("stale request applied")
	}
	m.busy = false
	m.modal = "compose"
	m.editor.SetValue("中文内容\n第二行")
	nextModel, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = nextModel.(model)
	if m.width != 60 || m.editor.Value() != "中文内容\n第二行" {
		t.Fatal("resize lost editor state")
	}
}
func TestPageTwoDoesNotInventMainPost(t *testing.T) {
	m := newModel()
	m.opts.Demo = false
	m.applyThread(threadResult{Root: forum.Post{ID: 100, Body: "主楼"}, Page: forum.Page{Page: 2, Count: 22, List: []forum.Post{{ID: 120, FollowID: 100, Body: "第二页回复"}}}})
	if got := m.reader.GetContent(); contains(got, "主楼 ·") {
		t.Fatal("reply labeled as OP")
	}
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
