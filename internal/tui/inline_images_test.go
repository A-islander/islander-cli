package tui

import (
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func inlineReaderFixture(site string, count int) model {
	m := newModel()
	m.opts = Options{Site: site, Images: "kitty"}
	root := forum.Post{ID: 100, Body: "主楼"}
	posts := []forum.Post{root}
	for i := 0; i < count; i++ {
		posts = append(posts, forum.Post{ID: 101 + i, FollowID: 100, Body: strings.Repeat("很长的回复。", 8), Attachments: []forum.Media{{Type: "image", URL: "https://example.test/original.png", Thumbnail: "https://example.test/thumb.jpg"}}})
	}
	m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, List: posts}})
	m.reconcileInlineImages() // Commands not run; model tests inject completions.
	return m
}

func TestInlineImagesStableLayoutAndBoundedLoading(t *testing.T) {
	for _, site := range []string{"islander", "x", "bog"} {
		t.Run(site, func(t *testing.T) {
			m := inlineReaderFixture(site, 50)
			defer func() { m.clearInlineImages() }()
			if len(m.imageSlots) != 50 || len(m.inlineImages.entries) != 2 {
				t.Fatal("not lazy/concurrency bounded")
			}
			if m.imageSlots[0].url != "https://example.test/thumb.jpg" {
				t.Fatal("did not prefer thumbnail")
			}
			m.movePost(1)
			beforeOffset, beforeEnd := m.reader.YOffset(), m.readerItems[1].end
			e := m.inlineImages.entries[m.imageSlots[0].key]
			id := e.id
			e.cancel()
			next, _ := m.Update(inlineImageLoaded{id, image.NewRGBA(image.Rect(0, 0, 80, 60)), nil})
			m = next.(model)
			if m.reader.YOffset() != beforeOffset || m.readerItems[1].end != beforeEnd || m.activePost != 1 {
				t.Fatal("decode moved reader")
			}
			m.inlineImagesUpdate(uv.KittyGraphicsEvent{Options: kitty.Options{ID: id}, Payload: []byte("OK")})
			if !strings.ContainsRune(m.reader.View(), kitty.Placeholder) {
				t.Fatal("thumbnail not in thread")
			}
			// Move through a long thread, filling the cache without any real HTTP.
			for i := 1; i < 50; i++ {
				m.movePost(i - m.activePost)
				m.reconcileInlineImages()
				pending := []int{}
				for _, e := range m.inlineImages.entries {
					if e.loading {
						e.cancel()
						pending = append(pending, e.id)
					}
				}
				if len(pending) > 2 || len(m.inlineImages.entries) > inlineImageCacheLimit {
					t.Fatal("cache/concurrency limit exceeded")
				}
				for _, id := range pending {
					m.inlineImagesUpdate(inlineImageLoaded{id, image.NewRGBA(image.Rect(0, 0, 4, 2)), nil})
				}
			}
		})
	}
}

func TestInlineImagesDoNotSkipTallReplyAndPreserveSize(t *testing.T) {
	m := inlineReaderFixture("x", 2)
	defer func() { m.clearInlineImages() }()
	m.resize(60, 16)
	m.movePost(1)
	start := m.reader.YOffset()
	for i := 0; i < 4; i++ {
		m = press(m, "j")
		if m.activePost != 1 {
			t.Fatal("skipped image before reaching reply end")
		}
	}
	if m.reader.YOffset() <= start {
		t.Fatal("did not scroll inside image post")
	}
	for _, size := range [][2]int{{120, 36}, {60, 24}, {44, 16}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(model)
		for _, e := range m.inlineImages.entries {
			if e.loading {
				e.cancel()
				m.inlineImagesUpdate(inlineImageLoaded{e.id, image.NewRGBA(image.Rect(0, 0, 80, 80)), nil})
			}
		}
		view := m.View().Content
		if lipgloss.Height(view) != size[1] {
			t.Fatal("image expanded frame vertically")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("image expanded frame horizontally")
			}
		}
	}
}

