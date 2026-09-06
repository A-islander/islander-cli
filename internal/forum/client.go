package forum

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const ForumURL = "https://forum-api.islander.top/"
const UserURL = "https://user-api.islander.top/"
const PageSize = 20
const MaxFile = 20 << 20

type Board struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status int    `json:"status"`
	Value  string `json:"value"`
}
type Post struct {
	Attachments   []Media `json:"attachments"`
	SageAddIDs    []int   `json:"sageAddId"`
	SageSubIDs    []int   `json:"sageSubId"`
	ID            int     `json:"id"`
	FollowID      int     `json:"followId"`
	BoardID       int     `json:"plateId"`
	Status        int     `json:"status"`
	UserID        int     `json:"userId"`
	Time          int64   `json:"time"`
	Title         string  `json:"title"`
	Body          string  `json:"value"`
	Name          string  `json:"name"`
	MediaURL      string  `json:"mediaUrl"`
	ReplyCount    int     `json:"replyCount"`
	Quotes        []int   `json:"replyArr"`
	SageAdd       int     `json:"sageAddCount"`
	SageSub       int     `json:"sageSubCount"`
	Top           int     `json:"topStatus"`
	LastReplyTime int64   `json:"lastReplyTime"`
}

func (p Post) ThreadID() int {
	if p.FollowID > 0 {
		return p.FollowID
	}
	return p.ID
}

type Page struct {
	List    []Post `json:"list"`
	Count   int    `json:"count"`
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	HasMore bool   `json:"hasMore"`
}
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
type Media struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnailUrl"`
	Type      string `json:"type"`
}
type Draft struct {
	Sent     bool     `json:"sent,omitempty"`
	ID       string   `json:"id"`
	Cookie   string   `json:"cookie"`
	BoardID  int      `json:"plateId"`
	ThreadID int      `json:"followId,omitempty"`
	Title    string   `json:"title"`
	Body     string   `json:"value"`
	Files    []string `json:"files,omitempty"`
	Media    []Media  `json:"media,omitempty"`
	Updated  int64    `json:"updated"`
}

func (d Draft) Validate() error {
	if d.Sent {
		return errors.New("此草稿已发布，不能重复提交")
	}
	if strings.TrimSpace(d.Body) == "" || len(d.Body) > 8192 {
		return errors.New("正文不能为空，且不能超过 8192 字节")
	}
	if len(d.Title) > 128 {
		return errors.New("标题不能超过 128 字节")
	}
	if d.ThreadID < 0 || (d.ThreadID == 0 && d.BoardID <= 0) {
		return errors.New("请选择发串板块或有效的回复目标")
	}
	if len(d.Files)+len(d.Media) > 9 {
		return errors.New("一次最多 9 个附件")
	}
	for _, f := range d.Files {
		if _, err := FileInfo(f); err != nil {
			return err
		}
	}
	return nil
}
func FileInfo(path string) (os.FileInfo, error) {
	f, e := os.Stat(path)
	if e != nil {
		return nil, fmt.Errorf("无法读取附件 %s", Clean(filepath.Base(path)))
	}
	if !f.Mode().IsRegular() || f.Size() > MaxFile {
		return nil, errors.New("附件须为普通文件，单个不超过 20 MB")
	}
	return f, nil
}

var quoteRE = regexp.MustCompile(`(?i)No\.(\d+)`)

func QuoteIDs(body string) []int {
	ids := []int{}
	seen := map[int]bool{}
	for _, m := range quoteRE.FindAllStringSubmatch(body, -1) {
		id, _ := strconv.Atoi(m[1])
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// Remove terminal control characters from external text before presentation.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return -1
		}
		return r
	}, s)
}
func MediaItems(raw string) []Media {
	out := []Media{}
	var list []json.RawMessage
	if json.Unmarshal([]byte(raw), &list) != nil {
		if SafeURL(raw) {
			out = append(out, Media{URL: raw, Type: "image"})
		}
		return out
	}
	for _, v := range list {
		var m Media
		var s string
		if json.Unmarshal(v, &s) == nil {
			m = Media{URL: s, Type: "image"}
		} else {
			_ = json.Unmarshal(v, &m)
			if m.URL == "" {
				var legacy struct {
					Images string `json:"images"`
				}
				_ = json.Unmarshal(v, &legacy)
				m.URL = legacy.Images
			}
		}
		if SafeURL(m.URL) {
			if m.Type == "" {
				m.Type = "image"
			}
			out = append(out, m)
		}
	}
	return out
}
func SafeURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Host != "" && u.User == nil && (u.Scheme == "https" || u.Scheme == "http")
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

