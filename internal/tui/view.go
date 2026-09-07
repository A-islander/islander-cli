package tui

import (
	"fmt"
	"github.com/A-islander/islander-cli/internal/forum"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	ocean      = "#101F28"
	foam       = "#DBE8E5"
	muted      = "#91A8B0"
	teal       = "#78DCCA"
	sand       = "#E6C58C"
	lineColor  = "#344D59"
	selectedBG = "#213E47"
)

func ink(s, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(displayText(s))
}
func strong(s, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(displayText(s))
}
func badge(s, fg, bg string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(bg)).Bold(true).Padding(0, 1).Render(displayText(s))
}

// Halfwidth voiced marks are spacing characters in terminals, but current
// grapheme width libraries merge them with the preceding character: (ﾟДﾟ)
// measures 3 cells instead of 5. Use visually similar spacing glyphs only in
// the display layer, before wrapping/composition. Never change stored input.
// https://github.com/charmbracelet/lipgloss/issues/666
var displayReplacer = strings.NewReplacer("ﾟ", "°", "ﾞ", "˝", "\t", "    ")

func displayText(s string) string { return displayReplacer.Replace(s) }

func wrapText(s string, w int) string { return ansi.Wrap(displayText(s), max(1, w), "") }
func clip(s string, w int) string     { return ansi.Truncate(displayText(s), max(0, w), "…") }
func row(left, right string, width int) string {
	left, right = displayText(left), displayText(right)
	if ansi.StringWidth(left)+ansi.StringWidth(right)+2 > width {
		return clip(left, width)
	}
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-ansi.StringWidth(right)) + right
}

// Fit in terminal cells, not bytes or rune counts: Chinese text is usually two cells.
func rectangle(s string, w, h int) string {
	lines := strings.Split(displayText(s), "\n")
	result := make([]string, max(0, h))
	for i := range result {
		var content string
		if i < len(lines) {
			content = ansi.Truncate(lines[i], max(0, w), "")
		}
		result[i] = content + strings.Repeat(" ", max(0, w-ansi.StringWidth(content)))
	}
	return strings.Join(result, "\n")
}

func panel(content string, width, height int, focused bool) string {
	color := lineColor
	if focused {
		color = teal
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(foam)).Background(lipgloss.Color(ocean)).
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(color)).
		Padding(0, 2).Render(rectangle(content, width-6, height-2))
}

// Nested ANSI styles can reset the background instead of inheriting the outer
// Lip Gloss style. Resolve defaults after composition so text, padding and
// borders use the same background, even with a different terminal theme.
func screenView(width, height int, layers ...*lipgloss.Layer) tea.View {
	canvas := lipgloss.NewCanvas(width, height).Compose(lipgloss.NewCompositor(layers...))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue // Do not replace the continuation of a wide grapheme.
			}
			cell = cell.Clone()
			if cell.Style.Bg == nil {
				cell.Style.Bg = lipgloss.Color(ocean)
			}
			if cell.Style.Fg == nil {
				cell.Style.Fg = lipgloss.Color(foam)
			}
			canvas.SetCell(x, y, cell)
		}
	}
	v := tea.NewView(canvas.Render())
	v.AltScreen = true
	return v
}

func bodyText(body string, width int) string {
	return ink(wrapText(body, width), foam)
}

func (m model) mineButton() string {
	if !m.capabilities().Mine {
		return badge("g 切换站点", teal, selectedBG)
	}
	if m.kind == "mine" {
		return badge("m 我的内容", ocean, teal)
	}
	return badge("m 我的内容", teal, selectedBG)
}

func (m model) mineButtonX() int { return m.width - 1 - ansi.StringWidth(m.mineButton()) }

