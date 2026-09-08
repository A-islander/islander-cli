package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
)

type selectionTick struct{ Generation, Request, Target int }
type selectionResult struct {
	selectionTick
	Page   forum.Page
	Err    error
	Root   *forum.Post
	Resume *local.Navigation
}

// Selection requests have independent cancellation and never block list navigation.
func (m model) updateWithSelection(msg tea.Msg) (tea.Model, tea.Cmd) {
	var next tea.Model
	var cmd tea.Cmd
	switch v := msg.(type) {
	case selectionTick:
		if m.acceptSelection(v) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			m.selectionCancel = cancel
			c := m.client
			var resume *local.Navigation
			if m.store != nil {
				if browsing, err := m.store.Browsing(m.identity.Alias); err == nil {
					if saved, ok := browsing.ReadingPosition(v.Target); ok {
						n := m.navigation()
						n.ThreadID, n.ReadPage, n.AnchorID, n.AnchorFraction = v.Target, saved.ReadPage, saved.AnchorID, saved.AnchorFraction
						resume = &n
					}
				} else {
					m.stateError = err.Error()
				}
			}
			cmd = func() tea.Msg {
				defer cancel()
				if resume != nil {
					r, err := fetchRestoredThread(ctx, c, *resume)
					return selectionResult{selectionTick: v, Page: r.Page, Err: err, Root: &r.Root, Resume: resume}
				}
				p, err := c.List(ctx, "thread", v.Target, 1)
				return selectionResult{selectionTick: v, Page: p, Err: err}
			}
		}
		next = m
	case selectionResult:
		if m.acceptSelection(v.selectionTick) {
			m.selectedThreads[v.Target] = v
			m.selectionCancel = nil
			m.refreshReader(false)
		}
		next = m
	default:
		next, cmd = m.update(msg)
	}
	n := next.(model)
	selectionCmd := n.prepareSelection()
	if selectionCmd == nil {
		return n, cmd
	}
	if cmd == nil {
		return n, selectionCmd
	}
	return n, tea.Batch(cmd, selectionCmd)
}

func (m model) acceptSelection(v selectionTick) bool {
	t := m.current()
	return !m.reading && !m.busy && m.client != nil && t != nil && t.id == v.Target &&
		m.requestID == v.Generation && m.selectionID == v.Request && m.selectionTarget == v.Target
}

func (m *model) prepareSelection() tea.Cmd {
	// requestID changes on refresh, identity and site changes as well as full
	// reads. A cache belongs only to this list generation.
	if m.selectionGeneration != m.requestID {
		m.selectedThreads = nil
		m.selectionGeneration = m.requestID
		m.selectionTarget = 0
		if m.selectionCancel != nil {
			m.selectionCancel()
			m.selectionCancel = nil
		}
		if !m.reading {
			m.refreshReader(false)
		}
	}
	t := m.current()
	if m.opts.Demo || m.client == nil || m.reading || m.hasLoadedThread() || m.busy || m.modal != "" || !m.split() || t == nil {
		if m.selectionCancel != nil {
			m.selectionCancel()
			m.selectionCancel = nil
		}
		if m.selectionTarget != 0 {
			m.selectionID++
			m.selectionTarget = 0
		}
		return nil
	}
	if m.selectionTarget != 0 && m.selectionTarget != t.id {
		if m.selectionCancel != nil {
			m.selectionCancel()
			m.selectionCancel = nil
		}
		m.selectionID++
		m.selectionTarget = 0
	}
	// Personal-content reply entries contain that specific reply, not
	// roots; do not send reply IDs to external thread endpoints.
	if p, ok := m.raw[t.id]; ok && (p.FollowID > 0 || p.ParentUnknown) {
		return nil
	}
	if m.selectedThreads == nil {
		m.selectedThreads = map[int]selectionResult{}
	}
	if _, ok := m.selectedThreads[t.id]; ok {
		return nil
	}
	if m.selectionTarget == t.id {
		return nil
	}
	if m.selectionCancel != nil {
		m.selectionCancel()
		m.selectionCancel = nil
	}
	m.selectionID++
	m.selectionTarget = t.id
	v := selectionTick{m.requestID, m.selectionID, t.id}
	m.refreshReader(false)
	return tea.Tick(180*time.Millisecond, func(time.Time) tea.Msg { return v })
}

// The side reader uses the same unabridged post renderer as focused reading.
// Its cached page does not change persisted reading positions.
func (m *model) selectedThreadContent(t thread) string {
	result, loaded := m.selectedThreads[t.id]
	if root, ok := m.raw[t.id]; ok {
		t = m.displayThread(root)
	}
	if loaded && result.Err == nil {
		root := m.raw[t.id]
		if result.Root != nil {
			root = *result.Root
		}
		if result.Page.Root != nil {
			root = *result.Page.Root
		}
		t = m.displayThread(root)
		t.posts = nil
		for _, p := range result.Page.List {
			t.posts = append(t.posts, m.displayPost(p))
		}
		if len(t.posts) == 0 {
			t.posts = append(t.posts, m.displayPost(root))
		}
	}
	return m.threadContent(t)
}

func (m model) displayedThreadPage(id int) (forum.Page, bool) {
	if !m.opts.Demo && !m.reading && !m.hasLoadedThread() {
		r, ok := m.selectedThreads[id]
		return r.Page, ok && r.Err == nil
	}
	p, ok := m.pages[id]
	return p, ok
}

func (m model) hasLoadedThread() bool {
	t := m.current()
	return t != nil && m.loadedThreadID > 0 && t.id == m.loadedThreadID
}

// Opening an already loaded thread changes focus without repeating the request.
func (m *model) enterSelectedThread(id int) bool {
	t := m.current()
	if t != nil && t.id == id && m.hasLoadedThread() {
		m.reading = true
		m.readVisited = time.Now().UnixNano()
		m.refreshReader(false)
		return true
	}
	if t == nil || t.id != id || m.selectionGeneration != m.requestID {
		return false
	}
	r, ok := m.selectedThreads[id]
	if !ok || r.Err != nil {
		return false
	}
	root := m.raw[id]
	if root.FollowID > 0 || root.ParentUnknown {
		return false
	}
	if r.Root != nil {
		root = *r.Root
	}
	if r.Page.Root != nil {
		root = *r.Page.Root
	}
	result := threadResult{Root: root, Page: r.Page}
	if r.Resume != nil {
		m.pendingRestore = r.Resume
		result.Target = r.Resume.AnchorID
		m.applyRestoredThread(result)
	} else {
		m.applyThread(result)
	}
	return true
}

func (m model) selectedThreadAttachments() []menuItem {
	t := m.current()
	if t == nil {
		return nil
	}
	root, ok := m.raw[t.id]
	if !ok {
		return nil
	}
	posts := []forum.Post{root}
	if m.hasLoadedThread() {
		posts = nil
		for _, p := range t.posts {
			posts = append(posts, m.raw[p.id])
		}
	} else if r, loaded := m.selectedThreads[t.id]; loaded && r.Err == nil {
		if r.Page.Root != nil {
			posts[0] = *r.Page.Root
		}
		for _, p := range r.Page.List {
			if p.ID != posts[0].ID {
				posts = append(posts, p)
			}
		}
	}
	var items []menuItem
	for _, p := range posts {
		if p.Status == 2 {
			continue
		}
		for _, a := range p.Media() {
			items = append(items, menuItem{fmt.Sprintf("No.%d · %s · %s", p.ID, a.Type, a.URL), "attachment", a.URL})
		}
	}
	return items
}
