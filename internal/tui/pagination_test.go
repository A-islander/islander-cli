package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/ansi"
)

type paginationBackend struct {
	forum.Backend
	pages map[int]forum.Page
	calls []int
	fail  bool
}

func (*paginationBackend) Capabilities() forum.Capabilities {
	return forum.Capabilities{Publish: true, Reply: true}
}
func (c *paginationBackend) List(_ context.Context, _ string, _, page int) (forum.Page, error) {
	c.calls = append(c.calls, page)
	if c.fail {
		return forum.Page{}, errors.New("fixture failure")
	}
	p, ok := c.pages[page]
	if !ok {
		return forum.Page{Page: page, Count: -1}, nil
	}
	return p, nil
}
func pagingModel(t *testing.T, reading bool, start int) (model, *paginationBackend) {
	t.Helper()
	root := forum.Post{ID: 100, Body: "主楼", ReplyCount: 60}
	c := &paginationBackend{pages: map[int]forum.Page{}}
	for page := 1; page <= 3; page++ {
		posts := []forum.Post{}
		for i := 0; i < 2; i++ {
			id := 100 + page*10 + i
			follow := 0
			if reading {
				follow = 100
			}
			posts = append(posts, forum.Post{ID: id, FollowID: follow, Body: fmt.Sprintf("正文 %d", id)})
		}
		c.pages[page] = forum.Page{Page: page, Count: -1, Offset: -1, HasMore: page < 3, List: posts, Root: &root}
	}
	m := newModel()
	m.opts = Options{Site: "bog"}
	m.client = c
	m.resize(60, 24)
	if reading {
		m.applyThread(threadResult{Root: root, Page: c.pages[start]})
	} else {
		m.applyPage(c.pages[start])
	}
	return m, c
}
func pagingKey(m model, key string) (model, tea.Cmd) {
	msg := tea.KeyPressMsg{}
	switch key {
	case "j", "k", "n", "p", "P", "[", "]":
		msg.Code = []rune(key)[0]
		msg.Text = key
	case "enter":
		msg.Code = tea.KeyEnter
	case "space":
		msg.Code = tea.KeySpace
	case "pgdown":
		msg.Code = tea.KeyPgDown
	case "pgup":
		msg.Code = tea.KeyPgUp
	case "ctrl+home":
		msg.Code = tea.KeyHome
		msg.Mod = tea.ModCtrl
	case "ctrl+end":
		msg.Code = tea.KeyEnd
		msg.Mod = tea.ModCtrl
	case "ctrl+l":
		msg.Code = 'l'
		msg.Mod = tea.ModCtrl
	}
	n, cmd := m.Update(msg)
	return n.(model), cmd
}
func lastPagingItem(m *model) {
	if m.reading {
		m.movePost(len(m.current().posts))
		last := m.readerItems[len(m.readerItems)-1]
		m.reader.SetYOffset(max(last.line, last.end-m.reader.Height()))
	} else {
		m.moveSelection(len(m.visible))
	}
	m.syncPagingPosition()
}
func TestAutomaticPagingWaitsForLongReplyAndNestedQuote(t *testing.T) {
	for _, nested := range []bool{false, true} {
		m, c := pagingModel(t, true, 1)
		long := strings.Repeat("这一行也要读完\n", 50) + "最后一行"
		if nested {
			m.inlineQuotes["111"] = []inlineQuote{{post: post{id: 999, body: long}}}
			m.activePost = 1
			m.activeQuote = "111/999"
		} else {
			m.current().posts[1].body = long
			m.activePost = 1
		}
		m.refreshReader(false)
		item := selectedReaderItem(t, m)
		m.reader.SetYOffset(item.line)
		for i := 0; m.reader.YOffset()+m.reader.Height() < item.end; i++ {
			if i > 1000 {
				t.Fatal("stuck")
			}
			before := m.reader.YOffset()
			var cmd tea.Cmd
			m, cmd = pagingKey(m, "j")
			if cmd != nil || m.busy || m.selectedKey() != item.key || m.reader.YOffset() != before+1 {
				t.Fatal("auto paging skipped long content")
			}
		}
		m, cmd := pagingKey(m, "j")
		if !m.busy || cmd == nil {
			t.Fatal("did not load next page at end")
		}
		m = applyCommand(m, cmd)
		if fmt.Sprint(c.calls) != "[2]" || len(m.current().posts) != 4 || m.selectedKey() != "120" || m.navigation().ReadPage != 2 {
			t.Fatalf("wrong continuation: %v %s %+v", c.calls, m.selectedKey(), m.navigation())
		}
		m = press(m, "k")
		if m.selectedKey() != item.key || m.navigation().ReadPage != 1 {
			t.Fatal("back across loaded page seam lost quote or page")
		}
	}
}
func TestAutomaticListPagingDeduplicatesAndStopsAtLastPage(t *testing.T) {
	m, c := pagingModel(t, false, 1)
	p := c.pages[2]
	p.List = append([]forum.Post{c.pages[1].List[1]}, p.List...)
	c.pages[2] = p
	lastPagingItem(&m)
	m, cmd := pagingKey(m, "j")
	// Repeated input while a request is in flight cannot start a second request.
	m, second := pagingKey(m, "j")
	if second != nil {
		t.Fatal("duplicate in-flight page request")
	}
	m = applyCommand(m, cmd)
	if len(m.threads) != 4 || m.current().id != 120 || m.page != 2 {
		t.Fatalf("append/dedup failed: %d %+v", len(m.threads), m.navigation())
	}
	m = press(m, "k")
	if m.current().id != 111 || m.page != 1 {
		t.Fatal("cached previous page unavailable")
	}
	lastPagingItem(&m)
	m, cmd = pagingKey(m, "j")
	m = applyCommand(m, cmd)
	lastPagingItem(&m)
	m, cmd = pagingKey(m, "j")
	if cmd != nil || fmt.Sprint(c.calls) != "[2 3]" {
		t.Fatalf("loaded beyond final page: %v", c.calls)
	}
}
func TestPrependAfterJumpPreservesPositionAndMovesToPreviousTail(t *testing.T) {
	for _, reading := range []bool{false, true} {
		m, c := pagingModel(t, reading, 2)
		if reading {
			p := c.pages[1]
			p.List[1].Body = strings.Repeat("长回复\n", 60)
			c.pages[1] = p
		}
		m, cmd := pagingKey(m, "k")
		m = applyCommand(m, cmd)
		if fmt.Sprint(c.calls) != "[1]" {
			t.Fatal("previous page not loaded")
		}
		if reading {
			item := selectedReaderItem(t, m)
			if item.post.id != 111 || m.reader.YOffset()+m.reader.Height() < item.end || m.navigation().ReadPage != 1 {
				t.Fatal("previous long reply did not enter at tail")
			}
		} else if m.current().id != 111 || m.page != 1 {
			t.Fatal("previous list page not selected")
		}
	}
}
func TestPageInputBoundsUnknownTotalsAndLatest(t *testing.T) {
	m, c := pagingModel(t, true, 1)
	m = press(m, "P")
	if m.modal != "page-jump" || !strings.Contains(ansi.Strip(m.dialog()), "总页数未知") {
		t.Fatal("unknown total was invented")
	}
	for _, value := range []string{"", "0", "-1", "no", "1000001"} {
		m.input.SetValue(value)
		var cmd tea.Cmd
		m, cmd = pagingKey(m, "enter")
		if cmd != nil || m.modal != "page-jump" || m.pageJumpError == "" {
			t.Fatalf("accepted %q", value)
		}
	}
	m.input.SetValue("2")
	m, cmd := pagingKey(m, "enter")
	m = applyCommand(m, cmd)
	if m.navigation().ReadPage != 2 || len(m.current().posts) != 2 || m.reader.YOffset() != 0 {
		t.Fatal("numeric jump did not replace loaded range at page start")
	}
	m.opts.Site = "islander"
	p := m.pages[100]
	p.Size = 2
	p.Count = 6
	m.pages[100] = p
	m.threadWindow.pages[2] = p
	m = press(m, "P")
	m.input.SetValue("4")
	m, cmd = pagingKey(m, "enter")
	if cmd != nil || m.pageJumpError == "" {
		t.Fatal("jump exceeded known total")
	}
	m, cmd = pagingKey(m, "ctrl+l")
	m = applyCommand(m, cmd)
	if m.selectedKey() != "131" || m.navigation().ReadPage != 3 {
		t.Fatal("latest reply shortcut missed last reply")
	}
	if fmt.Sprint(c.calls) != "[2 3]" {
		t.Fatal("invalid input made requests")
	}
}
func TestPageFailureEmptyAndStaleResponsesRetainContent(t *testing.T) {
	for _, empty := range []bool{false, true} {
		m, c := pagingModel(t, false, 1)
		lastPagingItem(&m)
		if empty {
			c.pages[2] = forum.Page{Page: 2, Count: -1, HasMore: true}
		} else {
			c.fail = true
		}
		m, cmd := pagingKey(m, "j")
		m = applyCommand(m, cmd)
		if len(m.threads) != 2 || m.current().id != 111 || m.page != 1 {
			t.Fatal("failed/empty request replaced old page")
		}
		m, cmd = pagingKey(m, "j")
		if cmd != nil || len(c.calls) != 1 {
			t.Fatal("failure or empty page automatically retried")
		}
	}
	m, c := pagingModel(t, false, 1)
	lastPagingItem(&m)
	m, cmd := pagingKey(m, "j")
	m = press(m, "esc")
	m = applyCommand(m, cmd)
	if len(m.threads) != 2 || m.current().id != 111 {
		t.Fatal("cancelled response changed list")
	}
	c.fail = false
	m = press(m, "P")
	m.input.SetValue("99")
	m, cmd = pagingKey(m, "enter")
	m = applyCommand(m, cmd)
	if m.page != 1 || len(m.threads) != 2 || !strings.Contains(m.notice, "没有内容") {
		t.Fatal("empty jump destroyed existing content")
	}
}
func TestViewportAndMouseCanAutoPage(t *testing.T) {
	for _, key := range []string{"space", "pgdown", "wheel"} {
		m, c := pagingModel(t, true, 1)
		lastPagingItem(&m)
		var cmd tea.Cmd
		if key == "wheel" {
			n, next := m.Update(tea.MouseWheelMsg{X: 5, Y: 6, Button: tea.MouseWheelDown})
			m, cmd = n.(model), next
		} else {
			m, cmd = pagingKey(m, key)
		}
		m = applyCommand(m, cmd)
		if fmt.Sprint(c.calls) != "[2]" || len(m.current().posts) != 4 {
			t.Fatalf("%s did not continue", key)
		}
	}
}

