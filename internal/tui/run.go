package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"github.com/A-islander/islander-cli/internal/media"
	"github.com/charmbracelet/x/term"
	"os"
	"time"
)

type Options struct {
	ForumURL, UserURL, Images, Cookie string
	Site, DataDir, Backend            string
	StateWarning                      string
	Store                             *local.Store
	Demo                              bool
}

func Run(o Options) error {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("TUI 需要交互终端；程序读取请使用 --output json 命令")
	}
	m := newModel()
	m.opts = o
	m.store = o.Store
	m.stateError = o.StateWarning
	if !o.Demo {
		m.boardNames = []string{"全部"}
		m.threads = nil
		m.refilter()
		m.notice = "正在连接论坛…"
		var err error
		m.client, err = forum.NewBackend(o.Site, o.ForumURL, o.UserURL, "")
		if err != nil {
			return err
		}
		m.busy = true
		if err := m.setIdentity(o.Cookie); err != nil {
			m.notice = err.Error()
		}
		if p, err := local.ReadPreferences(o.DataDir); err == nil {
			m.siteConfigs = p.Sites
		}
		m.rememberSite()
	}
	terminal, restoreTerminal := prepareImageTerminal(o.Images)
	defer restoreTerminal()
	m.imageTerminal = terminal
	final, err := tea.NewProgram(m).Run()
	if last, ok := final.(model); ok {
		cleanup := last.clearInlineImages()
		if last.attachment.cancel != nil {
			last.attachment.cancel()
		}
		if last.attachment.sent {
			cleanup += media.KittyDelete(last.attachment.id)
		}
		if cleanup != "" {
			_, _ = fmt.Fprint(os.Stdout, media.KittyTransport(cleanup, terminal.tmux))
			if terminal.tmux {
				// tmux consumes PTY output asynchronously. Give the final
				// deletes time to pass through before restoring the pane option.
				time.Sleep(100 * time.Millisecond)
			}
		}
		last.flushPersistence()
		if !last.busy && (last.modal == "compose" || last.modal == "filepicker" || last.modal == "kaomoji") {
			if saveErr := last.saveDraft(); saveErr != nil {
				last.stateError = saveErr.Error()
			}
		}
		if last.stateError != "" {
			fmt.Fprintln(os.Stderr, "本地保存失败："+last.stateError)
		}
		if last.opts.StateWarning != "" {
			fmt.Fprintln(os.Stderr, last.opts.StateWarning)
		}
	}
	return err
}
func (m *model) setIdentity(alias string) error {
	who, token, err := m.store.Identity(alias)
	if err != nil {
		return err
	}
	client, err := forum.NewBackend(m.opts.Site, m.opts.ForumURL, m.opts.UserURL, token)
	if err != nil {
		return err
	}
	m.loadedThreadID = 0
	m.selectedThreads = nil
	m.identity, m.client = who, client
	m.loadPersistence()
	return nil
}
