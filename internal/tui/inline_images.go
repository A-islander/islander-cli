package tui

import (
	"context"
	"fmt"
	"image"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/media"
	uv "github.com/charmbracelet/ultraviolet"
)

const inlineImageRows = 6
const inlineImageCacheLimit = 16

var inlineImageSerial atomic.Uint32

// Decoders cannot be interrupted mid-frame. Keep cancelled but still decoding
// jobs inside the same limit as newly scheduled requests during fast scrolling.
var inlineImageWorkers = make(chan struct{}, 2)

type inlineImageSlot struct {
	key, url, original  string
	line, width, height int
}
type inlineImageEntry struct {
	id                   int
	url                  string
	cancel               context.CancelFunc
	img                  image.Image
	loading, sent, ready bool
	nativeFailed         bool
	err                  string
	cols, rows           int
	content              string
	touched              uint64
}
type inlineImageState struct {
	entries    map[string]*inlineImageEntry
	scope      string
	clock      uint64
	query      int
	capability int // 0 unknown, 1 Kitty, -1 manual attachment only
}
type inlineImageLoaded struct {
	id  int
	img image.Image
	err error
}
type inlineImageEncoded struct {
	id   int
	data string
	err  error
}
type inlineImageDisplayed struct{ id int }
type inlineImageTimeout struct {
	id    int
	query bool
}

func nextInlineImageID() int { return 0x600000 + int(inlineImageSerial.Add(1)&0x0fffff) }

// Reserve the same height before/after decoding and on failure. Async image
// completion therefore cannot move the reading anchor or pagination boundary.
func (m model) inlineCapability() int {
	if m.opts.Images == "off" || m.opts.Images == "blocks" || m.imageTerminal.fallback {
		return -1
	}
	if m.opts.Images == "kitty" || m.imageTerminal.silent {
		return 1
	}
	return m.inlineImages.capability
}

func (m *model) inlineImageLines(p post, key string, width, line int) []string {
	if p.deleted {
		return nil
	}
	return m.inlineMediaLines(m.raw[p.id].Media(), key, min(max(8, 40+m.imageZoom*8), width), max(2, inlineImageRows+m.imageZoom*2), line, 0, true)
}

// Collect visible image candidates even while capability is unknown, but never
// reserve image space or fetch media until native graphics support is established.
func (m *model) inlineMediaLines(items []forum.Media, key string, width, rows, line, limit int, caption bool) []string {
	if m.inlineCapability() < 0 {
		return nil
	}
	var lines []string
	count := 0
	for i, item := range items {
		if item.Type != "image" && !strings.HasPrefix(item.Type, "image/") {
			continue
		}
		u := item.Thumbnail
		if !forum.SafeURL(u) {
			u = item.URL
		}
		if !forum.SafeURL(u) {
			continue
		}
		slot := inlineImageSlot{
			key: fmt.Sprintf("%s:%s:%d:%s:%dx%d", m.opts.Site, key, i, u, width, rows),
			url: u, original: item.URL, line: line + len(lines), width: width, height: rows,
		}
		m.imageSlots = append(m.imageSlots, slot)
		count++
		if m.inlineCapability() == 1 {
			body := ink("▧ 正在读取小图…", muted)
			if e := m.inlineImages.entries[slot.key]; e != nil {
				switch {
				case e.err != "" || e.nativeFailed:
					body = ink("▧ a 加载附件", muted)
				case e.ready:
					body = e.content
				}
			}
			lines = append(lines, strings.Split(rectangle(body, width, rows), "\n")...)
			if caption {
				lines = append(lines, ink(clip(fmt.Sprintf("图片 %d · a 查看大图", i+1), width), muted))
			}
		}
		if limit > 0 && count >= limit {
			break
		}
	}
	return lines
}

// A terminal capability reply can add image rows. Preserve the visible post and
// its relative offset when replacing the compact, text-only layout.
func (m *model) refreshImageLayout() {
	key, delta := "", 0
	if m.reading {
		for _, item := range m.readerItems {
			if item.line <= m.reader.YOffset() {
				key, delta = item.key, m.reader.YOffset()-item.line
			}
		}
	}
	m.refreshReader(false)
	if key != "" {
		for _, item := range m.readerItems {
			if item.key == key {
				m.reader.SetYOffset(min(item.line+delta, max(item.line, item.end-1)))
				break
			}
		}
	}
}

func (m *model) clearInlineImages() string {
	var out strings.Builder
	for _, e := range m.inlineImages.entries {
		if e.cancel != nil {
			e.cancel()
		}
		if e.sent {
			out.WriteString(media.KittyDelete(e.id))
		}
	}
	// Keep the terminal capability across thread/site changes.
	m.inlineImages.entries = nil
	m.inlineImages.scope = ""
	return out.String()
}

