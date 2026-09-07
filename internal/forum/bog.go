package forum

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"hash/crc32"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bogClient struct {
	*external
	mu     sync.Mutex
	boards []Board
}

// BOG web navigation exposes names, not numeric API forum IDs. These stable
// local navigation IDs are never sent to a BOG API or used for publishing.
func bogBoardID(name string) int { return int(crc32.ChecksumIEEE([]byte(name)) & 0x7fffffff) }

func bogBoards(doc *html.Node) ([]Board, error) {
	nav := firstClass(doc, "forum-list")
	if nav == nil {
		return nil, &Error{"response", "BOG 板块导航结构已变化"}
	}
	boards := []Board{}
	seen := map[int]string{}
	var collision bool
	walk(nav, func(n *html.Node) bool {
		if n.Data != "a" {
			return true
		}
		u, err := url.Parse(attr(n, "href"))
		if err != nil {
			return false
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 || parts[0] != "f" || parts[1] == "时间线" {
			return false
		}
		name := parts[1]
		id := bogBoardID(name)
		if old, ok := seen[id]; ok {
			if old != name {
				collision = true
			}
			return false
		}
		seen[id] = name
		boards = append(boards, Board{ID: id, Key: name, Name: Clean(name)})
		return false
	})
	if collision || len(boards) == 0 {
		return nil, &Error{"response", "BOG 板块数据无效或导航编号冲突"}
	}
	return boards, nil
}

func (c *bogClient) document(ctx context.Context, path string) (*html.Node, error) {
	b, err := c.get(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(strings.NewReader(string(b)))
	if err != nil || firstClass(doc, "item-list") == nil || firstClass(doc, "pages") == nil {
		return nil, &Error{"response", "BOG 未返回论坛页面，可能需要权限或网页结构已变化"}
	}
	return doc, nil
}
func (c *bogClient) Boards(ctx context.Context) ([]Board, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.boards) > 0 {
		return append([]Board(nil), c.boards...), nil
	}
	doc, err := c.document(ctx, "f/"+url.PathEscape("时间线")+"/1")
	if err != nil {
		return nil, err
	}
	boards, err := bogBoards(doc)
	if err == nil {
		c.boards = boards
	}
	return append([]Board(nil), boards...), err
}
func (c *bogClient) Verify(context.Context) (User, error) {
	if c.cookie == "" {
		return User{}, &Error{"auth", "请提供 BOG 网页饼干"}
	}
	// Saving a cookie is intentionally not presented as account verification.
	return User{Key: "browser-cookie", Name: "BOG 饼干（未验证）", Verification: "unverified"}, nil
}

func (c *bogClient) Post(ctx context.Context, id int) (Post, error) {
	if id <= 0 {
		return Post{}, fmt.Errorf("编号必须大于 0")
	}
	b, err := c.get(ctx, "api/thread/"+strconv.Itoa(id), nil)
	if err != nil {
		return Post{}, err
	}
	var result struct {
		Code wireInt         `json:"code"`
		Info json.RawMessage `json:"info"`
	}
	if json.Unmarshal(b, &result) != nil {
		return Post{}, &Error{"response", "BOG 引用 JSON 格式异常"}
	}
	if result.Code != 6001 {
		return Post{}, &Error{"business", fmt.Sprintf("BOG 拒绝读取此内容（%d）", result.Code)}
	}
	var w struct {
		ID      wireInt `json:"id"`
		Res     wireInt `json:"res"`
		Time    int64   `json:"time"`
		Cookie  string  `json:"cookie"`
		Name    string  `json:"name"`
		Title   string  `json:"title"`
		Content string  `json:"content"`
		Images  []struct {
			URL string `json:"url"`
			Ext string `json:"ext"`
		} `json:"images"`
	}
	if json.Unmarshal(result.Info, &w) != nil || int(w.ID) != id {
		return Post{}, &Error{"response", "BOG 引用内容缺失或格式已变化"}
	}
	p := Post{Site: "bog", ID: id, FollowID: int(w.Res), Time: w.Time, AuthorID: Clean(w.Cookie), Name: Clean(w.Cookie), Title: HTMLText(w.Title), Body: HTMLText(w.Content), RawHTML: w.Content}
	if w.Name != "" {
		p.Name = HTMLText(w.Name) + " / " + p.AuthorID
	}
	p.Quotes = QuoteIDs(p.Body)
	for _, img := range w.Images {
		p.Attachments = append(p.Attachments, Media{URL: c.site.ForumURL + "image/large/" + url.PathEscape(img.URL) + img.Ext, Thumbnail: c.site.ForumURL + "image/thumb/" + url.PathEscape(img.URL) + ".jpg", Type: "image"})
	}
	return p, nil
}

var bogPostID = regexp.MustCompile(`^\s*#(\d+)`)
var bogCount = regexp.MustCompile(`查看全部\s*(\d+)\s*条回复`)

// Only inspect a post's own nodes; a list root contains preview replies too.
func ownClass(n *html.Node, kind string) *html.Node {
	var found *html.Node
	walk(n, func(v *html.Node) bool {
		if found != nil {
			return false
		}
		if v != n && (class(v, "item-reply") || class(v, "item-main")) {
			return false
		}
		if class(v, kind) {
			found = v
			return false
		}
		return true
	})
	return found
}
func (c *bogClient) htmlPost(n *html.Node, parent int) (Post, error) {
	pop := ownClass(n, "item-pop")
	m := bogPostID.FindStringSubmatch(nodeText(pop))
	if len(m) < 2 {
		return Post{}, &Error{"response", "BOG 帖子编号结构已变化"}
	}
	id, _ := strconv.Atoi(m[1])
	content := ownClass(n, "item-content")
	if content == nil || id <= 0 {
		return Post{}, &Error{"response", "BOG 正文结构已变化"}
	}
	p := Post{Site: "bog", ID: id, FollowID: parent, Body: nodeText(content), RawHTML: innerHTML(content), AuthorID: nodeText(ownClass(n, "item-id")), Title: nodeText(ownClass(n, "item-title"))}
	p.Name = p.AuthorID
	p.Quotes = QuoteIDs(p.Body)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", nodeText(ownClass(n, "item-time")), time.FixedZone("UTC+8", 8*3600)); err == nil {
		p.Time = t.Unix()
	}
	if label := ownClass(n, "item-fname"); label != nil {
		p.BoardName = nodeText(label)
		p.BoardID = bogBoardID(p.BoardName)
	}
	if count := bogCount.FindStringSubmatch(nodeText(ownClass(n, "item-footer"))); len(count) > 1 {
		p.ReplyCount, _ = strconv.Atoi(count[1])
	}
	base, _ := url.Parse(c.site.ForumURL)
	walk(n, func(v *html.Node) bool {
		if v != n && (class(v, "item-reply") || class(v, "item-main")) {
			return false
		}
		if v.Data == "img" && attr(v, "data-img") != "" {
			u, err := url.Parse(attr(v, "data-img"))
			if err != nil {
				return false
			}
			full := base.ResolveReference(u).String()
			thumb := ""
			if t, err := url.Parse(attr(v, "src")); err == nil {
				thumb = base.ResolveReference(t).String()
			}
			if SafeURL(full) {
				p.Attachments = append(p.Attachments, Media{URL: full, Thumbnail: thumb, Type: "image"})
			}
		}
		return true
	})
	return p, nil
}

