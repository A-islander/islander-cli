package tui

import (
	"context"
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

// Refresh only the page being read after a successful reply. Keep the loaded
// window and quote tree so continuous reading does not collapse to page one.
func (m *model) refreshAfterReply() tea.Cmd {
	m.syncPagingPosition()
	id, page := m.current().id, max(1, m.currentPage().Page)
	m.threadPrefetch.clear()
	return m.launch("reply-refresh", func(ctx context.Context, c forum.Backend) (any, error) {
		p, err := c.List(ctx, "thread", id, page)
		if err == nil && (p.Page != page || len(p.List) == 0) {
			err = fmt.Errorf("第 %d 页返回内容不完整", page)
		}
		return p, err
	})
}

func (m *model) applyReplyRefresh(p forum.Page) {
	t := m.current()
	if t == nil {
		return
	}
	key, offset, line := m.selectedKey(), m.reader.YOffset(), 0
	for _, item := range m.readerItems {
		if item.key == key {
			line = item.line
			break
		}
	}
	pages := m.threadWindow.pages
	if pages == nil {
		pages = map[int]forum.Page{}
	}
	pages[p.Page] = p
	numbers := make([]int, 0, len(pages))
	for n := range pages {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	window := newPageWindow(pages[numbers[0]])
	rows := append([]forum.Post(nil), pages[numbers[0]].List...)
	for _, n := range numbers[1:] {
		rows = append(rows, window.add(pages[n])...)
	}
	m.threadWindow = window
	t.posts = nil
	for _, row := range rows {
		t.posts = append(t.posts, m.displayPost(row))
	}
	if p.Root != nil {
		m.raw[t.id] = *p.Root
	}
	m.pages[t.id] = p
	m.refreshReader(false)
	for _, item := range m.readerItems {
		if item.key == key {
			m.setReaderItem(item)
			m.refreshReader(false)
			m.reader.SetYOffset(offset + item.line - line)
			break
		}
	}
	m.syncPagingPosition()
	m.savePosition()
	m.notice = "回复已发布 · 已保留当前页与阅读位置"
}
