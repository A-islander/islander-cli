package forum

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
)

const xReplyWeb = "https://www.nmbxd1.com/"

// Only the standard X API has a known corresponding website. Custom instances
// keep all credentials and writes on their explicitly configured endpoint.
func (h *external) replyBase() string {
	if h.site.ID == "x" && h.site.ForumURL == "https://api.nmb.best/api/" {
		return xReplyWeb
	}
	return h.site.ForumURL
}

func (h *external) ValidateDraft(d Draft) error {
	if d.ThreadID <= 0 {
		return Unsupported("外站新主串；当前已接入回复")
	}
	if err := d.Validate(); err != nil {
		return err
	}
	if h.site.ID == "x" && (len(d.Files) > 1 || len(d.Media) > 0) {
		return &Error{"invalid", "X 岛回复最多附一张本地图片，不支持复用独立上传链接"}
	}
	if h.site.ID == "bog" && len(utf16.Encode([]rune(d.Title))) > 50 {
		return &Error{"invalid", "BOG 回复标题不能超过 50 个字符"}
	}
	for _, path := range d.Files {
		if err := replyImage(path); err != nil {
			return err
		}
	}
	if h.site.ID == "bog" {
		for _, m := range d.Media {
			if !validBOGPic(m.ID) || m.URL != h.replyBase()+"image_pre/thumb/"+m.ID {
				return &Error{"invalid", "BOG 附件必须来自当前站点的上传结果"}
			}
		}
	}
	return nil
}

func replyImage(path string) error {
	if _, err := FileInfo(path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return &Error{"invalid", "无法读取回复图片"}
	}
	defer f.Close()
	head := make([]byte, 512)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return &Error{"invalid", "无法读取回复图片"}
	}
	switch http.DetectContentType(head[:n]) {
	case "image/jpeg", "image/png", "image/gif", "image/bmp":
		return nil
	}
	return &Error{"invalid", "外站回复附件须为 JPEG、PNG、GIF 或 BMP 图片"}
}

// Each submission has a private cookie jar: form/session tokens survive the
// preflight GET, while switching identities never inherits an old session.
type replySession struct {
	http    *http.Client
	base    *url.URL
	referer string
}

