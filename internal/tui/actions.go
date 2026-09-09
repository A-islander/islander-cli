package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/media"
)

func (m *model) openPostActions() {
	p, ok := m.activeRaw()
	if !ok {
		return
	}
	m.menu = []menuItem{
		{"R · 引用这条帖子并回复", "post-action", "R"},
		{"r · 回复当前串", "post-action", "r"},
		{"a · 查看这条帖子的附件", "post-action", "a"},
		{"v · 原位展开 / 收起引用", "post-action", "v"},
	}
	if m.capabilities().Sage {
		m.menu = append(m.menu, menuItem{"s · SAGE", "post-action", "s"}, menuItem{"S · 反对 SAGE", "post-action", "S"})
	}
	if m.capabilities().Manage && m.identity.Alias != "" && p.UserID == m.identity.ID {
		m.menu = append(m.menu, menuItem{"x · 删除", "post-action", "x"}, menuItem{"X · 恢复", "post-action", "X"})
	}
	if !m.capabilities().Publish {
		m.menu = []menuItem{{"a · 查看附件", "post-action", "a"}, {"v · 原位展开 / 收起引用", "post-action", "v"}}
		if m.capabilities().Reply {
			m.menu = append([]menuItem{{"R · 引用这条帖子并回复", "post-action", "R"}, {"r · 回复当前串", "post-action", "r"}}, m.menu...)
		}
	}
	label := "* · 收藏当前主串"
	if t := m.current(); t != nil && m.isFavorite(t.id) {
		label = "* · 取消收藏当前主串"
	}
	m.menu = append(m.menu, menuItem{label, "post-action", "*"}, menuItem{"F · 打开收藏", "post-action", "F"})
	m.returnModal = fmt.Sprintf("No.%d · 帖子操作", p.ID)
	m.menuIndex = 0
	m.modal = "menu"
}

