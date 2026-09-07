package forum

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Credentials are immutable per backend. Redirects are never followed, including
// same-host redirects, so credentials cannot escape through an upstream route.
type external struct {
	site   Site
	cookie string
	http   *http.Client
}

func newExternal(s Site, token string) (*external, error) {
	h := &external{site: s, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if token == "" {
		return h, nil
	}
	if strings.ContainsAny(token, "\r\n\x00") || len(token) > 16384 {
		return nil, &Error{"auth", "饼干格式无效"}
	}
	if s.ID == "x" {
		if !validCookieValue(token) {
			return nil, &Error{"auth", "请仅粘贴 userhash 的值（保留原有百分号编码）"}
		}
		h.cookie = "userhash=" + token
	} else {
		seen := map[string]bool{}
		var values []string
		for _, field := range strings.Split(token, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(field), "=")
			if !ok || seen[k] || (k != "bog_master" && k != "bog_sel" && k != "bog_list") || !validCookieValue(v) {
				return nil, &Error{"auth", "请提供 bog_master=…; bog_sel=…（可选 bog_list=…）"}
			}
			seen[k] = true
			values = append(values, k+"="+v)
		}
		if !seen["bog_master"] || !seen["bog_sel"] {
			return nil, &Error{"auth", "需要 bog_master 和 bog_sel"}
		}
		h.cookie = strings.Join(values, "; ")
	}
	return h, nil
}

func validCookieValue(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e || strings.ContainsRune("\";,\\", r) {
			return false
		}
	}
	return true
}

func (h *external) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	target := h.site.ForumURL + strings.TrimPrefix(path, "/")
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, &Error{"network", "无法创建请求"}
	}
	req.Header.Set("User-Agent", "islander-cli/0.0.1")
	if h.cookie != "" {
		req.Header.Set("Cookie", h.cookie)
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, &Error{"network", "连接失败、超时或读取已取消"}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, &Error{"auth", "饼干无效或当前内容需要权限"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &Error{"network", fmt.Sprintf("服务返回 HTTP %d（不跟随重定向）", resp.StatusCode)}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(b) > 8<<20 {
		return nil, &Error{"response", "响应读取失败或过大"}
	}
	return b, nil
}

func (h *external) Capabilities() Capabilities { return Capabilities{Verify: h.site.ID == "x"} }
func (h *external) Publish(context.Context, Draft) error {
	return Unsupported("发帖；可在站点网页操作")
}
func (h *external) Upload(context.Context, string) (Media, error) {
	return Media{}, Unsupported("上传附件")
}
func (h *external) Action(context.Context, string, int) error {
	return Unsupported("SAGE、删除或恢复")
}
func (h *external) Register(context.Context) (string, error) {
	return "", Unsupported("领取饼干；请使用站点网页")
}

// Some wire APIs alternate numeric strings and JSON numbers.
type wireInt int

func (v *wireInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || string(b) == `""` {
		*v = 0
		return nil
	}
	s := string(b)
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	}
	n, err := strconv.Atoi(s)
	*v = wireInt(n)
	return err
}

func scanReplyPage(ctx context.Context, c Reader, thread, id int) (int, error) {
	if thread <= 0 || id <= 0 {
		return 0, fmt.Errorf("编号必须大于 0")
	}
	if thread == id {
		return 1, nil
	}
	// Bounded scanning keeps a typo or missing post from crawling an entire site.
	for page := 1; page <= 20; page++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		p, err := c.List(ctx, "thread", thread, page)
		if err != nil {
			return 0, err
		}
		for _, post := range p.List {
			if post.ID == id {
				return page, nil
			}
		}
		if !p.HasMore {
			return 0, &Error{"not_found", "当前串中未找到此回复"}
		}
	}
	return 0, &Error{"limit", "回复定位超过 20 页；请直接打开主串并翻页"}
}
