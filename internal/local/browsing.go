package local

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/gofrs/flock"
)

type Preferences struct {
	Theme           string                `json:"theme,omitempty"`
	AgentSimulation bool                  `json:"agentSimulation,omitempty"`
	Version         int                   `json:"version"`
	LastSite        forum.Site            `json:"lastSite"`
	Sites           map[string]forum.Site `json:"sites"`
}

func DataRoot(root string) (string, error) {
	if root != "" {
		return root, nil
	}
	base, err := os.UserConfigDir()
	return filepath.Join(base, "islander"), err
}

func readVersioned(path string, dst any) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var header struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(b, &header) != nil || header.Version != 1 {
		return fmt.Errorf("%s 损坏或版本不支持；保留原文件", filepath.Base(path))
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("%s 内容无效；保留原文件", filepath.Base(path))
	}
	return nil
}

func ReadPreferences(root string) (Preferences, error) {
	p := Preferences{Version: 1, Sites: map[string]forum.Site{}}
	root, err := DataRoot(root)
	if err != nil {
		return p, err
	}
	err = readVersioned(filepath.Join(root, "preferences.json"), &p)
	// Preserve the selection saved by the initial terminal-theme prototype.
	if err == nil && p.Theme == "terminal" {
		p.Theme = "el"
	}
	if p.Sites == nil {
		p.Sites = map[string]forum.Site{}
	}
	return p, err
}

func lockedFile(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := flock.New(path + ".lock")
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("本地状态正被其他进程保存，请稍后重试")
	}
	defer lock.Unlock()
	return fn()
}

func RememberSite(root string, site forum.Site) error {
	s, err := forum.Resolve(site.ID, site.ForumURL, site.UserURL)
	if err != nil {
		return err
	}
	for _, endpoint := range []string{s.ForumURL, s.UserURL} {
		u, e := url.Parse(endpoint)
		if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("站点配置不能包含凭证、查询参数或片段")
		}
	}
	root, err = DataRoot(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "preferences.json")
	return lockedFile(path, func() error {
		p, err := ReadPreferences(root)
		if err != nil {
			return err
		}
		p.LastSite, p.Sites[s.ID] = s, s
		return atomic(path, p)
	})
}

// RememberAgentSimulation merges the UI choice without replacing site settings.
func RememberAgentSimulation(root string, enabled bool) error {
	root, err := DataRoot(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "preferences.json")
	return lockedFile(path, func() error {
		p, err := ReadPreferences(root)
		if err != nil {
			return err
		}
		p.AgentSimulation = enabled
		return atomic(path, p)
	})
}

// RememberTheme preserves the last site and Agent layout preference.
func RememberTheme(root, theme string) error {
	if theme != "islander" && theme != "el" {
		return errors.New("无效的主题")
	}
	root, err := DataRoot(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "preferences.json")
	return lockedFile(path, func() error {
		p, err := ReadPreferences(root)
		if err != nil {
			return err
		}
		p.Theme = theme
		return atomic(path, p)
	})
}

// Navigation keeps the list context separate from the thread currently open.
type Navigation struct {
	Kind           string  `json:"kind"`
	BoardID        int     `json:"boardId,omitempty"`
	BoardKey       string  `json:"boardKey,omitempty"`
	Page           int     `json:"page"`
	SelectedID     int     `json:"selectedId,omitempty"`
	Filter         string  `json:"filter,omitempty"`
	Newest         bool    `json:"newest,omitempty"`
	ThreadID       int     `json:"threadId,omitempty"`
	ReadPage       int     `json:"readPage,omitempty"`
	AnchorID       int     `json:"anchorId,omitempty"`
	AnchorFraction float64 `json:"anchorFraction,omitempty"`
}

type HistoryEntry struct {
	Navigation
	Title     string `json:"title"`
	VisitedAt int64  `json:"visitedAt"` // Unix nanoseconds; also fences late saves after deletion.
}

type Browsing struct {
	Session         Navigation     `json:"session"`
	History         []HistoryEntry `json:"history"`
	Favorites       []HistoryEntry `json:"favorites,omitempty"`
	HistoryDisabled bool           `json:"historyDisabled,omitempty"`
	ClearedAt       int64          `json:"clearedAt,omitempty"`
}

type browsingFile struct {
	Version    int                 `json:"version"`
	Identities map[string]Browsing `json:"identities"`
}

func identityKey(alias string) string {
	if alias == "" {
		return "anonymous"
	}
	return "cookie:" + alias
}

func (s *Store) readBrowsing() (browsingFile, error) {
	f := browsingFile{Version: 1, Identities: map[string]Browsing{}}
	err := readVersioned(filepath.Join(s.Dir, "browsing.json"), &f)
	if f.Identities == nil {
		f.Identities = map[string]Browsing{}
	}
	return f, err
}

func pruneHistory(b *Browsing) {
	cutoff := time.Now().Add(-90 * 24 * time.Hour).UnixNano()
	out := make([]HistoryEntry, 0, len(b.History))
	for _, e := range b.History {
		if e.VisitedAt >= cutoff && e.ThreadID > 0 {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].VisitedAt > out[j].VisitedAt })
	if len(out) > 500 {
		out = out[:500]
	}
	b.History = out
}

