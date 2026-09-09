package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

type pagePosition struct{ page, index int }
type pageWindow struct {
	id          uint64
	first, last int
	pages       map[int]forum.Page
	positions   map[int]pagePosition
	blocked     map[int]bool
}

func newPageWindow(p forum.Page) pageWindow {
	w := pageWindow{id: pageWindowSequence.Add(1), first: p.Page, last: p.Page, pages: map[int]forum.Page{}, positions: map[int]pagePosition{}, blocked: map[int]bool{}}
	w.add(p)
	return w
}
func (w *pageWindow) add(p forum.Page) []forum.Post {
	w.pages[p.Page] = p
	w.first = min(w.first, p.Page)
	w.last = max(w.last, p.Page)
	added := []forum.Post{}
	for i, post := range p.List {
		if _, ok := w.positions[post.ID]; !ok {
			w.positions[post.ID] = pagePosition{p.Page, i}
			added = append(added, post)
		}
	}
	return added
}
func (m *model) activeWindow() *pageWindow {
	if m.reading {
		return &m.threadWindow
	}
	return &m.listWindow
}
func (m model) currentPage() forum.Page {
	if m.reading {
		if t := m.current(); t != nil {
			return m.pages[t.id]
		}
	}
	return m.listPage
}
func (m model) totalPages() int {
	p := m.currentPage()
	if p.Count >= 0 && p.Size > 0 {
		return max(1, (p.Count+p.Size-1)/p.Size)
	}
	if m.reading && m.opts.Site == "x" {
		if t := m.current(); t != nil {
			if root, ok := m.raw[t.id]; ok && p.Size == 19 {
				return max(1, (root.ReplyCount+18)/19)
			}
		}
	}
	return 0
}

// Track the page of the selected post, rather than the last page downloaded.
func (m *model) syncPagingPosition() {
	t := m.current()
	if t == nil {
		return
	}
	if pos, ok := m.listWindow.positions[t.id]; ok {
		m.listPage = m.listWindow.pages[pos.page]
		m.page = pos.page
	}
	if m.reading && m.activePost >= 0 && m.activePost < len(t.posts) {
		if pos, ok := m.threadWindow.positions[t.posts[m.activePost].id]; ok {
			m.pages[t.id] = m.threadWindow.pages[pos.page]
		}
	}
}

type paginationResult struct {
	page                    forum.Page
	rootID, direction       int
	key                     string
	reading, extend, latest bool
}

func (m *model) requestPage(page int, extend bool, direction int, key string, latest bool) tea.Cmd {
	if page < 1 || page > 1000000 {
		m.pageJumpError = "页码必须在 1—1000000 之间"
		return nil
	}
	if total := m.totalPages(); total > 0 && page > total {
		m.pageJumpError = fmt.Sprintf("请输入 1—%d 之间的页码", total)
		return nil
	}
	m.flushPersistence()
	kind, id := m.kind, 0
	reading := m.reading
	if reading {
		if t := m.current(); t != nil {
			kind, id = "thread", t.id
		} else {
			return nil
		}
	} else if m.board > 0 && m.board <= len(m.apiBoards) {
		id = m.apiBoards[m.board-1].ID
	}
	m.modal = ""
	m.input.Blur()
	if m.usePrefetchedPage(paginationResult{page: forum.Page{Page: page}, rootID: id, direction: direction, key: key, reading: reading, extend: extend, latest: latest}) {
		return nil
	}
	m.activePrefetch().clear()
	return m.launch("pagination", func(ctx context.Context, c forum.Backend) (any, error) {
		p, err := c.List(ctx, kind, id, page)
		if err == nil && p.Page != page {
			err = fmt.Errorf("返回页码与请求不符，已保留当前位置")
		}
		if err == nil && !extend && page > 1 && len(p.List) == 0 {
			err = fmt.Errorf("第 %d 页没有内容，已保留当前位置", page)
		}
		if err != nil {
			p.Page = page
		}
		return paginationResult{page: p, rootID: id, direction: direction, key: key, reading: reading, extend: extend, latest: latest}, err
	})
}
func (m *model) applyPagination(r paginationResult) {
	if !r.extend {
		if r.reading {
			root := m.raw[r.rootID]
			if r.page.Root != nil {
				root = *r.page.Root
			}
			m.offsets[r.rootID] = 0
			delete(m.readerSelections, r.rootID)
			m.applyThread(threadResult{Root: root, Page: r.page})
			if r.latest {
				m.movePost(len(m.current().posts))
				m.reader.SetYOffset(max(0, m.readerItems[len(m.readerItems)-1].end-m.reader.Height()))
			}
		} else {
			m.applyPage(r.page)
		}
		m.pageJumpError = ""
		return
	}
	w := m.activeWindow()
	// Empty pages end this direction even if a server mistakenly says hasMore.
	if len(r.page.List) == 0 {
		w.blocked[r.page.Page] = true
		m.notice = "这一页没有更多内容 · P 跳页"
		return
	}
	t := m.current()
	selectedID := 0
	if t != nil {
		selectedID = t.id
	}
	oldKey, oldOffset, oldLine := m.selectedKey(), m.reader.YOffset(), 0
	for _, item := range m.readerItems {
		if item.key == oldKey {
			oldLine = item.line
		}
	}
	added := w.add(r.page)
	if r.reading {
		if t == nil || t.id != r.rootID {
			return
		}
		posts := make([]post, 0, len(added))
		for _, p := range added {
			posts = append(posts, m.displayPost(p))
		}
		if r.direction > 0 {
			t.posts = append(t.posts, posts...)
		} else {
			t.posts = append(posts, t.posts...)
			m.activePost += len(posts)
		}
		m.refreshReader(false)
		for _, item := range m.readerItems {
			if item.key == oldKey {
				m.reader.SetYOffset(oldOffset + item.line - oldLine)
				break
			}
		}
	} else {
		threads := make([]thread, 0, len(added))
		for _, p := range added {
			threads = append(threads, m.displayThread(p))
		}
		if r.direction > 0 {
			m.threads = append(m.threads, threads...)
		} else {
			m.threads = append(threads, m.threads...)
		}
		m.refilter()
		for i, index := range m.visible {
			if m.threads[index].id == selectedID {
				m.selected = i
				break
			}
		}
		m.ensureListVisible()
		m.refreshReader(false)
	}
	if len(added) > 0 {
		if !r.reading {
			m.moveSelection(r.direction)
		} else {
			switch r.key {
			case "j", "k", "down", "up":
				m.moveReaderItem(r.direction)
			case "n", "p":
				m.movePost(r.direction)
			default:
				amount := m.reader.Height()
				if r.key == "wheel" {
					amount = 3
				}
				if r.key == "ctrl+d" || r.key == "ctrl+u" {
					amount = max(1, amount/2)
				}
				m.reader.SetYOffset(m.reader.YOffset() + r.direction*amount)
				m.syncActivePost()
			}
		}
	}
	m.notice = fmt.Sprintf("已加载第 %d—%d 页 · P 跳页 · [ ] 翻页", w.first, w.last)
}

