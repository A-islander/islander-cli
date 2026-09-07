package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/charmbracelet/x/ansi"
)

func TestSiteSwitchIsolatesIdentityCachesAndResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "userhash=x-cookie" {
			t.Error("old identity crossed site boundary")
		}
		switch r.URL.Path {
		case "/getForumList":
			fmt.Fprint(w, `[{"forums":[{"id":30,"name":"技术"}]}]`)
		case "/getTimelineList":
			fmt.Fprint(w, `[{"id":1,"max_page":20}]`)
		case "/timeline":
			fmt.Fprint(w, `[{"id":100,"user_hash":"x-reader","content":"X body"}]`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	is, _ := local.NewSite(dir, "islander", "https://custom.example/", "https://user.example/", "file")
	if err := is.Import("daily", "islander-cookie", forum.User{ID: 7, Name: "islander"}); err != nil {
		t.Fatal(err)
	}
	xs, _ := local.NewSite(dir, "x", server.URL+"/", "", "file")
	if err := xs.Import("daily", "x-cookie", forum.User{Key: "cookie", Name: "X"}); err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.opts = Options{Site: "islander", ForumURL: "https://custom.example/", UserURL: "https://user.example/", DataDir: dir, Backend: "file"}
	m.store = is
	if err := m.setIdentity(""); err != nil {
		t.Fatal(err)
	}
	m.siteConfigs = map[string]forum.Site{"x": {ID: "x", ForumURL: server.URL + "/"}}
	m.raw[100] = forum.Post{ID: 100, Body: "old islander body"}
	m.inlineQuotes["100"] = []inlineQuote{{post: post{id: 200}}}
	m.offsets[100] = 45
	m.requestID = 9
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	cmd := m.switchSite("x")
	if cmd == nil || ctx.Err() == nil || m.requestID <= 9 || len(m.raw) != 0 || len(m.inlineQuotes) != 0 || len(m.offsets) != 0 || m.identity.ID != 0 || m.identity.Name != "X" {
		t.Fatal("site switch retained old identity, caches or request")
	}
	next, _, _ := m.extendedUpdate(resultMsg{ID: 9, Kind: "list", Value: forum.Page{List: []forum.Post{{ID: 100, Body: "late result"}}}})
	m = next.(model)
	if len(m.raw) != 0 {
		t.Fatal("late result crossed site boundary")
	}
	m = applyCommand(m, cmd)
	if m.busy || m.raw[100].Body != "X body" || m.store.Scope != xs.Scope {
		t.Fatalf("new site not loaded: %s", m.notice)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "X 岛") || !strings.Contains(view, "g 切换站点") {
		t.Fatal("site selection is not visible")
	}
	cmd = m.switchSite("islander")
	if cmd == nil || m.opts.ForumURL != "https://custom.example/" || m.identity.ID != 7 || m.store.Scope != is.Scope {
		t.Fatal("returning lost custom endpoint or original identity")
	}
	if m.cancel != nil {
		m.cancel()
	} // Do not execute the custom-server read.
}

type pagedReader struct {
	forum.Backend
	requested int
}

func (c *pagedReader) Capabilities() forum.Capabilities { return forum.Capabilities{} }
func (c *pagedReader) List(_ context.Context, _ string, _, page int) (forum.Page, error) {
	c.requested = page
	return forum.Page{Page: page, Count: -1, Offset: -1, HasMore: page < 2, List: []forum.Post{{ID: page, Body: "page"}}}, nil
}
func TestUnknownTotalPaginationAndUnavailableActions(t *testing.T) {
	c := &pagedReader{}
	m := newModel()
	m.opts = Options{Site: "bog"}
	m.client = c
	m.applyPage(forum.Page{Page: 1, Count: -1, Offset: -1, HasMore: true, List: []forum.Post{{ID: 1, Body: "page"}}})
	next, cmd, _ := m.extendedUpdate(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = applyCommand(next.(model), cmd)
	if c.requested != 2 || m.page != 2 {
		t.Fatal("unknown total blocked next page")
	}
	_, cmd, _ = m.extendedUpdate(tea.KeyPressMsg{Code: ']', Text: "]"})
	if cmd != nil {
		t.Fatal("advanced beyond last page")
	}
	m.applyThread(threadResult{Root: forum.Post{ID: 1}, Page: forum.Page{Page: 2, Count: -1, Offset: -1, List: []forum.Post{{ID: 2, FollowID: 1, Body: "reply"}}}})
	if strings.Contains(ansi.Strip(m.reader.View()), "20 楼") {
		t.Fatal("invented fixed-size floor")
	}
	m.openPostActions()
	for _, item := range m.menu {
		if item.Value != "a" && item.Value != "v" {
			t.Fatal("unsupported action visible")
		}
	}
	m.modal = ""
	if m.beginCompose(true, true) != nil || m.modal == "compose" || m.modal == "menu" {
		t.Fatal("unsupported write opened composer or cookies")
	}
	if m.openMine() != nil || m.kind == "mine" {
		t.Fatal("unsupported personal content opened")
	}
}
