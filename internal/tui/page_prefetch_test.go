package tui

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/A-islander/islander-cli/internal/forum"
)

func TestPrefetchOnePageWithoutMovingAndConsumeWithoutBusy(t *testing.T) {
	for _, reading := range []bool{false, true} {
		m, c := pagingModel(t, reading, 1)
		before := m.navigation()
		generation := m.requestID
		offset := m.reader.YOffset()
		cmd := m.preparePagePrefetch()
		if cmd == nil || m.busy {
			t.Fatal("prefetch missing or blocking")
		}
		m = drainPageCommands(m, cmd)
		if m.navigation() != before || m.reader.YOffset() != offset || m.requestID != generation || m.activeWindow().last != 1 || fmt.Sprint(c.calls) != "[2]" {
			t.Fatal("prefetch changed visible content or fetched beyond one page", c.calls)
		}
		for range 5 {
			if m.preparePagePrefetch() != nil {
				t.Fatal("repeated next-page prefetch")
			}
		}
		lastPagingItem(&m)
		m, cmd = pagingKey(m, "j")
		if m.busy || m.requestID != generation || m.activeWindow().last != 2 {
			t.Fatal("cached page did not join synchronously")
		}
		m = drainPageCommands(m, cmd)
		if fmt.Sprint(c.calls) != "[2 3]" || m.activeWindow().last != 2 {
			t.Fatal("did not keep just one page ahead", c.calls)
		}
		lastPagingItem(&m)
		m, cmd = pagingKey(m, "j")
		m = drainPageCommands(m, cmd)
		if m.busy || m.activeWindow().last != 3 || fmt.Sprint(c.calls) != "[2 3]" {
			t.Fatal("last page duplicated or not cached")
		}
	}
}

func TestPendingPrefetchKeepsInputResponsiveAndHonorsLatestPosition(t *testing.T) {
	for _, reading := range []bool{false, true} {
		for _, back := range []bool{false, true} {
			m, c := pagingModel(t, reading, 1)
			fetch := m.preparePagePrefetch()
			lastPagingItem(&m)
			for range 3 {
				var cmd tea.Cmd
				m, cmd = pagingKey(m, "j")
				if m.busy || cmd != nil {
					t.Fatal("in-flight prefetch blocked or duplicated")
				}
			}
			if back {
				m, _ = pagingKey(m, "k")
			}
			before := m.navigation()
			m = drainPageCommands(m, fetch)
			if m.busy {
				t.Fatal("background completion left UI busy")
			}
			if back {
				if m.navigation() != before || m.activeWindow().last != 1 {
					t.Fatal("late page moved user after they scrolled away")
				}
			} else if m.activeWindow().last != 2 {
				t.Fatal("waiting boundary did not join loaded page")
			}
			if c.calls[0] != 2 || len(c.calls) > 2 {
				t.Fatal("duplicated pending request", c.calls)
			}
		}
	}
}

func TestPrefetchFailureEmptyAndStaleResults(t *testing.T) {
	for _, reading := range []bool{false, true} {
		m, c := pagingModel(t, reading, 1)
		c.fail = true
		before, notice := m.navigation(), m.notice
		m = prefetchReady(m)
		if m.navigation() != before || m.notice != notice || m.busy || m.preparePagePrefetch() != nil {
			t.Fatal("background failure disturbed reader or retried")
		}
		lastPagingItem(&m)
		c.fail = false
		m, cmd := pagingKey(m, "j")
		m = drainPageCommands(m, cmd)
		if m.activeWindow().last != 2 {
			t.Fatal("explicit continuation could not retry failure")
		}

		m, c = pagingModel(t, reading, 1)
		c.pages[2] = forum.Page{Page: 2, HasMore: true}
		m = prefetchReady(m)
		lastPagingItem(&m)
		m, cmd = pagingKey(m, "j")
		if cmd != nil || len(c.calls) != 1 || m.activeWindow().last != 1 {
			t.Fatal("empty prefetch looped or cleared content")
		}

		m, c = pagingModel(t, reading, 1)
		late := m.preparePagePrefetch()
		if reading {
			m.applyThread(threadResult{Root: forum.Post{ID: 999}, Page: forum.Page{Page: 1, List: []forum.Post{{ID: 999, Body: "新串"}}}})
		} else {
			m.applyPage(forum.Page{Page: 1, List: []forum.Post{{ID: 999, Body: "新版块"}}})
		}
		m = drainPageCommands(m, late)
		if m.current().id != 999 || m.activeWindow().last != 1 || m.activePrefetch().ready != nil {
			t.Fatal("stale prefetch crossed page scope")
		}
	}
}

type cancellingPageBackend struct {
	forum.Backend
	started chan struct{}
}

func (c *cancellingPageBackend) List(ctx context.Context, _ string, _, _ int) (forum.Page, error) {
	close(c.started)
	<-ctx.Done()
	return forum.Page{}, ctx.Err()
}
func (c *cancellingPageBackend) Capabilities() forum.Capabilities { return forum.Capabilities{} }

func TestReplacingWindowCancelsBackgroundRequest(t *testing.T) {
	m, _ := pagingModel(t, false, 1)
	c := &cancellingPageBackend{started: make(chan struct{})}
	m.client = c
	cmd := m.preparePagePrefetch()
	done := make(chan struct{})
	go func() { cmd(); close(done) }()
	select {
	case <-c.started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	m.applyPage(forum.Page{Page: 1})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("old window request not cancelled")
	}
}

func prefetchReady(m model) model { cmd := m.preparePagePrefetch(); return drainPageCommands(m, cmd) }