func (h *external) newReplySession() (*replySession, error) {
	if h.cookie == "" {
		return nil, &Error{"auth", "请先导入当前岛的饼干"}
	}
	base, err := url.Parse(h.replyBase())
	if err != nil {
		return nil, &Error{"invalid", "回复站点地址无效"}
	}
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	req := &http.Request{Header: http.Header{"Cookie": {h.cookie}}}
	cookies := req.Cookies()
	for _, c := range cookies {
		c.Path = "/"
	}
	jar.SetCookies(base, cookies)
	client := *h.http
	client.Jar = jar
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &replySession{http: &client, base: base}, nil
}
func (s *replySession) request(ctx context.Context, method, target string, body io.Reader, contentType string) ([]byte, error) {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != s.base.Scheme || u.Host != s.base.Host || u.User != nil || u.Fragment != "" {
		return nil, &Error{"invalid", "拒绝向不同站点提交回复或饼干"}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, &Error{"invalid", "无法创建回复请求"}
	}
	req.Header.Set("User-Agent", "islander-cli/0.0.2")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if s.referer != "" {
		req.Header.Set("Referer", s.referer)
	}
	if method == http.MethodPost {
		req.Header.Set("Origin", s.base.Scheme+"://"+s.base.Host)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		if method == http.MethodPost {
			return nil, unknownReply()
		}
		return nil, &Error{"network", "无法读取回复表单；尚未提交"}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, &Error{"auth", "饼干无效、权限不足或需要网页验证；请在原站检查"}
	}
	if resp.StatusCode == 429 {
		return nil, &Error{"rate_limit", "站点限制发言频率，请稍后手动重试"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if method == http.MethodPost {
			return nil, unknownReply()
		}
		return nil, &Error{"network", fmt.Sprintf("回复表单返回 HTTP %d；不跟随重定向，尚未提交", resp.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		if method == http.MethodPost {
			return nil, unknownReply()
		}
		return nil, &Error{"response", "回复表单无法读取或过大；尚未提交"}
	}
	return data, nil
}
func unknownReply() error {
	return &Error{"unknown_result", "未能确认提交结果；草稿已保留。请先在原串核对，避免重复发送"}
}

func hasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}

type replyForm struct {
	fields url.Values
	action string
}

func parseReplyForm(data []byte, page string, site string, d Draft) (replyForm, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return replyForm{}, &Error{"response", "无法解析回复表单；尚未提交"}
	}
	target, body, action := "resto", "content", "/Home/Forum/doReplyThread.html"
	if site == "bog" {
		target, body, action = "res", "comment", "/post"
	}
	var forms []*html.Node
	walk(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "form" {
			u, e := url.Parse(attr(n, "action"))
			if e == nil && u.Path == action {
				forms = append(forms, n)
			}
			return false
		}
		return true
	})
	if len(forms) != 1 {
		return replyForm{}, &Error{"response", "未找到唯一回复表单；串可能锁定、需要验证或页面已改版，尚未提交"}
	}
	form := forms[0]
	if strings.ToLower(attr(form, "method")) != "post" {
		return replyForm{}, &Error{"response", "回复表单方法已变化；尚未提交"}
	}
	base, _ := url.Parse(page)
	rel, _ := url.Parse(attr(form, "action"))
	dest := base.ResolveReference(rel)
	if dest.Scheme != base.Scheme || dest.Host != base.Host || dest.User != nil || dest.RawQuery != "" || dest.Fragment != "" {
		return replyForm{}, &Error{"response", "回复表单目标不是当前站点；尚未提交"}
	}
	values := url.Values{}
	hasBody, hasTitle, hasImage := false, false, false
	var invalid error
	walk(form, func(n *html.Node) bool {
		name := attr(n, "name")
		if name == "" {
			return true
		}
		if n.Data == "input" && strings.EqualFold(attr(n, "type"), "hidden") {
			if name != "isManager" && name != "email" && name != "name" && name != "forum" && name != "fid" && name != "img" && name != "img[]" {
				values.Add(name, attr(n, "value"))
			}
		}
		if strings.Contains(strings.ToLower(name), "captcha") || name == "verify" {
			invalid = &Error{"challenge", "回复需要验证码；请到原站完成验证后重试，尚未提交"}
		}
		if n.Data == "input" && name == "image" && strings.EqualFold(attr(n, "type"), "file") && !hasAttr(n, "disabled") {
			hasImage = true
		}
		text := ""
		if n.Data == "textarea" && name == body && !hasAttr(n, "disabled") {
			hasBody = true
			text = d.Body
		}
		if n.Data == "input" && name == "title" {
			hasTitle = true
			text = d.Title
		}
		if text != "" {
			limit, _ := strconv.Atoi(attr(n, "maxlength"))
			if limit > 0 && len(utf16.Encode([]rune(text))) > limit {
				invalid = &Error{"invalid", "正文或标题超过当前网页表单限制；尚未提交"}
			}
		}
		if site == "x" && name == "water" && hasAttr(n, "checked") {
			values.Set("water", attr(n, "value"))
		}
		return true
	})
	if invalid != nil {
		return replyForm{}, invalid
	}
	if !hasBody || len(values[target]) != 1 || values.Get(target) != strconv.Itoa(d.ThreadID) {
		return replyForm{}, &Error{"response", "回复表单的主串编号不匹配；尚未提交"}
	}
	if site == "x" && (len(values["__hash__"]) != 1 || values.Get("__hash__") == "") {
		return replyForm{}, &Error{"response", "回复表单缺少当前校验字段；尚未提交"}
	}
	if site == "x" && len(d.Files) > 0 && !hasImage {
		return replyForm{}, &Error{"response", "当前回复表单不接受图片；尚未提交"}
	}
	if d.Title != "" && !hasTitle {
		return replyForm{}, &Error{"invalid", "当前回复表单不接受标题；尚未提交"}
	}
	values.Set(body, d.Body)
	if hasTitle {
		values.Set("title", d.Title)
	}
	if site == "bog" {
		dest.Path = strings.TrimSuffix(dest.Path, "/") + "/post"
	}
	return replyForm{fields: values, action: dest.String()}, nil
}

func (h *external) prepareReply(ctx context.Context, d Draft) (*replySession, replyForm, error) {
	if err := h.ValidateDraft(d); err != nil {
		return nil, replyForm{}, err
	}
	s, err := h.newReplySession()
	if err != nil {
		return nil, replyForm{}, err
	}
	page := h.replyBase() + "t/" + strconv.Itoa(d.ThreadID)
	if h.site.ID == "bog" {
		page += "/1"
	}
	s.referer = page
	data, err := s.request(ctx, http.MethodGet, page, nil, "")
	if err != nil {
		return nil, replyForm{}, err
	}
	form, err := parseReplyForm(data, page, h.site.ID, d)
	return s, form, err
}

