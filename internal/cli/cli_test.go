package cli

import (
	"bytes"
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
