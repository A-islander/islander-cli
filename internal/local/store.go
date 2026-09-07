package local

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/gofrs/flock"
	keyring "github.com/zalando/go-keyring"
)

type Cookie struct {
	Key          string `json:"key,omitempty"`
	Verification string `json:"verification,omitempty"`
	Alias        string `json:"alias"`
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Backend      string `json:"backend"`
}
type State struct {
	Active  string   `json:"active"`
	Cookies []Cookie `json:"cookies"`
}
type Store struct{ Dir, Scope, Backend string }

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,48}$`)

func New(root, forumURL, userURL, backend string) (*Store, error) {
	if backend != "keyring" && backend != "file" {
		return nil, errors.New("凭证存储只能是 keyring 或 file")
	}
	if root == "" {
		base, e := os.UserConfigDir()
		if e != nil {
			return nil, e
		}
		root = filepath.Join(base, "islander")
	}
	hash := sha256.Sum256([]byte(forumURL + "\n" + userURL))
	scope := hex.EncodeToString(hash[:8])
	dir := filepath.Join(root, scope)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	if e := os.Chmod(dir, 0700); e != nil {
		return nil, e
	}
	return &Store{dir, scope, backend}, nil
}
func (s *Store) Read() (State, error) {
	var v State
	b, e := os.ReadFile(filepath.Join(s.Dir, "cookies.json"))
	if os.IsNotExist(e) {
		return State{Cookies: []Cookie{}}, nil
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func atomic(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".write-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
func (s *Store) change(fn func(*State) error) error {
	lock := flock.New(filepath.Join(s.Dir, ".lock"))
	ok, e := lock.TryLock()
	if e != nil {
		return e
	}
	if !ok {
		return errors.New("身份存储正被其他进程修改，请稍后重试")
	}
	defer lock.Unlock()
	v, e := s.Read()
	if e != nil {
		return e
	}
	if e = fn(&v); e != nil {
		return e
	}
	return atomic(filepath.Join(s.Dir, "cookies.json"), v)
}
func (s *Store) Import(alias, token string, u forum.User) error {
	if !nameRE.MatchString(alias) {
		return errors.New("别名请用 1–48 位字母、数字、下划线或短横线")
	}
	if token == "" || (u.ID <= 0 && u.Key == "") {
		return errors.New("无效饼干")
	}
	return s.change(func(v *State) error {
		for _, c := range v.Cookies {
			if c.Alias == alias {
				return errors.New("别名已存在，请使用新别名")
			}
		}
		if s.Backend == "file" {
			if e := atomic(filepath.Join(s.Dir, "secret-"+alias+".json"), token); e != nil {
				return e
			}
		} else if e := keyring.Set("islander-"+s.Scope, alias, token); e != nil {
			return errors.New("系统凭证库不可用；可重新使用 --credential-store file 显式选择本地明文存储（0600）")
		}
		v.Cookies = append(v.Cookies, Cookie{Alias: alias, ID: u.ID, Name: forum.Clean(u.Name), Backend: s.Backend, Key: u.Key, Verification: u.Verification})
		v.Active = alias
		return nil
	})
}
func (s *Store) Identity(alias string) (Cookie, string, error) {
	v, e := s.Read()
	if e != nil {
		return Cookie{}, "", e
	}
	if alias == "" {
		alias = v.Active
	}
	if alias == "" {
		return Cookie{}, "", nil
	}
	for _, c := range v.Cookies {
		if c.Alias != alias {
			continue
		}
		var token string
		if c.Backend == "file" {
			b, err := os.ReadFile(filepath.Join(s.Dir, "secret-"+alias+".json"))
			if err != nil {
				return c, "", errors.New("无法读取凭证文件")
			}
			e = json.Unmarshal(b, &token)
		} else {
			token, e = keyring.Get("islander-"+s.Scope, alias)
		}
		if e != nil || token == "" {
			return c, "", errors.New("无法读取饼干，请解锁系统凭证库或重新导入")
		}
		return c, token, nil
	}
	return Cookie{}, "", errors.New("饼干别名不存在")
}
func (s *Store) Use(alias string) error {
	return s.change(func(v *State) error {
		if alias == "" {
			v.Active = ""
			return nil
		}
		for _, c := range v.Cookies {
			if c.Alias == alias {
				v.Active = alias
				return nil
			}
		}
		return errors.New("饼干别名不存在")
	})
}
func (s *Store) Remove(alias string) error {
	return s.change(func(v *State) error {
		for i, c := range v.Cookies {
			if c.Alias != alias {
				continue
			}
			var e error
			if c.Backend == "file" {
				e = os.Remove(filepath.Join(s.Dir, "secret-"+alias+".json"))
				if os.IsNotExist(e) {
					e = nil
				}
			} else {
				e = keyring.Delete("islander-"+s.Scope, alias)
			}
			if e != nil {
				return errors.New("移除凭证失败")
			}
			v.Cookies = append(v.Cookies[:i], v.Cookies[i+1:]...)
			if v.Active == alias {
				v.Active = ""
			}
			return nil
		}
		return errors.New("饼干别名不存在")
	})
}
func (s *Store) SaveDraft(d *forum.Draft) error { return s.saveDraft(d, true) }
func (s *Store) Drafts(alias string) ([]forum.Draft, error) {
	files, e := filepath.Glob(filepath.Join(s.Dir, "draft-*.json"))
	if e != nil {
		return nil, e
	}
	out := []forum.Draft{}
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			return nil, e
		}
		var d forum.Draft
		if e = json.Unmarshal(b, &d); e != nil {
			return nil, fmt.Errorf("草稿文件损坏：%s", filepath.Base(f))
		}
		if d.Cookie == alias {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out, nil
}
func (s *Store) DeleteDraft(id string) error {
	if !nameRE.MatchString(id) {
		return errors.New("无效草稿编号")
	}
	path := filepath.Join(s.Dir, "draft-"+id+".json")
	return lockedFile(path, func() error {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	})
}

// NewSite retains the exact legacy scope for Islander, including custom servers.
func NewSite(root, site, forumURL, userURL, backend string) (*Store, error) {
	if site == "" || site == "islander" {
		return New(root, forumURL, userURL, backend)
	}
	return New(root, site+"\n"+forumURL, userURL, backend)
}
