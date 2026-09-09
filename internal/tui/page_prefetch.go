package tui

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

var pageWindowSequence atomic.Uint64
var pagePrefetchSequence atomic.Uint64

type pagePrefetch struct {
	window, request uint64
	page            int
	pending, failed bool
	cancel          context.CancelFunc
	ready           *forum.Page
	waiting         *pageWait
}
type pageWait struct {
	result   paginationResult
	selected int
	key      string
	offset   int
}
type pagePrefetchResult struct {
	window, request uint64
	reading         bool
	page            forum.Page
	err             error
}

func (p *pagePrefetch) clear() {
	if p.cancel != nil {
		p.cancel()
	}
	*p = pagePrefetch{}
}
func (m *model) stopPagePrefetch() {
	m.listPrefetch.clear()
	m.threadPrefetch.clear()
	// A failed navigation may leave the previous content visible. Do not
	// prefetch using that content with the newly selected board or identity.
	m.listWindow.id, m.threadWindow.id = 0, 0
}
func (m *model) activePrefetch() *pagePrefetch {
	if m.reading {
		return &m.threadPrefetch
	}
	return &m.listPrefetch
}

// Exactly one page ahead of the selected page. Ready pages stay separate from
// visible content, so background completion cannot move the reading anchor.
func (m *model) preparePagePrefetch() tea.Cmd {
	if m.opts.Demo || m.client == nil || m.busy || m.modal != "" || (!m.reading && (m.filter != "" || m.newest)) {
		return nil
	}
	w := m.activeWindow()
	p := m.currentPage()
	if w.id == 0 || p.Page < 1 || !p.HasMore {
		return nil
	}
	page := p.Page + 1
	if page > 1000000 || w.blocked[page] {
		return nil
	}
	if _, loaded := w.pages[page]; loaded {
		return nil
	}
	slot := m.activePrefetch()
	if slot.window == w.id && slot.page == page {
		return nil
	}
	slot.clear()
	kind, id := m.kind, 0
	reading := m.reading
	if reading {
		t := m.current()
		if t == nil || !m.hasLoadedThread() {
			return nil
		}
		kind, id = "thread", t.id
	} else if m.board > 0 && m.board <= len(m.apiBoards) {
		id = m.apiBoards[m.board-1].ID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	*slot = pagePrefetch{window: w.id, request: pagePrefetchSequence.Add(1), page: page, pending: true, cancel: cancel}
	window, request, c := slot.window, slot.request, m.client
	return func() tea.Msg {
		defer cancel()
		p, err := c.List(ctx, kind, id, page)
		if err == nil && p.Page != page {
			err = fmt.Errorf("预取返回页码与请求不符")
		}
		return pagePrefetchResult{window: window, request: request, reading: reading, page: p, err: err}
	}
}

func (m model) pageWaitMatches(wait pageWait) bool {
	if m.busy || m.modal != "" || m.reading != wait.result.reading {
		return false
	}
	t := m.current()
	if t == nil || t.id != wait.selected {
		return false
	}
	return !m.reading || (m.selectedKey() == wait.key && m.reader.YOffset() == wait.offset)
}

func (m *model) usePrefetchedPage(r paginationResult) bool {
	slot := m.activePrefetch()
	if slot.window != m.activeWindow().id || slot.page != r.page.Page || slot.failed {
		return false
	}
	if slot.ready != nil {
		r.page = *slot.ready
		slot.clear()
		m.applyPagination(r)
		return true
	}
	if slot.pending {
		if t := m.current(); t != nil {
			slot.waiting = &pageWait{result: r, selected: t.id, key: m.selectedKey(), offset: m.reader.YOffset()}
			m.notice = "下一页正在预取，可继续浏览已加载内容"
			return true
		}
	}
	return false
}

func (m *model) pagePrefetchUpdate(msg tea.Msg) (tea.Cmd, bool) {
	r, ok := msg.(pagePrefetchResult)
	if !ok {
		return nil, false
	}
	slot, w := &m.listPrefetch, &m.listWindow
	if r.reading {
		slot, w = &m.threadPrefetch, &m.threadWindow
	}
	if r.request == 0 || r.request != slot.request || r.window != w.id || r.window != slot.window {
		return nil, true
	}
	wait := slot.waiting
	slot.pending, slot.cancel, slot.waiting = false, nil, nil
	if r.err != nil {
		slot.failed = true
		if wait != nil && m.pageWaitMatches(*wait) {
			m.notice = "下一页预取失败，继续向下或按 ] 重试"
		}
		return nil, true
	}
	if len(r.page.List) == 0 {
		w.blocked[slot.page] = true
		slot.failed = true
		if wait != nil && m.pageWaitMatches(*wait) {
			m.notice = "这一页没有更多内容 · P 跳页"
		}
		return nil, true
	}
	slot.ready = &r.page
	if wait != nil && m.pageWaitMatches(*wait) {
		from, listFrom := m.readerVisualOffset(), m.listVisualOffset()
		result := wait.result
		result.page = r.page
		slot.clear()
		m.applyPagination(result)
		if result.extend {
			return tea.Batch(m.startReaderScroll(from), m.startListScroll(listFrom)), true
		}
	}
	return nil, true
}
