package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
)

type resumeBackend struct {
	forum.Backend
	reads   []int
	missing bool
}

func (*resumeBackend) Capabilities() forum.Capabilities {
	return forum.Capabilities{Publish: true, Mine: true}
}
func (*resumeBackend) Boards(context.Context) ([]forum.Board, error) {
	return []forum.Board{{ID: 10, Key: "技术", Name: "技术"}}, nil
}
func (*resumeBackend) Post(_ context.Context, id int) (forum.Post, error) {
	return forum.Post{ID: id, Title: "历史串", BoardID: 10, Body: "主楼"}, nil
}
func (c *resumeBackend) List(_ context.Context, kind string, id, page int) (forum.Page, error) {
	if kind != "thread" {
		return forum.Page{Page: page, Count: -1, HasMore: true, List: []forum.Post{{ID: 100, BoardID: 10, Title: "历史串", Body: "主楼"}}}, nil
	}
	c.reads = append(c.reads, page)
	anchor := 111
	if c.missing {
		anchor = 222
	}
	return forum.Page{Page: page, Count: -1, HasMore: true, List: []forum.Post{
		{ID: 110, FollowID: id, Body: strings.Repeat("前一楼正文\n", 30)},
		{ID: anchor, FollowID: id, Body: strings.Repeat("目标楼正文\n", 40)},
	}}, nil
}

func persistenceModel(t *testing.T, dir, site string) (model, *resumeBackend) {
	t.Helper()
	s, err := local.NewSite(dir, site, "https://fixture.test/", "", "file")
	if err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.opts = Options{Site: site, DataDir: dir, ForumURL: "https://fixture.test/", Backend: "file"}
	m.store = s
	m.threads = nil
	m.refilter()
	m.fullscreen = true
	c := &resumeBackend{}
	m.client = c
	m.loadPersistence()
	return m, c
}

func drainMain(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i > 8 {
			t.Fatal("unbounded restore commands")
		}
		msg := cmd()
		if _, ok := msg.(resultMsg); !ok {
			t.Fatalf("unexpected command result %T", msg)
		}
		next, nextCmd, handled := m.extendedUpdate(msg)
		if !handled {
			t.Fatal("restore message unhandled")
		}
		m, cmd = next.(model), nextCmd
	}
	return m
}

func TestRestartStartsTimelineAndHistoryStillResumes(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			dir := t.TempDir()
			m, c := persistenceModel(t, dir, site)
			m = drainMain(t, m, m.initialLoad())
			m.kind, m.board = "board", 1
			p, _ := c.List(context.Background(), "board", 10, 2)
			m.applyPage(p)
			root, _ := c.Post(context.Background(), 100)
			p, _ = c.List(context.Background(), "thread", 100, 3)
			m.applyThread(threadResult{Root: root, Page: p})
			m.movePost(1)
			m.reader.SetYOffset(m.reader.YOffset() + 4)
			want := m.navigation()
			m.flushPersistence()
			if want.AnchorID != 111 || want.AnchorFraction <= 0 {
				t.Fatalf("bad source anchor: %+v", want)
			}
			next, backend := persistenceModel(t, dir, site)
			next = drainMain(t, next, next.initialLoad())
			got := next.navigation()
			if next.reading || next.kind != "timeline" || next.board != 0 || got.Page != 1 || got.SelectedID != 100 || len(backend.reads) != 0 {
				t.Fatalf("restart resumed old session: %+v reads=%v", got, backend.reads)
			}
			next.openHistory("100")
			next = drainMain(t, next, next.selectMenu())
			got = next.navigation()
			if !next.reading || next.kind != "board" || next.board != 1 || got.ReadPage != 3 || got.AnchorID != 111 || got.Page != 2 {
				t.Fatalf("explicit history lost navigation: %+v %s", got, next.notice)
			}
			if len(backend.reads) != 1 || backend.reads[0] != 3 {
				t.Fatalf("history scanned thread: %v", backend.reads)
			}
			if next.pendingRestore != nil {
				t.Fatal("restore never completed")
			}
			next.flushPersistence()
			b, err := next.store.Browsing("")
			if err != nil || len(b.History) != 1 || b.History[0].ThreadID != 100 {
				t.Fatal("history missing or duplicated")
			}
			next.openHistory("不存在")
			if len(next.menu) != 2 {
				t.Fatal("history filter did not work")
			}
			next.openHistory("100")
			if next.menu[0].Action != "history" {
				t.Fatal("history not reachable")
			}
		})
	}
}

func TestRestoreOnlyChecksNeighborPagesForMissingAnchor(t *testing.T) {
	m, c := persistenceModel(t, t.TempDir(), "x")
	c.missing = true
	n := local.Navigation{Kind: "timeline", Page: 1, ThreadID: 100, ReadPage: 5, AnchorID: 111}
	if err := m.store.SaveBrowsing("", n, nil); err != nil {
		t.Fatal(err)
	}
	m.loadPersistence()
	m = drainMain(t, m, m.initialLoad())
	m = drainMain(t, m, m.startRestore(n))
	if fmt.Sprint(c.reads) != "[5 4 6]" || !strings.Contains(m.notice, "原楼层不在附近页") {
		t.Fatalf("unbounded or silent restore: %v %s", c.reads, m.notice)
	}
}

