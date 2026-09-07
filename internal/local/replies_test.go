package local

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
)

func TestExternalReplyFailureKeepsDraftAndUploadProgress(t *testing.T) {
	for _, site := range []string{"x", "bog"} {
		t.Run(site, func(t *testing.T) {
			uploads, posts := 0, 0
			succeed := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if site == "x" {
						fmt.Fprint(w, `<form method="post" action="/Home/Forum/doReplyThread.html"><input type="hidden" name="resto" value="100"><input type="hidden" name="__hash__" value="token"><textarea name="content"></textarea><input type="file" name="image"></form>`)
					} else {
						fmt.Fprint(w, `<form method="post" action="/post"><input type="hidden" name="res" value="100"><textarea name="comment"></textarea></form>`)
					}
					return
				}
				if r.URL.Path == "/post/upload" {
					uploads++
					fmt.Fprint(w, `{"code":200,"pic":"uploaded.png"}`)
					return
				}
				posts++
				if site == "x" {
					if err := r.ParseMultipartForm(forum.MaxFile); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if len(r.MultipartForm.File["image"]) != 1 {
						t.Error("X image not in final submission")
					}
					if succeed {
						fmt.Fprint(w, `<p class="success">回复成功！</p>`)
					} else {
						fmt.Fprint(w, `<p class="error">验证码</p>`)
					}
				} else {
					if succeed {
						fmt.Fprint(w, `{"code":1}`)
					} else {
						fmt.Fprint(w, `{"code":101}`)
					}
				}
			}))
			defer server.Close()
			store, _ := NewSite(t.TempDir(), site, server.URL+"/", "", "file")
			token := "cookie"
			if site == "bog" {
				token = "bog_master=master; bog_sel=shadow"
			}
			client, _ := forum.NewBackend(site, server.URL, "", token)
			path := filepath.Join(t.TempDir(), "image.png")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
				t.Fatal(err)
			}
			f.Close()
			d, err := store.Publish(context.Background(), client, forum.Draft{Cookie: "daily", ThreadID: 100, Body: "reply", Files: []string{path}})
			if err == nil || d.Sent {
				t.Fatal("failed reply marked sent")
			}
			drafts, err := store.Drafts("daily")
			if err != nil || len(drafts) != 1 || drafts[0].ThreadID != 100 {
				t.Fatal("recoverable reply draft lost")
			}
			if site == "x" && (len(d.Files) != 1 || len(d.Media) != 0 || uploads != 0) {
				t.Fatal("X files were separately uploaded or lost")
			}
			if site == "bog" && (len(d.Files) != 0 || len(d.Media) != 1 || uploads != 1) {
				t.Fatal("BOG upload receipt not retained")
			}
			if posts != 1 {
				t.Fatal("failure automatically retried")
			}
			succeed = true
			d, err = store.Publish(context.Background(), client, d)
			if err != nil || !d.Sent || posts != 2 {
				t.Fatalf("explicit retry failed: %v", err)
			}
			if site == "bog" && uploads != 1 {
				t.Fatal("explicit retry reuploaded successful BOG image")
			}
			drafts, err = store.Drafts("daily")
			if err != nil || len(drafts) != 0 {
				t.Fatal("successful reply draft not removed")
			}
		})
	}
}
