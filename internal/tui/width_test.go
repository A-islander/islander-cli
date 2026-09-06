package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestKaomojiFitsBothWidthModels(t *testing.T) {
	for _, face := range []string{"(ﾟДﾟ)", "(ノﾟ∀ﾟ)ノ", "(ﾞДﾞ)", "ᕕ( ᐛ )ᕗ", "(╯°□°）╯︵ ┻━┻"} {
		for _, width := range []int{8, 16, 40} {
			// Check with a second width model: testing only the layout library's
			// own measurement would miss the original terminal overflow.
			content := bodyText(strings.Repeat(face, 12), width)
			for _, line := range strings.Split(content, "\n") {
				if got := ansi.StringWidthWc(line); got > width {
					t.Errorf("%q wrapped to %d exceeds %d", face, got, width)
				}
			}
			frame := panel(content, width+6, 10, true)
			for _, line := range strings.Split(frame, "\n") {
				if got := ansi.StringWidthWc(line); got != width+6 {
					t.Errorf("%q border width = %d, want %d", face, got, width+6)
				}
			}
		}
	}
}

func TestKaomojiFullFramesAndOriginalInput(t *testing.T) {
	const raw = "颜文字 (ﾟДﾟ) (ノﾟ∀ﾟ)ノ (ﾞДﾞ)"
	for _, size := range [][2]int{{120, 36}, {60, 24}, {44, 16}} {
		for _, mode := range []string{"list", "reader", "quote", "compose", "filter"} {
			t.Run(fmt.Sprint(size)+mode, func(t *testing.T) {
				m := newModel()
				m.resize(size[0], size[1])
				post := &m.current().posts[0]
				post.body = strings.Repeat(raw, 10)
				m.current().title = raw
				m.current().excerpt = raw
				m.refreshReader(false)
				switch mode {
				case "reader":
					m.reading = true
				case "quote":
					m.reading = true
					m.inlineQuotes[fmt.Sprint(post.id)] = []inlineQuote{{post: *post}}
					m.refreshReader(false)
				case "compose":
					m.modal = "compose"
					m.editor.SetValue(raw)
					m.titleInput.SetValue(raw)
				case "filter":
					m.startInput("filter")
					m.input.SetValue(raw)
				}
				for n, line := range strings.Split(m.View().Content, "\n") {
					if w := ansi.StringWidthWc(line); w > size[0] {
						t.Errorf("line %d width %d exceeds screen %d", n, w, size[0])
					}
				}
				if post.body != strings.Repeat(raw, 10) || m.current().title != raw {
					t.Fatal("rendering changed forum content")
				}
				if mode == "compose" {
					m.syncDraft()
					if m.draft.Body != raw || m.draft.Title != raw {
						t.Fatal("rendering changed draft input")
					}
				}
			})
		}
	}
}

func TestDisplayPreservesComplexGraphemes(t *testing.T) {
	for _, s := range []string{"👨‍👩‍👧‍👦", "🫶🏽", "e\u0301", "🇨🇳"} {
		if displayText(s) != s || ansi.Strip(clip(s, 2)) != s {
			t.Errorf("split or changed grapheme %q", s)
		}
	}
}
