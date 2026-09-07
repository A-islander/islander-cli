package forum

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const xFormFixture = `<form method="post" action="/Home/Forum/doReplyThread.html"><input type="hidden" name="resto" value="100"><input type="hidden" name="__hash__" value="fresh-token"><input type="checkbox" name="isManager" value="true"><input name="title" maxlength="100"><textarea name="content" maxlength="10000"></textarea><input type="checkbox" name="water" value="true" checked><input type="file" name="image"></form>`
const bogFormFixture = `<form method="post" action="/post"><input type="hidden" name="res" value="100"><input name="title" maxlength="50" disabled><textarea name="comment"></textarea></form>`

func replyPNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "回复.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
func requireReplyCode(t *testing.T, err error, want string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("got %v, want %s", err, want)
	}
}
func TestXReplyUsesFreshFormSessionAndAtomicImage(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		cookie, err := r.Cookie("userhash")
		if err != nil || cookie.Value != "secret%AB" || r.Header.Get("Authorization") != "" {
			t.Error("wrong X identity transport")
		}
		switch r.URL.Path {
		case "/t/100":
			http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "form-session", Path: "/"})
			fmt.Fprint(w, xFormFixture)
		case "/Home/Forum/doReplyThread.html":
			session, err := r.Cookie("PHPSESSID")
			if err != nil || session.Value != "form-session" {
				t.Error("lost preflight session cookie")
			}
			if err := r.ParseMultipartForm(MaxFile); err != nil {
				t.Fatal(err)
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("resto") != "100" || r.FormValue("content") != ">>No.101\n正文 & = +" || r.FormValue("__hash__") != "fresh-token" || r.FormValue("isManager") != "" || r.FormValue("water") != "true" {
				t.Error("incorrect reply fields")
			}
			if !strings.HasSuffix(r.Referer(), "/t/100") || r.Header.Get("Origin") == "" {
				t.Error("missing form request headers")
			}
			f, metadata, err := r.FormFile("image")
			if err != nil {
				t.Fatal(err)
			}
			if metadata.Filename != "回复.png" || metadata.Header.Get("Content-Type") != "image/png" {
				t.Error("multipart image filename or MIME type lost")
			}
			if _, err = png.Decode(f); err != nil {
				t.Error("multipart image damaged")
			}
			f.Close()
			fmt.Fprint(w, `<p class="success">回复成功！</p>`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, err := NewBackend("x", server.URL, "", "secret%AB")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Capabilities().CanPublish(true) || c.Capabilities().CanPublish(false) {
		t.Fatal("reply capability not independent")
	}
	err = c.Publish(context.Background(), Draft{ThreadID: 100, Body: ">>No.101\n正文 & = +", Files: []string{replyPNG(t)}})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(calls) != "[GET /t/100 POST /Home/Forum/doReplyThread.html]" {
		t.Fatalf("extra writes: %v", calls)
	}
}

func TestBOGUploadAndReplyWireFormat(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, want := range map[string]string{"bog_master": "master#code", "bog_sel": "shadow"} {
			cookie, err := r.Cookie(key)
			if err != nil || cookie.Value != want {
				t.Error("BOG identity missing")
			}
		}
		switch r.URL.Path {
		case "/post/upload":
			posts++
			if err := r.ParseMultipartForm(MaxFile); err != nil {
				t.Fatal(err)
			}
			defer r.MultipartForm.RemoveAll()
			f, _, err := r.FormFile("image")
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			fmt.Fprint(w, `{"code":200,"pic":"picture.png"}`)
		case "/t/100/1":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fresh", Path: "/"})
			fmt.Fprint(w, bogFormFixture)
		case "/post/post":
			posts++
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Error("BOG reply must be form encoded")
			}
			r.ParseForm()
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "fresh" {
				t.Error("lost BOG form session")
			}
			if r.PostForm.Get("res") != "100" || r.PostForm.Get("comment") != ">>Po.101\n正文 & = +" || fmt.Sprint(r.PostForm["img[]"]) != "[picture.png]" || r.PostForm.Get("forum") != "" {
				t.Error("wrong BOG reply data")
			}
			fmt.Fprint(w, `{"code":1}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, err := NewBackend("bog", server.URL, "", "bog_master=master#code; bog_sel=shadow")
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.Upload(context.Background(), replyPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Publish(context.Background(), Draft{ThreadID: 100, Body: ">>Po.101\n正文 & = +", Media: []Media{m}}); err != nil {
		t.Fatal(err)
	}
	if posts != 2 {
		t.Fatal("reply retried or uploaded twice")
	}
}

func TestReplyPreflightRejectsWrongTargetAndForeignAction(t *testing.T) {
	for _, tc := range []struct{ name, form, code string }{
		{"foreign", strings.ReplaceAll(xFormFixture, `action="/Home`, `action="https://foreign.test/Home`), "response"},
		{"wrong-target", strings.ReplaceAll(xFormFixture, `value="100"`, `value="101"`), "response"},
		{"missing-hash", strings.ReplaceAll(xFormFixture, `name="__hash__"`, `name="other"`), "response"},
		{"duplicate", xFormFixture + xFormFixture, "response"},
		{"captcha", strings.ReplaceAll(xFormFixture, "</form>", `<input name="captcha"></form>`), "challenge"},
		{"changed-limit", strings.ReplaceAll(xFormFixture, `maxlength="10000"`, `maxlength="1"`), "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				fmt.Fprint(w, tc.form)
			}))
			defer server.Close()
			c, _ := NewBackend("x", server.URL, "", "cookie")
			requireReplyCode(t, c.Publish(context.Background(), Draft{ThreadID: 100, Body: "hello"}), tc.code)
			if writes != 0 {
				t.Fatal("invalid preflight sent a reply")
			}
		})
	}
}

