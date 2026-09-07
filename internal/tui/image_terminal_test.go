package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestTmuxPanePassthroughLifecycle(t *testing.T) {
	for _, local := range []string{"", "off", "on", "all"} {
		t.Run("local="+local, func(t *testing.T) {
			current := local
			var writes []string
			run := func(args ...string) (string, error) {
				command := strings.Join(args, " ")
				if !strings.Contains(command, "-t %12") {
					t.Fatalf("command must target only own pane: %s", command)
				}
				switch args[0] {
				case "display-message":
					return "xterm-ghostty", nil
				case "show-options":
					if current == "" && strings.Contains(command, "-A") {
						return "off", nil
					}
					return current, nil
				case "set-option":
					writes = append(writes, command)
					if strings.Contains(command, "-u") {
						current = ""
					} else {
						current = args[len(args)-1]
					}
					return "", nil
				}
				return "", errors.New("unexpected command")
			}
			terminal, restore := prepareTmuxImages("auto", "%12", run)
			if !terminal.tmux || !terminal.silent || terminal.fallback || current == "off" || current == "" {
				t.Fatal("Ghostty did not enable pane passthrough")
			}
			restore()
			if current != local {
				t.Fatalf("did not restore local/inherited setting: %q != %q", current, local)
			}
			if (local == "on" || local == "all") && len(writes) != 0 {
				t.Fatal("changed existing enabled option")
			}
		})
	}
}

func TestTmuxFallbackAndUserChanges(t *testing.T) {
	calls := 0
	terminal, _ := prepareTmuxImages("auto", "%2", func(args ...string) (string, error) { calls++; return "xterm-256color", nil })
	if !terminal.fallback || calls != 1 {
		t.Fatal("unknown terminal should not enable passthrough")
	}
	terminal, _ = prepareTmuxImages("kitty", "", func(args ...string) (string, error) { t.Fatal("missing pane ran tmux"); return "", nil })
	if !terminal.fallback {
		t.Fatal("missing tmux pane must fall back")
	}
	current := "off"
	terminal, restore := prepareTmuxImages("kitty", "%2", func(args ...string) (string, error) {
		switch args[0] {
		case "display-message":
			return "unknown", nil
		case "show-options":
			return current, nil
		case "set-option":
			current = args[len(args)-1]
			return "", nil
		}
		return "", nil
	})
	if !terminal.silent {
		t.Fatal("explicit kitty override failed")
	}
	current = "all"
	restore()
	if current != "all" {
		t.Fatal("overwrote user's intervening change")
	}
	terminal, _ = prepareTmuxImages("kitty", "%2", func(...string) (string, error) { return "", errors.New("tmux unavailable") })
	if !terminal.fallback {
		t.Fatal("failed configuration must fall back")
	}
}
