package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/ansi"
)

func TestUntitledPreviewKeepsBodyParagraphs(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m := newModel()
			m.opts = Options{Site: site}
			m.resize(120, 36)
			body := "第一段预览。\n\n第二段正文。\n" + strings.Repeat("后面的长正文。", 100)
			m.applyPage(forum.Page{Page: 1, Count: 1, Size: 20, List: []forum.Post{{ID: 1, Site: site, Body: body}}})
			preview := ansi.Strip(m.reader.GetContent())
			if !strings.Contains(preview, "第一段预览。") || !strings.Contains(preview, "第二段正文。") {
				t.Fatalf("preview hides body paragraphs behind a generated title: %q", preview)
			}
			content := ansi.Strip(m.threadContent(*m.current()))
			if strings.Count(content, "第一段预览。") != 1 {
				t.Fatal("untitled post repeats the entire body as a title")
			}
			if m.raw[1].Body != body {
				t.Fatal("preview modified stored content")
			}
		})
	}
}

func TestThreeSiteSideReaderLoadsCompletePage(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("preview must only read")
				}
				switch site {
				case "islander":
					if r.URL.Path != "/forum/list" {
						t.Errorf("unexpected preview request %s", r.URL.Path)
					}
					posts := []forum.Post{{ID: 100, Body: "主楼正文"}}
					for i := 1; i <= 7; i++ {
						posts = append(posts, forum.Post{ID: 100 + i, FollowID: 100, Body: fmt.Sprintf("第%d条回复\n完整正文第一行\n完整正文第二行\n完整正文第三行\n回复尾部%d", i, i)})
					}
					json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"count": 8, "list": posts}})
				case "x":
					if r.URL.Path != "/thread" {
						t.Errorf("unexpected preview request %s", r.URL.Path)
					}
					posts := []map[string]any{{"id": 9999999, "user_hash": "Tips", "content": "系统提示"}}
					for i := 1; i <= 7; i++ {
						posts = append(posts, map[string]any{"id": 100 + i, "content": fmt.Sprintf("第%d条回复\n完整正文第一行\n完整正文第二行\n完整正文第三行\n回复尾部%d", i, i), "user_hash": "reader"})
					}
					json.NewEncoder(w).Encode(map[string]any{"id": 100, "ReplyCount": 7, "content": "主楼正文", "Replies": posts})
				case "bog":
					if r.URL.Path != "/t/100/1" {
						t.Errorf("unexpected preview request %s", r.URL.Path)
					}
					fmt.Fprint(w, `<div class="item-list"><div class="item-main"><div class="item-pop">#100</div><div class="item-content">主楼正文</div>`)
					for i := 1; i <= 7; i++ {
						fmt.Fprintf(w, `<div class="item-reply"><div class="item-pop">#%d</div><div class="item-content">第%d条回复<br>完整正文第一行<br>完整正文第二行<br>完整正文第三行<br>回复尾部%d</div></div>`, 100+i, i, i)
					}
					fmt.Fprint(w, `</div></div><div class="pages"><ul class="page-main"><li><span>1</span></li></ul></div>`)
				}
			}))
			defer server.Close()
			userURL := ""
			if site == "islander" {
				userURL = server.URL
			}
			client, err := forum.NewBackend(site, server.URL, userURL, "")
			if err != nil {
				t.Fatal(err)
			}
			m := newModel()
			m.opts = Options{Site: site}
			m.client = client
			m.applyPage(forum.Page{Page: 1, List: []forum.Post{{ID: 100, Body: "主楼正文", ReplyCount: 7}}})
			cmd := m.prepareSelection()
			if cmd == nil || m.busy {
				t.Fatal("preview must automatically load without blocking selection")
			}
			next, fetch := m.Update(cmd())
			m = next.(model)
			if fetch == nil {
				t.Fatal("preview timer did not start reading")
			}
			m = applyCommand(m, fetch)
			view := ansi.Strip(m.reader.GetContent())
			for i := 1; i <= 7; i++ {
				if !strings.Contains(view, fmt.Sprintf("第%d条回复", i)) || !strings.Contains(view, fmt.Sprintf("回复尾部%d", i)) {
					t.Fatalf("reply %d was omitted or truncated: %s", i, view)
				}
			}
			if m.reading || len(m.pages) != 0 || m.selectedThreads[100].Page.HasMore {
				t.Fatal("preview replaced full reader state")
			}
			if cmd = m.prepareSelection(); cmd != nil {
				t.Fatal("cached preview fetched twice")
			}
			next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(model)
			if !m.reading || m.busy || cmd != nil || len(m.current().posts) != 8 {
				t.Fatal("enter did not reuse the loaded page immediately")
			}
			m.movePost(7)
			if !strings.Contains(ansi.Strip(m.reader.View()), "回复尾部7") {
				t.Fatal("the last reply cannot be read")
			}
		})
	}
}