func (s *Store) Browsing(alias string) (Browsing, error) {
	f, err := s.readBrowsing()
	b := f.Identities[identityKey(alias)]
	pruneHistory(&b)
	return b, err
}

func (s *Store) changeBrowsing(alias string, fn func(*Browsing)) error {
	path := filepath.Join(s.Dir, "browsing.json")
	return lockedFile(path, func() error {
		f, err := s.readBrowsing()
		if err != nil {
			return err
		}
		key := identityKey(alias)
		b := f.Identities[key]
		fn(&b)
		pruneHistory(&b)
		f.Identities[key] = b
		return atomic(path, f)
	})
}

func (s *Store) SaveBrowsing(alias string, n Navigation, entry *HistoryEntry) error {
	return s.changeBrowsing(alias, func(b *Browsing) {
		if b.HistoryDisabled || (entry != nil && entry.VisitedAt <= b.ClearedAt) {
			n.ThreadID, n.ReadPage, n.AnchorID, n.SelectedID, n.AnchorFraction = 0, 0, 0, 0, 0
		}
		b.Session = n
		if entry == nil || b.HistoryDisabled || entry.VisitedAt <= b.ClearedAt {
			return
		}
		e := *entry
		title := []rune(forum.Clean(e.Title))
		e.Title = string(title[:min(160, len(title))])
		for i, old := range b.Favorites {
			if old.ThreadID == e.ThreadID && old.VisitedAt <= e.VisitedAt {
				b.Favorites[i] = e
			}
		}
		for i, old := range b.History {
			if old.ThreadID == e.ThreadID {
				if old.VisitedAt <= e.VisitedAt {
					b.History[i] = e
				}
				return
			}
		}
		b.History = append(b.History, e)
	})
}

// Advancing the fence also prevents an already-open reader from immediately
// recreating a deleted entry. A fresh explicit thread read starts a new visit.
func (s *Store) DeleteHistory(alias string, id int) error {
	return s.changeBrowsing(alias, func(b *Browsing) {
		b.ClearedAt = time.Now().UnixNano()
		out := b.History[:0]
		for _, e := range b.History {
			if id != 0 && e.ThreadID != id {
				out = append(out, e)
			}
		}
		b.History = out
		for i := range b.Favorites {
			e := &b.Favorites[i]
			if id == 0 || e.ThreadID == id {
				e.ReadPage, e.AnchorID, e.AnchorFraction, e.VisitedAt = 1, 0, 0, 0
			}
		}
		if id == 0 || b.Session.ThreadID == id {
			b.Session.ThreadID, b.Session.ReadPage, b.Session.AnchorID = 0, 0, 0
			b.Session.AnchorFraction = 0
		}
	})
}

func (s *Store) SetHistoryDisabled(alias string, disabled bool) error {
	return s.changeBrowsing(alias, func(b *Browsing) {
		b.HistoryDisabled = disabled
		b.ClearedAt = time.Now().UnixNano()
		if disabled {
			b.Session.ThreadID, b.Session.ReadPage, b.Session.AnchorID, b.Session.SelectedID = 0, 0, 0, 0
			b.Session.AnchorFraction = 0
		}
	})
}

// Favorites are explicit user choices and do not expire with browsing history.
// Their reading positions follow the same pause and deletion rules as history.
func (s *Store) ToggleFavorite(alias string, entry HistoryEntry) (bool, error) {
	if entry.ThreadID <= 0 {
		return false, errors.New("无效的收藏串号")
	}
	added := false
	err := s.changeBrowsing(alias, func(b *Browsing) {
		for i, old := range b.Favorites {
			if old.ThreadID == entry.ThreadID {
				b.Favorites = append(b.Favorites[:i], b.Favorites[i+1:]...)
				return
			}
		}
		title := []rune(forum.Clean(entry.Title))
		entry.Title = string(title[:min(160, len(title))])
		// Favoriting a list preview must not invent or replace a reading position.
		entry.ReadPage, entry.AnchorID, entry.AnchorFraction, entry.VisitedAt = 1, 0, 0, 0
		for _, old := range b.History {
			if old.ThreadID == entry.ThreadID {
				entry.ReadPage, entry.AnchorID, entry.AnchorFraction, entry.VisitedAt = old.ReadPage, old.AnchorID, old.AnchorFraction, old.VisitedAt
				break
			}
		}
		b.Favorites = append([]HistoryEntry{entry}, b.Favorites...)
		added = true
	})
	return added, err
}

func (s *Store) RemoveFavorite(alias string, id int) error {
	return s.changeBrowsing(alias, func(b *Browsing) {
		for i, e := range b.Favorites {
			if e.ThreadID == id {
				b.Favorites = append(b.Favorites[:i], b.Favorites[i+1:]...)
				return
			}
		}
	})
}

func (b Browsing) ReadingPosition(id int) (Navigation, bool) {
	var newest HistoryEntry
	for _, entries := range [][]HistoryEntry{b.History, b.Favorites} {
		for _, e := range entries {
			if e.ThreadID == id && e.VisitedAt > newest.VisitedAt {
				newest = e
			}
		}
	}
	return newest.Navigation, newest.ThreadID > 0
}
