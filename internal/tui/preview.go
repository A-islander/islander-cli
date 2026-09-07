package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

type previewTick struct{ Generation, Request, Target int }
type previewResult struct {
	previewTick
	Page forum.Page
	Err  error
}

// Preview requests have their own cancellation/generation, and never block list
// navigation or replace an active full-thread request.
func (m model) updateWithPreview(msg tea.Msg) (tea.Model, tea.Cmd) {
	var next tea.Model
	var cmd tea.Cmd
	switch v := msg.(type) {
	case previewTick:
		if m.acceptPreview(v) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			m.previewCancel = cancel
			c := m.client
			cmd = func() tea.Msg {
				defer cancel()
				p, err := c.List(ctx, "thread", v.Target, 1)
				return previewResult{v, p, err}
			}
		}
		next = m
	case previewResult:
		if m.acceptPreview(v.previewTick) {
			m.previews[v.Target] = v
			m.previewCancel = nil
			m.refreshReader(false)
		}
		next = m
	default:
		next, cmd = m.update(msg)
	}
	n := next.(model)
	previewCmd := n.preparePreview()
	if previewCmd == nil {
		return n, cmd
	}
	if cmd == nil {
		return n, previewCmd
	}
	return n, tea.Batch(cmd, previewCmd)
}

func (m model) acceptPreview(v previewTick) bool {
	t := m.current()
	return !m.reading && !m.busy && m.client != nil && t != nil && t.id == v.Target &&
		m.requestID == v.Generation && m.previewID == v.Request && m.previewTarget == v.Target
}

func (m *model) preparePreview() tea.Cmd {
	// requestID changes on refresh, identity and site changes as well as full
	// reads. A cache belongs only to this list generation.
	if m.previewGeneration != m.requestID {
		m.previews = nil
		m.previewGeneration = m.requestID
		m.previewTarget = 0
		if m.previewCancel != nil {
			m.previewCancel()
			m.previewCancel = nil
		}
		if !m.reading {
			m.refreshReader(false)
		}
	}
	t := m.current()
	if m.opts.Demo || m.client == nil || m.reading || m.busy || m.modal != "" || !m.split() || t == nil {
		if m.previewCancel != nil {
			m.previewCancel()
			m.previewCancel = nil
		}
		if m.previewTarget != 0 {
			m.previewID++
			m.previewTarget = 0
		}
		return nil
	}
	if m.previewTarget != 0 && m.previewTarget != t.id {
		if m.previewCancel != nil {
			m.previewCancel()
			m.previewCancel = nil
		}
		m.previewID++
		m.previewTarget = 0
	}
	// Personal-content reply entries are previews of that specific reply, not
	// roots; do not send reply IDs to external thread endpoints.
	if p, ok := m.raw[t.id]; ok && (p.FollowID > 0 || p.ParentUnknown) {
		return nil
	}
	if m.previews == nil {
		m.previews = map[int]previewResult{}
	}
	if _, ok := m.previews[t.id]; ok {
		return nil
	}
	if m.previewTarget == t.id {
		return nil
	}
	if m.previewCancel != nil {
		m.previewCancel()
		m.previewCancel = nil
	}
	m.previewID++
	m.previewTarget = t.id
	v := previewTick{m.requestID, m.previewID, t.id}
	m.refreshReader(false)
	return tea.Tick(180*time.Millisecond, func(time.Time) tea.Msg { return v })
}

func previewLines(s string, width, limit int) []string {
	lines := strings.Split(wrapText(forum.Clean(strings.TrimSpace(s)), width), "\n")
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = clip(lines[limit-1], max(1, width-1)) + "…"
	}
	return lines
}

func (m model) previewContent(t thread) string {
	w := max(1, m.reader.Width()-2)
	p, ok := m.raw[t.id]
	if !ok {
		return m.previewFallback(t, w)
	}
	result, loaded := m.previews[t.id]
	root := p
	if loaded && result.Err == nil && result.Page.Root != nil {
		root = *result.Page.Root
	}
	lines := []string{strong(fmt.Sprintf("No.%d · %s", root.ID, forum.Clean(root.Name)), teal)}
	if root.Title != "" {
		lines = append(lines, strong(clip(forum.Clean(root.Title), w), foam))
	}
	body := root.Body
	if root.Status == 2 {
		body = "[这条内容已被删除]"
	}
	for _, line := range previewLines(body, w, 3) {
		lines = append(lines, ink(line, foam))
	}
	if count := len(root.Media()); count > 0 {
		lines = append(lines, ink(fmt.Sprintf("▧ %d 个附件", count), sand))
	}
	if p.FollowID > 0 || p.ParentUnknown {
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "")
	if !loaded {
		hint := "正在加载回复预览…"
		if m.client == nil {
			hint = "Enter 打开串，读取回复"
		}
		return strings.Join(append(lines, ink(hint, muted)), "\n")
	}
	if result.Err != nil {
		return strings.Join(append(lines, ink(clip("回复预览失败："+result.Err.Error(), w), muted), ink("Enter 打开串 / Ctrl+R 重试", teal)), "\n")
	}
	replies := []forum.Post{}
	for _, reply := range result.Page.List {
		if reply.ID != root.ID {
			replies = append(replies, reply)
		}
	}
	more := result.Page.HasMore || len(replies) > 5
	if len(replies) > 5 {
		replies = replies[:5]
	}
	if len(replies) == 0 {
		return strings.Join(append(lines, ink("暂无回复", muted)), "\n")
	}
	label := fmt.Sprintf("回复预览 · %d 条", len(replies))
	if more {
		label += " · Enter 查看更多"
	}
	lines = append(lines, ink(label, teal))
	// Keep five short replies visible in the normal 120x36 layout. Full text
	// remains available in the reader; previews never modify source posts.
	limit := max(1, min(2, (m.reader.Height()-len(lines))/len(replies)-1))
	for _, reply := range replies {
		header := fmt.Sprintf("No.%d · %s", reply.ID, forum.Clean(reply.Name))
		if len(reply.Media()) > 0 {
			header += " · ▧"
		}
		lines = append(lines, ink(clip(header, w), sand))
		body := reply.Body
		if reply.Status == 2 {
			body = "[这条回复已被删除]"
		}
		for _, line := range previewLines(body, w, limit) {
			lines = append(lines, ink(line, foam))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) previewFallback(t thread, w int) string {
	if len(t.posts) == 0 {
		return ""
	}
	return bodyText(strings.Join(previewLines(t.posts[0].body, w, 3), "\n"), w)
}
