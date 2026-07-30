// Surf runs as a tray application by default and exposes explicit server,
// diagnostics, and update commands from the same executable.
package main

import (
	"fmt"
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
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "Usage: surf [serve|doctor|update|version]")
		os.Exit(2)
	}
	if len(os.Args) == 1 {
		if err := runTray(); err != nil {
			fmt.Fprintln(os.Stderr, "surf:", err)
			os.Exit(1)
		}
		return
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = app.Serve()
	case "doctor":
		err = doctor()
	case "update":
		err = runCommandUpdate()
	case "version":
		fmt.Printf("surf %s\nprotocol %s\n", config.AppVersion, config.NativeVersion)
		return
	default:
		fmt.Fprintln(os.Stderr, "Usage: surf [serve|doctor|update|version]")
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "surf:", err)
		os.Exit(1)
	}
}
