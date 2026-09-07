package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestInlineImageLoadingAndLimits(t *testing.T) {
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 2)))
	huge := append([]byte(nil), pngData.Bytes()...)
	binary.BigEndian.PutUint32(huge[16:20], 10000)
	binary.BigEndian.PutUint32(huge[20:24], 10000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials forwarded to media")
		}
		switch r.URL.Path {
		case "/ok":
			w.Write(pngData.Bytes())
		case "/huge":
			w.Write(huge)
		default:
			w.Write([]byte("not an image"))
		}
	}))
	defer server.Close()
	img, err := LoadImage(context.Background(), server.URL+"/ok")
	if err != nil || img.Bounds().Dx() != 4 {
		t.Fatalf("decode failed: %v", err)
	}
	for _, path := range []string{"/huge", "/bad"} {
		if _, err = LoadImage(context.Background(), server.URL+path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestInlineFitAndTransparency(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	cols, rows := FitCells(img, 80, 20)
	if cols != 80 || rows != 20 {
		t.Fatalf("distorted aspect %d x %d", cols, rows)
	}
	content := Blocks(img, cols, rows)
	if !strings.Contains(content, "38;2;16;31;40;") {
		t.Fatal("transparent image lost TUI background")
	}
	for _, line := range strings.Split(content, "\n") {
		if ansi.StringWidth(line) != cols {
			t.Fatal("blocks overflow")
		}
	}
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
}

func TestKittyVirtualPlacement(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	data, err := KittyUpload(img, 0x500001, 4, 1)
	if err != nil || !strings.Contains(data, "a=T,U=1") || !strings.Contains(data, "i=5242881,p=1,c=4,r=1") {
		t.Fatal("not a virtual image placement")
	}
	cells := KittyCells(0x500001, 4, 2)
	if strings.Count(cells, string(kitty.Placeholder)) != 8 {
		t.Fatal("missing placeholders")
	}
	for _, line := range strings.Split(cells, "\n") {
		if ansi.StringWidth(line) != 4 {
			t.Fatal("diacritics changed cell width")
		}
	}
	if KittyDelete(0x500001) != "\x1b_Ga=d,d=I,i=5242881,q=2\x1b\\" {
		t.Fatal("delete must target only owned image")
	}
}

func TestTmuxTransportChunks(t *testing.T) {
	// Incompressible pixels produce more than one 4096-byte protocol chunk.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var n uint32 = 123
	for i := range img.Pix {
		n = n*1664525 + 1013904223
		img.Pix[i] = byte(n >> 24)
	}
	raw, err := KittyUpload(img, 0x600001, 40, 6)
	if err != nil {
		t.Fatal(err)
	}
	chunks := strings.Count(raw, "\x1b_G")
	if chunks < 2 {
		t.Fatal("fixture did not exercise multiple chunks")
	}
	wrapped := KittyTransport(raw, true)
	if strings.Count(wrapped, "\x1bPtmux;") != chunks {
		t.Fatal("each upload chunk requires a separate DCS")
	}
	var decoded strings.Builder
	for _, part := range strings.Split(wrapped, "\x1bPtmux;")[1:] {
		if !strings.HasSuffix(part, "\x1b\\") {
			t.Fatal("missing outer ST")
		}
		inner := strings.TrimSuffix(part, "\x1b\\")
		decoded.WriteString(strings.ReplaceAll(inner, "\x1b\x1b", "\x1b"))
	}
	if decoded.String() != raw || KittyTransport(raw, false) != raw {
		t.Fatal("transport corrupted payload")
	}
	for _, data := range []string{KittyQuery(12), KittyDelete(12), KittyResize(12, 4, 2)} {
		want := "\x1bPtmux;" + strings.ReplaceAll(data, "\x1b", "\x1b\x1b") + "\x1b\\"
		if KittyTransport(data, true) != want {
			t.Fatal("control command not escaped")
		}
	}
}

func TestThumbnailBoundsWithoutChangingFullPreview(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 800, 600))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data.Bytes()) }))
	defer server.Close()
	thumb, err := LoadThumbnail(context.Background(), server.URL)
	if err != nil || thumb.Bounds().Dx() != 256 || thumb.Bounds().Dy() != 192 {
		t.Fatalf("thumbnail bounds: %v %v", thumb, err)
	}
	full, err := LoadImage(context.Background(), server.URL)
	if err != nil || full.Bounds().Dx() != 800 || full.Bounds().Dy() != 600 {
		t.Fatal("small-image limits reduced original preview")
	}
}

func TestZoomedImageRegions(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	blocks := BlocksRegion(img, 8, 4, 3, 1, 2, 1)
	if !strings.Contains(blocks, "38;2;3;2;0;48;2;3;3;0") || ansi.StringWidth(blocks) != 2 {
		t.Fatal("cropped blocks sampled wrong source pixels")
	}
	cells := KittyCellsRegion(0x500001, 3, 4, 2, 1)
	first := string(kitty.Placeholder) + string(kitty.Diacritic(4)) + string(kitty.Diacritic(3))
	if !strings.HasPrefix(ansi.Strip(cells), first) || ansi.StringWidth(cells) != 2 {
		t.Fatal("native crop lost full placement coordinates")
	}
}