func TestInlineNativePlacementTimeoutAndStaleReplies(t *testing.T) {
	m := inlineReaderFixture("x", 1)
	defer func() { m.clearInlineImages() }()
	m.opts.Images = "kitty"
	m.inlineImages.capability = 1
	var e *inlineImageEntry
	for _, entry := range m.inlineImages.entries {
		e = entry
	}
	e.cancel()
	m.movePost(1)
	cmd, _ := m.inlineImagesUpdate(inlineImageLoaded{e.id, image.NewRGBA(image.Rect(0, 0, 80, 60)), nil})
	if cmd == nil {
		t.Fatal("native encoding not started")
	}
	m.inlineImagesUpdate(cmd())
	event := uv.KittyGraphicsEvent{Options: kitty.Options{ID: e.id, PlacementID: 1}, Payload: []byte("OK")}
	m.inlineImagesUpdate(event)
	if !e.ready || !strings.ContainsRune(m.View().Content, kitty.Placeholder) {
		t.Fatal("inline placeholder missing")
	}
	// Foreground encodes the image ID; compositor must preserve it while clipping.
	canvas := lipgloss.NewCanvas(m.width, m.height).Compose(lipgloss.NewLayer(m.View().Content))
	found := false
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			c := canvas.CellAt(x, y)
			if c != nil && strings.ContainsRune(c.Content, kitty.Placeholder) {
				r, g, b, _ := c.Style.Fg.RGBA()
				if int(r>>8)<<16|int(g>>8)<<8|int(b>>8) != e.id {
					t.Fatal("image ID changed in compositor")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("native image not visible")
	}
	e.ready = false
	m.inlineImagesUpdate(inlineImageTimeout{e.id, false})
	m.inlineImagesUpdate(event)
	if e.ready || !e.nativeFailed {
		t.Fatal("late acknowledgement revived timed-out image")
	}
	oldID := e.id
	m.reading = false
	m.resize(80, 24)
	m.reconcileInlineImages()
	m.inlineImagesUpdate(inlineImageLoaded{oldID, image.NewRGBA(image.Rect(0, 0, 4, 2)), nil})
	if len(m.inlineImages.entries) != 0 {
		t.Fatal("stale decode revived closed thread")
	}
	if cleanup, handled := m.inlineImagesUpdate(event); !handled || cleanup == nil {
		t.Fatal("late response did not clean owned image")
	}
}

func TestInlineImagesQuotesAndOff(t *testing.T) {
	m := inlineReaderFixture("bog", 1)
	defer func() { m.clearInlineImages() }()
	p := m.displayPost(forum.Post{ID: 200, Body: "引用", Attachments: []forum.Media{{Type: "image", URL: "https://example.test/quote.png"}, {Type: "video", URL: "https://example.test/video.mp4"}}})
	m.inlineQuotes["101"] = []inlineQuote{{post: p}}
	m.refreshReader(false)
	if len(m.imageSlots) != 2 || !strings.Contains(m.imageSlots[1].key, "101/200") {
		t.Fatal("quote images missing or video treated as image")
	}
	oldID := m.imageSlots[0].key
	m.opts.Site = "x"
	m.reconcileInlineImages()
	if m.inlineImages.entries[oldID] != nil {
		t.Fatal("site switch reused old site's image")
	}
	m.opts.Images = "off"
	m.reconcileInlineImages()
	if len(m.imageSlots) != 0 || len(m.inlineImages.entries) != 0 || !strings.Contains(m.reader.View(), "附件") {
		t.Fatal("off must preserve attachment actions without previews")
	}
}

func TestTmuxQuietInlineAndAttachmentUpload(t *testing.T) {
	m := inlineReaderFixture("x", 1)
	defer func() { m.clearInlineImages() }()
	m.imageTerminal = imageTerminal{tmux: true, silent: true}
	m.inlineImages.capability = 1
	var e *inlineImageEntry
	for _, entry := range m.inlineImages.entries {
		e = entry
	}
	e.cancel()
	m.inlineImagesUpdate(inlineImageLoaded{e.id, image.NewRGBA(image.Rect(0, 0, 8, 6)), nil})
	m.inlineImagesUpdate(inlineImageDisplayed{e.id})
	if !e.ready {
		t.Fatal("quiet tmux upload waited for unavailable ACK")
	}
	data := m.kittyUploadData("\x1b_Ga=T,q=0,m=0;abc\x1b\\")
	if !strings.Contains(data, ",q=2,") {
		t.Fatal("tmux upload must suppress replies")
	}
	a := attachmentModel()
	defer func() {
		if a.attachment.cancel != nil {
			a.attachment.cancel()
		}
	}()
	a.imageTerminal = m.imageTerminal
	a.attachment.capability = 1
	a.attachmentUpdate(attachmentLoaded{a.attachment.id, image.NewRGBA(image.Rect(0, 0, 8, 6)), nil})
	a.attachmentUpdate(attachmentDisplayed{a.attachment.id})
	if !a.attachment.ready {
		t.Fatal("full image did not support quiet tmux transport")
	}
}

func TestThumbnailHTTPFallbackAndFailedLoadDoesNotRetry(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("media received credentials")
		}
		if r.URL.Path == "/original.png" {
			_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 10, 10)))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	for _, fail := range []bool{false, true} {
		m := newModel()
		m.opts = Options{Site: "islander", Images: "kitty"}
		original := server.URL + "/original.png"
		if fail {
			original = server.URL + "/missing.png"
		}
		root := forum.Post{ID: 100, Body: "图片", Attachments: []forum.Media{{Type: "image", URL: original, Thumbnail: server.URL + "/thumb.png"}}}
		m.applyThread(threadResult{Root: root, Page: forum.Page{Page: 1, List: []forum.Post{root}}})
		cmd := m.reconcileInlineImages()
		if cmd == nil {
			t.Fatal("visible image did not load")
		}
		loaded, ok := cmd().(inlineImageLoaded)
		if !ok {
			t.Fatal("unexpected command")
		}
		if (loaded.err != nil) != fail {
			t.Fatalf("thumbnail fallback failed: %v", loaded.err)
		}
		m.inlineImagesUpdate(loaded)
		if retry := m.reconcileInlineImages(); retry != nil {
			t.Fatal("completed/failed media retried automatically")
		}
		m.clearInlineImages()
	}
	if strings.Join(requests, ",") != "/thumb.png,/original.png,/thumb.png,/missing.png" {
		t.Fatal(requests)
	}
}
