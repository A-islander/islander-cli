package forum

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestXReadingContracts(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.Header.Get("Cookie") != "userhash=test%ABcookie" || r.Header.Get("Authorization") != "" {
			t.Error("wrong read credential transport")
		}
		switch r.URL.Path {
		case "/getForumList":
			fmt.Fprint(w, `[{"forums":[{"id":"-1","name":"时间线"},{"id":"30","name":"技术","msg":"a<br>b"}]}]`)
		case "/getTimelineList":
			fmt.Fprint(w, `[{"id":1,"max_page":2}]`)
		case "/getCDNPath":
			fmt.Fprint(w, `[{"url":"https://images.example/"}]`)
		case "/ref":
			fmt.Fprint(w, `{"id":"101","user_hash":"reader","content":"&gt;&gt;No.100<br>(　ﾟ 3ﾟ)\u001b"}`)
		case "/showf":
			fmt.Fprint(w, `[]`)
		case "/timeline":
			rows := []map[string]any{}
			for i := 1; i <= 20; i++ {
				rows = append(rows, map[string]any{"id": i, "content": "thread"})
			}
			json.NewEncoder(w).Encode(rows)
		case "/thread":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			replies := []map[string]any{{"id": 9999999, "user_hash": "Tips", "content": "advert"}}
			for i := 1; i <= 19; i++ {
				replies = append(replies, map[string]any{"id": 100 + (page-1)*19 + i, "user_hash": "r", "content": "reply"})
			}
			json.NewEncoder(w).Encode(map[string]any{"id": 100, "ReplyCount": "38", "img": "2026/a", "ext": ".png", "content": "root", "Replies": replies})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, err := NewBackend("x", server.URL, "", "test%ABcookie")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	boards, err := c.Boards(ctx)
	if err != nil || len(boards) != 1 || boards[0].ID != 30 || boards[0].Value != "a\nb" {
		t.Fatalf("boards %+v %v", boards, err)
	}
	ref, err := c.Post(ctx, 101)
	if err != nil || !ref.ParentUnknown || ref.UserID != 0 || ref.AuthorID != "reader" || len(ref.Quotes) != 1 || ref.Quotes[0] != 100 || strings.ContainsRune(ref.Body, '\x1b') {
		t.Fatalf("ref %+v %v", ref, err)
	}
	for page := 1; page <= 2; page++ {
		p, err := c.List(ctx, "thread", 100, page)
		if err != nil {
			t.Fatal(err)
		}
		want := 19
		if page == 1 {
			want++
		}
		if len(p.List) != want || p.HasMore != (page == 1) || p.Root == nil || p.Root.ParentUnknown || p.Root.Media()[0].URL != "https://images.example/image/2026/a.png" {
			t.Fatalf("page %+v", p)
		}
		if page == 2 && (p.List[0].ID != 120 || p.List[0].FollowID != 100) {
			t.Fatal("reply page repeats OP or loses parent")
		}
	}
	line, err := c.List(ctx, "timeline", 0, 2)
	if err != nil || line.HasMore {
		t.Fatalf("timeline limit %v %v", line, err)
	}
	if _, err = c.List(ctx, "timeline", 0, 3); err == nil {
		t.Fatal("accepted clamped timeline page")
	}
	u, err := c.Verify(ctx)
	if err != nil || u.ID != 0 || u.Verification != "restricted-board" {
		t.Fatalf("identity %+v %v", u, err)
	}
	before := requests
	if err = c.Publish(ctx, Draft{}); err == nil {
		t.Fatal("external publishing enabled")
	}
	if _, err = c.Upload(ctx, "unused"); err == nil {
		t.Fatal("external uploading enabled")
	}
	if err = c.Action(ctx, "sage", 100); err == nil {
		t.Fatal("external sage enabled")
	}
	if _, err = c.List(ctx, "mine", 0, 1); err == nil {
		t.Fatal("mine pretends to work")
	}
	if requests != before {
		t.Fatal("unsupported operation made a request")
	}
}

func TestExternalFailuresAndCookieValidation(t *testing.T) {
	for _, tc := range []struct{ site, token string }{{"x", "x; other=y"}, {"x", "x\ny"}, {"bog", "bog_master=a; bad=b"}, {"bog", "bog_master=a; bog_sel=b; bog_sel=c"}} {
		if _, err := NewBackend(tc.site, "https://example.org/", "", tc.token); err == nil {
			t.Errorf("accepted invalid %s cookie", tc.site)
		}
	}
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ref" {
			fmt.Fprint(w, `{"success":false,"error":"饼干 secret%ABcookie"}`)
			return
		}
		http.Redirect(w, r, target.URL, 302)
	}))
	defer server.Close()
	c, _ := NewBackend("x", server.URL, "", "secret%ABcookie")
	if _, err := c.Boards(context.Background()); err == nil || called {
		t.Fatal("redirect followed")
	}
	_, err := c.Post(context.Background(), 1)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "auth" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error was not classified/redacted: %v", err)
	}
}

