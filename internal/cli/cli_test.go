package cli

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpDoesNotInitializeServices(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"tui", "--help"}, {"reply", "create", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			a := &app{}
			cmd := a.root()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Usage:") {
				t.Fatalf("expected CLI help, got %q", out.String())
			}
			if a.client != nil || a.store != nil {
				t.Fatal("help initialized API or credential storage")
			}
		})
	}
}

func TestSiteSelectionAndExplicitReadIdentity(t *testing.T) {
	var cookie string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Islander authorization sent to X")
		}
		cookie = r.Header.Get("Cookie")
		fmt.Fprint(w, `[{"forums":[{"id":30,"name":"技术"}]}]`)
	}))
	defer server.Close()
	dir := t.TempDir()
	s, _ := local.NewSite(dir, "x", server.URL+"/", "", "file")
	if err := s.Import("daily", "opaque-cookie", forum.User{Key: "cookie"}); err != nil {
		t.Fatal(err)
	}
	for _, auth := range []bool{false, true} {
		a := &app{}
		cmd := a.root()
		args := []string{"--site", "x", "--forum-url", server.URL, "--data-dir", dir, "board", "list"}
		if auth {
			args = append(args, "--cookie", "daily")
		}
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		want := ""
		if auth {
			want = "userhash=opaque-cookie"
		}
		if cookie != want || a.site != "x" || a.store.Scope != s.Scope {
			t.Fatal("CLI default anonymity or site selection failed")
		}
	}
}

func TestExternalWritesFailBeforePreviewOrAuthentication(t *testing.T) {
	for _, command := range [][]string{{"thread", "create"}, {"post", "sage", "100"}, {"cookie", "register", "daily"}} {
		a := &app{}
		cmd := a.root()
		cmd.SetArgs(append([]string{"--site", "bog", "--data-dir", t.TempDir()}, command...))
		err := cmd.Execute()
		var apiErr *forum.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "unsupported" {
			t.Fatalf("%v: %v", command, err)
		}
	}
}

func TestUnknownCommandDoesNotOpenTUI(t *testing.T) {
	a := &app{}
	cmd := a.root()
	cmd.SetArgs([]string{"typo"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unknown command should fail")
	}
	if a.client != nil || a.store != nil {
		t.Fatal("unknown command initialized services")
	}
}

func TestExternalReplyPreviewDoesNotSendRequests(t *testing.T) {
	for _, site := range []string{"x", "bog"} {
		t.Run(site, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
			defer server.Close()
			dir := t.TempDir()
			store, _ := local.NewSite(dir, site, server.URL+"/", "", "file")
			token := "cookie"
			if site == "bog" {
				token = "bog_master=master; bog_sel=shadow"
			}
			if err := store.Import("daily", token, forum.User{Key: "cookie"}); err != nil {
				t.Fatal(err)
			}
			body := filepath.Join(t.TempDir(), "reply.txt")
			if err := os.WriteFile(body, []byte("reply"), 0600); err != nil {
				t.Fatal(err)
			}
			base := []string{"--site", site, "--forum-url", server.URL, "--data-dir", dir, "--cookie", "daily", "reply", "create", "--thread", "100", "--quote", "101", "--body-file", body}
			for _, flags := range [][]string{{"--dry-run"}, {"--confirm", "wrong-confirmation"}} {
				a := &app{}
				cmd := a.root()
				cmd.SetArgs(append(append([]string{}, base...), flags...))
				err := cmd.Execute()
				if flags[0] == "--dry-run" && err != nil {
					t.Fatal(err)
				}
				if flags[0] == "--confirm" && (err == nil || !strings.Contains(err.Error(), "确认码")) {
					t.Fatal("incorrect confirmation not rejected")
				}
			}
			if requests != 0 {
				t.Fatal("preview or wrong confirmation contacted external forum")
			}
		})
	}
}
