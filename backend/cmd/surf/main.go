// Surf runs as a tray application by default and exposes explicit server,
// diagnostics, and update commands from the same executable.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"surf-backend/internal/app"
	"surf-backend/internal/config"
	"surf-backend/internal/control"
	"surf-backend/internal/process"
)

func main() {
	private := len(os.Args) >= 3 && os.Args[1] == "_internal"
	prepareConsole(!private)
	if private {
		var err error
		switch os.Args[2] {
		case "update-helper":
			err = runUpdateHelper(os.Args[3:])
		case "child-guard":
			err = process.RunChildGuardian(os.Args[3:])
		default:
			err = fmt.Errorf("unknown internal command")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "surf update:", err)
			os.Exit(1)
		}
		return
	}
	args, err := applyHomeFlag(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "surf:", err)
		os.Exit(1)
	}
	if len(args) == 0 {
		if err := runTray(); err != nil {
			fmt.Fprintln(os.Stderr, "surf:", err)
			os.Exit(1)
		}
		return
	}

	err = nil
	switch args[0] {
	case "serve":
		err = runServe(args[1:])
	case "status":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf status")
		} else {
			err = runStatusCommand()
		}
	case "quit":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf quit")
		} else {
			err = runQuitCommand()
		}
	case "doctor":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf doctor")
		} else {
			err = doctor()
		}
	case "update":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf update")
		} else {
			err = runCommandUpdate()
		}
	case "version":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf version")
			break
		}
		fmt.Printf("surf %s\ncompatibility %s\n", config.AppVersion, config.CompatibilityVersion)
		return
	case "pair":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf pair")
			break
		}
		err = runPairCommand()
	case "devices":
		err = runDevicesCommand(args[1:])
	case "logs":
		err = runLogsCommand(args[1:])
	case "clipboard":
		err = runClipboardCommand(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "Usage: surf [--home PATH] [serve|status|quit|pair|devices|clipboard|logs|doctor|update|version]")
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		if args[0] == "serve" {
			log.Printf("surf serve: %v", err)
		}
		fmt.Fprintln(os.Stderr, "surf:", err)
		os.Exit(1)
	}
}

func runServe(args []string) error {
	pair := false
	if len(args) == 1 && args[0] == "--pair" {
		pair = true
	} else if len(args) != 0 {
		return fmt.Errorf("usage: surf serve [--pair]")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	watchDesktopParent(os.Stdin, cancel)
	ready := make(chan control.Descriptor, 1)
	done := make(chan error, 1)
	go func() { done <- app.ServeContext(ctx, ready) }()
	if pair {
		select {
		case <-ready:
			go func() {
				if err := runPairCommandContext(ctx); err != nil && ctx.Err() == nil {
					log.Printf("surf pair: %v", err)
				}
			}()
		case err := <-done:
			return err
		}
	}
	return <-done
}

func watchDesktopParent(parent io.Reader, cancel context.CancelFunc) {
	if os.Getenv("SURF_PARENT_GUARD") != "1" {
		return
	}
	go func() {
		_, _ = io.Copy(io.Discard, parent)
		log.Printf("surf serve: desktop parent exited")
		cancel()
	}()
}

func applyHomeFlag(args []string) ([]string, error) {
	if len(args) == 0 || args[0] != "--home" {
		return args, nil
	}
	if len(args) < 2 || args[1] == "" {
		return nil, fmt.Errorf("usage: surf --home PATH COMMAND")
	}
	if err := os.Setenv("SURF_HOME", args[1]); err != nil {
		return nil, fmt.Errorf("set SURF_HOME: %w", err)
	}
	return args[2:], nil
}
