// Surf runs as a tray application by default and exposes explicit server,
// diagnostics, and update commands from the same executable.
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"surf-backend/internal/app"
	"surf-backend/internal/config"
)

func main() {
	prepareConsole(len(os.Args) > 1 && os.Args[1] != "update-helper")
	if len(os.Args) >= 2 && os.Args[1] == "update-helper" {
		if err := runUpdateHelper(os.Args[2:]); err != nil {
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
	case "daemon":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf daemon")
		} else {
			watchDesktopParent(os.Stdin, os.Exit)
			err = app.Serve()
		}
	case "status":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf status")
		} else {
			err = runStatusCommand()
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
		fmt.Printf("surf %s\nprotocol %s\n", config.AppVersion, config.NativeVersion)
		return
	case "pair":
		if len(args) != 1 {
			err = fmt.Errorf("usage: surf pair")
			break
		}
		err = runPairCommand()
	case "devices":
		err = runDevicesCommand(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "Usage: surf [--home PATH] [daemon|status|pair|devices|doctor|update|version]")
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		if args[0] == "daemon" {
			log.Printf("surf daemon: %v", err)
		}
		fmt.Fprintln(os.Stderr, "surf:", err)
		os.Exit(1)
	}
}

func watchDesktopParent(parent io.Reader, exit func(int)) {
	if os.Getenv("SURF_PARENT_GUARD") != "1" {
		return
	}
	go func() {
		_, _ = io.Copy(io.Discard, parent)
		log.Printf("surf daemon: desktop parent exited")
		exit(0)
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
