package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"path/filepath"
)

func (m model) siteBranding() (name, wordmark, slogan string) {
	switch m.opts.Site {
	case "x":
		return "X岛", "NMBXD", "人，是会思考的芦苇"
	case "bog":
		return "BOG岛", "BOG", "[xxx]"
	default:
		return "岛民岛", "ISLANDER", "岛民岛的岛是岛民的岛"
	}
}

func (m model) capabilities() forum.Capabilities {
	if m.client != nil {
		return m.client.Capabilities()
	}
	// The offline prototype and existing in-memory views use Islander behavior.
	return (&forum.Client{}).Capabilities()
}
func (m *model) openSites() {
	m.menu = nil
	for _, s := range forum.Sites() {
		label := s.Name
		if s.ID == m.opts.Site || (s.ID == "islander" && m.opts.Site == "") {
			label += " · 当前"
		}
		m.menu = append(m.menu, menuItem{label, "site", s.ID})
	}
	m.menuIndex = 0
	m.modal = "menu"
	m.returnModal = "切换站点"
}
func (m *model) switchSite(id string) tea.Cmd {
	m.flushPersistence()
	if m.siteConfigs == nil {
		m.siteConfigs = map[string]forum.Site{}
	}
	current, err := forum.Resolve(m.opts.Site, m.opts.ForumURL, m.opts.UserURL)
	if err != nil {
		m.notice = err.Error()
		return nil
	}
	m.siteConfigs[current.ID] = current
	s, ok := m.siteConfigs[id]
	if !ok {
		s, err = forum.Resolve(id, "", "")
		if err != nil {
			m.notice = err.Error()
			return nil
		}
	}
	o := m.opts
	o.Site, o.ForumURL, o.UserURL, o.Cookie = id, s.ForumURL, s.UserURL, ""
	if o.DataDir == "" && m.store != nil {
		o.DataDir = filepath.Dir(m.store.Dir)
	}
	if o.Backend == "" {
		if m.store != nil {
			o.Backend = m.store.Backend
		} else {
			o.Backend = "keyring"
		}
	}
	store, err := local.NewSite(o.DataDir, id, s.ForumURL, s.UserURL, o.Backend)
	if err != nil {
		m.notice = err.Error()
		return nil
	}
	// A fresh model owns every post ID, inline quote and scroll position. Never
	// carry old numeric caches into a new site or allow old async results in.
	next := newModel()
	o.Store = store
	next.opts, next.store, next.siteConfigs = o, store, m.siteConfigs
	next.opts.Demo = false
	next.boardNames = []string{"全部"}
	next.threads = nil
	next.refilter()
	next.requestID = m.requestID + 1
	next.fullscreen = m.fullscreen
	next.chatStyle = m.chatStyle
	next.theme = m.theme
	next.agentSeed = m.agentSeed
	next.imageCache = m.imageCache
	next.resize(m.width, m.height)
	if err = next.setIdentity(""); err != nil {
		m.notice = err.Error()
		return nil
	}
	if m.selectionCancel != nil {
		m.selectionCancel()
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.stopPagePrefetch()
	*m = next
	m.rememberSite()
	m.refilter()
	return m.initialLoad()
}
