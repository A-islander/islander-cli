package tui

import (
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
			preview := ansi.Strip(m.reader.View())
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

func TestThreeSitePreviewShowsFiveReplies(t *testing.T) {
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
					for i := 1; i <= 5; i++ {
						posts = append(posts, forum.Post{ID: 100 + i, FollowID: 100, Body: fmt.Sprintf("第%d条回复", i)})
					}
					json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"count": 6, "list": posts}})
				case "x":
					if r.URL.Path != "/thread" {
						t.Errorf("unexpected preview request %s", r.URL.Path)
					}
					posts := []map[string]any{{"id": 9999999, "user_hash": "Tips", "content": "系统提示"}}
					for i := 1; i <= 5; i++ {
						posts = append(posts, map[string]any{"id": 100 + i, "content": fmt.Sprintf("第%d条回复", i), "user_hash": "reader"})
					}
					json.NewEncoder(w).Encode(map[string]any{"id": 100, "ReplyCount": 5, "content": "主楼正文", "Replies": posts})
				case "bog":
					if r.URL.Path != "/t/100/1" {
						t.Errorf("unexpected preview request %s", r.URL.Path)
					}
					fmt.Fprint(w, `<div class="item-list"><div class="item-main"><div class="item-pop">#100</div><div class="item-content">主楼正文</div>`)
					for i := 1; i <= 5; i++ {
						fmt.Fprintf(w, `<div class="item-reply"><div class="item-pop">#%d</div><div class="item-content">第%d条回复</div></div>`, 100+i, i)
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
			m.applyPage(forum.Page{Page: 1, List: []forum.Post{{ID: 100, Body: "主楼正文", ReplyCount: 5}}})
			cmd := m.preparePreview()
			if cmd == nil || m.busy {
				t.Fatal("preview must automatically load without blocking selection")
			}
			next, fetch := m.Update(cmd())
			m = next.(model)
			if fetch == nil {
				t.Fatal("preview timer did not start reading")
			}
			m = applyCommand(m, fetch)
			view := ansi.Strip(m.reader.View())
			for i := 1; i <= 5; i++ {
				if !strings.Contains(view, fmt.Sprintf("第%d条回复", i)) {
					t.Fatalf("reply %d missing from preview: %s", i, view)
				}
			}
			if m.reading || len(m.pages) != 0 || m.previews[100].Page.HasMore {
				t.Fatal("preview replaced full reader state")
			}
			if cmd = m.preparePreview(); cmd != nil {
				t.Fatal("cached preview fetched twice")
			}
		})
	}
}

func TestPreviewRejectsOldSelectionAndIdentity(t *testing.T) {
	m := newModel()
	m.opts = Options{Site: "x"}
	m.client, _ = forum.NewBackend("x", "https://example.org/", "", "")
	m.applyPage(forum.Page{Page: 1, List: []forum.Post{{ID: 100, Body: "first"}, {ID: 200, Body: "second"}}})
	cmd := m.preparePreview()
	old := cmd().(previewTick)
	ctx, cancel := context.WithCancel(context.Background())
	m.previewCancel = cancel
	m.moveSelection(1)
	m.preparePreview()
	if ctx.Err() == nil {
		t.Fatal("old selection request not cancelled")
	}
	bad := previewResult{previewTick: old, Page: forum.Page{List: []forum.Post{{ID: 101, Body: "wrong reply"}}}}
	next, _ := m.Update(bad)
	m = next.(model)
	if _, ok := m.previews[100]; ok {
		t.Fatal("stale selection accepted")
	}
	if strings.Contains(m.reader.GetContent(), "wrong reply") {
		t.Fatal("old reply rendered below new thread")
	}
	current := previewTick{m.requestID, m.previewID, m.previewTarget}
	m.requestID++ // A site/identity/list change invalidates the old response.
	next, _ = m.Update(previewResult{previewTick: current, Page: bad.Page})
	m = next.(model)
	if len(m.previews) != 0 {
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
		t.Fatal("browsing previews overwrote saved reading position")
	}
	m.reading = true
	m.refreshReader(true)
	if m.reader.YOffset() != 30 {
		t.Fatal("reopening thread lost saved reading position")
	}
}
