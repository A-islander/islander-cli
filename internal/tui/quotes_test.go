package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/charmbracelet/x/ansi"
)

func TestNestedInlineQuotesPreservePosition(t *testing.T) {
	m := press(newModel(), "enter")
	m.movePost(3)
	m.reader.SetYOffset(m.reader.YOffset() + 2)
	offset := m.reader.YOffset()
	rootItems := len(m.readerItems)
	m = press(m, "v")
	if m.modal != "" || m.reader.YOffset() != offset || len(m.readerItems) != rootItems+1 {
		t.Fatal("expansion should stay inline at the current position")
	}
	m = press(m, "down")
	assertSelectedPost(t, m, 10431)
	m = press(m, "v")
	nestedOffset := m.reader.YOffset()
	m = press(m, "down")
	assertSelectedPost(t, m, 10428)
	if m.activeQuote != "10433/10431/10428" {
		t.Fatal("nested quote path lost")
	}
	for _, width := range []int{120, 60, 44} {
		m.resize(width, 24)
		for _, line := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidthWc(line) > width {
				t.Fatal("nested quote overflowed terminal")
			}
		}
	}
	m.resize(120, 36)
	m = press(m, "esc")
	if m.activeQuote != "10433/10431" || m.reader.YOffset() != nestedOffset {
		t.Fatal("return to nested parent lost position")
	}
	m = press(m, "esc")
	if m.activeQuote != "" || m.activePost != 3 || m.reader.YOffset() != offset || !m.reading {
		t.Fatal("return to original post lost position")
	}
	m = press(m, "v")
	if len(m.readerItems) != rootItems || len(m.inlineQuotes) != 0 {
		t.Fatal("collapsing parent left orphaned nested quotes")
	}
	if m.reader.YOffset() != offset {
		t.Fatal("collapse moved reading position")
	}
}

func TestInlineQuotesFetchMultipleCyclesAndActions(t *testing.T) {
	posts := map[int]forum.Post{
		200: {ID: 200, Body: "第一条引用 (ﾟДﾟ)", Quotes: []int{300}},
		201: {ID: 201, Body: "另一条引用"},
		300: {ID: 300, Body: "嵌套引用", Quotes: []int{200}, MediaURL: `["https://example.com/nested.png"]`},
	}
	reads := map[int]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/forum/get" || r.Header.Get("Authorization") != "test-cookie" {
			t.Error("unexpected quote request")
		}
		id, _ := strconv.Atoi(r.URL.Query().Get("postId"))
		reads[id]++
		p, ok := posts[id]
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"code": 404, "msg": "帖子不存在"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": p})
	}))
	defer server.Close()
	m := newModel()
	m.opts.Demo = false
	m.identity = local.Cookie{Alias: "daily"}
	m.client, _ = forum.New(server.URL, server.URL, "test-cookie")
	root := forum.Post{ID: 100, Body: "原串", Quotes: []int{200, 201, 999, 200}}
	m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, Count: 1, List: []forum.Post{root}}})
	m = applyCommand(m, m.toggleInlineQuotes())
	if m.modal != "" || len(m.inlineQuotes["100"]) != 3 || len(m.readerItems) != 4 {
		t.Fatal("multiple references should expand inline and deduplicate IDs")
	}
	if !strings.Contains(ansi.Strip(m.reader.GetContent()), "无法读取引用") || m.inlineQuotes["100"][2].post.id != 999 {
		t.Fatal("failed quote is not shown inline")
	}
	m = press(m, "down")
	assertSelectedPost(t, m, 200)
	m = applyCommand(m, m.toggleInlineQuotes())
	m = press(m, "down")
	assertSelectedPost(t, m, 300)
	m = press(m, "R")
	if m.modal != "compose" || m.draft.Body != "No.300\n" || m.draft.ThreadID != 100 {
		t.Fatal("quoted reply did not target the selected nested quote")
	}
	m.modal = ""
	m = press(m, "s")
	if m.modal != "confirm" || m.confirmID != 300 {
		t.Fatal("SAGE targets the wrong nested post")
	}
	m.modal = ""
	m = press(m, "a")
	defer m.attachment.cancel()
	if m.modal != "attachment" || !m.attachmentDirect || len(m.menu) != 1 || m.menu[0].Value != "https://example.com/nested.png" {
		t.Fatal("attachments target the wrong nested post")
	}
	m.modal = ""
	m = applyCommand(m, m.toggleInlineQuotes())
	m = press(m, "down")
	assertSelectedPost(t, m, 200)
	count := len(m.readerItems)
	m = applyCommand(m, m.toggleInlineQuotes())
	if len(m.readerItems) != count || !strings.Contains(m.notice, "循环引用") {
		t.Fatal("cyclic reference should stop expanding")
	}
	if reads[200] != 1 || reads[201] != 1 || reads[300] != 1 || reads[999] != 1 {
		t.Fatalf("unexpected duplicate requests: %v", reads)
	}
	// A stale response must not add content to the current reader.
	next, _, _ := m.extendedUpdate(resultMsg{ID: m.requestID - 1, Kind: "inline-quote", Value: inlineQuoteResult{parent: "stale"}})
	m = next.(model)
	if _, ok := m.inlineQuotes["stale"]; ok {
		t.Fatal("stale quote response was applied")
	}
}

func TestFloorNavigationSkipsInlineQuotes(t *testing.T) {
	m := press(newModel(), "enter")
	m.movePost(3)
	m = press(m, "v")
	m = press(m, "down")
	if m.activeQuote == "" {
		t.Fatal("down did not select inline quote")
	}
	m = press(m, "n")
	if m.activePost != 4 || m.activeQuote != "" {
		t.Fatal("n should select the next original floor")
	}
	assertSelectedPost(t, m, m.current().posts[4].id)
}
