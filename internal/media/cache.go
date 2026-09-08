package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/gofrs/flock"
)

// ImageCache stores validated source bytes, so thumbnail decoding never
// replaces the larger image used by the attachment viewer. It is independent
// of terminal placements and does not carry forum credentials.
type ImageCache struct {
	dir        string
	maxBytes   int64
	maxEntries int
	maxAge     time.Duration
	mu         sync.Mutex
	keys       map[string]*imageCacheGate
}

type imageCacheGate struct {
	ch    chan struct{}
	users int
}

func ImageCacheDir(dataDir string) (string, error) {
	if dataDir != "" {
		return filepath.Join(dataDir, "cache", "images"), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "islander", "images"), nil
}

func NewImageCache(directory string) *ImageCache {
	return &ImageCache{dir: directory, maxBytes: 256 << 20, maxEntries: 1024,
		maxAge: 30 * 24 * time.Hour, keys: map[string]*imageCacheGate{}}
}

func imageCacheKey(u string) string {
	sum := sha256.Sum256([]byte(u))
	return hex.EncodeToString(sum[:]) + ".img"
}

func (c *ImageCache) LoadImage(ctx context.Context, u string) (image.Image, error) {
	return c.load(ctx, u, 1600, 1200, false)
}

func (c *ImageCache) LoadThumbnail(ctx context.Context, u string) (image.Image, error) {
	return c.load(ctx, u, 320, 192, false)
}

// ReloadImage bypasses a hit but retains the previous good file if the server
// fails or returns invalid data. Ordinary reopening can still use that file.
func (c *ImageCache) ReloadImage(ctx context.Context, u string) (image.Image, error) {
	return c.load(ctx, u, 1600, 1200, true)
}

// Only the same URL waits; cancelling one reader does not cancel another
// reader waiting to load it. Idle gates are removed rather than kept forever.
func (c *ImageCache) acquire(ctx context.Context, key string) (func(), error) {
	c.mu.Lock()
	g := c.keys[key]
	if g == nil {
		g = &imageCacheGate{ch: make(chan struct{}, 1)}
		c.keys[key] = g
	}
	g.users++
	c.mu.Unlock()
	drop := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		g.users--
		if g.users == 0 {
			delete(c.keys, key)
		}
	}
	select {
	case g.ch <- struct{}{}:
		return func() { <-g.ch; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

func (c *ImageCache) load(ctx context.Context, u string, width, height int, refresh bool) (image.Image, error) {
	if c == nil || c.dir == "" {
		return loadImage(ctx, u, width, height)
	}
	if !forum.SafeURL(u) {
		return nil, errors.New("附件链接无效")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := imageCacheKey(u)
	release, err := c.acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := filepath.Join(c.dir, key)
	if !refresh {
		if b, ok := c.read(path); ok {
			img, err := decodeImage(ctx, b, width, height)
			if err == nil {
				now := time.Now()
				_ = os.Chtimes(path, now, now)
				return img, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// A corrupt cache entry must not make a valid remote image unusable.
			_ = os.Remove(path)
		}
	}
	b, err := fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	img, err := decodeImage(ctx, b, width, height)
	if err != nil {
		return nil, err
	}
	// Cache availability is optional: a full or read-only disk must not break
	// an otherwise successful image preview.
	c.write(ctx, key, b)
	return img, nil
}

func (c *ImageCache) read(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > forum.MaxFile || time.Since(info.ModTime()) > c.maxAge {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, forum.MaxFile+1))
	return b, err == nil && len(b) <= forum.MaxFile
}

func (c *ImageCache) write(ctx context.Context, key string, b []byte) {
	if int64(len(b)) > c.maxBytes || ctx.Err() != nil || os.MkdirAll(c.dir, 0700) != nil {
		return
	}
	// Serialize pruning and replacement across processes. Do not wait for a
	// busy cache when the downloaded image is already ready to show.
	lock := flock.New(filepath.Join(c.dir, ".lock"))
	lockCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	ok, err := lock.TryLockContext(lockCtx, 20*time.Millisecond)
	if err != nil || !ok {
		return
	}
	defer lock.Unlock()
	f, err := os.CreateTemp(c.dir, ".download-*")
	if err != nil {
		return
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil || closeErr != nil || ctx.Err() != nil {
		return
	}
	if os.Rename(name, filepath.Join(c.dir, key)) != nil {
		return
	}
	c.prune()
}

func (c *ImageCache) prune() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type cacheFile struct {
		name string
		size int64
		used time.Time
	}
	files := []cacheFile{}
	var total int64
	for _, e := range entries {
		name := e.Name()
		if len(name) != 68 || !strings.HasSuffix(name, ".img") {
			continue
		}
		if _, err := hex.DecodeString(name[:64]); err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if time.Since(info.ModTime()) > c.maxAge && os.Remove(filepath.Join(c.dir, name)) == nil {
			continue
		}
		files = append(files, cacheFile{name, info.Size(), info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	count := len(files)
	for _, f := range files {
		if total <= c.maxBytes && count <= c.maxEntries {
			break
		}
		if os.Remove(filepath.Join(c.dir, f.name)) == nil {
			total -= f.size
			count--
		}
	}
}
