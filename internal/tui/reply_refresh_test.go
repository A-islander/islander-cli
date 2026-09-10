package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
)

func TestReplyRefreshKeepsContinuousWindowAndReadingAnchor(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		for _, agent := range []bool{false, true} {
			for _, nested := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/agent=%v/quote=%v", site, agent, nested), func(t *testing.T) {
					m, c := pagingModel(t, true, 1)
					m.opts.Site, m.chatStyle = site, agent
					second := c.pages[2]
					second.List[1].Body = strings.Repeat("长回复正文\n", 60)
					c.pages[2] = second
					m.applyPagination(paginationResult{page: second, rootID: 100, reading: true, extend: true, direction: 1})
					m.applyPagination(paginationResult{page: c.pages[3], rootID: 100, reading: true, extend: true, direction: 1})
					m.activePost = 3
					if nested {
						m.inlineQuotes["121"] = []inlineQuote{{post: post{id: 900, body: "引用"}}}
						m.inlineQuotes["121/900"] = []inlineQuote{{post: post{id: 901, body: strings.Repeat("嵌套引用\n", 60)}}}
						m.activeQuote = "121/900/901"
					}
					m.refreshReader(false)
					item := selectedReaderItem(t, m)
					m.reader.SetYOffset(item.line + 10)
					m.syncPagingPosition()
					before, key, offset := m.navigation(), m.selectedKey(), m.reader.YOffset()
					// A newly visible reply may be added to this page without moving to it.
					second.List = append(second.List, forum.Post{ID: 129, FollowID: 100, Body: "刚刚发送的回复"})
					c.pages[2] = second
					m.draft = forum.Draft{ID: "sent", ThreadID: 100, Body: "reply"}
					m.modal, m.busy = "publish", true
					next, cmd, _ := m.extendedUpdate(resultMsg{ID: m.requestID, Kind: "publish"})
					m = next.(model)
					if cmd == nil || m.draft.ID != "" || m.modal != "" {
						t.Fatal("reply success not handled")
					}
					next, _, _ = m.extendedUpdate(cmd())
					m = next.(model)
					if fmt.Sprint(c.calls) != "[2]" {
						t.Fatal("refreshed wrong page", c.calls)
					}
					if m.busy || m.navigation() != before || m.selectedKey() != key || m.reader.YOffset() != offset {
						t.Fatalf("reading position changed: before=%+v after=%+v offsets=%d/%d", before, m.navigation(), offset, m.reader.YOffset())
					}
					if m.threadWindow.first != 1 || m.threadWindow.last != 3 || len(m.current().posts) != 7 || m.raw[129].Body != "刚刚发送的回复" {
						t.Fatal("lost loaded pages or new reply")
					}
					if nested && len(m.inlineQuotes["121/900"]) != 1 {
						t.Fatal("nested reference collapsed")
					}
				})
			}
		}
	}
}

func TestReplyRefreshFailureDoesNotUndoSuccessfulPost(t *testing.T) {
	for _, failure := range []string{"network", "empty", "wrong-page"} {
		t.Run(failure, func(t *testing.T) {
			m, c := pagingModel(t, true, 2)
			m.movePost(1)
			before, body := m.navigation(), m.reader.GetContent()
			m.draft = forum.Draft{ID: "sent", ThreadID: 100, Body: "reply"}
			m.modal, m.busy = "publish", true
			switch failure {
			case "network":
				c.fail = true
			case "empty":
				c.pages[2] = forum.Page{Page: 2}
			case "wrong-page":
				c.pages[2] = c.pages[1]
			}
			next, cmd, _ := m.extendedUpdate(resultMsg{ID: m.requestID, Kind: "publish"})
			m = next.(model)
			next, _, _ = m.extendedUpdate(cmd())
			m = next.(model)
			if m.busy || m.modal != "" || m.draft.ID != "" || m.navigation() != before || m.reader.GetContent() != body || !strings.Contains(m.notice, "回复已发布，刷新失败") {
				t.Fatal("refresh failure lost position or implied post failed", m.notice)
			}
		})
	}
}
