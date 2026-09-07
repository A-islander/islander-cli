package local

import (
	"context"
	"github.com/A-islander/islander-cli/internal/forum"
)

// Publish is called only after an identity/content/target preview is confirmed.
// Save successful uploads so a later failure does not require uploading again.
func (s *Store) Publish(ctx context.Context, c forum.Backend, d forum.Draft) (forum.Draft, error) {
	if !c.Capabilities().CanPublish(d.ThreadID > 0) {
		return d, forum.Unsupported("发帖")
	}
	if e := forum.ValidateDraft(c, d); e != nil {
		return d, e
	}
	if e := s.SaveDraft(&d); e != nil {
		return d, e
	}
	for len(d.Files) > 0 && !forum.InlineFiles(c) {
		m, e := c.Upload(ctx, d.Files[0])
		if e != nil {
			return d, e
		}
		d.Media = append(d.Media, m)
		d.Files = d.Files[1:]
		if e = s.SaveDraft(&d); e != nil {
			return d, e
		}
	}
	if e := c.Publish(ctx, d); e != nil {
		return d, e
	}
	// Keep a sent marker if deletion fails; a local cleanup error is not a failed post.
	d.Sent = true
	_ = s.SaveDraft(&d)
	_ = s.DeleteDraft(d.ID)
	return d, nil
}
