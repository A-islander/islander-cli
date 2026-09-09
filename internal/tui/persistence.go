package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
)

var persistenceSequence atomic.Uint64

type persistenceTick struct{ ID uint64 }
type draftSaveTick struct{ ID uint64 }

func (m *model) loadPersistence() {
	m.stopPagePrefetch()
	m.listWindow, m.threadWindow = pageWindow{}, pageWindow{}
	m.stateReady = false
	m.pendingRestore = nil
	m.favoriteEntries = nil
	m.readVisited = 0
	m.stateTickID = persistenceSequence.Add(1)
	m.draftTickID = persistenceSequence.Add(1)
	if m.opts.Demo || m.store == nil {
		return
	}
	b, err := m.store.Browsing(m.identity.Alias)
	if err != nil {
		m.stateError = err.Error()
		return
	}
	m.favoriteEntries = b.Favorites
	// Homepage entry starts on the timeline. Saved thread positions remain
	// available through explicit selection, history and favorites.
}

func (m *model) rememberSite() {
	if m.opts.Demo || m.store == nil {
		return
	}
	s, err := forum.Resolve(m.opts.Site, m.opts.ForumURL, m.opts.UserURL)
	if err == nil {
		err = local.RememberSite(filepath.Dir(m.store.Dir), s)
	}
	if err != nil {
		m.opts.StateWarning = "偏好保存失败：" + err.Error()
	}
}

func (m model) navigation() local.Navigation {
	n := local.Navigation{Kind: m.kind, Page: max(1, m.page), Filter: m.filter, Newest: m.newest}
	if m.board > 0 && m.board <= len(m.apiBoards) {
		b := m.apiBoards[m.board-1]
		n.BoardID, n.BoardKey = b.ID, b.Key
	}
	t := m.current()
	if t == nil {
		return n
	}
	n.SelectedID = t.id
	if m.jumpSource != nil {
		n.SelectedID = m.jumpSource.id
	}
	if !m.reading {
		return n
	}
	n.ThreadID, n.ReadPage = t.id, max(1, m.pages[t.id].Page)
	if len(t.posts) == 0 {
		return n
	}
	i := max(0, min(m.activePost, len(t.posts)-1))
	n.AnchorID = t.posts[i].id
	if i < len(m.postLines) {
		start, end := m.postLines[i], m.reader.TotalLineCount()
		if i+1 < len(m.postLines) {
			end = m.postLines[i+1]
		}
		n.AnchorFraction = min(1.0, max(0.0, float64(m.reader.YOffset()-start)/float64(max(1, end-start))))
	}
	return n
}

func (m *model) flushPersistence() {
	if m.opts.Demo || m.store == nil || !m.stateReady || m.busy || m.pendingRestore != nil {
		return
	}
	n := m.navigation()
	var entry *local.HistoryEntry
	if n.ThreadID > 0 && m.readVisited > 0 {
		entry = &local.HistoryEntry{Navigation: n, Title: m.current().title, VisitedAt: m.readVisited}
	}
	if err := m.store.SaveBrowsing(m.identity.Alias, n, entry); err != nil {
		m.stateError = err.Error()
	} else {
		m.stateError = ""
	}
}

