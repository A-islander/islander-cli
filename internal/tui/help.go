package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type helpEntry struct{ key, title, detail string }
type helpState struct {
	selected, top, offset int
	detail                bool
}

func (m model) isHelp() bool { return m.modal == "help" || m.modal == "live-help" }

// Each entry describes one action. Alternate keys for the same action share a
// row; context-specific bindings explicitly name their context.
func (m model) helpEntries() []helpEntry {
	items := []helpEntry{
		{"↓ / j", "向下浏览", "在串列表中选择下一条串。\n\n阅读串时向下浏览当前楼层；超长回复逐行读完后才切换下一楼。普通回复会尽量完整显示，阅读位置会保留。"},
		{"↑ / k", "向上浏览", "在串列表中选择上一条串。\n\n阅读串时向上浏览当前楼层；超长回复先逐行浏览，再切换上一楼。"},
		{"Enter", "进入阅读 / 楼层操作", "在列表中进入选中的串，复用已经加载的内容。\n\n在线阅读时打开当前楼层的操作菜单，可查看附件、引用回复等；实际项目取决于站点能力。\n\n在本帮助页中，Enter 只打开说明，不执行命令。"},
		{"→ / l", "进入串阅读", "从串列表进入选中的串。已经加载的内容直接复用，不因切换焦点而重新请求。"},
		{"← / h", "返回串列表", "离开阅读区域，回到串列表，保留列表选择及阅读位置。\n\n浏览嵌套引用时，先返回上一层引用。Agent 页面也可点击右上角 exit 返回列表。"},
		{"Tab", "切换浏览焦点", "在串列表与阅读区域之间切换。\n\n发新串的编辑页面中，Tab 改为切换标题和正文；回复已有串时不需要标题。"},
		{"Esc", "返回 / 关闭", "在阅读区域返回上一层引用或串列表。弹窗中取消当前操作或返回上一层。\n\n编辑页面会先保存草稿再返回；本帮助页先从详情返回命令列表，再关闭帮助。"},
		{"n", "下一楼", "阅读串时直接选择下一楼。超长回复需要逐行看完时使用 ↓ / j。"},
		{"p", "上一楼", "阅读串时直接选择上一楼。回看超长回复时也可以使用 ↑ / k 逐行滚动。"},
		{"PgDn / 空格", "向下滚动正文", "在阅读区域向下滚动一屏正文。串列表中 PgDn 向下移动一屏，空格用于正文。\n\n在线浏览读到已加载内容边界时，可继续浏览以自动加载下一页。"},
		{"PgUp", "向上滚动正文", "在阅读区域向上滚动一屏正文；在串列表中向上移动一屏。"},
		{"v", "展开 / 收起引用", "选中包含引用的楼层，按 v 在原位展开引用，再按可收起。\n\n展开的引用中仍可选择下一层引用并继续展开。Esc 返回上一层；R 用于撰写引用回复。"},
		{"/", "筛选当前列表", "输入文字筛选当前已加载列表的标题和摘要，Enter 确认，Esc 取消。\n\n这是当前列表的本地筛选，不是全站搜索。清空输入再确认可清除筛选。"},
		{":", "按编号定位", "输入帖子编号，例如 No.10433 或 10433，然后按 Enter。\n\n在线模式会请求相应内容；离线演示只在模拟内容中查找。Esc 取消输入。"},
		{"f", "切换分栏 / 单栏", "在宽终端中切换左右分栏和单栏布局，保留当前内容。\n\n终端较窄时保持单栏；Agent 模拟布局使用独立的单栏页面。"},
		{"F6", "Agent 模拟切换", "切换论坛布局与 Agent 对话布局，并记住选择。阅读位置和编辑内容保留。\n\nAgent 页面隐藏论坛标识，显示模拟命令记录与 Working 动画；这些记录不会执行命令。点击底部输入框或按 r 可编辑回复。"},
		{"F7", "切换主题", "在原主题和 el 主题之间切换，并记住选择。\n\nel 柔和适配终端背景与文字色，图片保持原色。终端没有返回完整颜色时使用原主题；终端换色后按两次 F7 可重新查询。"},
		{"鼠标左键", "选择与阅读", "点击列表中的帖子标题进入阅读；Agent 列表点击卡片内容即可进入。点击正文楼层可选中它。\n\n帮助列表中点击命令打开详情。点击弹窗外部返回，按键作用与鼠标互不冲突。"},
		{"鼠标右键", "打开 / 关闭操作菜单", "对选中楼层打开操作菜单。菜单打开后，再次右键或点击外部可关闭。\n\n帮助页中右键返回上一层，不会对后面的帖子执行操作。"},
		{"鼠标滚轮", "滚动所在区域", "在列表或正文区域滚动对应内容。\n\n帮助列表中移动选择，详情页中滚动说明；原图页面中滚轮改为放大或缩小图片。"},
		{"?", "打开帮助", "打开本命令列表。↑↓ / j k 选择，Enter 查看详情，Esc 返回。\n\n列表和详情都支持 PgUp / PgDn、Home / End；查看说明不会执行对应命令。"},
		{"q / Ctrl+C", "退出程序", "浏览页面中退出程序。编辑中 Ctrl+C 会先保存草稿。\n\n帮助页中 q 返回上一层，Ctrl+C 退出程序。"},
	}
	if m.opts.Demo {
		return append(items, helpEntry{"1—5", "选择演示板块", "按编号切换演示板块。全部内容为本地模拟数据，不连接正式服务。"})
	}
	items = append(items,
		helpEntry{"g", "切换站点", "在岛民岛、X岛和 BOG岛之间选择。站点的饼干、草稿、收藏与历史分别保存，主题与 Agent 布局保持不变。"},
		helpEntry{"b", "选择板块", "打开当前站点的板块列表，↑↓ 选择，Enter 进入。可以返回时间线；站点支持时也显示 SAGE 入口。"},
		helpEntry{"Ctrl+[", "上一板块", "切换到列表中相邻的上一板块，包括时间线；到达开头后停止。\n\n部分终端会把 Ctrl+[ 识别为 Esc，此时请按 b 选择板块。"},
		helpEntry{"Ctrl+]", "下一板块", "切换到列表中相邻的下一板块，到达末尾后停止。也可以按 b 打开完整板块列表。"},
		helpEntry{"1—9", "按编号选择板块", "按顶部板块标签的数字切换板块，从第 1 页开始。列表较长时请按 b 选择。"},
		helpEntry{"[", "上一页", "在当前串列表或串内请求上一页。已经到第 1 页时不再向前。"},
		helpEntry{"]", "下一页", "在当前串列表或串内请求下一页。服务端确认没有更多内容时停止。\n\n持续向下浏览到边界，也可自动加载相邻页。"},
		helpEntry{"P", "按页码跳转", "输入大于 0 的页码，Enter 跳转，Esc 取消。\n\n输入框中 Ctrl+Home 到第 1 页；总页数已知时 Ctrl+End 到末页，串内 Ctrl+L 到末页最新回复。总页数未知时仍可直接输入页码。"},
		helpEntry{"H", "浏览历史", "打开当前站点、当前身份的本地浏览历史。选择记录继续阅读，可恢复上次页码与阅读位置。\n\n列表中按 x 移除记录，需要确认。"},
		helpEntry{"F", "收藏列表", "打开当前站点、当前身份保存的收藏，选择后继续阅读。列表中按 x 移除收藏，需要确认。"},
		helpEntry{"*", "收藏 / 取消收藏", "收藏当前主串，再按一次取消收藏。即使选中串内回复，收藏目标仍是主串。\n\n收藏保存在本地，按 F 查看。"},
		helpEntry{"t", "当前页按发布时间排序", "按当前已加载列表的发布时间排序，并回到列表第一条。\n\n只调整本地列表，不改变服务器排序；Ctrl+R 可恢复服务端顺序。"},
		helpEntry{"Ctrl+R", "刷新当前内容", "重新请求当前串页或列表页。列表刷新会恢复服务端顺序。\n\n原图刷新使用原图页面的 r 或 Enter。"},
		helpEntry{"i", "饼干管理", "管理当前站点的饼干：导入、选择或移除。导入时先填别名，再粘贴饼干内容。\n\n发帖和回复需要可用饼干；没有饼干时编辑窗口会显示提示。"},
		helpEntry{"a", "查看附件", "打开当前选中楼层的附件；列表中可查看选中串已加载的附件。图片优先在终端中显示，不支持时使用字符预览，也可交给系统打开。"},
		helpEntry{"+", "放大图片", "串内有小图时按 + 放大预览。查看原图时按 + 或向上滚轮放大。\n\n原图超出窗口时，Shift+方向键移动查看区域。"},
		helpEntry{"-", "缩小图片", "串内有小图时按 - 缩小预览。查看原图时按 - 或向下滚轮缩小。"},
		helpEntry{"原图 · 0", "适应窗口", "在原图页面按 0 恢复适应窗口的大小。"},
		helpEntry{"原图 · ← / →", "切换附件", "在当前楼层的附件之间切换。也可使用 h / l 或 [ / ]，到达首尾后停止。"},
		helpEntry{"原图 · Shift+方向", "移动图片区域", "在放大的原图中移动可见区域；↑↓ 也可以上下移动。"},
		helpEntry{"原图 · r / Enter", "强制刷新图片", "重新下载当前图片，成功后更新缓存；刷新失败时保留之前可用的缓存。"},
		helpEntry{"原图 · b", "字符图片预览", "在原图页面切换为字符块预览，可用于终端原生图片显示异常时查看。"},
		helpEntry{"原图 · o", "系统打开附件", "将当前附件交给系统打开，通常由浏览器处理。"},
		helpEntry{"原图 · s", "下载附件", "下载当前附件到本地。完成后查看界面提示中的保存结果。"},
		helpEntry{"L", "站务与友链", "打开站务与友链列表。选择链接后 Enter 交给系统浏览器打开。"},
	)
	caps := m.capabilities()
	if caps.Mine {
		items = append(items, helpEntry{"m", "我的内容", "打开当前身份的内容页面，查看自己发布的串与回复。未选择饼干时先选择身份。"})
	}
	if caps.Publish {
		items = append(items, helpEntry{"c", "发布新串", "在当前站点创建新串，按 Tab 切换标题和正文。请先选择目标板块和可用饼干。\n\nCtrl+P 打开发布预览，确认页 Enter 才会发送；Esc 保存草稿返回。"})
	}
	if caps.Reply || caps.Publish {
		items = append(items,
			helpEntry{"r", "回复当前串", "进入串阅读后按 r 打开编辑框；Agent 模式也可点击底部输入框。\n\n回复需要当前站点的可用饼干。编辑后 Ctrl+P 预览，再 Enter 确认发送。"},
			helpEntry{"R", "引用选中楼层回复", "先选中要引用的楼层，再按大写 R。编辑器会加入引用信息，发送目标仍是该楼层所在的主串。\n\nCtrl+P 预览后确认发送；小写 v 只是查看引用。"},
			helpEntry{"d", "本地草稿", "打开当前身份保存的草稿。选择草稿继续编辑，按 x 可确认后移除。草稿按站点与身份隔离。"},
			helpEntry{"编辑 · F3", "选择颜文字", "打开颜文字列表，方向键选择，Enter 插入正文。每行四个；BOG岛使用方括号版本。"},
			helpEntry{"编辑 · F2", "编辑历史", "查看当前草稿的本地编辑快照，选择历史版本恢复后继续编辑。"},
			helpEntry{"编辑 · Ctrl+A", "选择附件文件", "打开文件浏览器，进入目录选择附件；返回后继续编辑。可用格式与数量由站点校验。"},
			helpEntry{"编辑 · Ctrl+X", "移除草稿附件", "从当前草稿移除全部附件。已经上传到服务器的文件可能仍然保留。"},
			helpEntry{"编辑 · Ctrl+P", "预览发布", "校验并预览当前草稿。预览页面中 Enter 确认上传并发布，Esc 返回编辑；预览本身不会发布内容。"},
			helpEntry{"编辑 · Ctrl+S / Esc", "保存草稿返回", "保存当前标题、正文与附件信息并退出编辑。之后按 d 打开草稿继续编辑。"},
		)
	}
	if caps.Sage {
		items = append(items,
			helpEntry{"s", "SAGE", "对选中楼层发起 SAGE。需要当前站点饼干，并在确认页按 Enter 执行。"},
			helpEntry{"S", "反对 SAGE", "对选中楼层发起反对 SAGE。需要当前站点饼干，并在确认页按 Enter 执行。"})
	}
	if caps.Manage {
		items = append(items,
			helpEntry{"x", "删除自己的内容", "对选中的、属于当前身份的内容发起删除。确认目标后 Enter 执行，Esc 取消。"},
			helpEntry{"X", "恢复自己的内容", "对选中的、属于当前身份的内容发起恢复。需要站点支持与当前身份权限，确认后执行。"})
	}
	return items
}

