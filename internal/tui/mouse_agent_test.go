package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/charmbracelet/x/ansi"
)

func clickAt(m model, x, y int, button tea.MouseButton) (model, tea.Cmd) {
	n, c := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: button})
	return n.(model), c
}

func TestBracketBoardNavigation(t *testing.T) {
	for _, agent := range []bool{false, true} {
		m, _ := persistenceModel(t, t.TempDir(), "x")
		m.chatStyle = agent
		m.boardNames = []string{"全部", "技术", "日常"}
		m.apiBoards = []forum.Board{{ID: 10, Name: "技术"}, {ID: 20, Name: "日常"}}
		m.reading, m.filter, m.page = true, "旧筛选", 3
		for _, step := range []struct {
			key   rune
			board int
		}{{']', 1}, {']', 2}, {']', 2}, {'[', 1}, {'[', 0}, {'[', 0}} {
			m.busy = false
			m, _ = updateKey(m, step.key, tea.ModCtrl)
			if m.board != step.board || m.reading || m.filter != "" || m.chatStyle != agent {
				t.Fatal("board shortcut did not preserve layout and reset navigation")
			}
			if (m.board == 0) != (m.kind == "timeline") {
				t.Fatal("board shortcut selected the wrong list type")
			}
		}
		m.busy = false
		m.modal = "menu"
		m, _ = updateKey(m, ']', tea.ModCtrl)
		if m.board != 0 || m.modal != "menu" {
			t.Fatal("board shortcut escaped a modal")
		}
	}
}

func TestMouseSelectMixedCardsAndTitleOpen(t *testing.T) {
	m := newModel()
	// Use real card text to derive pointer coordinates, including the three-row
	// untitled card before the four-row titled one.
	m.opts = Options{Site: "x", Images: "off"}
	m.applyPage(forum.Page{Page: 1, List: []forum.Post{{ID: 100, Body: "短卡片"}, {ID: 200, Title: "目标卡片", Body: "第二张正文"}}})
	m.selectedThreads = map[int]selectionResult{200: {Page: forum.Page{Page: 1, List: []forum.Post{{ID: 200, Title: "目标卡片", Body: "第二张正文"}}}}}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	y := -1
	for i, line := range lines {
		if strings.Contains(line, "目标卡片") {
			y = i
			break
		}
	}
	if y < 0 {
		t.Fatal("target card missing")
	}
	m, _ = clickAt(m, 6, y+1, tea.MouseLeft)
	if m.selected != 1 || m.reading {
		t.Fatal("single click should select the card")
	}
	m, _ = clickAt(m, 6, y, tea.MouseLeft)
	if !m.reading || m.current().id != 200 || !m.hasLoadedThread() || m.busy {
		t.Fatal("title click should enter the cached thread")
	}
}

func TestAgentListModeAndCardClicks(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {80, 24}} {
		for _, target := range []struct {
			name    string
			id, row int
		}{
			{"title", 100, 0}, {"read annotation", 100, 1}, {"excerpt", 100, 2},
			{"untitled", 200, 4}, {"untitled annotation", 200, 5},
		} {
			t.Run(fmt.Sprintf("%v/%s", size, target.name), func(t *testing.T) {
				m := imagePreviewFixture("x", "off")
				m.selectedThreads[200] = selectionResult{Page: forum.Page{Page: 1, List: []forum.Post{m.raw[200]}}}
				m.resize(size[0], size[1])
				for i := 0; i < 3; i++ {
					m, _ = updateKey(m, tea.KeyF6, 0)
					if m.reading || m.selected != 0 || m.busy {
						t.Fatal("layout toggle entered reading or changed selected thread")
					}
				}
				lines := strings.Split(ansi.Strip(m.View().Content), "\n")
				y := -1
				for i, line := range lines {
					if strings.Contains(line, "图片预览") {
						y = i
						break
					}
				}
				if y < 0 {
					t.Fatal("agent list missing")
				}
				m, _ = clickAt(m, 6, y+3, tea.MouseLeft)
				if m.reading {
					t.Fatal("card gap opened a thread")
				}
				m, _ = clickAt(m, 6, y+target.row, tea.MouseLeft)
				if !m.reading || m.current().id != target.id || !m.hasLoadedThread() || m.busy {
					t.Fatal("agent card click did not open cached target")
				}
				m, _ = updateKey(m, 'h', 0)
				if m.reading || !strings.Contains(ansi.Strip(m.View().Content), "Read local context") {
					t.Fatal("return did not restore agent list")
				}
			})
		}
	}
}