// Disk writes happen on the model goroutine. Timers only deliver identifiers,
// so a late timer cannot write an old site's snapshot or resurrect a draft.
func (m model) updatePersistent(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.BlurMsg:
		m.flushPersistence()
		if !m.opts.Demo && !m.busy && m.modal == "compose" {
			if err := m.saveDraft(); err != nil {
				m.stateError = err.Error()
			}
		}
		return m, nil
	case persistenceTick:
		if v.ID == m.stateTickID {
			m.flushPersistence()
		}
		return m, nil
	case draftSaveTick:
		if v.ID == m.draftTickID && m.modal == "compose" && !m.busy && m.store != nil {
			m.syncDraft()
			if err := m.store.AutoSaveDraft(&m.draft); err != nil {
				m.stateError = "草稿保存失败：" + err.Error()
			} else {
				m.stateError = ""
			}
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && !m.opts.Demo {
		// Flush before any command that can leave the current list/reader.
		if m.modal == "" || m.modal == "menu" {
			switch key.String() {
			case "enter", "esc", "q", "ctrl+c", "g", "m", "b", "i", "H", "F", "*", "a", "[", "]", "ctrl+r", "t", ":", "h", "left", "right", "l", "tab":
				m.flushPersistence()
			}
		}
		if m.modal == "compose" && key.String() == "f2" && !m.busy {
			return m, m.openDraftEdits()
		}
		if m.modal == "menu" && strings.HasPrefix(m.returnModal, "收藏") && key.String() == "/" {
			return m, m.inputDialog("favorites-filter", "按标题或串号筛选收藏")
		}
		if m.modal == "favorites-filter" {
			switch key.String() {
			case "esc":
				m.openFavorites("")
				return m, nil
			case "enter":
				m.openFavorites(m.input.Value())
				return m, nil
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
		}
		if m.modal == "menu" && strings.HasPrefix(m.returnModal, "浏览历史") && key.String() == "/" {
			return m, m.inputDialog("history-filter", "按标题或串号筛选历史")
		}
		if m.modal == "history-filter" {
			switch key.String() {
			case "esc":
				m.openHistory("")
				return m, nil
			case "enter":
				m.openHistory(m.input.Value())
				return m, nil
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
		}
		if m.modal == "menu" && strings.HasPrefix(m.returnModal, "草稿编辑历史") && (key.String() == "esc" || key.String() == "q") {
			return m, m.editDraft()
		}
	}
	before := m.navigation()
	oldBody, oldTitle := m.editor.Value(), m.titleInput.Value()
	oldFiles := fmt.Sprint(m.draft.Files, m.draft.Media)
	next, cmd := m.updateWithSelection(msg)
	n := next.(model)
	n.syncPagingPosition()
	var tasks []tea.Cmd
	if cmd != nil {
		tasks = append(tasks, cmd)
	}
	if !n.opts.Demo && n.store != nil {
		if n.stateReady && !n.busy && n.pendingRestore == nil && (before != n.navigation() || n.readVisited != m.readVisited) {
			n.stateTickID = persistenceSequence.Add(1)
			id := n.stateTickID
			tasks = append(tasks, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return persistenceTick{id} }))
		}
		if n.modal == "compose" && !n.busy && (oldBody != n.editor.Value() || oldTitle != n.titleInput.Value() || oldFiles != fmt.Sprint(n.draft.Files, n.draft.Media)) {
			n.draftTickID = persistenceSequence.Add(1)
			id := n.draftTickID
			tasks = append(tasks, tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg { return draftSaveTick{id} }))
		}
	}
	if len(tasks) == 0 {
		return n, nil
	}
	if len(tasks) == 1 {
		return n, tasks[0]
	}
	return n, tea.Batch(tasks...)
}

func (m *model) startRestore(n local.Navigation) tea.Cmd {
	m.pendingRestore = &n
	m.kind, m.board = n.Kind, 0
	switch m.kind {
	case "timeline", "board":
	case "mine":
		if m.identity.Alias == "" || !m.capabilities().Mine {
			m.kind = "timeline"
		}
	case "sage":
		if !m.capabilities().Sage {
			m.kind = "timeline"
		}
	default:
		m.kind = "timeline"
	}
	if m.kind == "board" {
		for i, b := range m.apiBoards {
			if (n.BoardKey != "" && b.Key == n.BoardKey) || (n.BoardKey == "" && b.ID == n.BoardID) {
				m.board = i + 1
				break
			}
		}
		if m.board == 0 {
			m.kind = "timeline"
			n.Page = 1
			m.notice = "原板块已不可用，回到当前岛时间线"
		}
	}
	m.newest = n.Newest
	return m.loadList(max(1, n.Page))
}

func (m *model) resumeAfterList() tea.Cmd {
	if m.pendingRestore == nil {
		return nil
	}
	n := *m.pendingRestore
	m.filter = n.Filter
	m.refilter()
	if m.newest {
		m.sortNewest()
	}
	for i, index := range m.visible {
		if m.threads[index].id == n.SelectedID {
			m.selected = i
			break
		}
	}
	m.ensureListVisible()
	m.refreshReader(false)
	if n.ThreadID <= 0 {
		m.pendingRestore = nil
		return nil
	}
	if t := m.current(); t != nil && t.id != n.ThreadID {
		saved := *t
		m.jumpSource = &saved
	}
	return m.resumeThread(n)
}

