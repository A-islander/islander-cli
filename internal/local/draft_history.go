package local

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/A-islander/islander-cli/internal/forum"
)

// Embed the original draft fields so existing readers and draft lists remain
// compatible. Current content and its bounded history commit in one rename.
type draftRecord struct {
	forum.Draft
	EditVersion int           `json:"editHistoryVersion,omitempty"`
	Edits       []forum.Draft `json:"editHistory,omitempty"`
}

func sameDraft(a, b forum.Draft) bool {
	a.Updated, b.Updated = 0, 0
	return reflect.DeepEqual(a, b)
}

func (s *Store) saveDraft(d *forum.Draft, checkpoint bool) error {
	if d.Cookie == "" {
		return errors.New("草稿必须绑定饼干")
	}
	if d.ID == "" {
		var b [12]byte
		if _, err := rand.Read(b[:]); err != nil {
			return err
		}
		d.ID = hex.EncodeToString(b[:])
	}
	if !nameRE.MatchString(d.ID) {
		return errors.New("无效草稿编号")
	}
	path := filepath.Join(s.Dir, "draft-"+d.ID+".json")
	return lockedFile(path, func() error {
		var r draftRecord
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if json.Unmarshal(b, &r) != nil {
				return errors.New("草稿损坏，保留原文件")
			}
			if r.EditVersion != 0 && r.EditVersion != 1 {
				return errors.New("草稿历史版本不支持，保留原文件")
			}
			if r.Cookie != d.Cookie || r.ThreadID != d.ThreadID || r.BoardID != d.BoardID {
				return errors.New("草稿身份或目标不匹配")
			}
			if r.Sent && !d.Sent {
				return errors.New("草稿已发送，不能再次保存为待发送内容")
			}
			if len(r.Edits) == 0 && !r.Sent {
				r.Edits = append(r.Edits, r.Draft)
			}
		}
		d.Updated = time.Now().Unix()
		if d.Sent {
			r.Edits = nil
		} else if len(r.Edits) == 0 || ((checkpoint || d.Updated-r.Edits[len(r.Edits)-1].Updated >= 30) && !sameDraft(r.Edits[len(r.Edits)-1], *d)) {
			r.Edits = append(r.Edits, *d)
		}
		if len(r.Edits) > 20 {
			r.Edits = r.Edits[len(r.Edits)-20:]
		}
		r.Draft = *d
		r.EditVersion = 1
		return atomic(path, r)
	})
}

func (s *Store) AutoSaveDraft(d *forum.Draft) error { return s.saveDraft(d, false) }

func (s *Store) DraftEdits(id, alias string) ([]forum.Draft, error) {
	if !nameRE.MatchString(id) {
		return nil, errors.New("无效草稿编号")
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "draft-"+id+".json"))
	if err != nil {
		return nil, err
	}
	var r draftRecord
	if json.Unmarshal(b, &r) != nil {
		return nil, errors.New("草稿历史损坏")
	}
	if r.EditVersion != 0 && r.EditVersion != 1 {
		return nil, errors.New("草稿历史版本不支持")
	}
	if r.Cookie != alias || r.Sent {
		return nil, errors.New("草稿身份不匹配或已发送")
	}
	return r.Edits, nil
}
