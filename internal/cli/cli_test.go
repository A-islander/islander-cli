package cli

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/A-islander/islander-cli/internal/forum"
	"github.com/A-islander/islander-cli/internal/local"
	"net/http"
	"net/http/httptest"
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
