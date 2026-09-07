package tui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func imagePreviewFixture(site, mode string) model {
	m := newModel()
	m.opts = Options{Site: site, Images: mode}
	media := func(id int) []forum.Media {
		return []forum.Media{{Type: "image", URL: fmt.Sprintf("https://example.test/%d.png", id), Thumbnail: fmt.Sprintf("https://example.test/%d-thumb.png", id)}}
	}
	root := forum.Post{ID: 100, Body: "主楼正文", Title: "图片预览", Attachments: media(100), ReplyCount: 5}
	m.applyPage(forum.Page{Page: 1, List: []forum.Post{root, {ID: 200, Body: "另一串"}}})
	replies := []forum.Post{root}
	for i := 1; i <= 5; i++ {
		replies = append(replies, forum.Post{ID: 100 + i, FollowID: 100, Body: fmt.Sprintf("第%d条回复", i), Attachments: media(100 + i)})
	}
	m.previews = map[int]previewResult{100: {Page: forum.Page{Page: 1, List: replies}}}
	m.refreshReader(false)
	return m
}

func TestThreeSiteImagesInListPreview(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m := imagePreviewFixture(site, "kitty")
			defer func() { m.clearInlineImages() }()
			m.reconcileInlineImages()
			if m.reading || len(m.imageSlots) != 6 || len(m.inlineImages.entries) != 2 {
				t.Fatal("preview did not schedule visible root/reply images")
			}
			before := m.reader.TotalLineCount()
			for step := 0; step < 4; step++ {
				ids := []int{}
				for _, e := range m.inlineImages.entries {
					if e.loading {
						e.cancel()
						ids = append(ids, e.id)
					}
				}
				for _, id := range ids {
					m.inlineImagesUpdate(inlineImageLoaded{id, image.NewRGBA(image.Rect(0, 0, 80, 40)), nil})
					m.inlineImagesUpdate(uv.KittyGraphicsEvent{Options: kitty.Options{ID: id}, Payload: []byte("OK")})
				}
				m.reconcileInlineImages()
			}
			view := m.reader.View()
			if !strings.ContainsRune(view, kitty.Placeholder) || m.reader.TotalLineCount() != before || m.reader.YOffset() != 0 {
				t.Fatal("preview image missing or shifted layout")
			}
			for i := 1; i <= 5; i++ {
				if !strings.Contains(ansi.Strip(view), fmt.Sprintf("第%d条回复", i)) {
					t.Fatalf("image hid reply %d", i)
				}
			}
			if len(m.pages) != 0 || len(m.raw) != 2 {
				t.Fatal("image preview changed full-reader post cache")
			}
			for _, line := range strings.Split(m.View().Content, "\n") {
				if ansi.StringWidth(line) > m.width {
					t.Fatal("preview image overflowed frame")
				}
			}
			old := m.inlineImages.entries[m.imageSlots[0].key].id
			m.moveSelection(1)
			m.reconcileInlineImages()
			m.inlineImagesUpdate(inlineImageLoaded{old, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil})
			if len(m.inlineImages.entries) != 0 || strings.ContainsRune(m.reader.View(), kitty.Placeholder) {
				t.Fatal("old image survived preview selection change")
			}
		})
	}
}

func TestUnsupportedPreviewOnlyHintsAndManualA(t *testing.T) {
	for _, mode := range []string{"auto", "blocks", "off"} {
		t.Run(mode, func(t *testing.T) {
			m := imagePreviewFixture("x", mode)
			m.reconcileInlineImages()
			if len(m.inlineImages.entries) != 0 {
				t.Fatal("download started before native support was established")
			}
			if mode == "auto" {
				if m.inlineImages.query == 0 {
					t.Fatal("native capability was not queried")
				}
				m.inlineImagesUpdate(inlineImageTimeout{m.inlineImages.query, true})
			}
			m.reconcileInlineImages()
			view := m.reader.View()
			if len(m.inlineImages.entries) != 0 || len(m.imageSlots) != 0 || !strings.Contains(view, "a 加载附件") || strings.Contains(view, "▀") || strings.ContainsRune(view, kitty.Placeholder) {
				t.Fatal("unsupported preview should remain compact text only")
			}
			if m.reader.TotalLineCount() > 22 {
				t.Fatal("unsupported preview reserved blank image regions")
			}
			// Remove the root attachment: a must still reach reply-only media.
			root := m.raw[100]
			root.Attachments = nil
			m.raw[100] = root
			result := m.previews[100]
			result.Page.List[0] = root
			m.previews[100] = result
			m.refreshReader(false)
			m = press(m, "a")
			if m.modal != "attachment" || len(m.menu) != 5 || !strings.Contains(m.menu[0].Value, "101.png") {
				t.Fatal("preview a did not include reply attachments")
			}
			if mode != "off" && !m.attachment.loading {
				t.Fatal("a did not start manual attachment loading")
			}
			if m.attachment.cancel != nil {
				m.attachment.cancel()
			}
			m.clearInlineImages()
		})
	}
}