func TestMouseReaderSelectionMenuAndWheelFollowPointer(t *testing.T) {
	m := imagePreviewFixture("x", "off")
	item := m.readerItems[1]
	m, _ = clickAt(m, m.readerX()+5, browseTop+2+item.line, tea.MouseLeft)
	if !m.reading || m.selectedKey() != "101" || m.busy {
		t.Fatal("right pane click did not select cached reply")
	}
	// A right click opens operations for the hit reply, not the previous cursor.
	item = m.readerItems[2]
	m, _ = clickAt(m, m.readerX()+5, browseTop+2+item.line, tea.MouseRight)
	if m.modal != "menu" || !strings.Contains(m.returnModal, "102") {
		t.Fatal("context menu targets wrong reply")
	}
	m.modal = ""
	n, _ := m.Update(tea.MouseWheelMsg{X: 6, Y: 8, Button: tea.MouseWheelDown})
	m = n.(model)
	if m.reading || m.selected != 1 {
		t.Fatal("wheel over list did not return focus and select next card")
	}
}

func TestAgentBackButtonPreservesNavigation(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {80, 24}, {44, 16}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := imagePreviewFixture("x", "off")
			m.toggleChatStyle()
			m.resize(size[0], size[1])
			if strings.Contains(ansi.Strip(m.View().Content), "exit") {
				t.Fatal("list should not show a back button")
			}
			m.focusMouseReader()
			m.moveReaderItem(2)
			m.reader.SetYOffset(m.readerItems[2].line)
			selected, top, offset, key := m.selected, m.listTop, m.reader.YOffset(), m.selectedKey()
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			label := "exit"
			index := strings.Index(lines[1], label)
			if index < 0 || !strings.Contains(lines[1][:index], ">_ Codex") {
				t.Fatal("back button missing to the right of Codex")
			}
			x := ansi.StringWidth(lines[1][:index])
			if x != size[0]-2-ansi.StringWidth(label) {
				t.Fatal("exit should align with the right header edge")
			}
			m.openPostActions()
			m, _ = clickAt(m, x, 1, tea.MouseLeft)
			if !m.reading || m.modal != "" {
				t.Fatal("menu dismissal clicked through to back button")
			}
			m, _ = clickAt(m, x, 1, tea.MouseLeft)
			if m.reading || m.selected != selected || m.listTop != top || strings.Contains(ansi.Strip(m.View().Content), label) {
				t.Fatal("back button did not restore the same list position")
			}
			m, _ = clickAt(m, 6, m.listContentTop(), tea.MouseLeft)
			if !m.reading || m.busy || m.reader.YOffset() != offset || m.selectedKey() != key {
				t.Fatal("reopening lost the cached reading position")
			}
		})
	}
}

func TestMouseBoardAndMenuUseRenderedCoordinates(t *testing.T) {
	m := newModel()
	x := 1 + ansi.StringWidth(m.boardTabs()[0]) + 1
	m, _ = clickAt(m, x, tabsY, tea.MouseLeft)
	if m.board != 1 {
		t.Fatal("board tab not selected")
	}
	m.modal = "menu"
	m.returnModal = "板块"
	m.menu = []menuItem{{"回到全部", "board", "0"}, {"综合", "board", "1"}}
	dialog := m.dialog()
	x = (m.width-lipgloss.Width(dialog))/2 + 4
	y := (m.height-lipgloss.Height(dialog))/2 + 4
	m, _ = clickAt(m, x, y, tea.MouseLeft)
	if m.modal != "" || m.board != 0 {
		t.Fatalf("menu click missed rendered first row: board=%d modal=%s", m.board, m.modal)
	}
}

func TestAgentSwitchPreservesReaderAndFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {80, 24}, {44, 16}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m, _ := pagingModel(t, true, 2)
			m.resize(size[0], size[1])
			m.moveReaderItem(1)
			key := m.selectedKey()
			m, _ = clickAt(m, m.modeButtonX()+2, 0, tea.MouseLeft)
			if !m.chatStyle || m.selectedKey() != key || m.pages[100].Page != 2 || m.busy {
				t.Fatal("style switch changed navigation or fetched again")
			}
			for _, editing := range []bool{false, true} {
				if editing {
					m.identity.Alias = "fixture"
					m.beginCompose(true, false)
					m.editor.SetValue("第一行\n( ﾟ∀。) 第二行")
				}
				v := m.View().Content
				if !strings.Contains(ansi.Strip(v), "Read internal/tui/model.go") || !strings.Contains(ansi.Strip(v), "gpt-6-astra high") {
					t.Fatal("agent reader/reply box missing")
				}
				for _, hidden := range []string{"F6 Agent模拟切换", "b 板块", "楼主", "岛民岛的岛", "我的内容", "Ctrl+P 预览发送"} {
					if strings.Contains(ansi.Strip(v), hidden) {
						t.Fatalf("forum chrome leaked into Agent view: %s", hidden)
					}
				}
				if lipgloss.Height(v) != size[1] {
					t.Fatal("agent view height exceeds terminal")
				}
				for _, line := range strings.Split(v, "\n") {
					if ansi.StringWidth(line) > size[0] {
						t.Fatal("agent line exceeds terminal")
					}
				}
			}
			body := m.editor.Value()
			m, _ = updateKey(m, tea.KeyF6, 0)
			if m.chatStyle || m.modal != "compose" || m.editor.Value() != body || m.selectedKey() != key {
				t.Fatal("toggle lost editor or reader state")
			}
		})
	}
}

func TestAgentReplyUsesDraftPreviewAndConfirmedTarget(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/forum/reply" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		writes++
		var d struct {
			Value    string
			FollowID int
		}
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			t.Error(err)
		}
		if d.FollowID != 100 || d.Value != "模拟页真实回复草稿" || r.Header.Get("Authorization") != "test-cookie" {
			t.Errorf("wrong reply: %+v", d)
		}
		w.Write([]byte(`{"code":200,"data":null}`))
	}))
	defer server.Close()
	store, err := local.New(t.TempDir(), server.URL, server.URL, "file")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Import("fixture", "test-cookie", forum.User{ID: 1, Name: "测试"}); err != nil {
		t.Fatal(err)
	}
	store.Use("fixture")
	m := newModel()
	m.opts = Options{ForumURL: server.URL, UserURL: server.URL, Images: "off"}
	m.store = store
	m.setIdentity("")
	m.applyThread(threadResult{Root: forum.Post{ID: 100, Body: "主楼"}, Page: forum.Page{Page: 1, Count: 1, List: []forum.Post{{ID: 100, Body: "主楼"}}}})
	m, _ = updateKey(m, tea.KeyF6, 0)
	y := m.agentReplyTop() + 1
	m, _ = clickAt(m, 5, y, tea.MouseLeft)
	if !m.inlineAgentCompose() || m.draft.ThreadID != 100 {
		t.Fatal("reply box did not open inline editor")
	}
	m.editor.SetValue("模拟页真实回复草稿")
	m, _ = updateKey(m, tea.KeyEnter, 0)
	if m.modal != "compose" || writes != 0 {
		t.Fatal("Enter in editor must insert newline, not publish")
	}
	m.editor.SetValue("模拟页真实回复草稿")
	m, _ = updateKey(m, tea.KeyEscape, 0)
	drafts, err := store.Drafts("fixture")
	if err != nil || len(drafts) != 1 || drafts[0].Body != "模拟页真实回复草稿" {
		t.Fatal("inline draft not saved")
	}
	m, _ = clickAt(m, 5, y, tea.MouseLeft)
	if m.editor.Value() != "模拟页真实回复草稿" {
		t.Fatal("reply click discarded saved draft")
	}
	// Clicking another hint must not trigger preview.
	m, _ = clickAt(m, 70, m.height-1, tea.MouseLeft)
	if m.modal != "compose" {
		t.Fatal("unrelated footer text triggered preview")
	}
	m, _ = updateKey(m, 'p', tea.ModCtrl)
	if m.modal != "publish" || writes != 0 {
		t.Fatal("preview must not publish")
	}
	var cmd tea.Cmd
	m, cmd = updateKey(m, tea.KeyEnter, 0)
	m = applyCommand(m, cmd)
	if writes != 1 || m.draft.ID != "" {
		t.Fatal("confirmed reply did not complete")
	}
}

func TestAgentReplyWithoutCookieOpensNotice(t *testing.T) {
	m, _ := pagingModel(t, true, 1)
	m, _ = updateKey(m, tea.KeyF6, 0)
	m, _ = clickAt(m, 5, m.agentReplyTop()+1, tea.MouseLeft)
	if m.modal != "menu" || !strings.Contains(m.cookieNotice, "没有饼干") {
		t.Fatal("missing cookie notice")
	}
}

