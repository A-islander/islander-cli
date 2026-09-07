package local

import (
	"github.com/A-islander/islander-cli/internal/forum"
	"testing"
)

func TestSiteScopesPreserveLegacyAndSeparateAliasesDrafts(t *testing.T) {
	dir := t.TempDir()
	legacy, err := New(dir, forum.ForumURL, forum.UserURL, "file")
	if err != nil {
		t.Fatal(err)
	}
	is, err := NewSite(dir, "islander", forum.ForumURL, forum.UserURL, "file")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Dir != is.Dir || legacy.Scope != is.Scope {
		t.Fatal("old data scope changed")
	}
	x, err := NewSite(dir, "x", forum.ForumURL, forum.UserURL, "file")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSite(dir, "bog", forum.ForumURL, forum.UserURL, "file")
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range []*Store{is, x, b} {
		if err = s.Import("daily", []string{"is-cookie", "x-cookie", "bog-cookie"}[i], forum.User{Key: "opaque", Name: "local alias"}); err != nil {
			t.Fatal(err)
		}
	}
	for i, s := range []*Store{legacy, x, b} {
		who, token, err := s.Identity("daily")
		if err != nil || who.ID != 0 || token != []string{"is-cookie", "x-cookie", "bog-cookie"}[i] {
			t.Fatal("identity scope collision")
		}
	}
	draft := forum.Draft{Cookie: "daily", Body: "draft", BoardID: 1}
	if err = is.SaveDraft(&draft); err != nil {
		t.Fatal(err)
	}
	if ds, err := x.Drafts("daily"); err != nil || len(ds) != 0 {
		t.Fatal("cross-site draft visible")
	}
	custom, _ := NewSite(dir, "x", "https://custom.example/", "", "file")
	if custom.Scope == x.Scope {
		t.Fatal("custom endpoint shares credentials")
	}
}
