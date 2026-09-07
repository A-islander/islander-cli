package forum

import (
	"context"
	"fmt"
	"strings"
)

type Capabilities struct {
	Publish  bool `json:"publish"`
	Reply    bool `json:"reply"`
	Mine     bool `json:"mine"`
	Sage     bool `json:"sage"`
	Manage   bool `json:"manage"`
	Register bool `json:"register"`
	Verify   bool `json:"verify"`
}

func (c Capabilities) CanPublish(reply bool) bool {
	return c.Publish || (reply && c.Reply)
}

// Adapters may validate their own draft constraints and submit files together
// with the final form, instead of using a separate upload endpoint.
func ValidateDraft(c Backend, d Draft) error {
	if v, ok := c.(interface{ ValidateDraft(Draft) error }); ok {
		return v.ValidateDraft(d)
	}
	return d.Validate()
}

func InlineFiles(c Backend) bool {
	v, ok := c.(interface{ InlineFiles() bool })
	return ok && v.InlineFiles()
}

type Reader interface {
	Boards(context.Context) ([]Board, error)
	Post(context.Context, int) (Post, error)
	List(context.Context, string, int, int) (Page, error)
	ReplyPage(context.Context, int, int) (int, error)
}

type Publisher interface {
	Publish(context.Context, Draft) error
	Upload(context.Context, string) (Media, error)
	Action(context.Context, string, int) error
}

type IdentityProvider interface {
	Verify(context.Context) (User, error)
	Register(context.Context) (string, error)
}

type Backend interface {
	Reader
	Publisher
	IdentityProvider
	Capabilities() Capabilities
}

type Site struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ForumURL string `json:"forumURL"`
	UserURL  string `json:"userURL,omitempty"`
}

func Sites() []Site {
	return []Site{
		{"islander", "岛民岛", ForumURL, UserURL},
		{"x", "X 岛", "https://api.nmb.best/api/", ""},
		{"bog", "BOG", "https://bog.ac/", ""},
	}
}

func Resolve(site, f, u string) (Site, error) {
	if site == "" {
		site = "islander"
	}
	for _, s := range Sites() {
		if s.ID != site {
			continue
		}
		if f != "" {
			s.ForumURL = f
		}
		if u != "" {
			if site != "islander" {
				return Site{}, fmt.Errorf("--user-url 仅用于岛民岛")
			}
			s.UserURL = u
		}
		// Reuse endpoint validation without introducing another HTTP protocol.
		v := s.UserURL
		if v == "" {
			v = s.ForumURL
		}
		c, err := New(s.ForumURL, v, "")
		if err != nil {
			return Site{}, err
		}
		s.ForumURL = c.Forum
		if s.UserURL != "" {
			s.UserURL = c.User
		}
		return s, nil
	}
	return Site{}, fmt.Errorf("未知站点 %q；可选 islander、x、bog", Clean(site))
}

func NewBackend(site, f, u, token string) (Backend, error) {
	s, err := Resolve(site, f, u)
	if err != nil {
		return nil, err
	}
	if s.ID == "islander" {
		return New(s.ForumURL, s.UserURL, token)
	}
	h, err := newExternal(s, token)
	if err != nil {
		return nil, err
	}
	if s.ID == "x" {
		return &xClient{external: h}, nil
	}
	return &bogClient{external: h}, nil
}

func (*Client) Capabilities() Capabilities {
	return Capabilities{Publish: true, Reply: true, Mine: true, Sage: true, Manage: true, Register: true, Verify: true}
}

func Unsupported(action string) error {
	return &Error{"unsupported", "当前站点尚未支持：" + action}
}

func Quote(site string, id int) string {
	prefix := "No."
	if site == "x" {
		prefix = ">>No."
	}
	if site == "bog" {
		prefix = ">>Po."
	}
	return fmt.Sprintf("%s%d", prefix, id)
}

func (p Post) Media() []Media {
	if len(p.Attachments) > 0 {
		return p.Attachments
	}
	return MediaItems(p.MediaURL)
}

func (p Page) Label() string {
	if p.Count < 0 || p.Size <= 0 {
		return fmt.Sprintf("第 %d 页", max(1, p.Page))
	}
	return fmt.Sprintf("%d/%d页", max(1, p.Page), max(1, (p.Count+p.Size-1)/p.Size))
}

func validPage(page int) error {
	if page < 1 || page > 1000000 {
		return fmt.Errorf("页码必须在 1 到 1000000 之间")
	}
	return nil
}

func cleanName(s string) string {
	s = strings.TrimSpace(HTMLText(s))
	if s == "无名氏" || s == "无标题" {
		return ""
	}
	return s
}