func TestPreviewRejectsOldSelectionAndIdentity(t *testing.T) {
	m := newModel()
	m.opts = Options{Site: "x"}
	m.client, _ = forum.NewBackend("x", "https://example.org/", "", "")
	m.applyPage(forum.Page{Page: 1, List: []forum.Post{{ID: 100, Body: "first"}, {ID: 200, Body: "second"}}})
	cmd := m.prepareSelection()
	old := cmd().(selectionTick)
	ctx, cancel := context.WithCancel(context.Background())
	m.selectionCancel = cancel
	m.moveSelection(1)
	m.prepareSelection()
	if ctx.Err() == nil {
		t.Fatal("old selection request not cancelled")
	}
	bad := selectionResult{selectionTick: old, Page: forum.Page{List: []forum.Post{{ID: 101, Body: "wrong reply"}}}}
	next, _ := m.Update(bad)
	m = next.(model)
	if _, ok := m.selectedThreads[100]; ok {
		t.Fatal("stale selection accepted")
	}
	if strings.Contains(m.reader.GetContent(), "wrong reply") {
		t.Fatal("old reply rendered below new thread")
	}
	current := selectionTick{m.requestID, m.selectionID, m.selectionTarget}
	m.requestID++ // A site/identity/list change invalidates the old response.
	next, _ = m.Update(selectionResult{selectionTick: current, Page: bad.Page})
	m = next.(model)
	if len(m.selectedThreads) != 0 {
		t.Fatal("old identity result cached")
	}
}

func TestPreviewStartsAtBodyWithoutLosingReadingPosition(t *testing.T) {
	m := newModel()
	m.opts = Options{Site: "islander"}
	body := "预览应该从这里开始\n" + strings.Repeat("长正文\n", 60)
	m.applyPage(forum.Page{Page: 1, List: []forum.Post{{ID: 1, Body: body}, {ID: 2, Body: "另一串"}}})
	m.reading = true
	m.refreshReader(false)
	m.reader.SetYOffset(30)
	m.leaveReading()
	if m.reader.YOffset() != 0 || !strings.Contains(ansi.Strip(m.reader.View()), "预览应该从这里开始") {
		t.Fatal("list preview reused a deep reading offset")
	}
	m.moveSelection(1)
	m.moveSelection(-1)
	if m.offsets[1] != 30 {
		t.Fatal("browsing selectedThreads overwrote saved reading position")
	}
	m.reading = true
	m.refreshReader(true)
	if m.reader.YOffset() != 30 {
		t.Fatal("reopening thread lost saved reading position")
	}
}

