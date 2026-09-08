package cli

import (
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/spf13/cobra"
)

func TestRememberedSiteOnlyAffectsUnqualifiedTUI(t *testing.T) {
	dir := t.TempDir()
	if err := local.RememberSite(dir, forum.Site{ID: "bog", ForumURL: "https://custom-bog.test/"}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args      []string
		site, url string
		firstRun  bool
	}{
		{[]string{"tui"}, "bog", "https://custom-bog.test/", false},
		{[]string{"--site", "x", "tui"}, "x", "https://api.nmb.best/api/", false},
		{[]string{"tui", "--site", "islander"}, "islander", forum.ForumURL, false},
		{[]string{"tui", "--forum-url", "https://custom-islander.test/"}, "islander", "https://custom-islander.test/", false},
		{[]string{"tui", "--demo"}, "islander", forum.ForumURL, false},
		{[]string{"site", "info"}, "islander", forum.ForumURL, false},
		{[]string{"tui"}, "x", "https://api.nmb.best/api/", true},
		{[]string{"tui", "--site", "islander"}, "islander", forum.ForumURL, true},
		{[]string{"site", "info"}, "islander", forum.ForumURL, true},
	} {
		t.Run(test.args[0]+test.site, func(t *testing.T) {
			a := &app{}
			root := a.root()
			// Retain Cobra's real flag parsing and pre-run, skip terminal/network IO.
			var stub func(*cobra.Command)
			stub = func(c *cobra.Command) {
				if c.RunE != nil {
					c.RunE = func(*cobra.Command, []string) error { return nil }
				}
				for _, child := range c.Commands() {
					stub(child)
				}
			}
			stub(root)
			dataDir := dir
			if test.firstRun {
				dataDir = t.TempDir()
			}
			root.SetArgs(append([]string{"--data-dir", dataDir}, test.args...))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if a.site != test.site || a.f != test.url {
				t.Fatalf("got %s %s", a.site, a.f)
			}
		})
	}
}
