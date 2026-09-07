package cli

import (
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/spf13/cobra"
)

func (a *app) restoreTUISite(cmd *cobra.Command) {
	if cmd.Name() != "tui" || a.demo || cmd.Flags().Changed("site") || cmd.Flags().Changed("forum-url") || cmd.Flags().Changed("user-url") {
		return
	}
	p, err := local.ReadPreferences(a.dir)
	if err != nil {
		a.stateWarning = err.Error()
		return
	}
	if p.LastSite.ID == "" {
		return
	}
	s, err := forum.Resolve(p.LastSite.ID, p.LastSite.ForumURL, p.LastSite.UserURL)
	if err != nil {
		a.stateWarning = "上次站点配置不可用，已回到岛民岛"
		return
	}
	a.site, a.f, a.u = s.ID, s.ForumURL, s.UserURL
}
