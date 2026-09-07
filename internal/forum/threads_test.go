package forum

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const xThreadFormFixture = `<form method="post" action="/Home/Forum/doPostThread.html"><input type="hidden" name="fid" value="4"><input type="hidden" name="__hash__" value="fresh-token"><input name="title" maxlength="100"><textarea name="content" maxlength="10000"></textarea><input type="file" name="image"></form>`
const bogThreadFormFixture = `<form method="post" action="/post"><div class="compose-title"><strong>正在发送至</strong><span>综合版</span></div><input type="hidden" name="forum" value="1"><input name="title" maxlength="50"><textarea name="comment"></textarea></form>`
const bogBoardNavigationFixture = `<div class="forum-list"><a href="/f/综合版">综合版</a></div><div class="item-list"></div><div class="pages"></div>`

func TestExternalNewThreadUsesSelectedBoardAndExistingImageFlow(t *testing.T) {
	for _, site := range []string{"x", "bog"} {
		for _, withImage := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/image=%t", site, withImage), func(t *testing.T) {
				writes := 0
				form := xThreadFormFixture
				id, boardName, target, body, action := 4, "综合版1", "fid", "content", "/Home/Forum/doPostThread.html"
				token := "test-cookie"
				if site == "bog" {
					form = bogThreadFormFixture
					id = bogBoardID("综合版")
					boardName = "综合版"
					target = "forum"
					body = "comment"
					action = "/post/post"
					token = "bog_master=master; bog_sel=shadow"
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/getForumList":
						fmt.Fprint(w, `[{"forums":[{"id":"4","name":"综合版1"}]}]`)
					case "/f/时间线/1":
						fmt.Fprint(w, bogBoardNavigationFixture)
					case "/f/综合版1", "/f/综合版/1":
						http.SetCookie(w, &http.Cookie{Name: "session", Value: "fresh", Path: "/"})
						fmt.Fprint(w, form)
					case "/post/upload":
						writes++
						if err := r.ParseMultipartForm(MaxFile); err != nil {
							t.Error(err)
							return
						}
						defer r.MultipartForm.RemoveAll()
						f, _, err := r.FormFile("image")
						if err != nil {
							t.Error(err)
						} else {
							f.Close()
						}
						fmt.Fprint(w, `{"code":200,"pic":"thread.png"}`)
					case action:
						writes++
						if site == "x" {
							if err := r.ParseMultipartForm(MaxFile); err != nil {
								t.Error(err)
								return
							}
							defer r.MultipartForm.RemoveAll()
						} else {
							r.ParseForm()
						}
						wantTarget := "4"
						if site == "bog" {
							wantTarget = "1"
						}
						cookie, err := r.Cookie("session")
						if err != nil || cookie.Value != "fresh" {
							t.Error("lost form session")
						}
						if r.PostForm.Get(target) != wantTarget || r.PostForm.Get(body) != "新串 & + =" || r.PostForm.Get("title") != "标题" || r.PostForm.Has("resto") || r.PostForm.Has("res") {
							t.Errorf("wrong new-thread form: %v", r.PostForm)
						}
						if !strings.Contains(r.Referer(), "/f/") {
							t.Error("wrong referer")
						}
						if site == "x" {
							if r.PostForm.Get("__hash__") != "fresh-token" {
								t.Error("missing hash")
							}
							if withImage {
								f, _, err := r.FormFile("image")
								if err != nil {
									t.Error(err)
								} else {
									f.Close()
								}
							}
							fmt.Fprint(w, `<p class="success">发帖成功！</p>`)
						} else {
							if withImage && fmt.Sprint(r.PostForm["img[]"]) != "[thread.png]" {
								t.Error("lost BOG image")
							}
							fmt.Fprint(w, `{"code":1}`)
						}
					default:
						t.Errorf("unexpected %s %s for %s", r.Method, r.URL.Path, boardName)
						w.WriteHeader(404)
					}
				}))
				defer server.Close()
				c, _ := NewBackend(site, server.URL, "", token)
				d := Draft{BoardID: id, Title: "标题", Body: "新串 & + ="}
				if withImage {
					path := replyPNG(t)
					if site == "x" {
						d.Files = []string{path}
					} else {
						media, err := c.Upload(context.Background(), path)
						if err != nil {
							t.Fatal(err)
						}
						d.Media = []Media{media}
					}
				}
				if err := c.Publish(context.Background(), d); err != nil {
					t.Fatal(err)
				}
				want := 1
				if site == "bog" && withImage {
					want++
				}
				if writes != want {
					t.Fatalf("unexpected writes %d", writes)
				}
			})
		}
	}
}

func TestNewThreadPreflightRejectsChangedOrAmbiguousTargets(t *testing.T) {
	for _, tc := range []struct {
		site, form string
		id         int
	}{
		{"x", strings.Replace(xThreadFormFixture, `value="4"`, `value="5"`, 1), 4},
		{"x", strings.Replace(xThreadFormFixture, `name="__hash__"`, `name="other"`, 1), 4},
		{"x", strings.Replace(xThreadFormFixture, `action="/Home`, `action="https://foreign.test/Home`, 1), 4},
		{"x", xFormFixture, 4},
		{"x", xThreadFormFixture + xThreadFormFixture, 4},
		{"bog", strings.Replace(bogThreadFormFixture, "综合版", "其他版", 1), bogBoardID("综合版")},
		{"bog", strings.Replace(bogThreadFormFixture, `value="1"`, `value="0"`, 1), bogBoardID("综合版")},
		{"bog", strings.Replace(bogThreadFormFixture, "</form>", `<input type="hidden" name="res" value="100"></form>`, 1), bogBoardID("综合版")},
		{"bog", strings.Replace(bogThreadFormFixture, "</form>", `<input name="captcha"></form>`, 1), bogBoardID("综合版")},
	} {
		writes := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				writes++
				return
			}
			switch r.URL.Path {
			case "/getForumList":
				fmt.Fprint(w, `[{"forums":[{"id":4,"name":"综合版1"}]}]`)
			case "/f/时间线/1":
				fmt.Fprint(w, bogBoardNavigationFixture)
			default:
				fmt.Fprint(w, tc.form)
			}
		}))
		token := "test-cookie"
		if tc.site == "bog" {
			token = "bog_master=master; bog_sel=shadow"
		}
		c, _ := NewBackend(tc.site, server.URL, "", token)
		if err := c.Publish(context.Background(), Draft{BoardID: tc.id, Body: "hello"}); err == nil {
			t.Errorf("accepted invalid %s form", tc.site)
		}
		if writes != 0 {
			t.Error("submitted to invalid form")
		}
		server.Close()
	}
}