func (m model) helpSize() (w, h, capacity int) {
	w, h = max(8, min(82, m.width-6)), max(7, min(30, m.height-4))
	return w, h, max(1, h-6)
}

func (m *model) normalizeHelp() {
	entries := m.helpEntries()
	_, _, capacity := m.helpSize()
	m.help.selected = max(0, min(m.help.selected, len(entries)-1))
	m.help.top = max(0, min(m.help.top, max(0, len(entries)-capacity)))
	if m.help.selected < m.help.top {
		m.help.top = m.help.selected
	}
	if m.help.selected >= m.help.top+capacity {
		m.help.top = m.help.selected - capacity + 1
	}
	m.help.offset = max(0, min(m.help.offset, max(0, len(m.helpDetailLines())-capacity)))
}

func (m model) helpDetailLines() []string {
	w, _, _ := m.helpSize()
	entries := m.helpEntries()
	e := entries[max(0, min(m.help.selected, len(entries)-1))]
	return strings.Split(wrapText("按键："+e.key+"\n\n"+e.detail, w-6), "\n")
}

func (m model) helpDialog() string {
	m.normalizeHelp()
	w, h, capacity := m.helpSize()
	iw := w - 6
	entries := m.helpEntries()
	title := row(strong("命令帮助", teal), ink(fmt.Sprintf("%d/%d", m.help.selected+1, len(entries)), muted), iw)
	footer := "↑↓ 选择 · Enter 详情 · Esc 关闭"
	var lines []string
	if m.help.detail {
		title = strong(clip(entries[m.help.selected].title, iw), teal)
		detail := m.helpDetailLines()
		lines = append([]string(nil), detail[m.help.offset:min(len(detail), m.help.offset+capacity)]...)
		footer = "↑↓ 滚动 · Esc 返回列表"
		if len(detail) > capacity {
			title = row(clip(title, max(1, iw-10)), ink(fmt.Sprintf("%d/%d", m.help.offset+1, len(detail)), muted), iw)
		}
		for i, line := range lines {
			lines[i] = ink(line, foam)
		}
	} else {
		for i := m.help.top; i < min(len(entries), m.help.top+capacity); i++ {
			e := entries[i]
			line := "  " + clip(e.key+"  "+e.title, iw-2)
			if i == m.help.selected {
				line = highlightLine(strong("> "+clip(e.key+"  "+e.title, iw-2), teal), iw)
			}
			lines = append(lines, line)
		}
	}
	return panel(title+"\n\n"+rectangle(strings.Join(lines, "\n"), iw, capacity)+"\n\n"+ink(clip(footer, iw), muted), w, h, true)
}

