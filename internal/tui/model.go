package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
)

type model struct {
	stateReady                                  bool
	stateError                                  string
	stateTickID, draftTickID                    uint64
	pendingRestore                              *local.Navigation
	readVisited                                 int64
	newest                                      bool
	historyEntries                              []local.HistoryEntry
	favoriteEntries                             []local.HistoryEntry
	attachmentDirect                            bool
	draftEdits                                  []forum.Draft
	attachment                                  attachmentView
	jumpSource                                  *thread
	opts                                        Options
	store                                       *local.Store
	client                                      forum.Backend
	siteConfigs                                 map[string]forum.Site
	listPage                                    forum.Page
	previewID, previewGeneration, previewTarget int
	previewCancel                               context.CancelFunc
	previews                                    map[int]previewResult
	identity                                    local.Cookie
	boardNames                                  []string
	apiBoards                                   []forum.Board
	raw                                         map[int]forum.Post
	pages                                       map[int]forum.Page
	kind                                        string
	page, total                                 int
	listError                                   string
	busy                                        bool
	requestID                                   int
	cancel                                      context.CancelFunc
	editor                                      textarea.Model
	titleInput                                  textinput.Model
	editTitle                                   bool
	draft                                       forum.Draft
	menu                                        []menuItem
	menuIndex                                   int
	confirmAction                               string
	confirmID                                   int
	aliasInput                                  string
	returnModal                                 string
	pendingMine                                 bool
	filePicker                                  fileBrowser

	threads                  []thread
	visible                  []int
	board, selected, listTop int
	width, height            int
	reading, fullscreen      bool
	reader, popup            viewport.Model
	input                    textinput.Model
	modal, filter, notice    string
	offsets                  map[int]int
	postLines                []int
	activePost               int
	activeQuote              string
	inlineQuotes             map[string][]inlineQuote
	quoteOffsets             map[string]int
	readerItems              []readerItem
}

func newModel() model {
	i := textinput.New()
	i.CharLimit = 80
	i.Prompt = "› "
	m := model{
		opts: Options{Demo: true}, boardNames: boards, kind: "timeline", page: 1, raw: map[int]forum.Post{}, pages: map[int]forum.Page{}, editor: textarea.New(), titleInput: textinput.New(), threads: demoThreads(), width: 120, height: 36,
		reader: viewport.New(), popup: viewport.New(), input: i,
		offsets:      make(map[int]int),
		inlineQuotes: make(map[string][]inlineQuote),
		quoteOffsets: make(map[string]int),
		notice:       "欢迎上岛。选一条串，停一会儿。",
	}
	m.refilter()
	m.resize(m.width, m.height)
	return m
}

func (m model) Init() tea.Cmd {
	if m.opts.Demo {
		return nil
	}
	return func() tea.Msg { return startMsg{} }
}

func (m model) current() *thread {
	if len(m.visible) == 0 {
		return nil
	}
	return &m.threads[m.visible[m.selected]]
}

func (m *model) savePosition() {
	if !m.opts.Demo && !m.reading {
		return
	}
	if t := m.current(); t != nil {
		m.offsets[t.id] = m.reader.YOffset()
	}
}

func (m *model) refilter() {
	m.visible = nil
	q := strings.ToLower(strings.TrimSpace(m.filter))
	for i, t := range m.threads {
		if m.opts.Demo && m.board > 0 && t.board != m.boardNames[m.board] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.title+" "+t.excerpt+" "+strconv.Itoa(t.id)), q) {
			continue
		}
		m.visible = append(m.visible, i)
	}
	m.selected, m.listTop, m.activePost = 0, 0, 0
	m.activeQuote = ""
	m.refreshReader(true)
}

func (m model) split() bool      { return m.width >= 100 && !m.fullscreen }
func (m model) panelHeight() int { return max(6, m.height-8) }
func (m model) listWidth() int {
	if m.split() {
		return min(43, m.width/3)
	}
	return m.width - 2
}
func (m model) readerWidth() int {
	if m.split() {
		return m.width - 3 - m.listWidth()
	}
	return m.width - 2
}

func (m *model) resize(w, h int) {
	m.width, m.height = w, h
	m.reader.SetWidth(max(8, m.readerWidth()-6))
	m.reader.SetHeight(max(1, m.panelHeight()-6))
	m.input.SetWidth(max(8, min(56, w-14)))
	m.popup.SetWidth(max(8, min(66, w-14)))
	m.popup.SetHeight(max(1, min(16, h-14)))
	m.refreshReader(false)
	m.editor.SetWidth(max(10, min(70, w-14)))
	m.editor.SetHeight(max(3, min(12, h-16)))
	m.titleInput.SetWidth(max(10, min(70, w-14)))
	m.ensureListVisible()
}

func (m *model) refreshReader(restore bool) {
	offset := m.reader.YOffset()
	m.postLines = nil
	m.readerItems = nil
	t := m.current()
	if t == nil {
		m.reader.SetContent("")
		return
	}
	if !m.opts.Demo && !m.reading {
		m.reader.SetContent(m.previewContent(*t))
		m.reader.GotoTop()
		return
	}
	if restore {
		offset = m.offsets[t.id]
	}
	m.activePost = max(0, min(m.activePost, len(t.posts)-1))
	m.reader.SetContent(m.threadContent(*t))
	m.reader.SetYOffset(offset)
}