func (m *model) reconcileInlineImages() tea.Cmd {
	s := &m.inlineImages
	scope := ""
	if t := m.current(); t != nil && (m.reading || m.split()) && m.opts.Images != "off" {
		scope = fmt.Sprintf("%s:%s:%d:%t", m.opts.Site, m.opts.ForumURL, t.id, m.reading)
		if !m.reading {
			scope += fmt.Sprintf(":%d", m.requestID)
		}
	}
	var tasks []tea.Cmd
	if scope != s.scope {
		if data := m.clearInlineImages(); data != "" {
			tasks = append(tasks, m.kittyRaw(data))
		}
		s.scope = scope
		m.refreshReader(false)
	}
	if scope == "" || m.modal != "" || m.busy {
		return tea.Batch(tasks...)
	}
	if s.entries == nil {
		s.entries = map[string]*inlineImageEntry{}
	}
	s.capability = m.inlineCapability()
	// One viewport of prefetch on each side; cap requests and decoded images.
	top, bottom := m.reader.YOffset()-m.reader.Height(), m.reader.YOffset()+2*m.reader.Height()
	if !m.reading {
		top, bottom = 0, m.reader.Height()
	}
	wanted := map[string]bool{}
	var slots []inlineImageSlot
	for _, slot := range m.imageSlots {
		if slot.line+slot.height > top && slot.line < bottom && len(slots) < inlineImageCacheLimit {
			wanted[slot.key] = true
			slots = append(slots, slot)
		}
	}
	if len(slots) > 0 && s.capability == 0 && s.query == 0 {
		s.query = nextInlineImageID()
		id := s.query
		tasks = append(tasks, tea.Sequence(m.kittyRaw(media.KittyQuery(id)), tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return inlineImageTimeout{id, true} })))
	}
	if s.capability != 1 {
		if len(s.entries) > 0 {
			savedScope := s.scope
			if data := m.clearInlineImages(); data != "" {
				tasks = append(tasks, m.kittyRaw(data))
			}
			s.scope = savedScope
			m.refreshImageLayout()
		}
		return tea.Batch(tasks...)
	}
	changed := false
	loading := 0
	for key, e := range s.entries {
		if e.loading && !wanted[key] {
			e.cancel()
			delete(s.entries, key)
			changed = true
		} else if e.loading {
			loading++
		}
	}
	for _, slot := range slots {
		s.clock++
		if e := s.entries[slot.key]; e != nil {
			e.touched = s.clock
			continue
		}
		var cached image.Image
		for _, e := range s.entries {
			if e.url == slot.url && e.img != nil {
				cached = e.img
				break
			}
		}
		if cached == nil && loading >= 2 {
			continue
		}
		if len(s.entries) >= inlineImageCacheLimit {
			oldest := ""
			for key, e := range s.entries {
				if !wanted[key] && (oldest == "" || e.touched < s.entries[oldest].touched) {
					oldest = key
				}
			}
			if oldest == "" {
				continue
			}
			e := s.entries[oldest]
			if e.sent {
				tasks = append(tasks, m.kittyRaw(media.KittyDelete(e.id)))
			}
			delete(s.entries, oldest)
		}
		id := nextInlineImageID()
		if cached != nil {
			cols, rows := media.FitCells(cached, slot.width, slot.height)
			e := &inlineImageEntry{id: id, url: slot.url, img: cached, cols: cols, rows: rows, touched: s.clock}
			s.entries[slot.key] = e
			tasks = append(tasks, m.encodeInlineImage(e))
			changed = true
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		s.entries[slot.key] = &inlineImageEntry{id: id, url: slot.url, cancel: cancel, loading: true, cols: slot.width, rows: slot.height, touched: s.clock}
		loading++
		changed = true
		tasks = append(tasks, func() tea.Msg {
			defer cancel()
			select {
			case inlineImageWorkers <- struct{}{}:
				defer func() { <-inlineImageWorkers }()
			case <-ctx.Done():
				return inlineImageLoaded{id: id, err: ctx.Err()}
			}
			img, err := media.LoadThumbnail(ctx, slot.url)
			if err != nil && ctx.Err() == nil && slot.url != slot.original && forum.SafeURL(slot.original) {
				img, err = media.LoadThumbnail(ctx, slot.original)
			}
			return inlineImageLoaded{id, img, err}
		})
	}
	if changed {
		m.refreshReader(false)
	}
	return tea.Batch(tasks...)
}

