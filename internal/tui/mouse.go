package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Forum chrome coordinates. Agent content uses its own shared layout helpers.
const browseTop = 3
const tabsY = 2

type listClick struct {
	site, identity       string
	id, x, y, generation int
	at                   time.Time
}
type mousePane int

const (
	noMousePane mousePane = iota
	listMousePane
	readerMousePane
)

func (m model) readerX() int {
	if m.split() {
		return m.listWidth() + 2
	}
	return 1
}
func (m model) paneAt(x, y int) mousePane {
	if m.chatStyle {
		if x < 4 || x >= m.width-4 || y < agentContentTop || y >= agentContentTop+m.reader.Height() {
			return noMousePane
		}
		if m.reading {
			return readerMousePane
		}
		return listMousePane
	}
	if y < browseTop+1 || y >= browseTop+m.panelHeight()-1 {
		return noMousePane
	}
	if m.split() {
		if x > 1 && x < m.listWidth() {
			return listMousePane
		}
		if x > m.readerX() && x < m.readerX()+m.readerWidth()-1 {
			return readerMousePane
		}
	} else if x > 1 && x < m.width-2 {
		if m.reading {
			return readerMousePane
		}
		return listMousePane
	}
	return noMousePane
}
func (m model) boardTabs() []string {
	var tabs []string
	for i, b := range m.boardNames {
		selected := i == m.board && m.kind != "mine" && m.kind != "sage"
		label := fmt.Sprintf("%d %s", i+1, b)
		if m.width < 65 {
			label = fmt.Sprintf("%d%s", i+1, b)
			if selected {
				label = strong(label, teal)
			} else {
				label = ink(label, muted)
			}
		} else if selected {
			label = badge(label, ocean, teal)
		} else {
			label = ink(" "+label+" ", muted)
		}
		tabs = append(tabs, label)
	}
	return tabs
}
func (m model) boardAt(x int) int {
	limit := m.width - 2
	if !m.opts.Demo {
		limit -= ansi.StringWidth(m.mineButton()) + 2
	}
	rendered := clip(strings.Join(m.boardTabs(), " "), limit)
	if strings.HasSuffix(ansi.Strip(rendered), "…") {
		limit = ansi.StringWidth(rendered) - 1
	}
	start := 1
	for i, tab := range m.boardTabs() {
		end := start + ansi.StringWidth(tab)
		if x >= start && x < end && end <= 1+limit {
			return i
		}
		start = end + 1
	}
	return -1
}

// Browsing can be replaced while it loads. Writes and identity changes must
// finish normally, even if their caller has no modal open.
func (m model) canSwitchBoard() bool {
	if !m.busy {
		return true
	}
	switch m.requestKind {
	case "list", "thread", "resume-thread", "pagination", "inline-quote", "reply-refresh":
		return true
	}
	return false
}

