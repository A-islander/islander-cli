package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
)

func TestRapidBoardSwitchCancelsHTTPAndRejectsLateResults(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/showf" {
			t.Errorf("unexpected path %s", r.URL.Path)
			return
		}
		if r.URL.Query().Get("id") == "10" {
			close(started)
			select {
			case <-r.Context().Done():
				close(cancelled)
			case <-time.After(3 * time.Second):
			}
			return
		}
		fmt.Fprint(w, `[{"id":200,"content":"new board"}]`)
	}))
	defer server.Close()
	c, err := forum.NewBackend("x", server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.opts = Options{Site: "x", Images: "off"}
	m.client = c
	m.boardNames = []string{"全部", "A", "B"}
	m.apiBoards = []forum.Board{{ID: 10}, {ID: 20}}
	switchNext := func() tea.Cmd {
		next, cmd, handled := m.extendedUpdate(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
		m = next.(model)
		if !handled || cmd == nil {
			t.Fatal("board key ignored during read")
		}
		return cmd
	}
	first := switchNext()
	defer func() {
		if m.cancel != nil {
			m.cancel()
		}
	}()
	oldID := m.requestID
	done := make(chan tea.Msg, 1)
	go func() { done <- first() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("old request never started")
	}
	second := switchNext()
	if m.board != 2 || !m.busy || m.requestID <= oldID {
		t.Fatal("latest key did not replace read")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("old HTTP request was not cancelled")
	}
	var old tea.Msg
	select {
	case old = <-done:
	case <-time.After(time.Second):
		t.Fatal("old request did not return")
	}
	next, _, _ := m.extendedUpdate(old)
	m = next.(model)
	if !m.busy || m.board != 2 {
		t.Fatal("cancelled result cleared new request")
	}
	next, _, _ = m.extendedUpdate(second())
	m = next.(model)
	if m.busy || m.current() == nil || m.current().id != 200 {
		t.Fatal("new board failed to load", m.notice)
	}
	next, _, _ = m.extendedUpdate(resultMsg{ID: oldID, Kind: "list", Value: forum.Page{Page: 1, List: []forum.Post{{ID: 999}}}})
	m = next.(model)
	if m.current().id != 200 || m.board != 2 {
		t.Fatal("late success overwrote latest board")
	}
}

func TestBoardSwitchOnlyReplacesBrowsingRequests(t *testing.T) {
	for _, kind := range []string{"list", "thread", "resume-thread", "pagination", "inline-quote", "reply-refresh", "initial", "publish", "action", "identity", "registered"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := pagingModel(t, false, 1)
			m.boardNames = []string{"全部", "A"}
			m.apiBoards = []forum.Board{{ID: 10}}
			m.launch(kind, func(context.Context, forum.Backend) (any, error) { return nil, nil })
			// Also cancel next-page and selected-thread work when leaving the board.
			listCtx, listCancel := context.WithCancel(context.Background())
			threadCtx, threadCancel := context.WithCancel(context.Background())
			selectionCtx, selectionCancel := context.WithCancel(context.Background())
			m.listPrefetch.cancel, m.threadPrefetch.cancel, m.selectionCancel = listCancel, threadCancel, selectionCancel
			defer listCancel()
			defer threadCancel()
			defer selectionCancel()
			oldID := m.requestID
			m, _ = updateKey(m, ']', tea.ModCtrl)
			defer m.cancel()
			read := kind == "list" || kind == "thread" || kind == "resume-thread" || kind == "pagination" || kind == "inline-quote" || kind == "reply-refresh"
			if read {
				if m.board != 1 || m.requestID <= oldID || listCtx.Err() == nil || threadCtx.Err() == nil || selectionCtx.Err() == nil {
					t.Fatal("read or related work not superseded")
				}
			} else if m.board != 0 || m.requestID != oldID || !m.busy {
				t.Fatal("interrupted non-browsing operation")
			}
		})
	}
}