func (c *bogClient) List(ctx context.Context, kind string, id, page int) (Page, error) {
	p := Page{Page: page, Count: -1, Offset: -1, List: []Post{}}
	if err := validPage(page); err != nil {
		return p, err
	}
	name, path := "", ""
	switch kind {
	case "timeline":
		name = "时间线"
	case "board":
		boards, err := c.Boards(ctx)
		if err != nil {
			return p, err
		}
		for _, b := range boards {
			if b.ID == id {
				name = b.Key
				break
			}
		}
		if name == "" {
			return p, fmt.Errorf("BOG 导航板块不存在；请使用 board list 返回的 id 或板块名称")
		}
	case "thread":
		if id <= 0 {
			return p, fmt.Errorf("编号必须大于 0")
		}
		path = "t/" + strconv.Itoa(id)
	default:
		return p, Unsupported(kind)
	}
	if name != "" {
		path = "f/" + url.PathEscape(name)
	}
	doc, err := c.document(ctx, path+"/"+strconv.Itoa(page))
	if err != nil {
		return p, err
	}
	container := firstClass(doc, "item-list")
	var parseErr error
	rootID := 0
	walk(container, func(n *html.Node) bool {
		if parseErr != nil {
			return false
		}
		if class(n, "item-main") {
			post, e := c.htmlPost(n, 0)
			if e != nil {
				parseErr = e
				return false
			}
			rootID = post.ID
			if kind == "thread" {
				p.Root = &post
			}
			if kind == "thread" && rootID != id {
				parseErr = &Error{"response", "BOG 返回了其他主串，已停止读取"}
				return false
			}
			if name != "" && name != "时间线" {
				post.BoardName = name
				post.BoardID = id
			}
			if kind != "thread" || page == 1 {
				p.List = append(p.List, post)
			}
			return kind == "thread"
		}
		if kind == "thread" && class(n, "item-reply") {
			post, e := c.htmlPost(n, id)
			if e != nil {
				parseErr = e
				return false
			}
			p.List = append(p.List, post)
			return false
		}
		return true
	})
	if parseErr != nil {
		return p, parseErr
	}
	if kind == "thread" && rootID == 0 {
		return p, &Error{"not_found", "BOG 未返回主串"}
	}
	// Navigation windows do not expose a reliable total. Reject silently clamped
	// pages, and accept next links only within the current board/thread.
	currentPage := 0
	pager := firstClass(doc, "pages")
	walk(firstClass(pager, "page-main"), func(n *html.Node) bool {
		if n.Data == "span" {
			if v, e := strconv.Atoi(strings.TrimSpace(nodeText(n))); e == nil {
				currentPage = v
			}
			return false
		}
		return true
	})
	if currentPage != page {
		return p, &Error{"response", "BOG 返回的页码与请求不符，或分页结构已变化"}
	}
	expectedPath, _ := url.PathUnescape(path)
	walk(pager, func(n *html.Node) bool {
		if n.Data != "a" {
			return true
		}
		u, e := url.Parse(attr(n, "href"))
		if e != nil {
			return false
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 3 && strings.Join(parts[:2], "/") == expectedPath {
			next, _ := strconv.Atoi(parts[2])
			if next == page+1 {
				p.HasMore = true
			}
		}
		return false
	})
	return p, nil
}
func (c *bogClient) ReplyPage(ctx context.Context, thread, id int) (int, error) {
	return scanReplyPage(ctx, c, thread, id)
}