func TestPreviewAndStaleTimersDoNotWriteHistory(t *testing.T) {
	dir := t.TempDir()
	m, _ := persistenceModel(t, dir, "x")
	m = drainMain(t, m, m.initialLoad())
	m.flushPersistence()
	b, _ := m.store.Browsing("")
	if len(b.History) != 0 {
		t.Fatal("list preview entered history")
	}
	m.stateTickID = 12345
	next, _ := persistenceModel(t, dir, "bog")
	updated, _ := next.Update(persistenceTick{12345})
	next = updated.(model)
	if _, err := os.Stat(filepath.Join(next.store.Dir, "browsing.json")); !os.IsNotExist(err) {
		t.Fatal("old site timer wrote a new site file")
	}
}

func TestComposerAutosaveAndRevisionRestore(t *testing.T) {
	m, _ := persistenceModel(t, t.TempDir(), "islander")
	m.identity = local.Cookie{Alias: "daily"}
	m.draft = forum.Draft{Cookie: "daily", BoardID: 1, Body: "first version"}
	m.editDraft()
	if err := m.saveDraft(); err != nil {
		t.Fatal(err)
	}
	id := m.draft.ID
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(model)
	if m.draftTickID == 0 {
		t.Fatal("typing did not schedule autosave")
	}
	updated, _ = m.Update(draftSaveTick{m.draftTickID})
	m = updated.(model)
	ds, err := m.store.Drafts("daily")
	if err != nil || len(ds) != 1 || ds[0].Body == "first version" {
		t.Fatal("latest edit not saved")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	m = updated.(model)
	if m.modal != "menu" || len(m.menu) < 2 {
		t.Fatalf("edit history unavailable: %s %s", m.modal, m.stateError)
	}
	m.menuIndex = len(m.menu) - 1
	m.selectMenu()
	if m.modal != "compose" || m.editor.Value() != "first version" || m.draft.ID != id {
		t.Fatal("revision not restored to same draft")
	}
	m.modal = ""
	stale := m.draftTickID
	if err := m.store.DeleteDraft(id); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(draftSaveTick{stale})
	m = updated.(model)
	ds, _ = m.store.Drafts("daily")
	if len(ds) != 0 {
		t.Fatal("late timer resurrected deleted draft")
	}
}

func TestClearHistoryDoesNotDeleteDraft(t *testing.T) {
	m, _ := persistenceModel(t, t.TempDir(), "islander")
	m.identity = local.Cookie{Alias: "daily"}
	m.draft = forum.Draft{Cookie: "daily", BoardID: 1, Body: "keep this"}
	if err := m.store.SaveDraft(&m.draft); err != nil {
		t.Fatal(err)
	}
	n := local.Navigation{Kind: "timeline", Page: 1, ThreadID: 100}
	if err := m.store.SaveBrowsing("daily", n, &local.HistoryEntry{Navigation: n, VisitedAt: time.Now().UnixNano()}); err != nil {
		t.Fatal(err)
	}
	m.openHistory("")
	m.menuIndex = len(m.menu) - 1
	m.selectMenu()
	if m.modal != "confirm" {
		t.Fatal("clear has no confirmation")
	}
	m.confirm()
	b, _ := m.store.Browsing("daily")
	ds, _ := m.store.Drafts("daily")
	if len(b.History) != 0 || len(ds) != 1 {
		t.Fatal("clear did not isolate history from drafts")
	}
}

func TestBlurFlushesComposerBeforeDebounce(t *testing.T) {
	m, _ := persistenceModel(t, t.TempDir(), "islander")
	m.identity = local.Cookie{Alias: "daily"}
	m.draft = forum.Draft{Cookie: "daily", BoardID: 1, Body: "unsaved"}
	m.editDraft()
	next, _ := m.Update(tea.BlurMsg{})
	m = next.(model)
	ds, err := m.store.Drafts("daily")
	if err != nil || len(ds) != 1 || ds[0].Body != "unsaved" {
		t.Fatal("focus loss did not flush draft")
	}
	if !m.View().ReportFocus {
		t.Fatal("terminal focus reporting not enabled")
	}
}

func TestReopenLongThreadFromListAndFavorites(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m, c := persistenceModel(t, t.TempDir(), site)
			m = drainMain(t, m, m.initialLoad())
			m = drainMain(t, m, m.loadThread(100, 42, 0))
			m.movePost(1)
			m.reader.SetYOffset(m.reader.YOffset() + 3)
			m = press(m, "*") // Must flush the current location before favoriting.
			if !m.isFavorite(100) {
				t.Fatal("favorite shortcut did not save thread")
			}
			m = press(m, "esc")
			if m.reading {
				t.Fatal("did not return to list")
			}
			// A cached first page must not replace the retained page 42.
			m.selectedThreads = map[int]selectionResult{100: {Page: forum.Page{Page: 1}}}
			c.reads = nil
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = applyCommand(next.(model), cmd)
			if n := m.navigation(); n.ReadPage != 42 || n.AnchorID != 111 || n.AnchorFraction <= 0 || len(c.reads) != 0 {
				t.Fatalf("list reopening lost position or scanned thread: %+v %v", n, c.reads)
			}
			m = press(m, "F")
			if len(m.menu) != 1 || m.menu[0].Action != "favorite" || !strings.Contains(m.menu[0].Label, "42") {
				t.Fatal("favorite entry lacks saved page")
			}
			m = press(m, "/")
			if m.modal != "favorites-filter" || !strings.Contains(m.View().Content, "筛选收藏") {
				t.Fatal("favorite filter unavailable")
			}
			m.input.SetValue("不存在")
			m = press(m, "enter")
			if len(m.menu) != 0 {
				t.Fatal("favorite filter not applied")
			}
			m.openFavorites("100")
			c.reads = nil
			m = drainMain(t, m, m.selectMenu())
			if n := m.navigation(); n.ReadPage != 42 || n.AnchorID != 111 || fmt.Sprint(c.reads) != "[42]" {
				t.Fatalf("favorite reopening lost position: %+v %v", n, c.reads)
			}
			m = press(m, "F")
			m = press(m, "x")
			if m.modal != "confirm" || m.confirmAction != "remove-favorite" {
				t.Fatal("remove favorite not confirmed")
			}
			m = press(m, "enter")
			if m.isFavorite(100) || len(m.menu) != 0 {
				t.Fatal("favorite still listed after removal")
			}
			b, err := m.store.Browsing("")
			if err != nil || len(b.History) != 1 {
				t.Fatal("unfavoriting removed history")
			}
		})
	}
}

