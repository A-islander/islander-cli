package tui

import (
	"strings"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/charmbracelet/x/ansi"
)

func TestComposeWithoutCookieExplainsInDialog(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		for _, key := range []string{"c", "r", "R"} {
			for _, size := range [][2]int{{120, 36}, {44, 16}} {
				m := newModel()
				m.opts = Options{Site: site, ForumURL: "https://fixture.test/"}
				m.store, _ = local.NewSite(t.TempDir(), site, m.opts.ForumURL, "", "file")
				m.client, _ = forum.NewBackend(site, m.opts.ForumURL, "", "")
				root := forum.Post{ID: 100, Body: "root"}
				m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, List: []forum.Post{root}}})
				m.resize(size[0], size[1])
				if key == "c" {
					m.reading = false
					if !strings.Contains(ansi.Strip(m.View().Content), "c 发串") {
						t.Fatalf("%s lacks c hint", site)
					}
				}
				m = press(m, key)
				if m.modal != "menu" {
					t.Fatalf("%s/%s did not open cookie menu", site, key)
				}
				view := ansi.Strip(m.dialog())
				for _, want := range []string{"没有饼干，无法发串/回复串", "请先导入或选择当前岛的饼干", "导入饼干", "Enter 打开"} {
					if !strings.Contains(view, want) {
						t.Fatalf("%s/%s/%v missing %s in dialog: %s", site, key, size, want, view)
					}
				}
				m = press(m, "esc")
				m = press(m, "i")
				if strings.Contains(ansi.Strip(m.dialog()), "无法发串") {
					t.Fatal("ordinary cookie management retained compose warning")
				}
			}
		}
	}
}
