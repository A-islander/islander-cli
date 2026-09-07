package tui

import (
	"context"
	"fmt"
	"image"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/media"
	uv "github.com/charmbracelet/ultraviolet"
)

var attachmentSerial atomic.Uint32

type attachmentView struct {
	id          int
	cancel      context.CancelFunc
	img         image.Image
	loading     bool
	err         string
	capability  int // 0 awaiting query, 1 supported, -1 fallback
	sent, ready bool
	cols, rows  int
	content     string
}
type attachmentLoaded struct {
	id  int
	img image.Image
	err error
}
type attachmentEncoded struct {
	id   int
	data string
	err  error
}
type attachmentTimeout struct {
	id     int
	upload bool
}

func (m *model) closeAttachment() tea.Cmd {
	old := m.attachment
	if old.cancel != nil {
		old.cancel()
	}
	m.attachment = attachmentView{}
	if old.id != 0 && (old.sent || old.ready) {
		return tea.Raw(media.KittyDelete(old.id))
	}
	return nil
}

func (m *model) openAttachment() tea.Cmd {
	cleanup := m.closeAttachment()
	m.modal = "attachment"
	if len(m.menu) == 0 || m.menuIndex < 0 || m.menuIndex >= len(m.menu) {
		return cleanup
	}
	id := 0x500000 + int(attachmentSerial.Add(1)&0x0fffff)
	m.attachment = attachmentView{id: id, loading: true}
	if m.opts.Images == "off" {
		m.attachment.loading = false
		m.attachment.err = "图片预览已关闭 · o 外部打开 / s 下载"
		return cleanup
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	m.attachment.cancel = cancel
	u := m.menu[m.menuIndex].Value
	load := func() tea.Msg {
		defer cancel()
		img, err := media.LoadImage(ctx, u)
		return attachmentLoaded{id, img, err}
	}
	var query tea.Cmd
	if m.opts.Images == "blocks" {
		m.attachment.capability = -1
	} else if m.opts.Images == "kitty" {
		m.attachment.capability = 1
	} else {
		query = tea.Sequence(tea.Raw(media.KittyQuery(id)), tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg { return attachmentTimeout{id, false} }))
	}
	return tea.Batch(cleanup, load, query)
}

func (m model) attachmentSize() (int, int) { return max(8, min(m.width-8, 110)), max(3, m.height-12) }
func (m *model) renderAttachment() {
	a := &m.attachment
	if a.img == nil {
		return
	}
	w, h := m.attachmentSize()
	a.cols, a.rows = media.FitCells(a.img, w, h)
	if a.ready {
		a.content = media.KittyCells(a.id, a.cols, a.rows)
	} else {
		a.content = media.Blocks(a.img, a.cols, a.rows)
	}
}
func (m *model) encodeAttachment() tea.Cmd {
	a := &m.attachment
	if a.img == nil || a.sent || a.capability != 1 {
		return nil
	}
	a.sent = true
	id, img, cols, rows := a.id, a.img, a.cols, a.rows
	return func() tea.Msg {
		data, err := media.KittyUpload(img, id, cols, rows)
		return attachmentEncoded{id, data, err}
	}
}

func (m *model) attachmentUpdate(msg tea.Msg) (tea.Cmd, bool) {
	switch v := msg.(type) {
	case attachmentLoaded:
		if m.modal != "attachment" || m.attachment.id != v.id {
			return nil, true
		}
		m.attachment.loading = false
		m.attachment.cancel = nil
		if v.err != nil {
			m.attachment.err = v.err.Error()
			return nil, true
		}
		m.attachment.img = v.img
		m.renderAttachment()
		return m.encodeAttachment(), true
	case attachmentEncoded:
		if m.modal != "attachment" || m.attachment.id != v.id {
			return nil, true
		}
		if v.err != nil {
			m.attachment.capability = -1
			return nil, true
		}
		return tea.Sequence(tea.Raw(v.data), tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return attachmentTimeout{v.id, true} })), true
	case attachmentTimeout:
		if m.modal != "attachment" || m.attachment.id != v.id {
			return nil, true
		}
		if (!v.upload && m.attachment.capability == 0) || (v.upload && !m.attachment.ready) {
			m.attachment.capability = -1
			m.attachment.ready = false
			m.renderAttachment()
			if v.upload {
				return tea.Raw(media.KittyDelete(v.id)), true
			}
		}
		return nil, true
	case uv.KittyGraphicsEvent:
		if v.Options.ID < 0x500000 || v.Options.ID > 0x5fffff {
			return nil, false
		}
		if m.modal != "attachment" || m.attachment.id != v.Options.ID {
			return tea.Raw(media.KittyDelete(v.Options.ID)), true
		}
		a := &m.attachment
		if string(v.Payload) != "OK" {
			a.capability = -1
			a.ready = false
			m.renderAttachment()
			return tea.Raw(media.KittyDelete(a.id)), true
		}
		if a.capability < 0 {
			return tea.Raw(media.KittyDelete(a.id)), true
		}
		if a.capability == 0 {
			a.capability = 1
			return m.encodeAttachment(), true
		}
		if a.sent {
			a.ready = true
			m.renderAttachment()
			return tea.Raw(media.KittyResize(a.id, a.cols, a.rows)), true
		}
		return nil, true
	case tea.WindowSizeMsg:
		if m.modal != "attachment" {
			return nil, false
		}
		m.resize(v.Width, v.Height)
		m.renderAttachment()
		if m.attachment.ready {
			return tea.Raw(media.KittyResize(m.attachment.id, m.attachment.cols, m.attachment.rows)), true
		}
		return nil, true
	case tea.KeyPressMsg:
		if m.modal != "attachment" {
			return nil, false
		}
		switch v.String() {
		case "esc", "q":
			cmd := m.closeAttachment()
			m.modal = "menu"
			return cmd, true
		case "ctrl+c":
			return tea.Sequence(m.closeAttachment(), tea.Quit), true
		case "left", "h", "[", "right", "l", "]":
			delta := 1
			if v.String() == "left" || v.String() == "h" || v.String() == "[" {
				delta = -1
			}
			next := m.menuIndex + delta
			if next < 0 || next >= len(m.menu) || m.menu[next].Action != "attachment" {
				return nil, true
			}
			m.menuIndex = next
			return m.openAttachment(), true
		case "enter", "r":
			return m.openAttachment(), true
		case "b":
			m.attachment.capability = -1
			m.attachment.ready = false
			m.renderAttachment()
			return tea.Raw(media.KittyDelete(m.attachment.id)), true
		case "o", "s":
			return nil, false // Existing open/download actions.
		}
		return nil, true
	}
	return nil, false
}

func (m model) attachmentDialog() string {
	w, h := m.attachmentSize()
	a := m.attachment
	label := fmt.Sprintf("附件 %d/%d", m.menuIndex+1, len(m.menu))
	body := a.content
	if a.loading {
		body = ink("正在读取图片…", muted)
	} else if a.err != "" {
		body = bodyText(a.err, w)
	}
	mode := "字符预览"
	if a.ready {
		mode = "图片预览"
	}
	if a.loading || a.err != "" {
		mode = ""
	}
	content := strong(label, teal) + "  " + ink(mode, muted) + "\n\n" + rectangle(body, w, h) + "\n\n" +
		ink(clip("← → 切换 · Esc 返回 · r 重试 · b 字符预览", w), sand) + "\n" +
		ink(clip("o 外部打开 · s 下载 · GIF 显示首帧", w), muted)
	return panel(strings.TrimRight(content, "\n"), w+6, h+8, true)
}
