package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
	"surf-backend/internal/app"
	"surf-backend/internal/control"
	"surf-backend/internal/process"
	"surf-backend/internal/protocol"
	"surf-backend/internal/web"
)

type browserCommand struct{ status, resume, force, yes bool }

func parseBrowserCommand(args []string) (browserCommand, error) {
	var command browserCommand
	seen := map[string]bool{}
	for _, arg := range args {
		if seen[arg] {
			return command, fmt.Errorf("duplicate option %s", arg)
		}
		seen[arg] = true
		switch arg {
		case "--status":
			command.status = true
		case "--resume":
			command.resume = true
		case "--force":
			command.force = true
		case "--yes":
			command.yes = true
		default:
			return command, fmt.Errorf("usage: surf browser [--status | --resume [--force]] [--yes]")
		}
	}
	if command.status && (command.resume || command.force || command.yes) || command.force && !command.resume {
		return command, fmt.Errorf("--force requires --resume; --status takes no other options")
	}
	return command, nil
}

func confirmBrowserChange(message string, yes bool) error {
	fmt.Fprintln(os.Stdout, message)
	if yes {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("confirmation required; rerun with --yes to accept")
	}
	fmt.Fprint(os.Stdout, "Continue? [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(line)) != "y" && strings.ToLower(strings.TrimSpace(line)) != "yes" {
		return fmt.Errorf("canceled; browser unchanged")
	}
	return nil
}

func runBrowserCommand(args []string) error {
	command, err := parseBrowserCommand(args)
	if err != nil {
		return err
	}
	admin, err := newLocalAdmin()
	if err == nil {
		if !process.Running(admin.descriptor.PID) {
			_ = control.RemoveOwned(admin.home, admin.descriptor.AdminToken)
			err = control.ErrNotRunning
		}
	}
	if errors.Is(err, control.ErrNotRunning) {
		if command.status {
			fmt.Println("Surf is stopped; no browser setup session is running.")
			return nil
		}
		if command.resume {
			return fmt.Errorf("no Surf server is running; there is nothing to resume")
		}
		fmt.Println("Standalone browser setup. The iPad cannot connect during this session.")
		fmt.Println("Close all setup windows to finish. Surf will not start automatically.")
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return app.ServeBrowserContext(ctx, nil, true)
	}
	if err != nil {
		return err
	}
	var state protocol.BrowserModeEvent
	if err := admin.request(http.MethodGet, web.APIRoot+"/admin/browser", nil, &state); err != nil {
		return err
	}
	if command.status {
		printBrowserStatus(state)
		return nil
	}
	action := "open"
	if command.resume {
		action = "resume"
	}
	if command.resume && state.Standalone {
		return fmt.Errorf("standalone setup has no streaming server; close the browser or run surf quit")
	}
	if command.force && !state.CanForce {
		return fmt.Errorf("force close is only available after a normal resume could not close the browser")
	}
	if !(action == "open" && state.State == "setup") && !(action == "resume" && state.State == "streaming") {
		message := "Open Surf's existing tabs on this computer? Devices will pause. Pages reload; unsaved work and active transfers may be lost."
		if command.resume {
			message = "Close the setup browser and resume on devices? Pages reload; unsaved work and active transfers may be lost."
		}
		if command.force {
			message = "Force close Surf's unresponsive browser? Recent profile changes and unsaved work may be lost."
		}
		if err := confirmBrowserChange(message, command.yes); err != nil {
			return err
		}
	}
	if err := admin.request(http.MethodPost, web.APIRoot+"/admin/browser", map[string]any{"action": action, "revision": state.Revision, "force": command.force}, nil); err != nil {
		return err
	}
	deadline := time.Now().Add(90 * time.Second)
	last := ""
	for {
		if err := admin.request(http.MethodGet, web.APIRoot+"/admin/browser", nil, &state); err != nil {
			return err
		}
		if state.State != last {
			printBrowserStatus(state)
			last = state.State
		}
		if state.State == "failed" || state.CanForce {
			return fmt.Errorf("%s", state.Message)
		}
		if action == "open" && state.State == "setup" || action == "resume" && state.State == "streaming" {
			return nil
		}
		if action == "open" && state.State == "streaming" {
			return fmt.Errorf("setup could not open; streaming was restored: %s", state.Message)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("browser transition continues in Surf; check surf browser --status")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func printBrowserStatus(state protocol.BrowserModeEvent) {
	fmt.Printf("Browser: %s on %s\n", terminalText(state.State), terminalText(state.Host))
	if state.Message != "" {
		fmt.Println(terminalText(state.Message))
	}
	if state.Standalone {
		fmt.Println("Standalone setup: close all browser windows or run surf quit to finish.")
	} else if state.State == "setup" {
		fmt.Println("Close all setup windows or run surf browser --resume to return to devices.")
	}
	if state.CanForce {
		fmt.Println("If normal close keeps failing: surf browser --resume --force")
	}
}
