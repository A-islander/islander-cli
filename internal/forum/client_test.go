package forum

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIContracts(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/forum/index":
			if r.URL.Query().Get("page") != "0" || r.URL.Query().Get("plateId") != "7" {
				t.Error("pagination mapping")
			}
			w.Write([]byte(`{"code":200,"data":{"list":[{"id":42,"followId":0,"plateId":7,"value":"你好"}],"count":21}}`))
		case "/forum/reply":
			writes++
			if r.Method != "POST" || r.Header.Get("Authorization") != "test-token" {
				t.Error("write identity")
			}
			var p struct {
				FollowID int
				ReplyArr []int
				Value    string
				MediaURL string
			}
			if e := json.NewDecoder(r.Body).Decode(&p); e != nil {
				t.Fatal(e)
			}
			if p.FollowID != 42 || len(p.ReplyArr) != 1 || p.ReplyArr[0] != 43 || p.MediaURL != "[]" {
				t.Errorf("wrong reply payload: %+v", p)
			}
			w.Write([]byte(`{"code":200,"data":null}`))
		case "/img/upload":
			writes++
			if e := r.ParseMultipartForm(MaxFile + 1024); e != nil {
				t.Fatal(e)
			}
			f, _, e := r.FormFile("file")
			if e != nil {
				t.Fatal(e)
			}
			f.Close()
			w.Write([]byte(`{"code":200,"data":{"success":true,"RequestId":"a","data":{"url":"https://example.org/a.png"}}}`))
		case "/forum/delete/ownPost":
			writes++
			w.Write([]byte(`{"code":200,"data":{"status":false}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, _ := New(server.URL, server.URL, "test-token")
	ctx := context.Background()
	p, e := c.List(ctx, "board", 7, 1)
	if e != nil || !p.HasMore || p.Page != 1 || p.List[0].ID != 42 {
		t.Fatalf("page: %+v %v", p, e)
	}
	if e = c.Publish(ctx, Draft{ThreadID: 42, Body: "No.43\n你好 No.43"}); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(path, []byte("fake upload bytes"), 0600)
	m, e := c.Upload(ctx, path)
	if e != nil || m.URL != "https://example.org/a.png" {
		t.Fatalf("upload: %+v %v", m, e)
	}
	if e = c.Action(ctx, "delete", 42); e == nil {
		t.Fatal("false business status accepted")
	}
	if writes != 3 {
		t.Fatalf("unexpected write count %d", writes)
	}
}
func TestRedirectAndErrorRedaction(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/plate/get" {
			http.Redirect(w, r, target.URL, 302)
			return
		}
		w.Write([]byte(`{"code":403,"msg":"secret-token denied"}`))
	}))
	defer server.Close()
	c, _ := New(server.URL, server.URL, "secret-token")
	if _, e := c.Boards(context.Background()); e == nil || leaked {
		t.Fatal("redirect followed")
	}
	_, e := c.Verify(context.Background())
	if e == nil || strings.Contains(e.Error(), "secret-token") {
		t.Fatal("token not redacted")
	}
}
func TestValidationAndLegacyMedia(t *testing.T) {
	if e := (Draft{BoardID: 1, Body: strings.Repeat("海", 2731)}).Validate(); e == nil {
		t.Fatal("byte limit ignored")
	}
	if e := (Draft{BoardID: 1, Body: "hello", Sent: true}).Validate(); e == nil {
		t.Fatal("sent draft accepted")
	}
	for _, raw := range []string{`["https://example.org/a.jpg"]`, `[{"images":"https://example.org/a.jpg"}]`, `https://example.org/a.jpg`} {
		if len(MediaItems(raw)) != 1 {
			t.Fatal(raw)
		}
	}
	if len(MediaItems(`[{"url":"file:///etc/passwd"}]`)) != 0 {
		t.Fatal("unsafe media URL")
	}
	if strings.ContainsRune(Clean("\x1b[31mhello\x07"), '\x1b') {
		t.Fatal("terminal control retained")
	}
}