func TestReplyResponsesNeverTreatHTTP200AsSuccess(t *testing.T) {
	for _, tc := range []struct{ site, body, code string }{
		{"x", `<p class="error">secret%AB 饼干不可用</p>`, "auth"},
		{"x", `<p class="error">验证码</p>`, "challenge"},
		{"x", `<p class="success">回复成功？并没有</p>`, "unknown_result"},
		{"x", `回复成功`, "unknown_result"},
		{"bog", `{"code":101,"info":"secret%AB"}`, "challenge"},
		{"bog", `{"code":1102}`, "duplicate"},
		{"bog", `{"code":6001}`, "unknown_result"},
		{"bog", `{"code":200}`, "unknown_result"},
		{"bog", `{"code":1101}`, "not_found"},
	} {
		t.Run(tc.site+tc.code+tc.body, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if tc.site == "x" {
						fmt.Fprint(w, xFormFixture)
					} else {
						fmt.Fprint(w, bogFormFixture)
					}
					return
				}
				writes++
				io.Copy(io.Discard, r.Body)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			token := "secret%AB"
			if tc.site == "bog" {
				token = "bog_master=secret%AB; bog_sel=shadow"
			}
			c, _ := NewBackend(tc.site, server.URL, "", token)
			err := c.Publish(context.Background(), Draft{ThreadID: 100, Body: "hello"})
			requireReplyCode(t, err, tc.code)
			if strings.Contains(err.Error(), "secret%AB") || writes != 1 {
				t.Fatal("credential leaked or failed write retried")
			}
		})
	}
}

func TestReplyRedirectDoesNotLeakCookiesOrRetry(t *testing.T) {
	leaked := false
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked = true }))
	defer foreign.Close()
	for _, redirectMethod := range []string{"GET", "POST"} {
		writes := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				writes++
			}
			if r.Method == redirectMethod {
				http.Redirect(w, r, foreign.URL, 307)
				return
			}
			fmt.Fprint(w, xFormFixture)
		}))
		c, _ := NewBackend("x", server.URL, "", "secret%AB")
		err := c.Publish(context.Background(), Draft{ThreadID: 100, Body: "hello"})
		want := "network"
		if redirectMethod == "POST" {
			want = "unknown_result"
		}
		requireReplyCode(t, err, want)
		if leaked || writes > 1 {
			t.Fatal("redirect followed or reply retried")
		}
		server.Close()
	}
}