const bogNav = `<div class="forum-list"><a href="/f/时间线">时间线</a><a href="/f/综合版">综合版</a></div>`

func bogFixture(page int) string {
	reply := fmt.Sprintf(`<div class="item-reply"><header><span class="item-id">reply-hash</span><span class="item-time">2026-09-07 12:00:00</span><div class="item-pop">#%d <div>menu</div></div></header><div class="item-content">&gt;&gt;Po.100<br>ᕕ[ ᐛ ]ᕗ<script>bad()</script><div class="item-content-shadow">查看更多</div></div><img src="/image/thumb/reply.jpg" data-img="/image/large/reply.png"></div>`, 100+page)
	next := ""
	if page == 1 {
		next = `<a href="/t/100/2" class="page-next">下一页</a>`
	}
	return bogNav + `<div class="item-list"><div class="item"><div class="item-main"><header><span class="item-id">root-hash</span><div class="item-pop">#100 <div>menu</div></div><a class="item-time" href="/t/100">2026-09-07 10:00:00</a></header><div class="item-content">root<br>line</div><img src="/image/thumb/root.jpg" data-img="/image/large/root.png">` + reply + `<footer class="item-footer">查看全部 <span>2</span> 条回复</footer></div></div></div><div class="pages"><ul class="page-main"><li><span>` + strconv.Itoa(page) + `</span></li></ul>` + next + `</div>`
}
func TestBOGReadingContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "bog_master=master%23secret; bog_sel=display" {
			t.Error("unexpected BOG request")
		}
		switch {
		case r.URL.Path == "/api/thread/102":
			fmt.Fprint(w, `{"code":6001,"info":{"id":102,"res":100,"cookie":"display","content":"&gt;&gt;Po.101<br>reply","images":[{"url":"photo","ext":".gif"}]}}`)
		case strings.HasSuffix(r.URL.Path, "/2"):
			fmt.Fprint(w, bogFixture(2))
		default:
			fmt.Fprint(w, bogFixture(1))
		}
	}))
	defer server.Close()
	c, err := NewBackend("bog", server.URL, "", "bog_master=master%23secret; bog_sel=display")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	b, err := c.Boards(ctx)
	if err != nil || len(b) != 1 || b[0].Key != "综合版" {
		t.Fatalf("boards %+v %v", b, err)
	}
	list, err := c.List(ctx, "board", b[0].ID, 1)
	if err != nil || len(list.List) != 1 || list.List[0].Body != "root\nline" || len(list.List[0].Media()) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	for page := 1; page <= 2; page++ {
		p, err := c.List(ctx, "thread", 100, page)
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if page == 1 {
			want = 2
		}
		if len(p.List) != want || p.HasMore != (page == 1) || p.Count != -1 || p.Offset != -1 || p.Root.ID != 100 {
			t.Fatalf("page %+v", p)
		}
		reply := p.List[len(p.List)-1]
		if reply.FollowID != 100 || reply.AuthorID != "reply-hash" || reply.Body != ">>Po.100\nᕕ[ ᐛ ]ᕗ" || reply.Media()[0].URL != server.URL+"/image/large/reply.png" {
			t.Fatalf("reply %+v", reply)
		}
	}
	post, err := c.Post(ctx, 102)
	if err != nil || post.ThreadID() != 100 || post.Quotes[0] != 101 || post.Media()[0].URL != server.URL+"/image/large/photo.gif" {
		t.Fatalf("quote %+v %v", post, err)
	}
	page, err := c.ReplyPage(ctx, 100, 102)
	if err != nil || page != 2 {
		t.Fatalf("reply page %d %v", page, err)
	}
	u, err := c.Verify(ctx)
	if err != nil || u.Verification != "unverified" || c.Capabilities().Verify {
		t.Fatal("BOG cookie claimed verified")
	}
}

func TestBOGRejectsChangedHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>login or redesigned page</body></html>`)
	}))
	defer server.Close()
	c, _ := NewBackend("bog", server.URL, "", "")
	if _, err := c.List(context.Background(), "timeline", 0, 1); err == nil {
		t.Fatal("unexpected HTML became a valid empty page")
	}
}
