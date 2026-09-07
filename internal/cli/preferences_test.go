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
	}{
		{[]string{"tui"}, "bog", "https://custom-bog.test/"},
		{[]string{"--site", "x", "tui"}, "x", "https://api.nmb.best/api/"},
		{[]string{"tui", "--site", "islander"}, "islander", forum.ForumURL},
		{[]string{"tui", "--forum-url", "https://custom-islander.test/"}, "islander", "https://custom-islander.test/"},
		{[]string{"tui", "--demo"}, "islander", forum.ForumURL},
		{[]string{"site", "info"}, "islander", forum.ForumURL},
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
			root.SetArgs(append([]string{"--data-dir", dir}, test.args...))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if a.site != test.site || a.f != test.url {
				t.Fatalf("got %s %s", a.site, a.f)
			}
		})
	}
}