func (m *model) resumeThread(n local.Navigation) tea.Cmd {
	m.threadPrefetch.clear()
	m.pendingRestore = &n
	return m.launch("resume-thread", func(ctx context.Context, c forum.Backend) (any, error) {
		return fetchRestoredThread(ctx, c, n)
	})
}

func fetchRestoredThread(ctx context.Context, c forum.Backend, n local.Navigation) (threadResult, error) {
	root, err := c.Post(ctx, n.ThreadID)
	if err != nil {
		return threadResult{}, err
	}
	page := max(1, n.ReadPage)
	p, err := c.List(ctx, "thread", n.ThreadID, page)
	if page > 1 && (err != nil || len(p.List) == 0) {
		p, err = c.List(ctx, "thread", n.ThreadID, 1)
	}
	if err != nil {
		return threadResult{}, err
	}
	hasAnchor := func(p forum.Page) bool {
		for _, post := range p.List {
			if post.ID == n.AnchorID {
				return true
			}
		}
		return false
	}
	if n.AnchorID > 0 && !hasAnchor(p) {
		for _, neighbor := range []int{p.Page - 1, p.Page + 1} {
			if neighbor < 1 || (neighbor > p.Page && !p.HasMore) {
				continue
			}
			other, e := c.List(ctx, "thread", n.ThreadID, neighbor)
			if e == nil && hasAnchor(other) {
				p = other
				break
			}
		}
	}
	if p.Root != nil {
		root = *p.Root
	}
	return threadResult{Root: root, Page: p, Target: n.AnchorID}, nil
}

func (m *model) applyRestoredThread(r threadResult) {
	m.applyThread(r)
	if m.pendingRestore != nil {
		n := *m.pendingRestore
		found := false
		if t := m.current(); t != nil {
			for i, p := range t.posts {
				if p.id == n.AnchorID && i < len(m.postLines) {
					start, end := m.postLines[i], m.reader.TotalLineCount()
					if i+1 < len(m.postLines) {
						end = m.postLines[i+1]
					}
					m.reader.SetYOffset(start + int(min(1.0, max(0.0, n.AnchorFraction))*float64(max(1, end-start))))
					m.activePost = i
					found = true
					break
				}
			}
		}
		if n.AnchorID == 0 {
			m.notice += " · 已恢复阅读页码"
		} else if found {
			m.notice += " · 已恢复阅读位置"
		} else {
			m.notice += " · 原楼层不在附近页，已打开可用页"
		}
	}
	m.pendingRestore = nil
}

func (m *model) sortNewest() {
	sort.SliceStable(m.visible, func(i, j int) bool {
		return m.raw[m.threads[m.visible[i]].id].Time > m.raw[m.threads[m.visible[j]].id].Time
	})
}

func (m *model) openHistory(query string) {
	if m.store == nil {
		return
	}
	b, err := m.store.Browsing(m.identity.Alias)
	if err != nil {
		m.stateError = err.Error()
		return
	}
	m.historyEntries = b.History
	m.menu = nil
	q := strings.ToLower(strings.TrimSpace(query))
	for _, e := range b.History {
		label := fmt.Sprintf("No.%d · 第 %d 页 · %s · %s", e.ThreadID, max(1, e.ReadPage), time.Unix(0, e.VisitedAt).Format("01-02 15:04"), e.Title)
		if q == "" || strings.Contains(strings.ToLower(label), q) {
			m.menu = append(m.menu, menuItem{label, "history", strconv.Itoa(e.ThreadID)})
		}
	}
	label := "暂停记录"
	if b.HistoryDisabled {
		label = "启用记录"
	}
	m.menu = append(m.menu, menuItem{label, "history-toggle", ""}, menuItem{"清空当前岛 / 当前身份的历史", "history-clear", ""})
	m.menuIndex = 0
	m.modal = "menu"
	alias := m.identity.Alias
	if alias == "" {
		alias = "访客"
	}
	m.returnModal = "浏览历史 · " + m.environmentLabel() + " / " + alias + " · / 筛选 · x 删除"
}

