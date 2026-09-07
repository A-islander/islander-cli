package forum

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type xClient struct {
	*external
	mu  sync.Mutex
	cdn string
}

type xPost struct {
	ID         wireInt `json:"id"`
	FID        wireInt `json:"fid"`
	ReplyCount wireInt `json:"ReplyCount"`
	Content    string  `json:"content"`
	Now        string  `json:"now"`
	Hash       string  `json:"user_hash"`
	Name       string  `json:"name"`
	Title      string  `json:"title"`
	Img        string  `json:"img"`
	Ext        string  `json:"ext"`
	Sage       wireInt `json:"sage"`
	Replies    []xPost `json:"Replies"`
}

func (c *xClient) json(ctx context.Context, path string, q url.Values, out any) error {
	b, err := c.get(ctx, path, q)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b)) == "null" {
		return &Error{"response", "X 岛返回了空响应"}
	}
	var failure struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(b, &failure) == nil && (failure.Error != "" || (failure.Success != nil && !*failure.Success)) {
		code := "business"
		if strings.Contains(failure.Error, "饼干") || strings.Contains(failure.Error, "登入") {
			code = "auth"
		}
		// Remote error strings can contain reflected cookies; use a fixed message.
		return &Error{code, "X 岛拒绝请求；内容可能不存在、需要饼干或访问权限"}
	}
	if json.Unmarshal(b, out) != nil {
		return &Error{"response", "X 岛 JSON 格式异常"}
	}
	return nil
}

func (c *xClient) Boards(ctx context.Context) ([]Board, error) {
	var groups []struct {
		Forums []struct {
			ID   wireInt `json:"id"`
			Name string  `json:"name"`
			Msg  string  `json:"msg"`
		} `json:"forums"`
	}
	if err := c.json(ctx, "getForumList", nil, &groups); err != nil {
		return nil, err
	}
	boards := []Board{}
	seen := map[int]bool{}
	for _, g := range groups {
		for _, b := range g.Forums {
			if b.ID > 0 && !seen[int(b.ID)] {
				seen[int(b.ID)] = true
				boards = append(boards, Board{ID: int(b.ID), Name: HTMLText(b.Name), Value: HTMLText(b.Msg)})
			}
		}
	}
	if len(boards) == 0 {
		return nil, &Error{"response", "X 岛板块列表为空或格式已变化"}
	}
	return boards, nil
}

func (c *xClient) imageBase(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cdn != "" {
		return c.cdn, nil
	}
	var paths []struct {
		URL string `json:"url"`
	}
	if err := c.json(ctx, "getCDNPath", nil, &paths); err != nil {
		return "", err
	}
	for _, p := range paths {
		if SafeURL(p.URL) {
			c.cdn = strings.TrimRight(p.URL, "/") + "/"
			return c.cdn, nil
		}
	}
	return "", &Error{"response", "X 岛未返回有效图片 CDN"}
}

var xDateRE = regexp.MustCompile(`\([^)]*\)`)

