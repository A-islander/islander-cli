package tui

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func attachmentModel() model {
	m := newModel()
	m.opts.Images = "blocks"
	m.reading = true
	m.refreshReader(false)
	m.movePost(2)
	m.menu = []menuItem{{"first", "attachment", "https://example.org/a.png"}, {"second", "attachment", "https://example.org/b.png"}}
	m.openAttachment() // Commands intentionally not executed: no network in this fixture.
	return m
}

func TestInlineAttachmentSwitchAndReturn(t *testing.T) {
	m := attachmentModel()
	originalOffset := m.reader.YOffset()
	originalPost := m.activePost
	oldID := m.attachment.id
	cancelled := false
	cancel := m.attachment.cancel
	m.attachment.cancel = func() { cancelled = true; cancel() }
	m = press(m, "l")
	if m.menuIndex != 1 || m.attachment.id == oldID || !cancelled {
		t.Fatal("switch failed to cancel previous image")
	}
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	next, _ := m.Update(attachmentLoaded{oldID, img, nil})
	m = next.(model)
	if m.attachment.img != nil {
		t.Fatal("stale image replaced selected attachment")
	}
	next, _ = m.Update(attachmentLoaded{m.attachment.id, img, nil})
	m = next.(model)
	if m.attachment.loading || m.attachment.img == nil || !strings.Contains(m.View().Content, "▀") {
		t.Fatal("image did not appear inside TUI")
	}
	m = press(m, "esc")
	if m.modal != "menu" || m.attachment.img != nil || m.reader.YOffset() != originalOffset || m.activePost != originalPost {
		t.Fatal("closing image lost reading position")
	}
}

func TestInlineAttachmentNativeAndFallback(t *testing.T) {
	m := attachmentModel()
	defer m.attachment.cancel()
	m.attachment.capability = 0
	id := m.attachment.id
	next, _ := m.Update(attachmentLoaded{id, image.NewRGBA(image.Rect(0, 0, 40, 20)), nil})
	m = next.(model)
	next, encode := m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: id}, Payload: []byte("OK")})
	m = next.(model)
	if encode == nil {
		t.Fatal("supported terminal did not start image encoding")
	}
	next, send := m.Update(encode())
	m = next.(model)
	if send == nil || !m.attachment.sent {
		t.Fatal("no native image transfer")
	}
	next, _ = m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: id, PlacementID: 1}, Payload: []byte("OK")})
	m = next.(model)
	if !m.attachment.ready {
		t.Fatal("native placement not confirmed")
	}
	view := m.View().Content
	if !strings.ContainsRune(view, kitty.Placeholder) {
		t.Fatal("compositor removed image placeholders")
	}
	canvas := lipgloss.NewCanvas(m.width, m.height).Compose(lipgloss.NewLayer(view))
	found := false
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			c := canvas.CellAt(x, y)
			if c != nil && strings.ContainsRune(c.Content, kitty.Placeholder) {
				r, g, b, _ := c.Style.Fg.RGBA()
				if int(r>>8)<<16|int(g>>8)<<8|int(b>>8) != id {
					t.Fatal("compositor corrupted image ID colour")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("native image not on screen")
	}
	m = press(m, "b")
	if m.attachment.ready || !strings.Contains(m.View().Content, "▀") {
		t.Fatal("manual fallback failed")
	}
	m = press(m, "esc")
	next, cleanup := m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: id}, Payload: []byte("OK")})
	m = next.(model)
	if cleanup == nil || m.modal != "menu" {
		t.Fatal("late graphics response reopened image")
	}
}

func TestAttachmentTimeoutAndScreenSizes(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {60, 24}, {44, 16}} {
		m := attachmentModel()
		cancel := m.attachment.cancel
		cancel()
		m.attachment.capability = 0
		m.attachment.cancel = nil
		next, _ := m.Update(attachmentLoaded{m.attachment.id, image.NewRGBA(image.Rect(0, 0, 90, 70)), nil})
		m = next.(model)
		next, _ = m.Update(attachmentTimeout{m.attachment.id, false})
		m = next.(model)
		if m.attachment.capability != -1 {
			t.Fatal("unknown terminal failed to fall back")
		}
		next, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(model)
		view := m.View().Content
		if lipgloss.Height(view) != size[1] {
			t.Fatal("attachment changed screen height")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("attachment overflowed terminal width")
			}
		}
		if !strings.Contains(view, "Esc 返回") {
			t.Fatal("attachment controls not visible")
		}
	}
}