func (m *model) chooseBoard(index int) tea.Cmd {
	if index < 0 || index >= len(m.boardNames) {
		return nil
	}
	m.flushPersistence()
	m.pendingRestore = nil
	m.savePosition()
	m.board, m.filter, m.reading = index, "", false
	m.kind = "timeline"
	if m.opts.Demo {
		m.refilter()
		return nil
	}
	if index > 0 {
		m.kind = "board"
	}
	return m.loadList(1)
}
func (m model) menuLayout(width int) (prefix string, start, capacity int) {
	prefix = strong(m.returnModal, teal) + "\n\n"
	capacity = max(1, m.height-12)
	if m.cookieNotice != "" && strings.HasPrefix(m.returnModal, "饼干 ·") {
		note := bodyText(m.cookieNotice, width)
		prefix += ink(note, sand) + "\n\n"
		capacity = max(1, capacity-lipgloss.Height(note)-1)
	}
	start = max(0, m.menuIndex-capacity+1)
	return
}
func (m *model) mouseKey(code rune) tea.Cmd {
	next, cmd := m.update(tea.KeyPressMsg{Code: code})
	*m = next.(model)
	return cmd
}
func (m *model) focusMouseReader() bool {
	if m.reading {
		return true
	}
	offset := m.reader.YOffset()
	if m.opts.Demo {
		m.reading = true
		m.refreshReader(false)
	} else {
		t := m.current()
		if t == nil || !m.enterSelectedThread(t.id) {
			m.notice = "串内容尚未加载完成 · Enter 打开阅读"
			return false
		}
	}
	// A pointer acts on the content currently visible, even if keyboard entry
	// would restore a different saved anchor within the cached page.
	m.reader.SetYOffset(offset)
	m.syncActivePost()
	return true
}
func (m *model) focusMouseList() {
	if m.reading {
		m.flushPersistence()
		m.leaveReading()
	}
}
func (m model) listAt(y int) int {
	m.setListOffset(m.listVisualOffset())
	row := y - m.listContentTop()
	if row < 0 || row >= m.panelHeight()-4 {
		return -1
	}
	row += m.listInset
	for i, end := m.listTop, m.listEnd(m.listTop); i < end; i++ {
		height := m.listItemHeight(i)
		if row < height-1 {
			return i
		}
		if row < height {
			return -1
		} // Card gap.
		row -= height
	}
	return -1
}
func (m model) readerAt(y int) (readerItem, bool) {
	if y < m.readerTop() || y >= m.readerTop()+m.reader.Height() {
		return readerItem{}, false
	}
	row := y - m.readerTop() + m.reader.YOffset()
	for _, item := range m.readerItems {
		if row >= item.line && row < item.actionEnd {
			return item, true
		}
	}
	return readerItem{}, false
}
func (m *model) mouseMenu(click tea.MouseClickMsg) tea.Cmd {
	dialog := m.dialog()
	x, y := (m.width-lipgloss.Width(dialog))/2, (m.height-lipgloss.Height(dialog))/2
	if click.Button == tea.MouseRight || click.X < x || click.X >= x+lipgloss.Width(dialog) || click.Y < y || click.Y >= y+lipgloss.Height(dialog) {
		m.lastListClick = listClick{}
		return m.mouseKey(tea.KeyEscape)
	}
	if click.Button != tea.MouseLeft {
		return nil
	}
	if click.X < x+3 || click.X >= x+lipgloss.Width(dialog)-3 || click.Y < y+1 || click.Y >= y+lipgloss.Height(dialog)-1 {
		return nil
	}
	prefix, start, capacity := m.menuLayout(lipgloss.Width(dialog) - 6)
	row := click.Y - (y + 2 + strings.Count(prefix, "\n"))
	index := start + row
	if row < 0 || row >= capacity || index < 0 || index >= len(m.menu) {
		return nil
	}
	m.menuIndex = index
	m.lastListClick = listClick{}
	return m.selectMenu()
}
func (m *model) mouseWheel(w tea.MouseWheelMsg) tea.Cmd {
	m.lastListClick = listClick{}
	if m.modal == "attachment" {
		dialog := m.dialog()
		x, y := (m.width-lipgloss.Width(dialog))/2, (m.height-lipgloss.Height(dialog))/2
		if w.X >= x && w.X < x+lipgloss.Width(dialog) && w.Y >= y && w.Y < y+lipgloss.Height(dialog) {
			key := rune(0)
			if w.Button == tea.MouseWheelUp {
				key = '+'
			} else if w.Button == tea.MouseWheelDown {
				key = '-'
			}
			if key != 0 {
				cmd, _ := m.attachmentUpdate(tea.KeyPressMsg{Code: key, Text: string(key)})
				return cmd
			}
		}
		return nil
	}
	if m.modal == "publish" {
		m.popup, _ = m.popup.Update(w)
		return nil
	}
	if m.modal == "menu" {
		if len(m.menu) > 0 {
			if w.Button == tea.MouseWheelDown {
				m.menuIndex = min(len(m.menu)-1, m.menuIndex+1)
			}
			if w.Button == tea.MouseWheelUp {
				m.menuIndex = max(0, m.menuIndex-1)
			}
		}
		return nil
	}
	if m.modal != "" {
		return nil
	}
	pane := m.paneAt(w.X, w.Y)
	if pane == noMousePane {
		return nil
	}
	if pane == listMousePane {
		m.focusMouseList()
	} else if !m.focusMouseReader() {
		return nil
	}
	// Both panes use the same cursor navigation as the keyboard, including
	// page boundaries. Never infer selection from a scrolled viewport.
	if w.Button == tea.MouseWheelDown {
		return m.mouseKey('j')
	}
	if w.Button == tea.MouseWheelUp {
		return m.mouseKey('k')
	}
	return nil
}
func (m *model) mouseClick(c tea.MouseClickMsg) tea.Cmd {
	if c.Button != tea.MouseLeft && c.Button != tea.MouseRight {
		return nil
	}
	if m.modal == "menu" {
		return m.mouseMenu(c)
	}
	if !m.chatStyle && c.Y == 0 && c.Button == tea.MouseLeft && c.X >= m.modeButtonX() && c.X < m.width-1 {
		m.toggleChatStyle()
		return nil
	}
	if cmd, handled := m.clickAgentReply(c); handled {
		return cmd
	}
	if m.modal != "" {
		return nil
	}
	if c.Button == tea.MouseLeft && m.agentBackButtonAt(c.X, c.Y) {
		m.lastListClick = listClick{}
		m.focusMouseList()
		return nil
	}
	if !m.chatStyle && c.Y == tabsY && c.Button == tea.MouseLeft {
		m.lastListClick = listClick{}
		if !m.opts.Demo && c.X >= m.mineButtonX() && c.X < m.width-1 {
			if !m.capabilities().Mine {
				m.openSites()
				return nil
			}
			return m.openMine()
		}
		if index := m.boardAt(c.X); index >= 0 {
			return m.chooseBoard(index)
		}
		return nil
	}
	switch m.paneAt(c.X, c.Y) {
	case listMousePane:
		if c.Button != tea.MouseLeft {
			return nil
		}
		if !m.chatStyle && c.Y == browseTop+1 {
			title := "岛上此刻"
			if m.chatStyle {
				title = "对话列表"
			}
			if m.kind == "mine" {
				title = "我的内容 · " + m.identity.Alias
			}
			if m.filter != "" {
				title = "筛选结果"
			}
			label := m.listPage.Label()
			if ansi.StringWidth(title)+ansi.StringWidth(label)+2 <= m.listWidth()-6 && c.X >= 4+m.listWidth()-6-ansi.StringWidth(label) && c.X < m.listWidth()-2 {
				m.focusMouseList()
				return m.mouseKey('P')
			}
			return nil
		}
		index := m.listAt(c.Y)
		if index < 0 {
			return nil
		}
		titleY := m.listContentTop() + m.listRow(index) - m.listVisualOffset()
		titleClick := c.Y == titleY && c.X >= 4 && c.X < m.listWidth()-2
		m.focusMouseList()
		now := time.Now()
		id := m.threads[m.visible[index]].id
		previous := m.lastListClick
		double := previous.id == id && previous.site == m.opts.Site && previous.identity == m.identity.Alias && previous.generation == m.requestID && previous.x == c.X && previous.y == c.Y && now.Sub(previous.at) <= 400*time.Millisecond
		if index != m.selected {
			m.moveSelection(index - m.selected)
		}
		m.lastListClick = listClick{site: m.opts.Site, identity: m.identity.Alias, id: id, x: c.X, y: c.Y, generation: m.requestID, at: now}
		if m.chatStyle || titleClick || double {
			m.lastListClick = listClick{}
			return m.mouseKey(tea.KeyEnter)
		}
	case readerMousePane:
		m.lastListClick = listClick{}
		if !m.chatStyle && c.Y == browseTop+m.panelHeight()-2 && c.Button == tea.MouseLeft {
			if m.focusMouseReader() {
				return m.mouseKey('P')
			}
			return nil
		}
		item, ok := m.readerAt(c.Y)
		if !ok || c.X < m.readerX()+3+2*min(item.depth, 4) || c.X >= m.readerX()+m.readerWidth()-3 {
			return nil
		}
		row := c.Y - m.readerTop() + m.reader.YOffset()
		if !m.focusMouseReader() {
			return nil
		}
		// Use the key from the rendered page, not a guessed floor based on height.
		for _, current := range m.readerItems {
			if current.key == item.key {
				item = current
				break
			}
		}
		m.setReaderItem(item)
		m.refreshReader(false)
		if c.Button == tea.MouseRight {
			m.openPostActions()
			return nil
		}
		if row == item.quoteLine {
			return m.toggleInlineQuotes()
		}
		if row >= item.attachmentFrom && row < item.attachmentTo {
			return m.mouseAttachment(row, c.X-(m.readerX()+3+2*min(item.depth, 4)))
		}
	}
	return nil
}
func (m *model) mouseAttachment(row, x int) tea.Cmd {
	if m.chatStyle {
		x -= 2
	}
	// An attachment hint opens the first media item; a thumbnail opens that image.
	url := ""
	for _, slot := range m.imageSlots {
		if row >= slot.line && row <= slot.line+slot.height && x >= 0 && x < slot.width {
			url = slot.original
			break
		}
	}
	p, ok := m.activeRaw()
	if !ok {
		return nil
	}
	m.menu = nil
	for _, a := range p.Media() {
		m.menu = append(m.menu, menuItem{a.Type + " · " + a.URL, "attachment", a.URL})
	}
	if len(m.menu) == 0 {
		return nil
	}
	m.menuIndex = 0
	for i, a := range m.menu {
		if a.Value == url {
			m.menuIndex = i
			break
		}
	}
	m.attachmentDirect = true
	m.returnModal = fmt.Sprintf("No.%d · 附件", p.ID)
	return m.openAttachment()
}
func (m *model) mouseUpdate(msg tea.Msg) (tea.Cmd, bool) {
	switch v := msg.(type) {
	case tea.MouseClickMsg:
		if m.busy || m.width < 44 || m.height < 16 || v.Mod != 0 {
			return nil, true
		}
		m.flushPersistence()
		return m.mouseClick(v), true
	case tea.MouseWheelMsg:
		if m.busy || m.width < 44 || m.height < 16 || v.Mod != 0 {
			return nil, true
		}
		return m.mouseWheel(v), true
	case tea.MouseMotionMsg, tea.MouseReleaseMsg:
		return nil, true
	}
	return nil, false
}
