package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

type resultMsg struct {
	ID    int
	Kind  string
	Value any
	Err   error
}
type initialResult struct {
	Boards []forum.Board
	Page   forum.Page
}
type threadResult struct {
	Root   forum.Post
	Page   forum.Page
	Target int
}
type menuItem struct{ Label, Action, Value string }

type startMsg struct{}

func (m *model) initialLoad() tea.Cmd {
	return m.launch("initial", func(ctx context.Context, c forum.Backend) (any, error) {
		b, e := c.Boards(ctx)
		if e != nil {
			return nil, e
		}
		p, e := c.List(ctx, "timeline", 0, 1)
		return initialResult{b, p}, e
	})
}
func (m *model) launch(kind string, fn func(context.Context, forum.Backend) (any, error)) tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	m.cancel = cancel
	m.requestID++
	id := m.requestID
	c := m.client
	m.busy = true
	return func() tea.Msg { defer cancel(); v, e := fn(ctx, c); return resultMsg{id, kind, v, e} }
}
func (m model) environmentLabel() string {
	if m.opts.Demo {
		return "离线体验"
	}
	for _, s := range forum.Sites() {
		if m.opts.Site == s.ID && s.ID != "islander" {
			return s.Name
		}
	}
	if m.opts.ForumURL != forum.ForumURL {
		return "自定义服务"
	}
	return "岛民岛"
}
func (m model) identityLabel() string {
	if m.identity.Alias == "" {
		return "访客 · i 导入饼干"
	}
	return m.identity.Alias + " / " + m.identity.Name
}
func (m model) replyCount(t thread) int {
	if p, ok := m.raw[t.id]; ok {
		return p.ReplyCount
	}
	return len(t.posts) - 1
}
func (m model) boardLabel(id int) string {
	for _, b := range m.apiBoards {
		if b.ID == id {
			return forum.Clean(b.Name)
		}
	}
	if id <= 0 {
		return "板块未知"
	}
	return "板块 " + strconv.Itoa(id)
}
func (m *model) displayPost(p forum.Post) post {
	m.raw[p.ID] = p
	t := time.Unix(p.Time, 0).Format("01-02 15:04")
	if p.Time == 0 {
		t = "时间未知"
	}
	if p.Time > 1e12 {
		t = time.UnixMilli(p.Time).Format("01-02 15:04")
	}
	quote := 0
	if len(p.Quotes) > 0 {
		quote = p.Quotes[0]
	} else if ids := forum.QuoteIDs(p.Body); len(ids) > 0 {
		quote = ids[0]
	}
	att := ""
	if items := p.Media(); len(items) > 0 {
		att = fmt.Sprintf("%d 个附件 · a 加载附件", len(items))
	}
	return post{p.ID, forum.Clean(p.Name), t, forum.Clean(p.Body), quote, att, p.Status == 2}
}
func (m *model) displayThread(p forum.Post) thread {
	title := forum.Clean(p.Title)
	excerpt := strings.ReplaceAll(forum.Clean(p.Body), "\n", " ")
	if title == "" {
		excerpt = ""
		title = strings.ReplaceAll(forum.Clean(p.Body), "\n", " ")
		if p.FollowID > 0 {
			title = "↳ 回复 No." + strconv.Itoa(p.FollowID) + " · " + title
		}
	}
	if p.Status == 2 {
		title = "[已删除] " + title
	}
	if p.Status == 1 {
		title = "[SAGE] " + title
	}
	board := p.BoardName
	if board == "" {
		board = m.boardLabel(p.BoardID)
	}
	return thread{p.ID, board, title, excerpt, []post{m.displayPost(p)}}
}
func (m *model) applyPage(p forum.Page) {
	m.loadedThreadID = 0
	m.listWindow = newPageWindow(p)
	m.threadWindow = pageWindow{}
	m.stateReady = true
	m.inlineQuotes = map[string][]inlineQuote{}
	m.quoteOffsets = map[string]int{}
	m.listError = ""
	m.jumpSource = nil
	m.threads = nil
	m.page = p.Page
	m.total = p.Count
	m.listPage = p
	for _, v := range p.List {
		m.threads = append(m.threads, m.displayThread(v))
	}
	m.refilter()
	if m.newest {
		m.sortNewest()
		m.refreshReader(false)
	}
	m.notice = p.Label() + " · b 板块 / g 切站 / i 饼干"
	if m.kind == "mine" {
		m.notice = fmt.Sprintf("我的内容 · 饼干 %s · 发串与回复（含删除记录）· 第 %d 页 / %d 条", m.identity.Alias, p.Page, p.Count)
	}
}

