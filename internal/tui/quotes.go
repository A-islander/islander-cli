package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

type inlineQuote struct {
	post post
	err  string
}
type readerItem struct {
	key               string
	post              post
	root, depth, line int
}
type inlineQuoteResult struct {
	parent string
	ids    []int
	posts  map[int]forum.Post
	errors map[int]string
}

func (m model) selectedPost() (post, bool) {
	if m.activeQuote != "" {
		for _, item := range m.readerItems {
			if item.key == m.activeQuote {
				return item.post, true
			}
		}
	}
	if t := m.current(); t != nil && m.activePost >= 0 && m.activePost < len(t.posts) {
		return t.posts[m.activePost], true
	}
	return post{}, false
}

func (m model) selectedKey() string {
	if m.activeQuote != "" {
		return m.activeQuote
	}
	if p, ok := m.selectedPost(); ok {
		return strconv.Itoa(p.id)
	}
	return ""
}

func (m *model) setReaderItem(item readerItem) {
	m.activePost, m.activeQuote = item.root, ""
	if item.depth > 0 {
		m.activeQuote = item.key
	}
}

func (m *model) moveReaderItem(delta int) {
	if len(m.readerItems) == 0 {
		return
	}
	index := 0
	for i, item := range m.readerItems {
		if item.key == m.selectedKey() {
			index = i
			break
		}
	}
	item := m.readerItems[max(0, min(len(m.readerItems)-1, index+delta))]
	m.setReaderItem(item)
	m.refreshReader(false)
	m.reader.SetYOffset(item.line)
}

func (m *model) returnToQuoteParent() bool {
	if m.activeQuote == "" {
		return false
	}
	index := strings.LastIndex(m.activeQuote, "/")
	if index < 0 {
		return false
	}
	parent := m.activeQuote[:index]
	for _, item := range m.readerItems {
		if item.key == parent {
			m.setReaderItem(item)
			m.refreshReader(false)
			offset := item.line
			if saved, ok := m.quoteOffsets[parent]; ok {
				offset = saved
			}
			m.reader.SetYOffset(offset)
			return true
		}
	}
	return false
}

func (m model) quoteIDs(p post) []int {
	ids := []int{}
	if raw, ok := m.raw[p.id]; ok {
		ids = raw.Quotes
	}
	if len(ids) == 0 {
		ids = forum.QuoteIDs(p.body)
	}
	if len(ids) == 0 && p.quote != 0 {
		ids = []int{p.quote}
	}
	unique, seen := []int{}, map[int]bool{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	return unique
}

func (m *model) toggleInlineQuotes() tea.Cmd {
	if !m.reading {
		return nil
	}
	p, ok := m.selectedPost()
	if !ok {
		return nil
	}
	key := m.selectedKey()
	if _, open := m.inlineQuotes[key]; open {
		for branch := range m.inlineQuotes {
			if branch == key || strings.HasPrefix(branch, key+"/") {
				delete(m.inlineQuotes, branch)
				delete(m.quoteOffsets, branch)
			}
		}
		m.refreshReader(false)
		m.notice = "已收起引用"
		return nil
	}
	parts := strings.Split(key, "/")
	if len(parts) > 12 {
		m.notice = "引用最多原位展开 12 层"
		return nil
	}
	for _, ancestor := range parts[:len(parts)-1] {
		if ancestor == strconv.Itoa(p.id) {
			m.notice = "这是循环引用，已停止继续展开"
			return nil
		}
	}
	ids := m.quoteIDs(p)
	if len(ids) == 0 {
		m.notice = "当前帖子没有引用"
		return nil
	}
	m.quoteOffsets[key] = m.reader.YOffset()
	if m.opts.Demo {
		children := []inlineQuote{}
		for _, id := range ids {
			child := inlineQuote{post: post{id: id}, err: "引用内容不在离线数据中"}
			for _, t := range m.threads {
				for _, candidate := range t.posts {
					if candidate.id == id {
						child = inlineQuote{post: candidate}
					}
				}
			}
			children = append(children, child)
		}
		m.inlineQuotes[key] = children
		m.refreshReader(false)
		m.notice = "引用已原位展开 · ↓ 选中引用 · v 继续展开 / 收起 · Esc 返回上一层"
		return nil
	}
	result := inlineQuoteResult{parent: key, ids: ids, posts: map[int]forum.Post{}, errors: map[int]string{}}
	missing := []int{}
	for _, id := range ids {
		if cached, ok := m.raw[id]; ok {
			result.posts[id] = cached
		} else {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		m.applyInlineQuotes(result)
		return nil
	}
	return m.launch("inline-quote", func(ctx context.Context, c *forum.Client) (any, error) {
		for _, id := range missing {
			p, err := c.Post(ctx, id)
			if err != nil {
				result.errors[id] = forum.Clean(err.Error())
			} else {
				result.posts[id] = p
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		return result, nil
	})
}

func (m *model) applyInlineQuotes(result inlineQuoteResult) {
	children := []inlineQuote{}
	for _, id := range result.ids {
		child := inlineQuote{post: post{id: id}, err: result.errors[id]}
		if p, ok := result.posts[id]; ok {
			child.post = m.displayPost(p)
		}
		children = append(children, child)
	}
	m.inlineQuotes[result.parent] = children
	m.refreshReader(false)
	m.notice = fmt.Sprintf("已展开 %d 条引用 · ↓ 选中 · v 继续展开 / 收起 · Esc 返回上一层", len(children))
}
