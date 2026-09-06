package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/charmbracelet/x/ansi"
)

func TestMineButtonAndReplyRoundTrip(t *testing.T) {
	root := forum.Post{ID: 100, Body: "我的主串", BoardID: 1}
	reply := forum.Post{ID: 101, FollowID: 100, Body: "我的回复", BoardID: 1}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "test-cookie" {
			t.Error("personal content must use the selected identity and read-only requests")
		}
		var data any
		switch r.URL.Path {
		case "/forum/userList":
			requests++
			if r.URL.Query().Get("page") != "0" {
				t.Error("mine should open at page one")
			}
			data = map[string]any{"list": []forum.Post{root, reply}, "count": 2}
		case "/forum/get":
			data = root
			if r.URL.Query().Get("postId") == "101" {
				data = reply
			}
		case "/forum/postPage":
			data = map[string]int{"page": 1, "floor": 1}
		case "/forum/list":
			data = map[string]any{"list": []forum.Post{root, reply}, "count": 2}
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": data})
	}))
	defer server.Close()
	for _, width := range []int{120, 60, 44} {
		m := newModel()
		m.opts = Options{ForumURL: server.URL, UserURL: server.URL}
		m.resize(width, 24)
		m.identity = local.Cookie{Alias: "daily"}
		m.client, _ = forum.New(server.URL, server.URL, "test-cookie")
		m.filter = "旧筛选不应隐藏我的回复"
		view := m.View()
		if view.MouseMode != tea.MouseModeCellMotion || !strings.Contains(ansi.Strip(strings.Split(view.Content, "\n")[3]), "m 我的内容") {
			t.Fatal("mine button is not visible/clickable")
		}
		next, cmd := m.Update(tea.MouseClickMsg{X: m.mineButtonX() + 1, Y: 3, Button: tea.MouseLeft})
		m = applyCommand(next.(model), cmd)
		if m.kind != "mine" || m.filter != "" || len(m.visible) != 2 || !strings.Contains(ansi.Strip(m.View().Content), "我的内容 · daily") {
			t.Fatal("mine button did not open personal content")
		}
		m = press(m, "down")
		m, cmd = updateKey(m, tea.KeyEnter, 0)
		m = applyCommand(m, cmd)
		assertSelectedPost(t, m, 101)
		m = press(m, "esc")
		if m.reading || m.kind != "mine" || m.selected != 1 || m.current().id != 101 {
			t.Fatal("returning from a reply changed the personal list")
		}
		m, cmd = updateKey(m, 'm', 0)
		m = applyCommand(m, cmd)
		if m.kind != "mine" || m.page != 1 {
			t.Fatal("m shortcut failed")
		}
	}
	if requests != 6 {
		t.Fatalf("got %d personal list requests", requests)
	}
}

func TestMineGuestChoosesIdentity(t *testing.T) {
	m := newModel()
	m.opts = Options{ForumURL: "https://example.com/", UserURL: "https://example.com/"}
	store, err := local.New(t.TempDir(), m.opts.ForumURL, m.opts.UserURL, "file")
	if err != nil {
		t.Fatal(err)
	}
	m.store = store
	if err := store.Import("daily", "test-cookie", forum.User{ID: 1}); err != nil {
		t.Fatal(err)
	}
	m = press(m, "m")
	if m.modal != "menu" || !m.pendingMine || m.busy {
		t.Fatal("guest should choose identity before loading mine")
	}
	m = press(m, "enter") // Command is not executed; this test makes no network request.
	if m.identity.Alias != "daily" || m.kind != "mine" || !m.busy || m.pendingMine {
		t.Fatal("identity choice did not resume mine navigation")
	}
	if m.cancel != nil {
		m.cancel()
	}
}