type Client struct {
	Forum, User, Token string
	HTTP               *http.Client
}

func New(f, u, token string) (*Client, error) {
	for _, s := range []string{f, u} {
		p, e := url.Parse(s)
		if e != nil || !SafeURL(s) || p.RawQuery != "" || p.Fragment != "" {
			return nil, errors.New("API 地址须为 http(s) URL，不含用户信息、查询或片段")
		}
	}
	return &Client{strings.TrimRight(f, "/") + "/", strings.TrimRight(u, "/") + "/", token, &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) request(ctx context.Context, base, method, path string, q url.Values, body io.Reader, ct string, out any) error {
	target := base + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req, e := http.NewRequestWithContext(ctx, method, target, body)
	if e != nil {
		return &Error{"network", "无法创建请求"}
	}
	if c.Token != "" {
		req.Header.Set("Authorization", c.Token)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return &Error{"network", "连接失败或超时；写操作未自动重试，请先核对是否已成功"}
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if e != nil || len(b) > 8<<20 {
		return &Error{"response", "响应读取失败或过大"}
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &Error{"auth", "饼干无效或没有权限"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{"network", fmt.Sprintf("服务返回 HTTP %d", resp.StatusCode)}
	}
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(b, &env) != nil {
		return &Error{"response", "服务器响应格式异常"}
	}
	if env.Code != 200 {
		code := "business"
		if env.Code == 401 || env.Code == 403 {
			code = "auth"
		}
		msg := Clean(env.Msg)
		if c.Token != "" {
			msg = strings.ReplaceAll(msg, c.Token, "[已隐藏]")
		}
		return &Error{code, fmt.Sprintf("请求被拒绝 (%d)：%s", env.Code, msg)}
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if json.Unmarshal(env.Data, out) != nil {
			return &Error{"response", "服务器数据格式异常"}
		}
	}
	return nil
}
func (c *Client) Boards(ctx context.Context) ([]Board, error) {
	var b []Board
	e := c.request(ctx, c.Forum, "GET", "plate/get", nil, nil, "", &b)
	return b, e
}
func (c *Client) Post(ctx context.Context, id int) (Post, error) {
	var p Post
	if id <= 0 {
		return p, errors.New("编号必须大于 0")
	}
	e := c.request(ctx, c.Forum, "GET", "forum/get", url.Values{"postId": {strconv.Itoa(id)}}, nil, "", &p)
	if e == nil && p.ID <= 0 {
		e = errors.New("内容不存在")
	}
	p.Attachments = MediaItems(p.MediaURL)
	return p, e
}
func (c *Client) List(ctx context.Context, kind string, id, page int) (Page, error) {
	p := Page{Page: page, Size: PageSize}
	if page < 1 || page > 1000000 {
		return p, errors.New("页码必须在 1 到 1000000 之间")
	}
	paths := map[string]string{"timeline": "forum/indexLast", "board": "forum/index", "thread": "forum/list", "mine": "forum/userList", "sage": "forum/sage/list"}
	path, ok := paths[kind]
	if !ok {
		return p, errors.New("未知列表类型")
	}
	if kind == "mine" && c.Token == "" {
		return p, errors.New("请先选择饼干")
	}
	q := url.Values{"page": {strconv.Itoa(page - 1)}, "size": {strconv.Itoa(PageSize)}}
	if kind == "board" {
		q.Set("plateId", strconv.Itoa(id))
	}
	if kind == "thread" {
		q.Set("postId", strconv.Itoa(id))
	}
	e := c.request(ctx, c.Forum, "GET", path, q, nil, "", &p)
	p.HasMore = page*PageSize < p.Count
	if p.List == nil {
		p.List = []Post{}
	}
	for i := range p.List {
		p.List[i].Attachments = MediaItems(p.List[i].MediaURL)
	}
	return p, e
}
func (c *Client) ReplyPage(ctx context.Context, thread, id int) (int, error) {
	var p struct {
		Page int `json:"page"`
	}
	e := c.request(ctx, c.Forum, "GET", "forum/postPage", url.Values{"postId": {strconv.Itoa(thread)}, "replyId": {strconv.Itoa(id)}, "size": {"20"}}, nil, "", &p)
	return max(1, p.Page), e
}
func (c *Client) Verify(ctx context.Context) (User, error) {
	var u User
	e := c.request(ctx, c.User, "GET", "user/get", nil, nil, "", &u)
	if e == nil && u.ID <= 0 {
		e = errors.New("无法验证饼干")
	}
	return u, e
}
func (c *Client) Register(ctx context.Context) (string, error) {
	var v struct {
		Token string `json:"token"`
	}
	anonymous := *c
	anonymous.Token = ""
	e := anonymous.request(ctx, c.User, "GET", "user/register", nil, nil, "", &v)
	if e == nil && v.Token == "" {
		e = errors.New("暂时无法领取饼干")
	}
	return v.Token, e
}
func (c *Client) Publish(ctx context.Context, d Draft) error {
	if c.Token == "" {
		return errors.New("请选择饼干")
	}
	if e := d.Validate(); e != nil {
		return e
	}
	if len(d.Files) > 0 {
		return errors.New("附件尚未上传")
	}
	media, _ := json.Marshal(d.Media)
	if d.Media == nil {
		media = []byte("[]")
	}
	if len(media) > 2048 {
		return errors.New("附件信息超过 2048 字节，请减少附件")
	}
	body := map[string]any{"value": d.Body, "replyArr": QuoteIDs(d.Body), "mediaUrl": string(media)}
	path := "forum/post"
	if d.ThreadID > 0 {
		path = "forum/reply"
		body["followId"] = d.ThreadID
	} else {
		body["plateId"] = d.BoardID
		body["title"] = d.Title
	}
	b, _ := json.Marshal(body)
	return c.request(ctx, c.Forum, "POST", path, nil, bytes.NewReader(b), "application/json", nil)
}
func (c *Client) Action(ctx context.Context, action string, id int) error {
	paths := map[string]string{"sage": "forum/sage/add", "unsage": "forum/sage/sub", "delete": "forum/delete/ownPost", "restore": "forum/recover/ownPost"}
	path, ok := paths[action]
	if !ok || id <= 0 {
		return errors.New("无效操作或编号")
	}
	if c.Token == "" {
		return errors.New("请选择饼干")
	}
	var data json.RawMessage
	e := c.request(ctx, c.Forum, "GET", path, url.Values{"postId": {strconv.Itoa(id)}}, nil, "", &data)
	if e != nil {
		return e
	}
	if action == "delete" || action == "restore" {
		var v struct {
			Status bool `json:"status"`
		}
		_ = json.Unmarshal(data, &v)
		if !v.Status {
			return errors.New("操作失败，可能不是自己的内容")
		}
	}
	return nil
}
func (c *Client) Upload(ctx context.Context, path string) (Media, error) {
	var media Media
	if c.Token == "" {
		return media, errors.New("请选择饼干")
	}
	if _, e := FileInfo(path); e != nil {
		return media, e
	}
	f, e := os.Open(path)
	if e != nil {
		return media, e
	}
	defer f.Close()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, e := w.CreateFormFile("file", filepath.Base(path))
	if e != nil {
		return media, e
	}
	n, e := io.Copy(part, io.LimitReader(f, MaxFile+1))
	if e != nil {
		return media, e
	}
	if n > MaxFile {
		return media, errors.New("附件超过 20 MB")
	}
	if e = w.Close(); e != nil {
		return media, e
	}
	var result struct {
		Success bool   `json:"success"`
		ID      string `json:"RequestId"`
		Data    struct {
			URL string `json:"url"`
		} `json:"data"`
		Images string `json:"images"`
	}
	e = c.request(ctx, c.Forum, "POST", "img/upload", nil, &b, w.FormDataContentType(), &result)
	if e != nil {
		return media, e
	}
	u := result.Data.URL
	if u == "" {
		u = result.Images
	}
	if !SafeURL(u) {
		return media, errors.New("上传未返回有效链接")
	}
	typ := "image"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".webm", ".mov":
		typ = "video"
	case ".mp3", ".ogg", ".wav":
		typ = "audio"
	}
	return Media{ID: result.ID, URL: u, Thumbnail: u, Type: typ}, nil
}