func TestCapabilityReplyPreservesReadingAnchor(t *testing.T) {
	m := inlineReaderFixture("x", 4)
	m.clearInlineImages()
	m.opts.Images = "auto"
	m.inlineImages.capability = 0
	m.refreshReader(false)
	m.movePost(3)
	m.reader.SetYOffset(m.reader.YOffset() + 1)
	beforeDelta := m.reader.YOffset() - m.readerItems[3].line
	m.reconcileInlineImages()
	query := m.inlineImages.query
	if query == 0 || len(m.inlineImages.entries) != 0 {
		t.Fatal("capability probe downloaded images")
	}
	m.inlineImagesUpdate(uv.KittyGraphicsEvent{Options: kitty.Options{ID: query}, Payload: []byte("OK")})
	if m.activePost != 3 || m.reader.YOffset()-m.readerItems[3].line != beforeDelta {
		t.Fatal("capability reply jumped reading anchor")
	}
	m.reconcileInlineImages()
	if len(m.inlineImages.entries) == 0 {
		t.Fatal("supported terminal did not begin downloads")
	}
	m.clearInlineImages()
}

func TestUnsupportedReaderAndHiddenPreviewDoNotLoad(t *testing.T) {
	m := imagePreviewFixture("bog", "kitty")
	m.resize(80, 24)
	if cmd := m.reconcileInlineImages(); cmd != nil || len(m.inlineImages.entries) != 0 {
		t.Fatal("hidden narrow-screen preview downloaded images")
	}
	m = inlineReaderFixture("bog", 3)
	m.clearInlineImages()
	m.opts.Images = "auto"
	m.inlineImages.capability = 0
	m.refreshReader(false)
	m.reconcileInlineImages()
	m.inlineImagesUpdate(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.inlineImages.query}, Payload: []byte("ENOTSUP")})
	m.reconcileInlineImages()
	if len(m.inlineImages.entries) != 0 || len(m.imageSlots) != 0 || !strings.Contains(m.reader.GetContent(), "a 加载附件") || strings.Contains(m.reader.GetContent(), "▀") {
		t.Fatal("unsupported reader did not stay manual")
	}
	m.clearInlineImages()
}

func TestImageZoomReusesPixelsAndPreservesSelection(t *testing.T) {
	for _, preview := range []bool{false, true} {
		m := inlineReaderFixture("x", 1)
		if preview {
			m.clearInlineImages()
			m = imagePreviewFixture("x", "kitty")
			m.reconcileInlineImages()
		} else {
			m.movePost(1)
		}
		slot := m.imageSlots[0]
		e := m.inlineImages.entries[slot.key]
		e.cancel()
		pixels := image.NewRGBA(image.Rect(0, 0, 80, 48))
		m.inlineImagesUpdate(inlineImageLoaded{e.id, pixels, nil})
		m.inlineImagesUpdate(uv.KittyGraphicsEvent{Options: kitty.Options{ID: e.id}, Payload: []byte("OK")})
		beforePost, beforeSelected := m.activePost, m.selected
		m = press(m, "+")
		enlarged := m.imageSlots[0]
		next := m.inlineImages.entries[enlarged.key]
		if m.imageZoom != 1 || enlarged.height <= slot.height || next == nil || next.img != pixels || next.loading {
			t.Fatal("zoom should enlarge cached pixels without another download")
		}
		if m.activePost != beforePost || m.selected != beforeSelected || !strings.Contains(m.View().Content, "+/- 小图") {
			t.Fatal("zoom moved selection or lacked controls")
		}
		m = press(m, "-")
		if m.imageZoom != 0 || m.imageSlots[0].height != slot.height {
			t.Fatal("minus did not restore original size")
		}
		m = press(m, "=")
		if m.imageZoom != 1 {
			t.Fatal("unshifted = did not enlarge")
		}
		m.modal = "compose"
		m.editor.Focus()
		before := m.editor.Value()
		m = press(m, "+")
		m = press(m, "-")
		if m.imageZoom != 1 || m.editor.Value() != before+"+-" {
			t.Fatal("zoom intercepted composer input")
		}
		m.clearInlineImages()
	}
}