func TestMouseQuoteAndAttachmentHitRenderedRows(t *testing.T) {
	m := press(newModel(), "enter")
	m.movePost(3)
	item := m.readerItems[3]
	if item.quoteLine < 0 {
		t.Fatal("fixture quote hint missing")
	}
	m.reader.SetYOffset(item.line)
	y := browseTop + 2 + item.quoteLine - m.reader.YOffset()
	m, _ = clickAt(m, m.readerX()+5, y, tea.MouseLeft)
	if len(m.inlineQuotes[item.key]) != 1 {
		t.Fatal("click did not expand quote inline")
	}
	m, _ = clickAt(m, m.readerX()+5, y, tea.MouseLeft)
	if len(m.inlineQuotes[item.key]) != 0 {
		t.Fatal("click did not collapse quote")
	}
	m = imagePreviewFixture("x", "off")
	item = m.readerItems[1]
	m.reader.SetYOffset(item.line)
	m, _ = clickAt(m, m.readerX()+5, browseTop+2+item.attachmentFrom-m.reader.YOffset(), tea.MouseLeft)
	if m.modal != "attachment" || m.menu[m.menuIndex].Value != "https://example.test/101.png" {
		t.Fatal("attachment click targeted wrong post")
	}
}

func TestAgentMouseUsesIndependentLayoutAndNeutralPalette(t *testing.T) {
	m := imagePreviewFixture("x", "off")
	m.focusMouseReader()
	m, _ = updateKey(m, tea.KeyF6, 0)
	// Former tabs and switch coordinates are now ordinary blank/header space.
	board := m.board
	m, _ = clickAt(m, 6, tabsY, tea.MouseLeft)
	m, _ = clickAt(m, m.modeButtonX()+2, 0, tea.MouseLeft)
	if !m.chatStyle || m.board != board || m.modal != "" || m.busy {
		t.Fatal("hidden forum control remained clickable")
	}
	item := m.readerItems[1]
	m.reader.SetYOffset(item.line)
	m, _ = clickAt(m, 6, m.readerTop(), tea.MouseLeft)
	if m.selectedKey() != "101" {
		t.Fatal("agent mouse selected wrong visible reply")
	}
	canvas := lipgloss.NewCanvas(m.width, m.height).Compose(lipgloss.NewLayer(m.View().Content))
	for _, pos := range [][2]int{{0, 0}, {m.width - 1, m.height - 1}, {2, 1}, {2, m.height - 1}} {
		cell := canvas.CellAt(pos[0], pos[1])
		if cell == nil || cell.Style.Bg == nil {
			t.Fatal("agent background not set")
		}
		r, g, b, _ := cell.Style.Bg.RGBA()
		if r>>8 != 0x18 || g>>8 != 0x18 || b>>8 != 0x18 {
			t.Fatal("forum colors leaked into agent background")
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), agentSession) {
		t.Fatal("simulated session line missing")
	}
	m, _ = clickAt(m, 6, m.readerTop(), tea.MouseRight)
	if m.modal != "menu" || !strings.Contains(m.returnModal, "101") {
		t.Fatal("agent context menu target incorrect")
	}
	m.modal = ""
	m, _ = updateKey(m, 'h', 0)
	m.selectedThreads[200] = selectionResult{Page: forum.Page{Page: 1, List: []forum.Post{m.raw[200]}}}
	m, _ = clickAt(m, 6, m.listContentTop()+m.listItemHeight(0)+1, tea.MouseLeft)
	if m.selected != 1 || !m.reading || m.current().id != 200 {
		t.Fatal("agent list click used forum coordinates")
	}
}