func (m *model) openDraftEdits() tea.Cmd {
	if err := m.saveDraft(); err != nil {
		m.stateError = err.Error()
		return nil
	}
	if m.store == nil {
		return nil
	}
	edits, err := m.store.DraftEdits(m.draft.ID, m.identity.Alias)
	if err != nil {
		m.stateError = err.Error()
		return nil
	}
	m.draftEdits = edits
	m.menu = nil
	for i := len(edits) - 1; i >= 0; i-- {
		d := edits[i]
		label := fmt.Sprintf("%s · %s", time.Unix(d.Updated, 0).Format("01-02 15:04:05"), strings.ReplaceAll(forum.Clean(d.Body), "\n", " "))
		m.menu = append(m.menu, menuItem{label, "draft-edit", strconv.Itoa(i)})
	}
	m.menuIndex = 0
	m.modal = "menu"
	m.returnModal = "草稿编辑历史 · Enter 恢复为新版本 · Esc 返回编辑"
	return nil
}

// Focus changes reuse the active reader or side-reader cache. Persisted positions
// are the fallback when this thread has not been loaded during this session.
func (m *model) openThread(id int) tea.Cmd {
	if m.enterSelectedThread(id) {
		return nil
	}
	if m.homeThread == id {
		return m.loadHomeThread(id)
	}
	if m.store != nil {
		b, err := m.store.Browsing(m.identity.Alias)
		if err != nil {
			m.stateError = err.Error()
		} else if saved, ok := b.ReadingPosition(id); ok {
			n := m.navigation()
			n.ThreadID, n.ReadPage, n.AnchorID, n.AnchorFraction = id, saved.ReadPage, saved.AnchorID, saved.AnchorFraction
			return m.resumeThread(n)
		}
	}
	return m.loadThread(id, 1, 0)
}

func (m *model) toggleFavorite() {
	t := m.current()
	if t == nil || m.store == nil {
		return
	}
	n := m.navigation()
	n.ThreadID = t.id
	if p, ok := m.raw[t.id]; ok {
		n.ThreadID = p.ThreadID()
	}
	added, err := m.store.ToggleFavorite(m.identity.Alias, local.HistoryEntry{Navigation: n, Title: t.title})
	if err != nil {
		m.stateError = err.Error()
		return
	}
	b, err := m.store.Browsing(m.identity.Alias)
	if err != nil {
		m.stateError = err.Error()
		return
	}
	m.favoriteEntries = b.Favorites
	m.notice = fmt.Sprintf("已取消收藏 No.%d", n.ThreadID)
	if added {
		m.notice = fmt.Sprintf("已收藏 No.%d · F 打开收藏", n.ThreadID)
	}
}

func (m model) isFavorite(id int) bool {
	if p, ok := m.raw[id]; ok {
		id = p.ThreadID()
	}
	for _, e := range m.favoriteEntries {
		if e.ThreadID == id {
			return true
		}
	}
	return false
}

func (m *model) openFavorites(query string) {
	if m.store == nil {
		return
	}
	b, err := m.store.Browsing(m.identity.Alias)
	if err != nil {
		m.stateError = err.Error()
		return
	}
	m.favoriteEntries = b.Favorites
	m.menu = nil
	q := strings.ToLower(strings.TrimSpace(query))
	for _, e := range b.Favorites {
		label := fmt.Sprintf("★ No.%d · 第 %d 页 · %s", e.ThreadID, max(1, e.ReadPage), e.Title)
		if q == "" || strings.Contains(strings.ToLower(label), q) {
			m.menu = append(m.menu, menuItem{label, "favorite", strconv.Itoa(e.ThreadID)})
		}
	}
	m.modal, m.menuIndex = "menu", 0
	alias := m.identity.Alias
	if alias == "" {
		alias = "访客"
	}
	m.returnModal = "收藏 · " + m.environmentLabel() + " / " + alias + " · / 筛选 · x 取消收藏"
	if len(m.menu) == 0 {
		m.returnModal += " · 无匹配项，选串按 * 收藏"
	}
}