func (m *model) threadContent(t thread) string {
	var lines []string
	var render func(post, string, int, int, string, string)
	render = func(p post, key string, root, depth int, title, failure string) {
		start := len(lines)
		itemIndex := len(m.readerItems)
		m.readerItems = append(m.readerItems, readerItem{key: key, post: p, root: root, depth: depth, line: start})
		indent := strings.Repeat("  ", min(depth, 4))
		w := max(1, m.reader.Width()-2-len(indent))
		floor := fmt.Sprintf("%02d 楼", root)
		if page, ok := m.pages[t.id]; ok {
			index := root
			if pos, exists := m.threadWindow.positions[p.id]; exists {
				page, index = m.threadWindow.pages[pos.page], pos.index
			}
			if page.Offset < 0 {
				floor = fmt.Sprintf("第%d页 · 本页 %d", page.Page, index+1)
			} else {
				floor = fmt.Sprintf("%02d 楼", page.Offset+index)
			}
		}
		isOP := root == 0
		if raw, ok := m.raw[p.id]; ok {
			isOP = raw.FollowID == 0 && !raw.ParentUnknown
		}
		if isOP {
			floor = "主楼"
		}
		if depth > 0 {
			floor = fmt.Sprintf("引用 · %d 层", depth)
		}
		meta := strong(fmt.Sprintf("No.%d", p.id), teal) + "  " + ink(p.author, sand)
		lines = append(lines, row(meta, ink(floor+" · "+p.time, muted), w), "")
		if depth == 0 && isOP && title != "" {
			lines = append(lines, strings.Split(strong(wrapText(title, w), foam), "\n")...)
			lines = append(lines, "")
		}
		if failure != "" {
			lines = append(lines, strings.Split(bodyText("无法读取引用："+failure+"\n返回上层收起后可重试。", w), "\n")...)
		} else if p.deleted {
			lines = append(lines, ink("[这条回复已被删除]", muted))
		} else {
			lines = append(lines, strings.Split(bodyText(p.body, w), "\n")...)
		}
		if p.attachment != "" {
			lines = append(lines, "")
			lines = append(lines, strings.Split(ink(wrapText("▧ "+p.attachment, w), sand), "\n")...)
		}
		if ids := m.quoteIDs(p); len(ids) > 0 {
			label := fmt.Sprintf("▸ %d 条引用 · v 展开", len(ids))
			if _, open := m.inlineQuotes[key]; open {
				label = fmt.Sprintf("▾ %d 条引用 · v 收起", len(ids))
			}
			lines = append(lines, "", ink(clip(label, w), teal))
		}
		selected := m.reading && ((depth == 0 && m.activeQuote == "" && root == m.activePost) || m.activeQuote == key)
		for n := start; n < len(lines); n++ {
			prefix := "  "
			if selected {
				prefix = strong("│ ", teal)
				if n == start {
					prefix = strong("› ", teal)
					lines[n] = lipgloss.NewStyle().Background(lipgloss.Color(selectedBG)).Render(rectangle(lines[n], w, 1))
				}
			} else if depth > 0 {
				prefix = ink("│ ", lineColor)
			}
			lines[n] = indent + prefix + lines[n]
		}
		m.readerItems[itemIndex].end = len(lines)
		lines = append(lines, "")
		for _, child := range m.inlineQuotes[key] {
			render(child.post, fmt.Sprintf("%s/%d", key, child.post.id), root, depth+1, "", child.err)
		}
	}
	for i, p := range t.posts {
		m.postLines = append(m.postLines, len(lines))
		title := t.title
		if raw, ok := m.raw[t.id]; ok {
			title = forum.Clean(raw.Title)
		}
		render(p, fmt.Sprint(p.id), i, 0, title, "")
		lines = append(lines, ink(strings.Repeat("─", max(1, m.reader.Width()-2)), lineColor), "")
	}
	end := "已经读到串尾了。慢慢来，岛一直在。"
	page, ok := m.pages[t.id]
	if m.threadWindow.last > 0 {
		page, ok = m.threadWindow.pages[m.threadWindow.last]
	}
	if ok && page.HasMore {
		end = "继续向下自动加载 · ] 下一页 · P 跳页"
	}
	if !m.opts.Demo {
		if _, ok := m.pages[t.id]; !ok {
			end = "Enter 打开串，读取回复"
		}
	}
	lines = append(lines, ink(end, muted))
	lines = append(lines, make([]string, max(0, m.reader.Height()-3))...)
	return strings.Join(lines, "\n")
}

