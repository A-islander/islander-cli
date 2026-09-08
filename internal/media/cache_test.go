package media

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cachePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestImageCachePersistsSourceBytesAcrossViewersAndRestarts(t *testing.T) {
	data := cachePNG(t, 800, 600)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials sent to image host")
		}
		w.Write(data)
	}))
	defer server.Close()
	dir := t.TempDir()
	c := NewImageCache(dir)
	u := server.URL + "/image.png?variant=original"
	ctx := context.Background()
	thumb, err := c.LoadThumbnail(ctx, u)
	if err != nil || thumb.Bounds().Dx() != 256 {
		t.Fatalf("thumbnail: %v", err)
	}
	full, err := c.LoadImage(ctx, u)
	if err != nil || full.Bounds().Dx() != 800 || requests.Load() != 1 {
		t.Fatalf("original lost pixels or refetched: %v", err)
	}
	if _, err = c.LoadImage(ctx, server.URL+"/image.png?variant=other"); err != nil || requests.Load() != 2 {
		t.Fatal("query variants shared an incorrect cache key")
	}
	server.Close()
	full, err = NewImageCache(dir).LoadImage(ctx, u)
	if err != nil || full.Bounds().Dx() != 800 {
		t.Fatalf("restart could not read offline cache: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, imageCacheKey(u)))
	if err != nil || !bytes.Equal(b, data) {
		t.Fatal("cache did not preserve source bytes")
	}
}

func TestImageCacheCoalescesConcurrentReads(t *testing.T) {
	data := cachePNG(t, 32, 16)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.Write(data) }))
	defer server.Close()
	c := NewImageCache(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := c.LoadImage(ctx, server.URL); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if requests.Load() != 1 || len(c.keys) != 0 {
		t.Fatal("duplicate download or leaked URL locks")
	}
}

func TestCancellingThumbnailDoesNotCancelWaitingViewer(t *testing.T) {
	data := cachePNG(t, 32, 16)
	started := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		w.Write(data)
	}))
	defer server.Close()
	c := NewImageCache(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, stop := context.WithCancel(ctx)
	errors := make(chan error, 2)
	go func() { _, err := c.LoadThumbnail(first, server.URL); errors <- err }()
	<-started
	go func() { _, err := c.LoadImage(ctx, server.URL); errors <- err }()
	stop()
	a, b := <-errors, <-errors
	if (a == nil) == (b == nil) || requests.Load() != 2 {
		t.Fatalf("cancelled reader affected waiting reader: %v, %v", a, b)
	}
	if _, err := c.LoadImage(ctx, server.URL); err != nil || requests.Load() != 2 {
		t.Fatal("successful waiter did not populate cache")
	}
}

func TestImageCacheRepairsCorruptionExpiresAndPreservesGoodRefresh(t *testing.T) {
	data := cachePNG(t, 32, 16)
	var requests, fail atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch fail.Load() {
		case 1:
			w.Write([]byte("not an image"))
		case 2:
			w.WriteHeader(503)
		default:
			w.Write(data)
		}
	}))
	defer server.Close()
	c := NewImageCache(t.TempDir())
	path := filepath.Join(c.dir, imageCacheKey(server.URL))
	ctx := context.Background()
	fail.Store(1)
	if _, err := c.LoadImage(ctx, server.URL); err == nil {
		t.Fatal("cached error body as image")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid image persisted")
	}
	fail.Store(0)
	if _, err := c.LoadImage(ctx, server.URL); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LoadImage(ctx, server.URL); err != nil || requests.Load() != 3 {
		t.Fatal("corrupt entry did not refetch")
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LoadImage(ctx, server.URL); err != nil || requests.Load() != 4 {
		t.Fatal("expired entry did not refetch")
	}
	fail.Store(2)
	if _, err := c.ReloadImage(ctx, server.URL); err == nil || requests.Load() != 5 {
		t.Fatal("refresh did not bypass cache")
	}
	if _, err := c.LoadImage(ctx, server.URL); err != nil || requests.Load() != 5 {
		t.Fatal("failed refresh destroyed good cache")
	}
	fail.Store(0)
	if _, err := c.ReloadImage(ctx, server.URL); err != nil || requests.Load() != 6 {
		t.Fatal("successful refresh failed")
	}
}

func TestImageCacheEvictsLeastRecentlyUsedAndExpiredFiles(t *testing.T) {
	data := cachePNG(t, 32, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer server.Close()
	for _, limit := range []string{"bytes", "entries"} {
		t.Run(limit, func(t *testing.T) {
			c := NewImageCache(t.TempDir())
			if limit == "bytes" {
				c.maxBytes = int64(len(data) * 2)
			} else {
				c.maxEntries = 2
			}
			ctx := context.Background()
			load := func(name string) {
				t.Helper()
				if _, err := c.LoadImage(ctx, server.URL+name); err != nil {
					t.Fatal(err)
				}
			}
			path := func(name string) string { return filepath.Join(c.dir, imageCacheKey(server.URL+name)) }
			load("/a")
			load("/b")
			old := time.Now().Add(-time.Hour)
			for _, name := range []string{"/a", "/b"} {
				if err := os.Chtimes(path(name), old, old); err != nil {
					t.Fatal(err)
				}
			}
			load("/a")
			load("/c")
			if _, err := os.Stat(path("/b")); !os.IsNotExist(err) {
				t.Fatal("least recently used entry was retained")
			}
			if _, err := os.Stat(path("/a")); err != nil {
				t.Fatal("recent hit was evicted")
			}
			old = time.Now().Add(-31 * 24 * time.Hour)
			if err := os.Chtimes(path("/a"), old, old); err != nil {
				t.Fatal(err)
			}
			load("/d")
			if _, err := os.Stat(path("/a")); !os.IsNotExist(err) {
				t.Fatal("expired entry was retained")
			}
		})
	}
}

func TestUnavailableImageCacheStillDisplaysImages(t *testing.T) {
	data := cachePNG(t, 32, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewImageCache(path).LoadImage(context.Background(), server.URL); err != nil {
		t.Fatal("unavailable cache broke preview", err)
	}
	dir, err := ImageCacheDir("custom-data")
	if err != nil || dir != filepath.Join("custom-data", "cache", "images") {
		t.Fatal("custom data directory ignored")
	}
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err = ImageCacheDir("")
	if err != nil || dir != filepath.Join(base, "islander", "images") {
		t.Fatal("default cache is not in system cache directory")
	}
}
