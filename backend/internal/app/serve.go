// Package app assembles and runs the Surf backend independently of its command-line
// entrypoint, allowing the desktop supervisor to carry the server in one file.
package app

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/grandcat/zeroconf"

	"surf-backend/internal/auth"
	"surf-backend/internal/browser"
	"surf-backend/internal/chromium"
	"surf-backend/internal/config"
	"surf-backend/internal/contentblocker"
	"surf-backend/internal/process"
	"surf-backend/internal/transport"
	"surf-backend/internal/web"
)

func Serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	instance, acquired, err := process.AcquireInstanceLock(
		filepath.Join(cfg.SurfHome, "server.lock"))
	if err != nil {
		return fmt.Errorf("lock Surf backend: %w", err)
	}
	if !acquired {
		return fmt.Errorf("another Surf backend is already using %s", cfg.Profile)
	}
	defer instance.Close()
	if err := Prepare(cfg); err != nil {
		return err
	}

	releaseChildren := process.ProtectChildren()
	defer releaseChildren()
	a, err := auth.New(cfg.Profile, cfg.SurfPassword, cfg.AuthDays)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	hub := transport.New()
	b, err := browser.New(cfg, hub)
	if err != nil {
		return err
	}
	hub.SetHandler(b)
	defer b.Shutdown()
	if err := b.Start(); err != nil {
		return err
	}
	if os.Getenv("SURF_ADVERTISE") != "0" {
		port := cfg.Port
		if value, err := strconv.Atoi(os.Getenv("SURF_ADVERTISE_PORT")); err == nil && value > 0 {
			port = value
		}
		ad, err := zeroconf.Register("Surf", "_surf._tcp", "local.", port,
			[]string{"path=/", "proto=http", "app=surf", "nv=" + config.NativeVersion}, nil)
		if err != nil {
			log.Printf("bonjour advertise failed: %v", err)
		} else {
			defer ad.Shutdown()
			log.Printf("bonjour advertised Surf on _surf._tcp port %d", port)
		}
	}
	srv := web.New(cfg, a, hub)
	srv.SetHealthCheck(b.Health)
	srv.SetStats(b.Stats)
	b.RegisterRoutes(srv)
	log.Printf("surf listening on %s:%d", cfg.BindAddr, cfg.Port)

	serverErr := make(chan error, 1)
	go func() { serverErr <- web.Listen(cfg.BindAddr, cfg.Port, srv.Handler()) }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		return err
	case <-b.Died():
		return fmt.Errorf("chromium connection lost")
	case signal := <-sig:
		log.Printf("received %s, shutting down", signal)
		return nil
	}
}

// Prepare resolves the browser and optional managed extensions.
func Prepare(cfg *config.Config) error {
	for _, dir := range []string{cfg.SurfHome, cfg.DownloadsDir, cfg.UploadsDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", dir, err)
		}
	}
	if err := chromium.PrepareProfile(cfg.Profile); err != nil {
		return err
	}
	if cfg.ChromePath != "" {
		log.Printf("runtime: using CHROME=%s", cfg.ChromePath)
	} else {
		log.Printf("runtime: resolving Chrome/Chromium (--headless=new)")
		var path, source string
		var err error
		path, source, err = chromium.Resolve(cfg.SurfHome)
		if err != nil {
			return err
		}
		cfg.ChromePath = path
		log.Printf("runtime: using %s at %s", source, path)
		if strings.HasPrefix(source, "managed ") {
			go func() {
				if err := chromium.UpdateManaged(cfg.SurfHome); err != nil {
					log.Printf("runtime: managed browser update check failed: %v", err)
				}
			}()
		}
	}
	if cfg.ContentBlocker {
		log.Printf("runtime: ensuring uBlock Origin Lite %s", contentblocker.Version)
		path, err := contentblocker.Ensure(cfg.SurfHome)
		if err != nil {
			return err
		}
		cfg.ContentBlockerPath = path
		log.Printf("runtime: content blocker ready at %s", path)
	}
	return nil
}
