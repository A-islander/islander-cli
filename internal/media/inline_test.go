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