func TestPageJumpControlsFitSmallScreens(t *testing.T) {
	for _, known := range []bool{false, true} {
		m, _ := pagingModel(t, true, 1)
		if known {
			p := m.pages[100]
			p.Count = 6
			p.Size = 2
			m.pages[100] = p
			m.threadWindow.pages[1] = p
		}
		m.resize(44, 16)
		m = press(m, "P")
		for _, invalid := range []bool{false, true} {
			if invalid {
				m.input.SetValue("0")
				m, _ = pagingKey(m, "enter")
			}
			view := ansi.Strip(m.dialog())
			for _, want := range []string{"跳转页码", "当前第 1 页", "Enter 跳转", "Esc 返回", "Ctrl+Home"} {
				if !strings.Contains(view, want) {
					t.Fatalf("missing %s in small dialog: %s", want, view)
				}
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidthWc(line) > 44 {
					t.Fatal("page dialog overflowed")
				}
			}
		}
	}
}

func TestPagingFloorsAndSavedPageFollowSelectedPost(t *testing.T) {
	m, c := pagingModel(t, true, 1)
	m.opts.Site = "islander"
	for number, p := range c.pages {
		p.Count = 6
		p.Size = 2
		p.Offset = (number - 1) * 2
		c.pages[number] = p
	}
	m.applyThread(threadResult{Root: *c.pages[1].Root, Page: c.pages[1]})
	lastPagingItem(&m)
	m, cmd := pagingKey(m, "j")
	m = applyCommand(m, cmd)
	if m.navigation().ReadPage != 2 {
		t.Fatal("saved page followed initial page")
	}
	text := ansi.Strip(m.threadContent(*m.current()))
	if !strings.Contains(text, "00 楼") || !strings.Contains(text, "03 楼") {
		t.Fatalf("wrong floor offsets across pages: %s", text)
	}
	m = press(m, "k")
	if m.navigation().ReadPage != 1 {
		t.Fatal("saved page followed last downloaded page")
	}
}