func (m *model) paginationInput(msg tea.Msg) (tea.Cmd, bool) {
	if m.opts.Demo || m.busy {
		return nil, false
	}
	if m.modal == "page-jump" {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				m.modal = ""
				m.input.Blur()
				return nil, true
			case "ctrl+c":
				return tea.Quit, true
			case "enter":
				value := strings.TrimSpace(m.input.Value())
				page, err := strconv.Atoi(value)
				if err != nil || page < 1 {
					m.pageJumpError = "请输入大于 0 的整数页码"
					return nil, true
				}
				return m.requestPage(page, false, 0, "", false), true
			case "ctrl+home":
				return m.requestPage(1, false, 0, "", false), true
			case "ctrl+end":
				if total := m.totalPages(); total > 0 {
					return m.requestPage(total, false, 0, "", false), true
				}
				m.pageJumpError = "总页数未知，请输入页码"
				return nil, true
			case "ctrl+l":
				if total := m.totalPages(); total > 0 && m.reading {
					return m.requestPage(total, false, 0, "", true), true
				}
				return nil, true
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return cmd, true
	}
	if m.modal != "" {
		return nil, false
	}
	key, dir := "", 0
	if k, ok := msg.(tea.KeyPressMsg); ok {
		key = k.String()
		if key == "P" {
			m.pageJumpError = ""
			cmd := m.inputDialog("page-jump", "输入页码")
			m.input.CharLimit = 7
			return cmd, true
		}
		switch key {
		case "j", "down", "n", "pgdown", "ctrl+d", "space":
			dir = 1
		case "k", "up", "p", "pgup", "ctrl+u":
			dir = -1
		}
	} else if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		key = "wheel"
		if wheel.Button == tea.MouseWheelDown {
			dir = 1
		} else if wheel.Button == tea.MouseWheelUp {
			dir = -1
		}
	}
	if dir == 0 || (!m.reading && (m.filter != "" || m.newest || key == "n" || key == "p" || key == "space")) {
		return nil, false
	}
	w := m.activeWindow()
	if w.first < 1 || len(w.pages) == 0 {
		return nil, false
	}
	boundary := false
	if m.reading {
		if len(m.readerItems) == 0 {
			return nil, false
		}
		first, last := m.readerItems[0], m.readerItems[len(m.readerItems)-1]
		if dir > 0 {
			boundary = m.reader.YOffset()+m.reader.Height() >= last.end
		} else {
			boundary = m.reader.YOffset() <= first.line
		}
		if key == "j" || key == "down" {
			boundary = boundary && m.selectedKey() == last.key
		}
		if key == "k" || key == "up" {
			boundary = boundary && m.selectedKey() == first.key
		}
		if key == "n" {
			boundary = m.activePost == len(m.current().posts)-1
		}
		if key == "p" {
			boundary = m.activePost == 0
		}
	} else {
		boundary = len(m.visible) > 0 && ((dir > 0 && m.selected == len(m.visible)-1) || (dir < 0 && m.selected == 0))
	}
	if !boundary {
		return nil, false
	}
	page := w.last + 1
	if dir < 0 {
		page = w.first - 1
	}
	if page < 1 || page > 1000000 || (dir > 0 && !w.pages[w.last].HasMore) {
		return nil, false
	}
	if w.blocked[page] {
		return nil, false
	}
	return m.requestPage(page, true, dir, key, false), true
}
func (m model) pageJumpContent(width int) string {
	total := "总页数未知"
	if n := m.totalPages(); n > 0 {
		total = fmt.Sprintf("共 %d 页", n)
	}
	content := strong("跳转页码", teal) + "\n" + ink(fmt.Sprintf("当前第 %d 页 · %s", max(1, m.currentPage().Page), total), muted) + "\n\n" + m.input.View() + "\n"
	if m.pageJumpError != "" {
		content += ink(clip(m.pageJumpError, width), sand) + "\n"
	}
	content += "\n" + ink("Enter 跳转 · Esc 返回", sand) + "\n" + ink("Ctrl+Home 首页", muted)
	if m.totalPages() > 0 {
		content += ink(" · Ctrl+End 末页", muted)
		if m.reading {
			content += "\n" + ink("Ctrl+L 最新回复", muted)
		}
	}
	return content
}