func multipartReply(fields url.Values, path string) (*bytes.Buffer, string, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for key, values := range fields {
		for _, value := range values {
			if err := w.WriteField(key, value); err != nil {
				return nil, "", err
			}
		}
	}
	if path != "" {
		if err := replyImage(path); err != nil {
			return nil, "", err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, "", &Error{"invalid", "无法打开回复图片"}
		}
		defer f.Close()
		head := make([]byte, 512)
		n, readErr := f.ReadAt(head, 0)
		if readErr != nil && readErr != io.EOF {
			return nil, "", &Error{"invalid", "无法读取回复图片"}
		}
		header := textproto.MIMEHeader{}
		// Match browser multipart filenames (RFC 7578), including Chinese names.
		filename := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\r", "", "\n", "").Replace(filepath.Base(path))
		header.Set("Content-Disposition", `form-data; name="image"; filename="`+filename+`"`)
		header.Set("Content-Type", http.DetectContentType(head[:n]))
		part, err := w.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		size, err := io.Copy(part, io.LimitReader(f, MaxFile+1))
		if err != nil || size > MaxFile {
			return nil, "", &Error{"invalid", "回复图片读取失败或超过 20 MB"}
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf, w.FormDataContentType(), nil
}

func (*xClient) InlineFiles() bool { return true }
func (c *xClient) Publish(ctx context.Context, d Draft) error {
	s, form, err := c.prepareReply(ctx, d)
	if err != nil {
		return err
	}
	path := ""
	if len(d.Files) == 1 {
		path = d.Files[0]
	}
	buf, contentType, err := multipartReply(form.fields, path)
	if err != nil {
		return err
	}
	data, err := s.request(ctx, http.MethodPost, form.action, buf, contentType)
	if err != nil {
		return err
	}
	return xReplyResult(data)
}
func xReplyResult(data []byte) error {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return unknownReply()
	}
	if failure := firstClass(doc, "error"); failure != nil {
		msg := nodeText(failure)
		switch {
		case strings.Contains(msg, "验证码"):
			return &Error{"challenge", "X 岛要求验证码，请到原站完成验证"}
		case strings.Contains(msg, "饼干"):
			return &Error{"auth", "X 岛拒绝了当前饼干，请在原站检查权限与状态"}
		case strings.Contains(msg, "频繁") || strings.Contains(msg, "间隔"):
			return &Error{"rate_limit", "X 岛限制发言频率，请稍后手动重试"}
		default:
			return &Error{"rejected", "X 岛拒绝了回复；请在原串网页检查锁定、版规、内容及图片限制"}
		}
	}
	if success := firstClass(doc, "success"); success != nil {
		msg := strings.TrimSpace(nodeText(success))
		for _, allowed := range []string{"回复成功", "回应成功", "发表成功", "发帖成功"} {
			if msg == allowed || msg == allowed+"！" || msg == allowed+"!" {
				return nil
			}
		}
	}
	return unknownReply()
}

func validBOGPic(id string) bool {
	if id == "" || len(id) > 200 || strings.Contains(id, "..") {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func bogWriteError(code int) error {
	switch code {
	case 4:
		return &Error{"rate_limit", "BOG 反垃圾检测暂停了发言，请到原站查看恢复时间"}
	case 101:
		return &Error{"challenge", "BOG 要求验证码；请在原站完成验证后手动重试"}
	case 1000, 1001, 1002, 1003, 1004:
		return &Error{"auth", "BOG 饼干或选中影武者不可用，请在原站检查"}
	case 301, 302, 303, 304, 1005, 1008:
		return &Error{"rejected", "BOG 拒绝了图片，请检查图片格式、大小、数量及图片发布权限"}
	case 1101:
		return &Error{"not_found", "BOG 回复目标不存在或已锁定"}
	case 1102:
		return &Error{"duplicate", "BOG 提示近期发布过相同内容，请先核对原串"}
	case 1100, 1103, 1201:
		return &Error{"rejected", "BOG 拒绝了回复，请在原站检查目标与内容"}
	}
	return unknownReply()
}
func (c *bogClient) Upload(ctx context.Context, path string) (Media, error) {
	s, err := c.newReplySession()
	if err != nil {
		return Media{}, err
	}
	s.referer = c.replyBase()
	buf, contentType, err := multipartReply(nil, path)
	if err != nil {
		return Media{}, err
	}
	data, err := s.request(ctx, http.MethodPost, c.replyBase()+"post/upload", buf, contentType)
	if err != nil {
		return Media{}, err
	}
	var result struct {
		Code wireInt `json:"code"`
		Pic  string  `json:"pic"`
	}
	if json.Unmarshal(data, &result) != nil {
		return Media{}, unknownReply()
	}
	if result.Code != 200 {
		return Media{}, bogWriteError(int(result.Code))
	}
	if !validBOGPic(result.Pic) {
		return Media{}, unknownReply()
	}
	return Media{ID: result.Pic, URL: c.replyBase() + "image_pre/thumb/" + result.Pic, Type: "image"}, nil
}
func (c *bogClient) Publish(ctx context.Context, d Draft) error {
	if len(d.Files) > 0 {
		return &Error{"invalid", "BOG 图片须先上传再回复"}
	}
	s, form, err := c.prepareReply(ctx, d)
	if err != nil {
		return err
	}
	for _, m := range d.Media {
		form.fields.Add("img[]", m.ID)
	}
	data, err := s.request(ctx, http.MethodPost, form.action, strings.NewReader(form.fields.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	var result struct {
		Code wireInt `json:"code"`
	}
	if json.Unmarshal(data, &result) != nil {
		return unknownReply()
	}
	if result.Code == 1 {
		return nil
	}
	return bogWriteError(int(result.Code))
}
