package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/A-islander/islander-cli/internal/media"
)

// tmux does not reliably forward graphics replies back into the application.
// In tmux we use quiet uploads only for a known client terminal, or explicit
// --images kitty. Ordinary terminals retain query/acknowledgement detection.
type imageTerminal struct{ tmux, silent, fallback bool }

type tmuxCommand func(...string) (string, error)

func runTmux(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "tmux", args...).Output()
	return strings.TrimSpace(string(b)), err
}

func prepareImageTerminal(mode string) (imageTerminal, func()) {
	if os.Getenv("TMUX") == "" || mode == "blocks" || mode == "off" {
		return imageTerminal{}, func() {}
	}
	return prepareTmuxImages(mode, os.Getenv("TMUX_PANE"), runTmux)
}

func prepareTmuxImages(mode, pane string, run tmuxCommand) (imageTerminal, func()) {
	result := imageTerminal{tmux: true, fallback: true}
	noop := func() {}
	if !strings.HasPrefix(pane, "%") {
		return result, noop
	}
	name, err := run("display-message", "-p", "-t", pane, "#{client_termname}")
	known := strings.Contains(name, "ghostty") || strings.Contains(name, "kitty")
	if mode != "kitty" && (err != nil || !known) {
		return result, noop
	}
	// Read both the effective and local value so inherited settings stay inherited.
	effective, err := run("show-options", "-A", "-p", "-v", "-t", pane, "allow-passthrough")
	if err != nil {
		return result, noop
	}
	if effective == "on" || effective == "all" {
		result.silent, result.fallback = true, false
		return result, noop
	}
	local, err := run("show-options", "-p", "-v", "-t", pane, "allow-passthrough")
	if err != nil {
		return result, noop
	}
	if _, err = run("set-option", "-p", "-t", pane, "allow-passthrough", "on"); err != nil {
		return result, noop
	}
	result.silent, result.fallback = true, false
	return result, func() {
		// Leave a user's intervening option change alone.
		now, err := run("show-options", "-p", "-v", "-t", pane, "allow-passthrough")
		if err != nil || now != "on" {
			return
		}
		if local == "" {
			_, _ = run("set-option", "-p", "-u", "-t", pane, "allow-passthrough")
		} else {
			_, _ = run("set-option", "-p", "-t", pane, "allow-passthrough", local)
		}
	}
}

func (m model) kittyRaw(data string) tea.Cmd {
	return tea.Raw(media.KittyTransport(data, m.imageTerminal.tmux))
}
func (m model) kittyUploadData(data string) string {
	if m.imageTerminal.silent {
		return strings.Replace(data, ",q=0,", ",q=2,", 1)
	}
	return data
}
