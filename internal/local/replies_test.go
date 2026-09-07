package local

import (
	"context"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/A-islander/islander-cli/internal/forum"
)

func TestExternalPublishFailureKeepsDraftAndUploadProgress(t *testing.T) {
	for _, site := range []string{"x", "bog"} {
		for _, newThread := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/new=%t", site, newThread), func(t *testing.T) {
				boardID, threadID := 0, 100
				if newThread {
					threadID = 0
					boardID = 4
					if site == "bog" {
						boardID = int(crc32.ChecksumIEEE([]byte("综合版")) & 0x7fffffff)
					}
				}
				uploads, posts := 0, 0
				succeed := false
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "GET" {
						if newThread {
							switch r.URL.Path {
							case "/getForumList":
								fmt.Fprint(w, `[{"forums":[{"id":4,"name":"综合版1"}]}]`)
							case "/f/时间线/1":
								fmt.Fprint(w, `<div class="forum-list"><a href="/f/综合版">综合版</a></div><div class="item-list"></div><div class="pages"></div>`)
							case "/f/综合版1":
								fmt.Fprint(w, `<form method="post" action="/Home/Forum/doPostThread.html"><input type="hidden" name="fid" value="4"><input type="hidden" name="__hash__" value="token"><textarea name="content"></textarea><input type="file" name="image"></form>`)
							case "/f/综合版/1":
								fmt.Fprint(w, `<form method="post" action="/post"><div class="compose-title"><span>综合版</span></div><input type="hidden" name="forum" value="1"><textarea name="comment"></textarea></form>`)
							default:
								t.Errorf("unexpected new-thread GET: %s", r.URL.Path)
							}
							return
						}

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
				d, err := store.Publish(context.Background(), client, forum.Draft{Cookie: "daily", ThreadID: threadID, BoardID: boardID, Body: "reply", Files: []string{path}})
				if err == nil || d.Sent {
					t.Fatal("failed reply marked sent")
				}
				drafts, err := store.Drafts("daily")
				if err != nil || len(drafts) != 1 || drafts[0].ThreadID != threadID || drafts[0].BoardID != boardID {
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
}
