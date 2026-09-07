package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/charmbracelet/x/ansi/kitty"
)

// LoadImage uses the same unauthenticated media transport as downloads. Decode
// limits apply before allocating pixels; callers own the bounded decoded image.
func LoadImage(ctx context.Context, u string) (image.Image, error) {
	return loadImage(ctx, u, 1600, 1200)
}

// LoadThumbnail bounds decoded cache entries even when a site has no thumbnail URL.
func LoadThumbnail(ctx context.Context, u string) (image.Image, error) {
	return loadImage(ctx, u, 320, 192)
}

func loadImage(ctx context.Context, u string, maxWidth, maxHeight int) (image.Image, error) {
	b, err := fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("此附件不是支持的图片；按 o 外部打开，或 s 下载")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
		return nil, errors.New("图片尺寸过大；按 o 外部打开")
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("图片解码失败")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	scale := min(1.0, min(float64(maxWidth)/float64(cfg.Width), float64(maxHeight)/float64(cfg.Height)))
	if scale < 1 {
		w, h := max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))
		out := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				out.Set(x, y, img.At(img.Bounds().Min.X+x*cfg.Width/w, img.Bounds().Min.Y+y*cfg.Height/h))
			}
		}
		img = out
	}
	return img, nil
}

// Terminal cells are approximately twice as tall as they are wide.
func FitCells(img image.Image, maxCols, maxRows int) (int, int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	cols, rows := max(1, min(maxCols, 200)), max(1, min(maxRows, 100))
	scale := min(float64(cols)/float64(w), float64(rows*2)/float64(h))
	return max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale/2))
}

func Blocks(img image.Image, cols, rows int) string {
	return BlocksRegion(img, cols, rows, 0, 0, cols, rows)
}

// BlocksRegion samples only the visible cells of a larger scaled image.
func BlocksRegion(img image.Image, cols, rows, left, top, width, height int) string {
	var out strings.Builder
	b := img.Bounds()
	// Composite transparent pixels on the same background as the TUI.
	rgb := func(c color.Color) (uint32, uint32, uint32) {
		r, g, b, a := c.RGBA()
		return (r + 0x10*(65535-a)/255) >> 8, (g + 0x1f*(65535-a)/255) >> 8, (b + 0x28*(65535-a)/255) >> 8
	}
	for y := top; y < top+height; y++ {
		for x := left; x < left+width; x++ {
			a := img.At(b.Min.X+x*b.Dx()/cols, b.Min.Y+y*2*b.Dy()/(rows*2))
			z := img.At(b.Min.X+x*b.Dx()/cols, b.Min.Y+(y*2+1)*b.Dy()/(rows*2))
			r, g, bl := rgb(a)
			rr, gg, bb := rgb(z)
			fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", r, g, bl, rr, gg, bb)
		}
		out.WriteString("\x1b[0m")
		if y+1 < top+height {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// A virtual placement follows ordinary text cells through the TUI renderer;
// there are no cursor-position writes racing Bubble Tea's screen updates.
// https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders
func KittyUpload(img image.Image, id, cols, rows int) (string, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(b.Bytes())
	var out strings.Builder
	for offset := 0; offset < len(encoded); offset += 4096 {
		end := min(len(encoded), offset+4096)
		more := 0
		if end < len(encoded) {
			more = 1
		}
		header := fmt.Sprintf("m=%d", more)
		if offset == 0 {
			header = fmt.Sprintf("a=T,U=1,f=100,t=d,i=%d,p=1,c=%d,r=%d,q=0,m=%d", id, cols, rows, more)
		}
		fmt.Fprintf(&out, "\x1b_G%s;%s\x1b\\", header, encoded[offset:end])
	}
	return out.String(), nil
}
func KittyCells(id, cols, rows int) string {
	return KittyCellsRegion(id, 0, 0, cols, rows)
}

// Keep the original row/column diacritics when clipping a zoomed placement.
func KittyCellsRegion(id, left, top, cols, rows int) string {
	var out strings.Builder
	for y := top; y < top+rows; y++ {
		fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm", (id>>16)&255, (id>>8)&255, id&255)
		for x := left; x < left+cols; x++ {
			out.WriteRune(kitty.Placeholder)
			out.WriteRune(kitty.Diacritic(y))
			out.WriteRune(kitty.Diacritic(x))
		}
		out.WriteString("\x1b[0m")
		if y+1 < top+rows {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
func KittyDelete(id int) string { return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id) }
func KittyQuery(id int) string  { return fmt.Sprintf("\x1b_Gi=%d,a=q,t=d,f=24,s=1,v=1;AAAA\x1b\\", id) }
func KittyResize(id, cols, rows int) string {
	return fmt.Sprintf("\x1b_Ga=p,U=1,i=%d,p=1,c=%d,r=%d,q=2\x1b\\", id, cols, rows)
}

// KittyTransport wraps each APC separately: a multi-chunk upload must not become
// one unbounded tmux DCS. All inner ESC bytes must be doubled, including ST.
func KittyTransport(data string, tmux bool) string {
	if !tmux {
		return data
	}
	var out strings.Builder
	for _, part := range strings.SplitAfter(data, "\x1b\\") {
		if part != "" {
			out.WriteString("\x1bPtmux;")
			out.WriteString(strings.ReplaceAll(part, "\x1b", "\x1b\x1b"))
			out.WriteString("\x1b\\")
		}
	}
	return out.String()
}