func TestFocusRoundTripKeepsLoadedPages(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m, c := persistenceModel(t, t.TempDir(), site)
			m = drainMain(t, m, m.initialLoad())
			m = drainMain(t, m, m.loadThread(100, 42, 0))
			m.fullscreen = false
			m.resize(120, 36)
			m.movePost(1)
			m.reader.SetYOffset(m.reader.YOffset() + 3)
			offset, generation := m.reader.YOffset(), m.requestID
			c.reads = nil
			for _, keys := range [][2]rune{{'h', 'l'}, {tea.KeyLeft, tea.KeyRight}, {'h', tea.KeyEnter}, {tea.KeyTab, tea.KeyTab}} {
				m, _ = updateKey(m, keys[0], 0)
				if m.reading || m.busy || m.reader.YOffset() != offset || m.prepareSelection() != nil {
					t.Fatal("returning left discarded the reader or scheduled another request")
				}
				m, _ = updateKey(m, keys[1], 0)
				if !m.reading || m.busy || m.reader.YOffset() != offset || m.activePost != 1 || m.pages[100].Page != 42 || m.requestID != generation || len(c.reads) != 0 {
					t.Fatal("entering the loaded thread fetched or reset its position")
				}
			}
			// Explicit refresh still fetches the current page.
			m, cmd := updateKey(m, 'r', tea.ModCtrl)
			m = applyCommand(m, cmd)
			if len(c.reads) != 1 || c.reads[0] != 42 {
				t.Fatal("explicit refresh did not fetch the current page")
			}
		})
	}
}

func TestSideReaderRestoresBeforeFocusWithoutRefetch(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m, c := persistenceModel(t, t.TempDir(), site)
			m = drainMain(t, m, m.initialLoad())
			m = drainMain(t, m, m.loadThread(100, 42, 0))
			m.movePost(1)
			m.reader.SetYOffset(m.reader.YOffset() + 3)
			m.flushPersistence()
			m = drainMain(t, m, m.loadList(1))
			m.fullscreen = false
			m.resize(120, 36)
			c.reads = nil
			tick := m.prepareSelection()
			if tick == nil {
				t.Fatal("side reader did not request content")
			}
			next, fetch := m.Update(tick())
			m = applyCommand(next.(model), fetch)
			if m.reading || fmt.Sprint(c.reads) != "[42]" {
				t.Fatalf("did not load saved page in side reader: %v", c.reads)
			}
			m, _ = updateKey(m, tea.KeyRight, 0)
			if !m.reading || m.busy || m.pages[100].Page != 42 || m.activePost != 1 || fmt.Sprint(c.reads) != "[42]" {
				t.Fatal("entering prefetched saved page repeated the request or lost the anchor")
			}
			if !strings.Contains(ansi.Strip(m.reader.GetContent()), "No.111") || m.reader.YOffset() <= m.postLines[1] {
				t.Fatal("saved offset or selected highlight was lost")
			}
		})
	}
}

func TestFocusRoundTripRetainsAppendedPages(t *testing.T) {
	m, c := pagingModel(t, true, 1)
	m.resize(120, 36)
	m.applyPagination(paginationResult{page: c.pages[2], rootID: 100, reading: true, extend: true, direction: 1})
	m.movePost(3)
	m.syncPagingPosition()
	m.requestID++ // Loading another page used a new request generation.
	offset := m.reader.YOffset()
	for _, key := range []string{"h", "l"} {
		m = press(m, key)
	}
	if !m.reading || m.busy || len(m.current().posts) != 4 || m.activePost != 3 || m.reader.YOffset() != offset || len(m.threadWindow.pages) != 2 || m.pages[100].Page != 2 || len(c.calls) != 0 {
		t.Fatal("focus switch discarded appended pages or requested them again")
	}
}

func TestForumBodyDisplaysConsecutiveLinesWithoutBlankRows(t *testing.T) {
	m := newModel()
	m.opts.Demo = false
	body := "第一行\n\n\n  \n第二行\n\t\n  第三行"
	root := forum.Post{ID: 100, Body: body}
	m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, List: []forum.Post{root}}})
	lines := strings.Split(ansi.Strip(m.reader.GetContent()), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "第一行" {
			if i+2 >= len(lines) || strings.TrimSpace(lines[i+1]) != "第二行" || !strings.HasPrefix(lines[i+2], "  第三行") {
				t.Fatal("blank rows remain or text indentation was changed")
			}
			if m.raw[100].Body != body || m.current().posts[0].body != body {
				t.Fatal("display compaction changed the source post")
			}
			return
		}
	}
	t.Fatal("body missing")
}