func (m *model) ensureListVisible() {
	capacity := max(1, (m.panelHeight()-4)/4)
	if m.selected < m.listTop {
		m.listTop = m.selected
	}
	if m.selected >= m.listTop+capacity {
		m.listTop = m.selected - capacity + 1
	}
	m.listTop = min(m.listTop, max(0, len(m.visible)-capacity))
}

func (m *model) moveSelection(delta int) {
	if len(m.visible) == 0 {
		return
	}
	m.savePosition()
	m.selected = max(0, min(len(m.visible)-1, m.selected+delta))
	m.activePost = 0
	m.activeQuote = ""
	m.refreshReader(true)
	m.syncActivePost()
	m.ensureListVisible()
}

func (m *model) syncActivePost() {
	previous, quote := m.activePost, m.activeQuote
	for _, item := range m.readerItems {
		if item.line <= m.reader.YOffset() {
			m.setReaderItem(item)
		}
	}
	if previous != m.activePost || quote != m.activeQuote {
		m.refreshReader(false)
	}
}

func (m *model) movePost(delta int) {
	if len(m.postLines) == 0 {
		return
	}
	m.activePost = max(0, min(len(m.postLines)-1, m.activePost+delta))
	m.activeQuote = ""
	m.refreshReader(false)
	m.reader.SetYOffset(m.postLines[m.activePost])
}

func (m *model) startInput(kind string) tea.Cmd {
	m.modal = kind
	m.input.SetValue("")
	if kind == "filter" {
		m.input.SetValue(m.filter)
		m.input.Placeholder = "标题、摘要或串编号"
	} else {
		m.input.Placeholder = "例如 10433 或 No.10433"
	}
	return m.input.Focus()
}

func (m *model) submitInput() {
	value := strings.TrimSpace(m.input.Value())
	if m.modal == "filter" {
		m.savePosition()
		m.filter = value
		m.reading = false
		m.refilter()
		m.notice = fmt.Sprintf("本地筛选 · %d 条匹配，范围仅限当前页串的标题、摘要和编号", len(m.visible))
	} else {
		id, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(value), "no."))
		found := false
		if err == nil {
			for ti, t := range m.threads {
				for pi, p := range t.posts {
					if p.id != id {
						continue
					}
					m.savePosition()
					m.board, m.filter = 0, ""
					m.refilter()
					for vi, index := range m.visible {
						if index == ti {
							m.selected = vi
						}
					}
					m.reading, m.activePost = true, pi
					m.refreshReader(true)
					m.reader.SetYOffset(m.postLines[pi])
					m.ensureListVisible()
					m.notice = fmt.Sprintf("已定位 No.%d", id)
					found = true
					break
				}
				if found {
					break
				}
			}
		}
		if !found {
			m.notice = "未找到这个编号；原型只包含本地模拟内容"
		}
	}
	m.input.Blur()
	m.modal = ""
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.extendedUpdate(msg); handled {
		return next, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.modal == "filter" || m.modal == "jump" {
			switch key {
			case "esc":
				m.modal = ""
				m.input.Blur()
				return m, nil
			case "enter":
				m.submitInput()
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.modal != "" {
			if key == "esc" || key == "q" || key == "enter" || key == "v" || key == "?" {
				m.modal = ""
				return m, nil
			}
			return m, nil
		}
		switch key {
		case "q":
			return m, tea.Quit
		case "?":
			m.modal = "help"
			return m, nil
		case "/":
			return m, m.startInput("filter")
		case ":":
			return m, m.startInput("jump")
		case "1", "2", "3", "4", "5":
			m.savePosition()
			m.board, _ = strconv.Atoi(key)
			m.board--
			m.reading, m.filter = false, ""
			m.refilter()
			m.notice = "已切换到" + m.boardNames[m.board] + " · 模拟数据"
			return m, nil
		case "esc", "left", "h":
			if m.reading && m.returnToQuoteParent() {
				return m, nil
			}
			if m.reading {
				m.leaveReading()
			} else if m.filter != "" {
				m.savePosition()
				m.filter = ""
				m.refilter()
				m.notice = "已清除筛选"
			}
			return m, nil
		case "tab":
			if m.current() != nil {
				m.reading = !m.reading
				m.refreshReader(false)
			}
			return m, nil
		case "enter", "right", "l":
			if m.current() != nil {
				m.reading = true
				m.refreshReader(false)
			}
			return m, nil
		case "f":
			m.fullscreen = !m.fullscreen
			m.resize(m.width, m.height)
			return m, nil
		case "v":
			if m.reading {
				return m, m.toggleInlineQuotes()
			}
			return m, nil
		case "n":
			if m.reading {
				m.movePost(1)
			}
			return m, nil
		case "p":
			if m.reading {
				m.movePost(-1)
			}
			return m, nil
		}
		if !m.reading {
			switch key {
			case "down", "j":
				m.moveSelection(1)
			case "up", "k":
				m.moveSelection(-1)
			case "pgdown", "ctrl+d":
				m.moveSelection(max(1, (m.panelHeight()-4)/4))
			case "pgup", "ctrl+u":
				m.moveSelection(-max(1, (m.panelHeight()-4)/4))
			case "home", "g":
				m.moveSelection(-len(m.visible))
			case "end", "G":
				m.moveSelection(len(m.visible))
			}
			return m, nil
		}
		if m.reading {
			switch key {
			case "down", "j":
				m.moveReaderItem(1)
				return m, nil
			case "up", "k":
				m.moveReaderItem(-1)
				return m, nil
			}
		}
		m.reader, _ = m.reader.Update(msg)
		m.syncActivePost()
		return m, nil
	}
	if m.modal == "filter" || m.modal == "jump" {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}