func (m model) listPanel() string {
	w, h := m.listWidth(), m.panelHeight()
	iw := w - 6
	title := strong("岛上此刻", foam)
	if m.kind == "mine" {
		title = strong("我的内容 · "+m.identity.Alias, teal)
	}
	if m.filter != "" {
		title = strong("筛选结果", foam)
	}
	lines := []string{row(title, ink(m.listPage.Label(), muted), iw), ""}
	if len(m.visible) == 0 {
		if m.busy {
			lines = append(lines, ink("正在读取内容…", muted))
		} else if m.listError != "" {
			lines = append(lines, bodyText("读取失败，按 Ctrl+R 重试。\n\n"+m.listError, iw))
		} else if m.kind == "mine" && m.filter == "" {
			lines = append(lines, bodyText("这个饼干还没有发串或回复。\n\n按 i 切换发帖时使用的饼干。", iw))
		} else {
			lines = append(lines, ink("暂时没有匹配的串。", muted), "", ink("/ 修改筛选 · Esc 清除", teal))
		}
	}
	capacity := max(1, (h-4)/4)
	for vi := m.listTop; vi < min(len(m.visible), m.listTop+capacity); vi++ {
		t := m.threads[m.visible[vi]]
		prefix := "  "
		if vi == m.selected {
			prefix = "› "
		}
		if m.isFavorite(t.id) {
			prefix += "★ "
		}
		first := clip(prefix+t.title, iw)
		meta := clip(fmt.Sprintf("  %s · No.%d · %d 回复", t.board, t.id, m.replyCount(t)), iw)
		excerpt := clip("  "+t.excerpt, iw)
		if vi == m.selected {
			style := lipgloss.NewStyle().Background(lipgloss.Color(selectedBG)).Width(iw)
			lines = append(lines, style.Foreground(lipgloss.Color(teal)).Bold(true).Render(first),
				style.Foreground(lipgloss.Color(foam)).Render(meta), style.Foreground(lipgloss.Color(muted)).Render(excerpt))
		} else {
			lines = append(lines, ink(first, foam), ink(meta, muted), ink(excerpt, muted))
		}
		lines = append(lines, "")
	}
	return panel(strings.Join(lines, "\n"), w, h, !m.reading)
}

func (m model) readerPanel() string {
	w, h := m.readerWidth(), m.panelHeight()
	t := m.current()
	if t == nil {
		return panel("\n"+ink("海面很安静。\n换个板块，或清除筛选再看看。", muted), w, h, m.reading)
	}
	label := "串预览"
	if m.reading {
		label = "正在阅读"
	}
	if m.isFavorite(t.id) {
		label += " ★"
	}
	header := row(strong(label, teal), ink(t.board+" / "+fmt.Sprintf("No.%d", t.id), muted), w-6)
	active, _ := m.selectedPost()
	status := fmt.Sprintf("› No.%d", active.id)
	if active.quote != 0 {
		status += fmt.Sprintf(" → No.%d · v 预览", active.quote)
	}
	position := fmt.Sprintf("%02d / %02d 楼", m.activePost, len(t.posts)-1)
	if page, ok := m.pages[t.id]; ok {
		index := m.activePost
		if pos, exists := m.threadWindow.positions[t.posts[m.activePost].id]; exists {
			page, index = m.threadWindow.pages[pos.page], pos.index
		}
		if page.Offset < 0 {
			position = fmt.Sprintf("本页 %d · %d页 · P 跳页", index+1, page.Page)
		} else {
			position = fmt.Sprintf("%d/%d楼 · %d页 · P 跳页", page.Offset+index, max(0, page.Count-1), page.Page)
		}
	}
	if !m.opts.Demo && !m.reading {
		position = "Enter 打开完整串"
		status = "主楼与回复预览"
	}
	if m.activeQuote != "" {
		position = "引用 · Esc 上一层"
	}
	content := header + "\n\n" + rectangle(m.reader.View(), w-6, h-6) + "\n\n" + row(ink(status, muted), ink(position, teal), w-6)
	return panel(content, w, h, m.reading)
}

func (m model) dialog() string {
	if m.modal == "attachment" {
		return m.attachmentDialog()
	}
	if s, ok := m.extendedDialog(); ok {
		return s
	}
	w := min(76, m.width-6)
	iw := w - 6
	var content string
	switch m.modal {
	case "help":
		content = strong("上岛指南", teal) + "\n" + ink("键盘就够了。找到一条串，慢慢读。", muted) + "\n\n" +
			"↑ ↓ / j k     选串或选中楼层\n" +
			"Enter / Tab   进入阅读 / 切换焦点\n" +
			"Esc / h       返回列表，保留阅读位置\n" +
			"n / p         下一楼 / 上一楼\n" +
			"v             原位展开 / 收起引用\n" +
			"PgUp / PgDn   翻页（正文也可用空格）\n" +
			"1 — 5         全部 / 日常 / 游戏 / 技术 / 创作\n" +
			"/             筛选本地串的标题、摘要和编号\n" +
			":             按 No.编号定位主楼或回复\n" +
			"f             切换分栏 / 单栏（宽屏）\n" +
			"q / Ctrl+C    退出\n\n" +
			ink("离线原型 · 全部内容虚构 · 不连接正式服务", sand)
	case "filter", "jump":
		title, hint := "筛选已加载的串", "只筛选本地标题、摘要和编号；留空可清除。"
		if m.modal == "jump" {
			title, hint = "到某一楼看看", "输入主楼或回复编号，例如 No.10433。"
		}
		content = strong(title, teal) + "\n\n" + ink(ansi.Wrap(hint, iw, ""), muted) + "\n\n" + m.input.View() + "\n\n" + ink("Enter 确定 · Esc 取消", muted)
	}
	if m.modal == "help" && m.height < 28 {
		content = strong("上岛指南", teal) + "\n\n↑↓ / jk 选串、逐行读长楼\nEnter / Tab 阅读、切换\nEsc 返回 · n/p 换楼 · v 引用\n1—5 板块 · / 筛选 · : 定位\nf 布局 · PgUp/PgDn 滚动\nq 退出 · Esc 关闭帮助"
	}
	h := min(m.height-4, lipgloss.Height(content)+4)
	return panel("\n"+content, w, h, true)
}