func TestHomepageLoadsFirstThreadPageInBothLayouts(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		for _, agent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/agent=%v", site, agent), func(t *testing.T) {
				dir := t.TempDir()
				m, c := persistenceModel(t, dir, site)
				for _, id := range []int{100, 999} {
					n := local.Navigation{Kind: "board", BoardID: 10, Page: 7, SelectedID: id, ThreadID: id, ReadPage: 42, AnchorID: 111, Filter: "旧筛选", Newest: true}
					entry := &local.HistoryEntry{Navigation: n, Title: fmt.Sprint(id), VisitedAt: time.Now().UnixNano()}
					if err := m.store.SaveBrowsing("", n, entry); err != nil {
						t.Fatal(err)
					}
				}
				if err := local.RememberAgentSimulation(dir, agent); err != nil {
					t.Fatal(err)
				}
				m.loadPersistence()
				m.loadUIPreferences()
				m.fullscreen = false
				m.resize(120, 36)
				m = drainMain(t, m, m.initialLoad())
				if m.chatStyle != agent || m.kind != "timeline" || m.board != 0 || m.page != 1 || m.selected != 0 || m.current().id != 100 || m.filter != "" || m.newest {
					t.Fatal("homepage restored old list instead of first timeline item")
				}
				if m.reading {
					t.Fatal("homepage entered reading before user selected a thread")
				}
				tick := m.prepareSelection()
				if tick == nil {
					t.Fatal("homepage did not schedule first thread")
				}
				next, fetch := m.Update(tick())
				m = applyCommand(next.(model), fetch)
				if m.reading || m.selectedThreads[100].Resume != nil || m.selectedThreads[100].Page.Page != 1 {
					t.Fatal("homepage prefetch changed focus or resumed old page")
				}
				m, _ = clickAt(m, 6, m.listContentTop(), tea.MouseLeft)
				if !m.reading || m.current().id != 100 || m.pages[100].Page != 1 || m.busy {
					t.Fatal("homepage click did not enter cached first page")
				}
				if fmt.Sprint(c.reads) != "[1]" {
					t.Fatalf("homepage made unnecessary thread reads: %v", c.reads)
				}
				saved, err := m.store.Browsing("")
				if err != nil || len(saved.History) != 2 {
					t.Fatal("homepage deleted saved history")
				}
			})
		}
	}
}

func TestAgentAppearancePersistsAndCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	m, _ := persistenceModel(t, dir, "x")
	m.toggleChatStyle()
	saved, err := local.ReadPreferences(dir)
	if err != nil || !saved.AgentSimulation {
		t.Fatal("mode toggle did not persist")
	}
	next, _ := persistenceModel(t, dir, "x")
	next.loadUIPreferences()
	if !next.chatStyle {
		t.Fatal("restart lost Agent appearance")
	}
	next.toggleChatStyle()
	last, _ := persistenceModel(t, dir, "x")
	last.loadUIPreferences()
	if last.chatStyle {
		t.Fatal("forum appearance was not saved")
	}
}