func TestAgentHidesMetadataAndKeepsSimulatedOutputStable(t *testing.T) {
	m := imagePreviewFixture("x", "off")
	m.agentSeed = 123
	m.focusMouseReader()
	m, _ = updateKey(m, tea.KeyF6, 0)
	before := ansi.Strip(m.reader.GetContent())
	view := ansi.Strip(m.View().Content)
	for _, hidden := range []string{"No.100", "No.101", "thread/No.", m.current().posts[0].author} {
		if hidden != "" && (strings.Contains(before, hidden) || strings.Contains(view, hidden)) {
			t.Fatalf("forum metadata visible: %s", hidden)
		}
	}
	if !strings.Contains(before, "• Ran ") || !strings.Contains(before, "  └ ") {
		t.Fatal("simulated command output missing")
	}
	m.refreshReader(false)
	if ansi.Strip(m.reader.GetContent()) != before {
		t.Fatal("repaint randomized transcript again")
	}
	m.resize(60, 24)
	m.resize(120, 36)
	if ansi.Strip(m.reader.GetContent()) != before {
		t.Fatal("resize changed random choices")
	}
	// Decoration belongs to scroll geometry, but has no post action hit target.
	item := m.readerItems[0]
	if item.actionEnd >= item.end {
		t.Fatal("fixture has no command block")
	}
	m.reader.SetYOffset(item.actionEnd)
	key := m.selectedKey()
	m, cmd := clickAt(m, 6, m.readerTop()+1, tea.MouseRight)
	if m.modal != "" || m.selectedKey() != key || cmd != nil {
		t.Fatal("simulated output acted as a post")
	}
	// The actual reply target is preserved, despite hidden browsing metadata.
	m.identity.Alias = "fixture"
	m.beginCompose(true, true)
	if m.draft.ThreadID != 100 || !strings.Contains(m.draft.Body, "100") {
		t.Fatal("visual simulation changed reply IDs")
	}
}

func TestAgentReferenceMaskDoesNotChangeForumContent(t *testing.T) {
	m := newModel()
	m.opts = Options{Site: "x", Images: "off"}
	root := forum.Post{ID: 123456, Body: "引用 No.654321 和 >>765432\n原文不变"}
	m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, List: []forum.Post{root}}})
	m, _ = updateKey(m, tea.KeyF6, 0)
	rendered := ansi.Strip(m.reader.GetContent())
	if strings.Contains(rendered, "654321") || strings.Contains(rendered, "765432") || !strings.Contains(rendered, "[reference]") {
		t.Fatal("reference IDs not masked in simulated body")
	}
	if m.raw[root.ID].Body != root.Body {
		t.Fatal("simulation modified stored post")
	}
	m, _ = updateKey(m, tea.KeyF6, 0)
	if !strings.Contains(ansi.Strip(m.reader.GetContent()), "No.654321") {
		t.Fatal("forum view lost original reference")
	}
}

func TestMouseMenusDismissWithoutClickingThrough(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, where := range []string{"right-inside", "right-outside", "title-behind", "mode-behind", "border", "padding"} {
			t.Run(fmt.Sprintf("agent=%v/%s", agent, where), func(t *testing.T) {
				m := imagePreviewFixture("x", "off")
				m.focusMouseReader()
				if agent {
					m.toggleChatStyle()
				}
				m.openPostActions()
				m.pendingMine = true
				selected, id, generation := m.selectedKey(), m.current().id, m.requestID
				dialog := m.dialog()
				x, y := (m.width-lipgloss.Width(dialog))/2, (m.height-lipgloss.Height(dialog))/2
				button := tea.MouseLeft
				wantClose := true
				switch where {
				case "right-inside":
					x += 5
					y += 4
					button = tea.MouseRight
				case "right-outside":
					x = 0
					y = 0
					button = tea.MouseRight
				case "title-behind":
					x = 6
					y = m.listContentTop()
				case "mode-behind":
					x = m.modeButtonX() + 2
					y = 0
				case "border":
					wantClose = false
				case "padding":
					x += 4
					y += 1
					wantClose = false
				}
				m, _ = clickAt(m, x, y, button)
				if wantClose != (m.modal == "") {
					t.Fatalf("menu close mismatch: %s", m.modal)
				}
				if m.current().id != id || m.selectedKey() != selected || m.requestID != generation || m.chatStyle != agent || !m.reading {
					t.Fatal("menu click reached underlying page")
				}
				if wantClose && m.pendingMine {
					t.Fatal("menu dismissal did not cancel pending action")
				}
			})
		}
	}
}

func TestMouseTitleOpensUntitledHomeWithoutCachedPage(t *testing.T) {
	m, c := persistenceModel(t, t.TempDir(), "x")
	m = drainMain(t, m, m.initialLoad())
	p := m.raw[100]
	p.Title = ""
	m.raw[100] = p
	m.threads[0] = m.displayThread(p)
	m.refreshReader(false)
	m, cmd := clickAt(m, 6, m.listContentTop(), tea.MouseLeft)
	if cmd == nil || !m.busy {
		t.Fatal("title did not start an uncached read")
	}
	m = applyCommand(m, cmd)
	if !m.reading || m.current().id != 100 || fmt.Sprint(c.reads) != "[1]" {
		t.Fatal("title did not open first page")
	}
}
