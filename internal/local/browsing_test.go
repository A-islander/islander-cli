package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/A-islander/islander-cli/internal/forum"
)

func TestPreferencesMergeAndPreserveCorruption(t *testing.T) {
	dir := t.TempDir()
	for _, s := range forum.Sites() {
		if err := RememberSite(dir, s); err != nil {
			t.Fatal(err)
		}
	}
	p, err := ReadPreferences(dir)
	if err != nil || p.LastSite.ID != "bog" || len(p.Sites) != 3 {
		t.Fatalf("preferences: %+v %v", p, err)
	}
	path := filepath.Join(dir, "preferences.json")
	broken := []byte(`{"version":9}`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RememberSite(dir, forum.Sites()[0]); err == nil {
		t.Fatal("overwrote unknown schema")
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(broken) {
		t.Fatal("corrupt source lost")
	}
}

func TestBrowsingIsolationAndDeleteFence(t *testing.T) {
	dir := t.TempDir()
	x, _ := NewSite(dir, "x", "https://example.test/", "", "file")
	bog, _ := NewSite(dir, "bog", "https://example.test/", "", "file")
	n := Navigation{Kind: "board", BoardID: 30, Page: 2, ThreadID: 100, ReadPage: 3, AnchorID: 110}
	e := HistoryEntry{Navigation: n, Title: "private title", VisitedAt: time.Now().UnixNano()}
	if err := x.SaveBrowsing("daily", n, &e); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Store{x, bog} {
		b, err := s.Browsing("")
		if err != nil || len(b.History) != 0 {
			t.Fatal("guest saw another identity")
		}
	}
	b, _ := bog.Browsing("daily")
	if len(b.History) != 0 {
		t.Fatal("same IDs crossed sites")
	}
	if err := x.SaveBrowsing("anonymous", Navigation{Kind: "timeline", Page: 7}, nil); err != nil {
		t.Fatal(err)
	}
	if err := x.SaveBrowsing("", Navigation{Kind: "timeline", Page: 8}, nil); err != nil {
		t.Fatal(err)
	}
	b, _ = x.Browsing("anonymous")
	if b.Session.Page != 7 {
		t.Fatal("alias collided with anonymous identity")
	}
	if err := x.DeleteHistory("daily", 100); err != nil {
		t.Fatal(err)
	}
	if err := x.SaveBrowsing("daily", n, &e); err != nil {
		t.Fatal(err)
	}
	b, _ = x.Browsing("daily")
	if len(b.History) != 0 || b.Session.ThreadID != 0 {
		t.Fatal("late save resurrected history or position")
	}
	e.VisitedAt = time.Now().UnixNano() + 1
	if err := x.SaveBrowsing("daily", n, &e); err != nil {
		t.Fatal(err)
	}
	b, _ = x.Browsing("daily")
	if len(b.History) != 1 {
		t.Fatal("fresh visit cannot be recorded")
	}
	if err := x.SetHistoryDisabled("daily", true); err != nil {
		t.Fatal(err)
	}
	e.ThreadID = 200
	e.VisitedAt = time.Now().UnixNano() + 1
	if err := x.SaveBrowsing("daily", n, &e); err != nil {
		t.Fatal(err)
	}
	b, _ = x.Browsing("daily")
	if len(b.History) != 1 || b.Session.ThreadID != 0 {
		t.Fatal("disabled history still recorded")
	}
}

func TestHistoryRetentionAndCorruptFile(t *testing.T) {
	s, _ := New(t.TempDir(), "https://a.test/", "https://b.test/", "file")
	if err := s.changeBrowsing("", func(b *Browsing) {
		for i := 1; i <= 600; i++ {
			b.History = append(b.History, HistoryEntry{Navigation: Navigation{ThreadID: i}, VisitedAt: time.Now().UnixNano() + int64(i)})
		}
		b.History = append(b.History, HistoryEntry{Navigation: Navigation{ThreadID: 9999}, VisitedAt: time.Now().Add(-91 * 24 * time.Hour).UnixNano()})
	}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Browsing("")
	if err != nil || len(b.History) != 500 || b.History[0].ThreadID != 600 {
		t.Fatal("retention failed")
	}
	path := filepath.Join(s.Dir, "browsing.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBrowsing("", Navigation{}, nil); err == nil {
		t.Fatal("corrupt history overwritten")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "broken" {
		t.Fatal("corrupt history not preserved")
	}
}

func TestDraftHistoryAtomicCompatibilityAndBounds(t *testing.T) {
	s, _ := New(t.TempDir(), "https://a.test/", "https://b.test/", "file")
	d := forum.Draft{ID: "legacy", Cookie: "daily", BoardID: 1, Body: "old draft"}
	if err := atomic(filepath.Join(s.Dir, "draft-legacy.json"), d); err != nil {
		t.Fatal(err)
	}
	d.Body = "autosaved current"
	if err := s.AutoSaveDraft(&d); err != nil {
		t.Fatal(err)
	}
	ds, err := s.Drafts("daily")
	if err != nil || len(ds) != 1 || ds[0].Body != d.Body {
		t.Fatal("legacy draft cannot be read")
	}
	for i := 0; i < 25; i++ {
		d.Body = string(rune('A' + i))
		if err := s.SaveDraft(&d); err != nil {
			t.Fatal(err)
		}
	}
	edits, err := s.DraftEdits(d.ID, "daily")
	if err != nil || len(edits) != 20 {
		t.Fatalf("edits=%d err=%v", len(edits), err)
	}
	if err := s.SaveDraft(&d); err != nil {
		t.Fatal(err)
	}
	again, _ := s.DraftEdits(d.ID, "daily")
	if len(again) != len(edits) {
		t.Fatal("duplicate snapshot")
	}
	if _, err := s.DraftEdits(d.ID, "other"); err == nil {
		t.Fatal("edits cross identity")
	}
	d.Cookie = "other"
	if err := s.SaveDraft(&d); err == nil {
		t.Fatal("overwrote another identity")
	}
	d.Cookie = "daily"
	raw, _ := os.ReadFile(filepath.Join(s.Dir, "draft-legacy.json"))
	var legacy forum.Draft
	if json.Unmarshal(raw, &legacy) != nil || legacy.Body != d.Body {
		t.Fatal("old format reader broken")
	}
	d.Sent = true
	if err := s.SaveDraft(&d); err != nil {
		t.Fatal(err)
	}
	d.Sent = false
	if err := s.AutoSaveDraft(&d); err == nil {
		t.Fatal("late autosave revived sent draft")
	}
	if err := s.DeleteDraft(d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DraftEdits(d.ID, "daily"); err == nil {
		t.Fatal("deleted draft retained revisions")
	}
}

func TestFavoritesSurviveHistoryRetentionAndRespectDeletion(t *testing.T) {
	s, _ := NewSite(t.TempDir(), "x", "https://fixture.test/", "", "file")
	e := HistoryEntry{Navigation: Navigation{Kind: "timeline", Page: 1, ThreadID: 100, ReadPage: 42, AnchorID: 900}, Title: "长串", VisitedAt: time.Now().UnixNano()}
	if err := s.SaveBrowsing("daily", e.Navigation, &e); err != nil {
		t.Fatal(err)
	}
	if added, err := s.ToggleFavorite("daily", e); err != nil || !added {
		t.Fatalf("favorite: %v %v", added, err)
	}
	// A later read updates both the history and the durable bookmark position.
	e.ReadPage, e.AnchorID, e.VisitedAt = 43, 919, time.Now().UnixNano()
	if err := s.SaveBrowsing("daily", e.Navigation, &e); err != nil {
		t.Fatal(err)
	}
	if err := s.changeBrowsing("daily", func(b *Browsing) {
		b.History[0].VisitedAt = time.Now().Add(-91 * 24 * time.Hour).UnixNano()
	}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Browsing("daily")
	n, found := b.ReadingPosition(100)
	if err != nil || len(b.History) != 0 || len(b.Favorites) != 1 || !found || n.ReadPage != 43 || n.AnchorID != 919 {
		t.Fatalf("favorite lost expired history's position: %+v %v", b, err)
	}
	if err := s.SetHistoryDisabled("daily", true); err != nil {
		t.Fatal(err)
	}
	e.ReadPage, e.VisitedAt = 44, time.Now().UnixNano()
	if err := s.SaveBrowsing("daily", e.Navigation, &e); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Browsing("daily")
	if b.Favorites[0].ReadPage != 43 {
		t.Fatal("paused history still updates favorite position")
	}
	if err := s.DeleteHistory("daily", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBrowsing("daily", e.Navigation, &e); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Browsing("daily")
	if _, found := b.ReadingPosition(100); len(b.Favorites) != 1 || found || b.Favorites[0].AnchorID != 0 {
		t.Fatal("clear must retain favorite, but clear its reading position")
	}
	if err := s.RemoveFavorite("daily", 100); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBrowsing("daily", e.Navigation, &e); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Browsing("daily")
	if len(b.Favorites) != 0 {
		t.Fatal("late save recreated a removed favorite")
	}
}

func TestFavoritesAreScopedAndDoNotCreateHistory(t *testing.T) {
	dir := t.TempDir()
	e := HistoryEntry{Navigation: Navigation{ThreadID: 100, ReadPage: 99, AnchorID: 999}, Title: "preview"}
	x, _ := NewSite(dir, "x", "https://fixture.test/", "", "file")
	if _, err := x.ToggleFavorite("daily", e); err != nil {
		t.Fatal(err)
	}
	for _, site := range []string{"islander", "x", "bog"} {
		s, _ := NewSite(dir, site, "https://fixture.test/", "", "file")
		for _, alias := range []string{"", "daily", "other"} {
			b, err := s.Browsing(alias)
			want := 0
			if site == "x" && alias == "daily" {
				want = 1
			}
			if err != nil || len(b.Favorites) != want || len(b.History) != 0 {
				t.Fatalf("favorite leaked or created history: %s/%s %+v %v", site, alias, b, err)
			}
			if _, found := b.ReadingPosition(100); found {
				t.Fatal("unread preview invented a reading position")
			}
		}
	}
	other, _ := NewSite(dir, "x", "https://other.test/", "", "file")
	b, _ := other.Browsing("daily")
	if len(b.Favorites) != 0 {
		t.Fatal("favorites crossed endpoints")
	}
	if added, err := x.ToggleFavorite("daily", e); err != nil || added {
		t.Fatal("toggle did not remove favorite")
	}
}