func (m *model) findInlineImage(id int) *inlineImageEntry {
	for _, e := range m.inlineImages.entries {
		if e.id == id {
			return e
		}
	}
	return nil
}
func (m *model) renderInlineImage(e *inlineImageEntry) {
	if e.img == nil {
		return
	}
	if e.ready {
		e.content = media.KittyCells(e.id, e.cols, e.rows)
	} else {
		e.content = ""
	}
}
func (m *model) encodeInlineImage(e *inlineImageEntry) tea.Cmd {
	if e.img == nil || e.sent || e.nativeFailed || m.inlineImages.capability != 1 {
		return nil
	}
	e.sent = true
	id, img, cols, rows := e.id, e.img, e.cols, e.rows
	return func() tea.Msg {
		data, err := media.KittyUpload(img, id, cols, rows)
		return inlineImageEncoded{id, data, err}
	}
}
func (m *model) inlineImagesUpdate(msg tea.Msg) (tea.Cmd, bool) {
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		if m.modal != "" || m.busy || (!m.reading && !m.split()) {
			return nil, false
		}
		delta := 0
		switch v.String() {
		case "+", "=":
			delta = 1
		case "-":
			delta = -1
		default:
			return nil, false
		}
		if m.inlineCapability() != 1 || len(m.imageSlots) == 0 {
			return nil, false
		}
		zoom := max(-2, min(3, m.imageZoom+delta))
		if zoom == m.imageZoom {
			return nil, true
		}
		m.imageZoom = zoom
		m.refreshImageLayout()
		m.notice = "小图大小已调整 · + / - 缩放 · a 查看原图"
		return nil, true
	case inlineImageLoaded:
		e := m.findInlineImage(v.id)
		if e == nil {
			return nil, true
		}
		e.loading, e.cancel = false, nil
		if v.err != nil {
			e.err = v.err.Error()
		} else {
			e.img = v.img
			e.cols, e.rows = media.FitCells(e.img, e.cols, e.rows)
			m.renderInlineImage(e)
		}
		m.refreshReader(false)
		return m.encodeInlineImage(e), true
	case inlineImageEncoded:
		e := m.findInlineImage(v.id)
		if e == nil {
			return nil, true
		}
		if v.err != nil {
			e.nativeFailed = true
			e.sent = false
			m.refreshReader(false)
			return nil, true
		}
		if m.imageTerminal.silent {
			return tea.Sequence(m.kittyRaw(m.kittyUploadData(v.data)), func() tea.Msg { return inlineImageDisplayed{v.id} }), true
		}
		return tea.Sequence(m.kittyRaw(v.data), tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return inlineImageTimeout{v.id, false} })), true
	case inlineImageDisplayed:
		if e := m.findInlineImage(v.id); e != nil && !e.nativeFailed && m.inlineImages.capability == 1 {
			e.ready = true
			m.renderInlineImage(e)
			m.refreshReader(false)
			return nil, true
		}
		return m.kittyRaw(media.KittyDelete(v.id)), true
	case inlineImageTimeout:
		if v.query {
			if v.id == m.inlineImages.query && m.inlineImages.capability == 0 {
				m.inlineImages.capability = -1
				m.refreshImageLayout()
			}
			return nil, true
		}
		if e := m.findInlineImage(v.id); e != nil && !e.ready {
			e.nativeFailed = true
			m.refreshReader(false)
			return m.kittyRaw(media.KittyDelete(v.id)), true
		}
		return nil, true
	case uv.KittyGraphicsEvent:
		id := v.Options.ID
		if id < 0x600000 || id > 0x6fffff {
			return nil, false
		}
		if id == m.inlineImages.query {
			if m.inlineImages.capability != 0 {
				return nil, true
			}
			m.inlineImages.capability = -1
			var tasks []tea.Cmd
			if string(v.Payload) == "OK" {
				m.inlineImages.capability = 1
				for _, e := range m.inlineImages.entries {
					if cmd := m.encodeInlineImage(e); cmd != nil {
						tasks = append(tasks, cmd)
					}
				}
			}
			m.refreshImageLayout()
			return tea.Batch(tasks...), true
		}
		e := m.findInlineImage(id)
		if e == nil {
			return m.kittyRaw(media.KittyDelete(id)), true
		}
		if string(v.Payload) != "OK" || e.nativeFailed {
			e.nativeFailed = true
			e.ready = false
			m.renderInlineImage(e)
			m.refreshReader(false)
			return m.kittyRaw(media.KittyDelete(id)), true
		}
		if !e.sent {
			return nil, true
		}
		e.ready = true
		m.renderInlineImage(e)
		m.refreshReader(false)
		return nil, true
	}
	return nil, false
}

// Run reconciliation after navigation and async results, so downloads never
// originate in View and obsolete results cannot revive another thread's images.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd, handled := m.inlineImagesUpdate(msg)
	if !handled {
		next, c := m.updatePersistent(msg)
		m, cmd = next.(model), c
	}
	more := m.reconcileInlineImages()
	if cmd == nil {
		return m, more
	}
	if more == nil {
		return m, cmd
	}
	return m, tea.Batch(cmd, more)
}