func (m *model) openMine() tea.Cmd {
	if !m.capabilities().Mine {
		m.notice = "当前站点不支持我的内容"
		return nil
	}
	if m.opts.Demo {
		m.notice = "离线原型没有个人内容；请连接论坛或运行本地试用岛"
		return nil
	}
	if m.identity.Alias == "" {
		m.pendingMine = true
		m.openCookies()
		m.notice = "选择发帖时使用的饼干，随后打开我的内容"
		return nil
	}
	m.pendingMine = false
	m.pendingRestore = nil
	m.savePosition()
	m.modal, m.filter = "", ""
	m.kind, m.board = "mine", 0
	m.threads, m.jumpSource = nil, nil
	m.page, m.total = 1, 0
	m.refilter()
	return m.loadList(1)
}

func (m *model) leaveReading() {
	m.savePosition()
	m.reading = false
	m.activeQuote = ""
	if m.jumpSource != nil && len(m.visible) > 0 {
		m.threads[m.visible[m.selected]] = *m.jumpSource
		m.jumpSource = nil
		m.activePost = 0
		m.refreshReader(true)
	}
	m.refreshReader(false)
}
func (m *model) loadList(page int) tea.Cmd {
	m.loadedThreadID = 0
	m.listError = ""
	kind, id := m.kind, 0
	if m.board > 0 && m.board <= len(m.apiBoards) {
		id = m.apiBoards[m.board-1].ID
	}
	m.reading = false
	return m.launch("list", func(ctx context.Context, c forum.Backend) (any, error) { return c.List(ctx, kind, id, page) })
}
func (m *model) loadThread(id, page, target int) tea.Cmd {
	if t := m.current(); m.reading && t != nil && t.id == id && m.pages[t.id].Page == page {
		m.savePosition()
	}
	return m.launch("thread", func(ctx context.Context, c forum.Backend) (any, error) {
		p, e := c.Post(ctx, id)
		if e != nil {
			return nil, e
		}
		if p.FollowID > 0 {
			target = p.ID
			p, e = c.Post(ctx, p.FollowID)
			if e != nil {
				return nil, e
			}
		}
		if target > 0 {
			page, e = c.ReplyPage(ctx, p.ID, target)
			if e != nil {
				return nil, e
			}
		}
		list, e := c.List(ctx, "thread", p.ID, page)
		if list.Root != nil {
			p = *list.Root
		}
		return threadResult{p, list, target}, e
	})
}
func (m *model) applyThread(r threadResult) {
	m.loadedThreadID = r.Root.ID
	m.threadWindow = newPageWindow(r.Page)
	m.stateReady = true
	m.readVisited = time.Now().UnixNano()
	t := m.displayThread(r.Root)
	t.posts = nil
	for _, p := range r.Page.List {
		t.posts = append(t.posts, m.displayPost(p))
	}
	if len(t.posts) == 0 {
		t.posts = append(t.posts, m.displayPost(r.Root))
	}
	m.pages[t.id] = r.Page
	if len(m.visible) == 0 {
		m.threads = append(m.threads, t)
		m.visible = []int{len(m.threads) - 1}
		m.selected = 0
	} else {
		m.threads[m.visible[m.selected]] = t
	}
	m.reading = true
	m.activePost = 0
	m.activeQuote = ""
	m.refreshReader(true)
	if r.Target > 0 {
		for i, p := range t.posts {
			if p.id == r.Target {
				m.movePost(i - m.activePost)
			}
		}
	}
	m.notice = fmt.Sprintf("串 No.%d · %s · [ ] 翻页 · v 引用 / a 附件", t.id, r.Page.Label())
}
func (m *model) activeRaw() (forum.Post, bool) {
	selected, ok := m.selectedPost()
	if !ok {
		return forum.Post{}, false
	}
	p, ok := m.raw[selected.id]
	return p, ok
}
func (m *model) openBoards() {
	m.menu = []menuItem{{"全部 · 最新回复", "board", "0"}}
	for i, b := range m.apiBoards {
		m.menu = append(m.menu, menuItem{forum.Clean(b.Name), "board", strconv.Itoa(i + 1)})
	}
	if m.capabilities().Sage {
		m.menu = append(m.menu, menuItem{"SAGE 内容", "sage", ""})
	}
	if m.capabilities().Mine {
		m.menu = append(m.menu, menuItem{"我的内容 · 含删除记录", "mine", ""})
	}
	m.menuIndex = 0
	m.modal = "menu"
	m.returnModal = "板块与时间线"
}