func (m *model) beginCompose(reply, quote bool) tea.Cmd {
	if !m.capabilities().CanPublish(reply) {
		m.notice = "当前站点暂未接入发帖；请使用站点网页"
		return nil
	}
	if m.identity.Alias == "" {
		m.notice = "请先导入或领取饼干"
		m.openCookies()
		m.cookieNotice = "没有饼干，无法发串/回复串\n请先导入或选择当前岛的饼干。"
		return nil
	}
	d := forum.Draft{Cookie: m.identity.Alias}
	if reply {
		t := m.current()
		if t == nil {
			return nil
		}
		d.ThreadID = t.id
		if p, ok := m.raw[t.id]; ok {
			d.ThreadID = p.ThreadID()
		}
		if quote {
			if p, ok := m.selectedPost(); ok {
				d.Body = forum.Quote(m.opts.Site, p.id) + "\n"
			}
		}
	} else {
		if m.board < 1 || m.board > len(m.apiBoards) {
			m.notice = "请先按 b 选择发串板块"
			m.openBoards()
			return nil
		}
		d.BoardID = m.apiBoards[m.board-1].ID
	}
	if m.chatStyle && reply && !quote && m.draft.ThreadID == d.ThreadID && m.draft.Cookie == d.Cookie && (m.draft.Body != "" || len(m.draft.Files) > 0 || len(m.draft.Media) > 0) {
		return m.editDraft()
	}
	m.draft = d
	return m.editDraft()
}
func (m *model) editDraft() tea.Cmd {
	m.modal = "compose"
	m.editor.SetValue(m.draft.Body)
	m.titleInput.SetValue(m.draft.Title)
	m.titleInput.Placeholder = "标题（可留空）"
	m.editTitle = false
	m.editor.CharLimit = 8192
	m.editor.ShowLineNumbers = false
	m.editor.Placeholder = "写点什么吧…"
	m.resizeEditor()
	return m.editor.Focus()
}
func (m *model) syncDraft() { m.draft.Body = m.editor.Value(); m.draft.Title = m.titleInput.Value() }
func (m *model) saveDraft() error {
	m.syncDraft()
	if m.store == nil {
		return nil
	}
	return m.store.SaveDraft(&m.draft)
}
func (m *model) openCookies() {
	m.cookieNotice = ""
	m.menu = nil
	if m.store != nil {
		v, e := m.store.Read()
		if e != nil {
			m.notice = e.Error()
			return
		}
		for _, c := range v.Cookies {
			mark := ""
			if c.Alias == m.identity.Alias {
				mark = " · 当前"
			}
			m.menu = append(m.menu, menuItem{c.Alias + " / " + c.Name + mark, "cookie", c.Alias})
		}
	}
	m.menu = append(m.menu, menuItem{"导入饼干（隐藏输入）", "import", ""})
	if m.capabilities().Register {
		m.menu = append(m.menu, menuItem{"领取新饼干", "register", ""})
	}
	m.menu = append(m.menu, menuItem{"使用访客身份", "anonymous", ""})
	m.modal = "menu"
	m.returnModal = "饼干 · x 移除选中的本机饼干"
	m.menuIndex = 0
}
func (m *model) openDrafts() {
	if m.identity.Alias == "" {
		m.notice = "请先选择饼干"
		return
	}
	ds, e := m.store.Drafts(m.identity.Alias)
	if e != nil {
		m.notice = e.Error()
		return
	}
	m.menu = nil
	for _, d := range ds {
		title := d.Title
		if title == "" {
			title = strings.ReplaceAll(d.Body, "\n", " ")
		}
		m.menu = append(m.menu, menuItem{forum.Clean(title), "draft", d.ID})
	}
	if len(m.menu) == 0 {
		m.notice = "当前饼干没有草稿"
		return
	}
	m.modal = "menu"
	m.returnModal = "本地草稿 · x 删除选中草稿"
	m.menuIndex = 0
}
func (m *model) inputDialog(kind, placeholder string) tea.Cmd {
	m.modal = kind
	m.input.SetValue("")
	m.input.EchoMode = textinput.EchoNormal
	m.input.CharLimit = 8192
	m.input.Placeholder = placeholder
	return m.input.Focus()
}
func (m *model) preview() {
	m.syncDraft()
	if e := forum.ValidateDraft(m.client, m.draft); e != nil {
		m.notice = e.Error()
		return
	}
	if e := m.saveDraft(); e != nil {
		m.notice = e.Error()
		return
	}
	target := fmt.Sprintf("板块：%s (%d)", m.boardLabel(m.draft.BoardID), m.draft.BoardID)
	if m.draft.ThreadID > 0 {
		target = fmt.Sprintf("回复串：No.%d", m.draft.ThreadID)
	}
	content := "站点：" + m.environmentLabel() + "\n饼干：" + m.identityLabel() + "\n" + target + "\n标题：" + forum.Clean(m.draft.Title) + "\n\n" + forum.Clean(m.draft.Body)
	if len(m.draft.Files) > 0 {
		content += "\n\n确认后上传：\n" + strings.Join(m.draft.Files, "\n")
	}
	for _, f := range m.draft.Media {
		content += "\n已上传：" + f.URL
	}
	m.popup.SetContent(bodyText(content, m.popup.Width()))
	m.popup.GotoTop()
	m.modal = "publish"
}
func (m *model) selectMenu() tea.Cmd {
	if len(m.menu) == 0 {
		return nil
	}
	item := m.menu[m.menuIndex]
	m.modal = ""
	switch item.Action {
	case "site":
		return m.switchSite(item.Value)
	case "history", "favorite":
		entries := m.historyEntries
		if item.Action == "favorite" {
			entries = m.favoriteEntries
		}
		id, _ := strconv.Atoi(item.Value)
		for _, entry := range entries {
			if entry.ThreadID == id {
				if len(m.apiBoards) == 0 {
					n := entry.Navigation
					m.pendingRestore = &n
					return m.initialLoad()
				}
				return m.startRestore(entry.Navigation)
			}
		}
	case "history-toggle":
		b, e := m.store.Browsing(m.identity.Alias)
		if e == nil {
			e = m.store.SetHistoryDisabled(m.identity.Alias, !b.HistoryDisabled)
		}
		if e != nil {
			m.stateError = e.Error()
		}
		m.openHistory("")
	case "history-clear":
		m.modal = "confirm"
		m.confirmAction = "clear-history"
		m.notice = "清空当前岛 / 当前身份的浏览历史？收藏和草稿将保留，收藏的阅读位置将清除。"
	case "draft-edit":
		i, e := strconv.Atoi(item.Value)
		if e == nil && i >= 0 && i < len(m.draftEdits) {
			d := m.draftEdits[i]
			if d.Cookie != m.identity.Alias || d.ID != m.draft.ID {
				m.stateError = "草稿身份已变化"
				return nil
			}
			m.draft = d
			if err := m.store.SaveDraft(&m.draft); err != nil {
				m.stateError = err.Error()
			}
			return m.editDraft()
		}
	case "post-action":
		next, cmd, _ := m.extendedUpdate(tea.KeyPressMsg{Code: []rune(item.Value)[0], Text: item.Value})
		*m = next.(model)
		return cmd
	case "board":
		index, _ := strconv.Atoi(item.Value)
		return m.chooseBoard(index)
	case "mine":
		return m.openMine()
	case "sage":
		m.pendingRestore = nil
		m.board = 0
		m.kind = item.Action
		m.filter = ""
		return m.loadList(1)
	case "cookie", "anonymous":
		if e := m.store.Use(item.Value); e != nil {
			m.notice = e.Error()
			return nil
		}
		if e := m.setIdentity(""); e != nil {
			m.notice = e.Error()
			return nil
		}
		m.raw = map[int]forum.Post{}
		m.pages = map[int]forum.Page{}
		m.inlineQuotes = map[string][]inlineQuote{}
		m.quoteOffsets = map[string]int{}
		m.offsets = map[int]int{}
		m.activeQuote = ""
		m.jumpSource = nil
		m.threads = nil
		m.refilter()
		m.kind = "timeline"
		m.board = 0
		if m.pendingMine && item.Action == "cookie" {
			return m.openMine()
		}
		m.pendingMine = false
		if m.pendingRestore != nil {
			return m.startRestore(*m.pendingRestore)
		}
		return m.loadList(1)
	case "import":
		return m.inputDialog("alias", "给饼干起一个英文别名，例如 daily")
	case "register":
		m.modal = "confirm"
		m.confirmAction = "register"
		m.notice = "确认向用户服务领取饼干"
		return nil
	case "draft":
		ds, e := m.store.Drafts(m.identity.Alias)
		if e != nil {
			m.notice = e.Error()
			return nil
		}
		for _, d := range ds {
			if d.ID == item.Value {
				m.draft = d
				return m.editDraft()
			}
		}
	case "attachment":
		m.attachmentDirect = false
		return m.openAttachment()
	case "link":
		return m.launch("media", func(_ context.Context, _ forum.Backend) (any, error) {
			return "已打开链接", media.Open(item.Value)
		})

	}
	return nil
}
func (m *model) confirm() tea.Cmd {
	action, id := m.confirmAction, m.confirmID
	s := m.store
	if action == "clear-history" || action == "remove-history" {
		target := 0
		if action == "remove-history" {
			target, _ = strconv.Atoi(m.menu[m.menuIndex].Value)
		}
		if err := s.DeleteHistory(m.identity.Alias, target); err != nil {
			m.stateError = err.Error()
		}
		m.openHistory("")
		return nil
	}
	if action == "remove-favorite" {
		target, _ := strconv.Atoi(m.menu[m.menuIndex].Value)
		if err := s.RemoveFavorite(m.identity.Alias, target); err != nil {
			m.stateError = err.Error()
		}
		m.openFavorites("")
		return nil
	}
	if action == "discard" {
		m.modal = ""
		m.draft = forum.Draft{}
		return nil
	}
	if action == "remove-cookie" {
		alias := m.menu[m.menuIndex].Value
		return m.launch("identity", func(_ context.Context, _ forum.Backend) (any, error) { return nil, s.Remove(alias) })
	}
	if action == "remove-draft" {
		e := s.DeleteDraft(m.menu[m.menuIndex].Value)
		if e != nil {
			m.notice = e.Error()
		}
		m.openDrafts()
		return nil
	}
	if action == "register" {
		if m.opts.Demo {
			m.notice = "离线模式不领取真实饼干"
			m.modal = ""
			return nil
		}
		return m.launch("registered", func(ctx context.Context, c forum.Backend) (any, error) { token, e := c.Register(ctx); return token, e })
	}
	return m.launch("action", func(ctx context.Context, c forum.Backend) (any, error) { return nil, c.Action(ctx, action, id) })
}
func (m *model) extendedUpdate(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyPressMsg); ok && !m.busy && m.modal == "" && (k.String() == "ctrl+[" || k.String() == "ctrl+]") {
		delta := 1
		if k.String() == "ctrl+[" {
			delta = -1
		}
		index := m.board + delta
		if index < 0 || index >= len(m.boardNames) {
			return *m, nil, true
		}
		cmd := m.chooseBoard(index)
		return *m, cmd, true
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "f6" && (m.modal == "" || m.modal == "compose") {
		m.toggleChatStyle()
		return *m, nil, true
	}
	if cmd, handled := m.mouseUpdate(msg); handled {
		return *m, cmd, true
	}
	if cmd, handled := m.attachmentUpdate(msg); handled {
		return *m, cmd, true
	}
	if cmd, handled := m.paginationInput(msg); handled {
		return *m, cmd, true
	}
	if _, ok := msg.(startMsg); ok {
		cmd := m.initialLoad()
		return *m, cmd, true
	}
	if result, ok := msg.(directoryMsg); ok {
		if m.modal == "filepicker" && result.request == m.filePicker.request {
			m.filePicker.loading = false
			m.filePicker.entries = result.entries
			if result.err != nil {
				m.filePicker.err = result.err.Error()
			}
		}
		return *m, nil, true
	}
	if cmd, ok := media.Handle(msg); ok {
		return *m, cmd, true
	}
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		return *m, nil, false
	}
	if r, ok := msg.(resultMsg); ok {
		if r.ID != m.requestID {
			return *m, nil, true
		}
		m.busy = false
		if r.Err != nil {
			if r.Kind == "pagination" {
				v := r.Value.(paginationResult)
				if v.extend {
					m.activeWindow().blocked[v.page.Page] = true
				}
				m.notice = "翻页失败：" + forum.Clean(r.Err.Error()) + " · [ ] 或 P 手动重试"
				return *m, nil, true
			}
			if m.pendingRestore != nil && (r.Kind == "list" || r.Kind == "resume-thread") {
				m.pendingRestore = nil
				m.stateReady = false
				m.notice = "无法恢复上次位置：" + forum.Clean(r.Err.Error())
				return *m, nil, true
			}
			if d, ok := r.Value.(forum.Draft); ok {
				m.draft = d
			}
			m.notice = forum.Clean(r.Err.Error())
			if r.Kind == "list" {
				m.listError = m.notice
			}
			if r.Kind == "publish" {
				m.modal = "compose"
				m.editor.SetValue(m.draft.Body)
			}
			return *m, nil, true
		}
		switch r.Kind {
		case "pagination":
			m.applyPagination(r.Value.(paginationResult))
		case "initial":
			v := r.Value.(initialResult)
			m.apiBoards = v.Boards
			m.boardNames = []string{"全部"}
			for _, b := range v.Boards {
				m.boardNames = append(m.boardNames, forum.Clean(b.Name))
			}
			m.applyPage(v.Page)
			if m.pendingRestore != nil {
				return *m, m.startRestore(*m.pendingRestore), true
			}
			if t := m.current(); t != nil {
				m.homeThread = t.id
				delete(m.offsets, t.id)
				delete(m.readerSelections, t.id)
			}
		case "list":
			m.applyPage(r.Value.(forum.Page))
			if m.pendingRestore != nil {
				return *m, m.resumeAfterList(), true
			}
		case "thread":
			m.applyThread(r.Value.(threadResult))
		case "resume-thread":
			m.applyRestoredThread(r.Value.(threadResult))
		case "inline-quote":
			m.applyInlineQuotes(r.Value.(inlineQuoteResult))
		case "publish":
			m.draft = forum.Draft{}
			m.modal = ""
			m.notice = "已发布"
			if m.reading && m.current() != nil {
				return *m, m.loadThread(m.current().id, 1, 0), true
			}
			return *m, m.loadList(1), true
		case "action":
			m.modal = ""
			m.notice = "操作成功"
			if m.reading && m.current() != nil {
				return *m, m.loadThread(m.current().id, 1, 0), true
			}
			return *m, m.loadList(m.page), true
		case "identity":
			m.aliasInput = ""
			m.raw = map[int]forum.Post{}
			m.pages = map[int]forum.Page{}
			m.inlineQuotes = map[string][]inlineQuote{}
			m.quoteOffsets = map[string]int{}
			m.offsets = map[int]int{}
			m.activeQuote = ""
			m.jumpSource = nil
			m.threads = nil
			m.refilter()
			m.kind = "timeline"
			m.board = 0
			m.modal = ""
			if e := m.setIdentity(""); e != nil {
				m.notice = e.Error()
			}
			if m.pendingMine && m.identity.Alias != "" {
				return *m, m.openMine(), true
			}
			if m.pendingRestore != nil {
				return *m, m.startRestore(*m.pendingRestore), true
			}
			m.openCookies()
			return *m, m.loadList(1), true
		case "registered":
			token := r.Value.(string)
			m.modal = "registered-token"
			m.input.SetValue(token)
			m.input.EchoMode = textinput.EchoPassword
			m.notice = "饼干已领取。输入别名保存"
			m.aliasInput = token
			return *m, m.inputDialog("register-alias", "新饼干的英文别名"), true
		case "media":
			m.notice = fmt.Sprint(r.Value)
		}
		return *m, nil, true
	}
	keymsg, iskey := msg.(tea.KeyPressMsg)
	if !iskey {
		if m.modal == "compose" {
			var cmd tea.Cmd
			if m.editTitle {
				m.titleInput, cmd = m.titleInput.Update(msg)
			} else {
				m.editor, cmd = m.editor.Update(msg)
			}
			return *m, cmd, true
		}
		if m.modal == "alias" || m.modal == "token" || m.modal == "attach" || m.modal == "register-alias" {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return *m, cmd, true
		}
		return *m, nil, false
	}
	k := keymsg.String()
	if m.busy {
		if (k == "g" || k == "H" || k == "F") && m.modal == "" {
			if m.cancel != nil {
				m.cancel()
			}
			m.requestID++
			m.busy = false
			m.pendingRestore = nil
			if k == "H" {
				m.openHistory("")
			} else if k == "F" {
				m.openFavorites("")
			} else {
				m.openSites()
			}
			return *m, nil, true
		}
		if (k == "esc" || k == "q" || k == "ctrl+c") && m.modal != "publish" && m.modal != "confirm" && m.modal != "token" && m.modal != "register-alias" {
			if m.cancel != nil {
				m.cancel()
			}
			m.requestID++
			m.busy = false
			m.notice = "已取消读取"
			m.pendingRestore = nil
			if k == "q" || k == "ctrl+c" {
				return *m, tea.Quit, true
			}
		}
		return *m, nil, true
	}
	if m.modal == "filepicker" {
		if k == "ctrl+c" {
			if err := m.saveDraft(); err != nil {
				m.filePicker.err = err.Error()
				return *m, nil, true
			}
			return *m, tea.Quit, true
		}
		return *m, m.updateFileBrowser(k), true
	}
	if m.modal == "kaomoji" {
		return *m, m.updateKaomoji(k), true
	}
	if m.modal == "compose" {
		switch k {
		case "f3":
			return *m, m.openKaomoji(), true
		case "ctrl+c", "ctrl+s":
			if e := m.saveDraft(); e != nil {
				m.notice = e.Error()
				return *m, nil, true
			}
			m.modal = ""
			m.notice = "草稿已保存"
			if k == "ctrl+c" {
				return *m, tea.Quit, true
			}
			return *m, nil, true
		case "esc":
			if e := m.saveDraft(); e != nil {
				m.notice = e.Error()
				return *m, nil, true
			}
			m.modal = ""
			m.notice = "已保存草稿并返回"
			return *m, nil, true
		case "ctrl+p":
			m.preview()
			return *m, nil, true
		case "ctrl+a":
			if err := m.saveDraft(); err != nil {
				m.stateError = err.Error()
				return *m, nil, true
			}
			return *m, m.openFileBrowser(), true
		case "ctrl+x":
			m.draft.Files = nil
			m.draft.Media = nil
			m.notice = "已从草稿移除附件；已上传文件仍可能留在服务器"
			return *m, nil, true
		case "tab":
			if m.draft.ThreadID == 0 {
				m.editTitle = !m.editTitle
				if m.editTitle {
					m.editor.Blur()
					return *m, m.titleInput.Focus(), true
				}
				m.titleInput.Blur()
				return *m, m.editor.Focus(), true
			}
		}
		var cmd tea.Cmd
		if m.editTitle {
			m.titleInput, cmd = m.titleInput.Update(msg)
		} else {
			m.editor, cmd = m.editor.Update(msg)
		}
		return *m, cmd, true
	}
	if m.modal == "publish" {
		if k == "esc" {
			return *m, m.editDraft(), true
		}
		if k == "enter" {
			if m.draft.Cookie != m.identity.Alias {
				m.notice = "身份已变化，请重新预览"
				return *m, nil, true
			}
			d := m.draft
			s := m.store
			return *m, m.launch("publish", func(ctx context.Context, c forum.Backend) (any, error) { return s.Publish(ctx, c, d) }), true
		}
		m.popup, _ = m.popup.Update(msg)
		return *m, nil, true
	}
	if m.modal == "confirm" {
		if k == "esc" {
			m.modal = ""
			return *m, nil, true
		}
		if k == "enter" {
			return *m, m.confirm(), true
		}
		return *m, nil, true
	}
	if m.modal == "menu" {
		switch k {
		case "esc", "q":
			m.modal = ""
			m.pendingMine = false
		case "j", "down":
			m.menuIndex = min(len(m.menu)-1, m.menuIndex+1)
		case "k", "up":
			m.menuIndex = max(0, m.menuIndex-1)
		case "enter":
			return *m, m.selectMenu(), true
		case "x":
			if len(m.menu) > 0 {
				item := m.menu[m.menuIndex]
				if item.Action == "cookie" || item.Action == "draft" || item.Action == "history" || item.Action == "favorite" {
					m.modal = "confirm"
					m.confirmAction = "remove-" + item.Action
					m.notice = "确认移除：" + item.Label
				}
			}
		}
		return *m, nil, true
	}
	if m.modal == "attachment" {
		if k == "esc" {
			m.modal = "menu"
			return *m, nil, true
		}
		if k == "o" || k == "enter" || k == "s" {
			item := m.menu[m.menuIndex]

			if k == "o" {
				return *m, m.launch("media", func(_ context.Context, _ forum.Backend) (any, error) {
					return "已交给系统打开", media.Open(item.Value)
				}), true
			}
			return *m, m.launch("media", func(ctx context.Context, _ forum.Backend) (any, error) { return media.Download(ctx, item.Value, "") }), true
		}
		return *m, nil, true
	}
	if m.modal == "alias" || m.modal == "token" || m.modal == "attach" || m.modal == "register-alias" {
		if k == "esc" {
			if m.modal == "attach" {
				return *m, m.openFileBrowser(), true
			}
			m.modal = ""
			m.input.SetValue("")
			m.aliasInput = ""
			return *m, nil, true
		}
		if k == "enter" {
			value := strings.TrimSpace(m.input.Value())
			switch m.modal {
			case "alias":
				m.aliasInput = value
				cmd := m.inputDialog("token", "粘贴饼干；内容会隐藏")
				m.input.EchoMode = textinput.EchoPassword
				return *m, cmd, true
			case "token", "register-alias":
				alias, token := m.aliasInput, value
				if m.modal == "register-alias" {
					alias, token = value, m.aliasInput
				}
				s := m.store
				f, u, site := m.opts.ForumURL, m.opts.UserURL, m.opts.Site
				m.input.SetValue("")
				return *m, m.launch("identity", func(ctx context.Context, _ forum.Backend) (any, error) {
					c, e := forum.NewBackend(site, f, u, token)
					if e != nil {
						return nil, e
					}
					who, e := c.Verify(ctx)
					if e != nil {
						return nil, e
					}
					return nil, s.Import(alias, token, who)
				}), true
			case "attach":
				if value != "" {
					if e := m.addAttachment(value); e != nil {
						m.notice = e.Error()
						return *m, nil, true
					}
				}
				return *m, m.editDraft(), true
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return *m, cmd, true
	}
	if m.modal == "jump" && k == "enter" && !m.opts.Demo {
		id, e := strconv.Atoi(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(m.input.Value())), "no."))
		if e != nil || id <= 0 {
			m.notice = "请输入有效编号"
			return *m, nil, true
		}
		m.modal = ""
		m.savePosition()
		if t := m.current(); t != nil && m.jumpSource == nil {
			saved := *t
			m.jumpSource = &saved
		}
		return *m, m.openThread(id), true
	}
	if m.modal != "" {
		return *m, nil, false
	}
	if m.opts.Demo {
		return *m, nil, false
	}
	switch k {
	case "F":
		m.openFavorites("")
		return *m, nil, true
	case "*":
		m.toggleFavorite()
		return *m, nil, true
	case "H":
		m.openHistory("")
		return *m, nil, true
	case "g":
		m.openSites()
		return *m, nil, true
	case "m":
		return *m, m.openMine(), true
	case "L":
		m.menu = []menuItem{{"岛民hub", "link", "http://gitea.islander.top"}, {"串备份", "link", "https://github.com/A-islander/post-backup"}, {"x岛（三酱岛）", "link", "https://www.nmbxd.com"}, {"脑洞", "link", "https://www.naodong.fun"}, {"bog岛", "link", "http://bog.ac"}, {"欢乐恶狗岛", "link", "https://huanleegao.com/t"}, {"钉幕石之音", "link", "https://mszym.top/"}}
		m.menuIndex = 0
		m.modal = "menu"
		m.returnModal = "站务与友链 · Enter 外部打开"
		return *m, nil, true
	case "t":
		m.reading = false
		m.newest = true
		m.sortNewest()
		m.selected = 0
		m.listTop = 0
		m.refreshReader(true)
		m.notice = "已按当前页最新发布排序；Ctrl+R 恢复服务端顺序"
		return *m, nil, true
	case "b":
		m.openBoards()
		return *m, nil, true
	case "i":
		m.pendingMine = m.kind == "mine"
		m.openCookies()
		return *m, nil, true
	case "d":
		m.openDrafts()
		return *m, nil, true
	case "c":
		return *m, m.beginCompose(false, false), true
	case "r", "R":
		if m.reading {
			return *m, m.beginCompose(true, k == "R"), true
		}
		return *m, nil, true
	case "enter", "right", "l", "tab":
		if m.reading {
			if k == "tab" {
				m.leaveReading()
			} else if k == "enter" {
				m.openPostActions()
			}
			return *m, nil, true
		}
		if t := m.current(); t != nil {
			m.savePosition()
			if p, ok := m.raw[t.id]; ok && p.FollowID > 0 && m.jumpSource == nil {
				saved := *t
				m.jumpSource = &saved
			}
			return *m, m.openThread(t.id), true
		}
		return *m, nil, true
	case "[", "]":
		delta := 1
		if k == "[" {
			delta = -1
		}
		p := m.currentPage()
		page := p.Page + delta
		if page >= 1 && (delta < 0 || p.HasMore) {
			return *m, m.requestPage(page, false, delta, k, false), true
		}
		return *m, nil, true
	case "ctrl+r":
		if !m.stateReady && len(m.apiBoards) == 0 {
			return *m, m.initialLoad(), true
		}
		if !m.reading {
			m.newest = false
		}
		if m.reading && m.current() != nil {
			return *m, m.loadThread(m.current().id, m.pages[m.current().id].Page, 0), true
		}
		return *m, m.loadList(m.page), true
	case "v":
		return *m, m.toggleInlineQuotes(), true
	case "a":
		p, ok := m.activeRaw()
		if ok {
			m.menu = nil
			for _, a := range p.Media() {
				m.menu = append(m.menu, menuItem{a.Type + " · " + a.URL, "attachment", a.URL})
			}
			if !m.reading {
				m.menu = m.selectedThreadAttachments()
			}
			if len(m.menu) > 0 {
				m.modal = "menu"
				m.returnModal = fmt.Sprintf("No.%d · 附件", p.ID)
				m.menuIndex = 0
				m.attachmentDirect = true
				return *m, m.openAttachment(), true
			} else {
				m.notice = "当前楼层没有附件"
			}
		}
		return *m, nil, true
	case "s", "S", "x", "X":
		if ((k == "s" || k == "S") && !m.capabilities().Sage) || ((k == "x" || k == "X") && !m.capabilities().Manage) {
			m.notice = "当前站点不支持此操作"
			return *m, nil, true
		}
		p, ok := m.activeRaw()
		if !ok {
			return *m, nil, true
		}
		if m.identity.Alias == "" {
			m.openCookies()
			return *m, nil, true
		}
		if (k == "x" || k == "X") && p.UserID != m.identity.ID {
			m.notice = "只能删除或恢复自己的内容"
			return *m, nil, true
		}
		actions := map[string]string{"s": "sage", "S": "unsage", "x": "delete", "X": "restore"}
		m.confirmAction = actions[k]
		m.confirmID = p.ID
		m.modal = "confirm"
		m.notice = fmt.Sprintf("%s · No.%d · 饼干 %s", m.confirmAction, p.ID, m.identity.Alias)
		return *m, nil, true
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n, _ := strconv.Atoi(k)
		m.board = n - 1
		if m.board > len(m.apiBoards) {
			m.board = 0
		}
		m.kind = "timeline"
		if m.board > 0 {
			m.kind = "board"
		}
		m.filter = ""
		return *m, m.loadList(1), true
	case "?":
		m.modal = "live-help"
		return *m, nil, true
	}
	return *m, nil, false
}

