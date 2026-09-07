package local

import (
	"context"
	"github.com/A-islander/islander-cli/internal/forum"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIdentityScopeAndPermissions(t *testing.T) {
	root := t.TempDir()
	s, e := New(root, "https://forum.example/", "https://user.example/", "file")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Import("daily", "secret-value", forum.User{ID: 1, Name: "name"}); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(s.Dir, "cookies.json"))
	if e != nil || strings.Contains(string(b), "secret-value") {
		t.Fatal("metadata contains credential")
	}
	st, e := os.Stat(filepath.Join(s.Dir, "secret-daily.json"))
	if e != nil {
		t.Fatal(e)
	}
	// Windows uses inherited ACLs rather than POSIX permission bits.
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
		t.Fatal("credential permissions")
	}
	_, token, e := s.Identity("daily")
	if e != nil || token != "secret-value" {
		t.Fatal("identity lookup")
	}
	other, _ := New(root, "http://test.example/", "https://user.example/", "file")
	if _, _, e = other.Identity("daily"); e == nil {
		t.Fatal("cross environment identity")
	}
	if e = s.Import("../escape", "x", forum.User{ID: 2}); e == nil {
		t.Fatal("unsafe alias")
	}
	if e = s.Import("daily", "overwrite", forum.User{ID: 2}); e == nil {
		t.Fatal("silent overwrite")
	}
	d := forum.Draft{Cookie: "daily", BoardID: 1, Body: "未完成"}
	if e = s.SaveDraft(&d); e != nil {
		t.Fatal(e)
	}
	ds, _ := s.Drafts("other")
	if len(ds) > 0 {
		t.Fatal("cross identity draft")
	}
	if e = s.Remove("daily"); e != nil {
		t.Fatal(e)
	}
	v, _ := s.Read()
	if v.Active != "" {
		t.Fatal("active credential retained")
	}
}
func TestFailedPublishRetainsUploadedDraft(t *testing.T) {
	uploads, posts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/img/upload" {
			uploads++
			w.Write([]byte(`{"code":200,"data":{"success":true,"data":{"url":"https://example.org/a.png"}}}`))
			return
		}
		posts++
		w.Write([]byte(`{"code":403,"msg":"rejected"}`))
	}))
	defer server.Close()
	s, _ := New(t.TempDir(), server.URL, server.URL, "file")
	c, _ := forum.New(server.URL, server.URL, "test-token")
	f := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(f, []byte("image"), 0600)
	d := forum.Draft{Cookie: "daily", BoardID: 1, Body: "hello", Files: []string{f}}
	saved, e := s.Publish(context.Background(), c, d)
	if e == nil {
		t.Fatal("failure accepted")
	}
	if len(saved.Files) != 0 || len(saved.Media) != 1 {
		t.Fatal("upload progress lost")
	}
	ds, e := s.Drafts("daily")
	if e != nil || len(ds) != 1 || len(ds[0].Files) != 0 {
		t.Fatal("recoverable draft missing")
	}
	if uploads != 1 || posts != 1 {
		t.Fatal("write retried automatically")
	}
}