func (m *model) helpBack() {
	if m.help.detail {
		m.help.detail = false
		m.help.offset = 0
	} else {
		m.modal = ""
	}
}

func (m *model) helpMove(delta int) {
	if m.help.detail {
		m.help.offset += delta
	} else {
		m.help.selected += delta
	}
	m.normalizeHelp()
}

func (m *model) helpUpdate(msg tea.Msg) (tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "?" && m.modal == "" {
		m.modal = "help"
		m.help = helpState{}
		return nil, true
	}
	if !m.isHelp() {
		return nil, false
	}
	m.normalizeHelp()
	_, _, capacity := m.helpSize()
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		switch v.String() {
		case "ctrl+c":
			return tea.Quit, true
		case "esc", "q", "?", "left", "h":
			m.helpBack()
		case "enter", "right", "l":
			m.help.detail = true
		case "down", "j":
			m.helpMove(1)
		case "up", "k":
			m.helpMove(-1)
		case "pgdown", "ctrl+d", "space":
			m.helpMove(capacity)
		case "pgup", "ctrl+u":
			m.helpMove(-capacity)
		case "home":
			m.helpMove(-1000000)
		case "end":
			m.helpMove(1000000)
		}
		return nil, true
	case tea.MouseClickMsg:
		if v.Mod != 0 {
			return nil, true
		}
		dialog := m.helpDialog()
		w, h := lipgloss.Width(dialog), lipgloss.Height(dialog)
		x, y := (m.width-w)/2, (m.height-h)/2
		if v.Button == tea.MouseRight {
			m.helpBack()
			return nil, true
		}
		if v.Button != tea.MouseLeft {
			return nil, true
		}
		if v.X < x || v.X >= x+w || v.Y < y || v.Y >= y+h {
			m.helpBack()
			return nil, true
		}
		row := v.Y - y - 3
		if !m.help.detail && v.X >= x+3 && v.X < x+w-3 && row >= 0 && row < capacity && m.help.top+row < len(m.helpEntries()) {
			m.help.selected = m.help.top + row
			m.help.detail = true
			m.help.offset = 0
		}
		return nil, true
	case tea.MouseWheelMsg:
		w, h, _ := m.helpSize()
		x, y := (m.width-w)/2, (m.height-h)/2
		if v.Mod == 0 && v.X >= x && v.X < x+w && v.Y >= y && v.Y < y+h {
			if v.Button == tea.MouseWheelDown {
				m.helpMove(1)
			} else if v.Button == tea.MouseWheelUp {
				m.helpMove(-1)
			}
		}
		return nil, true
	case tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.PasteMsg:
		return nil, true
	}
	return nil, false
}