func (m model) extendedDialog() (string, bool) {
	w := min(82, m.width-6)
	iw := w - 6
	var content string
	switch m.modal {
	case "page-jump":
		content = m.pageJumpContent(iw)
	case "kaomoji":
		w = min(112, m.width-6)
		iw = w - 6
		content = m.kaomojiContent(iw)
	case "filepicker":
		content = m.fileBrowserContent(iw)
	case "compose":
		target := m.boardLabel(m.draft.BoardID)
		if m.draft.ThreadID > 0 {
			target = fmt.Sprintf("回复 No.%d", m.draft.ThreadID)
		}
		content = strong(target+" · "+m.identity.Alias, teal) + "\n" + ink("F3 颜文字 · Ctrl+A 附件 · Ctrl+P 预览", muted) + "\n\n"
		if m.draft.ThreadID == 0 {
			content += m.titleInput.View() + "\n\n"
		}
		content += m.editor.View() + "\n" + ink(fmt.Sprintf("%d/8192 字节 · %d 个附件 · 自动保存 · F2 编辑历史 · Esc 保存返回", len(m.editor.Value()), len(m.draft.Files)+len(m.draft.Media)), sand)
	case "publish":
		content = strong("发布预览", teal) + "\n\n" + m.popup.View() + "\n\n" + ink("Enter 确认上传并发布 · Esc 返回编辑 · ↑↓ 滚动", sand)
	case "confirm":
		content = strong("确认操作", teal) + "\n\n" + bodyText(m.notice, iw) + "\n\n" + ink("Enter 确认 · Esc 取消", sand)
	case "menu":
		prefix, start, capacity := m.menuLayout(iw)
		content = prefix
		for i := start; i < min(len(m.menu), start+capacity); i++ {
			s := "  " + clip(m.menu[i].Label, iw-2)
			if i == m.menuIndex {
				s = strong("› "+clip(m.menu[i].Label, iw-2), teal)
			}
			content += s + "\n"
		}
		content += "\n" + ink("↑↓ 选择 · Enter 打开 · Esc / 右键 / 点外部关闭", muted)
	case "history-filter", "favorites-filter":
		label := "筛选浏览历史"
		if m.modal == "favorites-filter" {
			label = "筛选收藏"
		}
		content = strong(label, teal) + "\n\n" + m.input.View() + "\n\nEnter 筛选 · Esc 返回"
	case "alias", "token", "attach", "register-alias":
		labels := map[string]string{"alias": "导入饼干：别名", "token": "导入饼干：隐藏输入", "attach": "添加本地附件", "register-alias": "保存领取的饼干"}
		content = strong(labels[m.modal], teal) + "\n\n" + m.input.View() + "\n\n" + ink("Enter 确定 · Esc 返回", muted)
		if m.store != nil {
			content += "\n" + ink("凭证存储："+m.store.Backend, sand)
		}
	case "attachment":
		content = strong("附件", teal) + "\n\n" + bodyText(m.menu[m.menuIndex].Value, iw) + "\n\n" + ink("Enter 终端预览 · o 系统打开 · s 下载 · Esc 返回", sand)
	case "live-help":
		content = strong("上岛指南", teal) + "\n\n" + bodyText("鼠标：点击标题阅读 · 点击选楼层 · 右键操作\n滚轮跟随所在栏 · F6 Agent模拟切换\nF7 原主题 / el（柔和跟随终端）\nCtrl+[ / Ctrl+] 前后板块\ng 切换站点 · H 浏览历史 · F 收藏\n* 收藏 / 取消收藏当前主串\n↑↓ / jk 选串；长楼逐行读完再换楼 · Enter 操作\n底色高亮选中帖子 · Esc 引用返回上层\nn/p 上下楼 · PgUp/PgDn / 空格 滚动正文\n读到边界自动加载 · P 按页码跳转\nv 原位展开 / 收起引用 · 可继续展开嵌套引用\n: 按编号定位\nm 我的内容 · b 板块 / SAGE · [ ] 前后页\nc 发串 · r 回复 · R 引用选中楼层\ni 饼干管理 · d 本地草稿 · a 选中楼层附件\ns SAGE · S 反对 SAGE · x 删除 · X 恢复\n/ 筛选当前页 · t 页内最新发布 · L 站务友链\nCtrl+R 刷新 · f 布局\n编辑：F3 颜文字 · F2 编辑历史\nTab 切标题/正文 · Ctrl+A 浏览文件\nCtrl+X 移除草稿附件 · Ctrl+P 预览发布\nEsc / Ctrl+S 保存草稿 · q 退出", iw)
		if !m.capabilities().Manage {
			replyHelp := "发帖、我的内容及管理操作尚未接入。"
			if m.capabilities().Reply {
				replyHelp = "r 回复 · R 引用回复 · d 草稿\nF3 颜文字 · F2 编辑历史\nCtrl+A 选图 · Ctrl+P 预览后确认发送\n新主串、我的内容及管理操作尚未接入。"
			}
			if m.capabilities().Publish {
				replyHelp = "c 发串 · r 回复 · R 引用回复 · d 草稿\nF3 颜文字 · F2 编辑历史\nCtrl+A 选图 · Ctrl+P 预览后确认发送\n我的内容及管理操作尚未接入。"
			}
			content = strong(m.environmentLabel()+" · 浏览指南", teal) + "\n\n" + bodyText("鼠标：点击标题阅读 · 点击选楼层 · 右键操作\n滚轮跟随所在栏 · F6 Agent模拟切换\nF7 原主题 / el（柔和跟随终端）\nCtrl+[ / Ctrl+] 前后板块\ng 切换站点 · b 板块 · H 浏览历史 · i 饼干\nF 收藏列表 · * 收藏 / 取消收藏当前主串\n↑↓ / jk 选串；长楼逐行读完再换楼 · Enter 操作\nn/p 上下楼 · PgUp/PgDn 滚动\n读到边界自动加载 · P 按页码跳转\nv 原位展开 / 收起引用 · Esc 返回上层\na 加载附件 · +/- 小图大小\n[ ] 前后页 · : 按主串编号定位\n/ 当前页筛选 · Ctrl+R 刷新 · f 布局\nq 退出\n\n"+replyHelp, iw)
		}
	default:
		return "", false
	}
	h := min(m.height-4, lipgloss.Height(content)+4)
	return panel("\n"+content, w, h, true), true
}