func (m model) View() tea.View {
	name, wordmark, slogan := m.siteBranding()
	if m.width < 44 || m.height < 16 {
		return screenView(m.width, m.height, lipgloss.NewLayer(rectangle(name+"\n\n请把终端放大到至少 44 列 × 16 行。\nq 或 Ctrl+C 退出", m.width, m.height)))
	}
	iw := m.width - 2
	header := row(badge(name, ocean, teal)+"  "+strong(wordmark, foam), badge(m.environmentLabel(), sand, selectedBG)+"  "+ink(m.identityLabel(), muted), iw)
	subtitle := ink(slogan, muted)
	if m.kind == "mine" {
		subtitle = row(subtitle, ink("我的内容 · "+m.identity.Alias+" · 发串与回复（含删除记录）", teal), iw)
	}
	var tabs []string
	for i, b := range m.boardNames {
		label := fmt.Sprintf("%d %s", i+1, b)
		if i == m.board && m.kind != "mine" && m.kind != "sage" {
			tabs = append(tabs, badge(label, ocean, teal))
		} else {
			tabs = append(tabs, ink(" "+label+" ", muted))
		}
	}
	tabline := strings.Join(tabs, " ")
	if m.width < 65 {
		tabs = nil
		for i, b := range m.boardNames {
			label := fmt.Sprintf("%d%s", i+1, b)
			if i == m.board && m.kind != "mine" && m.kind != "sage" {
				label = strong(label, teal)
			} else {
				label = ink(label, muted)
			}
			tabs = append(tabs, label)
		}
		tabline = strings.Join(tabs, " ")
	}
	if !m.opts.Demo {
		button := m.mineButton()
		tabline = row(clip(tabline, iw-ansi.StringWidth(button)-2), button, iw)
	}
	var body string
	if m.split() {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.listPanel(), " ", m.readerPanel())
	} else if m.reading {
		body = m.readerPanel()
	} else {
		body = m.listPanel()
	}
	help := "b 板块  P 跳页  H 历史  F 收藏  Enter 阅读  a 图片  ? 帮助"
	if m.reading {
		help = "b 板块  P 跳页  H 历史  F 收藏  a 图片  r 回复 R 引用  Enter 操作  ? 帮助"
	}
	if m.width < 80 {
		help = "b 板块 P 跳页 H 历史 F 收藏 ? 帮助"
	}
	if !m.reading && m.capabilities().Publish {
		help = "c 发串  " + help
	}
	notice := m.notice
	if m.busy {
		notice = "正在处理… · Esc 可取消读取；写入请等待结果"
	}
	if m.filter != "" {
		notice = "筛选「" + m.filter + "」 · Esc 返回列表后可清除"
	}
	if m.stateError != "" {
		notice = "本地保存：" + m.stateError
	} else if m.opts.StateWarning != "" {
		notice = m.opts.StateWarning
	}
	content := header + "\n" + subtitle + "\n\n" + clip(tabline, iw) + "\n\n" + body + "\n" + ink(clip(notice, iw), muted) + "\n" + ink(clip(help, iw), teal) + "\n"
	base := lipgloss.NewStyle().Background(lipgloss.Color(ocean)).Foreground(lipgloss.Color(foam)).Padding(0, 1).Render(rectangle(content, iw, m.height))
	layers := []*lipgloss.Layer{lipgloss.NewLayer(base)}
	if m.modal != "" {
		dialog := m.dialog()
		layers = append(layers, lipgloss.NewLayer(dialog).X((m.width-lipgloss.Width(dialog))/2).Y((m.height-lipgloss.Height(dialog))/2).Z(1))
	}
	v := screenView(m.width, m.height, layers...)
	if !m.opts.Demo {
		v.MouseMode = tea.MouseModeCellMotion
		v.ReportFocus = true
	}
	return v
}