func (c *xClient) normalize(ctx context.Context, w xPost, parent int, unknown bool) (Post, error) {
	p := Post{Site: "x", ID: int(w.ID), FollowID: parent, ParentUnknown: unknown, BoardID: int(w.FID), AuthorID: Clean(w.Hash), Title: cleanName(w.Title), Body: HTMLText(w.Content), RawHTML: w.Content, ReplyCount: int(w.ReplyCount), Name: cleanName(w.Name)}
	if p.Name == "" {
		p.Name = p.AuthorID
	} else {
		p.Name += " / " + p.AuthorID
	}
	if w.Sage != 0 {
		p.Status = 1
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", xDateRE.ReplaceAllString(w.Now, " "), time.FixedZone("UTC+8", 8*3600))
	if err == nil {
		p.Time = t.Unix()
	}
	p.Quotes = QuoteIDs(p.Body)
	if w.Img != "" {
		base, err := c.imageBase(ctx)
		if err != nil {
			return p, err
		}
		// Resolve as a path, never as an arbitrary absolute media endpoint.
		path := strings.TrimLeft(w.Img, "/") + w.Ext
		p.Attachments = []Media{{URL: base + "image/" + path, Thumbnail: base + "thumb/" + path, Type: "image"}}
	}
	return p, nil
}

func (c *xClient) Post(ctx context.Context, id int) (Post, error) {
	if id <= 0 {
		return Post{}, fmt.Errorf("编号必须大于 0")
	}
	var w xPost
	if err := c.json(ctx, "ref", url.Values{"id": {strconv.Itoa(id)}}, &w); err != nil {
		return Post{}, err
	}
	if int(w.ID) != id || w.Hash == "Tips" {
		return Post{}, &Error{"not_found", "X 岛内容不存在"}
	}
	return c.normalize(ctx, w, 0, true)
}

func (c *xClient) List(ctx context.Context, kind string, id, page int) (Page, error) {
	p := Page{Page: page, Size: 20, Count: -1, Offset: -1, List: []Post{}}
	if err := validPage(page); err != nil {
		return p, err
	}
	path := ""
	limit := 0
	switch kind {
	case "timeline":
		path, id = "timeline", 1
		var lines []struct {
			ID      wireInt `json:"id"`
			MaxPage wireInt `json:"max_page"`
		}
		if err := c.json(ctx, "getTimelineList", nil, &lines); err != nil {
			return p, err
		}
		for _, line := range lines {
			if int(line.ID) == id {
				limit = int(line.MaxPage)
			}
		}
		if limit <= 0 {
			return p, &Error{"response", "X 岛时间线信息缺失"}
		}
		if page > limit {
			return p, &Error{"limit", "超过此时间线的页数上限"}
		}
	case "board":
		path = "showf"
	case "thread":
		path = "thread"
	default:
		return p, Unsupported(kind)
	}
	if id <= 0 {
		return p, fmt.Errorf("编号必须大于 0")
	}
	q := url.Values{"id": {strconv.Itoa(id)}, "page": {strconv.Itoa(page)}}
	var rows []xPost
	if kind == "thread" {
		var root xPost
		if err := c.json(ctx, path, q, &root); err != nil {
			return p, err
		}
		if int(root.ID) != id {
			return p, &Error{"not_found", "请提供 X 岛主串编号；引用接口不提供父串，可用 post get 单独读取"}
		}
		if page > max(1, (int(root.ReplyCount)+18)/19) {
			return p, &Error{"limit", "超过此串的回复页数上限"}
		}
		normalized, err := c.normalize(ctx, root, 0, false)
		if err != nil {
			return p, err
		}
		p.Root = &normalized
		if page == 1 {
			rows = append(rows, root)
		}
		rows = append(rows, root.Replies...)
		// X returns 19 actual replies per page, plus an optional Tips item.
		p.Size = 19
		p.HasMore = page*19 < int(root.ReplyCount)
	} else {
		if err := c.json(ctx, path, q, &rows); err != nil {
			return p, err
		}
		p.HasMore = len(rows) >= 20 && (limit == 0 || page < limit)
	}
	seen := map[int]bool{}
	for _, w := range rows {
		if w.Hash == "Tips" || seen[int(w.ID)] {
			continue
		}
		if w.ID <= 0 {
			return p, &Error{"response", "X 岛帖子编号缺失或格式已变化"}
		}
		seen[int(w.ID)] = true
		parent := 0
		if kind == "thread" && int(w.ID) != id {
			parent = id
		}
		post, err := c.normalize(ctx, w, parent, false)
		if err != nil {
			return p, err
		}
		p.List = append(p.List, post)
	}
	return p, nil
}
func (c *xClient) ReplyPage(ctx context.Context, thread, id int) (int, error) {
	return scanReplyPage(ctx, c, thread, id)
}
func (c *xClient) Verify(ctx context.Context) (User, error) {
	if c.cookie == "" {
		return User{}, &Error{"auth", "请提供 userhash"}
	}
	var rows []xPost
	if err := c.json(ctx, "showf", url.Values{"id": {"27"}, "page": {"1"}}, &rows); err != nil {
		return User{}, err
	}
	if rows == nil {
		return User{}, &Error{"response", "无法确认饼干访问权限"}
	}
	return User{Key: "cookie", Name: "X 岛饼干", Verification: "restricted-board"}, nil
}
