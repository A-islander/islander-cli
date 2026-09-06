package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/charmbracelet/x/term"
)

func Open(u string) error {
	if !forum.SafeURL(u) {
		return errors.New("附件链接无效")
	}
	name := "xdg-open"
	args := []string{u}
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	if runtime.GOOS == "windows" {
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", u}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e := exec.CommandContext(ctx, name, args...).Run(); e != nil {
		return errors.New("无法调用系统应用，请复制附件链接手动打开")
	}
	return nil
}
func fetch(ctx context.Context, u string) ([]byte, error) {
	if !forum.SafeURL(u) {
		return nil, errors.New("附件链接无效")
	}
	c := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || !forum.SafeURL(r.URL.String()) {
			return errors.New("附件重定向无效")
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return nil, e
	}
	resp, e := c.Do(req)
	if e != nil {
		return nil, errors.New("附件下载失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("附件返回 HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, forum.MaxFile+1))
	if e != nil {
		return nil, e
	}
	if len(b) > forum.MaxFile {
		return nil, errors.New("附件超过 20 MB，请使用外部应用打开")
	}
	return b, nil
}
func Download(ctx context.Context, u, directory string) (string, error) {
	b, e := fetch(ctx, u)
	if e != nil {
		return "", e
	}
	if directory == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		directory = filepath.Join(home, "Downloads", "islander")
	}
	if e = os.MkdirAll(directory, 0700); e != nil {
		return "", e
	}
	parsed, _ := url.Parse(u)
	ext := filepath.Ext(parsed.Path)
	if len(ext) > 10 || strings.ContainsAny(ext, "\\/:\x00") {
		ext = ""
	}
	f, e := os.CreateTemp(directory, "attachment-*"+ext)
	if e != nil {
		return "", e
	}
	name := f.Name()
	if _, e = f.Write(b); e != nil {
		f.Close()
		os.Remove(name)
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	return name, nil
}
func PreviewCmd(u, mode string, done func(error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		// ExecProcess temporarily releases Bubble Tea's terminal and restores it on return.
		return previewStart{u, mode, done}
	}
}

type previewStart struct {
	URL, Mode string
	Done      func(error) tea.Msg
}

func Handle(msg tea.Msg) (tea.Cmd, bool) {
	p, ok := msg.(previewStart)
	if !ok {
		return nil, false
	}
	exe, e := os.Executable()
	if e != nil {
		return func() tea.Msg { return p.Done(e) }, true
	}
	return tea.ExecProcess(exec.Command(exe, "_image", p.URL, p.Mode), func(err error) tea.Msg {
		if err != nil {
			return p.Done(errors.New("此附件无法在终端预览；请按 o 使用外部应用打开，或 s 下载"))
		}
		return p.Done(nil)
	}), true
}

// Viewer runs as a child process so graphic placements cannot race the TUI renderer.
func Viewer(u, mode string) error {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return errors.New("图片预览需要终端")
	}
	if mode == "off" {
		return errors.New("终端图片已关闭；请按 o 外部打开或 s 下载")
	}
	fmt.Println("正在读取图片…")
	b, e := fetch(context.Background(), u)
	if e != nil {
		return e
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil {
		return errors.New("此附件不是支持的图片，请按 o 使用外部应用打开")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
		return errors.New("图片尺寸过大，请使用外部应用打开")
	}
	img, _, e := image.Decode(bytes.NewReader(b))
	if e != nil {
		return e
	}
	w, h, e := term.GetSize(os.Stdout.Fd())
	if e != nil {
		return e
	}
	w = max(10, w-4)
	h = max(4, h-5)
	// Keep memory and transmission bounded, and fit the approximate terminal aspect ratio.
	maxW, maxH := min(1600, w*9), min(1200, h*18)
	scale := min(float64(maxW)/float64(cfg.Width), float64(maxH)/float64(cfg.Height))
	if scale < 1 {
		nw, nh := max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))
		small := image.NewRGBA(image.Rect(0, 0, nw, nh))
		for y := 0; y < nh; y++ {
			for x := 0; x < nw; x++ {
				small.Set(x, y, img.At(img.Bounds().Min.X+x*cfg.Width/nw, img.Bounds().Min.Y+y*cfg.Height/nh))
			}
		}
		img = small
	}
	state, e := term.MakeRaw(os.Stdin.Fd())
	if e != nil {
		return e
	}
	defer term.Restore(os.Stdin.Fd(), state)
	fmt.Print("\x1b[2J\x1b[H")
	input := make(chan string, 16)
	go func() {
		for {
			buf := make([]byte, 1024)
			n, e := os.Stdin.Read(buf)
			if n > 0 {
				input <- string(buf[:n])
			}
			if e != nil {
				close(input)
				return
			}
		}
	}()

	// Explicit query is required: TERM alone is unreliable through SSH and multiplexers.
	if mode == "auto" || mode == "kitty" {
		fmt.Print("\x1b_Gi=913,a=q,t=d,f=24,s=1,v=1;AAAA\x1b\\")
		var answer string
		timeout := time.NewTimer(600 * time.Millisecond)
	query:
		for {
			select {
			case part, ok := <-input:
				if !ok {
					break query
				}
				answer += part
				if strings.Contains(answer, "OK") || strings.Contains(answer, "ERROR") {
					break query
				}
			case <-timeout.C:
				break query
			}
		}
		timeout.Stop()
		if strings.Contains(answer, "OK") || mode == "kitty" {
			var pngData bytes.Buffer
			if e = png.Encode(&pngData, img); e != nil {
				return e
			}
			encoded := base64.StdEncoding.EncodeToString(pngData.Bytes())
			for offset := 0; offset < len(encoded); offset += 4096 {
				end := min(len(encoded), offset+4096)
				more := 0
				if end < len(encoded) {
					more = 1
				}
				header := fmt.Sprintf("m=%d", more)
				if offset == 0 {
					header = fmt.Sprintf("a=T,f=100,t=d,i=914,q=2,C=1,c=%d,r=%d,m=%d", max(1, img.Bounds().Dx()/9), max(1, img.Bounds().Dy()/18), more)
				}
				fmt.Printf("\x1b_G%s;%s\x1b\\", header, encoded[offset:end])
			}
			defer fmt.Print("\x1b_Ga=d,d=I,i=914,q=2\x1b\\")
			fmt.Printf("\x1b[%d;1H任意键返回\r\n", h+2)
			<-input
			return nil
		}
	}
	if mode == "auto" {
		fmt.Print("终端未确认图片协议，显示字符缩略图。\r\n")
	}
	// Color half-block fallback works without terminal image protocols.
	cols, rows := min(w, 80), min(h-2, 24)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			a := img.At(img.Bounds().Min.X+x*img.Bounds().Dx()/cols, img.Bounds().Min.Y+(y*2)*img.Bounds().Dy()/(rows*2))
			b := img.At(img.Bounds().Min.X+x*img.Bounds().Dx()/cols, img.Bounds().Min.Y+(y*2+1)*img.Bounds().Dy()/(rows*2))
			r, g, bl, _ := a.RGBA()
			rr, gg, bb, _ := b.RGBA()
			fmt.Printf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", r>>8, g>>8, bl>>8, rr>>8, gg>>8, bb>>8)
		}
		fmt.Print("\x1b[0m\r\n")
	}
	fmt.Print("字符预览 · 任意键返回\r\n")
	<-input
	return nil
}
